package main

import (
	"encoding/json"
	"strings"
	"testing"

	"interview-agent/internal/embedding"
)

func TestBuildEmbedderRealUsesConstructorDefaults(t *testing.T) {
	t.Setenv("INTERVIEW_EMBEDDING_API_KEY", "dummy")

	got, err := buildEmbedder("real", "http://127.0.0.1:8000/v1", "BAAI/bge-m3", 1024)
	if err != nil {
		t.Fatalf("buildEmbedder: %v", err)
	}

	real, ok := got.(*embedding.RealEmbedder)
	if !ok {
		t.Fatalf("got %T, want *embedding.RealEmbedder", got)
	}
	if real.HTTPClient == nil {
		t.Fatal("HTTPClient is nil")
	}
	if real.MaxRetries <= 0 {
		t.Fatalf("MaxRetries = %d, want positive", real.MaxRetries)
	}
	if real.BaseDelay <= 0 {
		t.Fatalf("BaseDelay = %v, want positive", real.BaseDelay)
	}
}

func TestUpsertSQLWritesExpectedPoints(t *testing.T) {
	if !strings.Contains(upsertSQL, "expected_points") {
		t.Fatal("upsert SQL should write expected_points")
	}
	if !strings.Contains(upsertSQL, "EXCLUDED.expected_points") {
		t.Fatal("upsert SQL should update expected_points on conflict")
	}
}

func TestUpsertSQLWritesQuestionMetadata(t *testing.T) {
	for _, marker := range []string{
		"scenario",
		"role_tags",
		"rubric",
		"sample_answer",
		"follow_up_hints",
		"locale",
		"status",
		"updated_at = now()",
	} {
		if !strings.Contains(upsertSQL, marker) {
			t.Fatalf("upsert SQL missing %q", marker)
		}
	}
}

func TestNormalizeSeedRowMetadataKeepsDatabaseArraysNonNull(t *testing.T) {
	row := seedRow{
		ID:            "go-003",
		Content:       "sync.Mutex 饥饿模式",
		SkillCategory: "go",
		Difficulty:    4,
	}

	normalizeSeedRowMetadata(&row)

	if row.Tags == nil {
		t.Fatal("Tags is nil")
	}
	if row.RoleTags == nil {
		t.Fatal("RoleTags is nil")
	}
	if row.ExpectedPoints == nil {
		t.Fatal("ExpectedPoints is nil")
	}
	if row.FollowUpHints == nil {
		t.Fatal("FollowUpHints is nil")
	}
	if row.Rubric == nil {
		t.Fatal("Rubric is nil")
	}
	raw, err := json.Marshal(row.Rubric)
	if err != nil {
		t.Fatalf("marshal rubric: %v", err)
	}
	if string(raw) != "{}" {
		t.Fatalf("rubric json = %s, want {}", raw)
	}
	if row.Locale != "zh-CN" {
		t.Fatalf("Locale = %q, want zh-CN", row.Locale)
	}
	if row.Status != "active" {
		t.Fatalf("Status = %q, want active", row.Status)
	}
}
