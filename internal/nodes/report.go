package nodes

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

// NewReportNode builds the final interview report from local session state.
//
// The report node deliberately avoids LLM calls: by the time the graph reaches
// report, evaluations and working memory already contain the decision signals.
func NewReportNode() graph.NodeFunc {
	return func(ctx context.Context, sess *domain.Session) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if sess.Status != "" {
			if err := sess.Status.Validate(); err != nil {
				return graph.Permanent(fmt.Errorf("report: invalid session status: %w", err))
			}
		}
		if sess.WorkingMemory == nil {
			sess.WorkingMemory = domain.NewWorkingMemory()
		}

		report := &domain.Report{
			SessionID:      sess.ID,
			OverallScore:   overallScore(sess.Rounds),
			SkillBreakdown: skillBreakdown(sess.WorkingMemory.SkillCoverage),
			Highlights:     reportHighlights(sess),
			Improvements:   reportImprovements(sess),
			NextSteps:      reportNextSteps(sess),
		}
		sess.Report = report
		sess.Status = domain.StatusCompleted
		sess.PendingDecision = nil
		return nil
	}
}

func overallScore(rounds []domain.AnswerRound) int {
	var sum, n int
	for i := range rounds {
		ev := rounds[i].FinalEvaluation()
		if ev == nil || ev.Score < 0 {
			continue
		}
		sum += ev.Score
		n++
	}
	if n == 0 {
		return 0
	}
	return (sum + n/2) / n
}

func skillBreakdown(coverage map[string]float64) map[string]int {
	out := make(map[string]int, len(coverage))
	for skill, cov := range coverage {
		if skill == "" {
			continue
		}
		score := int(cov*100 + 0.5)
		if score < 0 {
			score = 0
		}
		if score > 100 {
			score = 100
		}
		out[skill] = score
	}
	return out
}

func reportHighlights(sess *domain.Session) []string {
	var out []string
	mem := sess.WorkingMemory
	if len(mem.ConfirmedSkills) > 0 {
		out = append(out, "已确认技能："+strings.Join(sortedStrings(mem.ConfirmedSkills), "、"))
	}
	for _, s := range collectEvalText(sess.Rounds, func(ev *domain.Evaluation) []string {
		return ev.Strengths
	}) {
		out = append(out, s)
	}
	if len(out) == 0 {
		out = append(out, "暂无明确亮点，建议补充更多答题样本")
	}
	return uniqueStrings(out)
}

func reportImprovements(sess *domain.Session) []string {
	var out []string
	mem := sess.WorkingMemory
	if len(mem.WeakSkills) > 0 {
		out = append(out, "待加强技能："+strings.Join(sortedStrings(mem.WeakSkills), "、"))
	}
	for _, s := range collectEvalText(sess.Rounds, func(ev *domain.Evaluation) []string {
		return ev.Weaknesses
	}) {
		out = append(out, s)
	}
	if len(out) == 0 {
		out = append(out, "暂无明确短板，建议继续用更高难度题验证深度")
	}
	return uniqueStrings(out)
}

func reportNextSteps(sess *domain.Session) []string {
	mem := sess.WorkingMemory
	var out []string
	for _, skill := range sortedStrings(mem.WeakSkills) {
		out = append(out, fmt.Sprintf("优先补强 %s 相关题目与知识点", skill))
	}
	for _, s := range collectEvalText(sess.Rounds, func(ev *domain.Evaluation) []string {
		if strings.TrimSpace(ev.Suggestion) == "" {
			return nil
		}
		return []string{ev.Suggestion}
	}) {
		out = append(out, s)
	}
	if len(mem.DegradedReasons) > 0 {
		out = append(out, fmt.Sprintf("评估过程中部分环节降级：%s；建议复测这些环节以提高报告可信度",
			strings.Join(sortedMapKeys(mem.DegradedReasons), "、")))
	}
	if len(out) == 0 {
		out = append(out, "继续完成更多模拟面试轮次，积累稳定评估样本")
	}
	return uniqueStrings(out)
}

func collectEvalText(rounds []domain.AnswerRound, pick func(*domain.Evaluation) []string) []string {
	var out []string
	for i := range rounds {
		ev := rounds[i].FinalEvaluation()
		if ev == nil || ev.Score < 0 {
			continue
		}
		for _, s := range pick(ev) {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func sortedMapKeys(in map[string]string) []string {
	out := make([]string, 0, len(in))
	for k := range in {
		if strings.TrimSpace(k) != "" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
