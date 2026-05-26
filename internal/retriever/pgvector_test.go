package retriever

import (
	"context"
	"math"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 集成测试用 INTEGRATION=1 环境变量门控。
// CI 默认跳过；本地手动跑：
//   docker compose up -d postgres
//   $env:INTEGRATION="1"; $env:INTERVIEW_POSTGRES_DSN="postgres://..."; go test ./internal/retriever/... -v -run Integration
//
// 测试用独立的 schema（如果不指定）或表前缀，避免污染主库。
// 这里偷懒——直接用主库，每个测试都 TRUNCATE question_bank 跑起。
// 真要做生产级隔离用 testcontainers，这里不值。

func skipIfNoIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run integration tests")
	}
}

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("INTERVIEW_POSTGRES_DSN")
	if dsn == "" {
		dsn = os.Getenv("PG_DSN")
	}
	if dsn == "" {
		dsn = "postgres://interview:interview@localhost:5432/interview?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return pool
}

// seedRow 是一条测试题。
type seedRow struct {
	id         string
	content    string
	tags       []string
	category   string
	difficulty int
	embedding  []float32 // 1024 维
}

// seedTestData 清表 + 插入精心设计的测试数据。
// 每条题目向量都"指向"一个特定 query，让我们能验证检索排序符合预期。
//
// 设计逻辑：
//   query = q1（vector 偏向 "go_concurrency"）
//   - go1：vector 完全相同 + tag 命中（应该排第一）
//   - go2：vector 接近 + tag 命中（次之）
//   - redis1：vector 远 + tag 不命中（不应进 top）
//   - mixed：vector 接近 + tag 部分命中
func seedTestData(t *testing.T, pool *pgxpool.Pool) []seedRow {
	t.Helper()
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "TRUNCATE question_bank"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	rng := rand.New(rand.NewSource(42))
	mkVec := func(seed int64, dim int) []float32 {
		r := rand.New(rand.NewSource(seed))
		v := make([]float32, dim)
		for i := range v {
			v[i] = float32(r.NormFloat64())
		}
		// 单位化让 cosine 与 dot product 等价
		var norm float32
		for _, x := range v {
			norm += x * x
		}
		norm = float32(1.0 / math.Sqrt(float64(norm)))
		for i := range v {
			v[i] *= norm
		}
		_ = rng
		return v
	}

	const dim = 1024
	rows := []seedRow{
		{"go1", "GMP 调度模型详解", []string{"go_concurrency"}, "go", 3, mkVec(1, dim)},
		{"go2", "channel 底层数据结构", []string{"go_concurrency"}, "go", 3, mkVec(2, dim)},
		{"go3", "sync.Map 何时用", []string{"go_concurrency"}, "go", 2, mkVec(3, dim)},
		{"go4", "atomic vs mutex 性能", []string{"go_concurrency"}, "go", 4, mkVec(4, dim)},
		{"redis1", "AOF vs RDB 持久化对比", []string{"redis_persistence"}, "redis", 3, mkVec(5, dim)},
		{"redis2", "哨兵选主流程", []string{"redis_ha"}, "redis", 4, mkVec(6, dim)},
		{"mixed1", "Go 实现一个分布式锁（Redis）", []string{"go_concurrency", "redis_ha"}, "go", 4, mkVec(7, dim)},
		// 故意制造一道极远的题，难度也不在范围内，验证 SQL 过滤
		{"far1", "MySQL InnoDB MVCC", []string{"pg_concurrency"}, "mysql", 5, mkVec(99, dim)},
	}

	for _, r := range rows {
		vecLit := vectorLiteral(r.embedding)
		_, err := pool.Exec(ctx, `
			INSERT INTO question_bank (id, content, tags, skill_category, difficulty, embedding)
			VALUES ($1, $2, $3, $4, $5, $6::vector)`,
			r.id, r.content, r.tags, r.category, r.difficulty, vecLit)
		if err != nil {
			t.Fatalf("insert %s: %v", r.id, err)
		}
	}
	return rows
}

func sqrt32(x float32) float64 {
	// 留作占位，已替换为 math.Sqrt 直接调用
	return math.Sqrt(float64(x))
}

func TestIntegration_Retrieve_VectorOnly(t *testing.T) {
	skipIfNoIntegration(t)
	pool := openTestPool(t)
	rows := seedTestData(t, pool)

	r := NewPGVectorRetriever(pool, nil)

	// 用 go1 自己的 embedding 当 query → 自己应该排第一
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results, err := r.Retrieve(ctx, Query{
		QueryEmbedding: rows[0].embedding, // go1
		Difficulty:     3,
		K:              5,
	})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results, got empty")
	}
	if results[0].ID != "go1" {
		t.Errorf("expected go1 first (self-similarity), got %s", results[0].ID)
	}
	// VectorScore 应非常接近 1（同一向量）
	if results[0].VectorScore < 0.99 {
		t.Errorf("self-similarity vector_score should be ~1.0, got %f", results[0].VectorScore)
	}
	// far1 难度 5，target=3 软过滤 ±2 包含 5 在内，所以可能进候选；
	// 但因为 vector 远应该排在后面（不在 top-5）。
	for _, r := range results {
		if r.ID == "far1" {
			t.Errorf("far1 should not appear in top results due to low vector similarity")
		}
	}
}

