## MODIFIED Requirements

### Requirement: 通过 StatePatch 收敛关键 Session 写入

系统 MUST 支持用结构化 StatePatch 表达关键 Graph 节点对 Session 的写入，并通过统一入口应用这些写入。

#### Scenario: 写入当前面试内策略状态

- **WHEN** `update_difficulty` or `reflection_check` changes current-session strategy state
- **THEN** StatePatch SHOULD express the change through `WorkingMemory`
- **AND** the node SHOULD NOT directly mutate `Session.WorkingMemory`

#### Scenario: 写入反思后的下一步决策

- **WHEN** `reflection_check` decides to ask a new question, reflect, or end
- **THEN** StatePatch SHOULD express the decision through `PendingDecision`
- **AND** the node SHOULD NOT directly mutate `Session.PendingDecision`

#### Scenario: 携带幂等观测元数据

- **WHEN** a patch-aware cumulative node returns a StatePatch
- **THEN** StatePatch MAY include an idempotency key for checkpoint summary
- **AND** ApplyStatePatch MUST NOT treat that key as a business state write
