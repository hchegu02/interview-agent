package domain

import (
	"fmt"
	"time"
)

// StatePatch 表达节点对 Session 的结构化写入意图。
// 第一阶段只覆盖高风险字段，避免节点随意改大对象。
type StatePatch struct {
	CandidatePool        *[]Question
	RetrievalTrace       *RetrievalTrace
	PendingDecision      *Decision
	ClearPendingDecision bool
	AppendRound          *AnswerRound
	CurrentEvaluation    *Evaluation
	CompleteCurrentRound *time.Time
	Report               *Report
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
	if patch.CurrentEvaluation != nil {
		round := sess.CurrentRound()
		if round == nil {
			return fmt.Errorf("apply state patch: current round missing for evaluation")
		}
		round.Evaluation = patch.CurrentEvaluation
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
	return nil
}
