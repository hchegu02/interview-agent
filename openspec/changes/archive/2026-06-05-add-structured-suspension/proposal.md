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
