package retriever

import (
	"context"
	"sort"
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
