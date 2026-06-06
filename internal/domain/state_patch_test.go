package domain

import (
	"testing"
	"time"
)

func TestApplyStatePatch_ReplacesCandidatePoolAndTrace(t *testing.T) {
	trace := &RetrievalTrace{Query: "redis"}
	pool := []Question{{ID: "q1", Content: "Redis AOF?", Tags: []string{"redis"}}}
	sess := &Session{
		CandidatePool:  []Question{{ID: "old"}},
		RetrievalTrace: &RetrievalTrace{Query: "old"},
	}

	if err := ApplyStatePatch(sess, StatePatch{
		CandidatePool:  &pool,
		RetrievalTrace: trace,
	}); err != nil {
		t.Fatalf("apply patch: %v", err)
	}

	if len(sess.CandidatePool) != 1 || sess.CandidatePool[0].ID != "q1" {
		t.Fatalf("candidate pool = %+v", sess.CandidatePool)
	}
	if sess.RetrievalTrace == nil || sess.RetrievalTrace.Query != "redis" {
		t.Fatalf("retrieval trace = %+v", sess.RetrievalTrace)
	}

	pool[0].ID = "mutated"
	if sess.CandidatePool[0].ID != "q1" {
		t.Fatalf("candidate pool should be copied: %+v", sess.CandidatePool)
	}
}

func TestApplyStatePatch_AppendsRoundAndSetsDecision(t *testing.T) {
	now := time.Now()
	decision := &Decision{Action: ActionAskNew, Reasoning: "cover redis", DecidedAt: now}
	round := &AnswerRound{
		RoundID:   "r1",
		Question:  Question{ID: "q1", Content: "Redis AOF?"},
		DecidedAt: now,
	}
	sess := &Session{
		Rounds: []AnswerRound{{RoundID: "old", CompletedAt: now}},
	}

	if err := ApplyStatePatch(sess, StatePatch{
		PendingDecision: decision,
		AppendRound:     round,
	}); err != nil {
		t.Fatalf("apply patch: %v", err)
	}

	if sess.PendingDecision == nil || sess.PendingDecision.Action != ActionAskNew {
		t.Fatalf("pending decision = %+v", sess.PendingDecision)
	}
	if len(sess.Rounds) != 2 || sess.Rounds[1].RoundID != "r1" {
		t.Fatalf("rounds = %+v", sess.Rounds)
	}
}

func TestApplyStatePatch_AppendsCurrentRoundFollowUp(t *testing.T) {
	followUp := &FollowUp{
		Question: "work stealing 触发时机是什么?",
		Reason:   "深挖调度细节",
		AskedAt:  time.Now(),
	}
	sess := &Session{
		Rounds: []AnswerRound{{
			RoundID:  "r1",
			Question: Question{ID: "q1"},
			FollowUps: []FollowUp{{
				Question: "已有追问",
				Answer:   "已有回答",
			}},
		}},
	}

	if err := ApplyStatePatch(sess, StatePatch{AppendCurrentFollowUp: followUp}); err != nil {
		t.Fatalf("apply patch: %v", err)
	}

	if len(sess.Rounds[0].FollowUps) != 2 {
		t.Fatalf("followups = %+v", sess.Rounds[0].FollowUps)
	}
	if sess.Rounds[0].FollowUps[1].Question != followUp.Question {
		t.Fatalf("appended followup = %+v", sess.Rounds[0].FollowUps[1])
	}
}

func TestApplyStatePatch_WritesCurrentFollowUpEvaluation(t *testing.T) {
	eval := &Evaluation{QuestionID: "q1-followup", Score: 76}
	sess := &Session{
		Rounds: []AnswerRound{{
			RoundID:  "r1",
			Question: Question{ID: "q1"},
			FollowUps: []FollowUp{
				{Question: "first", Evaluation: &Evaluation{Score: 50}},
				{Question: "last"},
			},
		}},
	}

	if err := ApplyStatePatch(sess, StatePatch{CurrentFollowUpEvaluation: eval}); err != nil {
		t.Fatalf("apply patch: %v", err)
	}

	if sess.Rounds[0].FollowUps[0].Evaluation.Score != 50 {
		t.Fatalf("first follow-up evaluation should not change: %+v", sess.Rounds[0].FollowUps)
	}
	if sess.Rounds[0].FollowUps[1].Evaluation == nil || sess.Rounds[0].FollowUps[1].Evaluation.Score != 76 {
		t.Fatalf("last follow-up evaluation = %+v", sess.Rounds[0].FollowUps[1].Evaluation)
	}
}

