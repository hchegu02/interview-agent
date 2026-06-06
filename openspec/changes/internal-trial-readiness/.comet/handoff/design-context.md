# Comet Design Handoff

- Change: internal-trial-readiness
- Phase: design
- Mode: compact
- Context hash: e1487718af0d3c57d6a3a3b8e50f0f48eb9b04fbc94f7bd01b799b0b34766d0b

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/internal-trial-readiness/proposal.md

- Source: openspec/changes/internal-trial-readiness/proposal.md
- Lines: 1-37
- SHA256: 10bcfc8635049eed317001ebcd96e8e5dcbcdb231b7ba0b7aee7ae0d767012ae

```md
## Why

Interview Agent 已具备面试主流程、长期记忆、Agent 工具 trace、真实 GitHub 只读 client 边界和验证门禁，但当前默认装配仍偏开发和演示。内部团队试用前，需要把身份来源、真实工具开关、长期记忆观测、smoke 验证和能力限制收成明确边界，避免把 mock/demo 行为误当成可对外生产能力。

本 change 的目标是支持内部 3-10 人可控试用，不是外部生产发布。

## What Changes

- 定义内部试用身份模式：后端必须通过统一 resolver/authorizer 读取可信内部身份来源；开发态 `X-User-ID` / `owner_user_id` 只作为 dev fallback。
- 定义真实 GitHub 工具试用开关：默认仍使用 deterministic mock；内部试用环境必须显式配置真实只读 GitHub client，缺配置时返回稳定状态并可在 trace 中诊断。
- 定义 Agent 项目润色的真实/模拟工具口径：`project_polish` 不得把工具失败伪装成真实成功，也不得向前端暴露 token、HTTP body 或底层工具输入。
- 定义长期记忆写入试用观测：内部 smoke 必须能看到成功、跳过、失败或冲突重试耗尽的稳定观测结果。
- 增加内部试用 smoke 门禁：覆盖面试开始、答题、报告、长期记忆沉淀、Agent 项目润色、tool trace 和 memory observation。
- 同步 SDD/试用文档限制：明确内部试用不是完整 JWT/OIDC、租户、完整 MCP gateway/server/client runtime 或运行时 sub-agent。
- 不引入 **BREAKING** API 变更；新增响应字段必须保持可选和向后兼容。

## Capabilities

### New Capabilities

- None

### Modified Capabilities

- `identity-access-boundary`: 增加内部试用身份来源和 dev fallback 的行为边界。
- `agentkit-mcp-tools`: 增加内部试用真实工具显式启用、默认 mock 和可诊断配置缺失要求。
- `project-polish-tools`: 更新项目润色工具能力口径，覆盖真实 GitHub 只读工具试用与失败降级。
- `long-term-memory`: 增加内部试用 smoke 对长期记忆观测信号的要求。
- `quality-gates`: 增加内部试用 smoke 验证门禁要求。
- `system-design-documentation`: 增加内部试用边界和非生产能力声明要求。

## Impact

- 后端影响范围：`cmd/server` 装配、`internal/httpapi` 身份解析和长期记忆入口、`internal/agentkit` 工具注册和真实 GitHub client 装配、`internal/agent` / `internal/skills` 工具 trace 传递、`cmd/agent-verify` 或新增 smoke CLI。
- 前端影响范围：只读展示已有 `tool_trace` 和限制状态；不得新增直接调用外部 GitHub/MCP 的前端逻辑。
- 验证影响范围：Go 测试、前端测试/构建、`cmd/agent-verify` fixtures、内部试用 smoke 命令。
- 文档影响范围：`docs/SDD-Backend.md`、`docs/SDD-Frontend.md` 和必要的内部试用说明。
```

## openspec/changes/internal-trial-readiness/design.md

- Source: openspec/changes/internal-trial-readiness/design.md
- Lines: 1-66
- SHA256: 78a3691691e59790c90b8375f0ed9954697ca0b759596a6e042f9e693dd8669d

