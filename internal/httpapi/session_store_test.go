package httpapi

import (
	"context"
	"fmt"
	"testing"
	"time"

	"interview-agent/internal/domain"
)

func TestMemorySessionStore_GetMigratesLegacyState(t *testing.T) {
	store := NewMemorySessionStore()
	sess := &domain.Session{
		ID: "s1",
		WorkingMemory: &domain.WorkingMemory{
			Notes: map[string]string{
				"reflect_topic":        "redis",
				"eval_degraded_reason": "llm timeout",
			},
		},
	}
	if err := store.Save(context.Background(), sess); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.Get(context.Background(), "s1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.WorkingMemory.ReflectTopic != "redis" {
		t.Errorf("ReflectTopic = %q, want redis", got.WorkingMemory.ReflectTopic)
	}
	if got.WorkingMemory.DegradedReasons["eval"] != "llm timeout" {
		t.Errorf("eval degraded reason = %q", got.WorkingMemory.DegradedReasons["eval"])
	}
}

func TestMemorySessionStore_SaveRequiresID(t *testing.T) {
	store := NewMemorySessionStore()
	if err := store.Save(context.Background(), &domain.Session{}); err == nil {
		t.Fatal("expected error for empty session id")
	}
}

func TestMemorySessionStore_ListByUserOrdersByUpdatedAt(t *testing.T) {
	store := NewMemorySessionStore()
	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	sessions := []*domain.Session{
		{ID: "old", UserID: "u1", UpdatedAt: base},
		{ID: "other", UserID: "u2", UpdatedAt: base.Add(10 * time.Minute)},
		{ID: "new", UserID: "u1", UpdatedAt: base.Add(20 * time.Minute)},
		{ID: "mid", UserID: "u1", UpdatedAt: base.Add(10 * time.Minute)},
	}
	for _, sess := range sessions {
		if err := store.Save(context.Background(), sess); err != nil {
			t.Fatalf("save %s: %v", sess.ID, err)
		}
	}

	got, err := store.ListByUser(context.Background(), "u1", 2)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != "new" || got[1].ID != "mid" {
		t.Fatalf("order = [%s %s], want [new mid]", got[0].ID, got[1].ID)
	}
}

func TestMemorySessionStore_ListByUserCapsLimit(t *testing.T) {
	store := NewMemorySessionStore()
	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 105; i++ {
		sess := &domain.Session{
			ID:        fmt.Sprintf("s-%03d", i),
			UserID:    "u1",
			UpdatedAt: base.Add(time.Duration(i) * time.Second),
		}
		if err := store.Save(context.Background(), sess); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	got, err := store.ListByUser(context.Background(), "u1", 1000)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 100 {
		t.Fatalf("len = %d, want cap 100", len(got))
	}
}
