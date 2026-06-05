package retriever

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestHTTPRerankerUsesReturnedScores(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"scores":[{"id":"b","score":0.95},{"id":"a","score":0.10}]}`))
	}))
	defer server.Close()

	r := NewHTTPReranker(server.URL, 0)
	got, err := r.Rerank(context.Background(), Query{Text: "redis lock"}, []Result{
		{ID: "a", Content: "generic redis", Score: 0.8},
		{ID: "b", Content: "redis distributed lock", Score: 0.2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "a" {
		t.Fatalf("reranked = %+v, want b,a", got)
	}
	if got[0].Score != 0.95 || got[1].Score != 0.10 {
		t.Fatalf("scores = %+v", got)
	}
}

func TestHTTPRerankerErrorsOnServerFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad model", http.StatusBadGateway)
	}))
	defer server.Close()

	r := NewHTTPReranker(server.URL, 0)
	if _, err := r.Rerank(context.Background(), Query{Text: "redis"}, []Result{{ID: "a", Content: "redis"}}); err == nil {
		t.Fatal("expected server failure")
	}
}
