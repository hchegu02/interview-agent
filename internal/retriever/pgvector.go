package retriever

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGVectorRetriever 是基于 pgvector 的 Retriever 实现。
//
// 两阶段检索（route D，docs/design.md 第 4.3 节）：
//
//	stage 1（PG 端）：召回候选集
//	  - vector_candidates：HNSW 索引 + 距离算子 ORDER BY，难度软过滤 ±2
//	  - tag_candidates：GIN 索引扫 tags && query_tags
//	  - UNION 去重，返回原始特征（vec_dist / tag_overlap）
//
//	stage 2（Go 端）：Fusion 打分
//	  - LinearFusion 加权三路特征
//	  - 排序取 top-K
//
// 这种切法 PG 只走两条索引扫描（HNSW + GIN），不做表达式排序——避免破坏 HNSW
// 索引利用率。打分逻辑在 Go 里，权重可热调，A-B 实验时无需改 SQL。
type PGVectorRetriever struct {
	Pool   *pgxpool.Pool
	Fusion Fusion

	// HNSW 检索精度参数。生产建议 100~200；越大召回越全但越慢。
	// SQL 执行前会 SET LOCAL hnsw.ef_search = N
	EfSearch int
}

// NewPGVectorRetriever 构造。Pool 必须已经连通。
// fusion 传 nil 时用 LinearFusion 默认权重。
func NewPGVectorRetriever(pool *pgxpool.Pool, fusion Fusion) *PGVectorRetriever {
	if fusion == nil {
		fusion = NewLinearFusion(0, 0, 0)
	}
	return &PGVectorRetriever{
		Pool:     pool,
		Fusion:   fusion,
		EfSearch: 100,
	}
}

