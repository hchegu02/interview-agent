# state-patch-updates Specification

## Purpose
TBD - created by archiving change add-state-patch-updates. Update Purpose after archive.
## Requirements
### Requirement: 通过 StatePatch 收敛关键 Session 写入

系统 MUST 支持用结构化 StatePatch 表达关键 Graph 节点对 Session 的写入，并通过统一入口应用这些写入。

#### Scenario: 应用候选题和检索 trace 更新

- **WHEN** 节点产生新的候选题列表和检索 trace
- **THEN** StatePatch 可以替换 `Session.CandidatePool`
- **AND** StatePatch 可以替换 `Session.RetrievalTrace`

#### Scenario: 追加新面试轮次

- **WHEN** 选题节点决定提出新问题
- **THEN** StatePatch 可以追加一个 `AnswerRound`
- **AND** 不应覆盖已有历史轮次

#### Scenario: 追加当前轮追问

- **WHEN** 追问节点决定提出新的 follow-up question
- **THEN** StatePatch 可以向当前未完成轮次追加一个 `FollowUp`
- **AND** 不应覆盖已有追问历史
- **AND** 如果不存在当前轮次，应用 patch 必须返回错误

#### Scenario: 写入当前最后追问评估

- **WHEN** 追答评分节点产出最后一个 FollowUp 的 Evaluation
- **THEN** StatePatch 可以写入当前未完成轮次最后一个 `FollowUp` 的 `Evaluation`
- **AND** 如果不存在当前轮次或当前轮没有追问，应用 patch 必须返回错误

#### Scenario: 更新当前轮 Critic 追问信号

- **WHEN** 追问节点需要关闭或更新追问信号
- **THEN** StatePatch 可以更新当前未完成轮次 `CriticResult` 的 `HasProbeSignal` 和 `ProbeTopic`
- **AND** 不应覆盖 `GroundedScore`、`NeedRefine`、`Issues` 或 `Summary`
- **AND** 如果不存在当前轮次，应用 patch 必须返回错误

#### Scenario: 写入当前轮完整 CriticResult

- **WHEN** critic 节点产出当前回答的完整 CriticResult
- **THEN** StatePatch 可以写入当前未完成轮次的 `CriticResult`
- **AND** 不应覆盖 `Evaluation`、`FollowUps`、`RefinedEvaluation` 或 `CompletedAt`
- **AND** 如果不存在当前轮次，应用 patch 必须返回错误

#### Scenario: 写入当前轮评估

- **WHEN** 评分节点产出当前回答的 Evaluation
- **THEN** StatePatch 可以写入当前未完成轮次的 `Evaluation`
- **AND** 如果不存在当前轮次，应用 patch 必须返回错误

#### Scenario: 写入当前轮修正评估

- **WHEN** refine 节点产出当前回答的修正 Evaluation
- **THEN** StatePatch 可以写入当前未完成轮次的 `RefinedEval`
- **AND** 不应覆盖原始 `Evaluation`、`CriticResult`、`FollowUps` 或 `CompletedAt`
- **AND** 如果不存在当前轮次，应用 patch 必须返回错误

#### Scenario: 清理待执行决策

- **WHEN** 当前决策已经被评分节点消费
- **THEN** StatePatch 可以清理 `Session.PendingDecision`

#### Scenario: 写入终评报告

- **WHEN** 报告节点生成最终报告
- **THEN** StatePatch 可以替换 `Session.Report`

#### Scenario: 保持外部接口兼容

- **WHEN** 系统引入 StatePatch
- **THEN** Graph `NodeFunc` 签名不应改变
- **AND** HTTP API 响应结构不应因为 StatePatch 改变
