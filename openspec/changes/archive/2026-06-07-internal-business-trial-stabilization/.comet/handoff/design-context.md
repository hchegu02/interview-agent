# Comet Design Handoff

- Change: internal-business-trial-stabilization
- Phase: design
- Mode: compact
- Context hash: 60d3439bb16b7a311024e22df35b89c9206686636640206f5adfb23a6ace3505

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/internal-business-trial-stabilization/proposal.md

- Source: openspec/changes/internal-business-trial-stabilization/proposal.md
- Lines: 1-29
- SHA256: 772d08d6e52ed7937d930d6f00d0e5b47c47d8db598bb1d154d4baec26d9ce42

```md
## Why

Interview Agent 已经具备内部技术试用和 Go 后端 RAG 题库业务试用能力，但“业务试用稳定版”的判定仍主要依赖人工阅读 runbook。现在需要把业务试用脚本、反馈汇总和最小门禁固化成可复跑的工程入口，避免把“能演示”误判成“可扩大内部试用”。

## What Changes

- 增加内部业务试用稳定版判定：明确必须覆盖面试流程、报告、项目润色、长期记忆和题库/RAG trace 的可解释性。
- 增加业务试用反馈 fixture 或等价本地输入，支持维护者在扩大试用前检查反馈字段完整性和阻断项。
- 扩展内部试用 smoke，使其能输出业务试用稳定版相关 marker，并在缺少关键业务试用证据时失败。
- 更新内部试用 runbook、Go/No-Go 和启动清单，让业务试用扩大条件和验证命令一致。
- 不引入生产登录、租户、计费、完整 MCP runtime、运行时 sub-agent 或外部 SLA。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `internal-trial-rollout`: 增加内部业务试用稳定版的反馈证据、扩大条件和暂停条件。
- `quality-gates`: 增加业务试用 smoke / fixture 门禁，确保扩大内部试用前有可复跑证据。

## Impact

- 代码：预计影响 `cmd/internal-trial-smoke`、`internal/agentkit/verify` 或新增轻量 trial fixture 校验模块。
- 测试数据：新增业务试用反馈 fixture，避免依赖真实候选人数据。
- 文档：更新 `docs/ai/internal-trial-*`、`docs/SDD-Backend.md` 或 `docs/SDD-Frontend.md` 中的内部试用边界。
- 验证：继续使用 Go test、前端 test/build、agent-verify、internal-trial-smoke、OpenSpec strict validation；必要时加入 business trial fixture 校验。
```

## openspec/changes/internal-business-trial-stabilization/design.md

- Source: openspec/changes/internal-business-trial-stabilization/design.md
- Lines: 1-54
- SHA256: 065e99024e7ddfeee64191dc2a768f65d4cd0442b11608edd3684a70742bef6d

```md
## Context

当前内部试用体系已有技术 smoke、tool trace、长期记忆观测和业务 runbook。缺口在于业务试用扩大前缺少机器可验证的最低证据：是否完成固定脚本、是否包含关键评分、是否存在阻断项、是否把内部试用误写成生产发布。

## Goals

- 把“内部业务试用稳定版”的 Go/No-Go 从纯文档判断推进到本地可复跑验证。
- 保持默认离线、mock、无公网依赖。
- 让业务反馈和阻断问题使用结构化 fixture，方便开发团队复现和归类。
- 保留非生产边界，不把该阶段扩展成真实生产上线。

## Non-Goals

- 不接入 JWT/OIDC、租户、RBAC、计费或真实用户中心。
- 不实现完整 MCP Gateway、daemon、Sandbox 或运行时 sub-agent 调度。
- 不把真实候选人数据、完整简历、完整回答或完整报告纳入 fixture。
- 不要求默认 smoke 访问公网 GitHub 或真实外部工具。

## Approach

采用“扩展内部试用 smoke + 结构化业务反馈 fixture”的保守方案：

1. 新增或复用验证结构，读取一份业务试用反馈 fixture。
2. 校验固定脚本完成状态、面试流程评分、报告可用性评分、项目润色评分、是否建议扩大试用、阻断问题字段。
3. 将该校验纳入 `cmd/internal-trial-smoke` 默认离线路径，输出 `business_trial` marker。
4. 文档把内部业务试用稳定版定义为“可扩大内部试用”，不是生产上线批准。

该方案不需要新服务、不需要数据库迁移，也不改变 HTTP API。它把试用扩大前的最低证据变成门禁，适合当前阶段。

## Data Flow

```text
testdata/internal_trial/business_feedback_pass.json
  -> business feedback verifier
  -> cmd/internal-trial-smoke
  -> stdout marker + non-zero failure on missing/invalid evidence
  -> launch checklist / Go-No-Go docs
