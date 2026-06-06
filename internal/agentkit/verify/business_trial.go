package verify

import "strings"

type BusinessTrialFeedback struct {
	TrialRole             string `json:"trial_role"`
	TrialDate             string `json:"trial_date,omitempty"`
	Scenario              string `json:"scenario"`
	CompletedFixedScript  bool   `json:"completed_fixed_script"`
	InterviewFlowScore    int    `json:"interview_flow_score"`
	ReportUsefulnessScore int    `json:"report_usefulness_score"`
	ProjectPolishScore    int    `json:"project_polish_score"`
	ExpandRecommendation  string `json:"expand_recommendation"`
	HasBlocker            *bool  `json:"has_blocker"`
	MostValuable          string `json:"most_valuable,omitempty"`
	TopIssue              string `json:"top_issue,omitempty"`
	NextPriority          string `json:"next_priority,omitempty"`
}

type BusinessTrialFeedbackVerifier struct{}

func (BusinessTrialFeedbackVerifier) VerifyFeedback(feedback BusinessTrialFeedback) []Failure {
	return BusinessTrialFeedbackVerifier{}.Verify(feedback)
}

func (BusinessTrialFeedbackVerifier) Verify(feedback BusinessTrialFeedback) []Failure {
	failures := []Failure{}

	if strings.TrimSpace(feedback.TrialRole) == "" {
		failures = append(failures, Failure{Code: "business_trial_role_missing", Message: "business trial feedback trial_role is missing", Target: "trial_role"})
	}
	if strings.TrimSpace(feedback.Scenario) == "" {
		failures = append(failures, Failure{Code: "business_trial_scenario_missing", Message: "business trial feedback scenario is missing", Target: "scenario"})
	}
	if !feedback.CompletedFixedScript {
		failures = append(failures, Failure{Code: "business_trial_script_incomplete", Message: "business trial fixed script must be completed", Target: "completed_fixed_script"})
	}
	if !businessTrialScoreInRange(feedback.InterviewFlowScore) {
		failures = append(failures, Failure{Code: "business_trial_score_invalid", Message: "interview_flow_score must be between 1 and 5", Target: "interview_flow_score"})
	}
	if !businessTrialScoreInRange(feedback.ReportUsefulnessScore) {
		failures = append(failures, Failure{Code: "business_trial_score_invalid", Message: "report_usefulness_score must be between 1 and 5", Target: "report_usefulness_score"})
	}
	if !businessTrialScoreInRange(feedback.ProjectPolishScore) {
		failures = append(failures, Failure{Code: "business_trial_score_invalid", Message: "project_polish_score must be between 1 and 5", Target: "project_polish_score"})
	}

	recommendation := strings.ToLower(strings.TrimSpace(feedback.ExpandRecommendation))
	if recommendation != "yes" && recommendation != "no" && recommendation != "unsure" {
		failures = append(failures, Failure{Code: "business_trial_expand_recommendation_invalid", Message: "expand_recommendation must be yes, no, or unsure", Target: "expand_recommendation"})
	}
	if feedback.HasBlocker == nil {
		failures = append(failures, Failure{Code: "business_trial_has_blocker_missing", Message: "business trial feedback has_blocker is missing", Target: "has_blocker"})
	} else if *feedback.HasBlocker && recommendation == "yes" {
		failures = append(failures, Failure{Code: "business_trial_blocker_expansion_conflict", Message: "feedback with blocker cannot recommend expansion", Target: "expand_recommendation"})
	}

	return failures
}

func businessTrialScoreInRange(score int) bool {
	return score >= 1 && score <= 5
}
