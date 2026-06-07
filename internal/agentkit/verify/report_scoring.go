package verify

import (
	"fmt"
	"reflect"
	"strings"

	"interview-agent/internal/domain"
)

type ReportScoringVerifier struct{}

func (ReportScoringVerifier) VerifyReportScoring(sess *domain.Session) []Failure {
	if sess == nil || sess.Report == nil {
		return nil
	}

	failures := []Failure{}
	reviewsByRoundID := map[string]domain.RoundReview{}
	for _, review := range sess.Report.RoundReviews {
		reviewsByRoundID[review.RoundID] = review
	}

	for i := range sess.Rounds {
		round := sess.Rounds[i]
		if strings.TrimSpace(round.Answer) == "" {
			continue
		}
		review, ok := reviewsByRoundID[round.RoundID]
		if !ok {
			failures = append(failures, Failure{Code: "report_round_review_missing", Message: "answered round missing from report round_reviews", Target: round.RoundID})
			continue
		}
		if strings.TrimSpace(review.Answer) == "" {
			failures = append(failures, Failure{Code: "report_round_answer_missing", Message: "report round review missing original answer", Target: round.RoundID})
		}
		if eval := round.FinalEvaluation(); eval != nil {
			if eval.Score >= 0 && review.Score == nil {
				failures = append(failures, Failure{Code: "report_round_score_missing", Message: "scored round review missing score", Target: round.RoundID})
			}
			if review.Score != nil && *review.Score != eval.Score {
				failures = append(failures, Failure{Code: "report_round_score_mismatch", Message: fmt.Sprintf("report round score=%d want %d from evaluation", *review.Score, eval.Score), Target: round.RoundID})
			}
			if review.CountsTowardOverall != (eval.Score >= 0) {
				failures = append(failures, Failure{Code: "report_round_counts_toward_overall_mismatch", Message: "report round counts_toward_overall does not match evaluation score eligibility", Target: round.RoundID})
			}
			failures = append(failures, verifyScoringEvidence("report_round", round.RoundID, eval.Strengths, review.HitPoints, eval.Weaknesses, review.MissedPoints, eval.Suggestion, review.Suggestion)...)
		}
		failures = append(failures, verifyFollowUpReviews(round, review)...)
	}

	if got, want := sess.Report.OverallScore, domain.OverallScoreFromRoundReviews(sess.Report.RoundReviews); got != want {
		failures = append(failures, Failure{
			Code:    "report_overall_score_mismatch",
			Message: fmt.Sprintf("overall_score=%d want %d from round_reviews", got, want),
			Target:  "report.overall_score",
		})
	}
	return failures
}

func verifyFollowUpReviews(round domain.AnswerRound, review domain.RoundReview) []Failure {
	failures := []Failure{}
	reviewsByQuestion := map[string]domain.FollowUpReview{}
	for _, followReview := range review.FollowUps {
		reviewsByQuestion[followReview.Question] = followReview
	}
	for i := range round.FollowUps {
		follow := round.FollowUps[i]
		if strings.TrimSpace(follow.Answer) == "" {
			continue
		}
		followReview, ok := reviewsByQuestion[follow.Question]
		target := round.RoundID
		if follow.Question != "" {
			target = round.RoundID + ".follow_ups." + follow.Question
		}
		if !ok {
			failures = append(failures, Failure{Code: "report_followup_review_missing", Message: "answered follow-up missing from report round review", Target: target})
			continue
		}
		if strings.TrimSpace(followReview.Answer) == "" {
			failures = append(failures, Failure{Code: "report_followup_answer_missing", Message: "report follow-up review missing original answer", Target: target})
		}
		if follow.Evaluation != nil {
			if follow.Evaluation.Score >= 0 && followReview.Score == nil {
				failures = append(failures, Failure{Code: "report_followup_score_missing", Message: "scored follow-up review missing score", Target: target})
			}
			if followReview.Score != nil && *followReview.Score != follow.Evaluation.Score {
				failures = append(failures, Failure{Code: "report_followup_score_mismatch", Message: fmt.Sprintf("report follow-up score=%d want %d from evaluation", *followReview.Score, follow.Evaluation.Score), Target: target})
			}
			failures = append(failures, verifyScoringEvidence("report_followup", target, follow.Evaluation.Strengths, followReview.HitPoints, follow.Evaluation.Weaknesses, followReview.MissedPoints, follow.Evaluation.Suggestion, followReview.Suggestion)...)
		}
	}
	return failures
}

func verifyScoringEvidence(prefix, target string, sourceHits, reviewHits, sourceMisses, reviewMisses []string, sourceSuggestion, reviewSuggestion string) []Failure {
	failures := []Failure{}
	if len(sourceHits) > 0 && len(reviewHits) == 0 {
		failures = append(failures, Failure{Code: prefix + "_hit_points_missing", Message: "report review missing hit point evidence", Target: target})
	}
	if len(reviewHits) > 0 && !reflect.DeepEqual(sourceHits, reviewHits) {
		failures = append(failures, Failure{Code: prefix + "_hit_points_mismatch", Message: "report review hit point evidence does not match evaluation", Target: target})
	}
	if len(sourceMisses) > 0 && len(reviewMisses) == 0 {
		failures = append(failures, Failure{Code: prefix + "_missed_points_missing", Message: "report review missing missed point evidence", Target: target})
	}
	if len(reviewMisses) > 0 && !reflect.DeepEqual(sourceMisses, reviewMisses) {
		failures = append(failures, Failure{Code: prefix + "_missed_points_mismatch", Message: "report review missed point evidence does not match evaluation", Target: target})
	}
	if strings.TrimSpace(sourceSuggestion) != "" && strings.TrimSpace(reviewSuggestion) == "" {
		failures = append(failures, Failure{Code: prefix + "_suggestion_missing", Message: "report review missing suggestion evidence", Target: target})
	}
	if strings.TrimSpace(reviewSuggestion) != "" && reviewSuggestion != sourceSuggestion {
		failures = append(failures, Failure{Code: prefix + "_suggestion_mismatch", Message: "report review suggestion does not match evaluation", Target: target})
	}
	return failures
}
