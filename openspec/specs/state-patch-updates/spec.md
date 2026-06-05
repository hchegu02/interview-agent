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

#### Scenario: 写入当前轮评估

- **WHEN** 评分节点产出当前回答的 Evaluation
- **THEN** StatePatch 可以写入当前未完成轮次的 `Evaluation`
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

