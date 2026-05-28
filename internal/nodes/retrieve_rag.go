package nodes

import (
	"context"
	"fmt"
	"strings"

	"interview-agent/internal/domain"
	"interview-agent/internal/embedding"
	"interview-agent/internal/graph"
	"interview-agent/internal/retriever"
)

// retrieve_rag 节点设计：
//
//   1. Query 构造：missing_skills 优先 + must_have 兜底
//      - 优先 missing：候选人不擅长的地方更需要"题库覆盖"
//      - 没 missing 时用 must_have：高匹配场景下让验证题贴 JD 硬要求
//      - 还是空就用 key_skills 全集
//
//   2. Query Embedding 复用 Embedder 接口：
//      把 query 文本一次性 Embed,拿到 1024 维向量,塞进 Retriever.Query。
//      Mock 模式下 MockEmbedder 用 FNV hash 出确定性向量,CI 可重现。
//
//   3. 降级兜底：
//      Retriever / Embedder 出错时,不让整个会话挂掉——
//      用静态 fallback 题表（3-5 道通用题）兜底,并打 sess.WorkingMemory 标记
//      （后续 SSE 可以提示前端"题库降级中,体验受限"）。
//
//   4. K 默认 20：
//      pick_next 每轮从候选池挑 1 题,20 题足够支撑 10 轮深度对话。
//      多了会让 LLM 选题时上下文太长。

// fallbackQuestions 是 RAG 全链路失败时的"保底题库"。
//
// 选这 5 题的标准：
//   - 覆盖 Go / Redis / SD 三大类
//   - 难度中位（3）
//   - 经典且开放，可以拓展深度提问
//
// 这是临时降级方案,不应频繁触发,触发了运维要查 Embedder/PG 状态。
var fallbackQuestions = []domain.Question{
	{ID: "fallback-go-001", Content: "讲一下 Go 的 GMP 调度模型，G/M/P 各自的职责。",
		Tags: []string{"go_concurrency"}, Difficulty: 3, Source: "fallback", SkillCategory: "go",
		ExpectedPoints: []string{"G/M/P 三者定义", "P 的作用", "本地队列与全局队列", "work stealing"}},
	{ID: "fallback-go-002", Content: "Go channel 的底层数据结构是怎样的？",
		Tags: []string{"channel", "go_concurrency"}, Difficulty: 3, Source: "fallback", SkillCategory: "go",
		ExpectedPoints: []string{"hchan 结构", "sendq/recvq", "有缓冲与无缓冲路径", "goroutine 唤醒时机"}},
	{ID: "fallback-redis-001", Content: "Redis 的 AOF 和 RDB 持久化方式各自的取舍？",
		Tags: []string{"aof", "rdb", "redis_persistence"}, Difficulty: 3, Source: "fallback", SkillCategory: "redis",
		ExpectedPoints: []string{"AOF 追加命令", "RDB 快照", "恢复速度与数据丢失窗口", "aof rewrite"}},
	{ID: "fallback-sd-001", Content: "设计一个秒杀系统，谈一下库存扣减和防超卖。",
		Tags: []string{"system_design", "distributed_lock"}, Difficulty: 4, Source: "fallback", SkillCategory: "system-design",
		ExpectedPoints: []string{"Redis lua 原子扣减", "MQ 异步落库", "限流", "防超卖一致性"}},
	{ID: "fallback-sd-002", Content: "线上 P99 延迟抖动，给一个排查思路。",
		Tags: []string{"system_design", "performance"}, Difficulty: 4, Source: "fallback", SkillCategory: "system-design",
		ExpectedPoints: []string{"GC/调度", "下游慢调用", "锁竞争", "网络重传", "热点资源"}},
}

// RetrieveRAGOptions 暴露给图组装阶段的可调参数。
// 字段顺序按"调谐频率"排——TopK / TargetDifficulty 经常调，K candidates 一般不动。
type RetrieveRAGOptions struct {
	TopK             int // 候选池大小，默认 20
	TargetDifficulty int // 目标难度，默认 3，可被 GapStrategy 上调/下调
	VectorCandidates int // SQL 召回 vector 候选数，默认 K*5
	TagCandidates    int // SQL 召回 tag 候选数，默认 K*3
}

