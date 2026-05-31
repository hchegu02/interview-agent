package observability

import (
	"context"
	"errors"
	"testing"

	"interview-agent/internal/domain"
)

func TestTracingGraphCallback_RecordsNodeSpan(t *testing.T) {
	tracer := NewRecordingTracer()
	cb := NewTracingGraphCallback(tracer)
	sess := &domain.Session{ID: "sess-1"}

	cb.OnNodeStart(context.Background(), "pick_next", sess)
	cb.OnNodeEnd(context.Background(), "pick_next", sess)

	spans := tracer.Spans()
	if len(spans) != 1 {
		t.Fatalf("Spans len = %d, want 1", len(spans))
	}
	if spans[0].Name != "graph.node" {
		t.Fatalf("Name = %q, want graph.node", spans[0].Name)
	}
	if spans[0].Attrs["node"] != "pick_next" {
		t.Fatalf("Attrs[node] = %q, want pick_next", spans[0].Attrs["node"])
	}
	if spans[0].Attrs["session_id"] != "sess-1" {
		t.Fatalf("Attrs[session_id] = %q, want sess-1", spans[0].Attrs["session_id"])
	}
	if spans[0].Err != nil {
		t.Fatalf("Err = %v, want nil", spans[0].Err)
	}
}

func TestTracingGraphCallback_RecordsNodeError(t *testing.T) {
	tracer := NewRecordingTracer()
	cb := NewTracingGraphCallback(tracer)
	sess := &domain.Session{ID: "sess-err"}
	err := errors.New("boom")

	cb.OnNodeStart(context.Background(), "evaluate", sess)
	cb.OnNodeError(context.Background(), "evaluate", sess, err)

	spans := tracer.Spans()
	if len(spans) != 1 {
		t.Fatalf("Spans len = %d, want 1", len(spans))
	}
	if spans[0].Err == nil {
		t.Fatal("Err is nil, want non-nil")
	}
	if spans[0].Err.Error() != "boom" {
		t.Fatalf("Err = %v, want boom", spans[0].Err)
	}
}

func TestTracingGraphCallback_SeparatesInterleavedSessionsOnSameNode(t *testing.T) {
	tracer := NewRecordingTracer()
	cb := NewTracingGraphCallback(tracer)
	sessA := &domain.Session{ID: "sess-a"}
	sessB := &domain.Session{ID: "sess-b"}

	cb.OnNodeStart(context.Background(), "evaluate", sessA)
	cb.OnNodeStart(context.Background(), "evaluate", sessB)
	cb.OnNodeEnd(context.Background(), "evaluate", sessA)
	cb.OnNodeError(context.Background(), "evaluate", sessB, errors.New("boom"))

	spans := tracer.Spans()
	if len(spans) != 2 {
		t.Fatalf("Spans len = %d, want 2", len(spans))
	}

	bySession := map[string]SpanRecord{}
	for _, span := range spans {
		if span.Name != "graph.node" {
			t.Fatalf("Name = %q, want graph.node", span.Name)
		}
		if span.Attrs["node"] != "evaluate" {
			t.Fatalf("Attrs[node] = %q, want evaluate", span.Attrs["node"])
		}
		bySession[span.Attrs["session_id"]] = span
	}
	if bySession["sess-a"].Err != nil {
		t.Fatalf("sess-a Err = %v, want nil", bySession["sess-a"].Err)
	}
	if bySession["sess-b"].Err == nil || bySession["sess-b"].Err.Error() != "boom" {
		t.Fatalf("sess-b Err = %v, want boom", bySession["sess-b"].Err)
	}
}
