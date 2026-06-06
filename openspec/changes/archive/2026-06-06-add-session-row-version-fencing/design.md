## Context

当前 PG 写入使用 `updated_at` guard：

```sql
WHERE COALESCE((sessions.state_json->>'updated_at')::timestamptz, sessions.updated_at) <= EXCLUDED.updated_at
```

这个 guard 能挡住明显旧时间的快照，但它不是 CAS。真正的并发正确性应该由数据库中的单调版本列保证。

## Decision 1: Use PG row_version as the authoritative version

新增：

```sql
ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS row_version bigint NOT NULL DEFAULT 1;
```

权威版本在表列 `sessions.row_version`。`domain.Session.RowVersion` 只是保存路径携带 expected version 的载体，不能覆盖数据库版本。

### Insert

新 Session 插入时 `row_version = 1`，保存成功后回填到 `Session.RowVersion`。

### Update

已有 Session 更新时使用：

```sql
WHERE sessions.row_version = $expected_row_version
RETURNING row_version
```

成功后：

```sql
row_version = sessions.row_version + 1
```

如果 `RETURNING` 没有行，说明版本不匹配，返回 `ErrStaleSessionWrite`。

## Decision 2: Keep updated_at for sorting and compatibility

`updated_at` 继续写入：

- HTTP 展示。
- 会话列表倒序。
- 老数据兼容。

但它不再承担 PG Session 并发正确性的主职责。

## Decision 3: Do not implement Redis fencing token in this change

fencing token 要改 `SessionCoordinator` 的 `AcquireLease/RenewLease` 返回值，并把 token 带入 PG 写入条件。这个改动会穿透 Redis lease、service 和测试。

本阶段先做 `row_version`，解决旧快照覆盖问题。fencing token 留到后续独立设计。

## Compatibility

- 旧数据库执行 migration 后已有行 `row_version = 1`。
- 旧 `state_json` 没有 `row_version` 也可以读取，因为 `Get/ListByUser` 从表列回填。
- 新建内存 demo store 不强制模拟 PG CAS；并发正确性以 PG store 为准。
- HTTP response 不新增 `row_version` 字段，避免前端误以为需要参与条件更新。

## Risks

- `Start/Answer` 存在二次 `Save`。因此 `PGSessionStore.Save` 必须在每次成功后回填新版本，否则第二次保存会被误判 stale。
- 如果从旧 Redis snapshot 恢复且数据库中已有同 ID 更新行，snapshot 没有 `RowVersion` 会被 PG CAS 拒绝。这是正确行为：不能让未知版本快照覆盖已持久化状态。
- `domain.Session` 加入 `RowVersion` 会把存储并发元数据带入领域结构。当前为了保持 `SessionStore.Save(ctx, sess)` 接口兼容，这是最小代价方案。
