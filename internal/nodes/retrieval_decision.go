package nodes

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"interview-agent/internal/domain"
)

const (
	retrievalStrategyAskNew                = "ask_new"
	retrievalStrategyClarifyLowInformation = "clarify_low_information"
	retrievalStrategyEnd                   = "end"

	retrievalDecisionNoteKey = "retrieval_decision"
)

type RetrievalDecisionInput struct {
	Answer          string
	CandidatePool   []domain.Question
	RetrievalTrace  *domain.RetrievalTrace
	UsedQuestionIDs []string
	WorkingMemory   *domain.WorkingMemory
}

type RetrievalDecision struct {
	Strategy       string
	IncludeContext bool
	Selected       []domain.Question
	ConsumedIDs    []string
	Reason         string
	DegradedReason string
	LowInformation bool
	WeakRecall     bool
}

func decideRuntimeRetrieval(in RetrievalDecisionInput) RetrievalDecision {
	used := stringSetFold(in.UsedQuestionIDs)
	selected, consumed := filterUsedQuestions(in.CandidatePool, used)
	lowInfo := strings.TrimSpace(in.Answer) != "" && isLowInformationAnswer(in.Answer)
	weakRecall := isWeakRetrievalTrace(in.RetrievalTrace)

	decision := RetrievalDecision{
		Strategy:       retrievalStrategyAskNew,
		IncludeContext: !weakRecall,
		Selected:       selected,
		ConsumedIDs:    consumed,
		LowInformation: lowInfo,
		WeakRecall:     weakRecall,
	}
	if len(selected) == 0 {
		decision.Strategy = retrievalStrategyEnd
		decision.IncludeContext = false
		decision.Reason = fmt.Sprintf("候选题池已耗尽，已排除 %d 道已用题", len(consumed))
		decision.DegradedReason = "candidate_pool_exhausted_after_used_question_exclusion"
		return decision
	}
	switch {
	case lowInfo && weakRecall:
		decision.Strategy = retrievalStrategyClarifyLowInformation
		decision.IncludeContext = false
		decision.Reason = "回答信息量低且检索证据弱，先追问澄清，不继续放大弱召回上下文"
		decision.DegradedReason = "weak_recall_low_information_answer"
	case lowInfo:
		decision.Strategy = retrievalStrategyClarifyLowInformation
		decision.Reason = "回答信息量低，优先追问澄清核心思路"
	case weakRecall:
		decision.IncludeContext = false
		decision.Reason = "检索证据弱，出题仅使用候选池硬事实，不扩展上下文"
		decision.DegradedReason = "weak_retrieval_recall"
	default:
		decision.Reason = "检索证据可用，按候选池和运行时记忆继续出题"
	}
	if len(consumed) > 0 {
		decision.Reason += fmt.Sprintf("；已排除 %d 道已用题", len(consumed))
	}
	return decision
}

func filterUsedQuestions(pool []domain.Question, used map[string]struct{}) ([]domain.Question, []string) {
	out := make([]domain.Question, 0, len(pool))
	var consumed []string
	for _, q := range pool {
		id := strings.ToLower(strings.TrimSpace(q.ID))
		if id == "" {
			continue
		}
		if _, ok := used[id]; ok {
			consumed = append(consumed, q.ID)
			continue
		}
		out = append(out, q)
	}
	return out, consumed
}

func usedQuestionIDs(sess *domain.Session) []string {
	if sess == nil {
		return nil
	}
	out := make([]string, 0, len(sess.Rounds))
	for _, round := range sess.Rounds {
		if id := strings.TrimSpace(round.Question.ID); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func lastCompletedAnswer(sess *domain.Session) string {
	if sess == nil {
		return ""
	}
	for i := len(sess.Rounds) - 1; i >= 0; i-- {
		if strings.TrimSpace(sess.Rounds[i].Answer) != "" {
			return sess.Rounds[i].Answer
		}
	}
	return ""
}

func isLowInformationAnswer(answer string) bool {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return true
	}
	if utf8.RuneCountInString(answer) < 18 {
		return true
	}
	lower := strings.ToLower(answer)
	lowSignals := []string{
		"不知道", "不清楚", "不了解", "没用过", "不会", "不太会",
		"不知道怎么说", "没有思路", "pass", "skip", "no idea",
	}
	for _, signal := range lowSignals {
		if strings.Contains(lower, signal) {
			return true
		}
	}
	return false
}

func isWeakRetrievalTrace(trace *domain.RetrievalTrace) bool {
	if trace == nil {
		return true
	}
	if len(trace.FallbackReasons) > 0 {
		return true
	}
	if len(trace.Final) == 0 {
		return true
	}
	topScore := trace.Final[0].Score
	if topScore > 0 && topScore < 0.25 {
		return true
	}
	nonEmptyStages := 0
	for _, stage := range trace.Stages {
		if stage.Count > 0 || len(stage.Items) > 0 {
			nonEmptyStages++
		}
	}
	return nonEmptyStages == 0 && len(trace.Final) < 2
}

func stringSetFold(items []string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			out[item] = struct{}{}
		}
	}
	return out
}

func recordRetrievalDecision(mem *domain.WorkingMemory, decision RetrievalDecision) {
	if mem == nil {
		return
	}
	if mem.Notes == nil {
		mem.Notes = map[string]string{}
	}
	mem.Notes[retrievalDecisionNoteKey] = decision.Strategy + ": " + decision.Reason
	if decision.DegradedReason != "" {
		markDegradedReason(mem, "retrieval_decision", decision.DegradedReason)
	}
}