```

业务反馈 fixture 只保存试用角色、场景、脚本完成状态、1-5 分评分、是否适合扩大内部试用、最小问题摘要和阻断标记。它不得包含 token、私有仓库、完整简历、完整回答或完整报告。

## Error Handling

- fixture 缺失、JSON 格式错误或关键字段缺失：smoke 返回非 0。
- 评分越界或脚本未完成但仍标记可扩大：smoke 返回非 0。
- 出现阻断项但仍标记可扩大：smoke 返回非 0。
- 反馈文本为空不一定失败，但关键评分和扩大结论必须存在。

## Testing

- 新增 verifier 单测，覆盖通过 fixture、缺字段、评分越界、阻断项与扩大结论冲突。
- 扩展 `cmd/internal-trial-smoke` 测试，确认默认输出包含 `business_trial` marker。
- 运行 `go test ./cmd/internal-trial-smoke ./internal/agentkit/verify -count=1`。
- 收口时运行内部试用清单中的核心门禁。
```

## openspec/changes/internal-business-trial-stabilization/tasks.md

- Source: openspec/changes/internal-business-trial-stabilization/tasks.md
- Lines: 1-21
- SHA256: 457902e3c56b4be4338f1bf3054f5904cd0c9a0ea150d64331dd6b9024b650a3

```md
- [ ] 1. 增加业务试用反馈 fixture 校验
  - [ ] 1.1 定义最小业务反馈数据结构，限制为非敏感字段。
  - [ ] 1.2 实现校验逻辑：脚本完成、评分范围、扩大结论、阻断项冲突。
  - [ ] 1.3 增加通过和失败单测。
- [ ] 2. 扩展内部试用 smoke
  - [ ] 2.1 新增默认业务反馈 fixture 路径。
  - [ ] 2.2 将业务反馈校验纳入默认离线 smoke。
  - [ ] 2.3 输出稳定 `business_trial` marker。
- [ ] 3. 更新内部试用文档
  - [ ] 3.1 更新启动清单，加入业务反馈 fixture / smoke marker。
  - [ ] 3.2 更新业务试用 runbook，说明扩大条件和阻断条件。
  - [ ] 3.3 更新 Go/No-Go，明确内部业务试用稳定版不是生产上线。
  - [ ] 3.4 如接口或边界说明变化，同步 SDD。
- [ ] 4. 记录代码变更文档
  - [ ] 4.1 创建 `docs/code-changes/06-07-internal-business-trial-stabilization.md`。
  - [ ] 4.2 基于真实 diff 写入函数级说明、调用链、数据流、验证和风险。
- [ ] 5. 验证和收口
  - [ ] 5.1 运行 `go test ./cmd/internal-trial-smoke ./internal/agentkit/verify -count=1`。
  - [ ] 5.2 运行 `go run ./cmd/internal-trial-smoke`。
  - [ ] 5.3 运行 `openspec validate internal-business-trial-stabilization --strict`。
  - [ ] 5.4 更新 tasks 勾选并进入 Comet verify。
```

## openspec/changes/internal-business-trial-stabilization/specs/internal-trial-rollout/spec.md

