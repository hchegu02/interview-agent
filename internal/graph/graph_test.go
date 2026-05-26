package graph

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"interview-agent/internal/domain"
)

// nopNode 是一个简单的"已访问"标记节点构造器。
// 测试中大量用它构造"节点 X 被执行时往 trace 里追加 X"。
func recorder(trace *[]string, mu *sync.Mutex, name string) NodeFunc {
	return func(ctx context.Context, sess *domain.Session) error {
		mu.Lock()
		*trace = append(*trace, name)
		mu.Unlock()
		return nil
	}
}

// =============== 1. 线性 ===============

func TestGraph_Linear(t *testing.T) {
	var trace []string
	var mu sync.Mutex

	g := New("linear").
		AddNode("a", recorder(&trace, &mu, "a")).
		AddNode("b", recorder(&trace, &mu, "b")).
		AddNode("c", recorder(&trace, &mu, "c")).
		Entry("a").
		AddEdge("a", "b").
		AddEdge("b", "c").
		AddEdge("c", EndNode)

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := r.Invoke(context.Background(), &domain.Session{}); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	got := fmt.Sprint(trace)
	if got != "[a b c]" {
		t.Errorf("trace = %s, want [a b c]", got)
	}
}

func TestGraph_InvokeMigratesLegacySessionState(t *testing.T) {
	g := New("migrate").
		AddNode("a", func(ctx context.Context, sess *domain.Session) error {
			if sess.WorkingMemory.ReflectTopic != "redis" {
				t.Errorf("ReflectTopic = %q, want redis", sess.WorkingMemory.ReflectTopic)
			}
			if sess.WorkingMemory.DegradedReasons["eval"] != "llm timeout" {
				t.Errorf("eval degraded reason = %q", sess.WorkingMemory.DegradedReasons["eval"])
			}
			return nil
		}).
		Entry("a").
		AddEdge("a", EndNode)
	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	sess := &domain.Session{WorkingMemory: &domain.WorkingMemory{Notes: map[string]string{
		"reflect_topic":        "redis",
		"eval_degraded_reason": "llm timeout",
	}}}
	if err := r.Invoke(context.Background(), sess); err != nil {
		t.Fatalf("invoke: %v", err)
	}
}

// =============== 2. 并发 fan-out / fan-in ===============

func TestGraph_ParallelFanOut(t *testing.T) {
	var trace []string
	var mu sync.Mutex

	// __START__ → {a, b} → c
	g := New("parallel").
		AddNode("__START__", recorder(&trace, &mu, "start")).
		AddNode("a", recorder(&trace, &mu, "a")).
		AddNode("b", recorder(&trace, &mu, "b")).
		AddNode("c", recorder(&trace, &mu, "c")).
		Entry("__START__").
		AddEdge("__START__", "a").
		AddEdge("__START__", "b").
		AddEdge("a", "c").
		AddEdge("b", "c").
		AddEdge("c", EndNode)

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := r.Invoke(context.Background(), &domain.Session{}); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	// start 第一，c 最后，a 和 b 顺序不定但都在中间
	if len(trace) != 4 || trace[0] != "start" || trace[3] != "c" {
		t.Errorf("unexpected trace order: %v", trace)
	}
	gotA, gotB := false, false
	for _, n := range trace[1:3] {
		if n == "a" {
			gotA = true
		}
		if n == "b" {
			gotB = true
		}
	}
	if !gotA || !gotB {
		t.Errorf("expected both a and b in middle, got %v", trace)
	}
}

// 验证 c 只执行一次（fan-in 不重复触发）
func TestGraph_FanInDedup(t *testing.T) {
	var cCount int32

	g := New("fanin").
		AddNode("__START__", func(ctx context.Context, s *domain.Session) error { return nil }).
		AddNode("a", func(ctx context.Context, s *domain.Session) error { return nil }).
		AddNode("b", func(ctx context.Context, s *domain.Session) error { return nil }).
		AddNode("c", func(ctx context.Context, s *domain.Session) error {
			atomic.AddInt32(&cCount, 1)
			return nil
		}).
		Entry("__START__").
		AddEdge("__START__", "a").
		AddEdge("__START__", "b").
		AddEdge("a", "c").
		AddEdge("b", "c").
		AddEdge("c", EndNode)

	r, _ := g.Compile()
	if err := r.Invoke(context.Background(), &domain.Session{}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&cCount) != 1 {
		t.Errorf("c executed %d times, want 1", cCount)
	}
}

