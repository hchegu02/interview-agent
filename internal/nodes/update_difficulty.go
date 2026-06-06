package nodes

import (
	"context"
	"fmt"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

type UpdateDifficultyOptions struct {
	HighScoreThreshold int
	LowScoreThreshold  int
	StreakThreshold    int
}

func NewUpdateDifficultyNode(opts UpdateDifficultyOptions) graph.NodeFunc {
	if opts.HighScoreThreshold == 0 {
		opts.HighScoreThreshold = 80
	}
	if opts.LowScoreThreshold == 0 {
		opts.LowScoreThreshold = 50
	}
	if opts.StreakThreshold == 0 {
		opts.StreakThreshold = 2
	}

	return func(ctx context.Context, sess *domain.Session) error {
		round := latestScoredRound(sess)
		if round == nil {
			return fmt.Errorf("update_difficulty: no scored round: %w", graph.ErrPermanent)
		}
		final := round.FinalEvaluation()
		if final == nil {
			return fmt.Errorf("update_difficulty: no final evaluation: %w", graph.ErrPermanent)
		}
		if sess.WorkingMemory == nil {
			sess.WorkingMemory = domain.NewWorkingMemory()
		}
		state := ensureDifficultyState(sess.WorkingMemory)
		if round.RoundID != "" && state.LastRoundID == round.RoundID {
			return nil
		}
		if final.Score < 0 {
			state.LastRoundID = round.RoundID
			return nil
		}

		switch {
		case final.Score >= opts.HighScoreThreshold:
			state.CorrectStreak++
			state.WrongStreak = 0
			if state.CorrectStreak >= opts.StreakThreshold {
				state.Current = increaseDifficulty(state.Current)
				state.CorrectStreak = 0
				state.WrongStreak = 0
			}
		case final.Score < opts.LowScoreThreshold:
			state.WrongStreak++
			state.CorrectStreak = 0
			if state.WrongStreak >= opts.StreakThreshold {
				state.Current = decreaseDifficulty(state.Current)
				state.CorrectStreak = 0
				state.WrongStreak = 0
			}
		default:
			state.CorrectStreak = 0
			state.WrongStreak = 0
		}
		state.LastRoundID = round.RoundID
		return nil
	}
}

func latestScoredRound(sess *domain.Session) *domain.AnswerRound {
	if sess == nil || len(sess.Rounds) == 0 {
		return nil
	}
	for i := len(sess.Rounds) - 1; i >= 0; i-- {
		if sess.Rounds[i].FinalEvaluation() != nil {
			return &sess.Rounds[i]
		}
	}
	return nil
}

func ensureDifficultyState(mem *domain.WorkingMemory) *domain.DifficultyState {
	if mem.Difficulty == nil || mem.Difficulty.Current < domain.DifficultyEasy || mem.Difficulty.Current > domain.DifficultyHard {
		mem.Difficulty = domain.NewDifficultyState()
	}
	return mem.Difficulty
}

func increaseDifficulty(current domain.Difficulty) domain.Difficulty {
	if current >= domain.DifficultyHard {
		return domain.DifficultyHard
	}
	return current + 1
}

func decreaseDifficulty(current domain.Difficulty) domain.Difficulty {
	if current <= domain.DifficultyEasy {
		return domain.DifficultyEasy
	}
	return current - 1
}
