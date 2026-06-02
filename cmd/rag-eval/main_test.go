package main

import (
	"context"
	"errors"
	"math"
	"testing"

	"interview-agent/internal/embedding"
	"interview-agent/internal/questionbank"
	"interview-agent/internal/retriever"
)

func TestRetrievalMetrics(t *testing.T) {
	got := []string{"a", "b", "c", "d"}
	rel := []string{"b", "d"}

	if recall := recallAt(got, rel, 2); recall != 0.5 {
		t.Fatalf("recall@2 = %f", recall)
	}
	if mrr := mrrAt(got, rel, 10); mrr != 0.5 {
		t.Fatalf("mrr = %f", mrr)
	}
	if ndcg := ndcgAt(got, rel, 10); ndcg <= 0 || ndcg > 1 {
		t.Fatalf("ndcg = %f", ndcg)
	}
}

func TestAggregate(t *testing.T) {
	s := aggregate([]caseResult{
		{ID: "go-q1", Tags: []string{"go", "go_concurrency"}, Skill: "go", RecallAt5: 1, RecallAt10: 1, MRRAtK: 1, NDCGAtK: 1, LatencyMS: 10},
		{ID: "redis-q1", Tags: []string{"redis"}, Skill: "redis", RecallAt5: 0, RecallAt10: 0.5, MRRAtK: 0.5, NDCGAtK: 0.5, LatencyMS: 20, Empty: true, Fallback: true, Error: "x"},
	}, 10, "seed")

	if s.RecallAt5 != 0.5 {
		t.Fatalf("recall@5 = %f", s.RecallAt5)
	}
	if s.EmptyRate != 0.5 || s.FallbackRate != 0.5 || s.ErrorCount != 1 {
		t.Fatalf("rates/errors = %+v", s)
	}
	if math.Abs(s.P95LatencyMS-20) > 0.001 {
		t.Fatalf("p95 = %f", s.P95LatencyMS)
	}
	if s.BySkill["go"].RecallAt10 != 1 || s.BySkill["redis"].RecallAt10 != 0.5 {
		t.Fatalf("by skill = %+v", s.BySkill)
	}
	if s.ByTag["go_concurrency"].CaseCount != 1 {
		t.Fatalf("by tag = %+v", s.ByTag)
	}
}

func TestAggregateBuildsStableGroupsAndWorstGroups(t *testing.T) {
	s := aggregate([]caseResult{
		{ID: "go-q1", Tags: []string{"go", "concurrency"}, Skill: "go", RecallAt5: 1, RecallAt10: 1, MRRAtK: 1, NDCGAtK: 1, LatencyMS: 10},
		{ID: "redis-q1", Tags: []string{"redis", "cache"}, Skill: "redis", RecallAt5: 0, RecallAt10: 0.5, MRRAtK: 0.5, NDCGAtK: 0.5, LatencyMS: 20},
	}, 10, "seed")

	if got := s.Groups["skill:redis"].RecallAt5; got != 0 {
		t.Fatalf("skill redis recall@5 = %f, want 0", got)
	}
	if got := s.Groups["tag:cache"].CaseCount; got != 1 {
		t.Fatalf("tag cache cases = %d, want 1", got)
	}
	worst := worstGroups(s.Groups, groupGateOptions{
		MinCases:     1,
		MinRecallAt5: 0.8,
		Limit:        1,
	})
	if len(worst) != 1 || worst[0].Group != "skill:redis" || worst[0].Metric != "recall_at_5" {
		t.Fatalf("worst groups = %+v", worst)
	}
}

func TestThresholdFailures(t *testing.T) {
	s := summary{RecallAt5: 0.7, RecallAt10: 0.86, MRRAtK: 0.93, NDCGAtK: 0.81}
	failures := thresholdFailures(s, options{
		MinRecallAt5:  0.8,
		MinRecallAt10: 0.8,
		MinMRRAtK:     0.9,
		MinNDCGAtK:    0.9,
	})

	if len(failures) != 2 {
		t.Fatalf("failures = %v, want recall@5 and ndcg@k failures", failures)
	}
}

