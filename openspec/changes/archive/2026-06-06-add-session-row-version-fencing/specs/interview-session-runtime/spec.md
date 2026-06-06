## ADDED Requirements

### Requirement: PG Session Store 必须使用 row_version 防止旧快照覆盖新状态

PG Session Store MUST use an authoritative row_version compare-and-swap guard when updating existing Session rows.

#### Scenario: 新 Session 初始化版本

- **WHEN** 保存一个数据库中不存在的 Session
- **THEN** PG Session Store MUST insert it successfully
- **AND** the persisted row_version MUST be initialized
- **AND** the saved Session SHOULD receive the persisted row_version

#### Scenario: 匹配版本可以更新

- **WHEN** 保存的 Session row_version matches the persisted row_version
- **THEN** PG Session Store MUST update the row
- **AND** the persisted row_version MUST increase monotonically
- **AND** the saved Session SHOULD receive the new row_version

#### Scenario: 旧版本被拒绝

- **WHEN** 保存的 Session row_version does not match the persisted row_version
- **THEN** PG Session Store MUST reject the write
- **AND** 返回 stale session write 错误

#### Scenario: 旧 JSON 快照兼容读取

- **WHEN** 读取的 state_json does not contain row_version
- **THEN** PG Session Store MUST use the sessions.row_version column as the authoritative version

#### Scenario: UpdatedAt 不再作为主并发控制

- **WHEN** PG Session Store updates an existing row
- **THEN** updated_at MAY be used for ordering and display
- **AND** row_version MUST be used for write conflict detection
