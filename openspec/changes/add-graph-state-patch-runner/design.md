## Context

当前 Graph runner 已完成三件事：

- `Suspension` 让暂停语义结构化。
- `StatePatch` 让部分节点把写入集中到 `domain.ApplyStatePatch`。
- `GraphCheckpoint` 记录 runner 执行证据。

但写入模型还没有统一：legacy 节点仍能直接改 `Session`，并发 frontier 缺少写集检查。B 方案的核心不是立刻改光所有节点，而是先让 Graph 有能力区分 legacy 节点和 patch-aware 节点，并在并发执行前做安全检查。

## Design

### 1. NodeSpec

新增节点规格：

```go
type NodeSpec struct {
    Name   string
    Fn     NodeFunc
    Writes []string
}
```

`Writes` 使用稳定字段名，不直接使用 Go struct path。第一版建议使用粗粒度 key：

- `candidate_pool`
- `retrieval_trace`
- `pending_decision`
- `rounds`
- `current_evaluation`
- `current_round_completion`
- `report`
- `status`
- `working_memory`
- `suspension`

`AddNode(name, fn)` 继续存在，生成 legacy spec。新增 `AddNodeSpec(spec)` 用于声明写集。

### 2. Patch-aware Node

新增 patch-aware 节点类型：

```go
type PatchNodeFunc func(context.Context, *domain.Session) (domain.StatePatch, error)
```

并提供注册 helper：

```go
func PatchNode(name string, writes []string, fn PatchNodeFunc) NodeSpec
```

runner 执行 patch-aware 节点后调用 `domain.ApplyStatePatch`。如果 apply 失败，返回带节点名的 permanent error。

### 3. 并发写保护

执行并发 frontier 前检查：

- 任一节点是 legacy 且没有 `Writes`：拒绝并发执行。
- 两个节点 `Writes` 有交集：拒绝并发执行。
- 线性 frontier 不强制 `Writes`，保持兼容。

这会牺牲一部分“随便 fan-out”的自由，但能把并发写冲突在 runner 边界提前暴露。

### 4. 迁移策略

第一版在 Graph 层验证 patch-aware 模型，并在业务图装配层为关键节点声明写集，避免大范围业务节点重写：

- `retrieve_rag`
- `pick_next`
- `evaluate`
- `report`

这些业务节点函数当前仍保持 `NodeFunc` 兼容形态；后续可以逐步改成真正返回 `StatePatch` 的 patch-aware 节点。其它节点继续 legacy。业务图如果没有并发 frontier，不会被强制迁移。

### 5. 边界

- 不删除 `NodeFunc`。
- 不改变 HTTP API、SSE、Session JSON。
- 不实现完整 LangGraph runtime。
- 不实现 patch checkpoint 持久化。
- 不强制所有节点一次性迁移。

## Risks

- 写集过粗会拒绝一些实际安全的并发组合；第一版宁可保守。
- legacy 节点仍可在线性执行中直接改 Session；这是兼容性取舍。
- patch-aware 节点如果在返回 patch 前直接改 Session，runner 无法自动检测；需要测试和节点规范配合。

## Verification

- `go test ./internal/graph -count=1`
- `go test ./internal/graphs -count=1`
- `go test ./internal/nodes -count=1`
- `go test ./... -count=1`
- `openspec validate add-graph-state-patch-runner --strict`
