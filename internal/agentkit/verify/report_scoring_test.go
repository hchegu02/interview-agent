package verify

import (
	"testing"

	"interview-agent/internal/domain"
)

func TestReportScoringVerifier_PassSession(t *testing.T) {
	sess := validReportScoringSession()

	failures := ReportScoringVerifier{}.VerifyReportScoring(sess)
	if len(failures) != 0 {
		t.Fatalf("failures = %+v", failures)
	}
}

func TestReportScoringVerifier_MissingAnsweredRoundReview(t *testing.T) {
	sess := validReportScoringSession()
	sess.Report.RoundReviews = sess.Report.RoundReviews[1:]

	failures := ReportScoringVerifier{}.VerifyReportScoring(sess)
	requireFailureCode(t, failures, "report_round_review_missing")
}

func TestReportScoringVerifier_MissingOriginalAnswer(t *testing.T) {
	sess := validReportScoringSession()
	sess.Report.RoundReviews[0].Answer = ""

	failures := ReportScoringVerifier{}.VerifyReportScoring(sess)
	requireFailureCode(t, failures, "report_round_answer_missing")
}

func TestReportScoringVerifier_MissingScore(t *testing.T) {
	sess := validReportScoringSession()
	sess.Report.RoundReviews[0].Score = nil

	failures := ReportScoringVerifier{}.VerifyReportScoring(sess)
	requireFailureCode(t, failures, "report_round_score_missing")
}

func TestReportScoringVerifier_ScoreMismatch(t *testing.T) {
	sess := validReportScoringSession()
	wrongScore := 91
	sess.Report.RoundReviews[0].Score = &wrongScore

	failures := ReportScoringVerifier{}.VerifyReportScoring(sess)
	requireFailureCode(t, failures, "report_round_score_mismatch")
}

func TestReportScoringVerifier_CountsTowardOverallMismatch(t *testing.T) {
	sess := validReportScoringSession()
	sess.Report.RoundReviews[0].CountsTowardOverall = false
	sess.Report.OverallScore = domain.OverallScoreFromRoundReviews(sess.Report.RoundReviews)

	failures := ReportScoringVerifier{}.VerifyReportScoring(sess)
	requireFailureCode(t, failures, "report_round_counts_toward_overall_mismatch")
}

func TestReportScoringVerifier_MissingMainScoringEvidence(t *testing.T) {
	sess := validReportScoringSession()
	sess.Report.RoundReviews[0].HitPoints = nil
	sess.Report.RoundReviews[0].MissedPoints = nil
	sess.Report.RoundReviews[0].Suggestion = ""

	failures := ReportScoringVerifier{}.VerifyReportScoring(sess)
	requireFailureCode(t, failures, "report_round_hit_points_missing")
	requireFailureCode(t, failures, "report_round_missed_points_missing")
	requireFailureCode(t, failures, "report_round_suggestion_missing")
}

func TestReportScoringVerifier_MainScoringEvidenceMismatch(t *testing.T) {
	sess := validReportScoringSession()
	sess.Report.RoundReviews[0].HitPoints = []string{"错的命中点"}
	sess.Report.RoundReviews[0].MissedPoints = []string{"错的缺失点"}
	sess.Report.RoundReviews[0].Suggestion = "错的建议"

	failures := ReportScoringVerifier{}.VerifyReportScoring(sess)
	requireFailureCode(t, failures, "report_round_hit_points_mismatch")
	requireFailureCode(t, failures, "report_round_missed_points_mismatch")
	requireFailureCode(t, failures, "report_round_suggestion_mismatch")
}

func TestReportScoringVerifier_MissingAnsweredFollowUpReview(t *testing.T) {
	sess := validReportScoringSession()
	sess.Report.RoundReviews[0].FollowUps = nil

	failures := ReportScoringVerifier{}.VerifyReportScoring(sess)
	requireFailureCode(t, failures, "report_followup_review_missing")
}

func TestReportScoringVerifier_MissingFollowUpAnswer(t *testing.T) {
	sess := validReportScoringSession()
	sess.Report.RoundReviews[0].FollowUps[0].Answer = ""

	failures := ReportScoringVerifier{}.VerifyReportScoring(sess)
	requireFailureCode(t, failures, "report_followup_answer_missing")
}

func TestReportScoringVerifier_MissingFollowUpScoringEvidence(t *testing.T) {
	sess := validReportScoringSession()
	sess.Report.RoundReviews[0].FollowUps[0].Score = nil
	sess.Report.RoundReviews[0].FollowUps[0].HitPoints = nil

	failures := ReportScoringVerifier{}.VerifyReportScoring(sess)
	requireFailureCode(t, failures, "report_followup_score_missing")
	requireFailureCode(t, failures, "report_followup_hit_points_missing")
}

