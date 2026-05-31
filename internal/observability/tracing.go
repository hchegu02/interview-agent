package observability

import (
	"context"
	"sync"
	"time"
)

// SpanEnd finishes a span. A nil error marks the span as successful.
type SpanEnd func(error)

// Tracer is the small tracing boundary used by graph callbacks.
type Tracer interface {
	Start(ctx context.Context, name string, attrs map[string]string) (context.Context, SpanEnd)
}

// NoopTracer preserves default runtime behavior when tracing is not configured.
type NoopTracer struct{}

func (NoopTracer) Start(ctx context.Context, _ string, _ map[string]string) (context.Context, SpanEnd) {
	if ctx == nil {
		ctx = context.Background()
	}
	return ctx, func(error) {}
}

// SpanRecord is one completed span recorded by RecordingTracer.
type SpanRecord struct {
	Name      string
	Attrs     map[string]string
	StartedAt time.Time
	EndedAt   time.Time
	Err       error
}

// RecordingTracer is an in-memory Tracer for tests and diagnostics.
type RecordingTracer struct {
	now func() time.Time

	mu    sync.Mutex
	spans []SpanRecord
}

func NewRecordingTracer() *RecordingTracer {
	return &RecordingTracer{now: time.Now}
}

func (t *RecordingTracer) Start(ctx context.Context, name string, attrs map[string]string) (context.Context, SpanEnd) {
	if ctx == nil {
		ctx = context.Background()
	}
	if t == nil {
		return NoopTracer{}.Start(ctx, name, attrs)
	}
	startedAt := t.now()
	copiedAttrs := cloneStringMap(attrs)

	var once sync.Once
	return ctx, func(err error) {
		once.Do(func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.spans = append(t.spans, SpanRecord{
				Name:      name,
				Attrs:     cloneStringMap(copiedAttrs),
				StartedAt: startedAt,
				EndedAt:   t.now(),
				Err:       err,
			})
		})
	}
}

func (t *RecordingTracer) Spans() []SpanRecord {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]SpanRecord, len(t.spans))
	for i, span := range t.spans {
		out[i] = span
		out[i].Attrs = cloneStringMap(span.Attrs)
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
