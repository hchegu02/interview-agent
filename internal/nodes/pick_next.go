package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
	"interview-agent/internal/llm"
)

// pick_next 节点设计要点:
//
//   1. LLM 主导 + 规则约束:
//      候选池由 retrieve_rag 召回,LLM 在"已过滤+未问过"的候选里选,
//      代码侧负责"硬过滤"(已问过的题 / 不在池里的 id),LLM 只做"决策+解释"。
//      这样能解释性强(reasoning 落到报告页),又不会被 LLM 编造的 id 干扰。
//
//   2. 出完题立刻 suspend:
//      pick_next 写 PendingDecision + 新建 AnswerRound(Answer 留空),
//      然后返回 ErrSuspended,框架把 sess.CurrentNode=pick_next 持久化。
//      HTTP 层收到 suspend 后把题推给前端,等用户答完写回 CurrentRound.Answer,
//      调 Resume 推进到 evaluate。
//
//   3. 结束判定内置:
//      RemainingRounds==0 或 候选池全问完 → 写 Decision{Action:end},不 suspend,
//      正常返回让下游 router 走到 report。这样图结构里 pick_next 只有一个出度,
//      router 在 PendingDecision.Action 上分支。
//
//   4. reflect 补漏:
//      reflection_check 写入 WorkingMemory.ReflectTopic 后会跳回 pick_next。
//      pick_next 消费并清空该字段,优先把候选池缩到匹配 topic 的题。
//
//   5. LLM 失败兜底:
//      pick_next 是 critical path,LLM 抖动不能卡住会话。
//      失败时退化为"按 skill_coverage 最低的题随机一道",并在 DegradedReasons 打降级标记。

// PickNextOptions 暴露给图组装的可调参数。
type PickNextOptions struct {
	// Temperature 默认 0.3,选题需要一点随机但不能太发散
	Temperature float64
	// MaxTokens 默认 200,只输出 id + 短 reason
	MaxTokens int
	// RecentRoundsForContext 给 LLM 看最近几轮(避免 prompt 爆炸),默认 3
	RecentRoundsForContext int
}

// pickNextShape 是 LLM 输出 JSON 的形状。
type pickNextShape struct {
	NextQuestionID string `json:"next_question_id"`
	Reasoning      string `json:"reasoning"`
}

