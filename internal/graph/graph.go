package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"interview-agent/internal/domain"

	"golang.org/x/sync/errgroup"
)

const checkpointRecorderTimeout = 100 * time.Millisecond

// Graph 是声明式的节点 + 边集合。
// 使用流式 API 构造，最后 Compile() 得到可执行的 Runnable。
//
// 示例：
//
//	g := graph.New("interview").
//	    AddNode("parse_jd", parseJD).
//	    AddNode("parse_resume", parseResume).
//	    AddNode("gap_analyze", gapAnalyze).
//	    Entry("__START__").
//	    AddEdge("__START__", "parse_jd").
//	    AddEdge("__START__", "parse_resume").  // 与上一条 fan-out 并发
//	    AddEdge("parse_jd", "gap_analyze").    // gap_analyze fan-in
//	    AddEdge("parse_resume", "gap_analyze").
//	    AddBranch("critic", criticRouter)      // 条件分支
//
// 注意"__START__"是个特殊入口节点，用 nopNode 注册。
type Graph struct {
	name      string
	nodes     map[string]NodeFunc
	specs     map[string]NodeSpec
	edges     map[string][]string // from -> tos (多个表示 fan-out 并发)
	routers   map[string]Router   // from -> 条件路由（与 edges 互斥）
	entry     string
	maxSteps  int
	callbacks []Callback
	recorder  CheckpointRecorder
}

// New 创建一个空图。name 用于日志和错误信息。
func New(name string) *Graph {
	return &Graph{
		name:     name,
		nodes:    map[string]NodeFunc{},
		specs:    map[string]NodeSpec{},
		edges:    map[string][]string{},
		routers:  map[string]Router{},
		maxSteps: 200, // 全局步数兜底；业务循环用 WorkingMemory 预算更精确
	}
}

// AddNode 注册节点。重复注册会覆盖（最后一次生效）。
func (g *Graph) AddNode(name string, fn NodeFunc) *Graph {
	g.nodes[name] = fn
	g.specs[name] = NodeSpec{Name: name, Fn: fn, kind: nodeKindLegacy}
	return g
}

// AddNodeSpec 注册带写集元数据的节点。重复注册会覆盖（最后一次生效）。
func (g *Graph) AddNodeSpec(spec NodeSpec) *Graph {
	if spec.kind == nodeKindPatch {
		g.nodes[spec.Name] = nil
	} else {
		g.nodes[spec.Name] = spec.Fn
	}
	spec.Writes = append([]string(nil), spec.Writes...)
	g.specs[spec.Name] = spec
	return g
}

// Entry 指定入口节点。必须在 Compile 前调用一次。
func (g *Graph) Entry(name string) *Graph {
	g.entry = name
	return g
}

// AddEdge 添加无条件边。同一个 from 可以有多条 edge → fan-out 并发。
func (g *Graph) AddEdge(from, to string) *Graph {
	g.edges[from] = append(g.edges[from], to)
	return g
}

// AddBranch 给节点添加条件路由器。与该节点的静态 edges 互斥。
// Compile 会校验冲突。
func (g *Graph) AddBranch(from string, router Router) *Graph {
	g.routers[from] = router
	return g
}

// MaxSteps 设置全局步数预算（防止意外死循环）。默认 200。
func (g *Graph) MaxSteps(n int) *Graph {
	g.maxSteps = n
	return g
}

// WithCallbacks 注册回调。多个回调按注册顺序执行。
func (g *Graph) WithCallbacks(cbs ...Callback) *Graph {
	g.callbacks = append(g.callbacks, cbs...)
	return g
}

// WithCheckpointRecorder 注册可选 checkpoint recorder。
// recorder 只用于调试和验证，不应改变 Graph 业务结果。
func (g *Graph) WithCheckpointRecorder(recorder CheckpointRecorder) *Graph {
	g.recorder = recorder
	return g
}

