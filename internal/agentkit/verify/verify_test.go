package verify

import (
	"testing"

	"interview-agent/internal/agentkit"
	"interview-agent/internal/domain"
)

func TestReportCompletenessVerifier(t *testing.T) {
	v := ReportCompletenessVerifier{}
	sess := &domain.Session{ID: "s1"}
	failures := v.VerifyReport(sess)
	if len(failures) == 0 {
		t.Fatal("missing report should fail")
	}

	sess.Report = &domain.Report{
		SessionID:      "s1",
		OverallScore:   80,
		SkillBreakdown: map[string]int{"go": 80},
		TranscriptAnalysis: &domain.TranscriptAnalysis{
			Dimensions: []domain.TranscriptDimension{{Name: "技术相关性", Score: 80}},
		},
		DrillPlan:    []domain.DrillPlanItem{{Skill: "redis", Reason: "low score"}},
		Improvements: []string{"加强 Redis 一致性表达"},
		NextSteps:    []string{"复盘缓存异常场景"},
	}
	failures = v.VerifyReport(sess)
	if len(failures) != 0 {
		t.Fatalf("complete report failures = %+v", failures)
	}
}

func TestReportCompletenessVerifierAllowsEmptySkillBreakdownWithoutRounds(t *testing.T) {
	v := ReportCompletenessVerifier{}
	sess := &domain.Session{
		ID: "s1",
		Report: &domain.Report{
			SessionID: "s1",
			TranscriptAnalysis: &domain.TranscriptAnalysis{
				Dimensions: []domain.TranscriptDimension{{Name: "样本量", Score: 0}},
			},
			DrillPlan:    []domain.DrillPlanItem{{Skill: "综合表达", Reason: "no rounds"}},
			Improvements: []string{"先完成一轮有效答题"},
			NextSteps:    []string{"继续模拟训练"},
		},
	}
	failures := v.VerifyReport(sess)
	if len(failures) != 0 {
		t.Fatalf("empty-round report failures = %+v", failures)
	}
}

func TestRetrievalTraceVerifier(t *testing.T) {
	v := RetrievalTraceVerifier{}
	if failures := v.VerifyRetrieval(&domain.Session{ID: "s1"}); len(failures) == 0 {
		t.Fatal("missing trace should fail")
	}
	sess := &domain.Session{
		ID: "s1",
		RetrievalTrace: &domain.RetrievalTrace{
			Stages: []domain.RetrievalStageTrace{{Stage: "rerank", Items: []domain.RetrievalResultTrace{{ID: "q1", Score: 1}}}},
		},
	}
	if failures := v.VerifyRetrieval(sess); len(failures) != 0 {
		t.Fatalf("retrieval failures = %+v", failures)
	}
}

func TestToolCallVerifier(t *testing.T) {
	v := ToolCallVerifier{}
	events := []agentkit.HookEvent{
		{Type: agentkit.HookAfterTool, Name: "report.write", Permission: agentkit.PermissionWriteReport},
		{Type: agentkit.HookAfterTool, Name: "questionbank.search", Permission: agentkit.PermissionReadOnly, Error: "timeout"},
	}
	failures := v.VerifyToolEvents(events)
	if len(failures) != 1 || failures[0].Code != "tool_call_failed" {
		t.Fatalf("failures = %+v", failures)
	}
}
