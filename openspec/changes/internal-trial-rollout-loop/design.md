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
