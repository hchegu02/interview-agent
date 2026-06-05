---
comet_change: add-structured-suspension
role: technical-design
canonical_spec: openspec
---

# Structured Suspension Design

## Context

当前 Graph 暂停依赖 `ErrSuspended + Session.CurrentNode`。它能支持用户答题后的恢复，但表达能力太薄：只知道暂停节点，不知道暂停原因、等待输入类型和给前端展示的上下文。

本次设计目标是吸收 LangGraph interrupt 的可取点，但不迁移到 LangGraph，也不重写当前 Go Graph runner。

## Approach

在 `domain.Session` 上新增可选字段 `Suspension`：

```go
type Suspension struct {
    Node      string         `json:"node"`
    Reason    string         `json:"reason,omitempty"`
    Awaiting  string         `json:"awaiting"`
    Payload   map[string]any `json:"payload,omitempty"`
    CreatedAt time.Time      `json:"created_at"`
}
```

第一版只要求 `AwaitingAnswer`，为后续 `approval`、`tool_review` 预留常量。

## Compatibility

`CurrentNode` 必须保留：

- 老 Session 只包含 `current_node` 时仍可恢复。
- 新暂停同时写 `CurrentNode` 和 `Suspension.Node`。
- Resume 优先使用 `Suspension.Node`，缺失时回退 `CurrentNode`。
- 恢复推进后清理过期 `Suspension`，避免前端误判还在等待输入。

## Graph Integration

最小改动点在 `internal/graph`：

- 捕获 `ErrSuspended` 时集中写入默认 Suspension。
- 如果节点已经预先写入自定义 Suspension，则补齐缺失的 `Node`、`Awaiting`、`CreatedAt`。
- `Resume` 读取暂停节点时使用 helper，避免业务代码分散判断。

不改变 `NodeFunc` 签名，不要求所有节点改成返回 rich interrupt。

## HTTP / Frontend Contract

HTTP 响应只暴露可选 `suspension`：

- 后端构造响应时深拷贝 Suspension，避免 map payload 被调用方修改。
- 前端 `Session` 类型新增 `suspension?: Suspension`。
- 前端只把它作为展示和诊断字段，不参与恢复决策。

## Risks

- `Payload map[string]any` 过于自由：第一版仅用于简单 JSON 值，后续如有复杂 tool review payload，再做具名结构。
- `CurrentNode` 与 `Suspension.Node` 不一致：所有写入和读取集中在 graph helper 内。
- 恢复后未清理 Suspension：通过 Graph Resume 测试覆盖。

## Testing Strategy

- `internal/graph`：暂停时写入 Suspension。
- `internal/graph`：旧 Session 只有 `CurrentNode` 仍可 Resume。
- `internal/graph`：恢复成功后清理过期 Suspension。
- `internal/httpapi`：响应包含 Suspension 深拷贝。
- `web/src/types.ts`：类型能通过前端测试和 build。

## Non-Goals

- 不引入 LangGraph。
- 不实现 checkpoint / time-travel。
- 不实现 sub-agent runtime。
- 不重写 Graph runner。