// retrieveSQL 是两阶段召回的 SQL 模板。
//
// 参数：
//
//	$1 = query embedding (vector)
//	$2 = canonical query tags (text[])
//	$3 = target difficulty (smallint)
//	$4 = vector candidate limit (int)
//	$5 = tag candidate limit (int)
//	$6 = hard skill_category filter (text[])
//	$7 = hard scenario filter (text[])
//	$8 = hard min difficulty (int, 0 means unset)
//	$9 = hard max difficulty (int, 0 means unset)
//	$10 = hard tags-overlap filter (text[])
//	$11 = raw query text (text)
//	$12 = text candidate limit (int)
//
// 关键 design notes：
//   - vector_candidates 用 MATERIALIZED 强制物化，避免 PG 把 CTE 内联导致
//     ORDER BY embedding <=> $1 LIMIT N 被拍平，从而错失 HNSW 索引
//   - 难度软过滤 GREATEST/LEAST 兜底 1~5 边界
//   - 对 tag-only 命中的候选（vc 表 LEFT JOIN 不到），现场算一次 cosine distance
//     这是 SeqScan over 候选集（至多 K*3 行），开销可忽略
//   - tag_overlap 用 unnest + subquery 算交集大小，比 array intersection 函数
//     在 PG 16+ 上表现更稳定
const retrieveSQL = `
WITH vector_candidates AS MATERIALIZED (
    SELECT id, embedding <=> $1::vector AS vec_dist
    FROM question_bank
    WHERE difficulty BETWEEN GREATEST($3 - 2, 1) AND LEAST($3 + 2, 5)
      AND status = 'active'
      AND embedding_status = 'embedded'
      AND embedding IS NOT NULL
      AND (cardinality($6::text[]) = 0 OR skill_category = ANY($6::text[]))
      AND (cardinality($7::text[]) = 0 OR scenario = ANY($7::text[]))
      AND ($8::int = 0 OR difficulty >= $8)
      AND ($9::int = 0 OR difficulty <= $9)
      AND (cardinality($10::text[]) = 0 OR tags && $10::text[])
    ORDER BY embedding <=> $1::vector
    LIMIT $4
),
tag_candidates AS MATERIALIZED (
    SELECT id
    FROM question_bank
    WHERE (tags || ARRAY[skill_category]) && $2::text[]
      AND status = 'active'
      AND embedding_status = 'embedded'
      AND embedding IS NOT NULL
      AND difficulty BETWEEN GREATEST($3 - 2, 1) AND LEAST($3 + 2, 5)
      AND (cardinality($6::text[]) = 0 OR skill_category = ANY($6::text[]))
      AND (cardinality($7::text[]) = 0 OR scenario = ANY($7::text[]))
      AND ($8::int = 0 OR difficulty >= $8)
      AND ($9::int = 0 OR difficulty <= $9)
      AND (cardinality($10::text[]) = 0 OR tags && $10::text[])
    LIMIT $5
),
text_candidates AS MATERIALIZED (
    SELECT id, similarity(content, $11::text) AS text_score
    FROM question_bank
    WHERE $11::text <> ''
      AND status = 'active'
      AND embedding_status = 'embedded'
      AND embedding IS NOT NULL
      AND difficulty BETWEEN GREATEST($3 - 2, 1) AND LEAST($3 + 2, 5)
      AND (cardinality($6::text[]) = 0 OR skill_category = ANY($6::text[]))
      AND (cardinality($7::text[]) = 0 OR scenario = ANY($7::text[]))
      AND ($8::int = 0 OR difficulty >= $8)
      AND ($9::int = 0 OR difficulty <= $9)
      AND (cardinality($10::text[]) = 0 OR tags && $10::text[])
    ORDER BY similarity(content, $11::text) DESC
    LIMIT $12
),
candidates AS (
    SELECT id FROM vector_candidates
    UNION
    SELECT id FROM tag_candidates
    UNION
    SELECT id FROM text_candidates
)
SELECT
    q.id, q.content, q.tags, q.difficulty, q.skill_category, q.expected_points,
    COALESCE(vc.vec_dist, q.embedding <=> $1::vector) AS vec_dist,
    (SELECT count(*)::int FROM unnest(q.tags || ARRAY[q.skill_category]) t WHERE t = ANY($2::text[])) AS tag_overlap,
    vc.id IS NOT NULL AS vector_hit,
    tc.id IS NOT NULL AS tag_hit,
    txt.id IS NOT NULL AS text_hit,
    COALESCE(txt.text_score, 0) AS text_score
FROM candidates c
JOIN question_bank q USING (id)
LEFT JOIN vector_candidates vc USING (id)
LEFT JOIN tag_candidates tc USING (id)
LEFT JOIN text_candidates txt USING (id);
`

// Retrieve 执行完整两阶段检索。
func (r *PGVectorRetriever) Retrieve(ctx context.Context, q Query) ([]Result, error) {
	result, err := r.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	return result.Results, nil
}

func (r *PGVectorRetriever) Search(ctx context.Context, q Query) (PipelineResult, error) {
	start := time.Now()
	candidates, err := r.retrieveCandidates(ctx, q)
	if err != nil {
		return PipelineResult{}, err
	}
	return buildPGVectorPipelineResult(q, candidates, r.Fusion, float64(time.Since(start).Microseconds())/1000), nil
}

type pgCandidate struct {
	Candidate
	VectorHit bool
	TagHit    bool
	TextHit   bool
	TextScore float64
}

