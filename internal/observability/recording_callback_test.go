package observability

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
	"interview-agent/internal/llm"
)

func TestRecordingCallback_StartEndPairs(t *testing.T) {
	cb := NewRecordingCallback()
	// 注入虚拟时钟：起步 t0, end 在 t0+30ms
	t0 := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	calls := 0
	cb.now = func() time.Time {
		t := t0.Add(time.Duration(calls) * 30 * time.Millisecond)
		calls++
		return t
	}
	sess := &domain.Session{ID: "s1"}

	cb.OnNodeStart(context.Background(), "pick_next", sess)
	cb.OnNodeEnd(context.Background(), "pick_next", sess)

	got := cb.Snapshot()
	if len(got) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(got))
	}
	r := got[0]
	if r.Node != "pick_next" {
		t.Fatalf("Node = %q, want pick_next", r.Node)
	}
	if r.ErrClass != "ok" {
		t.Fatalf("ErrClass = %q, want ok", r.ErrClass)
	}
	if r.Duration != 30*time.Millisecond {
		t.Fatalf("Duration = %v, want 30ms", r.Duration)
	}
}

func TestRecordingCallback_ErrorPath(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		wantCls string
	}{
		{"suspended", fmt.Errorf("wait answer: %w", graph.ErrSuspended), "suspended"},
		{"graph_permanent", fmt.Errorf("bad: %w", graph.ErrPermanent), "permanent"},
		{"llm_transient", fmt.Errorf("retry exhausted: %w", fmt.Errorf("%w: 500", llm.ErrTransient)), "transient"},
		{"llm_schema_invalid", fmt.Errorf("%w: bad", llm.ErrSchemaInvalid), "schema_invalid"},
		{"llm_breaker_open", llm.ErrBreakerOpen, "breaker_open"},
		{"unclassified", errors.New("unknown"), "other"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cb := NewRecordingCallback()
			sess := &domain.Session{ID: "s1"}
			cb.OnNodeStart(context.Background(), "evaluate", sess)
			cb.OnNodeError(context.Background(), "evaluate", sess, tc.err)
			got := cb.Snapshot()
			if len(got) != 1 {
				t.Fatalf("Snapshot len = %d, want 1", len(got))
			}
			if got[0].ErrClass != tc.wantCls {
				t.Fatalf("ErrClass = %q, want %q", got[0].ErrClass, tc.wantCls)
			}
			if got[0].ErrMsg == "" {
				t.Fatalf("ErrMsg empty for err=%v", tc.err)
			}
		})
	}
}

func TestRecordingCallback_LoopMultipleStartEnd(t *testing.T) {
	cb := NewRecordingCallback()
	sess := &domain.Session{ID: "s1"}
	// agent loop 中同一节点 pick_next 会被多次访问
	for i := 0; i < 3; i++ {
		cb.OnNodeStart(context.Background(), "pick_next", sess)
		cb.OnNodeEnd(context.Background(), "pick_next", sess)
	}
	got := cb.Snapshot()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, r := range got {
		if r.Node != "pick_next" {
			t.Fatalf("got[%d].Node = %q, want pick_next", i, r.Node)
		}
		if r.ErrClass != "ok" {
			t.Fatalf("got[%d].ErrClass = %q, want ok", i, r.ErrClass)
		}
	}
}

func TestRecordingCallback_ResetClears(t *testing.T) {
	cb := NewRecordingCallback()
	sess := &domain.Session{ID: "s1"}
	cb.OnNodeStart(context.Background(), "a", sess)
	cb.OnNodeEnd(context.Background(), "a", sess)
	if len(cb.Snapshot()) != 1 {
		t.Fatalf("len = %d, want 1", len(cb.Snapshot()))
	}
	cb.Reset()
	if len(cb.Snapshot()) != 0 {
		t.Fatalf("len after Reset = %d, want 0", len(cb.Snapshot()))
	}
	// 重置后还能继续工作
	cb.OnNodeStart(context.Background(), "b", sess)
	cb.OnNodeEnd(context.Background(), "b", sess)
	if len(cb.Snapshot()) != 1 {
		t.Fatal("post-reset Start/End not paired")
	}
}

func TestRecordingCallback_SnapshotIsCopy(t *testing.T) {
	cb := NewRecordingCallback()
	sess := &domain.Session{ID: "s1"}
	cb.OnNodeStart(context.Background(), "a", sess)
	cb.OnNodeEnd(context.Background(), "a", sess)

	snap := cb.Snapshot()
	snap[0].ErrMsg = "tampered"

	got := cb.Snapshot()
	if got[0].ErrMsg == "tampered" {
		t.Fatal("Snapshot returned shared slice")
	}
}

// TestRecordingCallback_UnmatchedEndStillRecords 验证防御性写法：
// 即便 OnNodeEnd 没有对应的 OnNodeStart，也写一条记录而不是丢失。
func TestRecordingCallback_UnmatchedEndStillRecords(t *testing.T) {
	cb := NewRecordingCallback()
	sess := &domain.Session{ID: "s1"}
	cb.OnNodeEnd(context.Background(), "stray", sess)
	got := cb.Snapshot()
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Node != "stray" {
		t.Fatalf("Node = %q, want stray", got[0].Node)
	}
}
