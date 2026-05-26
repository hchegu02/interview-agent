package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
	"interview-agent/internal/llm"
	"interview-agent/internal/retriever"
)

// gap_analyze 设计要点：
//
//   1. 集合运算（matched / missing / overlap_score）走规则法
//      —— 简单、可解释、可单测，不烧 token。
//      Skill 在 parse_jd / parse_resume 出来时已经 canonical 化，
//      这里只是再走一遍 CanonicalizeTags 保底（防止有人手工填的 profile）。
//
//   2. Strategy 决策分两挡：
//      - 强匹配 (overlap >= 0.7) → 直接 validate
//      - 弱匹配 (overlap < 0.3) → 直接 cover_gap
//      - 中间地带 (0.3 ≤ overlap < 0.7) → LLM 兜底
//      理由：边界由阈值唯一确定，中间地带让 LLM 看上下文（年限差、加分项匹配等）
//      给个更靠谱的判断，并附 reason 用于报告页透出。
//
//   3. LLM 失败时降级为规则保底：取 overlap >= 0.5 ? validate : cover_gap
//      —— gap_analyze 是 critical path，不能让单次 LLM 抖动卡住整个会话。

const (
	overlapStrongMatchThreshold = 0.7
	overlapWeakMatchThreshold   = 0.3
)

// gapStrategyShape 是 LLM 兜底输出的 JSON 形状。
type gapStrategyShape struct {
	Strategy string `json:"strategy"`
	Reason   string `json:"reason"`
}

func validateGapStrategy(raw []byte) error {
	if err := llm.ValidateJSON(raw); err != nil {
		return err
	}
	if err := llm.ValidateFields(raw, "strategy", "reason"); err != nil {
		return err
	}
	return llm.ValidateEnum(raw, "strategy",
		string(domain.GapStrategyValidate),
		string(domain.GapStrategyExplore),
		string(domain.GapStrategyCoverGap),
	)
}

// NewGapAnalyzeNode 构造 gap_analyze 节点。
//
// model 用于 strategy 兜底；可传 nil（中间地带退化为 explore + "llm disabled" reason）。
func NewGapAnalyzeNode(model llm.ChatModel) graph.NodeFunc {
	return func(ctx context.Context, sess *domain.Session) error {
		if sess.JobProfile == nil {
			return fmt.Errorf("gap_analyze: job_profile required: %w", graph.ErrPermanent)
		}
		if sess.CandProfile == nil {
			return fmt.Errorf("gap_analyze: candidate_profile required: %w", graph.ErrPermanent)
		}

		jdSkills := retriever.CanonicalizeTags(sess.JobProfile.KeySkills)
		candSkills := retriever.CanonicalizeTags(sess.CandProfile.Skills)
		matched, missing := setDiff(jdSkills, candSkills)

		var overlap float64
		if n := len(jdSkills); n > 0 {
			overlap = float64(len(matched)) / float64(n)
		}

		report := &domain.GapReport{
			MatchedSkills: matched,
			MissingSkills: missing,
			OverlapScore:  overlap,
		}

		// 写回到 CandProfile.WeakSkills（保留向下兼容；老代码可能读这个字段）
		sess.CandProfile.WeakSkills = missing

		// Strategy 决策
		switch {
		case overlap >= overlapStrongMatchThreshold:
			report.Strategy = domain.GapStrategyValidate
			report.Reason = fmt.Sprintf("强匹配 %.2f，验证简历真实性", overlap)

		case overlap < overlapWeakMatchThreshold:
			report.Strategy = domain.GapStrategyCoverGap
			report.Reason = fmt.Sprintf("弱匹配 %.2f，优先覆盖缺失技能", overlap)

		default:
			// 中间地带：LLM 兜底
			s, reason, err := strategyByLLM(ctx, model, matched, missing, overlap,
				sess.CandProfile.Years, sess.JobProfile.YearsRequired)
			if err != nil {
				// 降级：用更保守的 explore 策略 + 标注原因
				report.Strategy = domain.GapStrategyExplore
				report.Reason = fmt.Sprintf("LLM 兜底失败，降级 explore：%v", err)
			} else {
				report.Strategy = s
				report.Reason = reason
			}
		}

		sess.GapReport = report
		return nil
	}
}

// setDiff 返回 (matched, missing)。
// matched = jd ∩ cand；missing = jd - cand。
// 输出按字典序排，保证可复现。
func setDiff(jd, cand []string) (matched, missing []string) {
	candSet := make(map[string]struct{}, len(cand))
	for _, s := range cand {
		candSet[s] = struct{}{}
	}
	for _, s := range jd {
		if _, ok := candSet[s]; ok {
			matched = append(matched, s)
		} else {
			missing = append(missing, s)
		}
	}
	sort.Strings(matched)
	sort.Strings(missing)
	return
}

// strategyByLLM 让 LLM 在中间地带挑 strategy。
// model == nil 时返回 explore + "llm disabled" reason，让上层走"无 LLM 兜底"路径。
func strategyByLLM(
	ctx context.Context,
	model llm.ChatModel,
	matched, missing []string,
	overlap float64,
	candYears, reqYears int,
) (domain.GapStrategy, string, error) {
	if model == nil {
		return domain.GapStrategyExplore, "llm disabled, default explore", nil
	}

	prompt := fmt.Sprintf(promptGapStrategyFallback,
		matched, missing, overlap, candYears, reqYears)
	messages := []llm.Message{{Role: "system", Content: prompt}}
	opts := llm.Options{Temperature: 0.2, MaxTokens: 200}

	resp, err := llm.CallWithSchema(ctx, model, messages, opts, validateGapStrategy, 1)
	if err != nil {
		return "", "", err
	}
	var shape gapStrategyShape
	if err := json.Unmarshal([]byte(resp.Content), &shape); err != nil {
		return "", "", fmt.Errorf("unmarshal strategy: %w", err)
	}
	s := domain.GapStrategy(strings.TrimSpace(shape.Strategy))
	if err := s.Validate(); err != nil {
		return "", "", err
	}
	return s, shape.Reason, nil
}
