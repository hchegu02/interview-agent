package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"testing"

	"interview-agent/internal/domain"
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

func TestSanitizeQueryTextRedactsSensitiveFragments(t *testing.T) {
	raw := `联系 me@example.com 手机 13800138000 地址 https://example.com/a?b=c api_key=sk-live token:"abc123"`
	got := sanitizeQueryText(raw)
	for _, forbidden := range []string{"me@example.com", "13800138000", "https://example.com", "sk-live", "abc123"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitizeQueryText leaked %q in %q", forbidden, got)
		}
	}
	for _, want := range []string{"[REDACTED_EMAIL]", "[REDACTED_PHONE]", "[REDACTED_URL]", "[REDACTED_SECRET]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("sanitizeQueryText = %q, want %s", got, want)
		}
	}
}

func TestExportQueriesFromSessions(t *testing.T) {
	sessions := []domain.Session{{
		ID: "sess-1",
		CandidatePool: []domain.Question{{
			ID:            "redis-001",
			Tags:          []string{"redis", "cache"},
			SkillCategory: "redis",
		}},
		RetrievalTrace: &domain.RetrievalTrace{
			Query:         "Redis 缓存击穿 token=abc123",
			OriginalQuery: "联系 me@example.com 讨论 Redis",
			Final:         []domain.RetrievalResultTrace{{ID: "redis-002"}},
		},
	}}

	got := exportQueriesFromSessions(sessions)
	if len(got) != 1 {
		t.Fatalf("queries = %+v, want one", got)
	}
	if got[0].ID != "sess-1:retrieval" || got[0].Skill != "redis" || len(got[0].CandidateIDs) != 2 {
		t.Fatalf("query = %+v", got[0])
	}
	if strings.Contains(got[0].Query, "abc123") || strings.Contains(got[0].OriginalQuery, "me@example.com") {
		t.Fatalf("query was not sanitized: %+v", got[0])
	}
}

func TestBuildCandidatePoolsFromSessionsMergesSources(t *testing.T) {
	sessions := []domain.Session{{
		ID:            "sess-pool",
		CandidatePool: []domain.Question{{ID: "q1"}, {ID: "q2"}},
		RetrievalTrace: &domain.RetrievalTrace{
			Query: "Go GMP 调度",
			Final: []domain.RetrievalResultTrace{
				{ID: "q2", Rank: 1, Score: 0.9},
			},
			Stages: []domain.RetrievalStageTrace{{
				Stage: "vector",
				Items: []domain.RetrievalResultTrace{
					{ID: "q1", Rank: 1, Score: 0.7},
					{ID: "q3", Rank: 2, Score: 0.4},
				},
			}},
		},
	}}

	bank := []questionbank.Item{
		{ID: "q3", Content: "Go GMP 调度模型", Tags: []string{"go"}},
		{ID: "q4", Content: "Redis AOF 持久化", Tags: []string{"redis"}},
	}
	got := buildCandidatePoolsFromSessions(sessions, bank)
	if len(got) != 1 || len(got[0].Candidates) != 4 {
		t.Fatalf("candidate pools = %+v, want one pool with four candidates", got)
	}
	byID := map[string]candidatePoolCandidate{}
	for _, candidate := range got[0].Candidates {
		byID[candidate.ID] = candidate
	}
	if byID["q2"].Sources["candidate_pool"].Rank != 2 || byID["q2"].Sources["final"].Rank != 1 {
		t.Fatalf("q2 sources = %+v", byID["q2"].Sources)
	}
	if byID["q2"].Rank != 1 || byID["q2"].Score != 0.9 {
		t.Fatalf("q2 rank/score = rank:%d score:%f", byID["q2"].Rank, byID["q2"].Score)
	}
	if byID["q1"].Sources["stage:vector"].Rank != 1 || byID["q3"].Sources["stage:vector"].Rank != 2 {
		t.Fatalf("stage sources = q1:%+v q3:%+v", byID["q1"].Sources, byID["q3"].Sources)
	}
	if byID["q3"].Sources["keyword"].Rank != 1 {
		t.Fatalf("q3 sources = %+v, want keyword source", byID["q3"].Sources)
	}
	if byID["q4"].Sources["random_negative"].Rank != 1 {
		t.Fatalf("q4 sources = %+v, want random_negative source", byID["q4"].Sources)
	}
}

