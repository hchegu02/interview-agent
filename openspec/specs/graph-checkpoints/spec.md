# graph-checkpoints Specification

## Purpose
TBD - created by archiving change add-graph-checkpoints. Update Purpose after archive.
## Requirements
### Requirement: 记录轻量 Graph checkpoint

系统 MUST 支持在 Graph 执行过程中记录轻量 checkpoint，用于排障和回归验证。

#### Scenario: 线性 frontier 记录节点前后快照

- **WHEN** Graph frontier 中只有一个节点
- **THEN** 系统应记录该节点执行前的 checkpoint
- **AND** 系统应记录该节点执行成功后的 checkpoint
- **AND** checkpoint 应包含 step、phase、frontier、node 和 snapshot

#### Scenario: 节点执行失败时记录错误 checkpoint

- **WHEN** 节点返回非 suspend 错误
- **THEN** 系统应记录错误 checkpoint
- **AND** checkpoint 应包含错误摘要

#### Scenario: 暂停时记录 suspended checkpoint

- **WHEN** 节点返回 `ErrSuspended`
- **THEN** 系统应在结构化 suspension 写入后记录 suspended checkpoint
- **AND** checkpoint snapshot 应能反映当前暂停信息

#### Scenario: Resume 记录恢复来源

- **WHEN** Graph 从暂停 Session 恢复
- **THEN** 系统应记录 resume_from checkpoint
- **AND** checkpoint 应包含恢复来源节点和下一轮 frontier

#### Scenario: 并发 frontier 只记录 batch 级 checkpoint

- **WHEN** Graph frontier 中包含多个并发节点
- **THEN** 系统应记录 frontier before / after / error checkpoint
- **AND** 系统不应为并发 frontier 伪造节点级 before / after snapshot

#### Scenario: checkpoint recorder 不改变外部接口

- **WHEN** 系统启用 Graph checkpoint
- **THEN** `NodeFunc` 签名不应改变
- **AND** HTTP API 响应结构不应改变
- **AND** Session JSON 不应因为 checkpoint 增加字段