```md
## Context

当前系统已经有一批生产边界基础：可替换 owner resolver/authorizer、长期记忆结构化观测、真实 GitHub 只读 client、Agent `tool_trace` 和 agent-verify fixture。缺口不在核心抽象，而在试用装配和验收口径：默认服务仍偏 mock，本地开发 header 容易被误读为鉴权，真实工具启用路径没有内部试用门禁，完整业务 smoke 尚未把面试、长期记忆和 Agent 工具串成一条可重复验证链路。

## Goals / Non-Goals

Goals:

- 支持内部团队可控试用完整业务链路。
- 保持默认本地/CI 离线可运行，不默认联网。
- 让真实工具、身份来源和长期记忆写入状态可诊断。
- 用 smoke 和现有验证命令定义“可试用”的最低门槛。

Non-goals:

- 不实现完整 JWT/OIDC、公开登录、租户、计费或生产级权限体系。
- 不实现完整 MCP gateway/server/client 生命周期。
- 不引入运行时 sub-agent。
- 不把长期记忆写入失败变成面试完成阻断条件。

## High-Level Approach

采用“内部试用配置包”而不是直接生产化：

1. 身份保持统一 resolver/authorizer 边界。内部试用环境只允许从可信内部来源解析用户，例如上游网关注入 header 或明确 allowlist；开发 fallback 保留但必须被配置和文档标记为 dev-only。
2. 工具注册保持显式装配。默认服务仍注册 deterministic mock；内部试用配置显式注入真实 `GitHubProjectClient`，缺 `HTTPClient`、`BaseURL` 或必要配置时返回稳定 `config_missing`，并通过 `tool_trace` 暴露状态摘要。
3. `project_polish` 继续通过 `ToolRegistry.Call` 调工具。Skill 只消费结构化 `ToolResult.Summary` 级别信息；失败时可以降级生成通用建议，但 trace 必须保留失败类别，不能让用户误以为真实仓库分析成功。
4. 长期记忆继续在面试完成后异步容错式沉淀。内部 smoke 必须能验证成功/跳过/失败/冲突耗尽观测信号，失败不阻断 completed session。
5. 试用门禁复用现有验证命令，再补一条业务 smoke，把关键 HTTP/CLI 路径串起来。

## Data Flow

```text
内部试用请求
  -> 可信身份来源 / dev fallback
  -> OwnerResolver
  -> Authorizer
  -> InterviewService start/answer
  -> Session completed + Report
  -> LongTermMemory observation
  -> Agent message project_polish
  -> ToolRegistry
  -> mock 或显式 GitHubProjectClient
  -> AgentResponse.tool_trace
  -> 前端只读展示
```

## Error Handling

- 身份缺失或不匹配：受保护用户资源返回结构化错误。
- 真实 GitHub 工具未配置：返回 `config_missing`，不默认联网。
- GitHub 调用失败或超时：记录 failed tool trace，Skill 不伪装成真实成功。
- 长期记忆写入失败：记录稳定错误类别，面试完成响应保持不变。
- smoke 失败：内部试用不得标记为通过。

## Testing Strategy

- 保留现有全量验证：`go test ./...`、`npm --prefix web run test`、`npm --prefix web run build`、`go run ./cmd/agent-verify ... -tool-events ... -memory-observations ...`、`openspec validate --all --strict`。
- 新增或扩展内部 smoke，覆盖完整试用链路和关键 trace 字段。
- 对配置缺失、mock 默认、真实工具显式启用、身份 fallback 限制分别增加最小测试或 fixture。

## Risks

- 最大风险是内部试用被误解为外部生产。缓解方式是配置名、SDD、trace 和 smoke 输出都明确 `internal trial` / `dev fallback` / `mock` / `config_missing`。
- 真实 GitHub API 可能引入网络不稳定。默认离线 mock，真实 client 只在显式配置下启用。
- 旧 `project-polish-tools` spec 仍有 mock-only 口径；本 change 必须同步更新，避免规格和实现继续漂移。
```

