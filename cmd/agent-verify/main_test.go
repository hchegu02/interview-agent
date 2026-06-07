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

func TestRunPassesWithToolEvents(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.json")
	writeSession(t, sessionPath, completeSession())
	toolEventsPath := filepath.Join("..", "..", "testdata", "agent_verify", "pass_tool_events.json")

	var stdout, stderr bytes.Buffer
	code := run(options{SessionPath: sessionPath, ToolEventsPath: toolEventsPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
}

func TestRunPassesWithFailedMemoryObservation(t *testing.T) {
	sessionPath := filepath.Join("..", "..", "testdata", "agent_verify", "pass_session.json")
	memoryObservationsPath := filepath.Join("..", "..", "testdata", "agent_verify", "pass_memory_observations.json")

	var stdout, stderr bytes.Buffer
	code := run(options{SessionPath: sessionPath, MemoryObservationsPath: memoryObservationsPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d, stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
}

func TestRunFailsWithReportScoringMissingReviewFixture(t *testing.T) {
	sessionPath := filepath.Join("..", "..", "testdata", "agent_verify", "fail_report_scoring_missing_review.json")

	var stdout, stderr bytes.Buffer
	code := run(options{SessionPath: sessionPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1, stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "report_round_review_missing") {
		t.Fatalf("summary should include report_round_review_missing, got %s", stdout.String())
	}
}

func TestRunFailsWithInvalidMemoryObservation(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.json")
	memoryObservationsPath := filepath.Join(dir, "memory_observations.json")
	writeSession(t, sessionPath, completeSession())
	writeMemoryObservations(t, memoryObservationsPath, []map[string]any{
		{"status": "failed", "session_id": "s1", "attempts": 1, "elapsed_ms": 2},
	})

	var stdout, stderr bytes.Buffer
	code := run(options{SessionPath: sessionPath, MemoryObservationsPath: memoryObservationsPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1, stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "memory_error_class_missing") {
		t.Fatalf("summary should include memory_error_class_missing, got %s", stdout.String())
	}
}

func TestRunFailsWithInvalidToolEvents(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.json")
	toolEventsPath := filepath.Join(dir, "tool_events.json")
	writeSession(t, sessionPath, completeSession())
	writeToolEvents(t, toolEventsPath, []map[string]string{
		{"Type": "before_tool", "Name": "github.project_analyze", "Permission": "read_only", "TraceID": "tr1"},
		{"Type": "after_tool", "Name": "github.project_analyze", "Permission": "read_only", "TraceID": "tr1", "Error": "timeout"},
	})

	var stdout, stderr bytes.Buffer
	code := run(options{SessionPath: sessionPath, ToolEventsPath: toolEventsPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1, stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "tool_call_failed") {
		t.Fatalf("summary should include tool failure, got %s", stdout.String())
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

func writeToolEvents(t *testing.T, path string, events []map[string]string) {
	t.Helper()
	raw, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal tool events: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write tool events: %v", err)
	}
}

func writeMemoryObservations(t *testing.T, path string, observations []map[string]any) {
	t.Helper()
	raw, err := json.Marshal(observations)
	if err != nil {
		t.Fatalf("marshal memory observations: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write memory observations: %v", err)
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
	score80 := 80
	return &domain.Session{
		ID: "s1",
		Rounds: []domain.AnswerRound{{
			RoundID:  "r1",
			Question: domain.Question{ID: "q1", Content: "讲一下 Go 的 GMP 调度模型。", SkillCategory: "go", ExpectedPoints: []string{"G/M/P 定义", "本地队列"}},
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
			DrillPlan: []domain.DrillPlanItem{{Skill: "go", Reason: "继续提高深度"}},
			RoundReviews: []domain.RoundReview{{
				RoundID:             "r1",
				Number:              1,
				Type:                "main",
				QuestionID:          "q1",
				Question:            "讲一下 Go 的 GMP 调度模型。",
				Answer:              "GMP 包含 G/M/P，并涉及本地队列和 work stealing。",
				Score:               &score80,
				HitPoints:           []string{"覆盖核心概念"},
				MissedPoints:        []string{"排障细节不足"},
				Suggestion:          "补充线上调度排查案例",
				ExpectedPoints:      []string{"G/M/P 定义", "本地队列"},
				CountsTowardOverall: true,
			}},
			Improvements: []string{"补充项目排障细节"},
			NextSteps:    []string{"复盘 Go 并发案例"},
		},
	}
}
