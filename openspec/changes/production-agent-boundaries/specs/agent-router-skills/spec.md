## ADDED Requirements

### Requirement: Agent 消息入口应返回结构化工具 trace

系统 MUST 在 Agent 消息响应中提供可选结构化工具 trace，用于展示工具调用状态，并保持现有 `intent`、`skill`、`confidence`、`reason` 和 `result` 字段兼容。

#### Scenario: Skill 调用工具成功

- **WHEN** Agent Skill 通过 ToolRegistry 成功调用工具
- **THEN** `/api/agent/message` 响应 SHOULD 包含 `tool_trace`
- **AND** trace 项 MUST 包含工具名、权限、状态和耗时信息

#### Scenario: Skill 调用工具失败

- **WHEN** Agent Skill 调用工具失败
- **THEN** `/api/agent/message` 响应 SHOULD 包含失败 trace 项
- **AND** trace 项 MUST 包含稳定错误类别

#### Scenario: Skill 未调用工具

- **WHEN** Agent Skill 没有调用任何工具
- **THEN** `/api/agent/message` MAY 省略 `tool_trace`
- **AND** 现有客户端 MUST 能继续解析响应

### Requirement: 前端只能只读展示后端工具 trace

前端 MUST 只根据后端返回的结构化 `tool_trace` 展示工具状态，不得从自然语言结果中反推工具调用状态。

#### Scenario: 前端展示工具 trace

- **WHEN** Agent 消息响应包含 `tool_trace`
- **THEN** 前端 MUST 展示工具名、状态和错误类别等只读信息

#### Scenario: 响应缺少工具 trace

- **WHEN** Agent 消息响应不包含 `tool_trace`
- **THEN** 前端 MUST 保持现有 Agent 结果展示
- **AND** 前端 MUST NOT 通过搜索 result 文案推断工具是否执行
