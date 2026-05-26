package llm

import (
	"context"
	"fmt"
)

type LimitedChatModel struct {
	inner ChatModel
	sem   chan struct{}
}

func NewLimitedChatModel(inner ChatModel, maxConcurrent int) ChatModel {
	if inner == nil || maxConcurrent <= 0 {
		return inner
	}
	return &LimitedChatModel{
		inner: inner,
		sem:   make(chan struct{}, maxConcurrent),
	}
}

func (m *LimitedChatModel) Generate(ctx context.Context, messages []Message, opts Options) (*Response, error) {
	if err := m.acquire(ctx); err != nil {
		return nil, err
	}
	defer m.release()
	return m.inner.Generate(ctx, messages, opts)
}

func (m *LimitedChatModel) Stream(ctx context.Context, messages []Message, opts Options) (<-chan Chunk, error) {
	if err := m.acquire(ctx); err != nil {
		return nil, err
	}
	ch, err := m.inner.Stream(ctx, messages, opts)
	if err != nil {
		m.release()
		return nil, err
	}
	out := make(chan Chunk)
	go func() {
		defer m.release()
		defer close(out)
		for chunk := range ch {
			select {
			case <-ctx.Done():
				out <- Chunk{Err: ctx.Err(), Done: true}
				return
			case out <- chunk:
			}
		}
	}()
	return out, nil
}

func (m *LimitedChatModel) Name() string {
	return m.inner.Name()
}

func (m *LimitedChatModel) acquire(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case m.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("llm concurrency limit wait: %w", ctx.Err())
	}
}

func (m *LimitedChatModel) release() {
	<-m.sem
}
