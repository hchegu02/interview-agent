package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"interview-agent/internal/domain"
)

func TestRunPassesCompleteSession(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.json")
	writeSession(t, sessionPath, completeSession())

	var stdout, stderr bytes.Buffer
	code := run(options{SessionPath: sessionPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	var summary verifySummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("unmarshal summary: %v\n%s", err, stdout.String())
	}
	if !summary.Pass || summary.FailureCount != 0 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestRunFailsWhenSessionMissingReportAndRetrievalTrace(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.json")
	writeSession(t, sessionPath, &domain.Session{ID: "s1"})

	var stdout, stderr bytes.Buffer
	code := run(options{SessionPath: sessionPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1, stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "report_missing") || !strings.Contains(out, "retrieval_trace_missing") {
		t.Fatalf("summary should include expected failures, got %s", out)
	}
}

func writeSession(t *testing.T, path string, sess *domain.Session) {
	t.Helper()
	raw, err := json.Marshal(sess)
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
}

func completeSession() *domain.Session {
	return &domain.Session{
		ID: "s1",
		Rounds: []domain.AnswerRound{{
			RoundID:  "r1",
			Question: domain.Question{ID: "q1", SkillCategory: "go"},
			Answer:   "GMP 包含 G/M/P，并涉及本地队列和 work stealing。",
			Evaluation: &domain.Evaluation{
				QuestionID: "q1",
				Score:      80,
				Strengths:  []string{"覆盖核心概念"},
				Weaknesses: []string{"排障细节不足"},
				Suggestion: "补充线上调度排查案例",
			},
		}},
		RetrievalTrace: &domain.RetrievalTrace{
			Query: "go concurrency",
			Final: []domain.RetrievalResultTrace{{
				ID:    "q1",
				Rank:  1,
				Score: 0.9,
			}},
		},
		Report: &domain.Report{
			SessionID:      "s1",
			OverallScore:   80,
			SkillBreakdown: map[string]int{"go": 80},
			TranscriptAnalysis: &domain.TranscriptAnalysis{
				Dimensions: []domain.TranscriptDimension{{Name: "技术相关性", Score: 80}},
			},
			DrillPlan:    []domain.DrillPlanItem{{Skill: "go", Reason: "继续提高深度"}},
			Improvements: []string{"补充项目排障细节"},
			NextSteps:    []string{"复盘 Go 并发案例"},
		},
	}
}
