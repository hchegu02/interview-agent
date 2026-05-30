package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// stubRecChatModel 是 recording 单测用 ChatModel，可注入返回值或错误。
// 名字加 rec 前缀避免与 breaker_test.go 中的 fakeChatModel 冲突。
type stubRecChatModel struct {
	resp      *Response
	err       error
	streamErr error
}

func (f *stubRecChatModel) Generate(ctx context.Context, messages []Message, opts Options) (*Response, error) {
	return f.resp, f.err
}
func (f *stubRecChatModel) Stream(ctx context.Context, messages []Message, opts Options) (<-chan Chunk, error) {
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	ch := make(chan Chunk, 1)
	ch <- Chunk{Done: true}
	close(ch)
	return ch, nil
}
func (f *stubRecChatModel) Name() string { return "stubrec" }

func TestRecordingChatModel_NilInnerReturnsNil(t *testing.T) {
	if m := NewRecordingChatModel(nil); m != nil {
		t.Fatalf("NewRecordingChatModel(nil) = %v, want nil", m)
	}
}

func TestRecordingChatModel_GenerateClosedPathRecordsOK(t *testing.T) {
	inner := &stubRecChatModel{
		resp: &Response{
			Content:          "hello",
			Model:            "test-model",
			PromptTokens:     12,
			CompletionTokens: 34,
		},
	}
	rec := NewRecordingChatModel(inner)
	// 注入虚拟时钟：start=t0, end=t0+50ms
	t0 := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	calls := 0
	rec.now = func() time.Time {
		t := t0.Add(time.Duration(calls) * 50 * time.Millisecond)
		calls++
		return t
	}

	resp, err := rec.Generate(context.Background(), nil, Options{})
	if err != nil {
		t.Fatalf("Generate err = %v", err)
	}
	if resp.Content != "hello" {
		t.Fatalf("Content = %q, want hello", resp.Content)
	}
	got := rec.Snapshot()
	if len(got) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(got))
	}
	r := got[0]
	if r.ErrClass != "ok" {
		t.Fatalf("ErrClass = %q, want ok", r.ErrClass)
	}
	if r.Model != "test-model" {
		t.Fatalf("Model = %q, want test-model", r.Model)
	}
	if r.PromptTokens != 12 || r.CompletionTokens != 34 {
		t.Fatalf("tokens = (%d,%d), want (12,34)", r.PromptTokens, r.CompletionTokens)
	}
	if r.Duration != 50*time.Millisecond {
		t.Fatalf("Duration = %v, want 50ms", r.Duration)
	}
	if !r.StartedAt.Equal(t0) {
		t.Fatalf("StartedAt = %v, want %v", r.StartedAt, t0)
	}
}

func TestRecordingChatModel_ObserverReceivesCallRecord(t *testing.T) {
	inner := &stubRecChatModel{
		resp: &Response{
			Model:            "test-model",
			PromptTokens:     10,
			CompletionTokens: 2,
		},
	}
	rec := NewRecordingChatModel(inner)
	var observed []CallRecord
	rec.SetObserver(func(record CallRecord) {
		observed = append(observed, record)
	})

	if _, err := rec.Generate(context.Background(), nil, Options{}); err != nil {
		t.Fatalf("Generate err = %v", err)
	}
	if len(observed) != 1 {
		t.Fatalf("observed len = %d, want 1", len(observed))
	}
	if observed[0].Model != "test-model" || observed[0].PromptTokens != 10 || observed[0].CompletionTokens != 2 {
		t.Fatalf("observed = %+v", observed[0])
	}
}

func TestRecordingChatModel_ClassifyErr(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantCls string
	}{
		{"nil_is_ok", nil, "ok"},
		{"breaker_open", ErrBreakerOpen, "breaker_open"},
		{"breaker_open_wrapped", fmt.Errorf("call: %w", ErrBreakerOpen), "breaker_open"},
		{"schema_invalid", fmt.Errorf("bad: %w", ErrSchemaInvalid), "schema_invalid"},
		{"permanent", fmt.Errorf("4xx: %w", ErrPermanent), "permanent"},
		{"transient_via_retry_exhausted",
			fmt.Errorf("retry exhausted after 3 attempts: %w",
				fmt.Errorf("%w: 500", ErrTransient)),
			"transient"},
		{"canceled", context.Canceled, "canceled"},
		{"deadline", context.DeadlineExceeded, "deadline"},
		{"other_unclassified", errors.New("mystery"), "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyChatErr(tt.err)
			if got != tt.wantCls {
				t.Fatalf("classifyChatErr(%v) = %q, want %q", tt.err, got, tt.wantCls)
			}
		})
	}
}