func TestThresholdFailuresIncludesWorstGroups(t *testing.T) {
	s := summary{
		RecallAt5:  0.9,
		RecallAt10: 0.9,
		MRRAtK:     0.9,
		NDCGAtK:    0.9,
		Groups: map[string]groupMetric{
			"skill:redis": {CaseCount: 2, RecallAt5: 0.25, RecallAt10: 0.5, MRRAtK: 0.5, NDCGAtK: 0.5},
		},
	}
	failures := thresholdFailures(s, options{
		MinGroupCases:     2,
		MinGroupRecallAt5: 0.7,
	})
	if len(failures) != 1 {
		t.Fatalf("failures = %v, want one group failure", failures)
	}
	if failures[0] != "group skill:redis recall_at_5 0.250 below threshold 0.700 cases=2" {
		t.Fatalf("failure = %q", failures[0])
	}
}

func TestThresholdFailuresDisablesGroupGatesWithoutMinGroupCases(t *testing.T) {
	s := summary{
		RecallAt5:  0.9,
		RecallAt10: 0.9,
		MRRAtK:     0.9,
		NDCGAtK:    0.9,
		Groups: map[string]groupMetric{
			"skill:redis": {CaseCount: 2, RecallAt5: 0.25, RecallAt10: 0.5, MRRAtK: 0.5, NDCGAtK: 0.5},
		},
	}
	failures := thresholdFailures(s, options{
		MinGroupRecallAt5: 0.7,
	})
	if len(failures) != 0 {
		t.Fatalf("failures = %v, want no group failure when min group cases is zero", failures)
	}
}

func TestStageDeltaComputesImprovement(t *testing.T) {
	s := summary{
		Stages: map[string]groupMetric{
			"vector": {RecallAt5: 0.70, MRRAtK: 0.80},
			"rrf":    {RecallAt5: 0.76, MRRAtK: 0.85},
		},
	}
	got := stageDeltas(s)
	if got["rrf_vs_vector_recall_at_5"] != 0.06 {
		t.Fatalf("delta = %.2f, want 0.06", got["rrf_vs_vector_recall_at_5"])
	}
}

func TestParseStageThresholds(t *testing.T) {
	got, err := parseStageThresholds("rrf=0.75,rerank=0.80")
	if err != nil {
		t.Fatal(err)
	}
	if got["rrf"] != 0.75 || got["rerank"] != 0.80 {
		t.Fatalf("got %+v", got)
	}
}

func TestParseStageThresholdsEmptyInput(t *testing.T) {
	got, err := parseStageThresholds(" ")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want empty map", got)
	}
}

func TestParseStageThresholdsRejectsInvalidInput(t *testing.T) {
	for _, raw := range []string{"bad", "=0.75", "rrf=", "rrf=NaN", "rrf=+Inf"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := parseStageThresholds(raw); err == nil {
				t.Fatalf("parseStageThresholds(%q) returned nil error", raw)
			}
		})
	}
}

func TestStageThresholdFailures(t *testing.T) {
	s := summary{Stages: map[string]groupMetric{"rrf": {RecallAt5: 0.70}}}
	failures := stageThresholdFailures(s, map[string]float64{"rrf": 0.75}, "recall@5")
	if len(failures) != 1 {
		t.Fatalf("failures = %+v, want one", failures)
	}
}

func TestStageThresholdFailuresMissingStage(t *testing.T) {
	s := summary{Stages: map[string]groupMetric{}}
	failures := stageThresholdFailures(s, map[string]float64{"rrf": 0.75}, "recall@5")
	if len(failures) != 1 || failures[0] != "stage rrf missing metric recall@5" {
		t.Fatalf("failures = %+v", failures)
	}
}

func TestStageThresholdFailuresMRRAtK(t *testing.T) {
	s := summary{Stages: map[string]groupMetric{"rerank": {MRRAtK: 0.79}}}
	failures := stageThresholdFailures(s, map[string]float64{"rerank": 0.80}, "mrr@k")
	if len(failures) != 1 {
		t.Fatalf("failures = %+v, want one", failures)
	}
}

func TestStageThresholdFailuresUnsupportedMetric(t *testing.T) {
	s := summary{Stages: map[string]groupMetric{"rrf": {RecallAt5: 1, MRRAtK: 1}}}
	failures := stageThresholdFailures(s, map[string]float64{"rrf": 0.75}, "ndcg@k")
	if len(failures) != 1 || failures[0] != "unsupported stage metric ndcg@k" {
		t.Fatalf("failures = %+v", failures)
	}
}

