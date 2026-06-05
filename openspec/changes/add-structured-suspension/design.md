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

- `map[string]any` payload 过度自由 -> 第一版只允许简单 JSON 值，并在节点 helper 中收口。
- 同时维护 `CurrentNode` 和 `Suspension` 可能出现不一致 -> 暂停写入和 Resume 读取必须集中在 graph runner/helper。
- 前端依赖暂停信息做业务判断 -> SDD 和类型注释中明确它是展示/诊断字段。
