package verify

import (
	"fmt"
	"strings"

	"interview-agent/internal/domain"
)

type ReportCompletenessVerifier struct{}

func (ReportCompletenessVerifier) VerifyReport(sess *domain.Session) []Failure {
	if sess == nil || sess.Report == nil {
		return []Failure{{Code: "report_missing", Message: "session report is missing", Target: "report"}}
	}
	rep := sess.Report
	failures := []Failure{}
	if strings.TrimSpace(rep.SessionID) == "" {
		failures = append(failures, Failure{Code: "report_session_id_missing", Message: "report session_id is missing", Target: "report.session_id"})
	} else if sess.ID != "" && rep.SessionID != sess.ID {
		failures = append(failures, Failure{Code: "report_session_id_mismatch", Message: "report session_id must match session id", Target: "report.session_id"})
	}
	if !scoreInRange(rep.OverallScore) {
		failures = append(failures, Failure{Code: "report_score_invalid", Message: fmt.Sprintf("overall score %d is outside 0-100", rep.OverallScore), Target: "report.overall_score"})
	}
	if len(sess.Rounds) > 0 && len(rep.SkillBreakdown) == 0 {
		failures = append(failures, Failure{Code: "report_skill_breakdown_missing", Message: "report skill breakdown is missing", Target: "report.skill_breakdown"})
	}
	for skill, score := range rep.SkillBreakdown {
		if strings.TrimSpace(skill) == "" {
			failures = append(failures, Failure{Code: "report_skill_name_missing", Message: "skill breakdown contains empty skill name", Target: "report.skill_breakdown"})
		}
		if !scoreInRange(score) {
			failures = append(failures, Failure{Code: "report_skill_score_invalid", Message: fmt.Sprintf("skill %q score %d is outside 0-100", skill, score), Target: "report.skill_breakdown"})
		}
	}
	if rep.TranscriptAnalysis == nil || len(rep.TranscriptAnalysis.Dimensions) == 0 {
		failures = append(failures, Failure{Code: "report_transcript_analysis_missing", Message: "report transcript analysis is missing", Target: "report.transcript_analysis"})
	} else {
		for _, dim := range rep.TranscriptAnalysis.Dimensions {
			if strings.TrimSpace(dim.Name) == "" {
				failures = append(failures, Failure{Code: "report_transcript_dimension_name_missing", Message: "transcript dimension name is missing", Target: "report.transcript_analysis.dimensions"})
			}
			if !scoreInRange(dim.Score) {
				failures = append(failures, Failure{Code: "report_transcript_dimension_score_invalid", Message: fmt.Sprintf("dimension %q score %d is outside 0-100", dim.Name, dim.Score), Target: "report.transcript_analysis.dimensions"})
			}
		}
	}
	if len(rep.DrillPlan) == 0 {
		failures = append(failures, Failure{Code: "report_drill_plan_missing", Message: "report drill plan is missing", Target: "report.drill_plan"})
	} else {
		for _, item := range rep.DrillPlan {
			if strings.TrimSpace(item.Skill) == "" {
				failures = append(failures, Failure{Code: "report_drill_skill_missing", Message: "drill plan skill is missing", Target: "report.drill_plan"})
			}
			if !scoreInRange(item.TargetScore) {
				failures = append(failures, Failure{Code: "report_drill_target_score_invalid", Message: fmt.Sprintf("drill target score %d is outside 0-100", item.TargetScore), Target: "report.drill_plan"})
			}
		}
	}
	if len(rep.Improvements) == 0 {
		failures = append(failures, Failure{Code: "report_improvements_missing", Message: "report improvements are missing", Target: "report.improvements"})
	}
	if len(rep.NextSteps) == 0 {
		failures = append(failures, Failure{Code: "report_next_steps_missing", Message: "report next steps are missing", Target: "report.next_steps"})
	}
	return failures
}

func scoreInRange(score int) bool {
	return score >= 0 && score <= 100
}
