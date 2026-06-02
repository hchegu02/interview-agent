package httpapi

import (
	"context"
	"testing"

	"interview-agent/internal/retriever"
)

type fakeSearchRetriever struct {
	result retriever.PipelineResult
}

func (f fakeSearchRetriever) Retrieve(context.Context, retriever.Query) ([]retriever.Result, error) {
	return f.result.Results, nil
}

func (f fakeSearchRetriever) Search(context.Context, retriever.Query) (retriever.PipelineResult, error) {
	return f.result, nil
}

func TestMetricsRetrieverPreservesSearch(t *testing.T) {
	wrapped := NewMetricsRetriever(fakeSearchRetriever{
		result: retriever.PipelineResult{
			Results: []retriever.Result{{ID: "q1"}},
			Trace: retriever.RetrievalTrace{
				Query: "go",
				Stages: []retriever.StageTrace{{
					Stage: retriever.StageRerank,
					Count: 1,
				}},
			},
		},
	}, newMetricsRecorder(), "pipeline")

	searcher, ok := wrapped.(interface {
		Search(context.Context, retriever.Query) (retriever.PipelineResult, error)
	})
	if !ok {
		t.Fatalf("wrapped retriever does not preserve Search")
	}
	got, err := searcher.Search(context.Background(), retriever.Query{Text: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Trace.Stages) != 1 || got.Trace.Stages[0].Stage != retriever.StageRerank {
		t.Fatalf("trace = %+v, want rerank trace", got.Trace)
	}
}
