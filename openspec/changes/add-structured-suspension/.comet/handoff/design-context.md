# Comet Design Handoff

- Change: add-structured-suspension
- Phase: design
- Mode: compact
- Context hash: 2a7612510c78781f5c19a6f29aab2e40cf742bd64ffcabbb247a470249c2e5f9

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/add-structured-suspension/proposal.md

- Source: openspec/changes/add-structured-suspension/proposal.md
- Lines: 1-32
- SHA256: ce27b649ec16b2740d0718386d98cb960ecfbcc42da614b7737c2572b6069408

```md
## Why

当前 Graph 暂停依赖 `ErrSuspended + Session.CurrentNode`。这个设计足够简单，但只能说明“停在哪个节点”，不能结构化表达“为什么暂停、等待什么输入、需要给前端或审计展示什么 payload”。

后续要做 Intent Router、Skill、Long-term Memory、动态难度和 Tool/MCP Adapter 时，会出现更多等待用户输入、人工确认或工具 review 的场景。继续只靠 `CurrentNode` 会让恢复语义和前后端接口越来越含糊。

## What Changes

- 在 `domain.Session` 中新增向后兼容的结构化暂停字段，用于表达暂停节点、原因、等待类型、payload 和创建时间。
- 保留 `CurrentNode`，确保旧 Session 和现有 `Resume` 语义不被破坏。
- 调整 Graph 暂停/恢复路径，使新逻辑写入结构化暂停信息，并在恢复后清理。
- 在 HTTP 响应和前端类型中暴露暂停信息，供页面和 SSE 排障使用。
- 补充单元测试和兼容性测试。

## Capabilities

### New Capabilities

- `structured-session-suspension`: 系统可以结构化记录 Graph 暂停原因和等待输入类型，同时兼容现有 `CurrentNode` 恢复机制。

### Modified Capabilities

<!-- No existing main spec capability is modified yet. -->

## Impact

- `internal/domain/session.go`: 新增 `Suspension` 领域结构和 `Session.Suspension` 字段。
- `internal/graph`: 调整暂停/恢复写入和读取逻辑。
- `internal/httpapi`: 响应结构和测试补充暂停信息。
- `web/src/types.ts`: 前端 Session 类型新增可选 suspension。
- 不引入新外部依赖。
- 不实现完整 LangGraph，不实现 sub-agent runtime。
```

## openspec/changes/add-structured-suspension/design.md

- Source: openspec/changes/add-structured-suspension/design.md
- Lines: 1-83
- SHA256: aed60b3430bcf0330e60be5f3d8f7b035cdc8314724ebb64eaf2c1b9e3aa05cc

[TRUNCATED]

```md
## Context

当前项目的 Graph runner 以 `Session` 聚合根为共享状态，节点返回 `graph.ErrSuspended` 时，runner 把节点名写到 `Session.CurrentNode`。HTTP 层随后保存 Session；用户提交回答后，服务加载 Session 并调用 `Runnable.Resume`，从暂停节点的下游继续执行。

这个模型简单、可解释、兼容当前业务。但它缺少结构化暂停语义：

- 不知道暂停原因。
- 不知道等待输入类型。
- 不知道前端应该展示什么上下文。
- 不方便未来工具审批、人类确认、skill 中断等场景复用。

## Goals / Non-Goals

**Goals:**

- 引入结构化 `Suspension`，但保留 `CurrentNode`。
- 暂停时写入节点、原因、等待类型、payload 和时间。
- 恢复时优先使用 `Suspension.Node`，缺失时回退 `CurrentNode`。
- HTTP 和前端只暴露可序列化、向后兼容的可选字段。
- 为后续 checkpoint、StatePatch 和工具审批打基础。

**Non-Goals:**

- 不重写 Graph runner。
- 不引入 LangGraph。
- 不实现 sub-agent runtime。
- 不实现完整 checkpoint/time-travel。
- 不把长期记忆塞进 `Session`。

## Decisions

### Decision 1: Add `Session.Suspension` as an optional field

新增字段使用 `omitempty`，保证旧 JSON 可继续反序列化，老前端也不会被迫消费。

```go
type Suspension struct {
    Node      string         `json:"node"`
    Reason    string         `json:"reason,omitempty"`
    Awaiting  string         `json:"awaiting"`
    Payload   map[string]any `json:"payload,omitempty"`
    CreatedAt time.Time      `json:"created_at"`
}
```

`Awaiting` 第一版建议只定义常量：

- `answer`
- `approval`
- `tool_review`

当前业务先使用 `answer`。

### Decision 2: Keep `CurrentNode` as compatibility field

不能删除 `CurrentNode`。它已经被 PG/Redis Session JSON、HTTP 响应、Graph Resume 和测试依赖。

第一版规则：

- 暂停时同时写 `CurrentNode` 和 `Suspension.Node`。
- Resume 时优先读 `Suspension.Node`。
- 如果没有 `Suspension`，继续读 `CurrentNode`。
- 成功继续执行后清理 `Suspension`，`CurrentNode` 仍由节点执行过程维护。

### Decision 3: Do not make NodeFunc return a rich interrupt yet

更彻底的方案是把 `ErrSuspended` 替换成 `Interrupt` 类型。但这会扩大改动面，影响所有暂停节点和测试。

本次先提供最小兼容扩展：

- 保持 `ErrSuspended`。
- runner 在捕获 suspend 时写默认 Suspension。
- 需要自定义 payload 的节点后续可通过 helper 预先写入 `sess.Suspension`。

### Decision 4: HTTP/frontend expose read-only suspension

前端只展示和诊断，不参与恢复决策。恢复仍由后端 `Resume` 控制。

## Risks / Trade-offs

```

