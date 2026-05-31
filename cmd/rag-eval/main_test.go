package main

import (
	"context"
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
