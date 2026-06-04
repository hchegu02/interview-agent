package verify

import "interview-agent/internal/domain"

type ReportCompletenessVerifier struct{}

func (ReportCompletenessVerifier) VerifyReport(sess *domain.Session) []Failure {
	if sess == nil || sess.Report == nil {
		return []Failure{{Code: "report_missing", Message: "session report is missing", Target: "report"}}
	}
	rep := sess.Report
	failures := []Failure{}
	if len(sess.Rounds) > 0 && len(rep.SkillBreakdown) == 0 {
		failures = append(failures, Failure{Code: "report_skill_breakdown_missing", Message: "report skill breakdown is missing", Target: "report.skill_breakdown"})
	}
	if rep.TranscriptAnalysis == nil || len(rep.TranscriptAnalysis.Dimensions) == 0 {
		failures = append(failures, Failure{Code: "report_transcript_analysis_missing", Message: "report transcript analysis is missing", Target: "report.transcript_analysis"})
	}
	if len(rep.DrillPlan) == 0 {
		failures = append(failures, Failure{Code: "report_drill_plan_missing", Message: "report drill plan is missing", Target: "report.drill_plan"})
	}
	if len(rep.Improvements) == 0 {
		failures = append(failures, Failure{Code: "report_improvements_missing", Message: "report improvements are missing", Target: "report.improvements"})
	}
	if len(rep.NextSteps) == 0 {
		failures = append(failures, Failure{Code: "report_next_steps_missing", Message: "report next steps are missing", Target: "report.next_steps"})
	}
	return failures
}
