package nodes

import (
	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

// routers.go: Agent 循环里的条件分支边。
//
// 拆成多个专职 router 而非一个集中 dispatcher 的理由:
//   - 每个 router 只看 session 的一小段状态(critic / followup / decision),
//     职责清晰、易单测、易在图组装时按分支点挂载。
//   - graph.Router 类型是 func(*Session) string, 单参数纯函数, 天然适合拆。
//   - 想看"critic 之后会去哪"翻 RouteAfterCritic 一处即可, 不用通读集中 router。
//
// 节点名约定(图组装时使用):
//   "pick_next" / "evaluate" / "critic" / "refine" / "probe_ask" / "probe_eval"
//   "update_memory" / "reflection_check" / "report"
//
// 在 graph.AddBranch 处使用, 例如:
//   g.AddBranch("critic", nodes.RouteAfterCritic)

const (
	NodeEvaluate         = "evaluate"
	NodeRefine           = "refine"
	NodeProbeAsk         = "probe_ask"
	NodeUpdateMemory     = "update_memory"
	NodePickNext         = "pick_next"
	NodeReport           = "report"
)

// RouteAfterPickNext: pick_next 之后的分支。
// pick_next 有两种结局:
//   - 成功选题: ErrSuspended (run() 已返回), Resume 后到这里继续走 evaluate
//   - 题池耗尽 / 无预算: 返回 nil + PendingDecision.Action=end → 直接进 report
// router 看 PendingDecision.Action 区分。
func RouteAfterPickNext(sess *domain.Session) string {
	if sess.PendingDecision != nil && sess.PendingDecision.Action == domain.ActionEnd {
		return NodeReport
	}
	return NodeEvaluate
}

// RouteAfterCritic: critic 节点之后的分支。
//   NeedRefine    → refine
//   HasProbeSignal → probe_ask
//   都没有        → update_memory
//
// 优先级 refine > probe 是因为 refine 是对评估本身的修正,
// 修正完的 evaluation 仍可能触发 probe(由 critic 信号决定);
// 这里只是首跳, 后续 RouteAfterRefine 会再看一次 probe 信号。
func RouteAfterCritic(sess *domain.Session) string {
	round := sess.CurrentRound()
	if round == nil || round.CriticResult == nil {
		return NodeUpdateMemory
	}
	c := round.CriticResult
	if c.NeedRefine {
		return NodeRefine
	}
	if c.HasProbeSignal {
		return NodeProbeAsk
	}
	return NodeUpdateMemory
}

// RouteAfterRefine: refine 完之后看是否还要 probe。
// refine 不会改写 probe 信号, 所以这里直接看 critic.HasProbeSignal。
func RouteAfterRefine(sess *domain.Session) string {
	round := sess.CurrentRound()
	if round == nil || round.CriticResult == nil {
		return NodeUpdateMemory
	}
	if round.CriticResult.HasProbeSignal {
		return NodeProbeAsk
	}
	return NodeUpdateMemory
}

// RouteAfterProbeEval: 多轮 probe 的关键 router。
// probe_eval 会重写 critic.HasProbeSignal:
//   - LLM 还想追 + 预算允许    → 信号留 true → 再回 probe_ask
//   - LLM 不追 / 预算耗尽 / 失败 → 信号置 false → 走 update_memory
// 所以本 router 逻辑很简单, 复杂度全在 probe_eval 里。
func RouteAfterProbeEval(sess *domain.Session) string {
	round := sess.CurrentRound()
	if round == nil || round.CriticResult == nil {
		return NodeUpdateMemory
	}
	if round.CriticResult.HasProbeSignal {
		return NodeProbeAsk
	}
	return NodeUpdateMemory
}

// RouteAfterReflection: Agent 循环的出口 router。
// reflection_check 把决策(ask_new/reflect/end)写进 sess.PendingDecision,
// 这里只做静态翻译:
//   ask_new  → pick_next
//   reflect  → pick_next   (ReflectTopic 已经在 WorkingMemory 里, pick_next 会读取)
//   end      → report      (没 PendingDecision 也按 end 处理, 防御性)
func RouteAfterReflection(sess *domain.Session) string {
	d := sess.PendingDecision
	if d == nil {
		return NodeReport
	}
	switch d.Action {
	case domain.ActionAskNew, domain.ActionReflect:
		return NodePickNext
	case domain.ActionEnd:
		return NodeReport
	default:
		// 未知 Action → 防御性兜底 end
		return NodeReport
	}
}

// 编译期保证 router 类型签名一致(以免后续重构跑偏)。
var (
	_ graph.Router = RouteAfterPickNext
	_ graph.Router = RouteAfterCritic
	_ graph.Router = RouteAfterRefine
	_ graph.Router = RouteAfterProbeEval
	_ graph.Router = RouteAfterReflection
)
