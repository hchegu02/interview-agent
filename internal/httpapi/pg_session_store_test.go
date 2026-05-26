package httpapi

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"interview-agent/internal/domain"
)

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