func TestCandidatePoolCandidatesReuseExistingMetrics(t *testing.T) {
	pool := candidatePoolRecord{
		QueryID: "query-metrics",
		Candidates: []candidatePoolCandidate{
			{ID: "q-noise", Rank: 1},
			{ID: "q-hit", Rank: 2},
			{ID: "q-late", Rank: 6},
		},
	}
	var returned []string
	for _, candidate := range pool.Candidates {
		returned = append(returned, candidate.ID)
	}

	got := scoreReturnedIDs(caseResult{
		ID:          pool.QueryID,
		RelevantIDs: []string{"q-hit", "q-late"},
	}, returned, 10)

	if !got.HitAt5 || !got.HitAt10 {
		t.Fatalf("hit metrics = hit@5:%v hit@10:%v", got.HitAt5, got.HitAt10)
	}
	if got.RecallAt5 != 1 || got.RecallAt10 != 1 || got.MRRAtK != 0.5 || got.NDCGAtK <= 0 {
		t.Fatalf("metrics = recall@5:%f recall@10:%f mrr:%f ndcg:%f", got.RecallAt5, got.RecallAt10, got.MRRAtK, got.NDCGAtK)
	}
}

func TestRunExportQueriesWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	sessionsPath := dir + "/sessions.jsonl"
	outPath := dir + "/queries.jsonl"
	raw := `{"id":"sess-jsonl","retrieval_trace":{"query":"Redis AOF https://secret.local"}}
`
	if err := os.WriteFile(sessionsPath, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), options{Mode: "export-queries", SessionsPath: sessionsPath, OutFile: outPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var got exportedQuery
	if err := json.Unmarshal(bytes.TrimSpace(out), &got); err != nil {
		t.Fatalf("unmarshal output %q: %v", out, err)
	}
	if got.SessionID != "sess-jsonl" || strings.Contains(got.Query, "secret.local") {
		t.Fatalf("exported query = %+v", got)
	}
}

