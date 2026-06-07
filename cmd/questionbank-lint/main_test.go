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

func TestLintItemsReportsDirtyQuestionContent(t *testing.T) {
	items := []questionbank.Item{{
		ID:             "dirty-agent-001",
		Content:        "有使用过吗你这个agent项目就是四个智能体 用langchain也可以实现啊 你用了langGraph 有哪些是用langchain不能实现的吗--（无法反驳..",
		Tags:           []string{"agent"},
		SkillCategory:  "ai_agent",
		Difficulty:     4,
		Scenario:       "project_experience",
		ExpectedPoints: []string{"Graph 编排"},
		Rubric:         map[string]string{"good": "ok"},
		SampleAnswer:   "Graph 编排有清晰状态。",
		FollowUpHints:  []string{"如何恢复？"},
	}}

	got := lintItems(items, lintOptions{MinExpectedPoints: 1, MinScenarioRatio: 0})

	if got.DirtyContentItems != 1 {
		t.Fatalf("DirtyContentItems = %d, want 1", got.DirtyContentItems)
	}
	if len(got.Issues) == 0 || !strings.Contains(got.Issues[0], "dirty question content") {
		t.Fatalf("issues = %v, want dirty content issue", got.Issues)
	}
}

func TestLintItemsReportsAdvisoryQuestionContentWithoutFailing(t *testing.T) {
	items := []questionbank.Item{{
		ID:             "cap-001",
		Content:        "CAP 是什么？为什么不能三者兼得？项目里怎么取舍？",
		Tags:           []string{"system_design"},
		SkillCategory:  "system-design",
		Difficulty:     4,
		Scenario:       "system-design",
		ExpectedPoints: []string{"CAP", "tradeoff"},
		Rubric:         map[string]string{"good": "ok"},
		SampleAnswer:   "CAP 需要按业务取舍。",
		FollowUpHints:  []string{"如何保证最终一致？"},
	}}

	got := lintItems(items, lintOptions{MinExpectedPoints: 1, MinScenarioRatio: 0})

	if got.DirtyContentItems != 0 || len(got.Issues) != 0 {
		t.Fatalf("dirty=%d issues=%v, want advisory only", got.DirtyContentItems, got.Issues)
	}
	if got.AdvisoryContentItems != 1 || len(got.Warnings) != 1 {
		t.Fatalf("advisory=%d warnings=%v, want one advisory warning", got.AdvisoryContentItems, got.Warnings)
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
	if !strings.Contains(err.String(), "load question bank") {
		t.Fatalf("stderr = %q", err.String())
	}
}