func TestRecordingChatModel_GenerateErrorPathsRecorded(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		wantCls string
	}{
		{"transient", fmt.Errorf("%w: 500", ErrTransient), "transient"},
		{"permanent", fmt.Errorf("%w: 400", ErrPermanent), "permanent"},
		{"breaker_open", ErrBreakerOpen, "breaker_open"},
		{"canceled", context.Canceled, "canceled"},
		{"schema_invalid", fmt.Errorf("%w: bad json", ErrSchemaInvalid), "schema_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inner := &stubRecChatModel{err: tc.err}
			rec := NewRecordingChatModel(inner)
			_, err := rec.Generate(context.Background(), nil, Options{})
			if err == nil {
				t.Fatal("expected error to propagate")
			}
			got := rec.Snapshot()
			if len(got) != 1 {
				t.Fatalf("Snapshot len = %d, want 1", len(got))
			}
			if got[0].ErrClass != tc.wantCls {
				t.Fatalf("ErrClass = %q, want %q", got[0].ErrClass, tc.wantCls)
			}
			if got[0].ErrMsg == "" {
				t.Fatalf("ErrMsg empty, want non-empty for err=%v", tc.err)
			}
		})
	}
}

func TestRecordingChatModel_StreamRecordsStartup(t *testing.T) {
	inner := &stubRecChatModel{}
	rec := NewRecordingChatModel(inner)
	ch, err := rec.Stream(context.Background(), nil, Options{})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}
	for range ch {
	}
	got := rec.Snapshot()
	if len(got) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(got))
	}
	if got[0].ErrClass != "ok" {
		t.Fatalf("ErrClass = %q, want ok", got[0].ErrClass)
	}
}

func TestRecordingChatModel_StreamStartupErrorRecorded(t *testing.T) {
	inner := &stubRecChatModel{streamErr: fmt.Errorf("%w: starting", ErrTransient)}
	rec := NewRecordingChatModel(inner)
	_, err := rec.Stream(context.Background(), nil, Options{})
	if err == nil {
		t.Fatal("expected stream startup error")
	}
	got := rec.Snapshot()
	if len(got) != 1 || got[0].ErrClass != "transient" {
		t.Fatalf("snapshot = %+v, want one transient record", got)
	}
}

func TestRecordingChatModel_ResetClears(t *testing.T) {
	inner := &stubRecChatModel{resp: &Response{Content: "x"}}
	rec := NewRecordingChatModel(inner)
	_, _ = rec.Generate(context.Background(), nil, Options{})
	_, _ = rec.Generate(context.Background(), nil, Options{})
	if got := rec.Snapshot(); len(got) != 2 {
		t.Fatalf("before Reset len = %d, want 2", len(got))
	}
	rec.Reset()
	if got := rec.Snapshot(); len(got) != 0 {
		t.Fatalf("after Reset len = %d, want 0", len(got))
	}
}

func TestRecordingChatModel_ConcurrentGenerateIsSafe(t *testing.T) {
	inner := &stubRecChatModel{resp: &Response{Content: "x", PromptTokens: 1, CompletionTokens: 1}}
	rec := NewRecordingChatModel(inner)

	const goroutines = 8
	const perG = 25
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				_, _ = rec.Generate(context.Background(), nil, Options{})
			}
		}()
	}
	wg.Wait()
	got := rec.Snapshot()
	if want := goroutines * perG; len(got) != want {
		t.Fatalf("Snapshot len = %d, want %d", len(got), want)
	}
}

func TestRecordingChatModel_SnapshotIsCopy(t *testing.T) {
	inner := &stubRecChatModel{resp: &Response{Content: "x"}}
	rec := NewRecordingChatModel(inner)
	_, _ = rec.Generate(context.Background(), nil, Options{})

	snap := rec.Snapshot()
	snap[0].ErrMsg = "tampered"

	got := rec.Snapshot()
	if got[0].ErrMsg == "tampered" {
		t.Fatal("Snapshot returned shared slice; mutating caller's copy leaked into internal state")
	}
}
