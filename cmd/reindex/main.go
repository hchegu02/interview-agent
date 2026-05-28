// Command reindex 把 seeds/*.json 里的题目向量化后 UPSERT 到 question_bank。
//
// 用法：
//
//	# Mock 模式（CI 友好，无需 API key）
//	go run ./cmd/reindex -seed seeds/question_bank.json -mode mock
//
//	# Real 模式（DashScope text-embedding-v4）
//	$env:INTERVIEW_EMBEDDING_API_KEY="sk-..."
//	go run ./cmd/reindex -seed seeds/question_bank.json -mode real \
//	    -model text-embedding-v4 -dim 1024
//
// 设计要点：
//   - 批量调用 Embedder.Embed，避免单条 HTTP 往返
//   - UPSERT 用 ON CONFLICT (id) DO UPDATE，幂等可重跑
//   - 入库前 CanonicalizeTags 一遍，确保库里只存 canonical 形式
//   - DSN 优先走环境变量 INTERVIEW_POSTGRES_DSN，兼容 PG_DSN
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"interview-agent/internal/embedding"
	"interview-agent/internal/retriever"
)

type seedRow struct {
	ID             string            `json:"id"`
	Content        string            `json:"content"`
	Tags           []string          `json:"tags"`
	SkillCategory  string            `json:"skill_category"`
	Difficulty     int               `json:"difficulty"`
	ExpectedPoints []string          `json:"expected_points,omitempty"`
	Scenario       string            `json:"scenario,omitempty"`
	RoleTags       []string          `json:"role_tags,omitempty"`
	Rubric         map[string]string `json:"rubric,omitempty"`
	SampleAnswer   string            `json:"sample_answer,omitempty"`
	FollowUpHints  []string          `json:"follow_up_hints,omitempty"`
	Locale         string            `json:"locale,omitempty"`
	Status         string            `json:"status,omitempty"`
}

func main() {
	var (
		seedPath = flag.String("seed", "seeds/question_bank.json", "种子文件路径")
		mode     = flag.String("mode", "mock", "mock | real")
		baseURL  = flag.String("base-url", "https://dashscope.aliyuncs.com/compatible-mode/v1", "embedding endpoint")
		model    = flag.String("model", "text-embedding-v4", "embedding model")
		dim      = flag.Int("dim", 1024, "embedding 维度，必须与 question_bank.embedding 一致")
		batch    = flag.Int("batch", 16, "单批 embed 条数")
		dsn      = flag.String("dsn", "", "PG DSN，留空读 INTERVIEW_POSTGRES_DSN / PG_DSN 环境变量")
		dryRun   = flag.Bool("dry-run", false, "只打印不落库，用于离线核对")
	)
	flag.Parse()

	if err := run(context.Background(), *seedPath, *mode, *baseURL, *model, *dim, *batch, *dsn, *dryRun); err != nil {
		log.Fatalf("reindex failed: %v", err)
	}
}

func run(ctx context.Context, seedPath, mode, baseURL, model string, dim, batch int, dsn string, dryRun bool) error {
	// 1. 读种子
	rows, err := loadSeeds(seedPath)
	if err != nil {
		return fmt.Errorf("load seeds: %w", err)
	}
	log.Printf("loaded %d rows from %s", len(rows), seedPath)

	// 2. 构造 embedder
	embedder, err := buildEmbedder(mode, baseURL, model, dim)
	if err != nil {
		return fmt.Errorf("build embedder: %w", err)
	}
	log.Printf("embedder=%s dim=%d", embedder.Name(), embedder.Dimension())

	// 3. 全量 embed（分批）
	texts := make([]string, len(rows))
	for i, r := range rows {
		texts[i] = embedText(r)
	}
	vectors, err := embedBatched(ctx, embedder, texts, batch)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	if len(vectors) != len(rows) {
		return fmt.Errorf("embed count mismatch: got %d, want %d", len(vectors), len(rows))
	}

	// 4. canonical tags
	for i := range rows {
		normalizeSeedRowMetadata(&rows[i])
	}

	if dryRun {
		for i, r := range rows {
			log.Printf("[dry-run] %s tags=%v dim=%d", r.ID, r.Tags, len(vectors[i]))
		}
		return nil
	}

	// 5. 连 PG 并 UPSERT
	if dsn == "" {
		dsn = postgresDSNFromEnv()
	}
	if dsn == "" {
		dsn = "postgres://interview:interview@localhost:5432/interview?sslmode=disable"
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect pg: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping pg: %w", err)
	}

	t0 := time.Now()
	if err := upsertAll(ctx, pool, rows, vectors); err != nil {
		return fmt.Errorf("upsert: %w", err)
	}
	log.Printf("upserted %d rows in %s", len(rows), time.Since(t0))
	return nil
}

func postgresDSNFromEnv() string {
	if dsn := os.Getenv("INTERVIEW_POSTGRES_DSN"); dsn != "" {
		return dsn
	}
	return os.Getenv("PG_DSN")
}

func loadSeeds(path string) ([]seedRow, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []seedRow
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	// 基础校验
	seen := map[string]bool{}
	for i, r := range rows {
		if r.ID == "" {
			return nil, fmt.Errorf("row[%d] missing id", i)
		}
		if seen[r.ID] {
			return nil, fmt.Errorf("duplicate id: %s", r.ID)
		}
		seen[r.ID] = true
		if r.Difficulty < 1 || r.Difficulty > 5 {
			return nil, fmt.Errorf("row %s difficulty out of [1,5]: %d", r.ID, r.Difficulty)
		}
		if strings.TrimSpace(r.Content) == "" {
			return nil, fmt.Errorf("row %s empty content", r.ID)
		}
	}
	return rows, nil
}

