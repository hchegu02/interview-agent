package httpapi

import (
	"time"

	"interview-agent/internal/domain"
)

type interviewResponse struct {
	SessionID        string                   `json:"session_id"`
	UserID           string                   `json:"user_id,omitempty"`
	Mode             string                   `json:"mode"`
	Status           string                   `json:"status"`
	Phase            string                   `json:"phase"`
	Progress         []interviewProgressStep  `json:"progress"`
	JobProfile       *domain.JobProfile       `json:"job_profile,omitempty"`
	CandidateProfile *domain.CandidateProfile `json:"candidate_profile,omitempty"`
	ProfileAnalysis  *domain.ProfileAnalysis  `json:"profile_analysis,omitempty"`
	RetrievalTrace   *domain.RetrievalTrace   `json:"retrieval_trace,omitempty"`
	Suspension       *domain.Suspension       `json:"suspension,omitempty"`
	WorkingMemory    *interviewWorkingMemory  `json:"working_memory,omitempty"`
	Question         *interviewQuestion       `json:"question,omitempty"`
	Rounds           []interviewRound         `json:"rounds,omitempty"`
	Report           *domain.Report           `json:"report,omitempty"`
	CreatedAt        time.Time                `json:"created_at"`
	UpdatedAt        time.Time                `json:"updated_at"`
}

type interviewProgressStep struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Status string `json:"status"`
}

type interviewQuestion struct {
	ID             string   `json:"id"`
	Content        string   `json:"content"`
	Tags           []string `json:"tags,omitempty"`
	Difficulty     int      `json:"difficulty,omitempty"`
	SkillCategory  string   `json:"skill_category,omitempty"`
	ExpectedPoints []string `json:"expected_points,omitempty"`
}

type interviewRound struct {
	RoundID   string              `json:"round_id"`
	Number    int                 `json:"number"`
	Question  interviewQuestion   `json:"question"`
	Answer    string              `json:"answer,omitempty"`
	FollowUps []interviewFollowUp `json:"follow_ups,omitempty"`
	Feedback  *interviewFeedback  `json:"feedback,omitempty"`
	Completed bool                `json:"completed"`
}

type interviewFollowUp struct {
	Question string             `json:"question"`
	Answer   string             `json:"answer,omitempty"`
	Feedback *interviewFeedback `json:"feedback,omitempty"`
}

type interviewFeedback struct {
	Score          int      `json:"score"`
	HitPoints      []string `json:"hit_points,omitempty"`
	MissedPoints   []string `json:"missed_points,omitempty"`
	Suggestion     string   `json:"suggestion,omitempty"`
	ExpectedPoints []string `json:"expected_points,omitempty"`
}

type interviewWorkingMemory struct {
	ConfirmedSkills []string                  `json:"confirmed_skills,omitempty"`
	WeakSkills      []string                  `json:"weak_skills,omitempty"`
	SuspectedSkills []string                  `json:"suspected_skills,omitempty"`
	SkillCoverage   map[string]float64        `json:"skill_coverage,omitempty"`
	Difficulty      *interviewDifficultyState `json:"difficulty,omitempty"`
	AvgScore        float64                   `json:"avg_score,omitempty"`
	RoundsAsked     int                       `json:"rounds_asked,omitempty"`
	MaxRounds       int                       `json:"max_rounds,omitempty"`
	ScoredRounds    int                       `json:"scored_rounds,omitempty"`
	DegradedRounds  int                       `json:"degraded_rounds,omitempty"`
	DegradedReasons map[string]string         `json:"degraded_reasons,omitempty"`
	ProbesUsed      int                       `json:"probes_used,omitempty"`
	MaxProbes       int                       `json:"max_probes,omitempty"`
	ReflectionsUsed int                       `json:"reflections_used,omitempty"`
	MaxReflections  int                       `json:"max_reflections,omitempty"`
	ReflectTopic    string                    `json:"reflect_topic,omitempty"`
}

type interviewDifficultyState struct {
	Current       domain.Difficulty `json:"current"`
	CorrectStreak int               `json:"correct_streak,omitempty"`
	WrongStreak   int               `json:"wrong_streak,omitempty"`
	LastRoundID   string            `json:"last_round_id,omitempty"`
}

func buildInterviewResponse(sess *domain.Session) interviewResponse {
	mode := sessionMode(sess)
	return interviewResponse{
		SessionID:        sess.ID,
		UserID:           sess.UserID,
		Mode:             mode,
		Status:           string(sess.Status),
		Phase:            interviewPhase(sess),
		Progress:         interviewProgress(sess),
		JobProfile:       cloneJobProfile(sess.JobProfile),
		CandidateProfile: cloneCandidateProfile(sess.CandProfile),
		ProfileAnalysis:  cloneProfileAnalysis(sess.ProfileAnalysis),
		RetrievalTrace:   cloneRetrievalTrace(sess.RetrievalTrace),
		Suspension:       cloneSuspension(sess.Suspension),
		WorkingMemory:    buildInterviewWorkingMemory(sess.WorkingMemory),
		Question:         buildInterviewQuestion(currentQuestion(sess), false),
		Rounds:           buildInterviewRounds(sess, mode),
		Report:           cloneReport(sess.Report),
		CreatedAt:        sess.CreatedAt,
		UpdatedAt:        sess.UpdatedAt,
	}
}

