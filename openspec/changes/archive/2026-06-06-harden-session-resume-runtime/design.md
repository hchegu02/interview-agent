## Context

Interview Agent 的 Graph 运行时已经具备结构化暂停、checkpoint、StatePatch 和写集保护。但 HTTP 运行链路还停留在“可恢复”的第一版：能从 Session 继续推进，但 lease、SSE、错误和 trace 的语义不够硬。

本阶段目标是把恢复链路收紧到工程业务可用的程度，同时控制改动面。核心原则：优先修正运行时边界，不扩展外部工具和数据库 schema。

## Goals / Non-Goals

**Goals:**

- 让 `Suspension` 成为回答恢复的主语义。
- 让 lease 只覆盖正在执行的 mutation，不占用等待用户输入阶段。
- 让 SSE 事件包含足够恢复状态：suspension、trace、replay gap。
- 让 HTTP 错误响应具备稳定 code 和 trace id。
- 保持旧 Session、旧前端和现有响应字段兼容。

**Non-Goals:**

- 不接真实 GitHub API。
- 不引入 PG fencing token / CAS。
- 不重写 Graph runner。
- 不删除 `CurrentNode`。
- 不改数据库 schema。

## Decisions

### Decision 1: Answer uses Suspension first

`fillPendingAnswer` 应先读取：

```text
Session.Suspension.Awaiting == "answer"
Session.Suspension.Node
```

然后按节点语义写入主问题或追问答案。只有 `Suspension` 缺失时，才回退 `CurrentNode`。这样后续 `approval`、`tool_review` 不会被误当成普通答案提交。

兼容策略：

- `pick_next` 仍写主问题答案。
- `probe_ask` 仍写最后一个追问答案。
- 旧 Session 只有 `CurrentNode` 时继续可用。
- `Suspension.Awaiting` 不是 `answer` 时返回 invalid state。

### Decision 2: Lease protects mutation, not idle waiting

Redis lease 用来避免多实例同时执行同一个 session mutation。它不应该覆盖“Graph 已暂停，正在等用户输入”的空闲阶段。

本阶段规则：

- `Start` 获取 lease 后执行 Graph。
- 如果 Graph 正常暂停并保存 Session，释放 lease。
- `Answer` 获取/续租 lease 后执行 Resume。
- 如果 Resume 后再次暂停，保存后释放 lease。
- 如果 session completed，保存后释放 lease。
- 执行失败时沿用现有失败释放逻辑。

暂不做 heartbeat 和 fencing token；这两个属于长 LLM 调用与旧 writer 防护问题，需要后续单独设计。

### Decision 3: SSE event exposes runtime state

REST response 已有 `suspension`，SSE event 应补齐同一类状态，避免断线恢复时只能猜。

事件新增字段全部 `omitempty`：

```go
Suspension *domain.Suspension `json:"suspension,omitempty"`
TraceID    string             `json:"trace_id,omitempty"`
ReplayGap  bool               `json:"replay_gap,omitempty"`
```

`suspension` 使用与 REST response 一致的 clone 规则。`trace_id` 来自请求 context。`replay_gap` 用于提示客户端：历史事件可能被裁剪，应以 snapshot 为准。

### Decision 4: Error response keeps compatibility but adds code

保留现有 `error` 字段，同时增加：

```json
{
  "code": "lease_conflict",
  "error": "...",
  "trace_id": "..."
}
```

第一版 code 集合：

- `session_not_found`
- `invalid_state`
- `lease_conflict`
- `invalid_config`
- `bad_request`
- `internal_error`

HTTP status 继续按现有语义映射，必要处再细化。

## Risks / Trade-offs

- 释放 lease 后仍可能有极长 LLM 调用超时导致旧 writer 写回；本阶段不解决，需要 PG CAS/fencing token 后续处理。
- SSE replay gap 在内存 hub 中只能基于 history 判断，Redis Streams 的精确裁剪检测更复杂；第一版可以保守标记。
- `Suspension.Payload` 当前是浅 clone，不能放复杂嵌套敏感数据；本阶段不扩大 payload 使用。
- 错误 code 新增是兼容字段，但前端如果开始依赖 code，后续需要保持稳定。

## Validation

- `go test ./internal/httpapi -count=1`
- `go test ./internal/graph ./internal/httpapi -count=1`
- `go test ./... -count=1`
- `openspec validate harden-session-resume-runtime --strict`
