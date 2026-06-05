package retriever

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

// Reranker 对融合后的候选做精排。
type Reranker interface {
	Rerank(ctx context.Context, q Query, candidates []Result) ([]Result, error)
}

// LexicalReranker 是本地确定性 rerank 默认实现。
type LexicalReranker struct{}

func NewLexicalReranker() *LexicalReranker {
	return &LexicalReranker{}
}

func (r *LexicalReranker) Rerank(ctx context.Context, q Query, candidates []Result) ([]Result, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	queryTokens := uniqueTokens(bm25Tokens(q.Text))
	out := append([]Result(nil), candidates...)
	for i := range out {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		lexicalScore := rerankLexicalScore(queryTokens, out[i])
		out[i].Score = 0.70*lexicalScore + 0.30*out[i].Score
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	return out, nil
}

func rerankLexicalScore(queryTokens []string, item Result) float64 {
	if len(queryTokens) == 0 {
		return 0
	}
	docTokens := uniqueTokens(bm25Tokens(docText(item)))
	if len(docTokens) == 0 {
		return 0
	}
	seen := make(map[string]bool, len(docTokens))
	for _, token := range docTokens {
		seen[token] = true
	}
	hits := 0
	for _, token := range queryTokens {
		if seen[token] {
			hits++
		}
	}
	return float64(hits) / float64(len(queryTokens))
}

// HTTPReranker 调用本地/内网 rerank 服务进行精排。
type HTTPReranker struct {
	endpoint string
	timeout  time.Duration
	client   *http.Client
}

type httpRerankRequest struct {
	Query      string                 `json:"query"`
	Candidates []httpRerankCandidate  `json:"candidates"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

type httpRerankCandidate struct {
	ID             string   `json:"id"`
	Content        string   `json:"content"`
	Tags           []string `json:"tags,omitempty"`
	Category       string   `json:"category,omitempty"`
	Difficulty     int      `json:"difficulty,omitempty"`
	ExpectedPoints []string `json:"expected_points,omitempty"`
}

type httpRerankResponse struct {
	Scores []httpRerankScore `json:"scores"`
}

type httpRerankScore struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
}

func NewHTTPReranker(endpoint string, timeout time.Duration) *HTTPReranker {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &HTTPReranker{
		endpoint: endpoint,
		timeout:  timeout,
		client:   &http.Client{Timeout: timeout},
	}
}

func (r *HTTPReranker) Rerank(ctx context.Context, q Query, candidates []Result) ([]Result, error) {
	if r == nil || r.endpoint == "" {
		return nil, fmt.Errorf("http reranker endpoint is empty")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	payload := httpRerankRequest{Query: q.Text, Candidates: make([]httpRerankCandidate, 0, len(candidates))}
	for _, c := range candidates {
		payload.Candidates = append(payload.Candidates, httpRerankCandidate{
			ID:             c.ID,
			Content:        c.Content,
			Tags:           append([]string(nil), c.Tags...),
			Category:       c.Category,
			Difficulty:     c.Difficulty,
			ExpectedPoints: append([]string(nil), c.ExpectedPoints...),
		})
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal rerank request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build rerank request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call rerank endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rerank endpoint status %d", resp.StatusCode)
	}
	var decoded httpRerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode rerank response: %w", err)
	}
	scores := make(map[string]float64, len(decoded.Scores))
	for _, item := range decoded.Scores {
		scores[item.ID] = item.Score
	}
	out := append([]Result(nil), candidates...)
	for i := range out {
		score, ok := scores[out[i].ID]
		if !ok {
			return nil, fmt.Errorf("rerank response missing score for %s", out[i].ID)
		}
		out[i].Score = score
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	return out, nil
}
