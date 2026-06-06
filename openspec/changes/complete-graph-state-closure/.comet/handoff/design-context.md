# Comet Design Handoff

- Change: complete-graph-state-closure
- Phase: design
- Mode: compact
- Context hash: fba08db59eaa10ebc434b07b24b0b157f337be7b57afaf0276e491080450d03b

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/complete-graph-state-closure/proposal.md

- Source: openspec/changes/complete-graph-state-closure/proposal.md
- Lines: 1-28
- SHA256: 762506eeff9cf9b505416f6bf4d3ba6eeafd4e5f38e00ffa704ffc466c6e75e0

```md
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
```

## openspec/changes/complete-graph-state-closure/design.md

- Source: openspec/changes/complete-graph-state-closure/design.md
- Lines: 1-76
- SHA256: 6baa5c20a133d4ca1a5bbb7b470b8d6329151537742326c05390a146f64243ce

```md
# Design: complete Graph state closure

## Approach

采用混合方案：

- 运行时继续使用字段式 `domain.StatePatch`，保持类型安全和简单 apply 规则。
- checkpoint 层记录 `PatchSummary`，用于观测和验证，不参与业务 replay。
- 幂等层使用节点级 idempotency key，优先保护累计型节点。
- Store 层继续利用已有 row version / fencing 语义，不在本 change 中扩 schema。

## Stage 1: complete PatchNode coverage

迁移剩余 legacy 节点：

- `update_difficulty`：返回 `WorkingMemory` patch，不直接修改 `WorkingMemory.Difficulty`。
- `reflection_check`：返回 `PendingDecision + WorkingMemory` patch，不直接修改 `PendingDecision`、`ReflectTopic`、`ReflectionsUsed` 或 `DegradedReasons`。

保留旧 `NewUpdateDifficultyNode` 和 `NewReflectionCheckNode` wrapper，避免破坏现有单测和局部子图。

## Stage 2: checkpoint patch summary

新增观测结构，挂在 checkpoint 上：

```go
type PatchSummary struct {
    Node        string
    Writes      []string
    RoundID     string
    Fields      []string
    Degraded    []string
    Suspended   bool
    Idempotency string
}
```

runner 在 patch apply 成功后生成摘要并交给 checkpoint recorder。摘要只描述写入字段和关键状态，不保存完整 patch 对象。

## Stage 3: node idempotency

对累计型节点引入 idempotency key：

```text
session_id + ":" + round_id + ":" + node_name
```

优先保护：

- `update_memory`
- `update_difficulty`
- `reflection_check`

重复执行时返回空 patch 或保持已有结果，不重复累加 `AvgScore`、`DegradedRounds`、`Difficulty` streak、`ReflectionsUsed`。

## Stage 4: verification gate

扩展 `cmd/agent-verify` 或其内部 verifier：

- 检查核心 Interview Graph 节点是否注册为 `PatchNode`。
- 检查关键节点写集是否声明。
- 检查 checkpoint 是否包含 patch summary。
- 检查累计型节点是否具备 idempotency marker。
- 检查 `update_memory -> update_difficulty -> reflection_check` 的顺序。

## Compatibility

- `Session` JSON 可新增 `omitempty` 运行时幂等 marker；旧 Session 缺字段时必须兼容。
- HTTP/SSE payload 不变化。
- 数据库 schema 不变化。
- `NodeFunc` 不删除，legacy wrapper 保留。

## Risks

- 幂等 marker 如果放入 `WorkingMemory`，会增加运行时状态体积；需要只记录当前面试必要 marker。
- checkpoint patch summary 不能泄露过多 prompt 或答案内容，只记录字段和摘要。
- `reflection_check` 同时涉及 LLM、规则兜底和路由决策，迁移时必须保持降级不中断语义。
```

## openspec/changes/complete-graph-state-closure/tasks.md

- Source: openspec/changes/complete-graph-state-closure/tasks.md
- Lines: 1-9
- SHA256: 7c207d29c6953e87c9fdc9b6ff91953badd837079e70c55a0252565de5e220ab