func TestRunBuildCandidatePoolAcceptsExportedQueries(t *testing.T) {
	dir := t.TempDir()
	queriesPath := dir + "/queries.jsonl"
	outPath := dir + "/pools.jsonl"
	raw := `{"id":"query-1","session_id":"sess-1","query":"Redis AOF token=abc123","candidate_ids":["redis-001"],"tags":["redis"],"skill":"redis"}
`
	if err := os.WriteFile(queriesPath, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), options{
		Mode:        "build-candidate-pool",
		QueriesPath: queriesPath,
		OutFile:     outPath,
		SeedPath:    "../../seeds/question_bank.json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var got candidatePoolRecord
	if err := json.Unmarshal(bytes.TrimSpace(out), &got); err != nil {
		t.Fatalf("unmarshal output %q: %v", out, err)
	}
	if got.QueryID != "query-1" || strings.Contains(got.Query, "abc123") {
		t.Fatalf("candidate pool record = %+v", got)
	}
	byID := map[string]candidatePoolCandidate{}
	for _, candidate := range got.Candidates {
		byID[candidate.ID] = candidate
	}
	if byID["redis-001"].Sources["exported_query"].Rank != 1 {
		t.Fatalf("redis-001 sources = %+v", byID["redis-001"].Sources)
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

func TestRunRejectsInvalidStageThresholdFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), options{MinStageRecallAt5: "rrf=NaN"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "ERROR: parse -min-stage-recall-at-5") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunAppliesStageThresholdGate(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), options{
		CasesPath:         "../../testdata/rag/golden_queries.jsonl",
		ConfigPath:        "../../config/config.yaml.example",
		SeedPath:          "../../seeds/question_bank.json",
		OutDir:            t.TempDir(),
		K:                 10,
		MinStageRecallAt5: "rrf=1.10",
		MinStageMRRAtK:    "rrf=0.10",
		MinGroupRecallAt5: 0,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "FAIL: stage rrf recall@5") {
		t.Fatalf("stderr = %q", stderr.String())
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
	if got.Cases[0].StageCandidates[retriever.StageVector][0] != "a" {
		t.Fatalf("stage candidates = %+v, want vector candidate a", got.Cases[0].StageCandidates)
	}
	if got.Cases[0].StageCandidates[retriever.StageRRF][0] != "b" {
		t.Fatalf("stage candidates = %+v, want fusion candidate b", got.Cases[0].StageCandidates)
	}
}

func TestEvaluateAddsPGVectorStageAliases(t *testing.T) {
	ctx := context.Background()
	embedder := embedding.NewMockEmbedder(8)
	r := fakePipelineSearcher{
		result: retriever.PipelineResult{
			Results:    []retriever.Result{{ID: "fusion-1"}},
			RRFResults: []retriever.Result{{ID: "fusion-1"}},
			Trace: retriever.RetrievalTrace{
				Stages: []retriever.StageTrace{
					{
						Stage: retriever.StageRule,
						Items: []retriever.ResultTrace{{ID: "tag-1", Rank: 1}},
					},
					{
						Stage: retriever.StageBM25,
						Items: []retriever.ResultTrace{{ID: "text-1", Rank: 1}},
					},
					{
						Stage: retriever.StageRRF,
						Items: []retriever.ResultTrace{{ID: "fusion-1", Rank: 1}},
					},
				},
			},
		},
	}

	got := evaluate(ctx, []evalCase{{
		ID:          "case-1",
		Query:       "Redis 持久化",
		RelevantIDs: []string{"fusion-1"},
	}}, 5, "pgvector", embedder, r)

	candidates := got.Cases[0].StageCandidates
	if candidates["tag"][0] != "tag-1" || candidates["text"][0] != "text-1" || candidates["fusion"][0] != "fusion-1" {
		t.Fatalf("pgvector aliases = %+v, want tag/text/fusion aliases", candidates)
	}
	if got.Stages["tag"].RecallAt5 != got.Stages[retriever.StageRule].RecallAt5 {
		t.Fatalf("tag stage alias = %+v, rule = %+v", got.Stages["tag"], got.Stages[retriever.StageRule])
	}
	if got.StageDeltas["fusion_vs_vector_recall_at_5"] != got.StageDeltas["rrf_vs_vector_recall_at_5"] {
		t.Fatalf("stage deltas = %+v, want fusion alias for rrf delta", got.StageDeltas)
	}
}

func TestEvaluateDoesNotDoubleCountExplicitRRFStage(t *testing.T) {
	ctx := context.Background()
	embedder := embedding.NewMockEmbedder(8)
	r := fakePipelineSearcher{
		result: retriever.PipelineResult{
			RRFResults: []retriever.Result{{ID: "b"}, {ID: "a"}},
			Results:    []retriever.Result{{ID: "a"}, {ID: "b"}},
			Trace: retriever.RetrievalTrace{
				Stages: []retriever.StageTrace{
					{
						Stage: retriever.StageRRF,
						Items: []retriever.ResultTrace{{ID: "b", Rank: 1}, {ID: "a", Rank: 2}},
					},
					{
						Stage: retriever.StageRerank,
						Items: []retriever.ResultTrace{{ID: "a", Rank: 1}, {ID: "b", Rank: 2}},
					},
				},
			},
		},
	}

	got := evaluate(ctx, []evalCase{{
		ID:          "case-1",
		Query:       "Redis AOF",
		RelevantIDs: []string{"a"},
	}}, 5, "seed", embedder, r)

	if got.Stages[retriever.StageRRF].CaseCount != 1 {
		t.Fatalf("rrf stage cases = %d, want 1", got.Stages[retriever.StageRRF].CaseCount)
	}
	if got.Stages[retriever.StageRRF].MRRAtK != 0.5 {
		t.Fatalf("rrf mrr = %f, want explicit trace order b,a", got.Stages[retriever.StageRRF].MRRAtK)
	}
	if got.Stages[retriever.StageRerank].MRRAtK != 1 {
		t.Fatalf("rerank mrr = %f, want rerank trace order a,b", got.Stages[retriever.StageRerank].MRRAtK)
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
		Vector:   seed,
		BM25:     retriever.NewBM25Retriever(docs),
		Rule:     retriever.NewRuleRetriever(docs),
		Reranker: retriever.NewLexicalReranker(),
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
