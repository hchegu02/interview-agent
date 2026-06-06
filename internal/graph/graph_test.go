package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

type panicCheckpointRecorder struct{}

func (panicCheckpointRecorder) RecordCheckpoint(ctx context.Context, checkpoint GraphCheckpoint) {
	panic("checkpoint recorder failed")
}

type blockingCheckpointRecorder struct{}

func (blockingCheckpointRecorder) RecordCheckpoint(ctx context.Context, checkpoint GraphCheckpoint) {
	select {
	case <-time.After(5 * checkpointRecorderTimeout):
	case <-ctx.Done():
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

func TestGraph_AddNodeLegacyLinearStillWorks(t *testing.T) {
	var ran bool
	g := New("legacy-linear").
		AddNode("a", func(ctx context.Context, sess *domain.Session) error {
			ran = true
			sess.UserID = "legacy"
			return nil
		}).
		Entry("a").
		AddEdge("a", EndNode)

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	sess := &domain.Session{}
	if err := r.Invoke(context.Background(), sess); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !ran || sess.UserID != "legacy" {
		t.Fatalf("legacy node did not run: ran=%v sess=%+v", ran, sess)
	}
}

func TestGraph_PatchNodeAppliesStatePatch(t *testing.T) {
	pool := []domain.Question{{ID: "q1", Content: "Redis AOF?"}}
	g := New("patch-node").
		AddNodeSpec(PatchNode("retrieve", []string{WriteCandidatePool}, func(ctx context.Context, sess *domain.Session) (domain.StatePatch, error) {
			return domain.StatePatch{CandidatePool: &pool}, nil
		})).
		Entry("retrieve").
		AddEdge("retrieve", EndNode)

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	sess := &domain.Session{}
	if err := r.Invoke(context.Background(), sess); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if len(sess.CandidatePool) != 1 || sess.CandidatePool[0].ID != "q1" {
		t.Fatalf("candidate pool = %+v", sess.CandidatePool)
	}
}

func TestGraph_PatchNodeCheckpointIncludesPatchSummary(t *testing.T) {
	rec := NewMemoryCheckpointRecorder(20)
	mem := domain.NewWorkingMemory()
	mem.DegradedReasons = map[string]string{"rag": "embedder failed"}
	g := New("patch-node-summary").
		AddNodeSpec(PatchNode("retrieve", []string{WriteCandidatePool, WriteWorkingMemory}, func(ctx context.Context, sess *domain.Session) (domain.StatePatch, error) {
			pool := []domain.Question{{ID: "q1", Content: "Redis AOF?"}}
			return domain.StatePatch{CandidatePool: &pool, WorkingMemory: mem}, nil
		})).
		Entry("retrieve").
		AddEdge("retrieve", EndNode).
		WithCheckpointRecorder(rec)

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := r.Invoke(context.Background(), &domain.Session{ID: "patch-summary"}); err != nil {
		t.Fatalf("invoke: %v", err)
	}

	after := findCheckpoint(rec.Snapshot(), CheckpointNodeAfter, "retrieve")
	if after == nil {
		t.Fatalf("missing node_after checkpoint: %+v", rec.Snapshot())
	}
	summary := after.PatchSummary
	if summary == nil {
		t.Fatal("node_after checkpoint should include patch summary")
	}
	if summary.Node != "retrieve" {
		t.Fatalf("summary node = %q, want retrieve", summary.Node)
	}
	if !stringSliceContains(summary.WrittenFields, "candidate_pool") || !stringSliceContains(summary.WrittenFields, "working_memory") {
		t.Fatalf("summary written fields = %+v, want candidate_pool and working_memory", summary.WrittenFields)
	}
	if !stringSliceContains(summary.Writes, WriteCandidatePool) || !stringSliceContains(summary.Writes, WriteWorkingMemory) {
		t.Fatalf("summary writes = %+v, want declared writes", summary.Writes)
	}
	if len(summary.DegradedComponents) != 1 || summary.DegradedComponents[0] != "rag" {
		t.Fatalf("summary degraded components = %+v, want [rag]", summary.DegradedComponents)
	}
}

func TestGraph_PatchNodeSuspendCheckpointIncludesPatchSummary(t *testing.T) {
	rec := NewMemoryCheckpointRecorder(20)
	pool := []domain.Question{{ID: "q1", Content: "Go GMP?"}}
	g := New("patch-node-suspend-summary").AddNodeSpec(
		PatchNode("pick_next", []string{WriteCandidatePool}, func(ctx context.Context, sess *domain.Session) (domain.StatePatch, error) {
			return domain.StatePatch{CandidatePool: &pool}, SuspendWithPatch(fmt.Errorf("waiting for answer: %w", ErrSuspended))
		}),
	).Entry("pick_next").AddEdge("pick_next", EndNode).WithCheckpointRecorder(rec)

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := r.Invoke(context.Background(), &domain.Session{ID: "patch-suspend-summary"}); err != nil {
		t.Fatalf("invoke should swallow suspend, got %v", err)
	}

	suspended := findCheckpoint(rec.Snapshot(), CheckpointSuspended, "pick_next")
	if suspended == nil {
		t.Fatalf("missing suspended checkpoint: %+v", rec.Snapshot())
	}
	if suspended.PatchSummary == nil {
		t.Fatal("suspended checkpoint should include patch summary")
	}
	if !suspended.PatchSummary.Suspended {
		t.Fatalf("patch summary should mark suspended: %+v", suspended.PatchSummary)
	}
	if !stringSliceContains(suspended.PatchSummary.WrittenFields, "candidate_pool") {
		t.Fatalf("summary written fields = %+v, want candidate_pool", suspended.PatchSummary.WrittenFields)
	}
}

func TestGraph_PatchNodeNilFuncReturnsInvalidConfig(t *testing.T) {
	g := New("patch-node-nil").
		AddNodeSpec(PatchNode("bad", []string{WriteReport}, nil)).
		Entry("bad").
		AddEdge("bad", EndNode)

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	err = r.Invoke(context.Background(), &domain.Session{})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invoke error = %v, want ErrInvalidConfig", err)
	}
}

func TestGraph_PatchNodeApplyFailureIsPermanent(t *testing.T) {
	g := New("patch-node-apply-failure").
		AddNodeSpec(PatchNode("evaluate", []string{WriteCurrentEvaluation}, func(ctx context.Context, sess *domain.Session) (domain.StatePatch, error) {
			return domain.StatePatch{CurrentEvaluation: &domain.Evaluation{QuestionID: "q1", Score: 80}}, nil
		})).
		Entry("evaluate").
		AddEdge("evaluate", EndNode)

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	err = r.Invoke(context.Background(), &domain.Session{})
	if !errors.Is(err, ErrPermanent) {
		t.Fatalf("invoke error = %v, want ErrPermanent", err)
	}
	if !strings.Contains(err.Error(), "evaluate: apply state patch") {
		t.Fatalf("error should include node name, got %v", err)
	}
}

func TestGraph_PatchNodeSuspendWithPatchAppliesPatchAndSuspends(t *testing.T) {
	pool := []domain.Question{{ID: "q1", Content: "Go GMP?"}}
	g := New("patch-node-suspend").AddNodeSpec(
		PatchNode("pick_next", []string{WriteCandidatePool}, func(ctx context.Context, sess *domain.Session) (domain.StatePatch, error) {
			return domain.StatePatch{CandidatePool: &pool}, SuspendWithPatch(fmt.Errorf("waiting for answer: %w", ErrSuspended))
		}),
	).Entry("pick_next").AddEdge("pick_next", EndNode)

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	sess := &domain.Session{}
	if err := r.Invoke(context.Background(), sess); err != nil {
		t.Fatalf("invoke should swallow suspend, got %v", err)
	}
	if len(sess.CandidatePool) != 1 || sess.CandidatePool[0].ID != "q1" {
		t.Fatalf("candidate pool = %+v, want q1", sess.CandidatePool)
	}
	if sess.CurrentNode != "pick_next" || sess.Suspension == nil {
		t.Fatalf("suspend state not recorded: current=%q suspension=%+v", sess.CurrentNode, sess.Suspension)
	}
}

func TestGraph_PatchNodeOrdinaryErrorDoesNotApplyPatch(t *testing.T) {
	pool := []domain.Question{{ID: "q1", Content: "Go GMP?"}}
	g := New("patch-node-error").AddNodeSpec(
		PatchNode("retrieve", []string{WriteCandidatePool}, func(ctx context.Context, sess *domain.Session) (domain.StatePatch, error) {
			return domain.StatePatch{CandidatePool: &pool}, errors.New("ordinary failure")
		}),
	).Entry("retrieve").AddEdge("retrieve", EndNode)

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	sess := &domain.Session{}
	if err := r.Invoke(context.Background(), sess); err == nil {
		t.Fatal("expected ordinary error")
	}
	if len(sess.CandidatePool) != 0 {
		t.Fatalf("ordinary error should not apply patch, got %+v", sess.CandidatePool)
	}
}

func TestGraph_ConcurrentFrontierRejectsConflictingWrites(t *testing.T) {
	var ran int32
	g := New("conflicting-writes").
		AddNode("__START__", func(ctx context.Context, sess *domain.Session) error { return nil }).
		AddNodeSpec(NodeSpec{
			Name:   "a",
			Fn:     func(ctx context.Context, sess *domain.Session) error { atomic.AddInt32(&ran, 1); return nil },
			Writes: []string{WriteWorkingMemory},
		}).
		AddNodeSpec(NodeSpec{
			Name:   "b",
			Fn:     func(ctx context.Context, sess *domain.Session) error { atomic.AddInt32(&ran, 1); return nil },
			Writes: []string{WriteWorkingMemory},
		}).
		Entry("__START__").
		AddEdge("__START__", "a").
		AddEdge("__START__", "b")

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := r.Invoke(context.Background(), &domain.Session{}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invoke error = %v, want ErrInvalidConfig", err)
	}
	if atomic.LoadInt32(&ran) != 0 {
		t.Fatalf("conflicting nodes should not run, ran=%d", ran)
	}
}

func TestGraph_ConcurrentFrontierRejectsLegacyNodeWithoutWrites(t *testing.T) {
	var ran int32
	g := New("legacy-parallel").
		AddNode("__START__", func(ctx context.Context, sess *domain.Session) error { return nil }).
		AddNode("legacy", func(ctx context.Context, sess *domain.Session) error {
			atomic.AddInt32(&ran, 1)
			return nil
		}).
		AddNodeSpec(NodeSpec{
			Name:   "safe",
			Fn:     func(ctx context.Context, sess *domain.Session) error { atomic.AddInt32(&ran, 1); return nil },
			Writes: []string{WriteReport},
		}).
		Entry("__START__").
		AddEdge("__START__", "legacy").
		AddEdge("__START__", "safe")

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := r.Invoke(context.Background(), &domain.Session{}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invoke error = %v, want ErrInvalidConfig", err)
	}
	if atomic.LoadInt32(&ran) != 0 {
		t.Fatalf("unsafe frontier should not run, ran=%d", ran)
	}
}

func TestGraph_CheckpointsLinearNodeSnapshots(t *testing.T) {
	rec := NewMemoryCheckpointRecorder(20)
	g := New("checkpoint-linear").
		AddNode("a", func(ctx context.Context, sess *domain.Session) error {
			sess.UserID = "after-a"
			return nil
		}).
		AddNode("b", func(ctx context.Context, sess *domain.Session) error {
			sess.Mode = "practice"
			return nil
		}).
		Entry("a").
		AddEdge("a", "b").
		AddEdge("b", EndNode).
		WithCheckpointRecorder(rec)

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := r.Invoke(context.Background(), &domain.Session{ID: "s-checkpoint"}); err != nil {
		t.Fatalf("invoke: %v", err)
	}

	checkpoints := rec.Snapshot()
	assertCheckpointPhase(t, checkpoints, CheckpointFrontierBefore, "")
	beforeA := findCheckpoint(checkpoints, CheckpointNodeBefore, "a")
	afterA := findCheckpoint(checkpoints, CheckpointNodeAfter, "a")
	if beforeA == nil || afterA == nil {
		t.Fatalf("missing node checkpoints: %+v", checkpoints)
	}
	if strings.Contains(string(beforeA.Snapshot), "after-a") {
		t.Fatalf("node_before should capture state before mutation: %s", string(beforeA.Snapshot))
	}
	if !strings.Contains(string(afterA.Snapshot), "after-a") {
		t.Fatalf("node_after should capture state after mutation: %s", string(afterA.Snapshot))
	}
	if checkpoints[0].Seq == 0 {
		t.Fatalf("checkpoint seq should be assigned: %+v", checkpoints[0])
	}
	for i := 1; i < len(checkpoints); i++ {
		if checkpoints[i].Seq <= checkpoints[i-1].Seq {
			t.Fatalf("checkpoint seq should increase: %+v", checkpoints)
		}
	}
}

func TestGraph_CheckpointsNodeAndFrontierError(t *testing.T) {
	rec := NewMemoryCheckpointRecorder(20)
	boom := errors.New("boom")
	g := New("checkpoint-error").
		AddNode("a", func(ctx context.Context, sess *domain.Session) error {
			sess.UserID = "before-error"
			return boom
		}).
		Entry("a").
		AddEdge("a", EndNode).
		WithCheckpointRecorder(rec)

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := r.Invoke(context.Background(), &domain.Session{ID: "s-error"}); !errors.Is(err, boom) {
		t.Fatalf("invoke error = %v, want boom", err)
	}

	checkpoints := rec.Snapshot()
	nodeErr := findCheckpoint(checkpoints, CheckpointNodeError, "a")
	if nodeErr == nil {
		t.Fatalf("missing node_error checkpoint: %+v", checkpoints)
	}
	if !strings.Contains(nodeErr.Error, "boom") {
		t.Fatalf("node_error Error = %q, want boom", nodeErr.Error)
	}
	if !strings.Contains(string(nodeErr.Snapshot), "before-error") {
		t.Fatalf("node_error snapshot should include mutation before failure: %s", string(nodeErr.Snapshot))
	}
	frontierErr := findCheckpoint(checkpoints, CheckpointFrontierError, "")
	if frontierErr == nil {
		t.Fatalf("missing frontier_error checkpoint: %+v", checkpoints)
	}
	if !strings.Contains(frontierErr.Error, "boom") {
		t.Fatalf("frontier_error Error = %q, want boom", frontierErr.Error)
	}
}

func TestGraph_CheckpointRecorderPanicDoesNotFailGraph(t *testing.T) {
	g := New("checkpoint-panic").
		AddNode("a", func(ctx context.Context, sess *domain.Session) error {
			sess.UserID = "ran"
			return nil
		}).
		Entry("a").
		AddEdge("a", EndNode).
		WithCheckpointRecorder(panicCheckpointRecorder{})

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	sess := &domain.Session{}
	if err := r.Invoke(context.Background(), sess); err != nil {
		t.Fatalf("invoke should ignore checkpoint recorder panic: %v", err)
	}
	if sess.UserID != "ran" {
		t.Fatalf("node did not run: %+v", sess)
	}
}

func TestGraph_CheckpointRecorderTimeoutDoesNotBlockGraph(t *testing.T) {
	g := New("checkpoint-timeout").
		AddNode("a", func(ctx context.Context, sess *domain.Session) error {
			sess.UserID = "ran"
			return nil
		}).
		Entry("a").
		AddEdge("a", EndNode).
		WithCheckpointRecorder(blockingCheckpointRecorder{})

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	sess := &domain.Session{}
	done := make(chan error, 1)
	go func() {
		done <- r.Invoke(context.Background(), sess)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("invoke should ignore slow checkpoint recorder: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("invoke timed out; checkpoint recorder should not block graph")
	}
	if sess.UserID != "ran" {
		t.Fatalf("node did not run: %+v", sess)
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
		AddNodeSpec(NodeSpec{Name: "a", Fn: recorder(&trace, &mu, "a"), Writes: []string{WriteCandidatePool}}).
		AddNodeSpec(NodeSpec{Name: "b", Fn: recorder(&trace, &mu, "b"), Writes: []string{WriteReport}}).
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

func TestGraph_CheckpointsParallelFrontierIsBatchOnly(t *testing.T) {
	rec := NewMemoryCheckpointRecorder(20)
	var trace []string
	var mu sync.Mutex

	g := New("checkpoint-parallel").
		AddNode("__START__", recorder(&trace, &mu, "start")).
		AddNodeSpec(NodeSpec{Name: "a", Fn: recorder(&trace, &mu, "a"), Writes: []string{WriteCandidatePool}}).
		AddNodeSpec(NodeSpec{Name: "b", Fn: recorder(&trace, &mu, "b"), Writes: []string{WriteReport}}).
		AddNode("c", recorder(&trace, &mu, "c")).
		Entry("__START__").
		AddEdge("__START__", "a").
		AddEdge("__START__", "b").
		AddEdge("a", "c").
		AddEdge("b", "c").
		AddEdge("c", EndNode).
		WithCheckpointRecorder(rec)

	r, err := g.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := r.Invoke(context.Background(), &domain.Session{}); err != nil {
		t.Fatalf("invoke: %v", err)
	}

	checkpoints := rec.Snapshot()
	var parallelBefore *GraphCheckpoint
	for i := range checkpoints {
		cp := &checkpoints[i]
		if cp.Phase == CheckpointFrontierBefore && len(cp.Frontier) == 2 {
			parallelBefore = cp
			break
		}
	}
	if parallelBefore == nil {
		t.Fatalf("missing parallel frontier checkpoint: %+v", checkpoints)
	}
	for _, cp := range checkpoints {
		if (cp.Node == "a" || cp.Node == "b") && (cp.Phase == CheckpointNodeBefore || cp.Phase == CheckpointNodeAfter) {
			t.Fatalf("parallel node checkpoint should not be recorded: %+v", cp)
		}
	}
}

// 验证 c 只执行一次（fan-in 不重复触发）
func TestGraph_FanInDedup(t *testing.T) {
	var cCount int32

	g := New("fanin").
		AddNode("__START__", func(ctx context.Context, s *domain.Session) error { return nil }).
		AddNodeSpec(NodeSpec{
			Name:   "a",
			Fn:     func(ctx context.Context, s *domain.Session) error { return nil },
			Writes: []string{WriteCandidatePool},
		}).
		AddNodeSpec(NodeSpec{
			Name:   "b",
			Fn:     func(ctx context.Context, s *domain.Session) error { return nil },
			Writes: []string{WriteReport},
		}).
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
		<-ctx.Done()
		return ctx.Err()
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
	if sess.Suspension == nil {
		t.Fatal("Suspension should be set")
	}
	if sess.Suspension.Node != "pick_next" {
		t.Errorf("Suspension.Node = %q, want pick_next", sess.Suspension.Node)
	}
	if sess.Suspension.Awaiting != domain.SuspensionAwaitingAnswer {
		t.Errorf("Suspension.Awaiting = %q, want %q", sess.Suspension.Awaiting, domain.SuspensionAwaitingAnswer)
	}
	if sess.Suspension.CreatedAt.IsZero() {
		t.Error("Suspension.CreatedAt should be set")
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
	if sess.Suspension == nil {
		t.Fatal("invoke should record suspension")
	}
	if err := rn.Resume(context.Background(), sess); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if sess.Suspension != nil {
		t.Fatalf("resume should clear stale suspension: %+v", sess.Suspension)
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

func TestGraph_CheckpointsSuspendAndResume(t *testing.T) {
	rec := NewMemoryCheckpointRecorder(30)
	var trace []string
	var mu sync.Mutex

	g := New("checkpoint-resume").
		AddNode("pick_next", func(ctx context.Context, sess *domain.Session) error {
			mu.Lock()
			trace = append(trace, "pick_next")
			mu.Unlock()
			return ErrSuspended
		}).
		AddNode("evaluate", recorder(&trace, &mu, "evaluate")).
		AddNode("report", recorder(&trace, &mu, "report")).
		Entry("pick_next").
		AddEdge("pick_next", "evaluate").
		AddEdge("evaluate", "report").
		AddEdge("report", EndNode).
		WithCheckpointRecorder(rec)

	rn, err := g.Compile()
	if err != nil {
		t.Fatal(err)
	}
	sess := &domain.Session{ID: "resume-checkpoint"}
	if err := rn.Invoke(context.Background(), sess); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	suspended := findCheckpoint(rec.Snapshot(), CheckpointSuspended, "pick_next")
	if suspended == nil {
		t.Fatalf("missing suspended checkpoint: %+v", rec.Snapshot())
	}
	if !strings.Contains(string(suspended.Snapshot), `"suspension"`) {
		t.Fatalf("suspended snapshot should include suspension: %s", string(suspended.Snapshot))
	}

	if err := rn.Resume(context.Background(), sess); err != nil {
		t.Fatalf("resume: %v", err)
	}
	resume := findCheckpoint(rec.Snapshot(), CheckpointResumeFrom, "pick_next")
	if resume == nil {
		t.Fatalf("missing resume_from checkpoint: %+v", rec.Snapshot())
	}
	if len(resume.Frontier) != 1 || resume.Frontier[0] != "evaluate" {
		t.Fatalf("resume frontier = %+v, want [evaluate]", resume.Frontier)
	}
}

func TestGraph_CheckpointsResumeWithNoNextFrontier(t *testing.T) {
	rec := NewMemoryCheckpointRecorder(10)
	g := New("checkpoint-resume-end").
		AddNode("done", func(ctx context.Context, sess *domain.Session) error { return nil }).
		Entry("done").
		AddEdge("done", EndNode).
		WithCheckpointRecorder(rec)

	rn, err := g.Compile()
	if err != nil {
		t.Fatal(err)
	}
	sess := &domain.Session{
		ID:          "resume-end",
		CurrentNode: "done",
		Suspension:  &domain.Suspension{Node: "done", Awaiting: domain.SuspensionAwaitingAnswer},
	}
	if err := rn.Resume(context.Background(), sess); err != nil {
		t.Fatalf("resume: %v", err)
	}
	resume := findCheckpoint(rec.Snapshot(), CheckpointResumeFrom, "done")
	if resume == nil {
		t.Fatalf("missing resume_from checkpoint: %+v", rec.Snapshot())
	}
	if len(resume.Frontier) != 0 {
		t.Fatalf("resume next frontier = %+v, want empty", resume.Frontier)
	}
	if sess.Suspension != nil {
		t.Fatalf("resume should clear suspension: %+v", sess.Suspension)
	}
}

func TestGraph_CheckpointsParallelSuspendDoesNotClaimNode(t *testing.T) {
	rec := NewMemoryCheckpointRecorder(20)
	g := New("checkpoint-parallel-suspend").
		AddNode("__START__", func(ctx context.Context, sess *domain.Session) error { return nil }).
		AddNodeSpec(NodeSpec{
			Name:   "a",
			Fn:     func(ctx context.Context, sess *domain.Session) error { return ErrSuspended },
			Writes: []string{WriteSuspension},
		}).
		AddNodeSpec(NodeSpec{
			Name: "b",
			Fn: func(ctx context.Context, sess *domain.Session) error {
				sess.UserID = "b-ran"
				return nil
			},
			Writes: []string{WriteReport},
		}).
		Entry("__START__").
		AddEdge("__START__", "a").
		AddEdge("__START__", "b").
		WithCheckpointRecorder(rec)

	rn, err := g.Compile()
	if err != nil {
		t.Fatal(err)
	}
	sess := &domain.Session{ID: "parallel-suspend"}
	if err := rn.Invoke(context.Background(), sess); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if sess.CurrentNode != "a" {
		t.Fatalf("CurrentNode = %q, want suspended node a", sess.CurrentNode)
	}
	if sess.Suspension == nil || sess.Suspension.Node != "a" {
		t.Fatalf("Suspension = %+v, want node a", sess.Suspension)
	}
	suspended := findCheckpoint(rec.Snapshot(), CheckpointSuspended, "")
	if suspended == nil {
		t.Fatalf("missing batch suspended checkpoint without node claim: %+v", rec.Snapshot())
	}
	if len(suspended.Frontier) != 2 {
		t.Fatalf("suspended frontier = %+v, want two nodes", suspended.Frontier)
	}
}

// TestGraph_Resume_LegacyCurrentNodeOnly 验证老 Session 只有 CurrentNode 时仍可恢复。
func TestGraph_Resume_LegacyCurrentNodeOnly(t *testing.T) {
	var trace []string
	var mu sync.Mutex

	g := New("legacy-resume").
		AddNode("pick_next", recorder(&trace, &mu, "pick_next")).
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
	sess := &domain.Session{CurrentNode: "pick_next"}
	if err := rn.Resume(context.Background(), sess); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	want := []string{"evaluate", "report"}
	if len(trace) != len(want) {
		t.Fatalf("trace=%v, want %v", trace, want)
	}
	for i, n := range want {
		if trace[i] != n {
			t.Errorf("trace[%d]=%q, want %q", i, trace[i], n)
		}
	}
}

func findCheckpoint(checkpoints []GraphCheckpoint, phase CheckpointPhase, node string) *GraphCheckpoint {
	for i := range checkpoints {
		if checkpoints[i].Phase == phase && checkpoints[i].Node == node {
			return &checkpoints[i]
		}
	}
	return nil
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertCheckpointPhase(t *testing.T, checkpoints []GraphCheckpoint, phase CheckpointPhase, node string) {
	t.Helper()
	if findCheckpoint(checkpoints, phase, node) == nil {
		t.Fatalf("missing checkpoint phase=%s node=%s: %+v", phase, node, checkpoints)
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