## openspec/changes/internal-trial-readiness/tasks.md

- Source: openspec/changes/internal-trial-readiness/tasks.md
- Lines: 1-32
- SHA256: 06270397dd86e583f5f619173f1eea13e8d41af044306c03948b9cb7d842a28e

```md
## 1. OpenSpec and Design

- [ ] 1.1 Confirm proposal, high-level design, delta specs and task scope for internal trial readiness.
- [ ] 1.2 Produce the Superpowers technical design doc through Comet design handoff.

## 2. Identity Boundary

- [ ] 2.1 Add or document internal-trial identity source configuration while preserving dev fallback behavior.
- [ ] 2.2 Cover missing identity, mismatched owner and dev fallback behavior with tests or fixtures.

## 3. Real Tool Trial Wiring

- [ ] 3.1 Add explicit internal-trial wiring for real read-only GitHub tool client without changing default mock behavior.
- [ ] 3.2 Ensure missing real tool configuration produces stable diagnostic state and trace.
- [ ] 3.3 Update `project_polish` behavior/spec tests so real, mock and failed tool paths are distinguishable.

## 4. Memory Observability Trial Gate

- [ ] 4.1 Ensure internal smoke or verification fixture observes long-term memory success, skipped, failed and conflict-exhausted states.
- [ ] 4.2 Confirm memory observation payloads do not include full answers, reports, tokens or private config.

## 5. Internal Trial Smoke and Quality Gates

- [ ] 5.1 Add a repeatable internal trial smoke command or script covering interview completion, report, memory observation, Agent project polish and tool trace.
- [ ] 5.2 Update `cmd/agent-verify` fixtures or related tests if the smoke needs new stable inputs.
- [ ] 5.3 Run Go, frontend, agent-verify, smoke and OpenSpec validation gates.

## 6. Documentation

- [ ] 6.1 Update backend/frontend SDD with internal trial boundaries and non-production limits.
- [ ] 6.2 Add code-change documentation after implementation based on the real diff.
- [ ] 6.3 Archive the OpenSpec change after verification passes.
```

## openspec/changes/internal-trial-readiness/specs/agentkit-mcp-tools/spec.md

- Source: openspec/changes/internal-trial-readiness/specs/agentkit-mcp-tools/spec.md
- Lines: 1-23
- SHA256: b3fe11c30ccfe16672ced5f8b07067de955d029626b48158c5ad2592d9f4af34

```md
## ADDED Requirements

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
```

## openspec/changes/internal-trial-readiness/specs/identity-access-boundary/spec.md

- Source: openspec/changes/internal-trial-readiness/specs/identity-access-boundary/spec.md
- Lines: 1-25
- SHA256: 31a36a677e8444f3a5ee3009f71b1c66f4689f8bd0bb85756016c6dc5c0e21de

```md
## ADDED Requirements

### Requirement: 内部试用身份来源必须区别于开发 fallback

系统 MUST 为内部团队试用提供明确身份来源边界，使试用环境可以从可信内部来源解析当前用户，同时保留开发模式 fallback 但不得把 fallback 描述为生产鉴权。

#### Scenario: 内部试用请求使用可信身份来源

- **WHEN** 服务运行在内部试用配置下
- **AND** 请求包含由可信内部网关或 allowlist 配置提供的当前用户标识
- **THEN** owner resolver MUST 使用该可信身份作为当前用户
- **AND** 业务 handler MUST 继续只依赖统一 resolver 和 authorizer

#### Scenario: 开发 fallback 不作为内部试用默认身份

- **WHEN** 服务运行在内部试用配置下
- **AND** 请求只提供开发模式 `X-User-ID` 或 `owner_user_id`
- **THEN** 系统 MUST NOT 静默把该 fallback 当作生产级身份来源
- **AND** 文档或配置 MUST 明确该路径仅用于本地开发或显式允许的试用调试

#### Scenario: 内部试用身份缺失

- **WHEN** 内部试用请求访问受保护用户资源但无法解析当前用户
- **THEN** 授权边界 MUST 拒绝访问
- **AND** 响应 MUST 使用稳定结构化错误格式
```

