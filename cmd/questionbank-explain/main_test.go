package main

import "testing"

func TestBuildFilterCompactsCSVTags(t *testing.T) {
	got := buildFilter(options{
		Query:         "Redis",
		SkillCategory: "backend",
		Scenario:      "troubleshooting",
		Difficulty:    4,
		TagsCSV:       " redis, performance ,,",
		Status:        "draft",
		Limit:         50,
		Cursor:        "100",
	})

	if got.Query != "Redis" || got.SkillCategory != "backend" || got.Scenario != "troubleshooting" || got.Difficulty != 4 {
		t.Fatalf("filter basic fields = %+v", got)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "redis" || got.Tags[1] != "performance" {
		t.Fatalf("tags = %+v", got.Tags)
	}
	if got.Status != "draft" || got.Limit != 50 || got.Cursor != "100" {
		t.Fatalf("filter paging/status = %+v", got)
	}
}

func TestRunFailsWithoutPostgresDSN(t *testing.T) {
	code := run(options{ConfigPath: "../../config/config.yaml.example"})
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}
