## ADDED Requirements

### Requirement: 结构化记录 Graph 暂停信息

系统 MUST 在 Graph 暂停等待外部输入时，保存结构化暂停信息，并继续兼容现有 `current_node` 字段。

#### Scenario: 节点暂停等待用户回答

- **WHEN** Graph 节点返回 `ErrSuspended`
- **THEN** Session 包含 `suspension.node`
- **AND** Session 包含 `suspension.awaiting`
- **AND** Session 继续保留 `current_node`

#### Scenario: 旧 Session 没有 suspension

- **WHEN** 服务恢复一个只有 `current_node`、没有 `suspension` 的旧 Session
- **THEN** Graph Resume 仍然可以从 `current_node` 的下游继续执行

#### Scenario: 前端读取会话详情

- **WHEN** HTTP API 返回 Session
- **THEN** 响应可以包含可选 `suspension`
- **AND** 缺失 `suspension` 不影响老会话展示

#### Scenario: 恢复后清理暂停信息

- **WHEN** Graph 从暂停状态成功恢复执行
- **THEN** Session 中过期的 `suspension` 不应继续表示当前仍在等待外部输入