// NewRetrieveRAGNode 构造 retrieve_rag 节点。
//
// 节点契约：
//
//	输入：sess.JobProfile / sess.GapReport（必须）
//	输出：sess.CandidatePool（[]domain.Question，长度 ≤ TopK）
//
// 失败语义：
//   - JobProfile / GapReport 缺失 → ErrPermanent
//   - Embedder 失败 → 降级到 fallbackQuestions，会话继续
//   - Retriever 失败 → 同上
//
// 降级标记：WorkingMemory.DegradedReasons 里追加 rag 原因，
// SSE 层可读这个标记给前端透出 "题库降级" 状态。
func NewRetrieveRAGNode(
	embedder embedding.Embedder,
	r retriever.Retriever,
	opts RetrieveRAGOptions,
) graph.NodeFunc {
	if opts.TopK <= 0 {
		opts.TopK = 20
	}
	if opts.TargetDifficulty < 1 || opts.TargetDifficulty > 5 {
		opts.TargetDifficulty = 3
	}

	return func(ctx context.Context, sess *domain.Session) error {
		if sess.JobProfile == nil {
			return fmt.Errorf("retrieve_rag: job_profile required: %w", graph.ErrPermanent)
		}
		if sess.GapReport == nil {
			return fmt.Errorf("retrieve_rag: gap_report required: %w", graph.ErrPermanent)
		}

		queryTags := buildQueryTags(sess.GapReport, sess.JobProfile)
		queryText := buildQueryText(queryTags, sess.JobProfile.Title)
		targetDiff := tuneDifficulty(opts.TargetDifficulty, sess.GapReport.Strategy)

		// 1. Embed query
		vectors, err := embedder.Embed(ctx, []string{queryText})
		if err != nil || len(vectors) != 1 || len(vectors[0]) == 0 {
			markDegraded(sess, fmt.Sprintf("embed failed: %v", err))
			sess.CandidatePool = cloneFallback(targetDiff, sess.QuestionBankFilter)
			return nil
		}

		// 2. Retrieve
		results, err := r.Retrieve(ctx, retriever.Query{
			QueryEmbedding:   vectors[0],
			Tags:             queryTags,
			Difficulty:       targetDiff,
			K:                opts.TopK,
			SkillCategories:  filterSkillCategories(sess.QuestionBankFilter),
			Scenarios:        filterScenarios(sess.QuestionBankFilter),
			DifficultyMin:    filterDifficultyMin(sess.QuestionBankFilter),
			DifficultyMax:    filterDifficultyMax(sess.QuestionBankFilter),
			FilterTags:       filterTags(sess.QuestionBankFilter),
			VectorCandidates: opts.VectorCandidates,
			TagCandidates:    opts.TagCandidates,
		})
		if err != nil {
			markDegraded(sess, fmt.Sprintf("retrieve failed: %v", err))
			sess.CandidatePool = cloneFallback(targetDiff, sess.QuestionBankFilter)
			return nil
		}
		if len(results) == 0 {
			markDegraded(sess, "retrieve returned 0 results")
			sess.CandidatePool = cloneFallback(targetDiff, sess.QuestionBankFilter)
			return nil
		}

		// 3. 写候选池
		pool := make([]domain.Question, 0, len(results))
		for _, res := range results {
			pool = append(pool, domain.Question{
				ID:             res.ID,
				Content:        res.Content,
				Tags:           res.Tags,
				Difficulty:     res.Difficulty,
				Source:         "rag-" + res.ID,
				SkillCategory:  res.Category,
				ExpectedPoints: append([]string(nil), res.ExpectedPoints...),
			})
		}
		sess.CandidatePool = pool
		return nil
	}
}

func filterSkillCategories(filter *domain.QuestionBankFilter) []string {
	if filter == nil {
		return nil
	}
	return append([]string(nil), filter.SkillCategories...)
}

func filterScenarios(filter *domain.QuestionBankFilter) []string {
	if filter == nil {
		return nil
	}
	return append([]string(nil), filter.Scenarios...)
}

func filterDifficultyMin(filter *domain.QuestionBankFilter) int {
	if filter == nil {
		return 0
	}
	return filter.DifficultyMin
}

