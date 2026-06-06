## ADDED Requirements

### Requirement: 动态难度应影响 RAG 目标难度

系统 MUST 在 RAG 检索候选题时读取当前 Session 的动态难度状态，并将其转换为题库检索基础目标难度。系统 MUST 在该基础目标难度上继续应用 `GapStrategy` 微调，并将最终检索目标难度限制在 1-5。

#### Scenario: easy 动态难度降低 RAG 目标难度

- **WHEN** `WorkingMemory.Difficulty.Current` 为 `easy`
- **THEN** RAG 检索基础目标难度 MUST 为 2

#### Scenario: medium 动态难度保持默认 RAG 目标难度

- **WHEN** `WorkingMemory.Difficulty.Current` 为 `medium`
- **THEN** RAG 检索基础目标难度 MUST 为 3

#### Scenario: hard 动态难度提高 RAG 目标难度

- **WHEN** `WorkingMemory.Difficulty.Current` 为 `hard`
- **THEN** RAG 检索基础目标难度 MUST 为 4

#### Scenario: 缺少动态难度状态时兼容旧行为

- **WHEN** Session 缺少 `WorkingMemory.Difficulty`
- **THEN** 系统 MUST 使用 `RetrieveRAGOptions.TargetDifficulty` 作为基础目标难度

#### Scenario: 非法动态难度状态时兼容旧行为

- **WHEN** `WorkingMemory.Difficulty.Current` 不属于 `easy`、`medium` 或 `hard`
- **THEN** 系统 MUST 使用 `RetrieveRAGOptions.TargetDifficulty` 作为基础目标难度

#### Scenario: GapStrategy 在动态基础目标上继续生效

- **WHEN** 动态基础目标难度已确定
- **THEN** 系统 MUST 按 `GapStrategy` 对该基础目标难度继续微调
- **AND** 最终检索目标难度 MUST 限制在 1-5

#### Scenario: 用户显式难度过滤保持硬约束

- **WHEN** 用户设置题库难度最小值或最大值过滤
- **THEN** 系统 MUST 将这些过滤条件原样传给 retriever
- **AND** 动态难度 MUST NOT 覆盖这些硬过滤条件
