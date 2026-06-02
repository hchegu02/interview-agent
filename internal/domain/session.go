// Package domain 定义面试场景的核心领域模型。
//
// 设计原则：
//   - Session 是聚合根，包含一次面试的所有状态
//   - 所有结构体可直接 JSON 序列化（Redis 快照、HTTP 响应、PG state_json 都用同一套）
//   - 子结构自包含、不互相引用，方便单测构造
//   - ID 一律用 ULID（时间有序 + 全局唯一 + 26 字符短，比 UUID 友好）
//
// 文件分布：
//   - session.go: 会话聚合根 + 静态画像类型
//   - agent.go:   Agent 运行时类型（AnswerRound / WorkingMemory / Decision / Critic）
package domain

import (
	"fmt"
	"time"
)

// SessionStatus 是面试会话的状态机。
type SessionStatus string

const (
	StatusCreated   SessionStatus = "created"   // 已创建，尚未执行
	StatusRunning   SessionStatus = "running"   // Graph 执行中
	StatusPaused    SessionStatus = "paused"    // SSE 断开等用户回来
	StatusCompleted SessionStatus = "completed" // 报告已生成
	StatusFailed    SessionStatus = "failed"    // 不可恢复错误
)

// Validate 检查状态是否合法。
func (s SessionStatus) Validate() error {
	switch s {
	case StatusCreated, StatusRunning, StatusPaused, StatusCompleted, StatusFailed:
		return nil
	default:
		return fmt.Errorf("invalid session status: %q", s)
	}
}

