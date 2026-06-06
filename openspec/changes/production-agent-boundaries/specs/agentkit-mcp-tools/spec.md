## ADDED Requirements

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
