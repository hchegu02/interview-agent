## Context

上一阶段已经在 Graph 中接入 `update_difficulty`：

```text
evaluate -> update_memory -> update_difficulty -> reflection_check
```

这个节点会根据当前轮最终评分维护 `WorkingMemory.Difficulty`。但下一题候选池由 `retrieve_rag` 生成，当前 `retrieve_rag` 的目标难度仍只来自静态 option 和 `GapStrategy`。因此动态难度没有实际进入出题链路。

## Goals / Non-Goals

**Goals:**

- `retrieve_rag` 使用 `WorkingMemory.Difficulty.Current` 推导基础目标难度。
- easy/medium/hard 映射到题库难度 2/3/4。
- 继续使用现有 `GapStrategy` 调整目标难度。
- 用户题库过滤条件继续作为硬约束传给 retriever。
- 旧 Session 缺少动态难度状态时保持默认行为。

**Non-Goals:**

- 不让 LLM 决定难度。
- 不改 `update_difficulty` 状态机规则。
- 不改 `pick_next` 的排序策略。
- 不读取长期记忆。
- 不新增 HTTP 响应字段。

## Decisions

### 1. 在 `retrieve_rag` 内解析动态难度

`retrieve_rag` 是构造 `retriever.Query.Difficulty` 的唯一业务节点，把动态难度映射放在这里最直接，也避免把题库难度概念泄漏到 `update_difficulty` 节点。

### 2. 三档映射为 2/3/4

`DifficultyEasy/Medium/Hard` 是面试节奏状态，不直接等同题库难度 1/2/3。映射到 2/3/4 可以避免 easy 直接落到过浅题，也给 `GapStrategyValidate` 和 `GapStrategyCoverGap` 保留上调/下调空间。

### 3. 显式过滤条件不被动态难度覆盖

`QuestionBankFilter.DifficultyMin/DifficultyMax` 是用户或前端给出的硬范围。动态难度只写入 `retriever.Query.Difficulty` 作为软目标，不能覆盖硬过滤字段。

## Risks / Trade-offs

- [Risk] hard + validate 会被调到 5，可能偏深。Mitigation：这是现有 `GapStrategy` 语义，仍由 clamp 限制到 1-5。
- [Risk] 旧 Session 没有 WorkingMemory。Mitigation：回退静态 `TargetDifficulty`。
- [Risk] 当前只影响候选池，不影响 `pick_next` 的 LLM prompt。Mitigation：先让检索闭环，后续再决定是否把动态难度显式写入选题 prompt。
