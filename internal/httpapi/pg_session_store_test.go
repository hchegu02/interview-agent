package httpapi

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"interview-agent/internal/domain"
)

type fakePGSessionRow struct {
	rowVersion int64
	err        error
}

func (r fakePGSessionRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 1 {
		return errors.New("unexpected scan destination count")
	}
	ptr, ok := dest[0].(*int64)
	if !ok {
		return errors.New("unexpected scan destination type")
	}
	*ptr = r.rowVersion
	return nil
}

func skipIfNoPGStoreIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run PG session store integration tests")
	}
}

func openPGStoreTestPool(t *testing.T) *pgxpool.Pool {
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

func TestIntegration_PGSessionStore_SaveGet(t *testing.T) {
	skipIfNoPGStoreIntegration(t)
	ctx := context.Background()
	pool := openPGStoreTestPool(t)
	store := NewPGSessionStore(pool, time.Hour)
	sess := &domain.Session{
		ID:          "pg-store-test",
		UserID:      "u1",
		Status:      domain.StatusRunning,
		CurrentNode: "pick_next",
		WorkingMemory: &domain.WorkingMemory{
			Notes: map[string]string{"reflect_topic": "redis"},
		},
	}
	if err := store.Save(ctx, sess); err != nil {
		t.Fatalf("save: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sessions WHERE id = $1`, sess.ID)
	})

	got, err := store.Get(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != sess.ID || got.UserID != "u1" {
		t.Fatalf("got wrong session: %+v", got)
	}
	if got.WorkingMemory.ReflectTopic != "redis" {
		t.Errorf("legacy reflect_topic should migrate, got %q", got.WorkingMemory.ReflectTopic)
	}
}

func TestIntegration_PGSessionStore_ListByUser(t *testing.T) {
	skipIfNoPGStoreIntegration(t)
	ctx := context.Background()
	pool := openPGStoreTestPool(t)
	store := NewPGSessionStore(pool, time.Hour)
	ids := []string{"pg-list-old", "pg-list-new", "pg-list-other"}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sessions WHERE id = ANY($1)`, ids)
	})

	if err := store.Save(ctx, &domain.Session{ID: "pg-list-old", UserID: "u-list", Status: domain.StatusRunning}); err != nil {
		t.Fatalf("save old: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := store.Save(ctx, &domain.Session{ID: "pg-list-new", UserID: "u-list", Status: domain.StatusCompleted}); err != nil {
		t.Fatalf("save new: %v", err)
	}
	if err := store.Save(ctx, &domain.Session{ID: "pg-list-other", UserID: "other", Status: domain.StatusRunning}); err != nil {
		t.Fatalf("save other: %v", err)
	}

	got, err := store.ListByUser(ctx, "u-list", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("len = %d, want at least 2", len(got))
	}
	if got[0].ID != "pg-list-new" || got[1].ID != "pg-list-old" {
		t.Fatalf("order = [%s %s], want [pg-list-new pg-list-old]", got[0].ID, got[1].ID)
	}
}

func TestPGInterval_DefaultsWhenNonPositive(t *testing.T) {
	if got := pgInterval(0); got != "86400 seconds" {
		t.Errorf("pgInterval(0) = %q, want 86400 seconds", got)
	}
	if got := pgInterval(-time.Second); got != "86400 seconds" {
		t.Errorf("pgInterval(-1s) = %q, want 86400 seconds", got)
	}
	if got := pgInterval(2 * time.Hour); got != "7200 seconds" {
		t.Errorf("pgInterval(2h) = %q, want 7200 seconds", got)
	}
}

func TestPGSessionStore_SaveWithQueryRowRejectsStaleWrite(t *testing.T) {
	store := NewPGSessionStore(nil, time.Hour)
	err := store.saveWithQueryRow(context.Background(), &domain.Session{
		ID:         "stale",
		UserID:     "u1",
		Status:     domain.StatusRunning,
		UpdatedAt:  time.Now(),
		RowVersion: 3,
	}, func(ctx context.Context, sql string, args ...any) pgSessionRow {
		if !strings.Contains(sql, "WHERE sessions.row_version = $8") {
			t.Fatalf("upsert SQL missing row_version guard: %s", sql)
		}
		if !strings.Contains(sql, "row_version = sessions.row_version + 1") {
			t.Fatalf("upsert SQL should increment row_version: %s", sql)
		}
		if len(args) != 8 {
			t.Fatalf("args len = %d, want 8", len(args))
		}
		if got := args[7]; got != int64(3) {
			t.Fatalf("row_version arg = %#v, want 3", got)
		}
		return fakePGSessionRow{err: pgx.ErrNoRows}
	})
	if !errors.Is(err, ErrStaleSessionWrite) {
		t.Fatalf("err = %v, want ErrStaleSessionWrite", err)
	}
}

func TestPGSessionStore_SaveWithQueryRowFillsZeroUpdatedAtAndRowVersion(t *testing.T) {
	store := NewPGSessionStore(nil, time.Hour)
	sess := &domain.Session{
		ID:     "zero-updated",
		UserID: "u1",
		Status: domain.StatusRunning,
	}

	if err := store.saveWithQueryRow(context.Background(), sess, func(ctx context.Context, sql string, args ...any) pgSessionRow {
		if len(args) != 8 {
			t.Fatalf("args len = %d, want 8", len(args))
		}
		if _, ok := args[6].(time.Time); !ok {
			t.Fatalf("updated_at arg type = %T", args[6])
		}
		if got := args[7]; got != int64(0) {
			t.Fatalf("initial row_version arg = %#v, want 0", got)
		}
		return fakePGSessionRow{rowVersion: 1}
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if sess.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt should be filled")
	}
	if sess.CreatedAt.IsZero() {
		t.Fatal("CreatedAt should be filled with zero UpdatedAt")
	}
	if sess.RowVersion != 1 {
		t.Fatalf("RowVersion = %d, want 1", sess.RowVersion)
	}
}

func TestPGSessionStore_SaveWithQueryRowPropagatesQueryError(t *testing.T) {
	store := NewPGSessionStore(nil, time.Hour)
	dbErr := errors.New("db down")
	err := store.saveWithQueryRow(context.Background(), &domain.Session{
		ID:         "exec-error",
		UserID:     "u1",
		Status:     domain.StatusRunning,
		UpdatedAt:  time.Now(),
		RowVersion: 2,
	}, func(ctx context.Context, sql string, args ...any) pgSessionRow {
		return fakePGSessionRow{err: dbErr}
	})
	if !errors.Is(err, dbErr) {
		t.Fatalf("err = %v, want dbErr", err)
	}
}

func TestPGSessionStore_SaveWithQueryRowUsesReturnedVersionForNextSave(t *testing.T) {
	store := NewPGSessionStore(nil, time.Hour)
	sess := &domain.Session{
		ID:         "versioned",
		UserID:     "u1",
		Status:     domain.StatusRunning,
		UpdatedAt:  time.Now(),
		RowVersion: 1,
	}
	var seenVersions []int64

	for _, returnedVersion := range []int64{2, 3} {
		if err := store.saveWithQueryRow(context.Background(), sess, func(ctx context.Context, sql string, args ...any) pgSessionRow {
			v, ok := args[7].(int64)
			if !ok {
				t.Fatalf("row_version arg type = %T", args[7])
			}
			seenVersions = append(seenVersions, v)
			return fakePGSessionRow{rowVersion: returnedVersion}
		}); err != nil {
			t.Fatalf("save returned version %d: %v", returnedVersion, err)
		}
	}

	if got, want := seenVersions, []int64{1, 2}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("seen row versions = %v, want %v", got, want)
	}
	if sess.RowVersion != 3 {
		t.Fatalf("RowVersion = %d, want 3", sess.RowVersion)
	}
}