func TestApplyStatePatch_CurrentFollowUpMissingForEvaluationReturnsError(t *testing.T) {
	err := ApplyStatePatch(&Session{Rounds: []AnswerRound{{RoundID: "r1", Question: Question{ID: "q1"}}}}, StatePatch{
		CurrentFollowUpEvaluation: &Evaluation{Score: 70},
	})
	if err == nil {
		t.Fatal("expected error when current follow-up is missing")
	}
}

func TestApplyStatePatch_WritesCurrentCriticResult(t *testing.T) {
	critic := &Critic{
		GroundedScore:  72,
		NeedRefine:     true,
		Issues:         []string{"覆盖不全"},
		Summary:        "需要重评",
		HasProbeSignal: true,
		ProbeTopic:     "work stealing",
	}
	existingEval := &Evaluation{QuestionID: "q1", Score: 80}
	sess := &Session{
		Rounds: []AnswerRound{{
			RoundID:    "r1",
			Question:   Question{ID: "q1"},
			Evaluation: existingEval,
			FollowUps:  []FollowUp{{Question: "keep"}},
		}},
	}

	if err := ApplyStatePatch(sess, StatePatch{CurrentCriticResult: critic}); err != nil {
		t.Fatalf("apply patch: %v", err)
	}

	got := sess.Rounds[0].CriticResult
	if got == nil || got.GroundedScore != 72 || !got.NeedRefine || !got.HasProbeSignal || got.ProbeTopic != "work stealing" {
		t.Fatalf("critic result = %+v", got)
	}
	if sess.Rounds[0].Evaluation != existingEval || len(sess.Rounds[0].FollowUps) != 1 {
		t.Fatalf("critic patch should not overwrite other round fields: %+v", sess.Rounds[0])
	}
}

func TestApplyStatePatch_CurrentRoundMissingForCriticResultReturnsError(t *testing.T) {
	err := ApplyStatePatch(&Session{}, StatePatch{
		CurrentCriticResult: &Critic{GroundedScore: 70},
	})
	if err == nil {
		t.Fatal("expected error when current round is missing")
	}
}

func TestApplyStatePatch_UpdatesCurrentCriticProbeSignalOnly(t *testing.T) {
	sess := &Session{
		Rounds: []AnswerRound{{
			RoundID:  "r1",
			Question: Question{ID: "q1"},
			CriticResult: &Critic{
				GroundedScore:  80,
				NeedRefine:     true,
				Issues:         []string{"issue"},
				Summary:        "summary",
				HasProbeSignal: true,
				ProbeTopic:     "old",
			},
		}},
	}

	if err := ApplyStatePatch(sess, StatePatch{
		CurrentCriticProbeSignal: &CriticProbeSignalPatch{
			HasProbeSignal: false,
			ProbeTopic:     "",
		},
	}); err != nil {
		t.Fatalf("apply patch: %v", err)
	}

	got := sess.Rounds[0].CriticResult
	if got.HasProbeSignal || got.ProbeTopic != "" {
		t.Fatalf("probe signal not updated: %+v", got)
	}
	if got.GroundedScore != 80 || !got.NeedRefine || len(got.Issues) != 1 || got.Summary != "summary" {
		t.Fatalf("critic audit fields should be preserved: %+v", got)
	}
}

func TestApplyStatePatch_CurrentRoundMissingForFollowUpReturnsError(t *testing.T) {
	err := ApplyStatePatch(&Session{}, StatePatch{
		AppendCurrentFollowUp: &FollowUp{Question: "q"},
	})
	if err == nil {
		t.Fatal("expected error when current round is missing")
	}
}

func TestApplyStatePatch_CurrentCriticMissingForProbeSignalReturnsError(t *testing.T) {
	err := ApplyStatePatch(&Session{Rounds: []AnswerRound{{RoundID: "r1", Question: Question{ID: "q1"}}}}, StatePatch{
		CurrentCriticProbeSignal: &CriticProbeSignalPatch{},
	})
	if err == nil {
		t.Fatal("expected error when current critic is missing")
	}
}