func normalizeSeedRowMetadata(r *seedRow) {
	r.Tags = nonNilStrings(retriever.CanonicalizeTags(r.Tags))
	r.RoleTags = nonNilStrings(retriever.CanonicalizeTags(r.RoleTags))
	r.ExpectedPoints = nonNilStrings(r.ExpectedPoints)
	r.FollowUpHints = nonNilStrings(r.FollowUpHints)
	if r.Rubric == nil {
		r.Rubric = map[string]string{}
	}
	if r.Locale == "" {
		r.Locale = "zh-CN"
	}
	if r.Status == "" {
		r.Status = "active"
	}
}

func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func buildEmbedder(mode, baseURL, model string, dim int) (embedding.Embedder, error) {
	switch mode {
	case "mock":
		return embedding.NewMockEmbedder(dim), nil
	case "real":
		key := os.Getenv("INTERVIEW_EMBEDDING_API_KEY")
		if key == "" {
			return nil, errors.New("INTERVIEW_EMBEDDING_API_KEY not set (do NOT paste keys in source/yaml)")
		}
		return embedding.NewRealEmbedder(baseURL, key, model, dim, 30*time.Second), nil
	default:
		return nil, fmt.Errorf("unknown mode: %s", mode)
	}
}

// embedText 把一条题目拼成 embed 用的语料。
// content 占主导，tags/category 作为弱信号串进去——避免噪声但有利于聚类。
func embedText(r seedRow) string {
	var sb strings.Builder
	sb.WriteString(r.Content)
	if len(r.Tags) > 0 {
		sb.WriteString("\nTags: ")
		sb.WriteString(strings.Join(r.Tags, ", "))
	}
	if r.SkillCategory != "" {
		sb.WriteString("\nCategory: ")
		sb.WriteString(r.SkillCategory)
	}
	return sb.String()
}

func embedBatched(ctx context.Context, e embedding.Embedder, texts []string, batch int) ([][]float32, error) {
	if batch <= 0 {
		batch = 16
	}
	out := make([][]float32, 0, len(texts))
	for i := 0; i < len(texts); i += batch {
		j := i + batch
		if j > len(texts) {
			j = len(texts)
		}
		vecs, err := e.Embed(ctx, texts[i:j])
		if err != nil {
			return nil, fmt.Errorf("batch [%d:%d]: %w", i, j, err)
		}
		out = append(out, vecs...)
		log.Printf("embedded %d/%d", len(out), len(texts))
	}
	return out, nil
}

// 重跑 reindex 不更新 created_at——保留首次入库时间，符合"种子题库"语义。
// updated_at 只表示题目内容/元数据最近一次被 reindex 覆盖的时间。
const upsertSQL = `
INSERT INTO question_bank (
    id, content, tags, skill_category, difficulty, expected_points,
    scenario, role_tags, rubric, sample_answer, follow_up_hints, locale, status,
    embedding
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11, $12, $13, $14::vector)
ON CONFLICT (id) DO UPDATE SET
    content         = EXCLUDED.content,
    tags            = EXCLUDED.tags,
    skill_category  = EXCLUDED.skill_category,
    difficulty      = EXCLUDED.difficulty,
    expected_points = EXCLUDED.expected_points,
    scenario        = EXCLUDED.scenario,
    role_tags       = EXCLUDED.role_tags,
    rubric          = EXCLUDED.rubric,
    sample_answer   = EXCLUDED.sample_answer,
    follow_up_hints = EXCLUDED.follow_up_hints,
    locale          = EXCLUDED.locale,
    status          = EXCLUDED.status,
    embedding       = EXCLUDED.embedding,
    updated_at = now();
`

func upsertAll(ctx context.Context, pool *pgxpool.Pool, rows []seedRow, vectors [][]float32) error {
	// 简单实现：逐行 exec。30~300 行规模无所谓；上了量再切批 COPY。
	for i, r := range rows {
		vecLit := vectorLiteral(vectors[i])
		rubric, err := json.Marshal(r.Rubric)
		if err != nil {
			return fmt.Errorf("row %s rubric: %w", r.ID, err)
		}
		if _, err := pool.Exec(ctx, upsertSQL,
			r.ID, r.Content, r.Tags, r.SkillCategory, r.Difficulty, r.ExpectedPoints,
			r.Scenario, r.RoleTags, string(rubric), r.SampleAnswer, r.FollowUpHints, r.Locale, r.Status,
			vecLit); err != nil {
			return fmt.Errorf("row %s: %w", r.ID, err)
		}
	}
	return nil
}

// vectorLiteral 与 internal/retriever/pgvector.go 里同名函数等价，
// 这里复制一份避免 cmd 包反向依赖 retriever 包的内部函数。
// 抽到 pkg/pgvecutil 也行，目前只有两个调用方就先不抽。
func vectorLiteral(v []float32) string {
	var sb strings.Builder
	sb.Grow(len(v) * 12)
	sb.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		// 用 %g 保留精度
		fmt.Fprintf(&sb, "%g", x)
	}
	sb.WriteByte(']')
	return sb.String()
}
