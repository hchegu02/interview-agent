## ADDED Requirements

### Requirement: Agent 验证门禁应覆盖真实工具事件

系统 MUST 提供 fixture 和验证逻辑，用于校验真实工具 MVP 的 before/after hook、权限、错误状态和 trace 输出。

#### Scenario: 真实工具事件 fixture 通过验证

- **WHEN** 开发者运行 Agent 验证门禁并提供真实工具事件 fixture
- **THEN** `cmd/agent-verify` MUST 校验工具 before/after hook 成对
- **AND** 验证 MUST 检查工具权限、状态和错误类别

#### Scenario: 工具 after 事件缺失

- **WHEN** tool event fixture 包含 before 事件但缺少对应 after 事件
- **THEN** Agent 验证门禁 MUST 失败

### Requirement: 验证门禁应覆盖长期记忆可观测性

系统 MUST 使用测试或 fixture 校验长期记忆写入成功、跳过、失败和 CAS 冲突重试耗尽时的观测信号。

#### Scenario: 长期记忆失败观测通过验证

- **WHEN** 长期记忆写入失败
- **THEN** 测试 MUST 断言系统记录失败状态和错误类别
- **AND** 测试 MUST 断言面试完成响应不被失败阻断

### Requirement: 本地验证命令应覆盖结构化 tool trace

系统 MUST 在本地测试或构建门禁中覆盖 `/api/agent/message` 的结构化 `tool_trace` 响应和前端只读展示。

#### Scenario: Agent 消息 fixture 包含 tool trace

- **WHEN** fixture 请求触发工具调用
- **THEN** HTTP 响应验证 MUST 检查 `tool_trace` 的工具名和状态

#### Scenario: 前端构建和测试覆盖 trace 展示

- **WHEN** 前端类型或展示逻辑增加 `tool_trace`
- **THEN** 前端测试 MUST 覆盖包含 trace 和不包含 trace 两种响应
