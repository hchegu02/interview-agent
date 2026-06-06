# agentkit-mcp-tools Specification

## Purpose

定义 AgentKit 工具注册、mock MCP 工具、显式真实只读工具接入、安全审计边界和验证要求。

## Requirements

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

### Requirement: AgentKit 应支持低风险真实工具 MVP

系统 MUST 在现有 `ToolRegistry` 边界内支持接入一个低风险真实工具，并保留 mock 实现用于本地测试和离线验证。

#### Scenario: 真实工具通过 ToolRegistry 调用

- **WHEN** Agent Skill 需要调用真实工具
- **THEN** 系统 MUST 通过 `ToolRegistry.Call` 执行调用
- **AND** 调用 MUST 经过权限、schema、超时和 hook 边界

#### Scenario: mock 工具仍可用于测试

- **WHEN** 测试或本地演示不配置真实工具凭据
- **THEN** 系统 MUST 能继续使用 deterministic mock 工具
- **AND** 现有 mock fixture MUST 保持可验证

#### Scenario: 真实工具不可用

- **WHEN** 真实工具配置缺失、超时或返回错误
- **THEN** 系统 MUST 返回结构化工具错误
- **AND** Skill MUST NOT 把工具失败伪装成成功结果

### Requirement: 真实工具 MVP 不得绕过安全和审计边界

系统 MUST 明确真实工具 MVP 不包含完整 MCP Gateway、Sandbox、daemon 或 runtime sub-agent，并且不得绕过现有 ToolRegistry 审计点。

#### Scenario: 工具调用产生 before/after hook

- **WHEN** 真实工具通过 ToolRegistry 被调用
- **THEN** 系统 MUST 产生成对的 before/after hook 事件
- **AND** after 事件 MUST 表达成功或错误状态

#### Scenario: 文档描述真实工具能力边界

- **WHEN** SDD 描述真实工具 MVP
- **THEN** 文档 MUST 声明当前只接入低风险工具
- **AND** 文档 MUST 声明未实现完整 MCP Gateway、Sandbox、daemon 或 runtime sub-agent

### Requirement: 内部试用真实工具必须显式启用

系统 MUST 保持默认 deterministic mock 工具行为，并只在内部试用配置显式启用时装配真实只读工具 client。

#### Scenario: 默认装配不访问真实 GitHub

- **WHEN** 服务未配置内部试用真实 GitHub 工具
- **THEN** 默认 Agent 工具注册 MUST 使用 deterministic mock
- **AND** 系统 MUST NOT 默认联网访问 GitHub

#### Scenario: 内部试用显式启用真实 GitHub 工具

- **WHEN** 内部试用环境显式配置真实 GitHub 工具 client 所需参数
- **THEN** `github.project_analyze` MUST 仍通过 `ToolRegistry.Call` 执行
- **AND** 调用 MUST 经过权限、schema、超时和 before/after hook

#### Scenario: 真实 GitHub 工具配置缺失

- **WHEN** 内部试用环境请求真实 GitHub 工具但配置不完整
- **THEN** 工具调用 MUST 返回稳定 `config_missing` 或等价错误类别
- **AND** 系统 MUST NOT 回退为伪装成功的真实调用
