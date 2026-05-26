package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeChatModel 是熔断器单测用的可编程内层。
// 通过 results 顺序返回，便于驱动 closed→open→halfOpen 各路径。
type fakeChatModel struct {
	mu       sync.Mutex
	results  []error
	calls    int
	inflight int32
	maxInflight int32
}

func newFakeChatModel(errs ...error) *fakeChatModel {
	return &fakeChatModel{results: errs}
}

func (f *fakeChatModel) Generate(ctx context.Context, messages []Message, opts Options) (*Response, error) {
	atomic.AddInt32(&f.inflight, 1)
	defer atomic.AddInt32(&f.inflight, -1)
	if v := atomic.LoadInt32(&f.inflight); v > atomic.LoadInt32(&f.maxInflight) {
		atomic.StoreInt32(&f.maxInflight, v)
	}

	f.mu.Lock()
	f.calls++
	var err error
	if len(f.results) > 0 {
		err = f.results[0]
		f.results = f.results[1:]
	}
	f.mu.Unlock()

	if err != nil {
		return nil, err
	}
	return &Response{Content: "ok", Model: "fake"}, nil
}

func (f *fakeChatModel) Stream(ctx context.Context, messages []Message, opts Options) (<-chan Chunk, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeChatModel) Name() string { return "fake" }

func TestBreaker_NilInnerReturnsNil(t *testing.T) {
	if got := NewBreakingChatModel(nil, 3, time.Second); got != nil {
		t.Fatalf("got %v, want nil for nil inner", got)
	}
}

func TestBreaker_ZeroThresholdSkipsWrapping(t *testing.T) {
	inner := newFakeChatModel()
	if got := NewBreakingChatModel(inner, 0, time.Second); got != ChatModel(inner) {
		t.Fatalf("threshold=0 should bypass and return inner, got %v", got)
	}
	if got := NewBreakingChatModel(inner, 3, 0); got != ChatModel(inner) {
		t.Fatalf("openDuration=0 should bypass and return inner, got %v", got)
	}
}

func TestBreaker_ClosedAllowsThroughOnSuccess(t *testing.T) {
	inner := newFakeChatModel(nil, nil, nil)
	b := NewBreakingChatModel(inner, 3, time.Second)
	for i := 0; i < 3; i++ {
		if _, err := b.Generate(context.Background(), nil, Options{}); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := b.(*BreakingChatModel).State(); got != "closed" {
		t.Fatalf("state = %q, want closed", got)
	}
}

func TestBreaker_OpensAfterThresholdTransientFailures(t *testing.T) {
	transient := fmt.Errorf("%w: simulated", ErrTransient)
	inner := newFakeChatModel(transient, transient, transient)
	b := NewBreakingChatModel(inner, 3, time.Second).(*BreakingChatModel)

	for i := 0; i < 3; i++ {
		_, err := b.Generate(context.Background(), nil, Options{})
		if !errors.Is(err, ErrTransient) {
			t.Fatalf("call %d err = %v, want ErrTransient", i, err)
		}
	}
	if got := b.State(); got != "open" {
		t.Fatalf("state = %q, want open after %d transient failures", got, 3)
	}

	// 进入 open 后立即再调一次，应该 fail-fast。
	_, err := b.Generate(context.Background(), nil, Options{})
	if !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("err = %v, want ErrBreakerOpen", err)
	}
	// 没有调到底层
	inner.mu.Lock()
	if inner.calls != 3 {
		t.Fatalf("inner.calls = %d, want 3 (fail-fast must not reach inner)", inner.calls)
	}
	inner.mu.Unlock()
}

func TestBreaker_DeadlineExceededCountsAsFailure(t *testing.T) {
	inner := newFakeChatModel(context.DeadlineExceeded, context.DeadlineExceeded)
	b := NewBreakingChatModel(inner, 2, time.Second).(*BreakingChatModel)
	for i := 0; i < 2; i++ {
		_, err := b.Generate(context.Background(), nil, Options{})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("call %d err = %v", i, err)
		}
	}
	if got := b.State(); got != "open" {
		t.Fatalf("state = %q, want open", got)
	}
}

