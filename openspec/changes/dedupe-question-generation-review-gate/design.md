# Design

## Current Behavior

`gateQuestionCandidates` 当前只使用 `seenContent` 检测同批 LLM 返回中的重复题。`GenerationService.Stage` 会把通过门禁的 candidates 转成 import items，再进入现有人工审核流程。`ImportService.Commit` 只提交 `ImportItemStatusValid` 且 review 策略允许的 item。

现有 `Commit` 已阻止 `needs_human_review` 和 `rejected` 静默进入正式题库，但它不知道“正式题库已有同题干”或“同一 import job 内多个 item 归一化后重复”。

## Approach

采用保守的文本归一化去重，不引入新 schema：

1. **统一归一化函数**
   - 复用或提升 `normalizeCandidateContent` 的语义。
   - 归一化规则只做大小写、空白和常见标点压缩，避免误杀语义相近但不同的问题。

2. **生成阶段去重**
   - 在 `GenerationService.Generate` 或 Stage 前加载正式题库中 active 题目的归一化 content key。
   - `gateQuestionCandidates` 支持传入已有 content keys。
   - 与已有题库重复的 candidate 进入 `RejectedCandidates`，标记 `duplicate_existing_content` 或复用扩展后的 duplicate flag。

3. **暂存/提交阶段保护**
   - 在 import staging 或 commit 前，对同一 job 内 item 做 content key 去重。
   - 重复 item 不写正式题库，并保留 review/agent review reason。
   - commit 前再次读取正式题库 active items，过滤重复内容，避免并发或旧数据绕过生成门禁。

4. **可诊断反馈**
   - 对重复题写入 `AgentReviewStatus=rejected` 或保留 `QualityFlags`，理由包含重复类型。
   - 不要求前端立即展示新字段，但 API 现有 import item / generation job 返回应能看到原因字段。

## Data Flow

```text
source chunks
  -> concept cards
  -> LLM candidates
  -> generation quality gates
       - required fields
       - source refs
       - same batch duplicate
       - existing question bank duplicate
  -> accepted candidates
  -> staged import items
       - same job duplicate guard
       - agent review status/reason
  -> human review
  -> commit
       - review policy
       - existing question bank duplicate guard
  -> question_bank + embedding
```

## Compatibility

- 不改变现有 `question_bank` schema。
- 旧导入项仍按现有 review 规则提交。
- 没有配置 PG store 或无法读取正式题库时，生成阶段至少保留同批去重；commit 阶段仍按已有 writer 行为执行，但测试应覆盖 memory store 路径。

## Risks

- 文本归一化过强会误杀相似但不同的题。第一版只做保守 exact-normalized dedupe。
- commit 前过滤重复题可能导致 import job `ImportedItems` 小于 accepted item 数量，需要测试确认状态和计数语义清楚。
- 如果正式题库已有重复历史数据，本 change 不负责清理历史数据，只防止新增。
