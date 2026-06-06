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
