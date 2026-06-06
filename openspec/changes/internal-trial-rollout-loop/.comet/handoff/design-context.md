# Comet Design Handoff

- Change: internal-trial-rollout-loop
- Phase: design
- Mode: compact
- Context hash: 92914082e2e1b3c6b55a5961f45402cc3f61257a7895d629366bee4f8b5a3174

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/internal-trial-rollout-loop/proposal.md

- Source: openspec/changes/internal-trial-rollout-loop/proposal.md
- Lines: 1-32
- SHA256: 6b2c7f9f772e329c67806eed4bb106cb072fffb8b3d0e79480744e79fcce60d9

```md
## Why

Interview Agent 已经具备内部团队试用的基础能力，但当前交付物仍偏工程 readiness：有配置、smoke 和边界说明，却缺少一套能让技术团队和业务试用者按同一口径执行、记录、诊断和收敛反馈的闭环。

现在继续堆真实鉴权、MCP runtime 或产品化平台会过早扩大范围。下一步应先把内部试用跑法固化，保证失败可复现、反馈可比较、Go/No-Go 决策有证据。

## What Changes

- 增加内部试用闭环能力，明确技术试用先行、业务试用后置的两阶段流程。
- 增加技术试用 Runbook，覆盖启动、验证、失败注入或失败观察、trace/memory/tool 状态采集。
- 增加业务试用 Runbook，覆盖 HR/面试官视角的面试、项目润色、报告评价和反馈收集。
- 增加问题记录模板，要求记录 session id、页面/API、复现步骤、期望/实际、`tool_trace` 和长期记忆观测状态。
- 增加产品反馈评分模板，轻量评估题目质量、追问质量、报告可信度、项目润色质量和整体可用性。
- 增加 Go/No-Go 标准，明确何时继续业务试用、暂停、回滚 mock 或禁止扩大范围。
- 不引入数据库 schema、dashboard、生产鉴权、完整 MCP runtime 或运行时 sub-agent。

## Capabilities

### New Capabilities

- `internal-trial-rollout`: 定义内部试用从技术验证到业务试用的操作闭环、记录模板、反馈指标和 Go/No-Go 决策边界。

### Modified Capabilities

None.

## Impact

- 主要影响文档、试用模板和验证说明。
- 可能补充轻量脚本或命令入口说明，但不得新增生产运行时依赖。
- 不改变 HTTP API、数据库 schema、Session JSON、前端类型或默认 mock/offline 行为。
- 不提交 token、密钥、私有配置或真实用户数据。
```

## openspec/changes/internal-trial-rollout-loop/design.md

- Source: openspec/changes/internal-trial-rollout-loop/design.md
- Lines: 1-69
- SHA256: 67ef11f6bbbf8afbcd6c36a866db96a6d6f5f15c3c60ab7b06fad743e171b31b

