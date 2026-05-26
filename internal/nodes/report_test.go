package nodes

import (
	"context"
	"errors"
	"testing"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

func buildReportSession() *domain.Session {
	mem := domain.NewWorkingMemory()
	mem.SkillCoverage["go"] = 0.82
	mem.SkillCoverage["redis"] = 0.46
	mem.ConfirmedSkills = []string{"go"}
	mem.WeakSkills = []string{"redis"}
	mem.DegradedReasons = map[string]string{
		"eval": "llm timeout",
		"rag":  "retrieve returned 0 results",
	}

	return &domain.Session{
		ID:            "sess-001",
		Status:        domain.StatusRunning,
		WorkingMemory: mem,
		Rounds: []domain.AnswerRound{
			{
				RoundID: "r1",
				Question: domain.Question{
					ID:            "q1",
					SkillCategory: "go",
				},
				Evaluation: &domain.Evaluation{
					QuestionID: "q1",
					Score:      78,
					Strengths:  []string{"GMP 结构清楚"},
					Weaknesses: []string{"没讲 work stealing"},
					Suggestion: "补 work stealing",
				},
			},
			{
				RoundID: "r2",
				Question: domain.Question{
					ID:            "q2",
					SkillCategory: "redis",
				},
				Evaluation: &domain.Evaluation{
					QuestionID: "q2",
					Score:      72,
					Strengths:  []string{"知道 set nx ex"},
					Weaknesses: []string{"lua 原子性不完整"},
					Suggestion: "补 lua 锁释放脚本",
				},
				RefinedEval: &domain.Evaluation{
					QuestionID: "q2",
					Score:      55,
					Strengths:  []string{"知道 set nx ex"},
					Weaknesses: []string{"lua 原子性不完整"},
					Suggestion: "补 lua 锁释放脚本",
				},
			},
			{
				RoundID: "r3",
				Question: domain.Question{
					ID:            "q3",
					SkillCategory: "mysql",
				},
				Evaluation: &domain.Evaluation{
					QuestionID: "q3",
					Score:      -1,
					Suggestion: "评估失败(降级)",
				},
			},
		},
	}
}

func TestReportNode_AggregatesSessionReport(t *testing.T) {
	sess := buildReportSession()
	node := NewReportNode()

	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("report node failed: %v", err)
	}
	if sess.Report == nil {
		t.Fatal("expected report to be written")
	}
	if sess.Status != domain.StatusCompleted {
		t.Errorf("status = %q, want completed", sess.Status)
	}
	if sess.PendingDecision != nil {
		t.Errorf("pending decision should be cleared, got %+v", sess.PendingDecision)
	}
	if sess.Report.SessionID != "sess-001" {
		t.Errorf("session_id = %q, want sess-001", sess.Report.SessionID)
	}
	// (78 + 55) / 2 = 66.5 -> 67
	if sess.Report.OverallScore != 67 {
		t.Errorf("overall_score = %d, want 67", sess.Report.OverallScore)
	}
	if sess.Report.SkillBreakdown["go"] != 82 {
		t.Errorf("go breakdown = %d, want 82", sess.Report.SkillBreakdown["go"])
	}
	if sess.Report.SkillBreakdown["redis"] != 46 {
		t.Errorf("redis breakdown = %d, want 46", sess.Report.SkillBreakdown["redis"])
	}
	if !contains(sess.Report.Highlights, "已确认技能：go") {
		t.Errorf("highlights = %v", sess.Report.Highlights)
	}
	if !contains(sess.Report.Improvements, "待加强技能：redis") {
		t.Errorf("improvements = %v", sess.Report.Improvements)
	}
	if !contains(sess.Report.NextSteps, "优先补强 redis 相关题目与知识点") {
		t.Errorf("next_steps = %v", sess.Report.NextSteps)
	}
	if !contains(sess.Report.NextSteps, "评估过程中部分环节降级：eval、rag；建议复测这些环节以提高报告可信度") {
		t.Errorf("next_steps should mention degraded components, got %v", sess.Report.NextSteps)
	}
}

func TestReportNode_NoRounds_StillBuildsEmptyReport(t *testing.T) {
	sess := &domain.Session{
		ID:            "sess-empty",
		Status:        domain.StatusRunning,
		WorkingMemory: domain.NewWorkingMemory(),
	}
	node := NewReportNode()

	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("report node failed: %v", err)
	}
	if sess.Report == nil {
		t.Fatal("expected report to be written")
	}
	if sess.Report.OverallScore != 0 {
		t.Errorf("overall_score = %d, want 0", sess.Report.OverallScore)
	}
	if len(sess.Report.NextSteps) == 0 {
		t.Error("expected a generic next step for empty report")
	}
}

func TestReportNode_InvalidStatus_Permanent(t *testing.T) {
	sess := &domain.Session{
		Status: domain.SessionStatus("weird"),
	}
	node := NewReportNode()

	err := node(context.Background(), sess)
	if !errors.Is(err, graph.ErrPermanent) {
		t.Errorf("expected ErrPermanent, got %v", err)
	}
}
