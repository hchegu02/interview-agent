// Package retriever 是题库的检索抽象。
//
// 设计目标：
//  1. 接口与实现解耦：Retriever 是接口，pgvector 是当前实现，
//     未来 ES / Milvus / 内存版都能插
//  2. 召回与排序解耦：SQL 只做候选集召回（HNSW 走索引 + GIN 走索引），
//     最终打分走 Fusion 接口（先用 LinearFusion，未来可换 RRF / cross-encoder）
//  3. 特征显式：Result 里保留 vector_score / tag_score / difficulty_score 三路
//     原始分，便于调权 / 日志诊断 / A-B 实验
//
// 关键设计取舍参见 docs/design.md 第 4 节"RAG 检索"。
package retriever

import "context"

// Query 是一次检索的输入。
//
// QueryEmbedding 由调用方提前算好——retriever 不依赖 embedder，
// 保持单一职责（embedder 不稳定时不会拖垮检索路径）。
type Query struct {
	Text           string    // 原始查询文本；PG retriever 当前忽略，离线 seed eval 可用作确定性文本排序信号
	QueryEmbedding []float32 // 必填；维度必须与 question_bank.embedding 一致
	Tags           []string  // 用户/上游节点提供的标签；会先经过 alias 归一化
	Difficulty     int       // 目标难度 1-5；用作软过滤 ±2 和 difficulty_score
	K              int       // 期望返回 top-K（默认 5）

	// 下面这些字段是用户选择的题库范围，作为硬过滤条件进入 SQL。
	// 空值表示不限制；Tags 仍用于主题打分，FilterTags 只用于候选集约束。
	SkillCategories []string
	Scenarios       []string
	DifficultyMin   int
	DifficultyMax   int
	FilterTags      []string
	ExcludeIDs      []string

	// VectorCandidates / TagCandidates 控制两路召回宽度，调权时偶尔会改。
	// 0 时走默认：VectorCandidates = K * 5, TagCandidates = K * 3
	VectorCandidates int
	TagCandidates    int
	TextCandidates   int
}

// Result 是融合打分后的最终候选。
// 三个特征分都保留下来，便于 Stage 4 的日志 / Stage 5 的前端展示。
type Result struct {
	ID             string
	Content        string
	Tags           []string
	Difficulty     int
	Category       string // skill_category
	ExpectedPoints []string

	Score           float64 // 融合后的最终分（0~1）
	VectorScore     float64 // 1 - cosine_distance，0~1
	TagScore        float64 // intersection_size / query_tag_count，0~1
	DifficultyScore float64 // 1 - |diff - target| / 2，0~1
}

// Candidate 是 SQL 召回阶段的原始特征——Fusion 的输入。
// 故意不直接复用 Result：候选阶段还没"融合分"，留个 Score 字段空着会误导。
type Candidate struct {
	ID             string
	Content        string
	Tags           []string
	Difficulty     int
	Category       string
	ExpectedPoints []string

	VecDist       float64 // pgvector 的 cosine distance，越小越近
	TagOverlap    int     // 候选 tags 与 query.Tags（归一化后）的交集大小
	QueryTagCount int     // 用于 tag_score 归一化的分母
	TargetDiff    int     // 用于 difficulty_score 计算
}

// Retriever 是题库检索接口。
//
// 上层 Agent 节点（retrieve_rag / pick_next）只依赖这个接口；
// 测试时用 InMemoryRetriever，生产用 PGVectorRetriever。
type Retriever interface {
	Retrieve(ctx context.Context, q Query) ([]Result, error)
}

const (
	// StageVector 标识向量召回阶段。
	StageVector = "vector"
	// StageBM25 标识 BM25 / 文本召回阶段。
	StageBM25 = "bm25"
	// StageRule 标识规则补召回阶段。
	StageRule = "rule"
	// StageRRF 标识多路召回融合阶段。
	StageRRF = "rrf"
	// StageRerank 标识重排阶段。
	StageRerank = "rerank"
)

// StageResult 是单个检索阶段产出的候选及其阶段证据。
type StageResult struct {
	Result
	Stage      string
	Rank       int
	StageScore float64
	Reason     string
}

// StageTrace 是一个检索阶段的诊断信息。
type StageTrace struct {
	Stage      string        `json:"stage"`
	Count      int           `json:"count"`
	DurationMS float64       `json:"duration_ms"`
	Items      []ResultTrace `json:"items,omitempty"`
	Error      string        `json:"error,omitempty"`
}

// ResultTrace 是单个结果在 trace 中的可序列化证据。
type ResultTrace struct {
	ID      string             `json:"id"`
	Rank    int                `json:"rank"`
	Score   float64            `json:"score"`
	Stage   string             `json:"stage,omitempty"`
	Reason  string             `json:"reason,omitempty"`
	Sources map[string]float64 `json:"sources,omitempty"`
}

// RetrievalTrace 记录一次检索请求的阶段路径和最终结果。
type RetrievalTrace struct {
	Query                string        `json:"query"`
	OriginalQuery        string        `json:"original_query,omitempty"`
	RewrittenQuery       string        `json:"rewritten_query,omitempty"`
	QueryRewriteReason   string        `json:"query_rewrite_reason,omitempty"`
	QueryRewriteFallback string        `json:"query_rewrite_fallback,omitempty"`
	HyDEMode             string        `json:"hyde_mode,omitempty"`
	HyDEStatus           string        `json:"hyde_status,omitempty"`
	HyDEFallback         string        `json:"hyde_fallback,omitempty"`
	HyDETextHash         string        `json:"hyde_text_hash,omitempty"`
	Stages               []StageTrace  `json:"stages"`
	Final                []ResultTrace `json:"final"`
	FallbackReasons      []string      `json:"fallback_reasons,omitempty"`
}
