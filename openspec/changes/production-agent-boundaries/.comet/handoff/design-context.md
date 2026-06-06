# Comet Design Handoff

- Change: production-agent-boundaries
- Phase: design
- Mode: compact
- Context hash: 83e9a3a0dc3230885609e98bf550e727af84c195728c0fd188c630f95e3af9ac

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/production-agent-boundaries/proposal.md

- Source: openspec/changes/production-agent-boundaries/proposal.md
- Lines: 1-34
- SHA256: 182fcebc8cd540d698be155aec7ff62723913af0d2c6c5b310885bc929fb2203

```md
## Why

Interview Agent 已经具备长期记忆、Agent Router、mock MCP Tool、SSE trace 和本地验证门禁，但这些能力仍停留在开发边界：画像读取依赖开发模式 owner header，长期记忆写入失败不可见，工具调用只有 mock foundation，前端也缺少结构化 tool trace 展示。下一阶段需要把这些边界收紧到可替换、可观测、可验证的生产接入形态。

本 change 的目标不是一次性实现完整用户中心、完整 MCP runtime 或 runtime sub-agent，而是把真实身份来源、低风险真实工具接入、工具 trace、长期记忆可观测性和验证门禁设计成稳定增量，避免继续在 mock 和自然语言结果上堆功能。

## What Changes

- 定义真实身份和访问边界：保留现有 `UserMemoryOwnerResolver` / `UserMemoryAuthorizer` 可注入点，设计从开发 header 迁移到真实身份来源的后端边界。
- 增强长期记忆写入可观测性：长期记忆写入失败、跳过、CAS 冲突重试和最终结果必须产生结构化可观测信号，但仍不阻断面试完成响应。
- 推进真实 MCP / Tool MVP：在现有 `internal/agentkit` ToolRegistry 边界内接入一个低风险真实工具，保留 schema、权限、超时、hook、错误码和 mock fixture。
- 增加结构化 Agent / Tool trace：后端返回工具调用 trace，前端只读展示工具状态，不从自然语言结果里反推工具执行。
- 扩展验证门禁：`cmd/agent-verify` 和本地验证 fixture 覆盖真实 tool event、长期记忆观测信号和结构化 trace。
- 不引入 **BREAKING** API 变更；新增 JSON 字段必须使用兼容的 `omitempty` / 可选字段。

## Capabilities

### New Capabilities

- `identity-access-boundary`: 定义真实身份来源、owner 解析和授权边界，覆盖从开发 header 到生产身份注入的替换路径。

### Modified Capabilities

- `long-term-memory`: 增加长期记忆写入可观测性要求，包括跳过、失败、CAS 冲突重试和成功写入的结构化信号。
- `agentkit-mcp-tools`: 将当前 mock MCP Tool foundation 扩展为可接入低风险真实工具的 MVP，同时继续复用 ToolRegistry 权限、超时和 hook 边界。
- `agent-router-skills`: 为 Agent 消息入口增加结构化 tool trace 响应要求，并保持现有 intent / skill / result 响应兼容。
- `quality-gates`: 增加真实 tool event fixture、tool trace 和长期记忆可观测性相关验证门禁。

## Impact

- 后端影响范围：`internal/httpapi` 身份解析和长期记忆写入观测、`internal/memory` 写入结果表达、`internal/agentkit` ToolRegistry / MCP adapter、`internal/agent` 和 `internal/skills` 的工具调用返回结构。
- 前端影响范围：`web/src/apiClient.ts`、`web/src/types.ts`、`web/src/candidatePages.tsx` 只读展示结构化 tool trace；不得直接访问 MCP 服务或自行判断工具状态。
- 验证影响范围：`cmd/agent-verify`、`testdata/agent_verify`、现有 Go 测试、前端测试和构建。
- 文档影响范围：`docs/SDD-Backend.md`、`docs/SDD-Frontend.md` 需要同步真实身份、真实工具 MVP、trace 和验证边界；不得把 Codex sub-agent 或完整 MCP Gateway 写成项目运行时能力。
```

