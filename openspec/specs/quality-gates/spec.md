# quality-gates Specification

## Purpose

定义本地质量门禁、Agent 输出验证、工具事件 fixture、长期记忆观测 fixture 和验证命令文档一致性要求。

## Requirements

### Requirement: 本地质量门禁应包含 Agent 输出验证

系统 MUST 提供本地质量门禁命令来运行 `cmd/agent-verify` 的通过用例，并将其纳入统一本地验证流程。

#### Scenario: 运行 Agent 验证门禁

- **WHEN** 开发者运行 `verify-agent`
- **THEN** 系统 MUST 执行 `go run ./cmd/agent-verify -session testdata/agent_verify/pass_session.json`
- **AND** 验证失败时命令 MUST 返回非 0

#### Scenario: Agent 验证门禁检查 Graph 状态收口

- **WHEN** 开发者运行 `cmd/agent-verify`
- **THEN** 系统 MUST 检查核心 Interview Graph 节点的 PatchNode 注册、写集和关键顺序
- **AND** 系统 MUST 检查累计型节点的重复执行风险

#### Scenario: verify-local 包含 Agent 验证

- **WHEN** 开发者运行 `verify-local`
- **THEN** 系统 MUST 执行 Agent 验证门禁

### Requirement: Agent Message mock tool 链路应有 HTTP fixture 验证

系统 MUST 使用 fixture 验证 `/api/agent/message` 的 `project_polish` mock GitHub 工具链路。

#### Scenario: ProjectPolish HTTP fixture 使用 mock tool

- **WHEN** fixture 请求包含 GitHub 仓库 URL
- **THEN** `/api/agent/message` MUST 返回 `skill.project_polish`
- **AND** 响应内容 MUST 包含 mock GitHub 项目分析 marker

### Requirement: 验证命令文档应与真实 Makefile 和 CLI 保持一致

系统 MUST 在 README、SDD 和 AI commands 中使用当前真实存在的 RAG eval 路径、参数和 Agent verify 命令。

#### Scenario: 文档使用当前 RAG eval 命令

- **WHEN** 文档描述 RAG eval
- **THEN** 命令 MUST 使用 `testdata/rag/golden_queries.jsonl`
- **AND** 命令 MUST 使用当前 `cmd/rag-eval` 支持的参数名

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

### Requirement: 本地质量门禁应包含内部试用 smoke

系统 MUST 提供可重复执行的内部试用 smoke 验证，用于判断当前构建是否达到内部团队真实试用门槛。

#### Scenario: 内部试用 smoke 覆盖完整业务链路

- **WHEN** 开发者运行内部试用 smoke 命令
- **THEN** smoke MUST 覆盖面试开始、答题推进、报告生成、长期记忆观测、Agent 项目润色和 `tool_trace`
- **AND** 任一关键步骤失败时命令 MUST 返回非 0

#### Scenario: 内部试用 smoke 区分 mock 和真实工具

- **WHEN** smoke 在默认配置下运行
- **THEN** 验证 MUST 能确认工具路径是 deterministic mock 或未启用真实工具
- **AND** smoke MUST NOT 要求默认环境联网

#### Scenario: 内部试用 smoke 校验真实工具配置缺失

- **WHEN** smoke 或 fixture 覆盖真实工具配置缺失路径
- **THEN** 验证 MUST 检查稳定错误类别或 trace 状态
- **AND** 验证 MUST 确认该状态没有被伪装成成功真实调用

### Requirement: 内部试用前必须运行既有验证门禁

系统 MUST 在内部试用说明中列出并维护当前必跑验证命令。

#### Scenario: 内部试用发布检查

- **WHEN** 维护者准备标记某版本可供内部试用
- **THEN** 检查清单 MUST 包含 Go 测试、前端测试、前端构建、agent-verify tool/memory fixtures、内部试用 smoke 和 OpenSpec strict validation
