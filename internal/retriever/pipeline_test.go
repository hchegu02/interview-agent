package retriever

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRetrievalStageTypesPreserveSourceEvidence(t *testing.T) {
	item := StageResult{
		Result:     Result{ID: "go-gmp-001", Content: "讲一下 Go GMP 调度模型", Score: 0.75},
		Stage:      "bm25",
		Rank:       2,
		StageScore: 4.5,
		Reason:     "matched term gmp",
	}
	if item.ID != "go-gmp-001" {
		t.Fatalf("id = %q, want go-gmp-001", item.ID)
	}
	if item.Result.Score != 0.75 || item.StageScore != 4.5 {
		t.Fatalf("score evidence not preserved: %+v", item)
	}
	if item.Stage != "bm25" || item.Rank != 2 || item.Reason == "" {
		t.Fatalf("stage evidence not preserved: %+v", item)
	}
}

type fixedStageRetriever struct {
	results []Result
	err     error
}

func (f fixedStageRetriever) Retrieve(ctx context.Context, q Query) ([]Result, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]Result(nil), f.results...), nil
}

type errorReranker struct {
	err error
}

func (e errorReranker) Rerank(context.Context, Query, []Result) ([]Result, error) {
	return nil, e.err
}

func TestRetrievalPipelineUsesRRFResults(t *testing.T) {
	p := NewRetrievalPipeline(RetrievalPipelineDeps{
		Vector: fixedStageRetriever{results: []Result{{ID: "a"}, {ID: "b"}}},
		BM25:   fixedStageRetriever{results: []Result{{ID: "b"}, {ID: "c"}}},
		Rule:   fixedStageRetriever{results: []Result{{ID: "d"}}},
	})
	got, err := p.Retrieve(context.Background(), Query{Text: "go gmp", K: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != "b" {
		t.Fatalf("got %+v, want b promoted by RRF", got)
	}
}

func TestRetrievalPipelineFallsBackToRRFWhenRerankerFails(t *testing.T) {
	p := NewRetrievalPipeline(RetrievalPipelineDeps{
		Vector:   fixedStageRetriever{results: []Result{{ID: "a"}, {ID: "b"}}},
		BM25:     fixedStageRetriever{results: []Result{{ID: "b"}}},
		Reranker: errorReranker{err: errors.New("reranker unavailable")},
	})
	got, err := p.Search(context.Background(), Query{Text: "go gmp", K: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 2 || got.Results[0].ID != "b" {
		t.Fatalf("results = %+v, want RRF fallback results", got.Results)
	}
	if len(got.Trace.Final) == 0 || got.Trace.Final[0].Stage != StageRRF {
		t.Fatalf("final trace = %+v, want RRF fallback stage", got.Trace.Final)
	}
	if len(got.Trace.FallbackReasons) != 1 || !strings.Contains(got.Trace.FallbackReasons[0], StageRerank) {
		t.Fatalf("fallback reasons = %+v, want rerank fallback", got.Trace.FallbackReasons)
	}
}

func TestRetrievalPipelineSearchReturnsTrace(t *testing.T) {
	p := NewRetrievalPipeline(RetrievalPipelineDeps{
		Vector: fixedStageRetriever{results: []Result{{ID: "a", Score: 0.9}}},
		BM25:   fixedStageRetriever{results: []Result{{ID: "b", Score: 2.0}}},
	})
	got, err := p.Search(context.Background(), Query{Text: "go gmp", K: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Trace.Stages) != 3 {
		t.Fatalf("stages = %+v, want vector, bm25 and rrf", got.Trace.Stages)
	}
	if got.Trace.Stages[2].Stage != StageRRF {
		t.Fatalf("last stage = %+v, want rrf", got.Trace.Stages[2])
	}
	if len(got.Trace.Final) != len(got.Results) {
		t.Fatalf("final trace len = %d, results len = %d", len(got.Trace.Final), len(got.Results))
	}
}

func TestRetrievalPipelineAppliesRerankerAfterRRF(t *testing.T) {
	p := NewRetrievalPipeline(RetrievalPipelineDeps{
		Vector:   fixedStageRetriever{results: []Result{{ID: "generic", Content: "Redis 缓存淘汰", Score: 0.9}}},
		BM25:     fixedStageRetriever{results: []Result{{ID: "exact", Content: "Redis AOF rewrite 新写入处理", Score: 0.1}}},
		Reranker: NewLexicalReranker(),
	})
	got, err := p.Search(context.Background(), Query{Text: "Redis AOF rewrite", K: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RRFResults) != 2 {
		t.Fatalf("rrf results = %+v, want pre-rerank evidence", got.RRFResults)
	}
	if len(got.Results) != 2 || got.Results[0].ID != "exact" {
		t.Fatalf("results = %+v, want reranked exact match first", got.Results)
	}
	if len(got.Trace.Final) != 2 || got.Trace.Final[0].Stage != StageRerank {
		t.Fatalf("final trace = %+v, want rerank final trace", got.Trace.Final)
	}
	foundRerank := false
	for _, stage := range got.Trace.Stages {
		if stage.Stage == StageRerank {
			foundRerank = true
			if len(stage.Items) != 2 || stage.Items[0].ID != "exact" {
				t.Fatalf("rerank stage = %+v, want exact first", stage)
			}
		}
	}
	if !foundRerank {
		t.Fatalf("trace stages = %+v, want rerank stage", got.Trace.Stages)
	}
}

func TestRetrievalPipelineSearchRecordsStageErrorAndContinues(t *testing.T) {
	p := NewRetrievalPipeline(RetrievalPipelineDeps{
		Vector: fixedStageRetriever{err: errors.New("vector unavailable")},
		BM25:   fixedStageRetriever{results: []Result{{ID: "b"}}},
	})
	got, err := p.Search(context.Background(), Query{Text: "go gmp", K: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 1 || got.Results[0].ID != "b" {
		t.Fatalf("results = %+v, want bm25 result after vector error", got.Results)
	}
	if len(got.Trace.FallbackReasons) != 1 || got.Trace.Stages[0].Error == "" {
		t.Fatalf("trace = %+v, want vector error recorded", got.Trace)
	}
}

func TestRetrievalPipelineSearchErrorsWhenAllAttemptedStagesFail(t *testing.T) {
	p := NewRetrievalPipeline(RetrievalPipelineDeps{
		Vector: fixedStageRetriever{err: errors.New("vector unavailable")},
		BM25:   fixedStageRetriever{err: errors.New("bm25 unavailable")},
	})
	_, err := p.Search(context.Background(), Query{Text: "go gmp", K: 2})
	if err == nil {
		t.Fatal("Search err = nil, want error when all attempted stages fail")
	}
	if !strings.Contains(err.Error(), StageVector) || !strings.Contains(err.Error(), StageBM25) {
		t.Fatalf("Search err = %q, want stage names", err.Error())
	}
}

func TestRetrievalPipelineRetrieveErrorsWhenAllAttemptedStagesFail(t *testing.T) {
	p := NewRetrievalPipeline(RetrievalPipelineDeps{
		Vector: fixedStageRetriever{err: errors.New("vector unavailable")},
		BM25:   fixedStageRetriever{err: errors.New("bm25 unavailable")},
	})
	_, err := p.Retrieve(context.Background(), Query{Text: "go gmp", K: 2})
	if err == nil {
		t.Fatal("Retrieve err = nil, want error when all attempted stages fail")
	}
	if !strings.Contains(err.Error(), StageVector) || !strings.Contains(err.Error(), StageBM25) {
		t.Fatalf("Retrieve err = %q, want stage names", err.Error())
	}
}

type countingStageRetriever struct {
	calls   int
	results []Result
	err     error
}

func (c *countingStageRetriever) Retrieve(ctx context.Context, q Query) ([]Result, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	return append([]Result(nil), c.results...), nil
}

func TestRetrievalPipelineStopsOnContextCanceled(t *testing.T) {
	vector := &countingStageRetriever{err: context.Canceled}
	bm25 := &countingStageRetriever{results: []Result{{ID: "b"}}}
	p := NewRetrievalPipeline(RetrievalPipelineDeps{
		Vector: vector,
		BM25:   bm25,
	})
	got, err := p.Search(context.Background(), Query{Text: "go gmp", K: 2})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Search err = %v, want context.Canceled", err)
	}
	if vector.calls != 1 || bm25.calls != 0 {
		t.Fatalf("calls: vector=%d bm25=%d, want vector=1 bm25=0", vector.calls, bm25.calls)
	}
	if len(got.Trace.Stages) != 1 || got.Trace.Stages[0].Stage != StageVector {
		t.Fatalf("trace = %+v, want only canceled vector stage", got.Trace)
	}
}

func TestRetrievalPipelineSkipsEmptyIDsInTraceAndFinalResults(t *testing.T) {
	p := NewRetrievalPipeline(RetrievalPipelineDeps{
		Vector: fixedStageRetriever{results: []Result{{ID: "", Score: 1}, {ID: "a", Score: 0.9}}},
	})
	got, err := p.Search(context.Background(), Query{Text: "go gmp", K: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 1 || got.Results[0].ID != "a" {
		t.Fatalf("results = %+v, want only non-empty ID", got.Results)
	}
	if len(got.Trace.Stages) != 2 {
		t.Fatalf("stages = %+v, want vector and rrf stages", got.Trace.Stages)
	}
	stage := got.Trace.Stages[0]
	if stage.Count != 1 {
		t.Fatalf("stage count = %d, want valid item count 1", stage.Count)
	}
	if len(stage.Items) != 1 || stage.Items[0].ID != "a" {
		t.Fatalf("stage items = %+v, want only non-empty ID", stage.Items)
	}
	if got.Trace.Stages[1].Stage != StageRRF || got.Trace.Stages[1].Count != 1 {
		t.Fatalf("rrf stage = %+v, want valid rrf stage", got.Trace.Stages[1])
	}
	if len(got.Trace.Final) != 1 || got.Trace.Final[0].ID != "a" {
		t.Fatalf("final trace = %+v, want only non-empty ID", got.Trace.Final)
	}
}