// Session 是一次面试会话的聚合根。
//
// Agent 化改造（vs 老版）：
//   - 删掉 Answers / Evaluations 两个 map，改为 Rounds []AnswerRound
//     一题一个 round，含追问 / critic / refine 的完整记录
//   - 加 WorkingMemory：Agent 决策依赖的运行时记忆
//   - 加 PendingDecision：pick_next 决策后等用户答题时挂在这里
//   - 加 CandidatePool：retrieve_rag 召回的题库候选（Agent 从中选）
type Session struct {
	ID          string        `json:"id"`             // ULID
	UserID      string        `json:"user_id"`        // 这个 session 属于哪个用户
	Mode        string        `json:"mode,omitempty"` // exam | practice，前端业务模式
	Status      SessionStatus `json:"status"`         // session 当前状态，控制流程走向
	CurrentNode string        `json:"current_node"`   // graph 节点名，断点恢复用

	// JobProfile/CandProfile 是 setup 阶段的输入画像。
	// 后续节点不要再直接解析原文；需要判断技能、年限、项目时应读这些结构化字段。
	JobProfile  *JobProfile       `json:"job_profile,omitempty"`       // job_profile 节点产物，LLM 从 JD 文本抽取的岗位画像
	CandProfile *CandidateProfile `json:"candidate_profile,omitempty"` // candidate_profile 节点产物，LLM 从简历文本抽取的候选人画像

	// gap_analyze 节点产物，agent loop 用 Strategy 调整提问倾向
	GapReport *GapReport `json:"gap_report,omitempty"`

	// analyze_profile 节点产物，给前端和报告解释 JD/简历匹配情况
	ProfileAnalysis *ProfileAnalysis `json:"profile_analysis,omitempty"`

	// retrieve_rag 召回的候选题池，pick_next_question 从这里选
	// 不直接给候选人——Agent 决策驱动出题顺序
	CandidatePool []Question `json:"candidate_pool,omitempty"`

	// RetrievalTrace 保存最近一次 retrieve_rag 的检索证据。
	// 它是面试会话事实的一部分，供报告、前端解释和排障使用。
	RetrievalTrace *RetrievalTrace `json:"retrieval_trace,omitempty"`

	// QuestionBankFilter 是用户在准备阶段选择的题库范围。
	// retrieve_rag 会把它转换成 Retriever 的硬过滤条件。
	QuestionBankFilter *QuestionBankFilter `json:"question_bank_filter,omitempty"`

	// Agent 运行时记忆，每轮读写
	WorkingMemory *WorkingMemory `json:"working_memory,omitempty"`

	// 答题轮记录，时间顺序追加
	Rounds []AnswerRound `json:"rounds,omitempty"`

	// pick_next/probe_decider 决策后等用户答题时挂在这里；
	// 用户答完后 answer_question 节点消费这个字段、清空、追加到 Rounds
	PendingDecision *Decision `json:"pending_decision,omitempty"`

	Report *Report `json:"report,omitempty"`

	// CreatedAt/UpdatedAt 不只是展示字段。列表接口按 UpdatedAt 倒序，
	// PG/Redis 恢复后也靠它判断用户最近一次操作。
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CurrentRound 返回最近一个未完成的 Round，没有就返回 nil。
// Agent 在 probe / refine 时需要操作"当前题"。
func (s *Session) CurrentRound() *AnswerRound {
	if len(s.Rounds) == 0 {
		return nil
	}
	last := &s.Rounds[len(s.Rounds)-1]
	if last.CompletedAt.IsZero() {
		return last
	}
	return nil
}

// JobProfile 是岗位画像，由 LLM 从 JD 文本中抽取。
type JobProfile struct {
	Title         string   `json:"title"`
	Level         string   `json:"level"`          // intern | junior | senior | staff
	KeySkills     []string `json:"key_skills"`     // 必备技能
	MustHave      []string `json:"must_have"`      // 硬性门槛，必须满足；与 KeySkills 区分用于 gap 强度
	NiceToHave    []string `json:"nice_to_have"`   // 加分项
	YearsRequired int      `json:"years_required"` // JD 中提到的最低年限
	JDRawText     string   `json:"jd_raw_text"`    // 原始 JD，用于 trace
}

// CandidateProfile 是候选人画像，由 LLM 从简历文本中抽取。
type CandidateProfile struct {
	Years         int             `json:"years"`
	Skills        []string        `json:"skills"`
	WeakSkills    []string        `json:"weak_skills"` // 与 JobProfile.KeySkills 取差集
	Projects      []ResumeProject `json:"projects"`    // 简历项目经历
	Highlights    []string        `json:"highlights"`  // 可被 dynamic probing 的"亮点"短语
	ResumeRawText string          `json:"resume_raw_text"`
}

// RetrievalTrace 是一次 RAG 检索的可序列化审计信息。
// 结构刻意镜像 retriever.RetrievalTrace，但放在 domain 包避免领域模型依赖检索实现包。
type RetrievalTrace struct {
	Query           string                 `json:"query"`
	Stages          []RetrievalStageTrace  `json:"stages,omitempty"`
	Final           []RetrievalResultTrace `json:"final,omitempty"`
	FallbackReasons []string               `json:"fallback_reasons,omitempty"`
}

// RetrievalStageTrace 是单个检索阶段的诊断信息。
type RetrievalStageTrace struct {
	Stage      string                 `json:"stage"`
	Count      int                    `json:"count"`
	DurationMS float64                `json:"duration_ms"`
	Items      []RetrievalResultTrace `json:"items,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

// RetrievalResultTrace 是单个候选在 trace 中的排序证据。
type RetrievalResultTrace struct {
	ID      string             `json:"id"`
	Rank    int                `json:"rank"`
	Score   float64            `json:"score"`
	Stage   string             `json:"stage,omitempty"`
	Reason  string             `json:"reason,omitempty"`
	Sources map[string]float64 `json:"sources,omitempty"`
}

// ResumeProject 是简历里一段项目经历，给 dynamic probing 节点用。
// 字段尽量薄——只保留 probe 时需要的"项目名 + 角色 + 亮点 + 技术栈"四要素。
type ResumeProject struct {
	Name       string   `json:"name"`
	Role       string   `json:"role,omitempty"`
	Highlights []string `json:"highlights,omitempty"`
	Stack      []string `json:"stack,omitempty"`
}

// ProfileAnalysis 是 JD 与简历的可解释匹配分析。
//
// 它不是出题策略本身；出题仍由 GapReport + RAG + Agent memory 决定。
// 这个结构服务两个实际场景：
//  1. 前端告诉用户为什么系统会这样问；
//  2. 简历优化场景指出缺口、风险点和可追问证据。
type ProfileAnalysis struct {
	MatchScore          int                `json:"match_score"`                    // 0-100
	Summary             string             `json:"summary"`                        // LLM 给的简短总结，可用于报告页透出
	YearsGap            int                `json:"years_gap"`                      // candidate years - required years
	MatchedRequirements []string           `json:"matched_requirements,omitempty"` // 与 JobProfile.KeySkills 交集，说明 JD 里哪些技能在简历里有体现
	MissingRequirements []string           `json:"missing_requirements,omitempty"`
	Strengths           []string           `json:"strengths,omitempty"`
	RiskPoints          []string           `json:"risk_points,omitempty"`
	ResumeSuggestions   []string           `json:"resume_suggestions,omitempty"`
	QuestionFocus       []string           `json:"question_focus,omitempty"`
	ProjectProbePlan    []ProjectProbePlan `json:"project_probe_plan,omitempty"`
}

// ProjectProbePlan 描述某个简历项目最值得追问的证据点。
type ProjectProbePlan struct {
	ProjectName       string `json:"project_name"`
	Focus             string `json:"focus"`
	Evidence          string `json:"evidence,omitempty"`
	SuggestedQuestion string `json:"suggested_question"`
}

// GapStrategy 是 gap_analyze 给后续 agent loop 的"提问倾向"决策。
//
//   - validate:   候选人技能高度匹配 → 深度题为主，验证简历真实性
//   - explore:   中等匹配 → 优先 probe 重叠技能、追问项目细节
//   - cover_gap: 低匹配 → 用题库题覆盖 missing skills，定位水平
type GapStrategy string

// 三种提问策略，agent loop 根据 GapReport.Strategy 调整提问倾向。

const (
	GapStrategyValidate GapStrategy = "validate"
	GapStrategyExplore  GapStrategy = "explore"
	GapStrategyCoverGap GapStrategy = "cover_gap"
)

// Validate 检查策略是否合法。LLM 输出兜底用。
func (s GapStrategy) Validate() error {
	switch s {
	case GapStrategyValidate, GapStrategyExplore, GapStrategyCoverGap:
		return nil
	default:
		return fmt.Errorf("invalid gap strategy: %q", s)
	}
}

// GapReport 是 gap_analyze 节点的产物。
//
// MatchedSkills / MissingSkills 用规则法算（CanonicalizeTags 归一化后求交/差集），
// 可解释、可单测；OverlapScore = matched / |jd.key_skills|，区间 [0,1]。
// Strategy 在 OverlapScore 边界附近用 LLM 兜底——边界场景规则不灵，让 LLM 看上下文。
type GapReport struct {
	MatchedSkills []string    `json:"matched_skills"`
	MissingSkills []string    `json:"missing_skills"`
	OverlapScore  float64     `json:"overlap_score"` // 0~1
	Strategy      GapStrategy `json:"strategy"`
	Reason        string      `json:"reason,omitempty"` // LLM 给的简短解释，可用于报告页透出
}

// QuestionBankFilter 描述一次面试允许使用的题库范围。
// 空字段表示不限制；多个值表示命中任意一个即可。
type QuestionBankFilter struct {
	SkillCategories []string `json:"skill_categories,omitempty"`
	Scenarios       []string `json:"scenarios,omitempty"`
	DifficultyMin   int      `json:"difficulty_min,omitempty"`
	DifficultyMax   int      `json:"difficulty_max,omitempty"`
	Tags            []string `json:"tags,omitempty"`
}

// Question 是一道面试题。
// Source 标识来源："rag-<id>" 表示题库种子，"llm-generated" 表示 LLM 改编。
type Question struct {
	ID         string   `json:"id"` // ULID 或题库 ID
	Content    string   `json:"content"`
	Tags       []string `json:"tags"`
	Difficulty int      `json:"difficulty"` // 1-5
	Source     string   `json:"source"`
	// 这道题主要考察哪个 skill 类目（用于 SkillCoverage 累计）
	SkillCategory string `json:"skill_category,omitempty"`
	// 期望要点:由题库 seed 给出的"答到这些点算掌握"的参考清单。
	// evaluate 节点会把这些点塞进 prompt,让 LLM 给分时有锚点而不是凭印象。
	// 可空(降级 fallback 题没有);为空时 evaluate 退化到"只看题+答案"。
	ExpectedPoints []string `json:"expected_points,omitempty"`
}

// Evaluation 是单题评估结果。
type Evaluation struct {
	QuestionID string   `json:"question_id"`
	Score      int      `json:"score"` // 0-100
	Strengths  []string `json:"strengths"`
	Weaknesses []string `json:"weaknesses"`
	Suggestion string   `json:"suggestion"`
}

// Report 是终评报告。
type Report struct {
	SessionID          string              `json:"session_id"`
	OverallScore       int                 `json:"overall_score"`
	SkillBreakdown     map[string]int      `json:"skill_breakdown"` // skill -> 0..100
	TranscriptAnalysis *TranscriptAnalysis `json:"transcript_analysis,omitempty"`
	DrillPlan          []DrillPlanItem     `json:"drill_plan,omitempty"`
	Highlights         []string            `json:"highlights"`
	Improvements       []string            `json:"improvements"`
	NextSteps          []string            `json:"next_steps"`
}

// TranscriptAnalysis 是对整场问答文本的可解释诊断。
type TranscriptAnalysis struct {
	RoundsAnalyzed     int                   `json:"rounds_analyzed"`
	AverageAnswerChars int                   `json:"average_answer_chars"`
	Dimensions         []TranscriptDimension `json:"dimensions"`
	Patterns           []string              `json:"patterns,omitempty"`
}

// TranscriptDimension 是一项回答质量维度。
type TranscriptDimension struct {
	Name     string   `json:"name"`
	Score    int      `json:"score"` // 0-100
	Evidence []string `json:"evidence,omitempty"`
	Advice   string   `json:"advice,omitempty"`
}

// DrillPlanItem 是报告给出的下一轮训练计划。
type DrillPlanItem struct {
	PracticeOrder          int      `json:"practice_order"`
	Skill                  string   `json:"skill"`
	Reason                 string   `json:"reason"`
	TargetScore            int      `json:"target_score"`
	RecommendedQuestionIDs []string `json:"recommended_question_ids,omitempty"`
	RecommendedQuestions   []string `json:"recommended_questions,omitempty"`
}

// 关于“为什么不把流程控制全交给状态机”：
//
// 这里已经在用 graph 作为持久化流程状态机：
//   - CurrentNode 是断点程序计数器，Run/Resume 依赖它恢复执行；
//   - AddEdge/AddBranch/Router 定义节点流转；
//   - ErrSuspended 表示等待外部输入，而不是业务失败。
//
// SessionStatus 只保留 created/running/paused/completed/failed 这种粗粒度生命周期。
// 题目选择、追问、反思、结束等细粒度决策必须落在 PendingDecision、Rounds、
// WorkingMemory 这些领域数据里。否则会把“流程位置”和“业务事实”混成一个巨大的
// enum，最后状态爆炸，恢复、测试、报告追溯都会更难。
//
// 节点内部如果继续变复杂，优先拆纯函数/小 router/独立领域方法；不要再新增一套
// 与 graph 并行的状态机。
