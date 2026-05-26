package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"
)

// RealChatModel 走 OpenAI-compatible HTTP API。
//
// 兼容厂商：DeepSeek、阿里通义（DashScope compatible-mode）、智谱、OpenRouter、OpenAI 本体。
// 切换只改 BaseURL + Model + APIKey，HTTP body 协议是同一套。
//
// 内置重试：网络抖动 / 429 / 5xx 自动指数退避，4xx 立即返回。
// 这跟 graph.WithRetry（节点级重试）是两层不同的概念——
// 这里治"短暂的传输层问题"，graph 那层治"节点逻辑层失败"（比如 LLM 给的 JSON 校验不过）。
type RealChatModel struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client

	// 重试配置
	MaxRetries int           // 总尝试次数（含首次），默认 3
	BaseDelay  time.Duration // 首次退避，默认 500ms
	MaxDelay   time.Duration // 单次退避上限，默认 8s
}

// NewRealChatModel 用合理默认值构造。
// timeout 是单次 HTTP 调用上限（不含重试），外层 ctx 可以更短。
func NewRealChatModel(baseURL, apiKey, model string, timeout time.Duration) *RealChatModel {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &RealChatModel{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		HTTPClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		MaxRetries: 3,
		BaseDelay:  500 * time.Millisecond,
		MaxDelay:   8 * time.Second,
	}
}

func (r *RealChatModel) Name() string { return r.Model }

// openAIRequest / openAIResponse 是 OpenAI Chat Completions 协议子集。
// 只放我们用到的字段，未知字段 json.Unmarshal 会忽略。
type openAIRequest struct {
	Model          string         `json:"model"`
	Messages       []Message      `json:"messages"`
	Temperature    float64        `json:"temperature,omitempty"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
	Stream         bool           `json:"stream,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"` // "json_object" 强制 JSON
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
	// 厂商可能在 error 字段里塞结构化错误（DashScope / 智谱常见）
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// Generate 是同步调用主入口。
//
// 重试策略：每次拿到 ErrTransient 就指数退避 + jitter 重试，
// 直到 MaxRetries 用完。ErrPermanent 立即返回。
// ctx 取消（超时 / 用户主动）立即中断，不再重试。
func (r *RealChatModel) Generate(ctx context.Context, messages []Message, opts Options) (*Response, error) {
	modelName := opts.Model
	if modelName == "" {
		modelName = r.Model
	}

	req := openAIRequest{
		Model:       modelName,
		Messages:    messages,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
	}
	if opts.ResponseFormat == "json_object" {
		req.ResponseFormat = &responseFormat{Type: "json_object"}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal request: %v", ErrPermanent, err)
	}

	var lastErr error
	for attempt := 0; attempt < r.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		resp, err := r.doOnce(ctx, body)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		// 永久错误直接返回，不浪费重试预算
		if errors.Is(err, ErrPermanent) {
			return nil, err
		}

		// 最后一次失败不再 sleep
		if attempt == r.MaxRetries-1 {
			break
		}
		if sleepErr := sleepCtx(ctx, r.backoff(attempt)); sleepErr != nil {
			return nil, sleepErr
		}
	}
	return nil, fmt.Errorf("retry exhausted after %d attempts: %w", r.MaxRetries, lastErr)
}

// doOnce 发起一次 HTTP 调用，不做重试。
// 把"单次调用"和"重试循环"拆开便于单测——
// 测试可以直接用 httptest.Server 让 doOnce 返回各种状态码，
// 而不用 mock 整个时间和退避序列。
func (r *RealChatModel) doOnce(ctx context.Context, body []byte) (*Response, error) {
	url := r.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrPermanent, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+r.APIKey)

	httpResp, err := r.HTTPClient.Do(httpReq)
	if err != nil {
		// 网络错误 / 超时——都算 transient。
		// context.Canceled / DeadlineExceeded 是用户主动取消，单独保留语义。
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: http do: %v", ErrTransient, err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrTransient, err)
	}

	if cerr := classifyHTTPStatus(httpResp.StatusCode, string(respBody)); cerr != nil {
		return nil, cerr
	}

	var parsed openAIResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		// 200 但 body 解析失败——通常是上游网关返回 HTML 错误页。
		// 不重试，因为 schema 已经不对了，再试也是同样结果。
		return nil, fmt.Errorf("%w: unmarshal response: %v (body=%s)", ErrPermanent, err, truncate(string(respBody), 200))
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("%w: api error: %s (code=%s, type=%s)",
			ErrPermanent, parsed.Error.Message, parsed.Error.Code, parsed.Error.Type)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("%w: no choices", ErrEmptyResponse)
	}
	content := parsed.Choices[0].Message.Content
	if content == "" {
		return nil, ErrEmptyResponse
	}

	return &Response{
		Content:          content,
		Model:            parsed.Model,
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
	}, nil
}

// Stream 阶段 5 接入 SSE 时再实现；目前 Agent loop 不需要流式。
func (r *RealChatModel) Stream(ctx context.Context, messages []Message, opts Options) (<-chan Chunk, error) {
	return nil, errors.New("RealChatModel.Stream: 待 stage 5 SSE 实现")
}

// backoff 计算第 attempt 次重试前的延迟。
// 指数 2^attempt + jitter，避免雪崩。
func (r *RealChatModel) backoff(attempt int) time.Duration {
	d := r.BaseDelay * (1 << attempt)
	if d > r.MaxDelay {
		d = r.MaxDelay
	}
	// 25% jitter
	jitter := time.Duration(float64(d) * 0.25)
	if jitter > 1 {
		offset := time.Duration(rand.Int63n(int64(jitter)))
		if rand.Intn(2) == 0 {
			d += offset
		} else if d > offset {
			d -= offset
		}
	}
	return d
}
