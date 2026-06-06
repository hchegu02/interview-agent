# Proposal: migrate graph non-suspend patch nodes

## 背景

项目已经引入 `domain.StatePatch` 和 `graph.PatchNode`，但 Interview Graph 里的业务节点仍主要是 legacy `NodeFunc`，节点内部自行调用 `domain.ApplyStatePatch`。这导致状态写入入口没有真正收口到 runner，checkpoint、写集声明和后续节点级审计也无法完全对齐。

## 目标

- 第一批只迁移不会返回 `ErrSuspended` 的业务节点：`retrieve_rag`、`evaluate`、`report`。
- 让这些节点通过 runner-level `PatchNode` 返回 `StatePatch`，由 `graph.Runnable` 统一 apply。
- 保留旧 `New*Node` API，避免破坏现有单测和直接调用场景。
- 保持 HTTP API、SSE、Session JSON 和数据库 schema 不变。

## 非目标

- 不迁移 `pick_next` 和 `probe_ask`。它们成功路径会返回 `ErrSuspended`，当前 runner 不会在 error 返回时 apply patch，直接迁移会丢失暂停前写入。
- 不改变 `PatchNodeFunc` 签名。
- 不引入 LangGraph 或运行时 sub-agent。

## 风险

- hook summary 过去依赖已写入的 `sess`，迁移后 patch 在函数返回后才 apply，必须改为从本地 patch/result 生成 summary。
- `WorkingMemory` 降级标记不能继续原地修改，否则会绕过 patch apply；需要通过 patch 替换记忆快照。
