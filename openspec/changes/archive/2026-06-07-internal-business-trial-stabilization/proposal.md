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
