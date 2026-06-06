# dynamic-difficulty Specification

## Purpose
TBD - created by archiving change add-dynamic-difficulty-foundation. Update Purpose after archive.
## Requirements
### Requirement: 面试运行时应维护动态难度状态

系统 MUST 在单次面试运行时状态中维护当前难度、连续高分次数、连续低分次数和最近已消费的评分轮次。

#### Scenario: 初始化动态难度

- **WHEN** 面试 Session 没有动态难度状态
- **THEN** 系统 MUST 按 medium 难度初始化

### Requirement: 动态难度应根据当前轮评分更新

系统 MUST 在当前轮评分写入工作记忆后，根据当前轮最终评分更新动态难度状态。

#### Scenario: 连续高分升难度

- **WHEN** 用户连续两轮最终评分达到高分阈值
- **THEN** 系统 MUST 将难度上调一档
- **AND** 难度 MUST NOT 超过 hard

#### Scenario: 连续低分降难度

- **WHEN** 用户连续两轮最终评分低于低分阈值
- **THEN** 系统 MUST 将难度下调一档
- **AND** 难度 MUST NOT 低于 easy

#### Scenario: 中间分数维持难度

- **WHEN** 用户当前轮最终评分处于高低阈值之间
- **THEN** 系统 MUST 保持当前难度不变
- **AND** 系统 MUST 清空连续高分和连续低分计数

#### Scenario: 降级评分不影响难度

- **WHEN** 当前轮最终评分小于 0
- **THEN** 系统 MUST 保持动态难度状态不变

#### Scenario: 重放同一轮评分不重复更新难度

- **WHEN** 动态难度节点重复处理同一个已评分 round
- **THEN** 系统 MUST 跳过重复处理
- **AND** 连续高分和连续低分计数 MUST 保持不变

### Requirement: 动态难度更新应接入 Agent Graph

系统 MUST 在 `update_memory` 之后、`reflection_check` 之前执行动态难度更新。

#### Scenario: Graph 执行动态难度节点

- **WHEN** 当前轮评分完成并写入工作记忆
- **THEN** Graph MUST 执行 `update_difficulty` 节点
- **AND** 之后继续进入 `reflection_check`