// =============== 3. 条件分支 ===============

func TestGraph_Branch(t *testing.T) {
	var trace []string
	var mu sync.Mutex

	// a → router → b or c
	g := New("branch").
		AddNode("a", recorder(&trace, &mu, "a")).
		AddNode("b", recorder(&trace, &mu, "b")).
		AddNode("c", recorder(&trace, &mu, "c")).
		AddNode("end", recorder(&trace, &mu, "end")).
		Entry("a").
		AddBranch("a", func(s *domain.Session) string {
			if s.UserID == "go-b" {
				return "b"
			}
			return "c"
		}).
		AddEdge("b", "end").
		AddEdge("c", "end").
		AddEdge("end", EndNode)

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	// 走 b 分支
	trace = nil
	if err := r.Invoke(context.Background(), &domain.Session{UserID: "go-b"}); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(trace) != "[a b end]" {
		t.Errorf("branch b: %v", trace)
	}

	// 走 c 分支
	trace = nil
	if err := r.Invoke(context.Background(), &domain.Session{UserID: "other"}); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(trace) != "[a c end]" {
		t.Errorf("branch c: %v", trace)
	}
}

// =============== 4. 循环 ===============

func TestGraph_Loop(t *testing.T) {
	var loopCount int32

	// loop_node 自循环 3 次后跳出
	g := New("loop").
		AddNode("loop_node", func(ctx context.Context, s *domain.Session) error {
			atomic.AddInt32(&loopCount, 1)
			return nil
		}).
		AddNode("end", func(ctx context.Context, s *domain.Session) error { return nil }).
		Entry("loop_node").
		AddBranch("loop_node", func(s *domain.Session) string {
			if atomic.LoadInt32(&loopCount) < 3 {
				return "loop_node"
			}
			return "end"
		}).
		AddEdge("end", EndNode)

	r, _ := g.Compile()
	if err := r.Invoke(context.Background(), &domain.Session{}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&loopCount) != 3 {
		t.Errorf("looped %d times, want 3", loopCount)
	}
}

func TestGraph_MaxStepsProtect(t *testing.T) {
	// 一个永远自循环的节点，必须被 MaxSteps 兜底
	g := New("infinite").
		AddNode("forever", func(ctx context.Context, s *domain.Session) error { return nil }).
		Entry("forever").
		AddBranch("forever", func(s *domain.Session) string { return "forever" }).
		MaxSteps(5)

	r, _ := g.Compile()
	err := r.Invoke(context.Background(), &domain.Session{})
	if !errors.Is(err, ErrMaxStepsExceeded) {
		t.Fatalf("want ErrMaxStepsExceeded, got %v", err)
	}
}

// =============== 5. Compile 校验 ===============

func TestCompile_Errors(t *testing.T) {
	t.Run("no entry", func(t *testing.T) {
		_, err := New("t").AddNode("a", nil).Compile()
		if !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("want ErrInvalidConfig, got %v", err)
		}
	})

	t.Run("entry not registered", func(t *testing.T) {
		_, err := New("t").Entry("missing").Compile()
		if !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("want ErrInvalidConfig, got %v", err)
		}
	})

	t.Run("edge to undefined", func(t *testing.T) {
		nop := func(ctx context.Context, s *domain.Session) error { return nil }
		_, err := New("t").AddNode("a", nop).Entry("a").AddEdge("a", "ghost").Compile()
		if !errors.Is(err, ErrNodeNotFound) {
			t.Errorf("want ErrNodeNotFound, got %v", err)
		}
	})

	t.Run("router and edge conflict", func(t *testing.T) {
		nop := func(ctx context.Context, s *domain.Session) error { return nil }
		_, err := New("t").
			AddNode("a", nop).AddNode("b", nop).
			Entry("a").
			AddEdge("a", "b").
			AddBranch("a", func(s *domain.Session) string { return "b" }).
			Compile()
		if !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("want ErrInvalidConfig, got %v", err)
		}
	})
}

