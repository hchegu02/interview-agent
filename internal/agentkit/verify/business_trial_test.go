package verify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBusinessTrialFeedbackVerifierValidPass(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "internal_trial", "business_feedback_pass.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var feedback BusinessTrialFeedback
	if err := json.Unmarshal(data, &feedback); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	failures := BusinessTrialFeedbackVerifier{}.VerifyFeedback(feedback)
	if len(failures) != 0 {
		t.Fatalf("valid feedback failures = %+v", failures)
	}
}

func TestBusinessTrialFeedbackVerifierRejectsIncompleteScript(t *testing.T) {
	feedback := validBusinessTrialFeedback()
	feedback.CompletedFixedScript = false

	failures := BusinessTrialFeedbackVerifier{}.VerifyFeedback(feedback)
	requireFailureCode(t, failures, "business_trial_script_incomplete")
}

func TestBusinessTrialFeedbackVerifierRejectsScoreOutOfRange(t *testing.T) {
	feedback := validBusinessTrialFeedback()
	feedback.InterviewFlowScore = 6

	failures := BusinessTrialFeedbackVerifier{}.VerifyFeedback(feedback)
	requireFailureCode(t, failures, "business_trial_score_invalid")
}

func TestBusinessTrialFeedbackVerifierRejectsBlockerExpansionConflict(t *testing.T) {
	feedback := validBusinessTrialFeedback()
	feedback.HasBlocker = true
	feedback.ExpandRecommendation = " YES "

	failures := BusinessTrialFeedbackVerifier{}.VerifyFeedback(feedback)
	requireFailureCode(t, failures, "business_trial_blocker_expansion_conflict")
}

func TestBusinessTrialFeedbackVerifierRejectsMissingRecommendation(t *testing.T) {
	feedback := validBusinessTrialFeedback()
	feedback.ExpandRecommendation = " "

	failures := BusinessTrialFeedbackVerifier{}.VerifyFeedback(feedback)
	requireFailureCode(t, failures, "business_trial_expand_recommendation_invalid")
}

func validBusinessTrialFeedback() BusinessTrialFeedback {
	return BusinessTrialFeedback{
		TrialRole:             "interviewer",
		TrialDate:             "2026-06-07",
		Scenario:              "go-backend-internal",
		CompletedFixedScript:  true,
		InterviewFlowScore:    4,
		ReportUsefulnessScore: 4,
		ProjectPolishScore:    4,
		ExpandRecommendation:  "yes",
		HasBlocker:            false,
		MostValuable:          "报告和追问可以辅助内部面试复盘。",
		TopIssue:              "题库覆盖仍需继续增加 Go 后端真实场景。",
		NextPriority:          "继续收集 Go 后端题库和报告质量反馈。",
	}
}

func requireFailureCode(t *testing.T, failures []Failure, code string) {
	t.Helper()
	for _, failure := range failures {
		if failure.Code == code {
			return
		}
	}
	t.Fatalf("missing failure code %s in %+v", code, failures)
}