## openspec/changes/production-agent-boundaries/design.md

- Source: openspec/changes/production-agent-boundaries/design.md
- Lines: 1-95
- SHA256: 0c723e0ed2982b274ceb34ac7155f69c23a30c581d2cf84d0a4259271115c2d0

[TRUNCATED]

```md
## Context

当前仓库已经有几条相关基础能力：

- `internal/httpapi/user_memory.go` 提供 `UserMemoryOwnerResolver` / `UserMemoryAuthorizer` 注入点，默认开发模式从 `X-User-ID` 或 `owner_user_id` 解析 owner。
- `internal/httpapi/interview_memory.go` 在面试完成后沉淀长期记忆，CAS 冲突会重试，但写入失败不阻断面试完成响应，当前可观测性不足。
- `internal/agentkit` 已有 `ToolRegistry`、权限、hook、mock MCP client 和默认 mock tool 注册。
- `/api/agent/message` 已返回结构化 intent / skill / result，但没有稳定 `tool_trace` 响应字段。
- `cmd/agent-verify` 已覆盖 RAG / Agent 输出和 mock tool event 的一部分门禁。

这次设计把上述能力推到生产接入边界：身份来源可替换、长期记忆写入可观测、真实工具 MVP 不绕过 ToolRegistry、trace 结构化返回、验证门禁能抓住回归。

## Goals / Non-Goals

**Goals:**

- 定义真实身份来源接入边界，避免业务 handler 直接耦合 JWT、cookie 或第三方认证细节。
- 给长期记忆写入增加结构化观测信号，覆盖成功、跳过、失败和 CAS 冲突重试耗尽。
- 在现有 ToolRegistry 内接入一个低风险真实工具 MVP，同时保留 deterministic mock。
- 为 `/api/agent/message` 增加兼容的 `tool_trace,omitempty` 响应字段，并让前端只读展示。
- 扩展 `cmd/agent-verify` / fixture / 前端测试，覆盖真实工具事件、tool trace 和长期记忆观测信号。

**Non-Goals:**

- 不实现完整 JWT 登录、用户中心、RBAC 或多租户权限模型。
- 不实现完整 MCP Gateway、daemon、Sandbox、外部工具市场或 runtime sub-agent。
- 不改变现有面试 Session JSON 的兼容性，不让前端复制后端状态机。
- 不让长期记忆写入失败阻断面试完成响应。

## Decisions

### 1. 身份边界放在 HTTP 层 resolver / authorizer，不下沉到业务模型

推荐方案：保留并推广 `UserMemoryOwnerResolver` / `UserMemoryAuthorizer` 模式，把真实身份来源接在 `cmd/server` 或 HTTP middleware 装配层。业务 handler 只接收“当前用户是谁”和“是否允许访问目标资源”的结果。

备选方案 A：在每个 handler 里直接解析 JWT。问题是认证细节会扩散，后续换身份提供方会改一堆业务代码。

备选方案 B：把 owner 直接写入 `domain.Session` 并让领域层判断权限。问题是当前要保护的是 HTTP 用户资源入口，强行把认证概念塞进领域模型会污染 Session 事实源。

### 2. 长期记忆观测使用小结构事件/日志，不保存正文

推荐方案：在长期记忆沉淀函数周围形成明确结果状态：`success`、`skipped`、`failed`、`conflict_retry_exhausted`。观测字段只包含 `user_id`、`session_id`、状态、错误类别、重试次数和耗时，不记录完整回答、完整报告或画像正文。

备选方案 A：直接把错误 `slog` 出去。问题是不可测试，也容易丢关键状态。

备选方案 B：把长期记忆写入失败变成 HTTP 错误。问题是破坏现有“面试完成优先”的用户流程，和现有 spec 冲突。

### 3. 真实工具 MVP 只能走 ToolRegistry

推荐方案：真实工具实现包装成现有 `Tool` / `MCPClient` 边界，所有调用仍通过 `ToolRegistry.Call`。这能复用 schema、权限、超时、before/after hook 和 mock fixture。第一版真实工具选低风险只读能力，例如 GitHub README / 仓库元数据分析。

备选方案 A：Skill 直接调 GitHub API。问题是绕过权限、超时和审计，等于废掉 AgentKit 边界。

备选方案 B：直接上完整 MCP Gateway。问题是范围过大，会引入生命周期、凭据、沙箱和网络安全问题，不符合 MVP。

### 4. Tool trace 是后端事实，前端只读展示

推荐方案：`/api/agent/message` 增加可选 `tool_trace` 字段。trace 项从 ToolRegistry hook 或 Skill 执行结果中收集，包含工具名、权限、状态、错误类别和耗时。前端只展示该字段；字段缺失时保持现有展示。

备选方案 A：前端从 `result.content` 文案推断工具状态。问题是脆弱且不可验证。

备选方案 B：把完整 raw tool input/output 全量返回前端。问题是暴露隐私、token 和外部响应细节，风险不值。

### 5. 验证门禁先覆盖边界，不追求端到端真实网络

推荐方案：新增 fixture 覆盖真实工具事件形状、tool trace 响应、长期记忆观测状态；真实网络调用用接口和 mock/fake 验证边界，避免 CI 依赖外部服务。

备选方案 A：CI 直接调用 GitHub 或真实 MCP 服务。问题是慢、不稳定，还会引入凭据管理。

备选方案 B：只跑 Go 单测不扩展 `agent-verify`。问题是无法覆盖跨模块输出契约，后续很容易破坏前端和验证链路。

## Risks / Trade-offs

- [Risk] 单个 change 覆盖身份、记忆、工具、trace、验证，范围偏大。 → Mitigation: 实现时按任务顺序小步提交，先身份和观测，再工具 MVP，再 trace 展示，最后门禁收口；不在同一任务里混改无关模块。
- [Risk] `tool_trace` 字段可能泄露工具输入或外部响应。 → Mitigation: trace 只暴露摘要字段和错误类别，不返回 raw payload、token、header 或完整正文。
- [Risk] 真实工具引入网络不稳定。 → Mitigation: 真实 client 必须有超时和错误分类；测试默认使用 fake/mock，不依赖外网。
- [Risk] 身份边界被误解为完整生产鉴权。 → Mitigation: SDD 明确本阶段只提供后端接入边界，不声明完整登录、RBAC 或用户中心。
- [Risk] 长期记忆观测和 HTTP 响应耦合过深。 → Mitigation: 观测信号由沉淀流程内部产生，不改变面试完成响应语义。

## Migration Plan
```