func TestBreaker_PermanentDoesNotCount(t *testing.T) {
	perm := fmt.Errorf("%w: bad config", ErrPermanent)
	inner := newFakeChatModel(perm, perm, perm, perm, perm)
	b := NewBreakingChatModel(inner, 3, time.Second).(*BreakingChatModel)
	for i := 0; i < 5; i++ {
		_, err := b.Generate(context.Background(), nil, Options{})
		if !errors.Is(err, ErrPermanent) {
			t.Fatalf("call %d err = %v", i, err)
		}
	}
	if got := b.State(); got != "closed" {
		t.Fatalf("state = %q, want closed (ErrPermanent must not count)", got)
	}
}

func TestBreaker_SchemaInvalidDoesNotCount(t *testing.T) {
	schemaErr := fmt.Errorf("%w: bad json", ErrSchemaInvalid)
	inner := newFakeChatModel(schemaErr, schemaErr, schemaErr, schemaErr)
	b := NewBreakingChatModel(inner, 2, time.Second).(*BreakingChatModel)
	for i := 0; i < 4; i++ {
		_, _ = b.Generate(context.Background(), nil, Options{})
	}
	if got := b.State(); got != "closed" {
		t.Fatalf("state = %q, want closed (ErrSchemaInvalid must not count)", got)
	}
}

func TestBreaker_CancelDoesNotCount(t *testing.T) {
	inner := newFakeChatModel(context.Canceled, context.Canceled, context.Canceled)
	b := NewBreakingChatModel(inner, 2, time.Second).(*BreakingChatModel)
	for i := 0; i < 3; i++ {
		_, _ = b.Generate(context.Background(), nil, Options{})
	}
	if got := b.State(); got != "closed" {
		t.Fatalf("state = %q, want closed", got)
	}
}

func TestBreaker_RetryExhaustedWrapsErrTransient(t *testing.T) {
	// 模拟 RealChatModel.Generate 的最终错误形态：
	// `retry exhausted after %d attempts: %w`，其中 %w 是 ErrTransient。
	wrapped := fmt.Errorf("retry exhausted after 3 attempts: %w", fmt.Errorf("%w: 500 server error", ErrTransient))
	inner := newFakeChatModel(wrapped, wrapped, wrapped)
	b := NewBreakingChatModel(inner, 3, time.Second).(*BreakingChatModel)
	for i := 0; i < 3; i++ {
		_, err := b.Generate(context.Background(), nil, Options{})
		if !errors.Is(err, ErrTransient) {
			t.Fatalf("call %d: err = %v, want errors.Is(err, ErrTransient)", i, err)
		}
	}
	if got := b.State(); got != "open" {
		t.Fatalf("state = %q, want open (retry-wrapped ErrTransient must count)", got)
	}
}

func TestBreaker_HalfOpenProbeSuccessClosesBreaker(t *testing.T) {
	transient := fmt.Errorf("%w: x", ErrTransient)
	// 前两次失败把熔断打开；第三次（probe）成功 → closed。
	inner := newFakeChatModel(transient, transient, nil, nil)
	b := NewBreakingChatModel(inner, 2, 50*time.Millisecond).(*BreakingChatModel)

	// 注入虚拟时钟
	var clock atomic.Int64
	clock.Store(int64(time.Now().UnixNano()))
	b.now = func() time.Time { return time.Unix(0, clock.Load()) }

	// 触发 open
	for i := 0; i < 2; i++ {
		_, _ = b.Generate(context.Background(), nil, Options{})
	}
	if got := b.State(); got != "open" {
		t.Fatalf("state = %q, want open", got)
	}

	// 时钟前进超过 openDuration，下一次调用应成为 probe
	clock.Add(int64(100 * time.Millisecond))
	if _, err := b.Generate(context.Background(), nil, Options{}); err != nil {
		t.Fatalf("probe call: %v", err)
	}
	if got := b.State(); got != "closed" {
		t.Fatalf("state = %q, want closed after successful probe", got)
	}
	// 熔断器闭合后正常调用仍 OK
	if _, err := b.Generate(context.Background(), nil, Options{}); err != nil {
		t.Fatalf("post-recovery call: %v", err)
	}
}