Full source: openspec/changes/add-structured-suspension/design.md

## openspec/changes/add-structured-suspension/tasks.md

- Source: openspec/changes/add-structured-suspension/tasks.md
- Lines: 1-20
- SHA256: d22d93f7cb19b8eed98a78b5532e397da9624d7b9134bbc3e6989796c5e3fcd5

```md
## 1. Domain And Graph

- [ ] 1.1 在 `internal/domain/session.go` 新增 `Suspension` 结构和 `Session.Suspension` 可选字段。
- [ ] 1.2 在 `internal/graph` 中集中处理暂停写入：保留 `CurrentNode`，同时写入默认 `Suspension`。
- [ ] 1.3 调整 `Runnable.Resume`：优先从 `Suspension.Node` 恢复，缺失时回退 `CurrentNode`。
- [ ] 1.4 恢复成功推进后清理过期 `Suspension`。

## 2. HTTP And Frontend Contract

- [ ] 2.1 在 `internal/httpapi` 响应中深拷贝并返回可选 `suspension`。
- [ ] 2.2 在 `web/src/types.ts` 增加 `Suspension` 类型和 `Session.suspension` 字段。

## 3. Tests And Verification

- [ ] 3.1 增加 Graph 暂停时写入 `Suspension` 的测试。
- [ ] 3.2 增加旧 Session 只有 `CurrentNode` 仍可 Resume 的兼容测试。
- [ ] 3.3 增加 HTTP 响应包含 suspension 深拷贝测试。
- [ ] 3.4 运行 `go test ./internal/graph ./internal/httpapi -count=1`。
- [ ] 3.5 运行 `npm --prefix web run test`。
- [ ] 3.6 更新 `docs/SDD-Backend.md` 中 Session / Graph 后续计划的实现状态。
```

## openspec/changes/add-structured-suspension/specs/structured-session-suspension/spec.md

- Source: openspec/changes/add-structured-suspension/specs/structured-session-suspension/spec.md
- Lines: 1-28
- SHA256: f3fb2727925c0058bfa8a7a6d6479ff6caad6b4821cc44c54dbf9c31e929a3a5

```md
## ADDED Requirements

### Requirement: 结构化记录 Graph 暂停信息

系统 MUST 在 Graph 暂停等待外部输入时，保存结构化暂停信息，并继续兼容现有 `current_node` 字段。

#### Scenario: 节点暂停等待用户回答

- **WHEN** Graph 节点返回 `ErrSuspended`
- **THEN** Session 包含 `suspension.node`
- **AND** Session 包含 `suspension.awaiting`
- **AND** Session 继续保留 `current_node`

#### Scenario: 旧 Session 没有 suspension

- **WHEN** 服务恢复一个只有 `current_node`、没有 `suspension` 的旧 Session
- **THEN** Graph Resume 仍然可以从 `current_node` 的下游继续执行

#### Scenario: 前端读取会话详情

- **WHEN** HTTP API 返回 Session
- **THEN** 响应可以包含可选 `suspension`
- **AND** 缺失 `suspension` 不影响老会话展示

#### Scenario: 恢复后清理暂停信息

- **WHEN** Graph 从暂停状态成功恢复执行
- **THEN** Session 中过期的 `suspension` 不应继续表示当前仍在等待外部输入
```

