## ADDED Requirements

### Requirement: PG Session Store 必须拒绝旧快照覆盖新状态

PG Session Store MUST prevent a stale Session snapshot from overwriting a newer persisted Session row.

#### Scenario: 新 Session 可以插入

- **WHEN** 保存一个数据库中不存在的 Session
- **THEN** PG Session Store MUST insert it successfully

#### Scenario: 新版本可以覆盖旧版本

- **WHEN** 保存的 Session `updated_at` 不早于数据库行 `updated_at`
- **THEN** PG Session Store MUST update the row

#### Scenario: 旧版本被拒绝

- **WHEN** 保存的 Session `updated_at` 早于数据库行 `updated_at`
- **THEN** PG Session Store MUST reject the write
- **AND** 返回 stale session write 错误

#### Scenario: 缺失 UpdatedAt 自动补齐

- **WHEN** 保存的 Session `updated_at` 为空
- **THEN** PG Session Store SHOULD fill it before persisting

### Requirement: Stale write 必须返回稳定错误码

HTTP interview APIs MUST expose stale Session write failures as structured conflicts.

#### Scenario: 旧 writer 写回失败

- **WHEN** Session 保存因 stale write guard 被拒绝
- **THEN** HTTP 响应 SHOULD 包含 `code=stale_session_write`
- **AND** HTTP 状态 SHOULD 为 409
