package nodes

import (
	"context"
	"fmt"
	"time"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

// update_memory 节点设计要点:
//
//   1. 纯本地聚合, 不调 LLM:
//      读 CurrentRound() 的 FinalEvaluation(refined > original 自动生效),
//      把分数 + 追答评估合并写回 WorkingMemory。零 LLM 开销, 错误面小。
//
//   2. 增量更新 + 单轮口径:
//      只看 CurrentRound, 不重扫 sess.Rounds。RoundsAsked 已经在 pick_next 里 ++ 过,
//      这里只负责 "把本轮成绩翻译成 WorkingMemory 信号"。
//      AvgScore 用 incremental formula (旧均值 + 增量/n) 避免 O(N) 重扫,
//      ConfirmedRoundsCount 单独记账给增量公式用。
//
//   3. 主答 + 追答加权:
//      combined = main_score * 0.7 + followups_avg_score * 0.3
//      只把 score>=0 的追答评估纳入均分; 没追答则 combined = main_score。
//      这样 probe 链路深挖出来的成果(优秀的追答) 也能拉高/拉低本题的最终覆盖度,
//      不会被"答主答时不完整, 追问后补救"的场景埋没。
//
//   4. 覆盖度 = 加权累加(float):
//      SkillCoverage[tag] += combined/100, 在 [0,1] 累加, 高分多累加得多。
//      pick_next 的"选 coverage 最低的"语义不变(数值比较), 但更贴近"已掌握程度"
//      而不是"考过几次"——同样考 3 道 redis 但 3 题都满分 vs 都零分,
//      coverage 应该有区别。
//
//   5. 降级 round 跳过:
//      FinalEvaluation.Score == -1 表示上游 evaluate 已降级,
//      这种数据不参与任何统计(否则把 SkillCoverage 拉低误导 pick_next),
//      只在 WorkingMemory.DegradedRounds 上 +1 给 reflection_check / report 观察。
//
//   6. 信号集合维护:
//      combined >= 70 → 写入 ConfirmedSkills, 同时清掉 WeakSkills 里同名项
//      combined <  50 → 写入 WeakSkills,    同时清掉 ConfirmedSkills 里同名项
//      50 ≤ combined < 70 是中间地带, 不动两个集合(让现有标签维持)。
//      集合内部去重 + 稳定顺序(按字符串排序)。

// UpdateMemoryOptions 暴露给图组装的参数。当前没有可调项, 留 placeholder
// 以便日后加权重等参数时不破坏调用签名。
type UpdateMemoryOptions struct {
	// MainWeight 主答在 combined 里的权重, 默认 0.7
	MainWeight float64
	// FollowUpWeight 追答均分在 combined 里的权重, 默认 0.3
	FollowUpWeight float64
	// ConfirmThreshold combined 分数 >= 此值进 ConfirmedSkills, 默认 70
	ConfirmThreshold int
	// WeakThreshold combined 分数 < 此值进 WeakSkills, 默认 50
	WeakThreshold int
}

// NewUpdateMemoryNode 构造 update_memory 节点。
//
// 节点契约:
//   输入: CurrentRound() 存在, FinalEvaluation 已填(score 可为 -1)
//   输出: 本轮信号合并入 WorkingMemory; round.CompletedAt 标记
//   返回: nil(始终); ErrPermanent: 无 round 或 无 final eval
func NewUpdateMemoryNode(opts UpdateMemoryOptions) graph.NodeFunc {
	if opts.MainWeight == 0 && opts.FollowUpWeight == 0 {
		opts.MainWeight = 0.7
		opts.FollowUpWeight = 0.3
	}
	if opts.ConfirmThreshold == 0 {
		opts.ConfirmThreshold = 70
	}
	if opts.WeakThreshold == 0 {
		opts.WeakThreshold = 50
	}

	return func(ctx context.Context, sess *domain.Session) error {
		round := sess.CurrentRound()
		if round == nil {
			return fmt.Errorf("update_memory: no current round: %w", graph.ErrPermanent)
		}
		final := round.FinalEvaluation()
		if final == nil {
			return fmt.Errorf("update_memory: no final evaluation: %w", graph.ErrPermanent)
		}
		if sess.WorkingMemory == nil {
			sess.WorkingMemory = domain.NewWorkingMemory()
		}
		mem := sess.WorkingMemory

		// 1. 降级数据: 不入任何统计, 只记账
		if final.Score < 0 {
			markDegradedRound(mem)
			round.CompletedAt = time.Now()
			return nil
		}

		// 2. 合并主答 + 追答评估
		combined := combinedScore(round, final.Score, opts)

		// 3. 决定本轮影响的 tags
		tags := targetTags(&round.Question)

		// 4. 覆盖度累加(归一化到 [0,1])
		if mem.SkillCoverage == nil {
			mem.SkillCoverage = map[string]float64{}
		}
		norm := float64(combined) / 100.0
		for _, t := range tags {
			mem.SkillCoverage[t] += norm
		}

		// 5. 信号集合维护
		switch {
		case combined >= opts.ConfirmThreshold:
			mem.ConfirmedSkills = addUnique(mem.ConfirmedSkills, tags)
			mem.WeakSkills = removeAll(mem.WeakSkills, tags)
		case combined < opts.WeakThreshold:
			mem.WeakSkills = addUnique(mem.WeakSkills, tags)
			mem.ConfirmedSkills = removeAll(mem.ConfirmedSkills, tags)
		}

		// 6. AvgScore 增量更新
		// 公式: new = old + (sample - old)/n, n = 已结算非降级的 round 数
		mem.ScoredRounds++
		mem.AvgScore = mem.AvgScore + (float64(combined)-mem.AvgScore)/float64(mem.ScoredRounds)

		round.CompletedAt = time.Now()
		return nil
	}
}

// combinedScore = main * w_main + followups_avg * w_follow
// 没有有效追答时退化为 main_score; 权重传 0 时也走 main_score 兜底
func combinedScore(round *domain.AnswerRound, mainScore int, opts UpdateMemoryOptions) int {
	wMain := opts.MainWeight
	wFollow := opts.FollowUpWeight
	if wMain <= 0 {
		return mainScore
	}

	var sum, n float64
	for _, f := range round.FollowUps {
		if f.Evaluation == nil || f.Evaluation.Score < 0 {
			continue
		}
		sum += float64(f.Evaluation.Score)
		n++
	}
	if n == 0 || wFollow <= 0 {
		return mainScore
	}
	followAvg := sum / n
	c := float64(mainScore)*wMain + followAvg*wFollow
	// 权重不一定和为 1, 这里按权重和归一化, 让 combined 仍在 [0,100]
	c /= (wMain + wFollow)
	if c < 0 {
		c = 0
	}
	if c > 100 {
		c = 100
	}
	return int(c + 0.5) // 四舍五入
}

// targetTags: 优先 SkillCategory(题库锚定的主类目), 没填则用 Tags
// 没 Tags 也没 SkillCategory 的题不影响信号(纯锻炼用)
func targetTags(q *domain.Question) []string {
	if q.SkillCategory != "" {
		return []string{q.SkillCategory}
	}
	return q.Tags
}

func addUnique(set []string, add []string) []string {
	exist := map[string]struct{}{}
	for _, s := range set {
		exist[s] = struct{}{}
	}
	for _, s := range add {
		if _, ok := exist[s]; ok || s == "" {
			continue
		}
		set = append(set, s)
		exist[s] = struct{}{}
	}
	return set
}

func removeAll(set []string, remove []string) []string {
	if len(set) == 0 || len(remove) == 0 {
		return set
	}
	bad := map[string]struct{}{}
	for _, s := range remove {
		bad[s] = struct{}{}
	}
	out := set[:0]
	for _, s := range set {
		if _, ok := bad[s]; !ok {
			out = append(out, s)
		}
	}
	return out
}

func markDegradedRound(mem *domain.WorkingMemory) {
	mem.DegradedRounds++
}