```md
## Context

当前 `internal-trial-readiness` 已完成并归档，系统具备内部试用的工程基础：内部试用配置、可信 owner header 边界、默认 mock/offline 工具、显式真实 GitHub 工具、长期记忆观测、`tool_trace` 和离线 `cmd/internal-trial-smoke`。

缺口不在单个功能，而在试用闭环：技术团队如何启动和诊断，业务/HR/面试官如何执行脚本，失败如何记录，反馈如何比较，什么条件下继续、暂停或回滚。没有这个闭环，内部试用会退化成口头演示，问题不可复现，产品反馈也不可比较。

## Goals / Non-Goals

**Goals:**

- 固化两阶段试用流程：技术团队先跑通和诊断，业务试用后进入。
- 让每个失败都有可复现记录：session id、页面/API、复现步骤、期望/实际、trace 状态和相关观测。
- 让业务反馈轻量但可比较：题目质量、追问质量、报告可信度、项目润色质量、整体可用性。
- 给出明确 Go/No-Go 标准，避免把 mock/offline、开发 fallback 或内部 header 描述成生产能力。

**Non-Goals:**

- 不实现 dashboard、反馈收集平台或数据仓库。
- 不实现完整 JWT/OIDC、租户、用户中心或生产级权限体系。
- 不实现完整 MCP Gateway/runtime、daemon、Sandbox 或运行时 sub-agent。
- 不改变默认 mock/offline 行为。
- 不处理真实用户隐私合规和外部生产发布。

## Decisions

### Decision 1: 文档和模板优先，不先做平台

采用 Runbook + 模板 + Go/No-Go 标准。内部试用的第一轮目标是产生高质量问题记录和产品反馈，不是自动化运营平台。

备选方案是直接做 dashboard 或 report collector。这个方案现在不合适：还没有真实试用数据，容易把采集字段、状态分类和评分维度做错。

### Decision 2: 技术试用先行，业务试用后置

技术团队先执行验证命令、启动服务、确认 smoke、检查 `tool_trace` 和长期记忆观测。业务/HR/面试官只在技术试用通过后进入固定脚本。

备选方案是直接让业务团队试用。风险是环境、mock 边界或 trace 配置问题会被误判成产品体验问题。

### Decision 3: 失败记录必须结构化，但不采集敏感正文

问题模板必须记录复现步骤和诊断字段，但不得要求记录 token、密钥、完整回答正文、完整报告正文或私有配置。`tool_trace` 和 memory observation 只记录状态、错误类别和摘要。

备选方案是让试用者自由描述问题。这个方案会导致问题难复现，后续修复成本高。

### Decision 4: Go/No-Go 使用最小硬门槛

继续业务试用前必须满足核心验证通过、默认不访问公网 GitHub、失败 trace 可诊断、长期记忆失败不阻断面试完成。扩大范围前再根据反馈结果决定是否设计平台化或生产化。

备选方案是把所有产品评分都做成硬门槛。第一轮数据量太小，硬门槛会制造伪精确。

## Risks / Trade-offs

- [Risk] 人工模板仍可能漏填关键字段 → Mitigation: 模板把 session id、复现步骤、trace 状态和期望/实际设为必填。
- [Risk] 业务试用者误以为这是生产发布 → Mitigation: Runbook 和 Go/No-Go 明确内部试用、mock/offline 和非生产边界。
- [Risk] 真实 GitHub 工具问题被混入默认试用反馈 → Mitigation: 默认试用使用 `github_tool_mode=mock`，real 模式单独标注和记录。
- [Risk] 文档散落导致执行口径不一致 → Mitigation: 以 `docs/ai/internal-trial-launch-checklist.md` 为入口，新增模板从该入口链接。

## Migration Plan

1. 补充内部试用入口文档，链接技术 Runbook、业务 Runbook、问题模板、反馈模板和 Go/No-Go 标准。
2. 对照现有验证命令更新技术 Runbook。
3. 用固定脚本描述业务试用流程，避免业务试用者需要理解内部 tool/runtime 细节。
4. 本地验证文档引用、OpenSpec strict validation 和必要的配置测试。

回滚策略：本 change 主要是文档和模板。若发现口径错误，可直接修订文档；不影响运行时代码。

## Open Questions

- 第一轮内部试用人数建议控制在 3-5 名技术试用者和 2-3 名业务试用者，实际名单由团队决定。
- 是否需要在第二轮把问题记录模板升级成表单或 dashboard，等第一轮反馈后再判断。
```

## openspec/changes/internal-trial-rollout-loop/tasks.md

- Source: openspec/changes/internal-trial-rollout-loop/tasks.md
- Lines: 1-24
- SHA256: b6ded460454988c4f4dd73b147da0c28f836b53ca0386be448a400b7e85b7f49

