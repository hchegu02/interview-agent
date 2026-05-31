package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
	"interview-agent/internal/llm"
)

// reflection_check 节点设计要点:
//
//   1. Agent 循环出口决策:
//      位置: update_memory 之后, pick_next 之前(或 end)。每轮结算完才看
//      "是否值得 reflect / 是否该 end"。和 pick_next 同样输出 Decision,
//      但 Action 域是 {ask_new, reflect, end} 三选一。
//
//   2. LLM 主导 + 节点内预算硬约束:
//      LLM 看 WorkingMemory 摘要给推荐, 但下列情况节点强制改写, 不让 LLM 决定:
//        - RoundsAsked >= MaxRounds      → 强制 end (预算耗尽)
//        - LLM 选 reflect 但 !CanReflect → 改 ask_new (反思预算耗尽)
//        - LLM 选 reflect 但 WeakSkills 空 → 改 ask_new (没东西反思)
//        - LLM 选 ask_new 但 RemainingRounds==0 → 改 end (无新题预算)
//      这样下游 router 只需要看 Decision.Action, 不需要重复检查预算。
//
//   3. reflect 复用 pick_next 造环:
//      reflect 决策时把 topic 写到 WorkingMemory.ReflectTopic, router 跳回 pick_next;
//      pick_next 读到这个字段后优先选匹配 topic 的题。
//      ReflectionsUsed++ 在本节点完成(避免 pick_next 反复加)。
//      ReflectTopic 在 pick_next 消费后由 pick_next 清掉。
//
//   4. 失败降级:
//      LLM 出错时走规则 fallback——和"LLM 主导"相反但保证会话存活:
//        - WeakSkills 非空 + CanReflect + 有剩余主题题预算  → reflect (优先补漏)
//        - 还有主题题预算                                 → ask_new
//        - 都没了                                       → end
//      DegradedReasons 打 reflection。
//
//   5. 输出落点:
//      写 sess.PendingDecision = &Decision{...}。这里和 pick_next 共用同一字段——
//      下一个被执行的节点(pick_next 或 router→end) 会消费它。

type ReflectionCheckOptions struct {
	Temperature float64 // 默认 0.2
	MaxTokens   int     // 默认 300
	MinRounds   int     // 默认 3, 防止一问一报
}

type reflectionShape struct {
	Action        string `json:"action"`
	Reasoning     string `json:"reasoning"`
	ReflectTopic  string `json:"reflect_topic"`
}

func validateReflection(raw []byte) error {
	if err := llm.ValidateJSON(raw); err != nil {
		return err
	}
	if err := llm.ValidateFields(raw, "action", "reasoning", "reflect_topic"); err != nil {
		return err
	}
	var s reflectionShape
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	switch s.Action {
	case "ask_new", "reflect", "end":
		return nil
	default:
		return fmt.Errorf("invalid action %q", s.Action)
	}
}

