# Design: migrate graph non-suspend patch nodes

## 现状

`graph.PatchNode` 已支持 `PatchNodeFunc(ctx, sess) (domain.StatePatch, error)`，runner 在节点成功返回后调用 `domain.ApplyStatePatch`。当前 `retrieve_rag`、`evaluate`、`report` 虽声明了写集，但实际仍通过 legacy `NodeFunc` 内部调用 `applyNodePatch`。

## 设计决策

### 1. 保留旧构造函数，新增 patch 构造函数

每个迁移节点新增 `New*PatchNode`，返回 `graph.PatchNodeFunc`：

- `NewRetrieveRAGPatchNode`
- `NewEvaluatePatchNode`
- `NewReportPatchNodeWithHook`

旧函数继续返回 `graph.NodeFunc`，内部调用 patch 函数并立即 `applyNodePatch`。这样单测、直接调用和已有外部代码保持兼容。

### 2. Hook summary 不能依赖 apply 后的 Session

Patch 函数在返回前发 after hook，此时 runner 还没 apply patch。因此 summary 必须从本地变量构造：

- retrieve：根据本地候选池长度和降级原因生成。
- evaluate：根据本地 `Evaluation` 生成。
- report：根据本地 `Report` 生成。

### 3. 降级标记通过 WorkingMemory patch 返回

`retrieve_rag` 和 `evaluate` 的降级路径会写 `WorkingMemory.DegradedReasons`。迁移后不能原地写 `sess.WorkingMemory`，改为复制一份 `WorkingMemory`，在副本上写降级原因，并把副本放入 `StatePatch.WorkingMemory`。

### 4. 不迁移挂起节点

当前 runner 在 `PatchNodeFunc` 返回 error 时不会 apply patch。`pick_next` / `probe_ask` 的成功暂停路径依赖 `ErrSuspended`，必须等后续设计“patch-on-suspend”语义后再迁移。

## 兼容性

- 不新增 Session JSON 字段。
- 不改变 HTTP/SSE payload。
- 不改变数据库 schema。
- 不改变 `graph.NodeFunc` 和旧 `New*Node` API。
