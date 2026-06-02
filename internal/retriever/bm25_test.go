package retriever

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBM25RetrieverRanksExactTechnicalTerm(t *testing.T) {
	r := NewBM25Retriever([]Result{
		{ID: "go-gmp", Content: "Go GMP 调度模型 G M P work stealing", Category: "go", Tags: []string{"go", "gmp"}},
		{ID: "go-channel", Content: "Go channel hchan sendq recvq", Category: "go", Tags: []string{"go", "channel"}},
	})
	got, err := r.Retrieve(nil, Query{Text: "GMP 调度", K: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].ID != "go-gmp" {
		t.Fatalf("top result = %+v, want go-gmp", got)
	}
}

func TestBM25RetrieverReturnsEmptyForEmptyQuery(t *testing.T) {
	r := NewBM25Retriever([]Result{{ID: "go-gmp", Content: "Go GMP"}})
	got, err := r.Retrieve(nil, Query{Text: "", K: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d results, want 0", len(got))
	}
}

func TestBM25RetrieverMatchesChineseSubstring(t *testing.T) {
	r := NewBM25Retriever([]Result{
		{ID: "scheduler", Content: "调度模型"},
		{ID: "channel", Content: "通道模型"},
	})
	got, err := r.Retrieve(nil, Query{Text: "调度", K: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].ID != "scheduler" {
		t.Fatalf("top result = %+v, want scheduler", got)
	}
}

func TestBM25RetrieverTrimsEnglishEdgePunctuation(t *testing.T) {
	r := NewBM25Retriever([]Result{
		{ID: "gmp", Content: "GMP."},
		{ID: "channel", Content: "channel"},
	})
	got, err := r.Retrieve(nil, Query{Text: "gmp", K: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].ID != "gmp" {
		t.Fatalf("top result = %+v, want gmp", got)
	}
}

func TestBM25RetrieverKeepsInternalEnglishSymbols(t *testing.T) {
	tests := []struct {
		name  string
		doc   string
		query string
		id    string
	}{
		{name: "node js", doc: "Use (node.js), runtime", query: "node.js", id: "node"},
		{name: "cpp", doc: "C++, memory model", query: "c++", id: "cpp"},
		{name: "csharp", doc: "C# async await.", query: "c#", id: "csharp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewBM25Retriever([]Result{
				{ID: tt.id, Content: tt.doc},
				{ID: "other", Content: "go scheduler channel"},
			})
			got, err := r.Retrieve(nil, Query{Text: tt.query, K: 1})
			if err != nil {
				t.Fatal(err)
			}
			if len(got) == 0 || got[0].ID != tt.id {
				t.Fatalf("top result = %+v, want %s", got, tt.id)
			}
		})
	}
}

func TestBM25RetrieverRepeatedQueryTokenDoesNotAmplifyScore(t *testing.T) {
	r := NewBM25Retriever([]Result{
		{ID: "go", Content: "go"},
		{ID: "rust", Content: "rust"},
	})
	one, err := r.Retrieve(nil, Query{Text: "go", K: 1})
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := r.Retrieve(nil, Query{Text: "go go go", K: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(one) == 0 || len(repeated) == 0 || one[0].ID != repeated[0].ID {
		t.Fatalf("top result changed: one=%+v repeated=%+v", one, repeated)
	}
	if one[0].Score != repeated[0].Score {
		t.Fatalf("score changed: one=%v repeated=%v", one[0].Score, repeated[0].Score)
	}
}

func TestBM25RetrieverCanceledContextReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := NewBM25Retriever([]Result{{ID: "go", Content: "go"}})
	_, err := r.Retrieve(ctx, Query{Text: "go", K: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestBM25RetrieverChecksContextDuringDocumentLoop(t *testing.T) {
	docs := []Result{
		{ID: "a", Content: "go"},
		{ID: "b", Content: "go"},
		{ID: "c", Content: "go"},
	}
	ctx := &cancelAfterErrContext{cancelAfter: 3}

	r := NewBM25Retriever(docs)
	_, err := r.Retrieve(ctx, Query{Text: "go", K: 3})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestBM25RetrieverTruncatesKWithStableIDOrdering(t *testing.T) {
	r := NewBM25Retriever([]Result{
		{ID: "b", Content: "go"},
		{ID: "a", Content: "go"},
		{ID: "c", Content: "go"},
	})
	got, err := r.Retrieve(nil, Query{Text: "go", K: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %+v", len(got), got)
	}
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("result order = %+v, want IDs a, b", got)
	}
}

type cancelAfterErrContext struct {
	context.Context
	cancelAfter int
	errCalls    int
}

func (c *cancelAfterErrContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (c *cancelAfterErrContext) Done() <-chan struct{} {
	return nil
}

func (c *cancelAfterErrContext) Err() error {
	c.errCalls++
	if c.errCalls >= c.cancelAfter {
		return context.Canceled
	}
	return nil
}

func (c *cancelAfterErrContext) Value(key interface{}) interface{} {
	return nil
}
