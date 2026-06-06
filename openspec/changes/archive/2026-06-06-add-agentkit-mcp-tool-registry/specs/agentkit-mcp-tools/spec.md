## ADDED Requirements

### Requirement: AgentKit 应提供默认 MCP mock 工具注册

系统 MUST 能在 `internal/agentkit` 中注册默认 MCP mock 工具，并通过现有 `ToolRegistry` 调用这些工具。

#### Scenario: 注册默认 MCP mock 工具

- **WHEN** 系统调用默认 MCP 工具注册函数
- **THEN** `ToolRegistry` MUST 包含 `github.project_analyze`
- **AND** `ToolRegistry` MUST 包含 `web.fetch`

#### Scenario: 调用 GitHub 项目分析 mock 工具

- **WHEN** 调用方通过 `ToolRegistry` 调用 `github.project_analyze`
- **THEN** 系统 MUST 返回结构化项目分析结果
- **AND** 结果 MUST 包含项目摘要、主要语言、亮点和风险点

#### Scenario: 调用网页抓取 mock 工具

- **WHEN** 调用方通过 `ToolRegistry` 调用 `web.fetch`
- **THEN** 系统 MUST 返回结构化网页结果
- **AND** 结果 MUST 包含 URL、标题和正文摘要

### Requirement: MCP mock 工具必须复用 ToolRegistry 边界

系统 MUST 通过现有 `ToolRegistry` 的权限、超时和 hook 边界调用 MCP mock 工具。

#### Scenario: 权限不匹配时拒绝调用

- **WHEN** 调用方使用不匹配的权限调用 MCP mock 工具
- **THEN** `ToolRegistry` MUST 拒绝调用
- **AND** MCP client MUST NOT 被执行

#### Scenario: 工具清单按名称稳定返回

- **WHEN** 调用方查询已注册工具清单
- **THEN** 系统 MUST 按工具名称升序返回工具 spec

### Requirement: 阶段 4 不应声明真实外部 MCP runtime

系统 MUST 明确当前 MCP 工具为 mock foundation，不得把它描述为已接入真实 GitHub API、真实网页抓取、Gateway、Sandbox 或 runtime sub-agent。

#### Scenario: 文档描述当前能力边界

- **WHEN** 后端 SDD 描述 MCP Adapter 阶段
- **THEN** 文档 MUST 声明当前只提供 mock MCP tool registry foundation
- **AND** 文档 MUST 声明真实外部工具接入属于后续阶段
