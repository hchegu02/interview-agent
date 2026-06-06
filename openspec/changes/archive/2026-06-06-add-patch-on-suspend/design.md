# Design: patch-on-suspend

## 设计目标

支持“节点产生状态更新后暂停”的业务语义，同时保持普通错误不写 Session。

## 核心接口

新增 Graph helper：

```go
func SuspendWithPatch(err error) error
func IsPatchSuspend(err error) bool
```

`SuspendWithPatch` 只作为错误 marker，不携带 patch。patch 仍由 `PatchNodeFunc` 的第一个返回值提供。

## Runner 行为

`Runnable.callNode` 执行 patch-aware 节点时：

1. 调用 `spec.patch(ctx, sess)`，得到 `(patch, err)`。
2. 如果 `err == nil`，保持现有逻辑：apply patch 后返回 nil。
3. 如果 `err != nil && IsPatchSuspend(err)`，先 apply patch，再返回该错误。
4. 如果 `err != nil` 但不是 patch suspend，直接返回错误，不 apply patch。

这样 `run()` 仍通过 `errors.Is(err, ErrSuspended)` 进入现有暂停逻辑，不改变 `CurrentNode` / `Suspension` 处理。

## pick_next 迁移

新增 `NewPickNextPatchNode`：

- 终止路径返回 `PendingDecision(Action=end)` patch，error nil。
- 出题路径返回 `PendingDecision(Action=ask_new)`、`AppendRound`、`WorkingMemory` patch，并返回 `SuspendWithPatch(...)`。
- LLM 降级和 reflect topic 消费只修改 `WorkingMemory` 副本，不提前修改 `sess.WorkingMemory`。

旧 `NewPickNextNode` 保留 wrapper：遇到 `IsPatchSuspend(err)` 时先 apply patch，再返回 suspend error，保证旧单测和直接调用兼容。

## 边界

- 本阶段不改 `PatchNodeFunc` 签名。
- 本阶段不迁移 `probe_ask`，因为它需要 `StatePatch` 支持 `AppendFollowUp` 和当前轮 `CriticResult` 更新。
- 不改变 HTTP API、SSE payload、Session JSON 或数据库 schema。
