## Context

`update_memory` 已经把当前题评分合并到 `WorkingMemory`，`pick_next` 和 `retrieve_rag` 也都读 `WorkingMemory`。动态难度属于单次面试中的运行时决策状态，应放在 `WorkingMemory`，而不是长期记忆或 HTTP 层。

当前 RAG 已有 `TargetDifficulty`，但它是静态 option，并且还会被 `GapStrategy` 微调。第一版先把动态难度状态维护好，下一阶段再让 retrieve/pick 节点读取它。

## Goals / Non-Goals

**Goals:**

- 新增 `Difficulty` 和 `DifficultyState`。
- `WorkingMemory` 持有当前动态难度状态。
- 新增 `update_difficulty` 节点，基于当前轮最终评分更新难度。
- Graph 在 `update_memory` 之后执行 `update_difficulty`。
- 降级评分 `Score < 0` 不影响难度状态。

**Non-Goals:**

- 不让 LLM 直接决定难度。
- 不读取长期记忆中的历史弱点。
- 不改变 RAG query 的目标难度。
- 不新增 HTTP 字段。
- 不改数据库 schema。

## Decisions

### 1. 状态放在 `WorkingMemory`

动态难度是当前 Session 的运行时状态，和 `AvgScore`、`RoundsAsked`、`WeakSkills` 一样服务 Agent loop。放在 `WorkingMemory` 可以随 Session 快照恢复，并保持 Graph 节点边界清楚。

备选方案是放在长期记忆。长期记忆跨 Session，不适合记录当前连对/连错 streak。

### 2. 独立 `update_difficulty` 节点

虽然 `update_memory` 已经读取评分，但动态难度是独立决策，单独节点更利于测试、观测和后续替换规则。

备选方案是在 `update_memory` 内直接修改难度。这个会让一个节点同时做“技能信号聚合”和“难度策略”，后续复杂度会堆在一起。

### 3. 简单三档规则

第一版规则：

- 初始难度为 medium。
- `score >= 80` 记一次 correct streak，清空 wrong streak。
- `score < 50` 记一次 wrong streak，清空 correct streak。
- `50 <= score < 80` 清空两个 streak，难度不变。
- 连续两次高分升一档，最高 hard。
- 连续两次低分降一档，最低 easy。
- `score < 0` 表示评估降级，不更新难度。
- `LastRoundID` 记录最近已消费的评分轮次，重复执行同一个 round 时直接跳过，避免 checkpoint replay 或重试导致 streak 被重复累计。

备选方案是按平均分连续滑动窗口计算。这个更平滑，但第一版不需要复杂窗口状态。

## Risks / Trade-offs

- [Risk] 状态已维护但暂时不影响选题 → Mitigation：这是第一阶段边界，下一 change 再接 retrieve/pick，避免一次改动过大。
- [Risk] 三档难度粗糙 → Mitigation：先保证可解释、可测试，后续再引入长期弱点和岗位职级。
- [Risk] 旧 Session 没有 DifficultyState → Mitigation：节点执行时如果字段为空，按 medium 初始化。
