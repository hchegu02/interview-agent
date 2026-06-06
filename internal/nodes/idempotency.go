package nodes

import (
	"fmt"

	"interview-agent/internal/domain"
)

func nodeIdempotencyKey(node, roundID string) string {
	if roundID == "" {
		roundID = "session"
	}
	return fmt.Sprintf("%s:%s", node, roundID)
}

func currentRoundID(sess *domain.Session) string {
	if sess == nil {
		return ""
	}
	if round := sess.CurrentRound(); round != nil {
		return round.RoundID
	}
	if round := latestRound(sess); round != nil {
		return round.RoundID
	}
	return ""
}

func latestRound(sess *domain.Session) *domain.AnswerRound {
	if sess == nil || len(sess.Rounds) == 0 {
		return nil
	}
	return &sess.Rounds[len(sess.Rounds)-1]
}

func isNodeApplied(mem *domain.WorkingMemory, key string) bool {
	return mem != nil && mem.AppliedNodes != nil && mem.AppliedNodes[key]
}

func markNodeApplied(mem *domain.WorkingMemory, key string) {
	if mem == nil || key == "" {
		return
	}
	if mem.AppliedNodes == nil {
		mem.AppliedNodes = map[string]bool{}
	}
	mem.AppliedNodes[key] = true
}
