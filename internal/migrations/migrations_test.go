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

func TestQuestionBankMetadataMigration(t *testing.T) {
	up := read(t, "003_question_bank_metadata.up.sql")
	down := read(t, "003_question_bank_metadata.down.sql")
	for _, col := range []string{
		"scenario",
		"role_tags",
		"rubric",
		"sample_answer",
		"follow_up_hints",
		"locale",
		"status",
		"updated_at",
	} {
		if !strings.Contains(up, col) {
			t.Fatalf("metadata migration missing column %q", col)
		}
		if !strings.Contains(down, "DROP COLUMN IF EXISTS "+col) {
			t.Fatalf("metadata down migration should drop column %q", col)
		}
	}
	if !strings.Contains(up, "idx_question_bank_status") {
		t.Fatal("metadata migration should add status index")
	}
	if !strings.Contains(up, "idx_question_bank_scenario") {
		t.Fatal("metadata migration should add scenario index")
	}
}

func TestQuestionBankImportMigration(t *testing.T) {
	up := read(t, "004_question_bank_imports.up.sql")
	down := read(t, "004_question_bank_imports.down.sql")
	for _, tbl := range []string{
		"question_bank_import_jobs",
		"question_bank_import_chunks",
		"question_bank_import_items",
	} {
		if !strings.Contains(up, "CREATE TABLE IF NOT EXISTS "+tbl) {
			t.Fatalf("import migration missing table %q", tbl)
		}
		if !strings.Contains(down, "DROP TABLE IF EXISTS "+tbl) {
			t.Fatalf("import migration down should drop table %q", tbl)
		}
	}
	for _, idx := range []string{
		"idx_qb_import_jobs_status",
		"idx_qb_import_chunks_job",
		"idx_qb_import_items_job_status",
	} {
		if !strings.Contains(up, idx) {
			t.Fatalf("import migration missing index %q", idx)
		}
	}
}

func TestQuestionBankEmbeddingStatusMigration(t *testing.T) {
	up := read(t, "005_question_bank_embedding_status.up.sql")
	down := read(t, "005_question_bank_embedding_status.down.sql")
	for _, col := range []string{
		"embedding_status",
		"embedding_model",
		"embedded_at",
		"embedding_error",
	} {
		if !strings.Contains(up, col) {
			t.Fatalf("embedding status migration missing column %q", col)
		}
		if !strings.Contains(down, "DROP COLUMN IF EXISTS "+col) {
			t.Fatalf("embedding status down migration should drop column %q", col)
		}
	}
	if !strings.Contains(up, "idx_question_bank_embedding_status") {
		t.Fatal("embedding status migration should add status index")
	}
	if !strings.Contains(up, "embedding IS NOT NULL") {
		t.Fatal("embedding status migration should mark existing vectors as embedded")
	}
}

func TestQuestionBankImportReviewStatusMigration(t *testing.T) {
	up := read(t, "007_question_bank_import_review_status.up.sql")
	down := read(t, "007_question_bank_import_review_status.down.sql")
	if !strings.Contains(up, "review_status") {
		t.Fatal("import review migration missing review_status column")
	}
	if !strings.Contains(up, "idx_qb_import_items_job_review") {
		t.Fatal("import review migration should add review index")
	}
	if !strings.Contains(down, "DROP COLUMN IF EXISTS review_status") {
		t.Fatal("import review down migration should drop review_status column")
	}
}

func TestQuestionBankImportFieldProvenanceMigration(t *testing.T) {
	up := read(t, "008_question_bank_import_field_provenance.up.sql")
	down := read(t, "008_question_bank_import_field_provenance.down.sql")
	for _, token := range []string{"question_bank_import_items", "field_provenance", "jsonb"} {
		if !strings.Contains(up, token) {
			t.Fatalf("import field provenance migration missing %q", token)
		}
	}
	if !strings.Contains(down, "DROP COLUMN IF EXISTS field_provenance") {
		t.Fatal("import field provenance down migration should drop field_provenance column")
	}
}

func TestQuestionBankImportLeaseMigration(t *testing.T) {
	up := read(t, "006_question_bank_import_lease.up.sql")
	down := read(t, "006_question_bank_import_lease.down.sql")
	for _, col := range []string{"owner_id", "lease_until"} {
		if !strings.Contains(up, col) {
			t.Fatalf("import lease migration missing column %q", col)
		}
		if !strings.Contains(down, "DROP COLUMN IF EXISTS "+col) {
			t.Fatalf("import lease down migration should drop column %q", col)
		}
	}
	if !strings.Contains(up, "idx_qb_import_jobs_lease") {
		t.Fatal("import lease migration should add lease index")
	}
}

func TestSessionRowVersionMigration(t *testing.T) {
	up := read(t, "010_session_row_version.up.sql")
	down := read(t, "010_session_row_version.down.sql")
	for _, token := range []string{"sessions", "row_version", "bigint", "DEFAULT 1"} {
		if !strings.Contains(up, token) {
			t.Fatalf("session row version migration missing %q", token)
		}
	}
	if !strings.Contains(down, "DROP COLUMN IF EXISTS row_version") {
		t.Fatal("session row version down migration should drop row_version column")
	}
}

func TestUserMemoryMigration(t *testing.T) {
	up := read(t, "011_user_memory.up.sql")
	down := read(t, "011_user_memory.down.sql")
	for _, token := range []string{"CREATE TABLE IF NOT EXISTS user_memory", "user_id text PRIMARY KEY", "memory_json jsonb NOT NULL", "updated_at timestamptz NOT NULL", "user_memory_user_id_not_blank"} {
		if !strings.Contains(up, token) {
			t.Fatalf("user memory migration missing %q", token)
		}
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS user_memory") {
		t.Fatal("user memory down migration should drop user_memory table")
	}
}

func TestUserMemoryRowVersionMigration(t *testing.T) {
	up := read(t, "012_user_memory_row_version.up.sql")
	down := read(t, "012_user_memory_row_version.down.sql")
	for _, token := range []string{"user_memory", "row_version", "bigint", "DEFAULT 1", "user_memory_row_version_positive"} {
		if !strings.Contains(up, token) {
			t.Fatalf("user memory row version migration missing %q", token)
		}
	}
	if !strings.Contains(down, "DROP COLUMN IF EXISTS row_version") {
		t.Fatal("user memory row version down migration should drop row_version column")
	}
	if !strings.Contains(down, "DROP CONSTRAINT IF EXISTS user_memory_row_version_positive") {
		t.Fatal("user memory row version down migration should drop positive constraint")
	}
}