func TestBreaker_HalfOpenProbeFailureReturnsToOpen(t *testing.T) {
	transient := fmt.Errorf("%w: x", ErrTransient)
	// 前两次失败打开，第三次（probe）还失败 → 回到 open，重置计时。
	inner := newFakeChatModel(transient, transient, transient)
	b := NewBreakingChatModel(inner, 2, 50*time.Millisecond).(*BreakingChatModel)

	var clock atomic.Int64
	clock.Store(int64(time.Now().UnixNano()))
	b.now = func() time.Time { return time.Unix(0, clock.Load()) }

	for i := 0; i < 2; i++ {
		_, _ = b.Generate(context.Background(), nil, Options{})
	}
	if got := b.State(); got != "open" {
		t.Fatalf("state = %q, want open", got)
	}

	// 时钟前进超 openDuration，发出 probe
	clock.Add(int64(100 * time.Millisecond))
	_, err := b.Generate(context.Background(), nil, Options{})
	if !errors.Is(err, ErrTransient) {
		t.Fatalf("probe err = %v, want ErrTransient", err)
	}
	if got := b.State(); got != "open" {
		t.Fatalf("state = %q, want open after probe failure", got)
	}

	// 紧接着再调用应当继续 fail-fast，因为 openedAt 被重置
	_, err = b.Generate(context.Background(), nil, Options{})
	if !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("post-failed-probe err = %v, want ErrBreakerOpen", err)
	}
}

func TestBreaker_HalfOpenAllowsOnlyOneProbe(t *testing.T) {
	// 多 goroutine 并发尝试 probe，只能有 1 个进入底层；其余必须收到 ErrBreakerOpen。
	transient := fmt.Errorf("%w: x", ErrTransient)
	// 先 2 次 transient 打开，再为可能的 probe 安排一个慢成功。
	inner := &slowFakeModel{
		first:    []error{transient, transient},
		probeOK:  true,
		barrier:  make(chan struct{}),
	}
	b := NewBreakingChatModel(inner, 2, 10*time.Millisecond).(*BreakingChatModel)

	var clock atomic.Int64
	clock.Store(int64(time.Now().UnixNano()))
	b.now = func() time.Time { return time.Unix(0, clock.Load()) }

	for i := 0; i < 2; i++ {
		_, _ = b.Generate(context.Background(), nil, Options{})
	}
	if got := b.State(); got != "open" {
		t.Fatalf("state = %q, want open", got)
	}

	clock.Add(int64(50 * time.Millisecond))

	const N = 8
	results := make(chan error, N)
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := b.Generate(context.Background(), nil, Options{})
			results <- err
		}()
	}

	// 给 goroutine 进入 tryAcquire 的机会
	time.Sleep(10 * time.Millisecond)
	close(inner.barrier) // 放行 probe

	wg.Wait()
	close(results)

	var failFast, succeeded int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrBreakerOpen):
			failFast++
		default:
			t.Fatalf("unexpected err: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("succeeded probes = %d, want exactly 1", succeeded)
	}
	if failFast != N-1 {
		t.Fatalf("fail-fast = %d, want %d", failFast, N-1)
	}
}

// slowFakeModel 用 barrier 控制 probe 阻塞时机，
// 让"并发只有 1 个 probe"的语义可观测。
type slowFakeModel struct {
	mu      sync.Mutex
	first   []error // 用完前按顺序返回
	probeOK bool
	barrier chan struct{}
}

func (s *slowFakeModel) Generate(ctx context.Context, messages []Message, opts Options) (*Response, error) {
	s.mu.Lock()
	if len(s.first) > 0 {
		err := s.first[0]
		s.first = s.first[1:]
		s.mu.Unlock()
		if err != nil {
			return nil, err
		}
		return &Response{Content: "ok"}, nil
	}
	s.mu.Unlock()

	// probe path：等 barrier 放开后再回应
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.barrier:
	}
	if s.probeOK {
		return &Response{Content: "probe-ok"}, nil
	}
	return nil, fmt.Errorf("%w: probe fail", ErrTransient)
}

func (s *slowFakeModel) Stream(ctx context.Context, messages []Message, opts Options) (<-chan Chunk, error) {
	return nil, errors.New("not implemented")
}

func (s *slowFakeModel) Name() string { return "slow-fake" }

func TestBreaker_SuccessResetsFailureCounter(t *testing.T) {
	transient := fmt.Errorf("%w: x", ErrTransient)
	// 失败、失败、成功、失败、失败 → 总共连续失败计数最多是 2（threshold=3 不该开）。
	inner := newFakeChatModel(transient, transient, nil, transient, transient)
	b := NewBreakingChatModel(inner, 3, time.Second).(*BreakingChatModel)
	for i := 0; i < 5; i++ {
		_, _ = b.Generate(context.Background(), nil, Options{})
	}
	if got := b.State(); got != "closed" {
		t.Fatalf("state = %q, want closed (success between failures should reset counter)", got)
	}
}