func filterDifficultyMax(filter *domain.QuestionBankFilter) int {
	if filter == nil {
		return 0
	}
	return filter.DifficultyMax
}

func filterTags(filter *domain.QuestionBankFilter) []string {
	if filter == nil {
		return nil
	}
	return append([]string(nil), filter.Tags...)
}

// buildQueryTags 决定本次 RAG 的目标标签集。
//
// 优先级：missing_skills > must_have > key_skills。
// 全空时返回 nil，让 Retriever 走纯 vector 路径（不会因 tags && '{}' 永远 false 而漏召回）。
func buildQueryTags(gap *domain.GapReport, job *domain.JobProfile) []string {
	if len(gap.MissingSkills) > 0 {
		return gap.MissingSkills
	}
	if len(job.MustHave) > 0 {
		return job.MustHave
	}
	if len(job.KeySkills) > 0 {
		return job.KeySkills
	}
	return nil
}

// buildQueryText 把 tag 列表拼成自然语言 query 给 Embedder。
// 加 "面试题" 这个语境锚点,让 embedding 偏向题库题的语义空间,而不是文档/教程。
func buildQueryText(tags []string, jobTitle string) string {
	var sb strings.Builder
	if jobTitle != "" {
		sb.WriteString(jobTitle)
		sb.WriteString(" 岗位面试题，重点考察：")
	} else {
		sb.WriteString("后端工程师面试题，重点考察：")
	}
	if len(tags) == 0 {
		sb.WriteString("综合能力")
	} else {
		sb.WriteString(strings.Join(tags, "、"))
	}
	return sb.String()
}

// tuneDifficulty 根据 strategy 微调目标难度：
//   - validate（强匹配）→ +1（验证深度）
//   - cover_gap（弱匹配）→ -1（先打基础）
//   - explore（中匹配） → 不变
//
// 边界自动 clamp 到 [1, 5]。
func tuneDifficulty(base int, s domain.GapStrategy) int {
	switch s {
	case domain.GapStrategyValidate:
		base++
	case domain.GapStrategyCoverGap:
		base--
	}
	if base < 1 {
		return 1
	}
	if base > 5 {
		return 5
	}
	return base
}

// markDegraded 在 WorkingMemory 上打降级标记,供 SSE / 报告页透出。
func markDegraded(sess *domain.Session, reason string) {
	if sess.WorkingMemory == nil {
		sess.WorkingMemory = &domain.WorkingMemory{}
	}
	markDegradedReason(sess.WorkingMemory, "rag", reason)
}

// cloneFallback 按目标难度筛选 fallback 题，避免难度爆炸式偏离。
// 难度差 ≤ 2 的题入选；都不满足时直接返回全集。
func cloneFallback(targetDiff int, filter *domain.QuestionBankFilter) []domain.Question {
	out := make([]domain.Question, 0, len(fallbackQuestions))
	for _, q := range fallbackQuestions {
		if abs(q.Difficulty-targetDiff) <= 2 && matchesFallbackFilter(q, filter) {
			out = append(out, q)
		}
	}
	if len(out) == 0 {
		for _, q := range fallbackQuestions {
			if abs(q.Difficulty-targetDiff) <= 2 {
				out = append(out, q)
			}
		}
	}
	if len(out) == 0 {
		out = append(out, fallbackQuestions...)
	}
	return out
}

func matchesFallbackFilter(q domain.Question, filter *domain.QuestionBankFilter) bool {
	if filter == nil {
		return true
	}
	if len(filter.SkillCategories) > 0 && !containsAnyString([]string{q.SkillCategory}, filter.SkillCategories) {
		return false
	}
	if filter.DifficultyMin > 0 && q.Difficulty < filter.DifficultyMin {
		return false
	}
	if filter.DifficultyMax > 0 && q.Difficulty > filter.DifficultyMax {
		return false
	}
	if len(filter.Tags) > 0 && !containsAnyString(q.Tags, filter.Tags) {
		return false
	}
	return true
}

func containsAnyString(items, targets []string) bool {
	for _, item := range items {
		for _, target := range targets {
			if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(target)) {
				return true
			}
		}
	}
	return false
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
