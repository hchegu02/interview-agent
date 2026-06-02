package retriever

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type RetrievalPipelineDeps struct {
	Vector   Retriever
	BM25     Retriever
	Rule     Retriever
	Reranker Reranker
}

type RetrievalPipeline struct {
	vector   Retriever
	bm25     Retriever
	rule     Retriever
	reranker Reranker
}

func NewRetrievalPipeline(deps RetrievalPipelineDeps) *RetrievalPipeline {
	return &RetrievalPipeline{vector: deps.Vector, bm25: deps.BM25, rule: deps.Rule, reranker: deps.Reranker}
}

func (p *RetrievalPipeline) Retrieve(ctx context.Context, q Query) ([]Result, error) {
	result, err := p.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	return result.Results, nil
}

type PipelineResult struct {
	Results    []Result
	RRFResults []Result
	Trace      RetrievalTrace
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
	rrfResults := MergeRRF(all, q.K, 60)
	trace.Stages = append(trace.Stages, stageTraceFromResults(StageRRF, rrfResults, 0, ""))
	results := rrfResults
	finalStage := StageRRF
	if p.reranker != nil {
		start := time.Now()
		reranked, err := p.reranker.Rerank(ctx, q, rrfResults)
		st := stageTraceFromResults(StageRerank, reranked, float64(time.Since(start).Microseconds())/1000, "")
		if err != nil {
			st.Error = err.Error()
			trace.Stages = append(trace.Stages, st)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return PipelineResult{RRFResults: rrfResults, Trace: trace}, err
			}
			trace.FallbackReasons = append(trace.FallbackReasons, StageRerank+": "+err.Error())
		} else {
			results = reranked
			finalStage = StageRerank
			trace.Stages = append(trace.Stages, st)
		}
	}
	for i, result := range results {
		trace.Final = append(trace.Final, ResultTrace{ID: result.ID, Rank: i + 1, Score: result.Score, Stage: finalStage})
	}
	return PipelineResult{Results: results, RRFResults: rrfResults, Trace: trace}, nil
}

func stageTraceFromResults(stage string, results []Result, durationMS float64, err string) StageTrace {
	st := StageTrace{Stage: stage, DurationMS: durationMS, Error: err}
	for _, result := range results {
		if result.ID == "" {
			continue
		}
		st.Items = append(st.Items, ResultTrace{ID: result.ID, Rank: len(st.Items) + 1, Score: result.Score, Stage: stage})
	}
	st.Count = len(st.Items)
	return st
}