// =============== 6. Callback ===============

type spyCallback struct {
	mu     sync.Mutex
	starts []string
	ends   []string
	errs   []string
}

func (s *spyCallback) OnNodeStart(ctx context.Context, name string, sess *domain.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.starts = append(s.starts, name)
}
func (s *spyCallback) OnNodeEnd(ctx context.Context, name string, sess *domain.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ends = append(s.ends, name)
}
func (s *spyCallback) OnNodeError(ctx context.Context, name string, sess *domain.Session, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errs = append(s.errs, name+":"+err.Error())
}

func TestGraph_Callback(t *testing.T) {
	spy := &spyCallback{}
	failingNode := func(ctx context.Context, s *domain.Session) error {
		return errors.New("oops")
	}

	g := New("cb").
		AddNode("ok", func(ctx context.Context, s *domain.Session) error { return nil }).
		AddNode("fail", failingNode).
		Entry("ok").
		AddEdge("ok", "fail").
		AddEdge("fail", EndNode).
		WithCallbacks(spy)

	r, _ := g.Compile()
	err := r.Invoke(context.Background(), &domain.Session{})
	if err == nil {
		t.Fatal("expected error from failing node")
	}
	if len(spy.starts) != 2 || spy.starts[0] != "ok" || spy.starts[1] != "fail" {
		t.Errorf("starts = %v", spy.starts)
	}
	if len(spy.ends) != 1 || spy.ends[0] != "ok" {
		t.Errorf("ends = %v (only ok should succeed)", spy.ends)
	}
	if len(spy.errs) != 1 {
		t.Errorf("errs = %v", spy.errs)
	}
}

// =============== 7. 装饰器：Retry + Timeout ===============

func TestWithRetry_Recovers(t *testing.T) {
	var attempts int32
	flaky := func(ctx context.Context, s *domain.Session) error {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			return errors.New("transient")
		}
		return nil
	}

	decorated := WithRetry(RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   1 * time.Millisecond,
		JitterRatio: 0,
	})(flaky)

	if err := decorated(context.Background(), &domain.Session{}); err != nil {
		t.Fatalf("should recover on 3rd attempt, got %v", err)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestWithRetry_PermanentSkipped(t *testing.T) {
	var attempts int32
	permFail := func(ctx context.Context, s *domain.Session) error {
		atomic.AddInt32(&attempts, 1)
		return Permanent(errors.New("malformed input"))
	}

	decorated := WithRetry(RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   1 * time.Millisecond,
	})(permFail)

	err := decorated(context.Background(), &domain.Session{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrPermanent) {
		t.Errorf("error should wrap ErrPermanent: %v", err)
	}
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("permanent error must not retry; attempts = %d", attempts)
	}
}

func TestWithRetry_Exhausted(t *testing.T) {
	alwaysFail := func(ctx context.Context, s *domain.Session) error {
		return errors.New("nope")
	}

	decorated := WithRetry(RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond,
	})(alwaysFail)

	err := decorated(context.Background(), &domain.Session{})
	if err == nil {
		t.Fatal("expected exhaustion error")
	}
	if !errors.Is(err, errors.Unwrap(err)) && err.Error() == "" {
		t.Errorf("unexpected: %v", err)
	}
}

