package nodes

import (
	"context"
	"testing"
	"time"

	"interview-agent/internal/domain"
)

func TestUpdateDifficultyPatchNode_ReturnsWorkingMemoryPatchWithoutMutatingSession(t *testing.T) {
	sess := buildDifficultySession(85)
	originalMemory := sess.WorkingMemory
	sess.WorkingMemory.Difficulty = &domain.DifficultyState{
		Current:       domain.DifficultyMedium,
		CorrectStreak: 1,
	}

	node := NewUpdateDifficultyPatchNode(UpdateDifficultyOptions{})
	patch, err := node(context.Background(), sess)
	if err != nil {
		t.Fatalf("update difficulty patch: %v", err)
	}
	if patch.WorkingMemory == nil {
		t.Fatal("patch.WorkingMemory is nil")
	}
	if patch.WorkingMemory == originalMemory {
		t.Fatal("patch.WorkingMemory aliases session WorkingMemory")
	}
	if got := patch.WorkingMemory.Difficulty.Current; got != domain.DifficultyHard {
		t.Fatalf("patch difficulty = %v, want hard", got)
	}
	if got := sess.WorkingMemory.Difficulty.Current; got != domain.DifficultyMedium {
		t.Fatalf("session difficulty mutated before patch apply: %v", got)
	}
	if got := sess.WorkingMemory.Difficulty.CorrectStreak; got != 1 {
		t.Fatalf("session correct streak mutated before patch apply: %v", got)
	}
}

func TestUpdateDifficulty_InitializesMedium(t *testing.T) {
	sess := buildDifficultySession(70)
	sess.WorkingMemory.Difficulty = nil

	node := NewUpdateDifficultyNode(UpdateDifficultyOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("update difficulty: %v", err)
	}
	if got := sess.WorkingMemory.Difficulty.Current; got != domain.DifficultyMedium {
		t.Fatalf("difficulty = %v, want medium", got)
	}
}

func TestUpdateDifficulty_WrapperAppliesPatchToSession(t *testing.T) {
	sess := buildDifficultySession(85)
	sess.WorkingMemory.Difficulty = &domain.DifficultyState{
		Current:       domain.DifficultyMedium,
		CorrectStreak: 1,
	}

	node := NewUpdateDifficultyNode(UpdateDifficultyOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("update difficulty wrapper: %v", err)
	}
	if got := sess.WorkingMemory.Difficulty.Current; got != domain.DifficultyHard {
		t.Fatalf("difficulty = %v, want hard", got)
	}
	if got := sess.WorkingMemory.Difficulty.LastRoundID; got != "r1" {
		t.Fatalf("last round id = %q, want r1", got)
	}
}

func TestUpdateDifficulty_EscalatesAfterTwoHighScores(t *testing.T) {
	sess := buildDifficultySession(85)
	sess.WorkingMemory.Difficulty = &domain.DifficultyState{
		Current:       domain.DifficultyMedium,
		CorrectStreak: 1,
	}

	node := NewUpdateDifficultyNode(UpdateDifficultyOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("update difficulty: %v", err)
	}
	if got := sess.WorkingMemory.Difficulty.Current; got != domain.DifficultyHard {
		t.Fatalf("difficulty = %v, want hard", got)
	}
	if sess.WorkingMemory.Difficulty.CorrectStreak != 0 || sess.WorkingMemory.Difficulty.WrongStreak != 0 {
		t.Fatalf("streaks should reset after escalation: %+v", sess.WorkingMemory.Difficulty)
	}
}

func TestUpdateDifficulty_DowngradesAfterTwoLowScores(t *testing.T) {
	sess := buildDifficultySession(40)
	sess.WorkingMemory.Difficulty = &domain.DifficultyState{
		Current:     domain.DifficultyMedium,
		WrongStreak: 1,
	}

	node := NewUpdateDifficultyNode(UpdateDifficultyOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("update difficulty: %v", err)
	}
	if got := sess.WorkingMemory.Difficulty.Current; got != domain.DifficultyEasy {
		t.Fatalf("difficulty = %v, want easy", got)
	}
}

func TestUpdateDifficulty_MidScoreKeepsDifficultyAndResetsStreaks(t *testing.T) {
	sess := buildDifficultySession(65)
	sess.WorkingMemory.Difficulty = &domain.DifficultyState{
		Current:       domain.DifficultyHard,
		CorrectStreak: 1,
		WrongStreak:   1,
	}

	node := NewUpdateDifficultyNode(UpdateDifficultyOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("update difficulty: %v", err)
	}
	got := sess.WorkingMemory.Difficulty
	if got.Current != domain.DifficultyHard || got.CorrectStreak != 0 || got.WrongStreak != 0 {
		t.Fatalf("difficulty state = %+v, want hard with reset streaks", got)
	}
}