func TestEvaluateCollectsPipelineStageMetrics(t *testing.T) {
	ctx := context.Background()
	embedder := embedding.NewMockEmbedder(8)
	r := fakePipelineSearcher{
		result: retriever.PipelineResult{
			Results: []retriever.Result{{ID: "b"}, {ID: "a"}},
			Trace: retriever.RetrievalTrace{
				Stages: []retriever.StageTrace{
					{
						Stage: retriever.StageVector,
						Items: []retriever.ResultTrace{{ID: "a", Rank: 1}},
					},
					{
						Stage: retriever.StageBM25,
						Items: []retriever.ResultTrace{{ID: "b", Rank: 1}},
					},
				},
			},
		},
	}

	got := evaluate(ctx, []evalCase{{
		ID:          "case-1",
		Query:       "Redis AOF",
		Tags:        []string{"redis"},
		RelevantIDs: []string{"b"},
	}}, 5, "seed", embedder, r)

	if got.RecallAt5 != 1 {
		t.Fatalf("final recall@5 = %f, want 1", got.RecallAt5)
	}
	if got.Stages[retriever.StageVector].RecallAt5 != 0 {
		t.Fatalf("vector recall@5 = %f, want 0", got.Stages[retriever.StageVector].RecallAt5)
	}
	if got.Stages[retriever.StageBM25].RecallAt5 != 1 {
		t.Fatalf("bm25 recall@5 = %f, want 1", got.Stages[retriever.StageBM25].RecallAt5)
	}
	if got.Stages[retriever.StageRRF].RecallAt5 != 1 {
		t.Fatalf("rrf recall@5 = %f, want 1", got.Stages[retriever.StageRRF].RecallAt5)
	}
	if got.StageDeltas["rrf_vs_vector_recall_at_5"] != 1 {
		t.Fatalf("stage deltas = %+v, want rrf vector recall improvement", got.StageDeltas)
	}
}

func TestEvaluateDoesNotCreateRRFStageWhenPipelineSearchFails(t *testing.T) {
	ctx := context.Background()
	embedder := embedding.NewMockEmbedder(8)
	r := fakePipelineSearcher{
		result: retriever.PipelineResult{
			Trace: retriever.RetrievalTrace{
				Stages: []retriever.StageTrace{{
					Stage: retriever.StageVector,
					Items: []retriever.ResultTrace{{ID: "a", Rank: 1}},
				}},
			},
		},
		err: errors.New("all stages failed"),
	}

	got := evaluate(ctx, []evalCase{{
		ID:          "case-1",
		Query:       "GMP",
		RelevantIDs: []string{"a"},
	}}, 5, "seed", embedder, r)

	if got.ErrorCount != 1 {
		t.Fatalf("error count = %d, want 1", got.ErrorCount)
	}
	if _, ok := got.Stages[retriever.StageRRF]; ok {
		t.Fatalf("rrf stage should not be created when pipeline search fails: %+v", got.Stages)
	}
	if got.Stages[retriever.StageVector].RecallAt5 != 1 {
		t.Fatalf("vector recall@5 = %f, want 1", got.Stages[retriever.StageVector].RecallAt5)
	}
}

func TestEvaluateStageMetricsUseAllCasesAsDenominator(t *testing.T) {
	ctx := context.Background()
	embedder := embedding.NewMockEmbedder(8)
	r := &scriptedPipelineSearcher{
		results: []retriever.PipelineResult{
			{
				Results: []retriever.Result{{ID: "a"}},
				Trace: retriever.RetrievalTrace{
					Stages: []retriever.StageTrace{{
						Stage: retriever.StageVector,
						Items: []retriever.ResultTrace{{ID: "a", Rank: 1}},
					}},
				},
			},
			{
				Results: []retriever.Result{{ID: "x"}},
				Trace:   retriever.RetrievalTrace{},
			},
		},
	}

	got := evaluate(ctx, []evalCase{
		{ID: "case-1", Query: "GMP", RelevantIDs: []string{"a"}},
		{ID: "case-2", Query: "MVCC", RelevantIDs: []string{"b"}},
	}, 5, "seed", embedder, r)

	if got.Stages[retriever.StageVector].CaseCount != 2 {
		t.Fatalf("vector stage cases = %d, want 2", got.Stages[retriever.StageVector].CaseCount)
	}
	if got.Stages[retriever.StageVector].RecallAt5 != 0.5 {
		t.Fatalf("vector recall@5 = %f, want 0.5", got.Stages[retriever.StageVector].RecallAt5)
	}
}

type fakePipelineSearcher struct {
	result retriever.PipelineResult
	err    error
}