// NewReflectionCheckNode 构造 reflection_check 节点。
//
// 节点契约:
//   输入: WorkingMemory 已初始化, 当前轮已 update_memory 完成
//   输出: sess.PendingDecision = Decision{Action, Reasoning, [ReflectTopic]}
//         如果 Action=reflect: WorkingMemory.ReflectTopic=topic, ReflectionsUsed++
//   返回: nil(始终)
func NewReflectionCheckNode(model llm.ChatModel, opts ReflectionCheckOptions) graph.NodeFunc {
	if opts.Temperature == 0 {
		opts.Temperature = 0.2
	}
	if opts.MaxTokens == 0 {
		opts.MaxTokens = 300
	}
	if opts.MinRounds == 0 {
		opts.MinRounds = 3
	}

	return func(ctx context.Context, sess *domain.Session) error {
		if sess.WorkingMemory == nil {
			sess.WorkingMemory = domain.NewWorkingMemory()
		}
		mem := sess.WorkingMemory

		// 0. 硬截断: 没主题题预算 → 直接 end, 不走 LLM
		if mem.RemainingRounds() <= 0 {
			sess.PendingDecision = &domain.Decision{
				Action:    domain.ActionEnd,
				Reasoning: "主题题预算耗尽,结束面试",
				DecidedAt: time.Now(),
			}
			return nil
		}

		// 1. LLM 推荐(失败走规则降级)
		shape, err := reflectionByLLM(ctx, model, sess, opts)
		if err != nil {
			markReflectionFallback(mem, err.Error())
			shape = ruleBasedReflection(mem)
		}

		// 2. 节点内预算强制约束(覆盖 LLM 输出)
		action := domain.Action(strings.TrimSpace(shape.Action))
		topic := strings.TrimSpace(shape.ReflectTopic)

		switch action {
		case domain.ActionReflect:
			if !mem.CanReflect() || len(mem.WeakSkills) == 0 {
				action = domain.ActionAskNew
				topic = ""
				shape.Reasoning = "原决策 reflect, 因" + reflectBlockReason(mem) + "改为 ask_new"
			} else if topic == "" || !containsStr(mem.WeakSkills, topic) {
				// topic 必须落在 WeakSkills 内, 否则取第一个 weak skill
				topic = mem.WeakSkills[0]
			}
		case domain.ActionAskNew:
			if mem.RemainingRounds() <= 0 {
				action = domain.ActionEnd
				shape.Reasoning = "原决策 ask_new, 但无剩余主题题预算, 改为 end"
			}
		case domain.ActionEnd:
			if mem.RoundsAsked < opts.MinRounds && mem.RemainingRounds() > 0 {
				action = domain.ActionAskNew
				shape.Reasoning = fmt.Sprintf("原决策 end, 但仅完成 %d/%d 道主题题, 未达到最小样本 %d, 改为 ask_new",
					mem.RoundsAsked, mem.MaxRounds, opts.MinRounds)
			}
		default:
			// schema validator 应该已经拦掉了, 但留个保险
			action = domain.ActionAskNew
		}

		decision := &domain.Decision{
			Action:    action,
			Reasoning: strings.TrimSpace(shape.Reasoning),
			DecidedAt: time.Now(),
		}
		if action == domain.ActionReflect {
			decision.ReflectTopic = topic
			mem.ReflectTopic = topic
			mem.ReflectionsUsed++
		}

		sess.PendingDecision = decision
		return nil
	}
}

func reflectionByLLM(
	ctx context.Context,
	model llm.ChatModel,
	sess *domain.Session,
	opts ReflectionCheckOptions,
) (*reflectionShape, error) {
	if model == nil {
		return nil, fmt.Errorf("llm disabled")
	}
	mem := sess.WorkingMemory

	prompt := fmt.Sprintf(promptReflectionCheck,
		mem.RoundsAsked, mem.MaxRounds,
		mem.AvgScore,
		mem.ConfirmedSkills,
		mem.WeakSkills,
		mem.SuspectedSkills,
		mem.MaxReflections-mem.ReflectionsUsed,
		mem.RemainingRounds(),
	)
	messages := []llm.Message{{Role: "system", Content: prompt}}
	llmOpts := llm.Options{Temperature: opts.Temperature, MaxTokens: opts.MaxTokens}

	resp, err := llm.CallWithSchema(ctx, model, messages, llmOpts, validateReflection, 1)
	if err != nil {
		return nil, err
	}
	var s reflectionShape
	if err := json.Unmarshal([]byte(resp.Content), &s); err != nil {
		return nil, fmt.Errorf("unmarshal reflection: %w", err)
	}
	return &s, nil
}

// ruleBasedReflection 是 LLM 降级时的规则 fallback。
// 优先级: 能 reflect 就 reflect > 还能 ask_new 就 ask_new > end
func ruleBasedReflection(mem *domain.WorkingMemory) *reflectionShape {
	if mem.CanReflect() && len(mem.WeakSkills) > 0 && mem.RemainingRounds() > 0 {
		return &reflectionShape{
			Action:       string(domain.ActionReflect),
			Reasoning:    fmt.Sprintf("规则降级: 还有 %d 个薄弱技能 + 反思预算未用", len(mem.WeakSkills)),
			ReflectTopic: mem.WeakSkills[0],
		}
	}
	if mem.RemainingRounds() > 0 {
		return &reflectionShape{
			Action:    string(domain.ActionAskNew),
			Reasoning: "规则降级: 继续出新题",
		}
	}
	return &reflectionShape{
		Action:    string(domain.ActionEnd),
		Reasoning: "规则降级: 预算耗尽",
	}
}

func reflectBlockReason(mem *domain.WorkingMemory) string {
	if !mem.CanReflect() {
		return "反思预算耗尽"
	}
	if len(mem.WeakSkills) == 0 {
		return "无薄弱技能可反思"
	}
	return "未知约束"
}

func markReflectionFallback(mem *domain.WorkingMemory, reason string) {
	markDegradedReason(mem, "reflection", reason)
}

func containsStr(set []string, s string) bool {
	for _, v := range set {
		if v == s {
			return true
		}
	}
	return false
}