func buildInterviewWorkingMemory(mem *domain.WorkingMemory) *interviewWorkingMemory {
	if mem == nil {
		return nil
	}
	out := &interviewWorkingMemory{
		ConfirmedSkills: append([]string(nil), mem.ConfirmedSkills...),
		WeakSkills:      append([]string(nil), mem.WeakSkills...),
		SuspectedSkills: append([]string(nil), mem.SuspectedSkills...),
		AvgScore:        mem.AvgScore,
		RoundsAsked:     mem.RoundsAsked,
		MaxRounds:       mem.MaxRounds,
		ScoredRounds:    mem.ScoredRounds,
		DegradedRounds:  mem.DegradedRounds,
		ProbesUsed:      mem.ProbesUsed,
		MaxProbes:       mem.MaxProbes,
		ReflectionsUsed: mem.ReflectionsUsed,
		MaxReflections:  mem.MaxReflections,
		ReflectTopic:    mem.ReflectTopic,
	}
	if mem.SkillCoverage != nil {
		out.SkillCoverage = make(map[string]float64, len(mem.SkillCoverage))
		for skill, coverage := range mem.SkillCoverage {
			out.SkillCoverage[skill] = coverage
		}
	}
	if mem.DegradedReasons != nil {
		out.DegradedReasons = make(map[string]string, len(mem.DegradedReasons))
		for component, reason := range mem.DegradedReasons {
			out.DegradedReasons[component] = reason
		}
	}
	if mem.Difficulty != nil {
		out.Difficulty = &interviewDifficultyState{
			Current:       mem.Difficulty.Current,
			CorrectStreak: mem.Difficulty.CorrectStreak,
			WrongStreak:   mem.Difficulty.WrongStreak,
			LastRoundID:   mem.Difficulty.LastRoundID,
		}
	}
	return out
}

func cloneSuspension(suspension *domain.Suspension) *domain.Suspension {
	if suspension == nil {
		return nil
	}
	payload := map[string]interface{}(nil)
	if suspension.Payload != nil {
		payload = make(map[string]interface{}, len(suspension.Payload))
		for key, value := range suspension.Payload {
			payload[key] = value
		}
	}
	return &domain.Suspension{
		Node:      suspension.Node,
		Reason:    suspension.Reason,
		Awaiting:  suspension.Awaiting,
		Payload:   payload,
		CreatedAt: suspension.CreatedAt,
	}
}

func cloneRetrievalTrace(trace *domain.RetrievalTrace) *domain.RetrievalTrace {
	if trace == nil {
		return nil
	}
	out := &domain.RetrievalTrace{
		Query:           trace.Query,
		FallbackReasons: append([]string(nil), trace.FallbackReasons...),
		Final:           cloneRetrievalResultTrace(trace.Final),
	}
	if len(trace.Stages) > 0 {
		out.Stages = make([]domain.RetrievalStageTrace, 0, len(trace.Stages))
		for _, stage := range trace.Stages {
			out.Stages = append(out.Stages, domain.RetrievalStageTrace{
				Stage:      stage.Stage,
				Count:      stage.Count,
				DurationMS: stage.DurationMS,
				Items:      cloneRetrievalResultTrace(stage.Items),
				Error:      stage.Error,
			})
		}
	}
	return out
}

func cloneRetrievalResultTrace(items []domain.RetrievalResultTrace) []domain.RetrievalResultTrace {
	if len(items) == 0 {
		return nil
	}
	out := make([]domain.RetrievalResultTrace, 0, len(items))
	for _, item := range items {
		sources := map[string]float64(nil)
		if item.Sources != nil {
			sources = make(map[string]float64, len(item.Sources))
			for key, value := range item.Sources {
				sources[key] = value
			}
		}
		out = append(out, domain.RetrievalResultTrace{
			ID:      item.ID,
			Rank:    item.Rank,
			Score:   item.Score,
			Stage:   item.Stage,
			Reason:  item.Reason,
			Sources: sources,
		})
	}
	return out
}

func currentQuestion(sess *domain.Session) *domain.Question {
	if sess.Report != nil {
		return nil
	}
	round := sess.CurrentRound()
	if round == nil {
		return nil
	}
	if sess.CurrentNode == "probe_ask" && len(round.FollowUps) > 0 {
		last := round.FollowUps[len(round.FollowUps)-1]
		return &domain.Question{
			ID:      round.Question.ID + "-followup",
			Content: last.Question,
			Tags:    round.Question.Tags,
			Source:  "probe",
		}
	}
	return &round.Question
}