// Compile 校验图结构合法性，返回可执行的 Runnable。
//
// 校验项：
//  1. 入口节点已注册
//  2. 所有边引用的节点都已注册（EndNode 除外）
//  3. 所有 router 注册在已存在的节点上
//  4. 静态边 / router 不在同一节点同时存在
func (g *Graph) Compile() (*Runnable, error) {
	if g.entry == "" {
		return nil, fmt.Errorf("%w: entry not set", ErrInvalidConfig)
	}
	if _, ok := g.specs[g.entry]; !ok {
		return nil, fmt.Errorf("%w: entry node %q not registered", ErrInvalidConfig, g.entry)
	}

	for from, tos := range g.edges {
		if _, ok := g.specs[from]; !ok {
			return nil, fmt.Errorf("%w: edge from undefined node %q", ErrNodeNotFound, from)
		}
		for _, to := range tos {
			if to == EndNode {
				continue
			}
			if _, ok := g.specs[to]; !ok {
				return nil, fmt.Errorf("%w: edge to undefined node %q (from %q)", ErrNodeNotFound, to, from)
			}
		}
	}

	for from := range g.routers {
		if _, ok := g.specs[from]; !ok {
			return nil, fmt.Errorf("%w: router on undefined node %q", ErrNodeNotFound, from)
		}
		if _, ok := g.edges[from]; ok {
			return nil, fmt.Errorf("%w: node %q has both static edges and router", ErrInvalidConfig, from)
		}
	}

	return &Runnable{g: g}, nil
}

// Runnable 是编译后的图。可被多次并发 Invoke（不同 Session 之间无共享状态）。
type Runnable struct {
	g *Graph
}

