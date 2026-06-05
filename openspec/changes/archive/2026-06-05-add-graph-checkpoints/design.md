## Context

当前 Graph runner 在 `Runnable.run` 中按 frontier 执行，每轮调用 `executeBatch`，节点前后只触发现有 `graph.Callback`：

```go
type Callback interface {
    OnNodeStart(ctx context.Context, node string, sess *domain.Session)
    OnNodeEnd(ctx context.Context, node string, sess *domain.Session)
    OnNodeError(ctx context.Context, node string, sess *domain.Session, err error)
}
```

Callback 适合 SSE、metrics、tracing，但它拿不到 `step`、`frontier`、resume 来源，也无法区分 batch 级事件。因此 checkpoint 不应塞进现有 Callback，而应作为 runner 级 recorder。

## Goals / Non-Goals

**Goals:**

- 记录 Graph 执行过程中的关键状态快照。
- 保持 `NodeFunc` 和现有 Callback 接口不变。
- 线性 frontier 记录节点级 before / after / error。
- 并发 frontier 只记录 batch 级 before / after / error，避免并发读写 Session 造成不可靠快照。
- suspend 后记录结构化暂停信息。
- resume 时记录从哪个 suspended node 恢复到哪个 next frontier。
- 提供内存 ring buffer，供测试、debug 和后续验证工具读取。

**Non-Goals:**

- 不实现 time travel UI。
- 不实现 checkpoint 回滚。
- 不写 PostgreSQL checkpoint 表。
- 不改变 HTTP API、SSE 事件结构或 Session JSON。
- 不保证并发 frontier 内单个节点的精确写入归因。
- 不把 `StatePatch` 当作唯一事实源，因为仍有节点直接写 Session。

## Decisions

### Decision 1: Add runner-level CheckpointRecorder

新增：

```go
type CheckpointPhase string

const (
    CheckpointFrontierBefore CheckpointPhase = "frontier_before"
    CheckpointFrontierAfter  CheckpointPhase = "frontier_after"
    CheckpointFrontierError  CheckpointPhase = "frontier_error"
    CheckpointNodeBefore     CheckpointPhase = "node_before"
    CheckpointNodeAfter      CheckpointPhase = "node_after"
    CheckpointNodeError      CheckpointPhase = "node_error"
    CheckpointSuspended      CheckpointPhase = "suspended"
    CheckpointResumeFrom     CheckpointPhase = "resume_from"
)

type GraphCheckpoint struct {
    Seq       int64
    Step      int
    Graph     string
    Phase     CheckpointPhase
    Frontier  []string
    Node      string
    Error     string
    Snapshot  []byte
    CreatedAt time.Time
}

type CheckpointRecorder interface {
    RecordCheckpoint(ctx context.Context, checkpoint GraphCheckpoint)
}
```

`Seq` 由 recorder 分配或 runner 单调递增，避免 `Resume` 后 `Step` 重新从 1 开始导致排序含糊。

### Decision 2: Do not change NodeFunc

节点签名保持：

```go
type NodeFunc func(ctx context.Context, sess *domain.Session) error
```

checkpoint 在 runner 内部围绕 batch / node 执行记录，不要求业务节点感知。

### Decision 3: Snapshot strategy is JSON bytes

第一版 snapshot 使用 `json.Marshal(sess)`，存为 `[]byte`。

规则：

- snapshot 失败时记录 checkpoint 的 `Error`，不让业务流程失败。
- `Frontier` slice 必须复制。
- `Snapshot` 作为不可变字节保存。
- 内存 recorder 返回 snapshot 时也必须复制，避免测试或调用方污染内部 buffer。

### Decision 4: Parallel frontier is batch-only

当 `len(frontier) > 1` 时，不记录单节点 before / after snapshot。原因：

- 多个节点会并发写同一个 `Session`。
- 在节点运行中 marshal Session 会与写入竞争。
- 当前 `CurrentNode` 在并发节点中也存在任意覆盖风险，不能把它当稳定节点归因。

第一版只记录：

- `frontier_before`
- `frontier_after`
- `frontier_error`

线性 frontier 才额外记录：

- `node_before`
- `node_after`
- `node_error`

### Decision 5: Ring buffer is the first storage

第一版新增内存 ring buffer recorder：

```go
type MemoryCheckpointRecorder struct { ... }
func NewMemoryCheckpointRecorder(capacity int) *MemoryCheckpointRecorder
func (r *MemoryCheckpointRecorder) RecordCheckpoint(ctx context.Context, cp GraphCheckpoint)
func (r *MemoryCheckpointRecorder) Snapshot() []GraphCheckpoint
```

容量小于等于 0 时使用安全默认值。只保留最近 N 条，避免调试功能吃内存。

## Risks / Trade-offs

- JSON snapshot 可能较大：第一版只通过可选 recorder 启用，不默认生产开启。
- 并发 frontier 无法做节点级精确归因：明确降级为 batch 级记录，避免假精确。
- Recorder 不能影响业务流程：runner 会隔离 recorder panic，并用短超时限制慢 recorder 对 Graph 延迟的影响。自定义 recorder 仍必须尊重 `context.Context`，否则 runner 无法强制终止该 recorder 的后台执行。Checkpoint 仍建议只在 debug、测试和验证场景开启。
- 如果后续要落 PG，需要单独 OpenSpec 设计表结构、清理策略和开关。
