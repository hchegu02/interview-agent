package agentkit

import (
	"context"
	"testing"
	"time"
)

func TestRecorderHookStoresCopies(t *testing.T) {
	rec := NewRecorderHook()
	ev := HookEvent{
		Type:          HookAfterSkill,
		TraceID:       "trace-1",
		SessionID:     "session-1",
		Name:          "question.retrieve",
		InputSummary:  "input",
		OutputSummary: "output",
		Latency:       5 * time.Millisecond,
		Permission:    PermissionReadOnly,
	}
	if err := rec.HandleHook(context.Background(), ev); err != nil {
		t.Fatalf("handle hook: %v", err)
	}
	events := rec.Events()
	if len(events) != 1 || events[0].Name != "question.retrieve" {
		t.Fatalf("events = %+v", events)
	}
	events[0].Name = "mutated"
	if rec.Events()[0].Name != "question.retrieve" {
		t.Fatal("recorder returned mutable backing storage")
	}
}

func TestNoopHook(t *testing.T) {
	var h Hook = NoopHook{}
	if err := h.HandleHook(context.Background(), HookEvent{Type: HookBeforeSkill}); err != nil {
		t.Fatalf("noop hook should not fail: %v", err)
	}
}
