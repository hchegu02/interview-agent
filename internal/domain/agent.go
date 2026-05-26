// Package domain — 文件 agent.go 定义"自适应面试 Agent"运行时的核心类型。
//
// 这些类型把 Session 从"无状态 workflow"升级成"有状态 Agent"：
//   - AnswerRound: 一题的完整生命周期（含追问、反思、修正）
//   - WorkingMemory: Agent 决策所依赖的运行时记忆
//   - Decision: pick_next/probe_decider/reflection_check 三个决策点的产物
//   - Critic: 评估反思节点的输出
//
// 设计原则同 session.go：
//   - 所有字段 JSON 可序列化（Redis snapshot / PG state_json 共用）
//   - 子结构自包含，可独立单测
//   - 字段命名优先于"前向兼容"——这是新项目，schema 还在迭代
package domain

import (
	"fmt"
	"time"
)

// AnswerRound 是一道主题题的完整记录。
//
// 为什么不复用 Answer/Evaluation 两个 map：
//   - Agent 流程里"一题"包含多次 LLM 交互（评估 + critic + 可能的 refine + 多个追问），
//     map[QuestionID] 无法表达这种层次结构
//   - 时间顺序很关键（critic 后才有 refined eval；probe 是评估后才决定），
//     用 slice 天然保留时序
//   - 断点恢复时按 RoundID 而非 QuestionID 索引，避免同题被多次 probe 的歧义
type AnswerRound struct {
	RoundID string `json:"round_id"` // ULID

	// 这一轮选的题（由 pick_next_question 决策填入）
	Question Question `json:"question"`
	// 出题原因（LLM 决策时的 reasoning，供可解释性 / 调试）
	PickReason string `json:"pick_reason,omitempty"`

	// 候选人对主题题的回答（追问的答案在 FollowUps 里）
	Answer string `json:"answer"`

	// 主评估（evaluate 节点输出）
	Evaluation *Evaluation `json:"evaluation,omitempty"`

	// 评估反思（critic 节点输出）
	CriticResult *Critic `json:"critic_result,omitempty"`

	// critic 触发 refine 后的修正评估；为 nil 表示原评估通过
	RefinedEval *Evaluation `json:"refined_eval,omitempty"`

	// 追问列表，可为空
	FollowUps []FollowUp `json:"follow_ups,omitempty"`

	DecidedAt   time.Time `json:"decided_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// FinalEvaluation 返回 RefinedEval（如果存在）否则 Evaluation。
// Report 节点统计成绩时用这个，避免每处都写一遍 if-else。
func (r *AnswerRound) FinalEvaluation() *Evaluation {
	if r.RefinedEval != nil {
		return r.RefinedEval
	}
	return r.Evaluation
}

// FollowUp 是一次追问及其回答。
// Reason 字段保留追问触发的依据（critic 指出的 issue 或 probe_decider 的推理），
// 这是 Agent 可解释性的关键——面试官追问"为什么问这个"时能讲清楚。
type FollowUp struct {
	Question   string      `json:"question"`
	Answer     string      `json:"answer"`
	Evaluation *Evaluation `json:"evaluation,omitempty"`
	Reason     string      `json:"reason"`
	AskedAt    time.Time   `json:"asked_at"`
}

// Critic 是评估反思节点的输出。
//
// 为什么单独建模而不是塞进 Evaluation：
//   - critic 是对"评估行为"的评估，和被评估的答案是两个层次
//   - critic 模型可以独立换（用更便宜的小模型）
//   - 失败的 critic（NeedRefine=true）后，Evaluation 会被替换，
//     但原始 Evaluation 仍保留在 AnswerRound.Evaluation 字段里供审计
type Critic struct {
	// Critic 给原评估打的"靠谱度"分数 0-100
	// 阈值低于 60 触发 refine
	GroundedScore int `json:"grounded_score"`
	// 是否需要让 evaluate 重做一次
	NeedRefine bool `json:"need_refine"`
	// critic 指出的具体问题，供 refine 时塞回 prompt
	Issues []string `json:"issues,omitempty"`
	// critic 的整体评价（一句话）
	Summary string `json:"summary"`

	// 探问信号(probe_signal):
	// critic 节点合并了"评估反思"和"是否值得追问"两个判断,
	// 共用 evaluation + answer 的上下文,一次 LLM 调用同时输出两个信号。
	// 设计动机:probe_decider 单独建节点的话,prompt 几乎和 critic 一模一样,
	//          省一次 LLM 调用比"职责更纯"的工程洁癖更划算。
	HasProbeSignal bool   `json:"has_probe_signal"`
	// 候选追问主题(LLM 提示的"答案中值得深挖的点"),
	// probe_ask 节点把它作为 hint 生成具体的追问问题。
	ProbeTopic string `json:"probe_topic,omitempty"`
}

// WorkingMemory 是 Agent 决策的运行时状态。
//
// 命名为 "Working Memory" 而非 "Context" / "State"：参考认知心理学的
// 短期工作记忆概念，正是 Agent loop 每轮读 + 写的内容。
//
// 几个字段的来源：
//   - ConfirmedSkills: evaluate.Score >= 70 的题对应技能进这里
//   - WeakSkills:      evaluate.Score < 50 的题对应技能进这里
//   - SuspectedSkills: parse_resume 抽取出来但还没被任何题验证的技能
//   - SkillCoverage:   每个技能的累计覆盖度（normalized score 累加,float），
//                      用于"覆盖率均衡"决策——pick_next 优先选 coverage 低的类目
//   - AvgScore:        所有 FinalEvaluation 的均分（动态难度调整用）
type WorkingMemory struct {
	ConfirmedSkills []string           `json:"confirmed_skills"`
	WeakSkills      []string           `json:"weak_skills"`
	SuspectedSkills []string           `json:"suspected_skills"`
	SkillCoverage   map[string]float64 `json:"skill_coverage"`
	AvgScore        float64        `json:"avg_score"`
	RoundsAsked     int            `json:"rounds_asked"`
	MaxRounds       int            `json:"max_rounds"` // 默认 8
	ScoredRounds    int            `json:"scored_rounds"`
	DegradedRounds  int            `json:"degraded_rounds"`
	DegradedReasons map[string]string `json:"degraded_reasons,omitempty"`

	// 追问预算（防止 LLM 无限追问）
	ProbesUsed int `json:"probes_used"`
	MaxProbes  int `json:"max_probes"` // 默认 4

	// 反思补漏预算（防止 LLM 无限补题）
	ReflectionsUsed int `json:"reflections_used"`
	MaxReflections  int `json:"max_reflections"` // 默认 1
	ReflectTopic    string `json:"reflect_topic,omitempty"`

	// Notes 是通用元数据袋,存非核心、非降级类的状态标记,如 fallback_used、
	// cost_capped 等。SSE 层 / 报告页可读这里给前端透出。
	// 主决策路径不依赖 Notes —— 它只是辅助信号。
	Notes map[string]string `json:"notes,omitempty"`
}

// NewWorkingMemory 用默认上限构造一个空记忆。
func NewWorkingMemory() *WorkingMemory {
	return &WorkingMemory{
		ConfirmedSkills: []string{},
		WeakSkills:      []string{},
		SuspectedSkills: []string{},
		SkillCoverage:   map[string]float64{},
		MaxRounds:       8,
		MaxProbes:       4,
		MaxReflections:  1,
	}
}

// RemainingRounds 剩余可问主题题数。Agent 循环退出条件之一。
func (m *WorkingMemory) RemainingRounds() int {
	r := m.MaxRounds - m.RoundsAsked
	if r < 0 {
		return 0
	}
	return r
}

// CanProbe 是否还能追问。
func (m *WorkingMemory) CanProbe() bool { return m.ProbesUsed < m.MaxProbes }

// CanReflect 是否还能反思补漏。
func (m *WorkingMemory) CanReflect() bool { return m.ReflectionsUsed < m.MaxReflections }

// Action 是 Agent 决策节点输出的动作类型。
type Action string

const (
	ActionAskNew  Action = "ask_new"  // 出新题
	ActionProbe   Action = "probe"    // 追问当前题
	ActionRefine  Action = "refine"   // critic 触发重评
	ActionReflect Action = "reflect"  // 反思补漏
	ActionEnd     Action = "end"      // 结束面试，进 report
)

// Validate 状态合法性检查。
func (a Action) Validate() error {
	switch a {
	case ActionAskNew, ActionProbe, ActionRefine, ActionReflect, ActionEnd:
		return nil
	default:
		return fmt.Errorf("invalid action: %q", a)
	}
}

// Decision 是 LLM 决策节点的统一输出。
//
// pick_next_question / probe_decider / reflection_check 三个节点都返回 Decision。
// 用统一类型让 Graph 的条件分支边可以泛化处理。
type Decision struct {
	Action Action `json:"action"`
	// LLM 解释为什么做这个决策（可解释性）
	Reasoning string `json:"reasoning"`
	// 不同 Action 用不同字段：
	NextQuestionID string `json:"next_question_id,omitempty"` // ask_new
	ProbeQuestion  string `json:"probe_question,omitempty"`   // probe
	ProbeReason    string `json:"probe_reason,omitempty"`     // probe
	ReflectTopic   string `json:"reflect_topic,omitempty"`    // reflect
	DecidedAt      time.Time `json:"decided_at"`
}