```md
# Tasks

- [ ] 1. 迁移 `update_difficulty` 到 runner-level `PatchNode`，保留 wrapper 和现有行为。
- [ ] 2. 迁移 `reflection_check` 到 runner-level `PatchNode`，保留 wrapper 和降级语义。
- [ ] 3. 为 checkpoint 增加 patch summary 记录，不改变业务 replay 语义。
- [ ] 4. 为累计型节点增加节点级 idempotency marker 和重复执行保护。
- [ ] 5. 扩展 agent-verify 门禁，检查 patch 注册、写集、顺序和幂等风险。
- [ ] 6. 更新 `docs/SDD-Backend.md`、OpenSpec 主 spec 和 `docs/code-changes`。
- [ ] 7. 运行 `go test ./...`、OpenSpec strict 校验和 agent-verify。
```

## openspec/changes/complete-graph-state-closure/specs/graph-state-patch-runner/spec.md

- Source: openspec/changes/complete-graph-state-closure/specs/graph-state-patch-runner/spec.md
- Lines: 1-39
- SHA256: 3031b0a4e0450d2af9a0831d35c131822d25465b711b537e7b395305322ff60c

```md
## MODIFIED Requirements

### Requirement: Graph runner 必须支持 patch-aware 节点

Graph runner MUST support nodes that return `domain.StatePatch` and apply those patches centrally.

#### Scenario: difficulty update uses runner-level patch

- **WHEN** the Interview Graph registers `update_difficulty`
- **THEN** it SHOULD register it as a patch-aware node
- **AND** the runner should apply its `StatePatch`
- **AND** the legacy direct-call constructor should remain compatible

#### Scenario: reflection check uses runner-level patch

- **WHEN** the Interview Graph registers `reflection_check`
- **THEN** it SHOULD register it as a patch-aware node
- **AND** the runner should apply its `StatePatch`
- **AND** the legacy direct-call constructor should remain compatible

### Requirement: Graph checkpoint must include patch summary

Graph checkpoint MUST record a compact summary of applied patches for observability and verification.

#### Scenario: patch summary is recorded after successful apply

- **WHEN** a patch-aware node applies a `StatePatch` successfully
- **THEN** checkpoint data MUST include the node name, declared writes, affected round id and written fields
- **AND** checkpoint data MUST NOT store the full patch as a replay source

### Requirement: cumulative nodes must be idempotent

Cumulative Interview Graph nodes MUST avoid applying the same round-level effect more than once.

#### Scenario: repeated cumulative node execution is skipped

- **WHEN** a cumulative node sees an already applied idempotency key for the current round and node
- **THEN** it MUST avoid applying duplicate cumulative effects
- **AND** it MUST preserve compatibility with existing sessions that have no marker
```

## openspec/changes/complete-graph-state-closure/specs/state-patch-updates/spec.md

- Source: openspec/changes/complete-graph-state-closure/specs/state-patch-updates/spec.md
- Lines: 1-17
- SHA256: 7bb56079642f0382aea1eb95e8deca7a3eaead6127f1ce530b7328d0a7ab4937

```md
## MODIFIED Requirements

### Requirement: 通过 StatePatch 收敛关键 Session 写入

系统 MUST 支持用结构化 StatePatch 表达关键 Graph 节点对 Session 的写入，并通过统一入口应用这些写入。

#### Scenario: 写入当前面试内策略状态

- **WHEN** `update_difficulty` or `reflection_check` changes current-session strategy state
- **THEN** StatePatch SHOULD express the change through `WorkingMemory`
- **AND** the node SHOULD NOT directly mutate `Session.WorkingMemory`

#### Scenario: 写入反思后的下一步决策

- **WHEN** `reflection_check` decides to ask a new question, reflect, or end
- **THEN** StatePatch SHOULD express the decision through `PendingDecision`
- **AND** the node SHOULD NOT directly mutate `Session.PendingDecision`
```

