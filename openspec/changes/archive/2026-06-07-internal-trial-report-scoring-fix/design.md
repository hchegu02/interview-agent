## Context

当前后端 `Report` 只保存总分、技能拆分、转录分析、训练计划和摘要列表；前端报告页的“逐题评分”主要从 `session.rounds` 临时拼出来。这个结构有两个坏味道：报告本身没有声明逐题明细契约；UI 可以显示 rounds，但报告一致性门禁无法验证“报告覆盖了所有有效回答、原答案没有丢、总分来自逐题分”。

本 change 优先修业务试用可信度，不做完整表单改版。真实 JD 只作为试用样例输入，招聘申请信息采集和大布局改造后续另开 change。

## Approach

采用“报告明细成为后端报告契约，前端只展示契约”的方式。

1. 后端在 `domain.Report` 增加兼容字段，例如 `round_reviews,omitempty`。
2. `report` 节点从 `Session.Rounds` 构建逐题明细：
   - 主问题：题号、题目 ID、题目正文、题型、原始回答、最终评分、命中点、缺失点、建议、是否参与总分。
   - 追问：作为主问题下的子明细，包含追问正文、原始回答和独立评分；已回答追问必须显示。
3. 总分只基于有效主问题最终评分聚合；追问评分用于单题证据和工作记忆，不默认重复计入总分，除非现有工作记忆规则明确合成。报告中必须说明聚合口径。
4. HTTP completed session 返回时继续保留 `rounds`，并返回 `report.round_reviews`；旧前端仍可读 `rounds`，新前端优先读报告明细。
5. 前端报告页顶部展示总分和摘要，随后立即展示逐题复盘；Agent/Graph/检索 trace 等诊断信息移到靠后区域。
6. 面试页按 `session.mode` 做最小展示隔离：
   - `exam`：隐藏题库范围、Agent 状态、Graph/事件调试状态。
   - `practice`：保留训练辅助，但调试信息不放在题目前面。

## Data Flow

```text
Session.Rounds
  -> report node
  -> Report.RoundReviews
  -> HTTP Session.report
  -> web ReportPage
  -> 逐题复盘卡片
```

每个 `RoundReview` 的分数来自 `AnswerRound.FinalEvaluation()`，即优先使用 refined evaluation。原始回答来自 `AnswerRound.Answer` 和 `FollowUp.Answer`，不做摘要、不改写。

## Error Handling

- 已回答但缺评分：报告明细必须标记为未评分，不参与有效总分；验证门禁应能捕获。
- 未回答题：可显示为未回答，但不得参与总分，不得生成高分。
- 总分和逐题有效分不一致：报告一致性验证失败。
- 旧 Session 缺少新字段：HTTP 仍兼容；新报告字段只在新生成报告后出现。

## Testing

- Go：覆盖 report node 生成所有主问题和已回答追问明细。
- Go：覆盖原始回答保留、未回答不计分、总分由有效逐题分聚合。
- Go：覆盖 HTTP completed session 返回 report round reviews。
- 前端：覆盖报告页展示原答案、逐题评分、追问评分，以及 exam 模式隐藏调试状态。
- 门禁：扩展 agent-verify 或 smoke，让缺题、缺答案、缺评分理由或总分不一致时失败。

## Trade-offs

- 不把完整招聘申请表单塞进本 change。它会影响开始页信息架构、数据模型和更多 UI 流程，应该单独设计。
- 不把追问默认计入总分。现有 `overallScore` 按主问题 `FinalEvaluation()` 聚合，改成追问混合会改变评分语义；本 change 先把追问作为独立证据展示。
- 不强制真实 LLM/Embedding。内部试用可信度先靠结构正确性和可复跑 fixture 保证。