Full source: openspec/changes/production-agent-boundaries/design.md

## openspec/changes/production-agent-boundaries/tasks.md

- Source: openspec/changes/production-agent-boundaries/tasks.md
- Lines: 1-40
- SHA256: 18685567bc81bcb56d33be9cb7cbe746579f1eaf9c22c1a6ff8a6a92f39be842

```md
## 1. 身份与访问边界

- [ ] 1.1 梳理 `internal/httpapi/user_memory.go` 当前 owner resolver / authorizer 调用链，确认生产身份注入点。
- [ ] 1.2 增加或调整身份 resolver 装配，使业务 handler 只依赖当前用户结果，不直接解析认证细节。
- [ ] 1.3 补充用户资源访问测试，覆盖本人访问、跨用户拒绝和缺身份拒绝。

## 2. 长期记忆写入可观测性

- [ ] 2.1 梳理 `persistLongTermMemory` 成功、跳过、失败和 CAS 冲突重试路径。
- [ ] 2.2 增加结构化观测结果或 recorder，记录 `user_id`、`session_id`、状态、错误类别、重试次数和耗时。
- [ ] 2.3 补充测试，覆盖成功、跳过、非冲突失败和 CAS 冲突重试耗尽。
- [ ] 2.4 确认观测信号不包含完整回答正文、完整报告正文、token 或私有配置。

## 3. 真实 MCP / Tool MVP

- [ ] 3.1 梳理 `internal/agentkit` ToolRegistry、MCP mock client、hook 和权限测试。
- [ ] 3.2 选择第一版低风险真实工具，并明确配置、超时、错误分类和 mock 回退策略。
- [ ] 3.3 在 ToolRegistry 边界内实现真实工具适配，保留 deterministic mock 用例。
- [ ] 3.4 补充工具调用测试，覆盖成功、权限拒绝、超时或配置缺失错误、before/after hook 成对。

## 4. Agent / Tool Trace 展示

- [ ] 4.1 设计后端 `tool_trace` DTO，字段只包含工具名、权限、状态、错误类别、耗时和必要摘要。
- [ ] 4.2 让 Agent Skill / AgentService 收集工具 trace，并通过 `/api/agent/message` 以 `omitempty` 字段返回。
- [ ] 4.3 前端补充 `tool_trace` TypeScript 类型和只读展示，缺字段时保持现有 Agent 页面行为。
- [ ] 4.4 补充 HTTP 和前端测试，覆盖包含 trace、不包含 trace 和工具失败 trace。

## 5. 验证门禁增强

- [ ] 5.1 扩展 `cmd/agent-verify` 或 fixture，验证真实工具事件 before/after 成对、权限、状态和错误类别。
- [ ] 5.2 增加长期记忆观测相关测试或 fixture，确认失败不阻断面试完成。
- [ ] 5.3 更新本地验证说明，明确需要运行 Go 测试、前端测试/build、agent-verify tool events。

## 6. 文档和收口

- [ ] 6.1 更新 `docs/SDD-Backend.md`，同步真实身份边界、长期记忆观测、真实工具 MVP、tool trace 和验证门禁。
- [ ] 6.2 更新 `docs/SDD-Frontend.md`，同步前端只读展示 `tool_trace` 且不直接调用 MCP 服务。
- [ ] 6.3 若本 change 修改代码，按项目规则新增或更新 `docs/code-changes/MM-DD-简短变更名.md`。
- [ ] 6.4 运行最小必要验证：`go test ./... -count=1`、`npm --prefix web run test`、`npm --prefix web run build`、`go run ./cmd/agent-verify -session testdata/agent_verify/pass_session.json -tool-events testdata/agent_verify/pass_tool_events.json`。
- [ ] 6.5 运行 `openspec validate production-agent-boundaries --strict` 并确认通过。
```

