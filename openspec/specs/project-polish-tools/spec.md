# project-polish-tools Specification

## Purpose

定义 `project_polish` skill 使用 GitHub 项目分析工具增强项目亮点提炼的行为边界，默认保持 deterministic mock，本地可验证；内部试用环境只允许显式配置的真实只读工具路径。

## Requirements

### Requirement: ProjectPolishSkill 应可使用 GitHub 项目分析工具

系统 MUST 在 `project_polish` skill 中支持通过 `github.project_analyze` 工具增强项目亮点提炼；该工具在默认环境 MAY 是 deterministic mock，在内部试用环境 MAY 是显式配置的真实只读 GitHub client。

#### Scenario: 输入包含 GitHub URL 时调用可用工具

- **WHEN** 用户消息或 context 中包含 GitHub 仓库 URL
- **AND** ToolRegistry 已注册 `github.project_analyze`
- **THEN** `project_polish` MUST 通过 `ToolRegistry.Call` 调用该工具
- **AND** 返回内容 SHOULD 融合工具返回的安全摘要或亮点

#### Scenario: 默认环境使用 mock 工具

- **WHEN** 默认 Agent 服务处理包含 GitHub URL 的项目润色请求
- **THEN** 系统 MUST 使用 deterministic mock `github.project_analyze` 增强输出
- **AND** 该路径 MUST 可被本地 fixture 稳定验证

#### Scenario: 内部试用环境使用真实只读工具

- **WHEN** 内部试用环境显式配置真实 GitHub 工具 client
- **THEN** `project_polish` MAY 使用真实公开仓库元数据生成项目建议
- **AND** 响应 MUST 通过顶层 `tool_trace` 暴露工具名、状态、错误类别和摘要

#### Scenario: 工具失败时降级但保留失败 trace

- **WHEN** `github.project_analyze` 调用失败、超时或配置缺失
- **THEN** `project_polish` MAY 返回通用项目亮点提炼建议
- **AND** 顶层 `tool_trace` MUST 保留失败状态或错误类别
- **AND** 系统 MUST NOT 把失败工具调用描述成真实仓库分析成功

### Requirement: 默认 Agent 服务应注入 mock tool registry

系统 MUST 在默认服务装配中注入 mock MCP tool registry，使 `/api/agent/message` 的 project polish 路径具备本地工具链路。

#### Scenario: 默认服务支持 project polish 工具增强

- **WHEN** 默认 Agent 服务处理包含 GitHub URL 的项目润色请求
- **THEN** 系统 MUST 使用 mock `github.project_analyze` 增强输出
