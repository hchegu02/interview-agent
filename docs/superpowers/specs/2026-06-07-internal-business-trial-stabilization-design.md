---
comet_change: internal-business-trial-stabilization
role: technical-design
canonical_spec: openspec
archived-with: 2026-06-07-internal-business-trial-stabilization
status: final
---

# 内部业务试用稳定版技术设计

## 目标

本阶段把 A 部分“内部业务试用稳定版”从文档判断推进到本地可复跑门禁。它只证明当前构建具备扩大内部业务试用的最低证据，不代表生产上线批准。

## 技术方案

采用扩展 `cmd/internal-trial-smoke` 的方案。新增一份非敏感业务反馈 fixture，由验证逻辑检查固定脚本是否完成、关键评分是否在 1-5 范围内、是否给出扩大试用结论，以及阻断项是否与扩大结论冲突。

默认 smoke 继续保持离线、mock、无公网依赖。通过后输出稳定 marker：

```text
business_trial: feedback evidence verified
```

失败时返回非 0，并输出可定位的失败原因。

## 组件

### 业务反馈 fixture

位置：`testdata/internal_trial/business_feedback_pass.json`

字段只保留试用角色、场景、脚本完成状态、面试流程评分、报告可用性评分、项目润色评分、是否建议扩大内部试用、阻断标记和简短摘要。不得包含 token、私有仓库、完整简历、完整回答或完整报告。

### 校验逻辑

优先放在 `internal/agentkit/verify`，与现有 report、retrieval trace、tool event、memory observation verifier 保持一致。

校验规则：

- 固定脚本必须完成。
- 三类评分必须在 1-5。
- 扩大试用结论必须明确。
- 如果存在阻断项，不能标记为适合扩大试用。
- 必填字段缺失、JSON 错误、评分越界均失败。

### Smoke 集成

`cmd/internal-trial-smoke` 增加默认 fixture 路径和加载逻辑。默认执行路径包含业务反馈校验，不新增外部依赖，不启动 HTTP 服务，不访问公网。

## 数据流

```text
testdata/internal_trial/business_feedback_pass.json
  -> verify.BusinessTrialFeedbackVerifier
  -> cmd/internal-trial-smoke.run
  -> stdout marker / stderr failures / exit code
  -> internal trial launch checklist
```

## 错误处理

- fixture 不存在：smoke 失败。
- JSON 无法解析：smoke 失败。
- 关键字段缺失：smoke 失败。
- 评分不在 1-5：smoke 失败。
- `has_blocker=true` 且 `expand_recommendation=yes`：smoke 失败。

## 测试策略

- `internal/agentkit/verify` 增加单测，覆盖通过、缺字段、评分越界、脚本未完成、阻断项与扩大结论冲突。
- `cmd/internal-trial-smoke` 增加测试，确认默认输出包含 `business_trial` marker。
- 最小验证命令：

```powershell
go test ./cmd/internal-trial-smoke ./internal/agentkit/verify -count=1
go run ./cmd/internal-trial-smoke
openspec validate internal-business-trial-stabilization --strict
```

## 风险

- 该门禁只能验证业务试用证据结构和最低一致性，不能自动判断报告质量是否真实优秀。
- 如果后续要收集真实反馈，应另开 change 设计存储、权限、脱敏和删除策略。
- 不应把 `business_trial` marker 解读为生产上线批准。