## openspec/changes/production-agent-boundaries/specs/agentkit-mcp-tools/spec.md

- Source: openspec/changes/production-agent-boundaries/specs/agentkit-mcp-tools/spec.md
- Lines: 1-39
- SHA256: 4fe00b26d5ad61fb47caf598a29f748c00c3e5d0fd0f1b19afce9603ad6d0ce2

```md
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
```

## openspec/changes/production-agent-boundaries/specs/agent-router-skills/spec.md

- Source: openspec/changes/production-agent-boundaries/specs/agent-router-skills/spec.md
- Lines: 1-38
- SHA256: 02e38439c7de582d57af7807be4a6073119357ff43001d6cd655ffd72870ef77

```md
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
```

## openspec/changes/production-agent-boundaries/specs/identity-access-boundary/spec.md

- Source: openspec/changes/production-agent-boundaries/specs/identity-access-boundary/spec.md
- Lines: 1-38
- SHA256: 9f8ad7368b1985c1341506406ad2caf7f71f992f16b14e370d4a8e6c9be03ef4

```md
## ADDED Requirements

### Requirement: 后端应通过可替换身份来源解析当前用户

系统 MUST 提供后端身份解析边界，用于从请求上下文解析当前用户身份，并允许开发模式 owner header 被生产身份来源替换。

#### Scenario: 生产身份来源解析当前用户

- **WHEN** HTTP 请求已经由上游认证层注入当前用户身份
- **THEN** 后端 MUST 通过统一 resolver 读取当前用户标识
- **AND** 业务 handler MUST NOT 直接解析 JWT、cookie 或第三方认证细节

#### Scenario: 开发模式 header 只作为回退来源

- **WHEN** 系统运行在未接入真实认证的开发模式
- **THEN** resolver MAY 从 `X-User-ID` 或 `owner_user_id` 解析用户标识
- **AND** 文档 MUST 明确该 header 不是生产安全边界

### Requirement: 用户资源访问必须经过授权边界

系统 MUST 在读取或写入用户归属资源前检查当前用户是否有权访问目标用户资源。

#### Scenario: 当前用户读取本人画像

- **WHEN** 当前用户标识与目标 `user_id` 一致
- **THEN** 授权边界 MUST 允许读取该用户长期画像

#### Scenario: 当前用户读取他人画像

- **WHEN** 当前用户标识与目标 `user_id` 不一致
- **THEN** 授权边界 MUST 拒绝访问
- **AND** HTTP API MUST 返回稳定未授权或禁止访问响应

#### Scenario: 缺少当前用户身份

- **WHEN** 请求无法解析当前用户身份
- **THEN** 受保护用户资源 API MUST 拒绝访问
- **AND** 响应 MUST 保持结构化错误格式
```

