// Package migrations 仅含测试，验证 SQL 文件的基础健康度。
//
// 这种测试的价值：在没启动 PG 的 CI 环境下，至少保证：
//   - migration 文件存在且非空
//   - up 和 down 配对
//   - up 里 CREATE 的表，down 里都有 DROP
//   - 关键扩展 / 索引声明存在
//
// 真正的"能跑通"验证靠 Stage 1.4 的 docker integration 测试。
package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func read(t *testing.T, name string) string {
	t.Helper()
	// 测试从 internal 目录执行时，回到仓库根再进 migrations
	candidates := []string{
		name,
		filepath.Join("..", "..", "migrations", name),
		filepath.Join("migrations", name),
	}
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err == nil {
			return string(b)
		}
	}
	t.Fatalf("cannot find %s in any candidate path", name)
	return ""
}

func TestUpDownPairing(t *testing.T) {
	up := read(t, "001_init.up.sql")
	down := read(t, "001_init.down.sql")

	tables := []string{"sessions", "question_bank", "idempotency_keys", "events"}
	for _, tbl := range tables {
		if !strings.Contains(up, "CREATE TABLE IF NOT EXISTS "+tbl) {
			t.Errorf("up missing CREATE TABLE for %s", tbl)
		}
		if !strings.Contains(down, "DROP TABLE IF EXISTS "+tbl) {
			t.Errorf("down missing DROP TABLE for %s", tbl)
		}
	}
}

func TestPgvectorExtension(t *testing.T) {
	up := read(t, "001_init.up.sql")
	if !strings.Contains(up, "CREATE EXTENSION IF NOT EXISTS vector") {
		t.Error("missing pgvector extension declaration")
	}
}

func TestHNSWIndex(t *testing.T) {
	up := read(t, "001_init.up.sql")
	if !strings.Contains(up, "USING hnsw") {
		t.Error("missing HNSW index — required for vector search")
	}
	if !strings.Contains(up, "vector_cosine_ops") {
		t.Error("HNSW should use vector_cosine_ops for cosine similarity")
	}
}

func TestGINIndexForTags(t *testing.T) {
	up := read(t, "001_init.up.sql")
	if !strings.Contains(up, "USING gin (tags)") {
		t.Error("missing GIN index for tags array filtering")
	}
}

func TestStatusCheckConstraint(t *testing.T) {
	up := read(t, "001_init.up.sql")
	// 保护：禁止脏数据写入非法状态
	for _, status := range []string{"'created'", "'running'", "'paused'", "'completed'", "'failed'"} {
		if !strings.Contains(up, status) {
			t.Errorf("status CHECK should include %s", status)
		}
	}
}

func TestPartialIndexesForCleanup(t *testing.T) {
	up := read(t, "001_init.up.sql")
	// 关键工程优化：partial index 只索引需要扫描的子集
	if !strings.Contains(up, "WHERE status IN ('running','paused')") {
		t.Error("orphan-takeover scan should use partial index on running/paused")
	}
	if !strings.Contains(up, "WHERE status IN ('completed','failed')") {
		t.Error("TTL cleanup scan should use partial index on completed/failed")
	}
}

func TestSeedFile(t *testing.T) {
	seed := read(t, "seed_question_bank.sql")
	// 至少 10 道题，覆盖 go / redis / system-design 三类
	for _, tag := range []string{"'go'", "'redis'", "'system-design'"} {
		if !strings.Contains(seed, tag) {
			t.Errorf("seed missing category %s", tag)
		}
	}
	if !strings.Contains(seed, "ON CONFLICT (id) DO NOTHING") {
		t.Error("seed should be idempotent via ON CONFLICT")
	}
}

func TestQuestionBankExpectedPointsColumn(t *testing.T) {
	up := read(t, "001_init.up.sql")
	if !strings.Contains(up, "expected_points") {
		t.Fatal("question_bank should store expected_points from seed data")
	}
	if !strings.Contains(up, "expected_points  text[]") && !strings.Contains(up, "expected_points text[]") {
		t.Fatal("expected_points should be a text[] column")
	}
}