- Source: openspec/changes/internal-business-trial-stabilization/specs/internal-trial-rollout/spec.md
- Lines: 1-35
- SHA256: 5f90a80929fdf88be378b62cd1c8573cb4567d4227f8cf69a8ce597745899a99

```md
## MODIFIED Requirements

### Requirement: 内部试用必须收集轻量产品反馈

业务试用流程 MUST 收集轻量、可比较的产品价值反馈，而不是只记录自由文本。系统 SHOULD provide a minimal machine-checkable feedback fixture so maintainers can verify that business-trial evidence is present before expanding the trial.

#### Scenario: 完成业务试用反馈

- **WHEN** 业务试用者完成一次完整面试和一次项目润色请求
- **THEN** 反馈模板 MUST 记录题目质量、追问质量、报告可信度、项目润色质量和整体可用性评分
- **AND** 反馈模板 MUST 支持补充简短文字说明

#### Scenario: 业务试用稳定版反馈证据可校验

- **WHEN** 维护者准备把内部试用扩大给更多业务试用者
- **THEN** 系统 MUST 提供本地可复跑的业务反馈证据校验
- **AND** 校验 MUST 覆盖固定脚本完成状态、关键评分、扩大结论和阻断项标记
- **AND** 业务反馈证据 MUST NOT 包含 token、私有仓库、完整简历、完整回答或完整报告

### Requirement: 内部试用必须定义 Go/No-Go 标准

内部试用流程 MUST 定义继续、暂停、回滚和禁止扩大范围的判定标准。业务试用稳定版 MUST be treated as controlled internal expansion, not production launch approval.

#### Scenario: 允许继续业务试用

- **WHEN** 核心验证门禁通过、默认配置不访问公网 GitHub、失败 trace 可诊断且长期记忆失败不阻断面试完成
- **AND** 业务试用反馈证据校验通过
- **THEN** 维护者 MAY 标记当前版本可进入受控业务试用
- **AND** 试用说明 MUST 保持内部试用和非生产边界

#### Scenario: 暂停或回滚试用

- **WHEN** 出现不可复现的关键失败、真实工具失败被伪装成成功、身份边界混乱、验证门禁失败或业务反馈存在阻断项但仍被标记可扩大
- **THEN** 维护者 MUST 暂停扩大试用范围
- **AND** 工具相关问题 MUST 优先回滚到 mock 模式
```

## openspec/changes/internal-business-trial-stabilization/specs/quality-gates/spec.md

- Source: openspec/changes/internal-business-trial-stabilization/specs/quality-gates/spec.md
- Lines: 1-39
- SHA256: 95a7af44a2b6fe4137f0e05803e563445bde05b8bc1c17a831525efc23139199

```md
## MODIFIED Requirements

### Requirement: 本地质量门禁应包含内部试用 smoke

系统 MUST 提供可重复执行的内部试用 smoke 验证，用于判断当前构建是否达到内部团队真实试用门槛。Smoke SHOULD include a business-trial evidence check before maintainers mark a version ready for broader internal business use.

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

#### Scenario: 内部业务试用反馈证据通过 smoke

- **WHEN** 维护者运行默认内部试用 smoke
- **THEN** smoke MUST 校验业务试用反馈 fixture 的关键字段和扩大结论
- **AND** smoke MUST 输出稳定业务试用 marker
- **AND** fixture 缺失、评分越界、脚本未完成却标记可扩大、或阻断项与扩大结论冲突时 smoke MUST 返回非 0

### Requirement: 内部试用前必须运行既有验证门禁

系统 MUST 在内部试用说明中列出并维护当前必跑验证命令。

#### Scenario: 内部试用发布检查

- **WHEN** 维护者准备标记某版本可供内部试用
- **THEN** 检查清单 MUST 包含 Go 测试、前端测试、前端构建、agent-verify tool/memory fixtures、内部试用 smoke、业务反馈证据校验和 OpenSpec strict validation
```

