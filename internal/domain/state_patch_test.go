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
