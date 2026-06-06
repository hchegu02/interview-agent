## ADDED Requirements

### Requirement: ProjectPolishSkill 应可使用 mock GitHub 项目分析工具

系统 MUST 在 `project_polish` skill 中支持通过 mock `github.project_analyze` 工具增强项目亮点提炼。

#### Scenario: 输入包含 GitHub URL 时调用 mock 工具

- **WHEN** 用户消息或 context 中包含 GitHub 仓库 URL
- **AND** ToolRegistry 已注册 `github.project_analyze`
- **THEN** `project_polish` MUST 调用该工具
- **AND** 返回内容 SHOULD 融合工具返回的项目摘要或亮点

#### Scenario: 无 GitHub URL 时保持旧行为

- **WHEN** 用户消息和 context 中都没有 GitHub 仓库 URL
- **THEN** `project_polish` MUST 返回通用项目亮点提炼建议
- **AND** 系统 MUST NOT 调用工具

#### Scenario: 工具失败时降级

- **WHEN** `github.project_analyze` 调用失败
- **THEN** `project_polish` MUST 返回通用项目亮点提炼建议
- **AND** 用户请求 MUST NOT 因工具失败而返回错误

### Requirement: 默认 Agent 服务应注入 mock tool registry

系统 MUST 在默认服务装配中注入 mock MCP tool registry，使 `/api/agent/message` 的 project polish 路径具备本地工具链路。

#### Scenario: 默认服务支持 project polish 工具增强

- **WHEN** 默认 Agent 服务处理包含 GitHub URL 的项目润色请求
- **THEN** 系统 MUST 使用 mock `github.project_analyze` 增强输出

### Requirement: 阶段 4.2 不应声明真实外部 GitHub 分析

系统 MUST 明确当前项目分析仍是 mock tool foundation，不得描述为已接入真实 GitHub API 或真实仓库抓取。

#### Scenario: 文档描述 mock 边界

- **WHEN** 后端 SDD 描述 ProjectPolishSkill 工具增强
- **THEN** 文档 MUST 声明当前只使用 mock `github.project_analyze`
- **AND** 真实 GitHub 分析属于后续阶段
