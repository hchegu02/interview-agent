package retriever

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type RetrievalPipelineDeps struct {
	Vector Retriever
	BM25   Retriever
	Rule   Retriever
}

type RetrievalPipeline struct {
	vector Retriever
	bm25   Retriever
	rule   Retriever
}

func NewRetrievalPipeline(deps RetrievalPipelineDeps) *RetrievalPipeline {
	return &RetrievalPipeline{vector: deps.Vector, bm25: deps.BM25, rule: deps.Rule}
}

func (p *RetrievalPipeline) Retrieve(ctx context.Context, q Query) ([]Result, error) {
	result, err := p.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	return result.Results, nil
}

type PipelineResult struct {
	Results []Result
	Trace   RetrievalTrace
}

func (p *RetrievalPipeline) Search(ctx context.Context, q Query) (PipelineResult, error) {
	stages := []struct {
		name string
		r    Retriever
	}{
		{name: StageVector, r: p.vector},
		{name: StageBM25, r: p.bm25},
		{name: StageRule, r: p.rule},
	}
	var all [][]StageResult
	trace := RetrievalTrace{Query: q.Text}
	attempted := 0
	succeeded := 0
	failed := 0
	for _, stage := range stages {
		if stage.r == nil {
			continue
		}
		attempted++
		start := time.Now()
		results, err := stage.r.Retrieve(ctx, q)
		stageItems := make([]StageResult, 0, len(results))
		st := StageTrace{Stage: stage.name, DurationMS: float64(time.Since(start).Microseconds()) / 1000}
		for _, result := range results {
			if result.ID == "" {
				continue
			}
			rank := len(stageItems) + 1
			stageItems = append(stageItems, StageResult{Result: result, Stage: stage.name, Rank: rank, StageScore: result.Score})
			st.Items = append(st.Items, ResultTrace{ID: result.ID, Rank: rank, Score: result.Score, Stage: stage.name})
		}
		st.Count = len(stageItems)
		if err != nil {
			failed++
			st.Error = err.Error()
			trace.Stages = append(trace.Stages, st)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return PipelineResult{Trace: trace}, err
			}
			trace.FallbackReasons = append(trace.FallbackReasons, stage.name+": "+err.Error())
			continue
		}
		succeeded++
		all = append(all, stageItems)
		trace.Stages = append(trace.Stages, st)
	}
	if attempted > 0 && succeeded == 0 && failed > 0 {
		return PipelineResult{Trace: trace}, fmt.Errorf("all attempted retrieval stages failed: %s", strings.Join(trace.FallbackReasons, "; "))
	}
	results := MergeRRF(all, q.K, 60)
	for i, result := range results {
		trace.Final = append(trace.Final, ResultTrace{ID: result.ID, Rank: i + 1, Score: result.Score, Stage: StageRRF})
	}
	return PipelineResult{Results: results, Trace: trace}, nil
}
