## MODIFIED Requirements

### Requirement: 通过 StatePatch 收敛关键 Session 写入

系统 MUST 支持用结构化 StatePatch 表达关键 Graph 节点对 Session 的写入，并通过统一入口应用这些写入。

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

#### Scenario: 保持外部接口兼容

- **WHEN** 系统引入 StatePatch
- **THEN** Graph `NodeFunc` 签名不应改变
- **AND** HTTP API 响应结构不应因为 StatePatch 改变
