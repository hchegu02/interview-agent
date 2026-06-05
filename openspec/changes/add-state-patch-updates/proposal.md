## Why

当前节点直接修改 `*domain.Session`。在线性流程里这个做法足够简单，但随着 RAG、选题、评分、报告、后续 Skill 和 Tool 审批节点增多，写入边界会越来越模糊：

- 很难看出某个节点到底改了哪些字段。
- 并发 frontier 中多个节点如果写同一块状态，风险只能靠约定控制。
- 后续 Graph checkpoint 只能记录完整 Session，无法解释“哪个节点产生了哪次状态变化”。

项目不需要全量迁移到 LangGraph，但可以吸收 state update / reducer 的优点：让关键节点把写入收敛到结构化 patch，再由统一入口 apply。

## What Changes

- 新增轻量 `StatePatch`，表达少数高风险字段的写入意图。
- 新增统一 apply 函数，集中处理 overwrite、append、merge 等规则。
- 第一阶段只覆盖 `retrieve_rag`、`pick_next`、`evaluate`、`report` 等节点的核心写入。
- 保持 `graph.NodeFunc(ctx, *Session) error` 签名不变，避免一次性重写 Graph runner。
- 为后续 Graph checkpoint、并发写保护和 agent-verify 细粒度验证打基础。

## Capabilities

### New Capabilities

- `state-patch-updates`: 系统可以用结构化 patch 表达关键 Graph 节点的 Session 写入，并通过统一入口应用。

### Modified Capabilities

<!-- No existing main spec capability is modified yet. -->

## Impact

- `internal/domain`: 新增 StatePatch 数据结构和 apply 规则。
- `internal/nodes`: 逐步让高风险节点通过 patch helper 写入关键字段。
- `internal/graph`: 第一阶段不改变 NodeFunc 签名；后续可在 checkpoint 中记录 patch。
- `internal/domain` / `internal/nodes` 测试：补充 patch apply 和节点行为等价测试。
- 不改变 HTTP API、Session JSON、前端类型和数据库 schema。
