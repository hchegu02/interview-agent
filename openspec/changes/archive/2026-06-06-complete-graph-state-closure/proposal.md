## Why

Interview Graph 已经把 `retrieve_rag`、`pick_next`、`evaluate`、`critic`、`refine`、`probe_ask`、`probe_eval`、`update_memory`、`report` 迁移到 runner-level `PatchNode`，但 Graph 状态收口还没有形成工程闭环：

- `update_difficulty` 和 `reflection_check` 仍是 legacy 节点，直接修改 `WorkingMemory` 和 `PendingDecision`。
- checkpoint 只能看到完整 Session 快照，不能直接看到节点 patch 写入摘要。
- `update_memory`、`update_difficulty`、`reflection_check` 等累计型节点存在重复执行导致重复累加的风险。
- `agent-verify` 尚未检查核心节点注册方式、patch 顺序、写集和幂等风险。

这会让恢复、重试、排障和后续并发扩展仍依赖人工约定。需要把 Graph 状态收口从“节点迁移”推进到“可验证、可排障、可防重放”的工程交付状态。

## What Changes

- 完成核心 Interview Graph 剩余 legacy 节点的 PatchNode 迁移：`update_difficulty`、`reflection_check`。
- 为 checkpoint 增加 patch 摘要记录能力，但不把 checkpoint 变成业务 replay 机制。
- 为累计型节点设计并实现节点级幂等 key，防止重复执行污染运行时记忆。
- 扩展 agent-verify 门禁，检查 PatchNode 注册、写集、patch 顺序和幂等风险。
- 更新 SDD、OpenSpec 和过程文档，明确这仍是 Go 自研 Graph，不是 LangGraph runtime。

## Non-Goals

- 不迁移到 LangGraph。
- 不把 `StatePatch` 改成通用 JSON Patch 或 op log。
- 不删除 `NodeFunc` 和 legacy wrapper。
- 不修改 HTTP/SSE payload。
- 不修改数据库 schema。
- 不实现完整 time travel、业务 replay 或 checkpoint 回滚。
- 不把 Codex sub-agent 开发方式写成运行时能力。