// NodeSpecs 返回编译后图的节点规格快照，供验证和调试使用。
func (r *Runnable) NodeSpecs() []NodeSpec {
	if r == nil || r.g == nil {
		return nil
	}
	out := make([]NodeSpec, 0, len(r.g.specs))
	for _, spec := range r.g.specs {
		spec.Writes = append([]string(nil), spec.Writes...)
		spec.Fn = nil
		spec.patch = nil
		out = append(out, spec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Successors 返回指定节点的静态后继；router 节点返回空切片。
func (r *Runnable) Successors(node string) []string {
	if r == nil || r.g == nil {
		return nil
	}
	return append([]string(nil), r.g.edges[node]...)
}

// HasRouter 返回指定节点是否配置了条件路由。
func (r *Runnable) HasRouter(node string) bool {
	if r == nil || r.g == nil {
		return false
	}
	_, ok := r.g.routers[node]
	return ok
}

type nodeExecutionError struct {
	node         string
	err          error
	patchSummary *PatchSummary
}

func (e nodeExecutionError) Error() string {
	return e.err.Error()
}

func (e nodeExecutionError) Unwrap() error {
	return e.err
}

func executionErrorNode(err error) string {
	var nodeErr nodeExecutionError
	if errors.As(err, &nodeErr) {
		return nodeErr.node
	}
	return ""
}

func executionErrorPatchSummary(err error) *PatchSummary {
	var nodeErr nodeExecutionError
	if errors.As(err, &nodeErr) {
		return nodeErr.patchSummary
	}
	return nil
}

// Invoke 执行图，传入的 Session 会被节点函数读写。
//
// 执行模型（frontier-based）：
//   - 维护一个"待执行节点集合" frontier
//   - 每一轮把 frontier 里的所有节点用 errgroup 并发执行
//   - 等全部完成后，根据 edges/routers 算出下一轮 frontier
//   - 直到 frontier 为空或步数耗尽
//
// 为什么用 frontier 而不是事件驱动 / 协程长跑：
//   - 自然支持 fan-out（多节点） / fan-in（多边汇聚到同一节点会被 dedup）
//   - 与 errgroup 天然契合
//   - 状态简单，断点恢复时只需要记当前 frontier
func (r *Runnable) Invoke(ctx context.Context, sess *domain.Session) error {
	sess.MigrateLegacyState()
	return r.run(ctx, sess, []string{r.g.entry})
}

// Resume 从 sess.CurrentNode 的下游边继续执行。
//
// 使用场景：
//
//	节点返回 ErrSuspended（例如 pick_next 出完题等用户答题），
//	Invoke 正常返回 nil；HTTP 层把用户输入写到 sess.PendingAnswer，
//	然后调用 Resume 推进到 evaluate 节点。
//
// Resume 不会重跑 sess.CurrentNode —— 它假设该节点已经完成"暂停前"的工作，
// 直接从该节点的 edges / router 算下一轮 frontier 启动。
//
// 错误：
//   - sess.CurrentNode 为空 → ErrInvalidConfig
//   - 暂停节点未注册 → ErrNodeNotFound
func (r *Runnable) Resume(ctx context.Context, sess *domain.Session) error {
	sess.MigrateLegacyState()
	current := suspendedNode(sess)
	if current == "" {
		return fmt.Errorf("%w: resume requires sess.CurrentNode", ErrInvalidConfig)
	}
	if _, ok := r.g.specs[current]; !ok {
		return fmt.Errorf("%w: suspended node %q not in graph", ErrNodeNotFound, current)
	}
	// 从暂停节点的下游开始；当作"刚执行完该节点"的状态
	next := r.nextFrontier(sess, []string{current})
	r.recordCheckpoint(ctx, 0, CheckpointResumeFrom, next, current, "", sess)
	if len(next) == 0 {
		sess.Suspension = nil
		return nil
	}
	sess.Suspension = nil
	return r.run(ctx, sess, next)
}

// run 是 Invoke / Resume 共享的执行内核。
//
// 执行模型（frontier-based）：
//   - 维护一个"待执行节点集合" frontier
//   - 每一轮把 frontier 里的所有节点用 errgroup 并发执行
//   - 等全部完成后，根据 edges/routers 算出下一轮 frontier
//   - 直到 frontier 为空或步数耗尽
//
// 为什么用 frontier 而不是事件驱动 / 协程长跑：
//   - 自然支持 fan-out（多节点） / fan-in（多边汇聚到同一节点会被 dedup）
//   - 与 errgroup 天然契合
//   - 状态简单，断点恢复时只需要记当前 frontier
//
// 节点返回 ErrSuspended 时正常返回 nil，runner 会把暂停节点写入
// sess.CurrentNode / sess.Suspension；调用方调 Resume 继续。
func (r *Runnable) run(ctx context.Context, sess *domain.Session, frontier []string) error {
	g := r.g
	steps := 0

	for len(frontier) > 0 {
		steps++
		if steps > g.maxSteps {
			return fmt.Errorf("%w: %d steps (graph=%s, last frontier=%v)",
				ErrMaxStepsExceeded, g.maxSteps, g.name, frontier)
		}

		r.recordCheckpoint(ctx, steps, CheckpointFrontierBefore, frontier, "", "", sess)
		if err := r.executeBatch(ctx, sess, frontier, steps); err != nil {
			if errors.Is(err, ErrSuspended) {
				// 暂停：并发 frontier 中不能让节点 goroutine 直接写 CurrentNode。
				// executeBatch 把出错节点带回主协程后，这里统一写程序计数器。
				node := executionErrorNode(err)
				if node == "" {
					node = sess.CurrentNode
				}
				sess.CurrentNode = node
				ensureSuspension(sess, node)
				checkpointNode := node
				if len(frontier) > 1 {
					checkpointNode = ""
				}
				r.recordCheckpointWithPatchSummary(ctx, steps, CheckpointSuspended, frontier, checkpointNode, err.Error(), sess, executionErrorPatchSummary(err))
				return nil
			}
			r.recordCheckpoint(ctx, steps, CheckpointFrontierError, frontier, "", err.Error(), sess)
			return err
		}
		r.recordCheckpoint(ctx, steps, CheckpointFrontierAfter, frontier, "", "", sess)

		frontier = r.nextFrontier(sess, frontier)
	}
	return nil
}

func suspendedNode(sess *domain.Session) string {
	if sess.Suspension != nil && sess.Suspension.Node != "" {
		return sess.Suspension.Node
	}
	return sess.CurrentNode
}

func ensureSuspension(sess *domain.Session, node string) {
	if sess.Suspension == nil {
		sess.Suspension = &domain.Suspension{}
	}
	if sess.Suspension.Node == "" {
		sess.Suspension.Node = node
	}
	if sess.Suspension.Awaiting == "" {
		sess.Suspension.Awaiting = domain.SuspensionAwaitingAnswer
	}
	if sess.Suspension.CreatedAt.IsZero() {
		sess.Suspension.CreatedAt = time.Now().UTC()
	}
}

// executeBatch 把当前 frontier 用 errgroup 并发执行。
// frontier 长度为 1 时跳过 errgroup 开销直接调用。
func (r *Runnable) executeBatch(ctx context.Context, sess *domain.Session, nodes []string, step int) error {
	if len(nodes) == 1 {
		return r.executeNode(ctx, sess, nodes[0], step, true, true)
	}
	if err := r.validateConcurrentFrontier(nodes); err != nil {
		return err
	}

	eg, ectx := errgroup.WithContext(ctx)
	for _, name := range nodes {
		name := name // 闭包捕获
		eg.Go(func() error {
			if err := r.executeNode(ectx, sess, name, step, false, false); err != nil {
				return err
			}
			return nil
		})
	}
	return eg.Wait()
}

func (r *Runnable) validateConcurrentFrontier(nodes []string) error {
	writers := make(map[string]string)
	for _, name := range nodes {
		spec, ok := r.g.specs[name]
		if !ok {
			return fmt.Errorf("%w: %q", ErrNodeNotFound, name)
		}
		if len(spec.Writes) == 0 {
			return fmt.Errorf("%w: concurrent node %q has no declared writes", ErrInvalidConfig, name)
		}
		for _, write := range spec.Writes {
			if prev, ok := writers[write]; ok {
				return fmt.Errorf("%w: concurrent nodes %q and %q both write %q", ErrInvalidConfig, prev, name, write)
			}
			writers[write] = name
		}
	}
	return nil
}

// executeNode 执行单个节点，前后调用 callback。
func (r *Runnable) executeNode(ctx context.Context, sess *domain.Session, name string, step int, checkpointNode, writeCurrentNode bool) error {
	g := r.g
	spec, ok := g.specs[name]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNodeNotFound, name)
	}
	if writeCurrentNode {
		sess.CurrentNode = name
	}

	if checkpointNode {
		r.recordCheckpoint(ctx, step, CheckpointNodeBefore, []string{name}, name, "", sess)
	}
	for _, cb := range g.callbacks {
		cb.OnNodeStart(ctx, name, sess)
	}
	patchSummary, err := r.callNode(ctx, sess, spec)
	for _, cb := range g.callbacks {
		if err != nil {
			cb.OnNodeError(ctx, name, sess, err)
		} else {
			cb.OnNodeEnd(ctx, name, sess)
		}
	}
	if checkpointNode {
		if err != nil {
			if !errors.Is(err, ErrSuspended) {
				r.recordCheckpointWithPatchSummary(ctx, step, CheckpointNodeError, []string{name}, name, err.Error(), sess, patchSummary)
			}
		} else {
			r.recordCheckpointWithPatchSummary(ctx, step, CheckpointNodeAfter, []string{name}, name, "", sess, patchSummary)
		}
	}
	if err != nil {
		return nodeExecutionError{node: name, err: err, patchSummary: patchSummary}
	}
	return nil
}

func (r *Runnable) callNode(ctx context.Context, sess *domain.Session, spec NodeSpec) (*PatchSummary, error) {
	if spec.kind == nodeKindPatch {
		if spec.patch == nil {
			return nil, fmt.Errorf("%w: node %q has nil patch function", ErrInvalidConfig, spec.Name)
		}
		patch, err := spec.patch(ctx, sess)
		if err != nil {
			if IsPatchSuspend(err) {
				if applyErr := domain.ApplyStatePatch(sess, patch); applyErr != nil {
					return nil, fmt.Errorf("%s: apply suspend state patch: %w: %v", spec.Name, ErrPermanent, applyErr)
				}
				return summarizeStatePatch(spec, patch, sess, true), err
			}
			return nil, err
		}
		if err := domain.ApplyStatePatch(sess, patch); err != nil {
			return nil, fmt.Errorf("%s: apply state patch: %w: %v", spec.Name, ErrPermanent, err)
		}
		return summarizeStatePatch(spec, patch, sess, false), nil
	}
	if spec.Fn == nil {
		return nil, fmt.Errorf("%w: node %q has nil function", ErrInvalidConfig, spec.Name)
	}
	return nil, spec.Fn(ctx, sess)
}

func summarizeStatePatch(spec NodeSpec, patch domain.StatePatch, sess *domain.Session, suspended bool) *PatchSummary {
	summary := &PatchSummary{
		Node:           spec.Name,
		Writes:         append([]string(nil), spec.Writes...),
		WrittenFields:  statePatchWrittenFields(patch),
		Suspended:      suspended,
		IdempotencyKey: patch.IdempotencyKey,
	}
	if patch.AppendRound != nil {
		summary.CurrentRoundID = patch.AppendRound.RoundID
	} else if sess != nil {
		if round := sess.CurrentRound(); round != nil {
			summary.CurrentRoundID = round.RoundID
		}
	}
	if patch.WorkingMemory != nil && len(patch.WorkingMemory.DegradedReasons) > 0 {
		summary.DegradedComponents = make([]string, 0, len(patch.WorkingMemory.DegradedReasons))
		for component := range patch.WorkingMemory.DegradedReasons {
			summary.DegradedComponents = append(summary.DegradedComponents, component)
		}
		sort.Strings(summary.DegradedComponents)
	}
	sort.Strings(summary.Writes)
	return summary
}

func statePatchWrittenFields(patch domain.StatePatch) []string {
	fields := make([]string, 0, 16)
	if patch.CandidatePool != nil {
		fields = append(fields, "candidate_pool")
	}
	if patch.RetrievalTrace != nil {
		fields = append(fields, "retrieval_trace")
	}
	if patch.PendingDecision != nil {
		fields = append(fields, "pending_decision")
	}
	if patch.ClearPendingDecision {
		fields = append(fields, "clear_pending_decision")
	}
	if patch.AppendRound != nil {
		fields = append(fields, "append_round")
	}
	if patch.AppendCurrentFollowUp != nil {
		fields = append(fields, "append_current_follow_up")
	}
	if patch.CurrentFollowUpEvaluation != nil {
		fields = append(fields, "current_follow_up_evaluation")
	}
	if patch.CurrentCriticResult != nil {
		fields = append(fields, "current_critic_result")
	}
	if patch.CurrentCriticProbeSignal != nil {
		fields = append(fields, "current_critic_probe_signal")
	}
	if patch.CurrentEvaluation != nil {
		fields = append(fields, "current_evaluation")
	}
	if patch.CurrentRefinedEvaluation != nil {
		fields = append(fields, "current_refined_evaluation")
	}
	if patch.CompleteCurrentRound != nil {
		fields = append(fields, "complete_current_round")
	}
	if patch.Report != nil {
		fields = append(fields, "report")
	}
	if patch.Status != nil {
		fields = append(fields, "status")
	}
	if patch.WorkingMemory != nil {
		fields = append(fields, "working_memory")
	}
	sort.Strings(fields)
	return fields
}

func (r *Runnable) recordCheckpoint(ctx context.Context, step int, phase CheckpointPhase, frontier []string, node, errMsg string, sess *domain.Session) {
	r.recordCheckpointWithPatchSummary(ctx, step, phase, frontier, node, errMsg, sess, nil)
}

func (r *Runnable) recordCheckpointWithPatchSummary(ctx context.Context, step int, phase CheckpointPhase, frontier []string, node, errMsg string, sess *domain.Session, patchSummary *PatchSummary) {
	if r == nil || r.g == nil || r.g.recorder == nil {
		return
	}
	snapshot, err := json.Marshal(sess)
	if err != nil {
		if errMsg == "" {
			errMsg = fmt.Sprintf("snapshot: %v", err)
		} else {
			errMsg = fmt.Sprintf("%s; snapshot: %v", errMsg, err)
		}
	}
	r.callCheckpointRecorder(ctx, GraphCheckpoint{
		Step:         step,
		SessionID:    sess.ID,
		Graph:        r.g.name,
		Phase:        phase,
		Frontier:     append([]string(nil), frontier...),
		Node:         node,
		Error:        errMsg,
		PatchSummary: clonePatchSummary(patchSummary),
		Snapshot:     snapshot,
		CreatedAt:    time.Now().UTC(),
	})
}

func (r *Runnable) callCheckpointRecorder(ctx context.Context, checkpoint GraphCheckpoint) {
	recordCtx, cancel := context.WithTimeout(ctx, checkpointRecorderTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			_ = recover()
		}()
		r.g.recorder.RecordCheckpoint(recordCtx, checkpoint)
	}()

	select {
	case <-done:
	case <-recordCtx.Done():
	}
}

// nextFrontier 根据当前已执行节点计算下一轮 frontier。
//   - 节点有 router → 调用 router，得到单个下一节点
//   - 节点有 edges → 全部加入 frontier（多个表示并发 fan-out）
//   - EndNode 被丢弃
//
// 用 map dedup 保证同一节点不会因为 fan-in 被重复执行。
func (r *Runnable) nextFrontier(sess *domain.Session, prev []string) []string {
	g := r.g
	nextSet := make(map[string]struct{})

	for _, name := range prev {
		if router, ok := g.routers[name]; ok {
			target := router(sess)
			if target != EndNode && target != "" {
				nextSet[target] = struct{}{}
			}
			continue
		}
		for _, to := range g.edges[name] {
			if to != EndNode {
				nextSet[to] = struct{}{}
			}
		}
	}

	out := make([]string, 0, len(nextSet))
	for n := range nextSet {
		out = append(out, n)
	}
	// 排序：让 frontier 顺序确定，便于 -race 之外的可复现性测试
	sort.Strings(out)
	return out
}
