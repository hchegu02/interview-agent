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

func TestReportCompletenessVerifierRejectsInvalidReportValues(t *testing.T) {
	v := ReportCompletenessVerifier{}
	sess := &domain.Session{
		ID: "s1",
		Rounds: []domain.AnswerRound{{
			RoundID:  "r1",
			Question: domain.Question{ID: "q1", SkillCategory: "go"},
		}},
		Report: &domain.Report{
			SessionID:      "other",
			OverallScore:   101,
			SkillBreakdown: map[string]int{"": 80, "go": -1},
			TranscriptAnalysis: &domain.TranscriptAnalysis{
				Dimensions: []domain.TranscriptDimension{{Name: "", Score: 120}},
			},
			DrillPlan:    []domain.DrillPlanItem{{Skill: "", TargetScore: -1}},
			Improvements: []string{"补充项目证据"},
			NextSteps:    []string{"继续训练"},
		},
	}
	failures := v.VerifyReport(sess)
	wantCodes := map[string]bool{
		"report_session_id_mismatch":                false,
		"report_score_invalid":                      false,
		"report_skill_name_missing":                 false,
		"report_skill_score_invalid":                false,
		"report_transcript_dimension_name_missing":  false,
		"report_transcript_dimension_score_invalid": false,
		"report_drill_skill_missing":                false,
		"report_drill_target_score_invalid":         false,
	}
	for _, failure := range failures {
		if _, ok := wantCodes[failure.Code]; ok {
			wantCodes[failure.Code] = true
		}
	}
	for code, seen := range wantCodes {
		if !seen {
			t.Fatalf("missing failure code %s in %+v", code, failures)
		}
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
			Query:  "go concurrency",
			Stages: []domain.RetrievalStageTrace{{Stage: "rerank", Items: []domain.RetrievalResultTrace{{ID: "q1", Score: 1}}}},
		},
	}
	if failures := v.VerifyRetrieval(sess); len(failures) != 0 {
		t.Fatalf("retrieval failures = %+v", failures)
	}
}

func TestRetrievalTraceVerifierRejectsInvalidTraceValues(t *testing.T) {
	v := RetrievalTraceVerifier{}
	sess := &domain.Session{
		ID: "s1",
		RetrievalTrace: &domain.RetrievalTrace{
			Final: []domain.RetrievalResultTrace{
				{ID: "q1", Rank: 1, Score: 0.9, Stage: "rerank"},
				{ID: "q1", Rank: -1, Score: -0.1, Stage: "rerank"},
				{ID: "", Rank: 3, Score: 0.4, Stage: "rerank"},
			},
		},
	}
	failures := v.VerifyRetrieval(sess)
	wantCodes := map[string]bool{
		"retrieval_query_missing":      false,
		"retrieval_final_duplicate_id": false,
		"retrieval_rank_invalid":       false,
		"retrieval_score_invalid":      false,
		"retrieval_item_id_missing":    false,
	}
	for _, failure := range failures {
		if _, ok := wantCodes[failure.Code]; ok {
			wantCodes[failure.Code] = true
		}
	}
	for code, seen := range wantCodes {
		if !seen {
			t.Fatalf("missing failure code %s in %+v", code, failures)
		}
	}
}

func TestToolCallVerifier(t *testing.T) {
	v := ToolCallVerifier{ExpectedTool: "github.project_analyze"}
	events := []agentkit.HookEvent{
		{Type: agentkit.HookBeforeTool, TraceID: "tr1", Name: "github.project_analyze", Permission: agentkit.PermissionReadOnly},
		{Type: agentkit.HookAfterTool, TraceID: "tr1", Name: "github.project_analyze", Permission: agentkit.PermissionReadOnly},
	}
	failures := v.VerifyToolEvents(events)
	if len(failures) != 0 {
		t.Fatalf("failures = %+v", failures)
	}
}

func TestToolCallVerifierReportsInvalidToolEvents(t *testing.T) {
	v := ToolCallVerifier{ExpectedTool: "github.project_analyze"}
	events := []agentkit.HookEvent{
		{Type: agentkit.HookBeforeTool, TraceID: "tr1", Name: "github.project_analyze", Permission: agentkit.PermissionReadOnly},
		{Type: agentkit.HookAfterTool, TraceID: "tr1", Name: "github.project_analyze", Permission: agentkit.PermissionReadOnly, Error: "timeout"},
		{Type: agentkit.HookAfterTool, TraceID: "tr2", Name: "web.fetch", Permission: agentkit.PermissionReadOnly},
		{Type: agentkit.HookBeforeTool, TraceID: "tr3", Name: "report.write", Permission: agentkit.PermissionWriteReport},
	}
	failures := v.VerifyToolEvents(events)
	wantCodes := map[string]bool{
		"tool_call_failed":              false,
		"tool_after_without_before":     false,
		"tool_permission_not_read_only": false,
		"tool_before_without_after":     false,
	}
	for _, failure := range failures {
		if _, ok := wantCodes[failure.Code]; ok {
			wantCodes[failure.Code] = true
		}
	}
	for code, seen := range wantCodes {
		if !seen {
			t.Fatalf("missing failure code %s in %+v", code, failures)
		}
	}
}

func TestToolCallVerifierRequiresExpectedTool(t *testing.T) {
	v := ToolCallVerifier{ExpectedTool: "github.project_analyze"}
	events := []agentkit.HookEvent{
		{Type: agentkit.HookBeforeTool, TraceID: "tr1", Name: "web.fetch", Permission: agentkit.PermissionReadOnly},
		{Type: agentkit.HookAfterTool, TraceID: "tr1", Name: "web.fetch", Permission: agentkit.PermissionReadOnly},
	}
	failures := v.VerifyToolEvents(events)
	if len(failures) != 1 || failures[0].Code != "tool_expected_not_called" {
		t.Fatalf("failures = %+v", failures)
	}
}

func TestGraphStructureVerifier(t *testing.T) {
	v := GraphStructureVerifier{}
	if failures := v.VerifyInterviewGraph(); len(failures) != 0 {
		t.Fatalf("graph structure failures = %+v", failures)
	}
}
