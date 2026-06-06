## Why

阶段 4.1 已经提供 `agentkit` mock MCP tools，但业务 Skill 还没有使用工具。`project_polish` 仍只返回静态模板，无法体现“项目资料 -> 项目亮点提炼”的工具链路。

需要小步把 `ProjectPolishSkill` 接到 mock `github.project_analyze`，验证 Skill -> ToolRegistry -> MCPToolAdapter -> MockMCPClient 的运行链路，同时保持无工具、无 URL 时的兼容行为。

## What Changes

- `internal/skills.Registry` 支持可选注入 `agentkit.ToolRegistry`。
- `project_polish` 在输入中识别 GitHub URL 时，通过 `ToolRegistry` 调用 `github.project_analyze`。
- 工具调用成功时，把 mock 项目分析结果融合进项目亮点建议。
- 无工具、无 GitHub URL 或工具失败时，保持旧的静态项目润色建议。
- `cmd/server` 装配 mock MCP tool registry 并注入默认 agent service。

## Non-Goals

- 不接真实 GitHub API。
- 不接真实网页抓取。
- 不改变 `/api/agent/message` JSON 响应结构。
- 不新增前端类型。
- 不实现 runtime sub-agent、Gateway、Sandbox。
- 不让用户输入决定工具权限。

## Impact

- 影响代码：`internal/skills`、`internal/agent`、`cmd/server` 及测试。
- 影响文档：`docs/SDD-Backend.md`、`docs/code-changes`。
- 不改变数据库 schema、Session JSON 或前端类型。
- 无新增第三方依赖。
