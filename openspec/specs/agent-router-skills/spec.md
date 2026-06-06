# agent-router-skills Specification

## Purpose

定义 Agent 消息入口、规则路由、Skill Registry、兼容响应结构和工具 trace 展示边界。

## Requirements

### Requirement: Agent 消息入口应返回结构化路由结果

系统 MUST 提供统一 Agent 消息入口，用于把用户请求路由到结构化 intent 和 skill。

#### Scenario: skill 请求被路由

- **WHEN** 用户提交包含知识讲解、测验或项目润色意图的消息
- **THEN** 系统应返回 `intent`、`skill`、`confidence` 和 `reason`
- **AND** 系统应返回 skill 执行结果

#### Scenario: 空消息被拒绝

- **WHEN** 用户提交空消息
- **THEN** 系统应返回请求错误

### Requirement: Skill Registry 应支持可复用能力组件

系统 MUST 支持通过 skill name 注册和执行可复用 skill。

#### Scenario: 已注册 skill 可执行

- **WHEN** Agent Router 选择已注册 skill
- **THEN** AgentService 应执行该 skill 并返回结果

#### Scenario: 未注册 skill 返回错误

- **WHEN** Agent Router 选择未注册 skill
- **THEN** AgentService 应返回结构化错误

### Requirement: 第一版 Router 不应依赖 LLM

系统 MUST 使用规则 Router 作为第一版 intent router。

#### Scenario: Router 规则可测试

- **WHEN** 测试输入固定消息
- **THEN** Router 输出应稳定可复现

### Requirement: 现有面试 API 保持兼容

新增 Agent 消息入口 MUST NOT 改变现有 `/api/interview/start` 和 `/api/interview/answer` 行为。

#### Scenario: Agent 消息不自动创建面试会话

- **WHEN** 用户消息被识别为 `interview.start`
- **THEN** 第一版 AgentService 应返回引导结果
- **AND** 不应绕过现有 interview service 自动创建 session

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
