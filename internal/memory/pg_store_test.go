package memory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakePGMemoryRow struct {
	raw       []byte
	updatedAt time.Time
	err       error
}

func (r fakePGMemoryRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 2 {
		return errors.New("unexpected scan destination count")
	}
	raw, ok := dest[0].(*[]byte)
	if !ok {
		return errors.New("unexpected memory_json destination")
	}
	updatedAt, ok := dest[1].(*time.Time)
	if !ok {
		return errors.New("unexpected updated_at destination")
	}
	*raw = append([]byte(nil), r.raw...)
	*updatedAt = r.updatedAt
	return nil
}

func TestNewPGStoreReturnsStore(t *testing.T) {
	var store Store = NewPGStore(nil)
	if store == nil {
		t.Fatal("NewPGStore returned nil")
	}
}

func TestPGStore_GetUserMemoryTrimsUserIDAndMapsNotFound(t *testing.T) {
	store := NewPGStore(nil).(*PGStore)
	var seenUserID string
	_, err := store.getWithQueryRow(context.Background(), " u1 ", func(ctx context.Context, sql string, args ...any) pgMemoryRow {
		if !strings.Contains(sql, "FROM user_memory") {
			t.Fatalf("select SQL should read user_memory: %s", sql)
		}
		if len(args) != 1 {
			t.Fatalf("args len = %d, want 1", len(args))
		}
		seenUserID = args[0].(string)
		return fakePGMemoryRow{err: pgx.ErrNoRows}
	})
	if !errors.Is(err, ErrUserMemoryNotFound) {
		t.Fatalf("err = %v, want ErrUserMemoryNotFound", err)
	}
	if seenUserID != "u1" {
		t.Fatalf("user id arg = %q, want trimmed u1", seenUserID)
	}
}

func TestPGStore_GetUserMemoryDecodesJSONAndUsesRowUpdatedAt(t *testing.T) {
	store := NewPGStore(nil).(*PGStore)
	rowUpdatedAt := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	staleJSONTime := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	raw, err := json.Marshal(&UserMemory{
		UserID:      "u1",
		Strengths:   []string{"项目表达清楚"},
		SkillScores: map[string]float64{"go": 80},
		UpdatedAt:   staleJSONTime,
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	got, err := store.getWithQueryRow(context.Background(), "u1", func(ctx context.Context, sql string, args ...any) pgMemoryRow {
		return fakePGMemoryRow{raw: raw, updatedAt: rowUpdatedAt}
	})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.UserID != "u1" || got.SkillScores["go"] != 80 {
		t.Fatalf("memory = %+v", got)
	}
	if !got.UpdatedAt.Equal(rowUpdatedAt) {
		t.Fatalf("UpdatedAt = %v, want row updated_at %v", got.UpdatedAt, rowUpdatedAt)
	}
}

func TestPGStore_UpsertUserMemoryValidatesUserID(t *testing.T) {
	store := NewPGStore(nil).(*PGStore)
	called := false
	err := store.upsertWithExec(context.Background(), &UserMemory{UserID: "   "}, func(ctx context.Context, sql string, args ...any) error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrInvalidMemoryInput) {
		t.Fatalf("err = %v, want ErrInvalidMemoryInput", err)
	}
	if called {
		t.Fatal("exec should not be called for invalid user_id")
	}

	err = store.upsertWithExec(context.Background(), nil, func(ctx context.Context, sql string, args ...any) error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrInvalidMemoryInput) {
		t.Fatalf("nil err = %v, want ErrInvalidMemoryInput", err)
	}
}

func TestPGStore_UpsertUserMemoryEncodesDefensiveCopyAndUpserts(t *testing.T) {
	store := NewPGStore(nil).(*PGStore)
	updatedAt := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	mem := &UserMemory{
		UserID:      " u1 ",
		Strengths:   []string{"原始强项"},
		Weaknesses:  []Weakness{{Topic: "redis", Evidence: "缓存击穿", Severity: 2}},
		SkillScores: map[string]float64{"redis": 62},
		LastAdvice:  []string{"复习 Redis"},
		UpdatedAt:   updatedAt,
	}
	var rawArg []byte
	err := store.upsertWithExec(context.Background(), mem, func(ctx context.Context, sql string, args ...any) error {
		if !strings.Contains(sql, "ON CONFLICT (user_id) DO UPDATE") {
			t.Fatalf("upsert SQL missing conflict update: %s", sql)
		}
		if !strings.Contains(sql, "memory_json = EXCLUDED.memory_json") {
			t.Fatalf("upsert SQL missing memory_json update: %s", sql)
		}
		if len(args) != 3 {
			t.Fatalf("args len = %d, want 3", len(args))
		}
		if got := args[0]; got != "u1" {
			t.Fatalf("user id arg = %#v, want trimmed u1", got)
		}
		raw, ok := args[1].([]byte)
		if !ok {
			t.Fatalf("memory json arg type = %T", args[1])
		}
		rawArg = append([]byte(nil), raw...)
		if got := args[2].(time.Time); !got.Equal(updatedAt) {
			t.Fatalf("updated_at arg = %v, want %v", got, updatedAt)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	mem.UserID = "changed"
	mem.Strengths[0] = "已污染"
	mem.SkillScores["redis"] = 100

	var stored UserMemory
	if err := json.Unmarshal(rawArg, &stored); err != nil {
		t.Fatalf("unmarshal stored json: %v", err)
	}
	if stored.UserID != "u1" || stored.Strengths[0] != "原始强项" || stored.SkillScores["redis"] != 62 {
		t.Fatalf("stored memory was not a defensive copy: %+v", stored)
	}
}

func TestIntegration_PGStore_SaveGet(t *testing.T) {
	skipIfNoPGMemoryIntegration(t)
	ctx := context.Background()
	pool := openPGMemoryTestPool(t)
	if _, err := pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS user_memory (
	user_id text PRIMARY KEY,
	memory_json jsonb NOT NULL,
	updated_at timestamptz NOT NULL DEFAULT now()
)`); err != nil {
		t.Fatalf("ensure table: %v", err)
	}
	store := NewPGStore(pool)
	userID := "pg-memory-test"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM user_memory WHERE user_id = $1`, userID)
	})

	mem := &UserMemory{
		UserID:      " pg-memory-test ",
		Strengths:   []string{"表达清楚"},
		SkillScores: map[string]float64{"go": 88},
		UpdatedAt:   time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertUserMemory(ctx, mem); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := store.GetUserMemory(ctx, " pg-memory-test ")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.UserID != userID || got.SkillScores["go"] != 88 {
		t.Fatalf("memory = %+v", got)
	}
}

func skipIfNoPGMemoryIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run PG memory store integration tests")
	}
}

func openPGMemoryTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("INTERVIEW_POSTGRES_DSN")
	if dsn == "" {
		dsn = os.Getenv("PG_DSN")
	}
	if dsn == "" {
		t.Skip("set INTERVIEW_POSTGRES_DSN or PG_DSN to run PG memory store integration tests")
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