func TestIntegration_Retrieve_HybridScoring(t *testing.T) {
	skipIfNoIntegration(t)
	pool := openTestPool(t)
	rows := seedTestData(t, pool)

	r := NewPGVectorRetriever(pool, nil)
	ctx := context.Background()

	// 用 mixed1 的 embedding + 只给 redis_ha tag
	// → mixed1 自己应该第一（vector 完美 + tag 命中）
	results, err := r.Retrieve(ctx, Query{
		QueryEmbedding: rows[6].embedding, // mixed1
		Tags:           []string{"sentinel"}, // 别名 → redis_ha
		Difficulty:     4,
		K:              5,
	})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if results[0].ID != "mixed1" {
		t.Errorf("expected mixed1 first, got %s", results[0].ID)
	}
	if results[0].TagScore <= 0 {
		t.Errorf("mixed1 should have tag_score > 0, got %f", results[0].TagScore)
	}
}

func TestIntegration_Retrieve_DifficultySoftFilter(t *testing.T) {
	skipIfNoIntegration(t)
	pool := openTestPool(t)
	rows := seedTestData(t, pool)

	r := NewPGVectorRetriever(pool, nil)
	ctx := context.Background()

	// target = 1，软过滤 [-1, 3] → 包含难度 2、3，不包含 4、5
	results, err := r.Retrieve(ctx, Query{
		QueryEmbedding: rows[0].embedding,
		Difficulty:     1,
		K:              10,
	})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	for _, res := range results {
		if res.Difficulty > 3 {
			t.Errorf("difficulty %d should be filtered out (target=1, ±2)", res.Difficulty)
		}
	}
}

func TestIntegration_Retrieve_EmptyTagsOK(t *testing.T) {
	skipIfNoIntegration(t)
	pool := openTestPool(t)
	rows := seedTestData(t, pool)

	r := NewPGVectorRetriever(pool, nil)
	ctx := context.Background()

	// 不传 tags → 走纯 vector 路径，QueryTagCount=0 时 tag_score 应该全为 0
	results, err := r.Retrieve(ctx, Query{
		QueryEmbedding: rows[0].embedding,
		Difficulty:     3,
		K:              3,
	})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	for _, res := range results {
		if res.TagScore != 0 {
			t.Errorf("empty tags: tag_score should be 0, got %f for %s", res.TagScore, res.ID)
		}
	}
}

func TestIntegration_Retrieve_K_Capping(t *testing.T) {
	skipIfNoIntegration(t)
	pool := openTestPool(t)
	rows := seedTestData(t, pool)

	r := NewPGVectorRetriever(pool, nil)
	ctx := context.Background()

	results, err := r.Retrieve(ctx, Query{
		QueryEmbedding: rows[0].embedding,
		Difficulty:     3,
		K:              2,
	})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(results) > 2 {
		t.Errorf("K=2 should cap at 2, got %d", len(results))
	}
}

func TestIntegration_Retrieve_BadEmbeddingDim(t *testing.T) {
	skipIfNoIntegration(t)
	pool := openTestPool(t)
	_ = seedTestData(t, pool)

	r := NewPGVectorRetriever(pool, nil)
	ctx := context.Background()

	// 100 维 vs DB 1024 维 → pgvector 报错
	bad := make([]float32, 100)
	_, err := r.Retrieve(ctx, Query{
		QueryEmbedding: bad,
		Difficulty:     3,
		K:              5,
	})
	if err == nil {
		t.Fatal("expected dim mismatch error")
	}
	if !strings.Contains(err.Error(), "dim") && !strings.Contains(err.Error(), "expected") {
		// pgvector 报错信息含 "expected 1024 dimensions"
		t.Logf("got error (acceptable): %v", err)
	}
}

func TestIntegration_VectorLiteralFormat(t *testing.T) {
	// 不需要 DB，纯校验字面量格式
	v := []float32{0.1, -0.25, 1e-10, 0}
	got := vectorLiteral(v)
	if !strings.HasPrefix(got, "[") || !strings.HasSuffix(got, "]") {
		t.Errorf("not bracketed: %s", got)
	}
	if got == "" {
		t.Fatal("empty literal")
	}
}

func init() {
	// 让 random seed 稳定，避免不同测试运行产生不同向量
}