## openspec/changes/internal-trial-readiness/specs/long-term-memory/spec.md

- Source: openspec/changes/internal-trial-readiness/specs/long-term-memory/spec.md
- Lines: 1-22
- SHA256: 4b065ade378fff12434aabc5f0fccc0f586bf6472db13507bbacb7ecd64a989c

```md
## ADDED Requirements

### Requirement: 内部试用 smoke 应覆盖长期记忆观测结果

系统 MUST 在内部团队试用门禁中验证长期记忆写入观测信号，确保完整面试链路中的画像沉淀状态可诊断。

#### Scenario: 内部 smoke 观察长期记忆写入成功

- **WHEN** 内部 smoke 完成带 `user_id` 和 Report 的面试
- **THEN** 验证结果 MUST 能确认长期记忆写入成功观测
- **AND** 观测 MUST 包含稳定状态、目标用户和 session 标识

#### Scenario: 内部 smoke 观察长期记忆跳过或失败

- **WHEN** 内部 smoke 或 fixture 模拟缺少必要输入、Store 不可用、非冲突错误或 CAS 冲突耗尽
- **THEN** 验证结果 MUST 能区分 skipped、failed 和 conflict-exhausted 类状态
- **AND** 面试 completed 响应 MUST 保持不被长期记忆失败阻断

#### Scenario: 内部试用观测不泄露敏感正文

- **WHEN** 内部 smoke 收集长期记忆观测
- **THEN** 观测 payload MUST NOT 包含完整回答正文、完整报告正文、token、密钥或私有配置
```

## openspec/changes/internal-trial-readiness/specs/project-polish-tools/spec.md

- Source: openspec/changes/internal-trial-readiness/specs/project-polish-tools/spec.md
- Lines: 1-31
- SHA256: f5cd27b200ea13067e6dd2fd08f97ae69421c98b62e9b330a608afc6402ee332

```md
## MODIFIED Requirements

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
```

## openspec/changes/internal-trial-readiness/specs/quality-gates/spec.md

- Source: openspec/changes/internal-trial-readiness/specs/quality-gates/spec.md
- Lines: 1-32
- SHA256: f5647e6f169072091999d8641866c45eec4504b7cab0bbd98ad60dcab2daf6b8

```md
## ADDED Requirements

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
```

## openspec/changes/internal-trial-readiness/specs/system-design-documentation/spec.md

- Source: openspec/changes/internal-trial-readiness/specs/system-design-documentation/spec.md
- Lines: 1-17
- SHA256: 3fd6c3a3c274e412625e5250a0b3dc2ab753493e59fa5f56a251cb7fcf72291a

```md
## ADDED Requirements

### Requirement: SDD 必须声明内部试用边界和非生产限制

项目 MUST 在系统设计文档中明确内部团队试用能力边界，避免把内部试用、mock 工具、开发 fallback 或 Codex 开发协作能力描述成对外生产能力。

#### Scenario: 后端 SDD 描述内部试用边界

- **WHEN** 读者查看 `docs/SDD-Backend.md`
- **THEN** 文档 MUST 说明内部试用支持的身份来源、真实 GitHub 工具显式启用、长期记忆观测和 smoke 门禁
- **AND** 文档 MUST 声明未实现完整 JWT/OIDC、租户体系、完整 MCP runtime 或运行时 sub-agent

#### Scenario: 前端 SDD 描述内部试用边界

- **WHEN** 读者查看 `docs/SDD-Frontend.md`
- **THEN** 文档 MUST 说明前端只展示后端返回的状态、限制和 trace
- **AND** 文档 MUST 声明前端不直接调用 GitHub、MCP 服务、用户中心或长期记忆写接口
```

