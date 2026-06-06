## Why

项目已经有 `internal/agentkit.ToolRegistry`、`MCPToolAdapter`、Hook 和权限边界，但阶段 4 的 MCP Adapter 还停留在抽象层：没有默认工具注册、没有可测试的 mock MCP 工具，也没有明确哪些工具可以被后续 Skill 使用。

如果按早期 SDD 建议另建 `internal/tools`，会和现有 `agentkit` 抽象重复，后续维护两套 Tool / MCP 接口。需要先把现有 `agentkit` 扩展成一个可用的 mock MCP tool registry，为后续 `ProjectPolishSkill` 和外部工具接入留出稳定边界。

## What Changes

- 在 `internal/agentkit` 内补齐 mock MCP client 和默认 MCP 工具注册能力。
- 注册两个只读 mock 工具：
  - `github.project_analyze`
  - `web.fetch`
- 为 `ToolRegistry` 增加必要的只读查询能力，便于测试和后续展示工具清单。
- 保持工具调用必须经过 `ToolRegistry` 的权限、超时和 hook 边界。
- 更新 SDD，明确阶段 4 复用 `internal/agentkit`，不新建重复 `internal/tools`。

## Non-Goals

- 不接真实 GitHub API。
- 不接真实网页抓取。
- 不实现完整 MCP Server / Client 协议生命周期。
- 不实现 Gateway、daemon、Sandbox 或 runtime sub-agent。
- 不改变 `/api/agent/message` 响应结构。
- 不让用户输入直接决定工具权限。

## Impact

- 影响代码：`internal/agentkit` 及其测试。
- 影响文档：`docs/SDD-Backend.md`、`docs/code-changes`。
- 不改变 HTTP API、数据库 schema、Session JSON 顶层字段或前端类型。
- 无新增第三方依赖。