func (r *PGVectorRetriever) retrieveCandidates(ctx context.Context, q Query) ([]pgCandidate, error) {
	if r.Pool == nil {
		return nil, errors.New("retriever: pool not initialized")
	}
	if len(q.QueryEmbedding) == 0 {
		return nil, errors.New("retriever: query embedding required")
	}
	K := q.K
	if K <= 0 {
		K = 5
	}
	vecN := q.VectorCandidates
	if vecN <= 0 {
		vecN = K * 5
	}
	tagN := q.TagCandidates
	if tagN <= 0 {
		tagN = K * 3
	}
	textN := q.TextCandidates
	if textN <= 0 {
		textN = K * 3
	}
	diff := q.Difficulty
	if diff < 1 || diff > 5 {
		diff = 3 // 没给目标难度时按中位
	}

	canonical := CanonicalizeTags(q.Tags)
	if canonical == nil {
		// pgx 对 text[] 参数：传 []string{} 会绑 '{}'，nil 会绑 NULL
		// 我们要的是空数组，避免 tags && NULL 永远 false
		canonical = []string{}
	}
	skillCategories := compactQueryStrings(q.SkillCategories)
	scenarios := compactQueryStrings(q.Scenarios)
	filterTags := CanonicalizeTags(q.FilterTags)
	if filterTags == nil {
		filterTags = []string{}
	}
	diffMin, diffMax := normalizeHardDifficultyRange(q.DifficultyMin, q.DifficultyMax)

	// pgvector 接受 '[0.1,0.2,...]' 文本格式作为 vector 类型字面量
	vecLit := vectorLiteral(q.QueryEmbedding)

	// 借连接 → 在 session 里 SET LOCAL hnsw.ef_search，事务结束自动还原
	conn, err := r.Pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire conn: %w", err)
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) // 读事务无所谓，最后用 Commit 也行；Rollback 更简单

	// SET LOCAL 只在当前事务有效，不污染连接池里别的 session
	if _, err := tx.Exec(ctx, "SET LOCAL hnsw.ef_search = "+strconv.Itoa(r.EfSearch)); err != nil {
		// 不致命——SET 失败大概率是 pgvector 版本不支持该参数，
		// 降级走默认 ef_search=40 也能用
		_ = err
	}

	rows, err := tx.Query(ctx, retrieveSQL, vecLit, canonical, diff, vecN, tagN, skillCategories, scenarios, diffMin, diffMax, filterTags, q.Text, textN)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var candidates []pgCandidate
	for rows.Next() {
		var c pgCandidate
		if err := rows.Scan(&c.ID, &c.Content, &c.Tags, &c.Difficulty,
			&c.Category, &c.ExpectedPoints, &c.VecDist, &c.TagOverlap,
			&c.VectorHit, &c.TagHit, &c.TextHit, &c.TextScore); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		c.QueryTagCount = len(canonical)
		c.TargetDiff = diff
		if q.Text != "" {
			vectorScore := clamp01(1 - c.VecDist)
			lexicalScore := lexicalSimilarity(q.Text, c.Content)
			c.VecDist = 1 - clamp01(0.75*vectorScore+0.25*lexicalScore)
		}
		candidates = append(candidates, c)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("rows iter: %w", rows.Err())
	}
	return candidates, nil
}

func buildPGVectorPipelineResult(q Query, candidates []pgCandidate, fusion Fusion, durationMS float64) PipelineResult {
	if fusion == nil {
		fusion = NewLinearFusion(0, 0, 0)
	}
	raw := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		raw = append(raw, c.Candidate)
	}
	results := fusion.Fuse(raw)
	K := q.K
	if K <= 0 {
		K = 5
	}
	if len(results) > K {
		results = results[:K]
	}
	trace := RetrievalTrace{Query: q.Text}
	trace.Stages = append(trace.Stages,
		pgCandidateStageTrace(StageVector, candidates, durationMS, func(c pgCandidate) (bool, float64) {
			return c.VectorHit, clamp01(1 - c.VecDist)
		}),
		pgCandidateStageTrace(StageRule, candidates, 0, func(c pgCandidate) (bool, float64) {
			if !c.TagHit {
				return false, 0
			}
			if c.QueryTagCount <= 0 {
				return true, 0
			}
			return true, clamp01(float64(c.TagOverlap) / float64(c.QueryTagCount))
		}),
		pgCandidateStageTrace(StageBM25, candidates, 0, func(c pgCandidate) (bool, float64) {
			return c.TextHit, clamp01(c.TextScore)
		}),
		stageTraceFromResults(StageRRF, results, 0, ""),
	)
	for i, result := range results {
		trace.Final = append(trace.Final, ResultTrace{
			ID:    result.ID,
			Rank:  i + 1,
			Score: result.Score,
			Stage: StageRRF,
			Sources: map[string]float64{
				"vector":     result.VectorScore,
				"tag":        result.TagScore,
				"difficulty": result.DifficultyScore,
			},
		})
	}
	return PipelineResult{Results: results, RRFResults: results, Trace: trace}
}