func normalizeInterviewMode(mode string) string {
	if mode == "practice" {
		return "practice"
	}
	return "exam"
}

func sessionMode(sess *domain.Session) string {
	if sess == nil {
		return "exam"
	}
	return normalizeInterviewMode(sess.Mode)
}

func shouldExposeFeedback(sess *domain.Session, mode string) bool {
	return mode == "practice" || (sess != nil && sess.Status == domain.StatusCompleted)
}

func buildInterviewQuestion(q *domain.Question, includeExpected bool) *interviewQuestion {
	if q == nil {
		return nil
	}
	out := &interviewQuestion{
		ID:            q.ID,
		Content:       q.Content,
		Tags:          append([]string(nil), q.Tags...),
		Difficulty:    q.Difficulty,
		SkillCategory: q.SkillCategory,
	}
	if includeExpected {
		out.ExpectedPoints = append([]string(nil), q.ExpectedPoints...)
	}
	return out
}

func buildInterviewRounds(sess *domain.Session, mode string) []interviewRound {
	if sess == nil || len(sess.Rounds) == 0 {
		return nil
	}
	exposeFeedback := shouldExposeFeedback(sess, mode)
	out := make([]interviewRound, 0, len(sess.Rounds))
	for i := range sess.Rounds {
		round := sess.Rounds[i]
		q := buildInterviewQuestion(&round.Question, exposeFeedback)
		if q == nil {
			continue
		}
		item := interviewRound{
			RoundID:   round.RoundID,
			Number:    i + 1,
			Question:  *q,
			Answer:    round.Answer,
			Completed: !round.CompletedAt.IsZero() || round.FinalEvaluation() != nil,
		}
		if exposeFeedback {
			item.Feedback = buildInterviewFeedback(round.FinalEvaluation(), round.Question.ExpectedPoints)
		}
		if len(round.FollowUps) > 0 {
			item.FollowUps = make([]interviewFollowUp, 0, len(round.FollowUps))
			for _, follow := range round.FollowUps {
				fu := interviewFollowUp{
					Question: follow.Question,
					Answer:   follow.Answer,
				}
				if exposeFeedback {
					fu.Feedback = buildInterviewFeedback(follow.Evaluation, nil)
				}
				item.FollowUps = append(item.FollowUps, fu)
			}
		}
		out = append(out, item)
	}
	return out
}

func buildInterviewFeedback(eval *domain.Evaluation, expected []string) *interviewFeedback {
	if eval == nil {
		return nil
	}
	return &interviewFeedback{
		Score:          eval.Score,
		HitPoints:      append([]string(nil), eval.Strengths...),
		MissedPoints:   append([]string(nil), eval.Weaknesses...),
		Suggestion:     eval.Suggestion,
		ExpectedPoints: append([]string(nil), expected...),
	}
}

func interviewPhase(sess *domain.Session) string {
	if sess == nil {
		return "preparing"
	}
	switch sess.Status {
	case domain.StatusCompleted:
		return "completed"
	case domain.StatusFailed:
		return "failed"
	}
	if currentQuestion(sess) != nil {
		return "answering"
	}
	switch sess.CurrentNode {
	case "parse_jd", "parse_resume", "gap_analyze", "analyze_profile", "retrieve_rag", "":
		return "preparing"
	case "report":
		return "reporting"
	default:
		return "evaluating"
	}
}

func interviewProgress(sess *domain.Session) []interviewProgressStep {
	steps := []interviewProgressStep{
		{Key: "jd", Label: "JD 分析"},
		{Key: "resume", Label: "简历匹配"},
		{Key: "rag", Label: "题库检索"},
		{Key: "question", Label: "出题规划"},
		{Key: "interview", Label: "面试进行"},
		{Key: "report", Label: "评估报告"},
	}
	phase := interviewPhase(sess)
	current := 0
	switch phase {
	case "preparing":
		current = 1
		if sess != nil && sess.GapReport != nil {
			current = 2
		}
		if sess != nil && sess.ProfileAnalysis != nil {
			current = 2
		}
		if sess != nil && len(sess.CandidatePool) > 0 {
			current = 3
		}
	case "answering", "evaluating":
		current = 4
	case "reporting":
		current = 5
	case "completed":
		current = len(steps)
	case "failed":
		current = 4
	}
	for i := range steps {
		switch {
		case phase == "failed" && i == current:
			steps[i].Status = "error"
		case i < current:
			steps[i].Status = "done"
		case i == current:
			steps[i].Status = "current"
		default:
			steps[i].Status = "pending"
		}
	}
	return steps
}
