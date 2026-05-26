package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RealEmbedder 走 OpenAI-compatible /embeddings 端点。
//
// 默认搭配 DashScope text-embedding-v4：
//   BaseURL = https://dashscope.aliyuncs.com/compatible-mode/v1
//   Model   = text-embedding-v4
//   Dim     = 1024（v4 默认；也可设 768/1536/2048，与 PG 表里 vector(N) 必须一致）
//
// DashScope 的兼容模式接口和 OpenAI 完全一致，请求/响应 schema 一样，
// 切换厂商只改 BaseURL+Key。这是用兼容协议的最大好处。
type RealEmbedder struct {
	BaseURL    string
	APIKey     string
	Model      string
	Dim        int
	HTTPClient *http.Client

	// 重试配置（与 RealChatModel 类似但更保守——embedding 通常更稳定）
	MaxRetries int
	BaseDelay  time.Duration
}

func NewRealEmbedder(baseURL, apiKey, model string, dim int, timeout time.Duration) *RealEmbedder {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &RealEmbedder{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		Dim:     dim,
		HTTPClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		MaxRetries: 3,
		BaseDelay:  300 * time.Millisecond,
	}
}

func (r *RealEmbedder) Dimension() int { return r.Dim }
func (r *RealEmbedder) Name() string   { return r.Model }

// embeddingRequest 是 OpenAI /embeddings 协议子集。
// DashScope 的兼容端点完全支持 input 数组，可以批量编码。
type embeddingRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"` // 显式声明维度，让 PG 与模型严格对齐
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// 可重试 / 永久错误的判定。
// embedding 包不引 llm 包是为了避免循环依赖，所以这边重新写一份分类——
// 后期如果两边逻辑漂移再统一抽到 commonerr 包。
var (
	errTransient = errors.New("embedding: transient error")
	errPermanent = errors.New("embedding: permanent error")
)

// Embed 批量编码。一次调用最多支持 25 条（DashScope 限制），
// 上层 reindex 工具会自己分片。
//
// 返回的 [][]float32 顺序与 texts 一致（依赖响应里的 index 排序）。
func (r *RealEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	body, err := json.Marshal(embeddingRequest{
		Model:      r.Model,
		Input:      texts,
		Dimensions: r.Dim,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: marshal: %v", errPermanent, err)
	}

	var lastErr error
	for attempt := 0; attempt < r.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out, err := r.doOnce(ctx, body, len(texts))
		if err == nil {
			return out, nil
		}
		lastErr = err
		if errors.Is(err, errPermanent) {
			return nil, err
		}
		if attempt == r.MaxRetries-1 {
			break
		}
		// 简单线性 + 指数退避混合，embedding 不太需要 jitter
		delay := r.BaseDelay * time.Duration(1<<attempt)
		if err := sleepCtx(ctx, delay); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("retry exhausted: %w", lastErr)
}

func (r *RealEmbedder) doOnce(ctx context.Context, body []byte, want int) ([][]float32, error) {
	url := r.BaseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: build req: %v", errPermanent, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.APIKey)

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: http: %v", errTransient, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read: %v", errTransient, err)
	}

	switch {
	case resp.StatusCode == 429 || (resp.StatusCode >= 500 && resp.StatusCode < 600) || resp.StatusCode == 408:
		return nil, fmt.Errorf("%w: status %d (%s)", errTransient, resp.StatusCode, truncate(string(respBody), 200))
	case resp.StatusCode >= 400:
		return nil, fmt.Errorf("%w: status %d (%s)", errPermanent, resp.StatusCode, truncate(string(respBody), 200))
	}

	var parsed embeddingResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("%w: unmarshal: %v", errPermanent, err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("%w: api: %s (code=%s)", errPermanent, parsed.Error.Message, parsed.Error.Code)
	}
	if len(parsed.Data) != want {
		return nil, fmt.Errorf("%w: got %d vectors, want %d", errPermanent, len(parsed.Data), want)
	}

	// 按 index 字段排序——OpenAI/DashScope 一般已按顺序返回，
	// 但 spec 不保证，所以 defensively 排一下。
	out := make([][]float32, want)
	for _, d := range parsed.Data {
		if d.Index < 0 || d.Index >= want {
			return nil, fmt.Errorf("%w: bad index %d", errPermanent, d.Index)
		}
		if len(d.Embedding) != r.Dim {
			return nil, fmt.Errorf("%w: vector dim %d, want %d", errPermanent, len(d.Embedding), r.Dim)
		}
		out[d.Index] = d.Embedding
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
