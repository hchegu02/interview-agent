## MODIFIED Requirements

### Requirement: 通过 StatePatch 收敛关键 Session 写入

系统 MUST 支持用结构化 StatePatch 表达关键 Graph 节点对 Session 的写入，并通过统一入口应用这些写入。

#### Scenario: 写入当前轮完整 CriticResult

- **WHEN** critic 节点产出当前回答的完整 CriticResult
- **THEN** StatePatch 可以写入当前未完成轮次的 `CriticResult`
- **AND** 不应覆盖 `Evaluation`、`FollowUps`、`RefinedEvaluation` 或 `CompletedAt`
- **AND** 如果不存在当前轮次，应用 patch 必须返回错误
