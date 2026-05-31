package main

import (
	"strings"
	"testing"

	"interview-agent/internal/questionbank"
)

func TestLintItemsDetectsMissingMetadata(t *testing.T) {
	items := []questionbank.Item{
		{
			ID:             "go-001",
			Content:        "Go channel 怎么关闭？",
			Tags:           []string{"channel"},
			SkillCategory:  "go",
			Difficulty:     3,
			ExpectedPoints: []string{"close 唤醒接收者"},
		},
	}

	got := lintItems(items, lintOptions{MinExpectedPoints: 3, MinScenarioRatio: 0.8})

	if got.Total != 1 {
		t.Fatalf("total = %d", got.Total)
	}
	if got.MissingScenario != 1 {
		t.Fatalf("missing scenario = %d", got.MissingScenario)
	}
	if got.MissingExpectedPoints != 1 {
		t.Fatalf("missing expected points = %d", got.MissingExpectedPoints)
	}
	if len(got.Issues) == 0 {
		t.Fatal("expected issues")
	}
}

func TestLintItemsAcceptsCompleteMetadata(t *testing.T) {
	items := []questionbank.Item{
		{
			ID:             "go-001",
			Content:        "Go channel 的底层结构？",
			Tags:           []string{"channel", "go_concurrency"},
			SkillCategory:  "go",
			Difficulty:     3,
			Scenario:       "fundamentals",
			ExpectedPoints: []string{"hchan", "sendq", "recvq"},
			Rubric:         map[string]string{"good": "完整"},
			SampleAnswer:   "channel 底层是 hchan。",
			FollowUpHints:  []string{"close 怎么做？"},
		},
	}

	got := lintItems(items, lintOptions{MinExpectedPoints: 3, MinScenarioRatio: 0.8})

	if len(got.Issues) != 0 {
		t.Fatalf("issues = %v", got.Issues)
	}
	if got.CompleteMetadataRatio != 1 {
		t.Fatalf("complete ratio = %f", got.CompleteMetadataRatio)
	}
}

func TestNormalizeContent(t *testing.T) {
	got := normalizeContent("  Go   Channel\n底层  ")
	if got != "go channel 底层" {
		t.Fatalf("normalizeContent = %q", got)
	}
}

func TestRunMissingFileReturnsUsageError(t *testing.T) {
	var out, err strings.Builder
	code := run(lintOptions{SeedPath: "missing.json"}, &out, &err)
	if code != 2 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(err.String(), "load seed") {
		t.Fatalf("stderr = %q", err.String())
	}
}