## openspec/changes/production-agent-boundaries/specs/long-term-memory/spec.md

- Source: openspec/changes/production-agent-boundaries/specs/long-term-memory/spec.md
- Lines: 1-38
- SHA256: fc313e16fdff2a9d92d2149ec0d9c37f60fbabf2fecb68efc7db0307abfcb152

```md
## ADDED Requirements

### Requirement: 长期记忆写入应产生结构化可观测信号

系统 MUST 在长期记忆沉淀过程中记录结构化可观测信号，覆盖成功、跳过、失败和冲突重试结果，同时保持面试完成响应不被长期记忆写入失败阻断。

#### Scenario: 长期记忆写入成功

- **WHEN** 面试完成后长期记忆合并并保存成功
- **THEN** 系统 MUST 记录包含 `user_id`、`session_id` 和结果状态的结构化信号

#### Scenario: 长期记忆写入被跳过

- **WHEN** 面试完成但缺少 `user_id`、Report 或长期记忆 Store
- **THEN** 系统 MUST 记录跳过原因
- **AND** 面试完成响应 MUST 保持不变

#### Scenario: 长期记忆写入失败

- **WHEN** 长期记忆 Store 返回非冲突错误
- **THEN** 系统 MUST 记录错误类别和目标用户
- **AND** 面试完成响应 MUST 保持不变

#### Scenario: 长期记忆 CAS 冲突重试耗尽

- **WHEN** 长期记忆写入因 CAS 冲突重试后仍失败
- **THEN** 系统 MUST 记录冲突次数和最终失败状态
- **AND** 面试完成响应 MUST 保持不变

### Requirement: 长期记忆观测信号不得泄露敏感正文

系统 MUST 限制长期记忆观测信号内容，避免记录完整回答正文、完整报告正文、token、密钥或私有配置。

#### Scenario: 写入失败日志不包含完整画像正文

- **WHEN** 长期记忆写入失败并产生日志或事件
- **THEN** 观测信号 MUST NOT 包含完整回答正文或完整报告正文
- **AND** 观测信号 SHOULD 只包含用户、会话、状态、错误类别和计数字段
```

## openspec/changes/production-agent-boundaries/specs/quality-gates/spec.md

- Source: openspec/changes/production-agent-boundaries/specs/quality-gates/spec.md
- Lines: 1-40
- SHA256: 9de7ecf3b19b0f4457c101c5ddd656299ca4f87e23ae5dc1313f4e10b4869334

```md
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
```