// NewPickNextNode 构造 pick_next 节点。
//
// 节点契约:
//
//	输入: sess.JobProfile / sess.GapReport / sess.CandidatePool / sess.WorkingMemory
//	输出:
//	  - 正常出题: sess.PendingDecision (Action=ask_new) + Rounds 追加新 round + 返回 ErrSuspended
//	  - 结束面试: sess.PendingDecision (Action=end) + 返回 nil
//
// model 可为 nil(测试 / 完全规则模式),走降级路径。
func NewPickNextNode(model llm.ChatModel, opts PickNextOptions) graph.NodeFunc {
	if opts.Temperature == 0 {
		opts.Temperature = 0.3
	}
	if opts.MaxTokens == 0 {
		opts.MaxTokens = 200
	}
	if opts.RecentRoundsForContext == 0 {
		opts.RecentRoundsForContext = 3
	}

	return func(ctx context.Context, sess *domain.Session) error {
		if sess.JobProfile == nil {
			return fmt.Errorf("pick_next: job_profile required: %w", graph.ErrPermanent)
		}
		if sess.WorkingMemory == nil {
			sess.WorkingMemory = domain.NewWorkingMemory()
		}
		mem := sess.WorkingMemory

		// 1. 过滤候选:去掉已问过的题
		asked := make(map[string]struct{}, len(sess.Rounds))
		for i := range sess.Rounds {
			asked[sess.Rounds[i].Question.ID] = struct{}{}
		}
		pool := make([]domain.Question, 0, len(sess.CandidatePool))
		for _, q := range sess.CandidatePool {
			if _, ok := asked[q.ID]; !ok {
				pool = append(pool, q)
			}
		}

		// 2. 终止判定
		if mem.RemainingRounds() <= 0 || len(pool) == 0 {
			reason := "已达最大轮次,转生成报告"
			if len(pool) == 0 {
				reason = "候选题池已耗尽,转生成报告"
			}
			decision := &domain.Decision{
				Action:    domain.ActionEnd,
				Reasoning: reason,
				DecidedAt: time.Now(),
			}
			return applyNodePatch(sess, "pick_next", domain.StatePatch{PendingDecision: decision})
		}

		// 3. 如果 reflection_check 指定了补漏 topic,优先缩小候选池
		reflectTopic := consumeReflectTopic(mem)
		if reflectTopic != "" {
			if reflected := filterByReflectTopic(pool, reflectTopic); len(reflected) > 0 {
				pool = reflected
			}
		}

		// 4. 选题:LLM 主导,失败降级到规则
		picked, reasoning, err := pickByLLM(ctx, model, sess, pool, opts)
		if err != nil {
			markPickFallback(sess, err.Error())
			picked, reasoning = pickByRule(pool, mem)
		}

		// 5. 写 PendingDecision + 新建 round
		now := time.Now()
		decision := &domain.Decision{
			Action:         domain.ActionAskNew,
			Reasoning:      reasoning,
			NextQuestionID: picked.ID,
			DecidedAt:      now,
		}
		round := &domain.AnswerRound{
			RoundID:    fmt.Sprintf("r%d-%d", len(sess.Rounds)+1, now.UnixNano()),
			Question:   picked,
			PickReason: reasoning,
			DecidedAt:  now,
		}
		if err := applyNodePatch(sess, "pick_next", domain.StatePatch{
			PendingDecision: decision,
			AppendRound:     round,
		}); err != nil {
			return err
		}
		mem.RoundsAsked++

		// 6. suspend 等用户答题
		return fmt.Errorf("pick_next: waiting for candidate answer (round=%d): %w",
			mem.RoundsAsked, graph.ErrSuspended)
	}
}

func consumeReflectTopic(mem *domain.WorkingMemory) string {
	topic := strings.TrimSpace(mem.ReflectTopic)
	mem.ReflectTopic = ""
	if topic == "" && mem.Notes != nil {
		topic = strings.TrimSpace(mem.Notes["reflect_topic"])
	}
	if mem.Notes != nil {
		delete(mem.Notes, "reflect_topic")
	}
	return topic
}

func filterByReflectTopic(pool []domain.Question, topic string) []domain.Question {
	if topic == "" {
		return pool
	}
	out := make([]domain.Question, 0, len(pool))
	for _, q := range pool {
		if q.SkillCategory == topic || containsStr(q.Tags, topic) {
			out = append(out, q)
		}
	}
	return out
}