func TestApplyStatePatch_WritesCurrentRoundEvaluationAndCompletion(t *testing.T) {
	completedAt := time.Now()
	eval := &Evaluation{QuestionID: "q1", Score: 82}
	sess := &Session{
		PendingDecision: &Decision{Action: ActionAskNew},
		Rounds: []AnswerRound{{
			RoundID:  "r1",
			Question: Question{ID: "q1"},
		}},
	}

	if err := ApplyStatePatch(sess, StatePatch{
		ClearPendingDecision: true,
		CurrentEvaluation:    eval,
		CompleteCurrentRound: &completedAt,
	}); err != nil {
		t.Fatalf("apply patch: %v", err)
	}

	if sess.PendingDecision != nil {
		t.Fatalf("pending decision should be cleared: %+v", sess.PendingDecision)
	}
	if sess.Rounds[0].Evaluation == nil || sess.Rounds[0].Evaluation.Score != 82 {
		t.Fatalf("evaluation = %+v", sess.Rounds[0].Evaluation)
	}
	if !sess.Rounds[0].CompletedAt.Equal(completedAt) {
		t.Fatalf("completed_at = %v, want %v", sess.Rounds[0].CompletedAt, completedAt)
	}
}

func TestApplyStatePatch_WritesCurrentRefinedEvaluation(t *testing.T) {
	refined := &Evaluation{QuestionID: "q1", Score: 55}
	original := &Evaluation{QuestionID: "q1", Score: 80}
	sess := &Session{
		Rounds: []AnswerRound{{
			RoundID:    "r1",
			Question:   Question{ID: "q1"},
			Evaluation: original,
			CriticResult: &Critic{
				NeedRefine: true,
			},
		}},
	}

	if err := ApplyStatePatch(sess, StatePatch{CurrentRefinedEvaluation: refined}); err != nil {
		t.Fatalf("apply patch: %v", err)
	}

	if sess.Rounds[0].RefinedEval == nil || sess.Rounds[0].RefinedEval.Score != 55 {
		t.Fatalf("refined eval = %+v", sess.Rounds[0].RefinedEval)
	}
	if sess.Rounds[0].Evaluation != original || sess.Rounds[0].CriticResult == nil {
		t.Fatalf("refine patch should not overwrite existing round fields: %+v", sess.Rounds[0])
	}
	if sess.Rounds[0].FinalEvaluation().Score != 55 {
		t.Fatalf("final evaluation should prefer refined eval, got %+v", sess.Rounds[0].FinalEvaluation())
	}
}

func TestApplyStatePatch_CurrentRoundMissingForRefinedEvaluationReturnsError(t *testing.T) {
	err := ApplyStatePatch(&Session{}, StatePatch{
		CurrentRefinedEvaluation: &Evaluation{Score: 55},
	})
	if err == nil {
		t.Fatal("expected error when current round is missing")
	}
}

func TestApplyStatePatch_CurrentRoundMissingReturnsError(t *testing.T) {
	err := ApplyStatePatch(&Session{}, StatePatch{
		CurrentEvaluation: &Evaluation{QuestionID: "q1", Score: 70},
	})
	if err == nil {
		t.Fatal("expected error when current round is missing")
	}
}

func TestApplyStatePatch_ReplacesReport(t *testing.T) {
	report := &Report{SessionID: "s1", OverallScore: 75}
	sess := &Session{Report: &Report{SessionID: "old"}}

	if err := ApplyStatePatch(sess, StatePatch{Report: report}); err != nil {
		t.Fatalf("apply patch: %v", err)
	}

	if sess.Report == nil || sess.Report.SessionID != "s1" || sess.Report.OverallScore != 75 {
		t.Fatalf("report = %+v", sess.Report)
	}
}

func TestApplyStatePatch_ReplacesStatus(t *testing.T) {
	status := StatusCompleted
	sess := &Session{Status: StatusRunning}

	if err := ApplyStatePatch(sess, StatePatch{Status: &status}); err != nil {
		t.Fatalf("apply patch: %v", err)
	}

	if sess.Status != StatusCompleted {
		t.Fatalf("status = %q, want completed", sess.Status)
	}
}

func TestApplyStatePatch_ReplacesWorkingMemory(t *testing.T) {
	mem := NewWorkingMemory()
	mem.RoundsAsked = 3
	sess := &Session{}

	if err := ApplyStatePatch(sess, StatePatch{WorkingMemory: mem}); err != nil {
		t.Fatalf("apply patch: %v", err)
	}

	if sess.WorkingMemory == nil || sess.WorkingMemory.RoundsAsked != 3 {
		t.Fatalf("working memory = %+v", sess.WorkingMemory)
	}
}
