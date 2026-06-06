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
