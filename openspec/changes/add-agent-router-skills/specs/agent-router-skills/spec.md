## ADDED Requirements

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