func (f fakePipelineSearcher) Retrieve(context.Context, retriever.Query) ([]retriever.Result, error) {
	return f.result.Results, f.err
}

func (f fakePipelineSearcher) Search(context.Context, retriever.Query) (retriever.PipelineResult, error) {
	return f.result, f.err
}

type scriptedPipelineSearcher struct {
	results []retriever.PipelineResult
	idx     int
}

func (s *scriptedPipelineSearcher) Retrieve(context.Context, retriever.Query) ([]retriever.Result, error) {
	if len(s.results) == 0 {
		return nil, nil
	}
	return s.results[0].Results, nil
}

func (s *scriptedPipelineSearcher) Search(context.Context, retriever.Query) (retriever.PipelineResult, error) {
	if s.idx >= len(s.results) {
		return retriever.PipelineResult{}, nil
	}
	result := s.results[s.idx]
	s.idx++
	return result, nil
}

func TestSeedRetrieverUsesQueryTextForRanking(t *testing.T) {
	ctx := context.Background()
	e := embedding.NewMockEmbedder(64)
	r, err := newSeedRetriever(ctx, []questionbank.Item{
		{
			ID:             "redis-001",
			Content:        "Redis AOF 和 RDB 持久化差异是什么？AOF rewrite 期间新写入怎么处理？",
			Tags:           []string{"redis", "redis_persistence"},
			SkillCategory:  "redis",
			Difficulty:     3,
			ExpectedPoints: []string{"AOF", "RDB", "rewrite"},
			Status:         "active",
		},
		{
			ID:             "redis-006",
			Content:        "Redis 6 多线程 IO 为什么只处理网络读写而不是并发执行命令？",
			Tags:           []string{"redis", "performance"},
			SkillCategory:  "redis",
			Difficulty:     3,
			ExpectedPoints: []string{"IO threads", "main thread"},
			Status:         "active",
		},
	}, e)
	if err != nil {
		t.Fatal(err)
	}
	vecs, err := e.Embed(ctx, []string{"Redis AOF 和 RDB 持久化差异是什么？"})
	if err != nil {
		t.Fatal(err)
	}

	got, err := r.Retrieve(ctx, retriever.Query{
		Text:           "Redis AOF 和 RDB 持久化差异是什么？",
		QueryEmbedding: vecs[0],
		Tags:           []string{"redis", "aof", "rdb", "persistence"},
		Difficulty:     3,
		K:              2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].ID != "redis-001" {
		t.Fatalf("top result = %+v, want redis-001 first", got)
	}
}

func TestSeedPipelineKeepsStrongLexicalMatchOnTop(t *testing.T) {
	ctx := context.Background()
	e := embedding.NewMockEmbedder(64)
	seed, err := newSeedRetriever(ctx, []questionbank.Item{
		{
			ID:             "redis-001",
			Content:        "Redis AOF 和 RDB 持久化差异是什么？AOF rewrite 期间新写入怎么处理？",
			Tags:           []string{"redis", "redis_persistence"},
			SkillCategory:  "redis",
			Difficulty:     3,
			ExpectedPoints: []string{"AOF", "RDB", "rewrite"},
			Status:         "active",
		},
		{
			ID:             "redis-006",
			Content:        "Redis 6 多线程 IO 为什么只处理网络读写而不是并发执行命令？",
			Tags:           []string{"redis", "performance"},
			SkillCategory:  "redis",
			Difficulty:     3,
			ExpectedPoints: []string{"IO threads", "main thread"},
			Status:         "active",
		},
	}, e)
	if err != nil {
		t.Fatal(err)
	}
	docs := seedResults(seed.items)
	pipeline := retriever.NewRetrievalPipeline(retriever.RetrievalPipelineDeps{
		Vector: seed,
		BM25:   retriever.NewBM25Retriever(docs),
		Rule:   retriever.NewRuleRetriever(docs),
	})
	vecs, err := e.Embed(ctx, []string{"Redis AOF 和 RDB 持久化差异是什么？"})
	if err != nil {
		t.Fatal(err)
	}

	got, err := pipeline.Retrieve(ctx, retriever.Query{
		Text:           "Redis AOF 和 RDB 持久化差异是什么？",
		QueryEmbedding: vecs[0],
		Tags:           []string{"redis", "aof", "rdb", "persistence"},
		Difficulty:     3,
		K:              2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].ID != "redis-001" {
		t.Fatalf("pipeline top result = %+v, want redis-001 first", got)
	}
}
