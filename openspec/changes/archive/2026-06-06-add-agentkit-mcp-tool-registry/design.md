## Context

当前 `agentkit` 已经具备核心原语：

- `Tool`
- `ToolSpec`
- `ToolCall`
- `ToolResult`
- `ToolRegistry`
- `MCPClient`
- `MCPToolAdapter`
- `Hook`
- `Permission`

这些已经覆盖阶段 4 的主要边界，因此下一步应该扩展 `internal/agentkit`，而不是新建一套 `internal/tools`。

## Goals / Non-Goals

**Goals:**

- 提供可测试的 mock MCP client。
- 提供默认 MCP 工具注册函数。
- 工具调用仍由 `ToolRegistry.Call` 统一处理权限、超时和 hook。
- 工具输出结构化，便于后续 Skill 消费。
- 对外只声明 mock 工具能力，不夸大成真实 MCP runtime。

**Non-Goals:**

- 不接真实网络。
- 不接真实 MCP server。
- 不修改 HTTP 层。
- 不修改 `internal/skills` 行为。
- 不新增运行时 sub-agent。

## Decisions

### 1. 复用 `internal/agentkit`

`ToolRegistry` 和 `MCPToolAdapter` 已经存在，重复新建 `internal/tools` 会让工具接口分裂。阶段 4 只在 `agentkit` 内补默认注册和 mock 实现。

### 2. mock 工具只做 deterministic 输出

`github.project_analyze` 和 `web.fetch` 第一版只返回确定性 mock 结果。这样可以在无网络、无 token、无外部 MCP server 的本地环境里稳定测试工具链路。

### 3. 工具权限固定在后端注册阶段

工具的 `Permission` 来自 `ToolSpec`，调用方必须传入匹配权限。后续 HTTP 或 Skill 接入时，权限不能由用户输入直接决定。

### 4. 不接入 Skill 行为

本 change 只完成工具注册基础。`ProjectPolishSkill` 是否调用 `github.project_analyze` 留到下一阶段，避免一次 change 同时改 registry、skill 和 HTTP 行为。

## Data Flow

```text
RegisterDefaultMCPTools
  -> ToolRegistry.Register(MCPToolAdapter)
  -> ToolRegistry.Call
  -> permission / timeout / before hook
  -> MCPToolAdapter.Call
  -> MockMCPClient.CallTool
  -> structured ToolResult
  -> after hook
```

## Error Handling

- 未知工具继续返回 `ErrToolNotFound`。
- 权限不匹配继续返回 `ErrPermissionDenied`。
- mock client 找不到工具时返回 `ErrToolNotFound`。
- 超时仍由 `ToolRegistry` 处理。

## Tests

- `ToolRegistry.List` 或等价查询能力按名称排序返回工具 spec。
- 默认 MCP 工具注册后包含 `github.project_analyze` 和 `web.fetch`。
- 调用 mock `github.project_analyze` 返回项目摘要、语言、亮点和风险点。
- 调用 mock `web.fetch` 返回 URL、标题和正文摘要。
- 权限不匹配时不能绕过 `ToolRegistry`。

## Risks / Trade-offs

- [Risk] mock 工具被误解为真实外部工具。Mitigation：命名和 SDD 明确为 mock / foundation。
- [Risk] 后续 Skill 直接依赖 mock 输出格式。Mitigation：输出保持结构化但简单，下一阶段接入时再定义 Skill 输入契约。
- [Risk] `ToolRegistry` 权限检查只是边界校验，不是完整授权系统。Mitigation：HTTP 接入前必须由后端映射权限，不能信任用户传参。