```md
## 1. OpenSpec And Design

- [ ] 1.1 Create proposal, design, delta spec, and task checklist for internal trial rollout.
- [ ] 1.2 Run OpenSpec validation for the change and repair artifact issues.

## 2. Rollout Documents

- [ ] 2.1 Update the internal trial launch checklist as the single entry point.
- [ ] 2.2 Add technical trial Runbook with startup, validation, diagnosis, and rollback steps.
- [ ] 2.3 Add business trial Runbook with fixed user flow and feedback collection steps.
- [ ] 2.4 Add issue record template with required reproduction and diagnostic fields.
- [ ] 2.5 Add product feedback scoring template.
- [ ] 2.6 Add Go/No-Go standard for continuing, pausing, rolling back, or expanding trial scope.

## 3. Verification

- [ ] 3.1 Verify documentation links and tracked paths.
- [ ] 3.2 Run minimal config or docs-related tests if touched.
- [ ] 3.3 Run `openspec validate internal-trial-rollout-loop --strict`.

## 4. Closeout

- [ ] 4.1 Update code-change documentation if implementation changes code.
- [ ] 4.2 Commit rollout documents and OpenSpec artifacts.
```

## openspec/changes/internal-trial-rollout-loop/specs/internal-trial-rollout/spec.md

- Source: openspec/changes/internal-trial-rollout-loop/specs/internal-trial-rollout/spec.md
- Lines: 1-59
- SHA256: 15952bd7aedc6571a88e1e31893f3f71dc9e1a385ae759e56aa106b33835f901

```md
## ADDED Requirements

### Requirement: 内部试用必须分阶段执行

系统试用流程 MUST 明确先由技术团队完成内部试用技术验证，再开放给业务、HR 或面试官进行产品体验试用。

#### Scenario: 技术试用通过后进入业务试用

- **WHEN** 技术团队完成内部试用启动、验证命令、smoke、tool trace 和长期记忆观测检查
- **THEN** 试用流程 MUST 允许进入业务试用阶段
- **AND** 业务试用说明 MUST 明确当前不是生产发布

#### Scenario: 技术试用未通过

- **WHEN** 任一核心验证命令、smoke 或关键诊断链路失败
- **THEN** 试用流程 MUST 阻止扩大到业务试用
- **AND** 维护者 MUST 记录失败原因和复现信息

### Requirement: 内部试用问题记录必须可复现

内部试用问题记录 MUST 使用结构化模板，确保失败可以被开发团队复现和归类。

#### Scenario: 记录技术失败

- **WHEN** 技术试用发现启动、验证、API、tool trace 或长期记忆观测失败
- **THEN** 问题记录 MUST 包含 session id 或命令、复现步骤、期望结果、实际结果和错误类别
- **AND** 问题记录 MUST NOT 包含 token、密钥、完整回答正文或完整报告正文

#### Scenario: 记录业务体验问题

- **WHEN** 业务试用者反馈题目、追问、报告或项目润色质量问题
- **THEN** 问题记录 MUST 包含试用脚本步骤、用户可见结果、期望结果和可选 session id
- **AND** 反馈 MUST 区分产品质量问题和技术链路失败

### Requirement: 内部试用必须收集轻量产品反馈

业务试用流程 MUST 收集轻量、可比较的产品价值反馈，而不是只记录自由文本。

#### Scenario: 完成业务试用反馈

- **WHEN** 业务试用者完成一次完整面试和一次项目润色请求
- **THEN** 反馈模板 MUST 记录题目质量、追问质量、报告可信度、项目润色质量和整体可用性评分
- **AND** 反馈模板 MUST 支持补充简短文字说明

### Requirement: 内部试用必须定义 Go/No-Go 标准

内部试用流程 MUST 定义继续、暂停、回滚和禁止扩大范围的判定标准。

#### Scenario: 允许继续业务试用

- **WHEN** 核心验证门禁通过、默认配置不访问公网 GitHub、失败 trace 可诊断且长期记忆失败不阻断面试完成
- **THEN** 维护者 MAY 标记当前版本可进入受控业务试用
- **AND** 试用说明 MUST 保持内部试用和非生产边界

#### Scenario: 暂停或回滚试用

- **WHEN** 出现不可复现的关键失败、真实工具失败被伪装成成功、身份边界混乱或验证门禁失败
- **THEN** 维护者 MUST 暂停扩大试用范围
- **AND** 工具相关问题 MUST 优先回滚到 mock 模式
```

