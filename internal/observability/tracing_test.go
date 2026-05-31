package observability

import (
	"context"
	"errors"
	"testing"
)

func TestNoopTracer_StartReturnsContextAndCallableEnd(t *testing.T) {
	ctx, end := NoopTracer{}.Start(context.Background(), "graph.node", map[string]string{
		"node": "pick_next",
	})
	if ctx == nil {
		t.Fatal("Start returned nil context")
	}
	if end == nil {
		t.Fatal("Start returned nil end func")
	}
	end(nil)
}

func TestRecordingTracer_RecordsSpanLifecycle(t *testing.T) {
	tracer := NewRecordingTracer()
	attrs := map[string]string{"node": "evaluate"}

	_, end := tracer.Start(context.Background(), "graph.node", attrs)
	attrs["node"] = "tampered"
	end(errors.New("boom"))

	spans := tracer.Spans()
	if len(spans) != 1 {
		t.Fatalf("Spans len = %d, want 1", len(spans))
	}
	if spans[0].Name != "graph.node" {
		t.Fatalf("Name = %q, want graph.node", spans[0].Name)
	}
	if spans[0].Attrs["node"] != "evaluate" {
		t.Fatalf("Attrs[node] = %q, want evaluate", spans[0].Attrs["node"])
	}
	if spans[0].Err == nil {
		t.Fatal("Err is nil, want non-nil")
	}

	spans[0].Attrs["node"] = "mutated"
	if got := tracer.Spans()[0].Attrs["node"]; got != "evaluate" {
		t.Fatalf("Spans returned shared attrs map, got %q", got)
	}
}