// pickByLLM 让 LLM 在候选池里挑一道并给 reasoning。
func pickByLLM(
	ctx context.Context,
	model llm.ChatModel,
	sess *domain.Session,
	pool []domain.Question,
	opts PickNextOptions,
) (domain.Question, string, error) {
	if model == nil {
		return domain.Question{}, "", fmt.Errorf("llm disabled")
	}

	job := sess.JobProfile
	cand := sess.CandProfile
	gap := sess.GapReport
	mem := sess.WorkingMemory

	candYears := 0
	if cand != nil {
		candYears = cand.Years
	}
	strategy, gapReason := "explore", ""
	if gap != nil {
		strategy, gapReason = string(gap.Strategy), gap.Reason
	}

	covJSON, _ := json.Marshal(mem.SkillCoverage)
	prompt := fmt.Sprintf(promptPickNext,
		job.Title, candYears, job.YearsRequired,
		strategy, gapReason,
		mem.ConfirmedSkills, mem.WeakSkills,
		string(covJSON),
		mem.AvgScore, mem.RoundsAsked, mem.MaxRounds,
		formatRecentRounds(sess.Rounds, opts.RecentRoundsForContext),
		formatCandidatePool(pool),
	)
	messages := []llm.Message{{Role: "system", Content: prompt}}
	llmOpts := llm.Options{Temperature: opts.Temperature, MaxTokens: opts.MaxTokens}

	// 构造闭包校验器:除了 JSON / 字段 / 也要校验 id 在池子里
	poolIDs := make(map[string]int, len(pool))
	for i, q := range pool {
		poolIDs[q.ID] = i
	}
	validator := func(raw []byte) error {
		if err := llm.ValidateJSON(raw); err != nil {
			return err
		}
		if err := llm.ValidateFields(raw, "next_question_id", "reasoning"); err != nil {
			return err
		}
		var s pickNextShape
		if err := json.Unmarshal(raw, &s); err != nil {
			return err
		}
		if _, ok := poolIDs[strings.TrimSpace(s.NextQuestionID)]; !ok {
			return fmt.Errorf("next_question_id %q not in candidate pool", s.NextQuestionID)
		}
		return nil
	}

	resp, err := llm.CallWithSchema(ctx, model, messages, llmOpts, validator, 1)
	if err != nil {
		return domain.Question{}, "", err
	}
	var shape pickNextShape
	if err := json.Unmarshal([]byte(resp.Content), &shape); err != nil {
		return domain.Question{}, "", err
	}
	idx := poolIDs[strings.TrimSpace(shape.NextQuestionID)]
	return pool[idx], strings.TrimSpace(shape.Reasoning), nil
}

// pickByRule 是 LLM 失败时的规则降级:
//   - 选 skill_coverage 最低的 category 对应的题
//   - 同分按 candidate pool 顺序取第一个(retrieve_rag 已按相关性排过)
func pickByRule(pool []domain.Question, mem *domain.WorkingMemory) (domain.Question, string) {
	best := pool[0]
	bestCov := mem.SkillCoverage[best.SkillCategory]
	for _, q := range pool[1:] {
		if mem.SkillCoverage[q.SkillCategory] < bestCov {
			best = q
			bestCov = mem.SkillCoverage[q.SkillCategory]
		}
	}
	return best, fmt.Sprintf("规则降级: 选 coverage 最低的 %s 类目题", best.SkillCategory)
}

// markPickFallback 在 DegradedReasons 打降级标记,SSE 层可透出"题目选择降级中"。
func markPickFallback(sess *domain.Session, reason string) {
	markDegradedReason(sess.WorkingMemory, "pick", reason)
}

// formatCandidatePool 拼候选题列表给 LLM,只暴露关键字段控制 prompt 长度。
// 按 id 排序,让 prompt 在测试里可复现。
func formatCandidatePool(pool []domain.Question) string {
	sorted := make([]domain.Question, len(pool))
	copy(sorted, pool)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	var sb strings.Builder
	for _, q := range sorted {
		fmt.Fprintf(&sb, "- id=%s  diff=%d  cat=%s  tags=%v\n  content=%s\n",
			q.ID, q.Difficulty, q.SkillCategory, q.Tags, truncate(q.Content, 80))
	}
	return sb.String()
}

// formatRecentRounds 给 LLM 喂最近 n 轮的"题目+得分"摘要,prompt 不至于膨胀。
func formatRecentRounds(rounds []domain.AnswerRound, n int) string {
	if len(rounds) == 0 {
		return "(暂无)"
	}
	start := len(rounds) - n
	if start < 0 {
		start = 0
	}
	var sb strings.Builder
	for i := start; i < len(rounds); i++ {
		r := rounds[i]
		score := -1
		if ev := r.FinalEvaluation(); ev != nil {
			score = ev.Score
		}
		fmt.Fprintf(&sb, "- [%s] cat=%s score=%d 内容=%s\n",
			r.RoundID, r.Question.SkillCategory, score, truncate(r.Question.Content, 50))
	}
	return sb.String()
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