func pgCandidateStageTrace(stage string, candidates []pgCandidate, durationMS float64, include func(pgCandidate) (bool, float64)) StageTrace {
	st := StageTrace{Stage: stage, DurationMS: durationMS}
	for _, c := range candidates {
		ok, score := include(c)
		if !ok || c.ID == "" {
			continue
		}
		item := ResultTrace{
			ID:    c.ID,
			Rank:  len(st.Items) + 1,
			Score: score,
			Stage: stage,
			Sources: map[string]float64{
				"vector":      clamp01(1 - c.VecDist),
				"tag_overlap": float64(c.TagOverlap),
				"text":        clamp01(c.TextScore),
			},
		}
		st.Items = append(st.Items, item)
	}
	st.Count = len(st.Items)
	return st
}

func compactQueryStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func normalizeHardDifficultyRange(min, max int) (int, int) {
	min = normalizeHardDifficulty(min)
	max = normalizeHardDifficulty(max)
	if min > 0 && max > 0 && min > max {
		return max, min
	}
	return min, max
}

func normalizeHardDifficulty(n int) int {
	if n < 1 || n > 5 {
		return 0
	}
	return n
}

func lexicalSimilarity(query, doc string) float64 {
	queryTokens := textTokenWeights(query)
	if len(queryTokens) == 0 {
		return 0
	}
	docTokens := textTokenWeights(doc)
	var hit, total float64
	for token, weight := range queryTokens {
		total += weight
		if _, ok := docTokens[token]; ok {
			hit += weight
		}
	}
	if total == 0 {
		return 0
	}
	return clamp01(hit / total)
}

func textTokenWeights(s string) map[string]float64 {
	out := map[string]float64{}
	var ascii strings.Builder
	han := make([]rune, 0, 8)
	flushASCII := func() {
		if ascii.Len() == 0 {
			return
		}
		token := ascii.String()
		ascii.Reset()
		if len(token) >= 2 {
			out[token] = 2
		}
	}
	flushHan := func() {
		if len(han) == 0 {
			return
		}
		if len(han) == 1 {
			out[string(han[0])] = 0.5
			han = han[:0]
			return
		}
		for i := 0; i < len(han)-1; i++ {
			out[string(han[i:i+2])] = 1
		}
		han = han[:0]
	}
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.Is(unicode.Han, r):
			flushASCII()
			han = append(han, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushHan()
			ascii.WriteRune(r)
		default:
			flushASCII()
			flushHan()
		}
	}
	flushASCII()
	flushHan()
	return out
}

// vectorLiteral 把 float32 切片格式化成 pgvector 字面量 '[0.1,0.2,...]'。
//
// 为什么手动拼字符串而不用 pgvector 的 Go binding：
//   - pgvector/pgx 的官方 binding 是 github.com/pgvector/pgvector-go，
//     但拖一个第三方库只为格式化字符串不值
//   - 字面量格式很稳，pgvector 文档明确支持
//   - 性能上比反射式 codec 略快
//
// 注意：strconv.FormatFloat 用 'g' 格式 + -1 精度——保留 IEEE-754 精确表示，
// 不会因为打印格式损失精度（'f' 固定小数位会丢尾数）
func vectorLiteral(v []float32) string {
	var sb strings.Builder
	sb.Grow(len(v) * 12) // 估算
	sb.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatFloat(float64(x), 'g', -1, 32))
	}
	sb.WriteByte(']')
	return sb.String()
}
