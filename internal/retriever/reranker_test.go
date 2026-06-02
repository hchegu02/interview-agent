package retriever

import (
	"context"
	"testing"
)

func TestLexicalRerankerPromotesExactQueryMatch(t *testing.T) {
	r := NewLexicalReranker()
	got, err := r.Rerank(context.Background(), Query{Text: "Redis AOF rewrite"}, []Result{
		{ID: "generic", Content: "Redis 性能优化和缓存淘汰策略", Score: 0.9},
		{ID: "exact", Content: "Redis AOF rewrite 期间新写入怎么处理", Score: 0.1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "exact" {
		t.Fatalf("reranked = %+v, want exact query match first", got)
	}
	if got[0].Score <= got[1].Score {
		t.Fatalf("scores = %+v, want rerank score descending", got)
	}
}

func TestLexicalRerankerKeepsStableOrderOnTie(t *testing.T) {
	r := NewLexicalReranker()
	got, err := r.Rerank(context.Background(), Query{Text: "unknown"}, []Result{
		{ID: "b", Content: "nothing", Score: 0.5},
		{ID: "a", Content: "nothing", Score: 0.5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("reranked = %+v, want stable ID order on tie", got)
	}
}
