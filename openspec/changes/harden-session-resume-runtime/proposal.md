## Why

当前后端已经有 `Session.Suspension`、Graph checkpoint、StatePatch、Redis/PG 会话恢复和 SSE 事件流。问题不是缺少框架骨架，而是这些能力还没有完全收口到可靠的 HTTP 运行链路：

- `Answer` 仍主要按 `CurrentNode` 字符串判断等待位置，结构化 `Suspension` 还没有成为恢复主语义。
- Redis lease 覆盖了等待用户输入阶段，`Start` 首题暂停后不释放 lease，容易让另一实例的合法回答短暂 409。
- SSE 事件没有暴露 `suspension`、`trace_id` 和回放缺口标记，断线恢复时客户端只能靠 `phase/question/rounds` 推断状态。
- HTTP 错误响应缺少稳定 code 和 trace，排查跨 REST/SSE/Graph/Redis 链路时证据断裂。

下一阶段应先把“可恢复 Graph”变成更可靠的业务运行时，而不是继续扩展真实外部工具。

## What Changes

- `Answer` 写入答案时优先使用 `Session.Suspension.Node/Awaiting`，旧 Session 再回退 `CurrentNode`。
- Graph 暂停等待用户输入后释放 mutation lease；lease 只保护正在执行的 Start/Answer mutation。
- SSE `InterviewEvent` 增加兼容字段：`suspension,omitempty`、`trace_id,omitempty`、`replay_gap,omitempty`。
- HTTP 错误响应增加稳定 `code` 和 `trace_id`，并保留现有 `error` 文本。
- 更新 SDD 和验证文档，说明本阶段不实现 PG fencing token、真实 GitHub 工具或完整 LangGraph runtime。

## Capabilities

### Modified Capabilities

- `structured-session-suspension`: HTTP 恢复链路优先消费结构化 suspension，`CurrentNode` 仅作为兼容回退。
- `interview-session-runtime`: 新增运行时可靠性要求，覆盖 mutation lease、SSE 事件状态和错误 trace。

## Impact

- `internal/httpapi/interview_flow.go`: 调整答案写入和 lease 生命周期。
- `internal/httpapi/interview_events.go`: 扩展事件结构与构造逻辑。
- `internal/httpapi/interview_stream.go`: 回放缺口和事件输出兼容处理。
- `internal/httpapi/interview_errors.go`、`middleware_trace.go`: 结构化错误和 trace helper。
- `internal/httpapi/*_test.go`: 补充恢复、lease、SSE 和错误响应测试。
- `docs/SDD-Backend.md`: 更新 13.1 后续计划状态。

## Non-Goals

- 不实现真实 GitHub/Web/MCP 网络工具。
- 不实现 PG CAS / fencing token；该项需要单独阶段处理存储接口和 schema 语义。
- 不删除 `CurrentNode`。
- 不改变 Session JSON 的已存在字段语义。
- 不实现完整 LangGraph、time travel 或 runtime sub-agent。
