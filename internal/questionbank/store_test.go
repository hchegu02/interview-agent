package questionbank

import (
	"context"
	"testing"
)

func TestMemoryStore_ListFiltersAndPaginates(t *testing.T) {
	store := NewMemoryStore([]Item{
		{ID: "go-001", Content: "Go channel 底层结构", Tags: []string{"go", "channel"}, SkillCategory: "go", Difficulty: 3, Scenario: "fundamentals", Status: "active"},
		{ID: "redis-001", Content: "Redis 热 key 排查", Tags: []string{"redis", "performance"}, SkillCategory: "redis", Difficulty: 4, Scenario: "troubleshooting", Status: "active"},
		{ID: "draft-001", Content: "草稿题", SkillCategory: "go", Difficulty: 2, Status: "draft"},
	})

	got, err := store.List(context.Background(), Filter{
		Query:         "key",
		SkillCategory: "redis",
		Tags:          []string{"performance"},
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "redis-001" {
		t.Fatalf("items = %+v, want redis-001", got.Items)
	}
	if got.NextCursor != "" {
		t.Fatalf("NextCursor = %q, want empty", got.NextCursor)
	}
}

func TestMemoryStore_ListDefaultsToActive(t *testing.T) {
	store := NewMemoryStore([]Item{
		{ID: "active", Content: "active", Status: "active"},
		{ID: "draft", Content: "draft", Status: "draft"},
	})

	got, err := store.List(context.Background(), Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "active" {
		t.Fatalf("items = %+v, want only active item", got.Items)
	}
}

func TestMemoryStore_FacetsCountsActiveItems(t *testing.T) {
	store := NewMemoryStore([]Item{
		{ID: "go-001", Tags: []string{"go", "channel"}, SkillCategory: "go", Difficulty: 3, Scenario: "fundamentals", Status: "active"},
		{ID: "go-002", Tags: []string{"go"}, SkillCategory: "go", Difficulty: 4, Scenario: "troubleshooting", Status: "active"},
		{ID: "draft-001", Tags: []string{"redis"}, SkillCategory: "redis", Difficulty: 2, Scenario: "fundamentals", Status: "draft"},
	})

	got, err := store.Facets(context.Background())
	if err != nil {
		t.Fatalf("Facets: %v", err)
	}
	if got.SkillCategories["go"] != 2 || got.SkillCategories["redis"] != 0 {
		t.Fatalf("skill facets = %+v", got.SkillCategories)
	}
	if got.Tags["go"] != 2 || got.Tags["redis"] != 0 {
		t.Fatalf("tag facets = %+v", got.Tags)
	}
	if got.Difficulties[3] != 1 || got.Difficulties[4] != 1 {
		t.Fatalf("difficulty facets = %+v", got.Difficulties)
	}
}
