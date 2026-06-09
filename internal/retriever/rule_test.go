package retriever

import (
	"context"
	"errors"
	"testing"
)

func TestRuleRetrieverFiltersBySkillAndDifficulty(t *testing.T) {
	r := NewRuleRetriever([]Result{
		{ID: "redis-zset", Content: "Redis ZSet", Category: "redis", Difficulty: 3, Tags: []string{"zset"}},
		{ID: "go-gmp", Content: "Go GMP", Category: "go", Difficulty: 4, Tags: []string{"gmp"}},
	})
	got, err := r.Retrieve(nil, Query{SkillCategories: []string{"redis"}, DifficultyMin: 2, DifficultyMax: 3, K: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "redis-zset" {
		t.Fatalf("got %+v, want redis-zset only", got)
	}
}

func TestRuleRetrieverMatchesTags(t *testing.T) {
	r := NewRuleRetriever([]Result{
		{ID: "mysql-mvcc", Content: "MySQL MVCC", Category: "mysql", Difficulty: 4, Tags: []string{"mvcc"}},
		{ID: "network-timewait", Content: "TIME_WAIT", Category: "network", Difficulty: 3, Tags: []string{"tcp"}},
	})
	got, err := r.Retrieve(nil, Query{FilterTags: []string{"mvcc"}, K: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "mysql-mvcc" {
		t.Fatalf("got %+v, want mysql-mvcc only", got)
	}
}

func TestRuleRetrieverExcludesIDs(t *testing.T) {
	r := NewRuleRetriever([]Result{
		{ID: "go-gmp", Content: "Go GMP", Category: "go", Difficulty: 3, Tags: []string{"go"}},
		{ID: "go-channel", Content: "Go channel", Category: "go", Difficulty: 3, Tags: []string{"go"}},
	})
	got, err := r.Retrieve(nil, Query{Tags: []string{"go"}, ExcludeIDs: []string{"go-gmp"}, K: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "go-channel" {
		t.Fatalf("got %+v, want go-channel only", got)
	}
}

func TestRuleRetrieverSkillCategoryIsHardFilter(t *testing.T) {
	r := NewRuleRetriever([]Result{
		{ID: "redis-zset", Content: "Redis ZSet", Category: "redis", Difficulty: 3, Tags: []string{"zset"}},
		{ID: "go-zset", Content: "Go tagged zset", Category: "go", Difficulty: 3, Tags: []string{"zset"}},
	})
	got, err := r.Retrieve(nil, Query{SkillCategories: []string{"redis"}, Tags: []string{"zset"}, K: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "redis-zset" {
		t.Fatalf("got %+v, want redis-zset only", got)
	}
}

func TestRuleRetrieverFilterTagsAreHardFilter(t *testing.T) {
	r := NewRuleRetriever([]Result{
		{ID: "mysql-index", Content: "MySQL Index", Category: "mysql", Difficulty: 3, Tags: []string{"index"}},
		{ID: "mysql-mvcc", Content: "MySQL MVCC", Category: "mysql", Difficulty: 4, Tags: []string{"mvcc"}},
	})
	got, err := r.Retrieve(nil, Query{SkillCategories: []string{"mysql"}, FilterTags: []string{"mvcc"}, K: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "mysql-mvcc" {
		t.Fatalf("got %+v, want mysql-mvcc only", got)
	}
}

func TestRuleRetrieverDifficultyRangeIsHardFilter(t *testing.T) {
	r := NewRuleRetriever([]Result{
		{ID: "redis-basic", Content: "Redis Basic", Category: "redis", Difficulty: 1, Tags: []string{"zset"}},
		{ID: "redis-zset", Content: "Redis ZSet", Category: "redis", Difficulty: 3, Tags: []string{"zset"}},
		{ID: "redis-advanced", Content: "Redis Advanced", Category: "redis", Difficulty: 5, Tags: []string{"zset"}},
	})
	got, err := r.Retrieve(nil, Query{
		SkillCategories: []string{"redis"},
		FilterTags:      []string{"zset"},
		DifficultyMin:   2,
		DifficultyMax:   4,
		K:               5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "redis-zset" {
		t.Fatalf("got %+v, want redis-zset only", got)
	}
}

func TestRuleRetrieverTagsAreSoftSignals(t *testing.T) {
	r := NewRuleRetriever([]Result{
		{ID: "redis-zset", Content: "Redis ZSet", Category: "redis", Difficulty: 3, Tags: []string{"zset"}},
		{ID: "go-gmp", Content: "Go GMP", Category: "go", Difficulty: 4, Tags: []string{"go"}},
	})
	got, err := r.Retrieve(nil, Query{Tags: []string{"go"}, K: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %+v, want both docs", got)
	}
	if got[0].ID != "go-gmp" || got[1].ID != "redis-zset" {
		t.Fatalf("got %+v, want go-gmp before redis-zset", got)
	}
}

func TestRuleRetrieverEmptyQueryDoesNotReturnAllDocs(t *testing.T) {
	r := NewRuleRetriever([]Result{
		{ID: "redis-zset", Content: "Redis ZSet", Category: "redis", Difficulty: 3, Tags: []string{"zset"}},
		{ID: "go-gmp", Content: "Go GMP", Category: "go", Difficulty: 4, Tags: []string{"go"}},
	})
	got, err := r.Retrieve(nil, Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want no docs", got)
	}
}

func TestRuleRetrieverWhitespaceTagsAreNotSoftSignals(t *testing.T) {
	r := NewRuleRetriever([]Result{
		{ID: "redis-zset", Content: "Redis ZSet", Category: "redis", Difficulty: 3, Tags: []string{"zset"}},
		{ID: "go-gmp", Content: "Go GMP", Category: "go", Difficulty: 4, Tags: []string{"go"}},
	})
	got, err := r.Retrieve(nil, Query{Tags: []string{"", " "}, K: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want no docs", got)
	}
}

func TestRuleRetrieverCanceledContextReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := NewRuleRetriever([]Result{{ID: "go", Content: "Go", Category: "go", Difficulty: 3, Tags: []string{"go"}}})
	_, err := r.Retrieve(ctx, Query{Tags: []string{"go"}, K: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got err %v, want context canceled", err)
	}
}

func TestRuleRetrieverDifficultyIsSoftSignal(t *testing.T) {
	r := NewRuleRetriever([]Result{
		{ID: "redis-basic", Content: "Redis Basic", Category: "redis", Difficulty: 2, Tags: []string{"zset"}},
		{ID: "redis-zset", Content: "Redis ZSet", Category: "redis", Difficulty: 3, Tags: []string{"zset"}},
	})
	got, err := r.Retrieve(nil, Query{Difficulty: 3, K: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %+v, want both docs", got)
	}
	if got[0].ID != "redis-zset" || got[1].ID != "redis-basic" {
		t.Fatalf("got %+v, want redis-zset before redis-basic", got)
	}
}

func TestRuleRetrieverChecksContextDuringScan(t *testing.T) {
	docs := make([]Result, 10)
	for i := range docs {
		docs[i] = Result{ID: "go-doc", Content: "Go", Category: "go", Difficulty: 3, Tags: []string{"go"}}
	}
	r := NewRuleRetriever(docs)
	ctx := &cancelAfterErrContext{cancelAfter: 2}

	_, err := r.Retrieve(ctx, Query{Tags: []string{"go"}, K: 5})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got err %v, want context canceled", err)
	}
}