func TestReportScoringVerifier_FollowUpScoringEvidenceMismatch(t *testing.T) {
	sess := validReportScoringSession()
	wrongScore := 81
	sess.Report.RoundReviews[0].FollowUps[0].Score = &wrongScore
	sess.Report.RoundReviews[0].FollowUps[0].HitPoints = []string{"错的追问命中点"}

	failures := ReportScoringVerifier{}.VerifyReportScoring(sess)
	requireFailureCode(t, failures, "report_followup_score_mismatch")
	requireFailureCode(t, failures, "report_followup_hit_points_mismatch")
}

func TestReportScoringVerifier_OverallMismatch(t *testing.T) {
	sess := validReportScoringSession()
	sess.Report.OverallScore = 99

	failures := ReportScoringVerifier{}.VerifyReportScoring(sess)
	requireFailureCode(t, failures, "report_overall_score_mismatch")
}

func validReportScoringSession() *domain.Session {
	score90 := 90
	score70 := 70
	followScore80 := 80
	return &domain.Session{
		ID: "s1",
		Rounds: []domain.AnswerRound{
			{
				RoundID: "r1",
				Question: domain.Question{
					ID:             "go-001",
					Content:        "讲一下 Go 的 GMP 调度模型。",
					SkillCategory:  "go",
					ExpectedPoints: []string{"G/M/P 定义", "本地队列"},
				},
				Answer: "G 是 goroutine，M 是线程，P 负责本地队列。",
				Evaluation: &domain.Evaluation{
					QuestionID: "go-001",
					Score:      90,
					Strengths:  []string{"覆盖核心概念"},
					Weaknesses: []string{"缺少 work stealing"},
					Suggestion: "补充调度细节",
				},
				FollowUps: []domain.FollowUp{{
					Question: "work stealing 在什么情况下发生？",
					Answer:   "本地队列为空时会从其他 P 偷取任务。",
					Evaluation: &domain.Evaluation{
						QuestionID: "go-001-followup",
						Score:      80,
						Strengths:  []string{"回答了触发条件"},
					},
				}},
			},
			{
				RoundID: "r2",
				Question: domain.Question{
					ID:             "redis-001",
					Content:        "Redis AOF 和 RDB 怎么取舍？",
					SkillCategory:  "redis",
					ExpectedPoints: []string{"aof", "rdb"},
				},
				Answer: "AOF 更偏实时恢复，RDB 更适合快照备份。",
				Evaluation: &domain.Evaluation{
					QuestionID: "redis-001",
					Score:      70,
					Strengths:  []string{"区分恢复和快照"},
					Weaknesses: []string{"缺少 fsync 策略"},
					Suggestion: "补充 appendfsync 策略",
				},
			},
		},
		Report: &domain.Report{
			SessionID:    "s1",
			OverallScore: 80,
			RoundReviews: []domain.RoundReview{
				{
					RoundID:             "r1",
					Number:              1,
					Type:                "main",
					QuestionID:          "go-001",
					Question:            "讲一下 Go 的 GMP 调度模型。",
					Answer:              "G 是 goroutine，M 是线程，P 负责本地队列。",
					Score:               &score90,
					HitPoints:           []string{"覆盖核心概念"},
					MissedPoints:        []string{"缺少 work stealing"},
					Suggestion:          "补充调度细节",
					ExpectedPoints:      []string{"G/M/P 定义", "本地队列"},
					CountsTowardOverall: true,
					FollowUps: []domain.FollowUpReview{{
						Question:  "work stealing 在什么情况下发生？",
						Answer:    "本地队列为空时会从其他 P 偷取任务。",
						Score:     &followScore80,
						HitPoints: []string{"回答了触发条件"},
					}},
				},
				{
					RoundID:             "r2",
					Number:              2,
					Type:                "main",
					QuestionID:          "redis-001",
					Question:            "Redis AOF 和 RDB 怎么取舍？",
					Answer:              "AOF 更偏实时恢复，RDB 更适合快照备份。",
					Score:               &score70,
					HitPoints:           []string{"区分恢复和快照"},
					MissedPoints:        []string{"缺少 fsync 策略"},
					Suggestion:          "补充 appendfsync 策略",
					ExpectedPoints:      []string{"aof", "rdb"},
					CountsTowardOverall: true,
				},
			},
		},
	}
}
