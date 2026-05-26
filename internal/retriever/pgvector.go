package retriever

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

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
    ORDER BY embedding <=> $1::vector
    LIMIT $4
),
tag_candidates AS MATERIALIZED (
    SELECT id
    FROM question_bank
    WHERE tags && $2::text[]
      AND difficulty BETWEEN GREATEST($3 - 2, 1) AND LEAST($3 + 2, 5)
    LIMIT $5
),
candidates AS (
    SELECT id FROM vector_candidates
    UNION
    SELECT id FROM tag_candidates
)
SELECT
    q.id, q.content, q.tags, q.difficulty, q.skill_category, q.expected_points,
    COALESCE(vc.vec_dist, q.embedding <=> $1::vector) AS vec_dist,
    (SELECT count(*)::int FROM unnest(q.tags) t WHERE t = ANY($2::text[])) AS tag_overlap
FROM candidates c
JOIN question_bank q USING (id)
LEFT JOIN vector_candidates vc USING (id);
`

// Retrieve 执行完整两阶段检索。
func (r *PGVectorRetriever) Retrieve(ctx context.Context, q Query) ([]Result, error) {
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

	rows, err := tx.Query(ctx, retrieveSQL, vecLit, canonical, diff, vecN, tagN)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var candidates []Candidate
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.ID, &c.Content, &c.Tags, &c.Difficulty,
			&c.Category, &c.ExpectedPoints, &c.VecDist, &c.TagOverlap); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		c.QueryTagCount = len(canonical)
		c.TargetDiff = diff
		candidates = append(candidates, c)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("rows iter: %w", rows.Err())
	}

	results := r.Fusion.Fuse(candidates)
	if len(results) > K {
		results = results[:K]
	}
	return results, nil
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