func TestWithTimeout_Triggers(t *testing.T) {
	slow := func(ctx context.Context, s *domain.Session) error {
		select {
		case <-time.After(100 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	decorated := WithTimeout(10 * time.Millisecond)(slow)
	err := decorated(context.Background(), &domain.Session{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want DeadlineExceeded, got %v", err)
	}
}

func TestCompose_OrderRetryThenTimeout(t *testing.T) {
	// Compose(WithRetry, WithTimeout): retry 外层 → 每次重试都启动新 timeout
	var attempts int32
	slow := func(ctx context.Context, s *domain.Session) error {
		atomic.AddInt32(&attempts, 1)
		select {
		case <-time.After(50 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	dec := Compose(
		WithRetry(RetryConfig{MaxAttempts: 3, BaseDelay: 1 * time.Millisecond}),
		WithTimeout(5*time.Millisecond),
	)
	out := dec(slow)

	err := out(context.Background(), &domain.Session{})
	if err == nil {
		t.Fatal("expected error")
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("retry should attempt 3 times across renewed timeouts; got %d", attempts)
	}
}

// =============== suspend / resume ===============

// TestGraph_Suspend 验证节点返回 ErrSuspended 时:
//   - Invoke 正常返回 nil（不当错误处理）
//   - sess.CurrentNode 留在暂停的那个节点
//   - 暂停节点的下游节点没被执行
func TestGraph_Suspend(t *testing.T) {
	var trace []string
	var mu sync.Mutex

	suspendingNode := func(ctx context.Context, sess *domain.Session) error {
		mu.Lock()
		trace = append(trace, "pick_next")
		mu.Unlock()
		return fmt.Errorf("waiting for answer: %w", ErrSuspended)
	}

	g := New("suspend").
		AddNode("pick_next", suspendingNode).
		AddNode("evaluate", recorder(&trace, &mu, "evaluate")).
		AddNode("report", recorder(&trace, &mu, "report")).
		Entry("pick_next").
		AddEdge("pick_next", "evaluate").
		AddEdge("evaluate", "report").
		AddEdge("report", EndNode)

	rn, err := g.Compile()
	if err != nil {
		t.Fatal(err)
	}
	sess := &domain.Session{}
	if err := rn.Invoke(context.Background(), sess); err != nil {
		t.Fatalf("invoke should swallow ErrSuspended, got: %v", err)
	}
	if sess.CurrentNode != "pick_next" {
		t.Errorf("CurrentNode = %q, want pick_next", sess.CurrentNode)
	}
	if len(trace) != 1 || trace[0] != "pick_next" {
		t.Errorf("downstream should not run, got trace=%v", trace)
	}
}

// TestGraph_Resume 验证 Resume 从 sess.CurrentNode 的下游继续。
func TestGraph_Resume(t *testing.T) {
	var trace []string
	var mu sync.Mutex

	// 第一次调用返回 suspend；Resume 后这个节点不应再被执行
	calls := 0
	suspendingNode := func(ctx context.Context, sess *domain.Session) error {
		calls++
		mu.Lock()
		trace = append(trace, "pick_next")
		mu.Unlock()
		return ErrSuspended
	}

	g := New("resume").
		AddNode("pick_next", suspendingNode).
		AddNode("evaluate", recorder(&trace, &mu, "evaluate")).
		AddNode("report", recorder(&trace, &mu, "report")).
		Entry("pick_next").
		AddEdge("pick_next", "evaluate").
		AddEdge("evaluate", "report").
		AddEdge("report", EndNode)

	rn, err := g.Compile()
	if err != nil {
		t.Fatal(err)
	}
	sess := &domain.Session{}
	if err := rn.Invoke(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if err := rn.Resume(context.Background(), sess); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if calls != 1 {
		t.Errorf("pick_next should run once; got %d", calls)
	}
	want := []string{"pick_next", "evaluate", "report"}
	if len(trace) != len(want) {
		t.Fatalf("trace=%v, want %v", trace, want)
	}
	for i, n := range want {
		if trace[i] != n {
			t.Errorf("trace[%d]=%q, want %q", i, trace[i], n)
		}
	}
}

// TestGraph_Resume_MissingCurrentNode resume 前必须有 CurrentNode。
func TestGraph_Resume_MissingCurrentNode(t *testing.T) {
	g := New("x").
		AddNode("a", recorder(new([]string), new(sync.Mutex), "a")).
		Entry("a").
		AddEdge("a", EndNode)
	rn, _ := g.Compile()
	err := rn.Resume(context.Background(), &domain.Session{})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}
