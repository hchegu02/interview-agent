package domain

import (
	"fmt"
	"time"
)

// StatePatch 表达节点对 Session 的结构化写入意图。
// 第一阶段只覆盖高风险字段，避免节点随意改大对象。
type StatePatch struct {
	CandidatePool             *[]Question
	RetrievalTrace            *RetrievalTrace
	PendingDecision           *Decision
	ClearPendingDecision      bool
	AppendRound               *AnswerRound
	AppendCurrentFollowUp     *FollowUp
	CurrentFollowUpEvaluation *Evaluation
	CurrentCriticResult       *Critic
	CurrentCriticProbeSignal  *CriticProbeSignalPatch
	CurrentEvaluation         *Evaluation
	CurrentRefinedEvaluation  *Evaluation
	CompleteCurrentRound      *time.Time
	Report                    *Report
	Status                    *SessionStatus
	WorkingMemory             *WorkingMemory
}

type CriticProbeSignalPatch struct {
	HasProbeSignal bool
	ProbeTopic     string
}

// ApplyStatePatch 把结构化 patch 应用到 Session。
// 这里集中定义 replace / append / current round 写入规则，避免分散在节点里。
func ApplyStatePatch(sess *Session, patch StatePatch) error {
	if sess == nil {
		return fmt.Errorf("apply state patch: nil session")
	}
	if patch.CandidatePool != nil {
		sess.CandidatePool = append([]Question(nil), (*patch.CandidatePool)...)
	}
	if patch.RetrievalTrace != nil {
		sess.RetrievalTrace = patch.RetrievalTrace
	}
	if patch.ClearPendingDecision {
		sess.PendingDecision = nil
	}
	if patch.PendingDecision != nil {
		sess.PendingDecision = patch.PendingDecision
	}
	if patch.AppendRound != nil {
		sess.Rounds = append(sess.Rounds, *patch.AppendRound)
	}
	if patch.AppendCurrentFollowUp != nil {
		round := sess.CurrentRound()
		if round == nil {
			return fmt.Errorf("apply state patch: current round missing for follow-up")
		}
		round.FollowUps = append(round.FollowUps, *patch.AppendCurrentFollowUp)
	}
	if patch.CurrentFollowUpEvaluation != nil {
		round := sess.CurrentRound()
		if round == nil {
			return fmt.Errorf("apply state patch: current round missing for follow-up evaluation")
		}
		if len(round.FollowUps) == 0 {
			return fmt.Errorf("apply state patch: current round follow-up missing for evaluation")
		}
		round.FollowUps[len(round.FollowUps)-1].Evaluation = patch.CurrentFollowUpEvaluation
	}
	if patch.CurrentCriticResult != nil {
		round := sess.CurrentRound()
		if round == nil {
			return fmt.Errorf("apply state patch: current round missing for critic result")
		}
		round.CriticResult = patch.CurrentCriticResult
	}
	if patch.CurrentCriticProbeSignal != nil {
		round := sess.CurrentRound()
		if round == nil {
			return fmt.Errorf("apply state patch: current round missing for critic probe signal")
		}
		if round.CriticResult == nil {
			return fmt.Errorf("apply state patch: current round critic missing for probe signal")
		}
		round.CriticResult.HasProbeSignal = patch.CurrentCriticProbeSignal.HasProbeSignal
		round.CriticResult.ProbeTopic = patch.CurrentCriticProbeSignal.ProbeTopic
	}
	if patch.CurrentEvaluation != nil {
		round := sess.CurrentRound()
		if round == nil {
			return fmt.Errorf("apply state patch: current round missing for evaluation")
		}
		round.Evaluation = patch.CurrentEvaluation
	}
	if patch.CurrentRefinedEvaluation != nil {
		round := sess.CurrentRound()
		if round == nil {
			return fmt.Errorf("apply state patch: current round missing for refined evaluation")
		}
		round.RefinedEval = patch.CurrentRefinedEvaluation
	}
	if patch.CompleteCurrentRound != nil {
		round := sess.CurrentRound()
		if round == nil {
			return fmt.Errorf("apply state patch: current round missing for completion")
		}
		round.CompletedAt = *patch.CompleteCurrentRound
	}
	if patch.Report != nil {
		sess.Report = patch.Report
	}
	if patch.Status != nil {
		sess.Status = *patch.Status
	}
	if patch.WorkingMemory != nil {
		sess.WorkingMemory = patch.WorkingMemory
	}
	return nil
}