func TestUpdateDifficulty_DegradedScoreSkipped(t *testing.T) {
	sess := buildDifficultySession(-1)
	sess.WorkingMemory.Difficulty = &domain.DifficultyState{
		Current:       domain.DifficultyHard,
		CorrectStreak: 1,
		WrongStreak:   1,
	}

	node := NewUpdateDifficultyNode(UpdateDifficultyOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("update difficulty: %v", err)
	}
	got := sess.WorkingMemory.Difficulty
	if got.Current != domain.DifficultyHard || got.CorrectStreak != 1 || got.WrongStreak != 1 {
		t.Fatalf("difficulty state changed on degraded score: %+v", got)
	}
}

func TestUpdateDifficulty_ReadsCompletedRoundAfterUpdateMemory(t *testing.T) {
	sess := buildDifficultySession(85)
	sess.Rounds[0].CompletedAt = testDifficultyTime()

	node := NewUpdateDifficultyNode(UpdateDifficultyOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("update difficulty: %v", err)
	}
	got := sess.WorkingMemory.Difficulty
	if got.CorrectStreak != 1 || got.LastRoundID != "r1" {
		t.Fatalf("difficulty state = %+v, want completed round consumed once", got)
	}
}

func TestUpdateDifficulty_ReplaySameRoundIsIdempotent(t *testing.T) {
	sess := buildDifficultySession(85)
	sess.Rounds[0].CompletedAt = testDifficultyTime()
	sess.WorkingMemory.Difficulty = &domain.DifficultyState{
		Current:       domain.DifficultyMedium,
		CorrectStreak: 1,
	}

	node := NewUpdateDifficultyNode(UpdateDifficultyOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("update difficulty first run: %v", err)
	}
	first := *sess.WorkingMemory.Difficulty
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("update difficulty replay: %v", err)
	}
	second := *sess.WorkingMemory.Difficulty
	if first != second {
		t.Fatalf("replay changed difficulty state: first=%+v second=%+v", first, second)
	}
	if !sess.WorkingMemory.AppliedNodes[nodeIdempotencyKey(NodeUpdateDifficulty, "r1")] {
		t.Fatalf("applied node marker missing: %+v", sess.WorkingMemory.AppliedNodes)
	}
}

func TestUpdateDifficulty_PatchCanBeAppliedByRunner(t *testing.T) {
	sess := buildDifficultySession(40)
	sess.WorkingMemory.Difficulty = &domain.DifficultyState{
		Current:     domain.DifficultyMedium,
		WrongStreak: 1,
	}

	node := NewUpdateDifficultyPatchNode(UpdateDifficultyOptions{})
	patch, err := node(context.Background(), sess)
	if err != nil {
		t.Fatalf("update difficulty patch: %v", err)
	}
	if err := domain.ApplyStatePatch(sess, patch); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if got := sess.WorkingMemory.Difficulty.Current; got != domain.DifficultyEasy {
		t.Fatalf("difficulty = %v, want easy", got)
	}
}

func TestUpdateDifficulty_ConsumesLatestScoredRoundOnly(t *testing.T) {
	sess := buildDifficultySession(85)
	sess.Rounds[0].CompletedAt = testDifficultyTime()
	sess.Rounds = append(sess.Rounds, domain.AnswerRound{
		RoundID:     "r2",
		CompletedAt: testDifficultyTime().Add(1),
		Evaluation: &domain.Evaluation{
			QuestionID: "q2",
			Score:      40,
		},
	})
	sess.WorkingMemory.Difficulty = &domain.DifficultyState{
		Current:     domain.DifficultyMedium,
		WrongStreak: 1,
		LastRoundID: "r1",
	}

	node := NewUpdateDifficultyNode(UpdateDifficultyOptions{})
	if err := node(context.Background(), sess); err != nil {
		t.Fatalf("update difficulty: %v", err)
	}
	got := sess.WorkingMemory.Difficulty
	if got.Current != domain.DifficultyEasy || got.LastRoundID != "r2" {
		t.Fatalf("difficulty state = %+v, want only latest r2 consumed", got)
	}
}

func buildDifficultySession(score int) *domain.Session {
	return &domain.Session{
		WorkingMemory: domain.NewWorkingMemory(),
		Rounds: []domain.AnswerRound{{
			RoundID: "r1",
			Question: domain.Question{
				ID: "q1",
			},
			Evaluation: &domain.Evaluation{
				QuestionID: "q1",
				Score:      score,
			},
		}},
	}
}

func testDifficultyTime() time.Time {
	return time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
}
