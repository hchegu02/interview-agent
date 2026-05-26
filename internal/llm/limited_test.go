package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

type blockingChatModel struct {
	entered chan struct{}
	release chan struct{}
}

func newBlockingChatModel() *blockingChatModel {
	return &blockingChatModel{
		entered: make(chan struct{}, 8),
		release: make(chan struct{}),
	}
}

func (m *blockingChatModel) Generate(ctx context.Context, messages []Message, opts Options) (*Response, error) {
	m.entered <- struct{}{}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-m.release:
		return &Response{Content: "ok", Model: "blocking"}, nil
	}
}

func (m *blockingChatModel) Stream(ctx context.Context, messages []Message, opts Options) (<-chan Chunk, error) {
	return nil, errors.New("not implemented")
}

func (m *blockingChatModel) Name() string { return "blocking" }

func TestLimitedChatModelLimitsConcurrentGenerate(t *testing.T) {
	inner := newBlockingChatModel()
	model := NewLimitedChatModel(inner, 1)

	firstDone := make(chan error, 1)
	go func() {
		_, err := model.Generate(context.Background(), []Message{{Role: "user", Content: "one"}}, Options{})
		firstDone <- err
	}()
	select {
	case <-inner.entered:
	case <-time.After(time.Second):
		t.Fatal("first call did not enter inner model")
	}

	secondDone := make(chan error, 1)
	go func() {
		_, err := model.Generate(context.Background(), []Message{{Role: "user", Content: "two"}}, Options{})
		secondDone <- err
	}()
	select {
	case <-inner.entered:
		t.Fatal("second call entered inner model before first released")
	case <-time.After(30 * time.Millisecond):
	}

	inner.release <- struct{}{}
	if err := <-firstDone; err != nil {
		t.Fatalf("first call: %v", err)
	}
	select {
	case <-inner.entered:
	case <-time.After(time.Second):
		t.Fatal("second call did not enter after first released")
	}
	inner.release <- struct{}{}
	if err := <-secondDone; err != nil {
		t.Fatalf("second call: %v", err)
	}
}

func TestLimitedChatModelRespectsContextWhileWaiting(t *testing.T) {
	inner := newBlockingChatModel()
	model := NewLimitedChatModel(inner, 1)

	firstDone := make(chan error, 1)
	go func() {
		_, err := model.Generate(context.Background(), []Message{{Role: "user", Content: "one"}}, Options{})
		firstDone <- err
	}()
	select {
	case <-inner.entered:
	case <-time.After(time.Second):
		t.Fatal("first call did not enter inner model")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := model.Generate(ctx, []Message{{Role: "user", Content: "two"}}, Options{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	select {
	case <-inner.entered:
		t.Fatal("timed-out call should not enter inner model")
	default:
	}

	inner.release <- struct{}{}
	if err := <-firstDone; err != nil {
		t.Fatalf("first call: %v", err)
	}
}
