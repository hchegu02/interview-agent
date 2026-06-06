## Why

上一阶段 `PGSessionStore` 已经用 `updated_at` 拒绝明显旧的 Session 快照。但时间戳不是强并发控制：

- 应用时间可能不单调。
- 两个 writer 可能基于同一快照推进出不同状态。
- Redis lease 只能挡住入口，不能阻止旧 owner 在 lease 过期后继续写回 PG。

本阶段先引入 PG `row_version` 乐观锁，让 Session 写入变成 compare-and-swap。这样旧快照不能覆盖新状态，且不需要改 Graph 节点语义。

## What Changes

- `sessions` 表新增 `row_version bigint NOT NULL DEFAULT 1`。
- `domain.Session` 增加 `RowVersion`，作为 store 层 CAS 的版本载体。
- `PGSessionStore.Get/ListByUser` 读取表列版本并回填到 `Session.RowVersion`。
- `PGSessionStore.Save` 在 conflict update 时要求 `sessions.row_version = incoming RowVersion`，成功后版本递增并回填。
- stale row version 继续复用 `ErrStaleSessionWrite` 和 HTTP `409 + code=stale_session_write`。
- 更新 SDD 和 OpenSpec 主规格，说明 `updated_at` 只负责展示/排序，PG 并发正确性由 `row_version` 承担。

## Capabilities

### Modified Capabilities

- `interview-session-runtime`: PG Session Store 必须使用 row_version 防止旧快照覆盖新状态。

## Impact

- `migrations/010_session_row_version.*.sql`: 新增 / 回滚 row_version 列。
- `migrations/README.md`: 登记 migration。
- `internal/domain/session.go`: 增加 `RowVersion` 字段。
- `internal/httpapi/pg_session_store.go`: 改为 `RETURNING row_version` 的 CAS upsert。
- `internal/httpapi/pg_session_store_test.go`: 覆盖版本回填、版本冲突和 SQL 条件。
- `internal/migrations/migrations_test.go`: 覆盖 migration 健康检查。
- `docs/SDD-Backend.md`: 更新阶段边界。

## Non-Goals

- 不实现 Redis fencing token。
- 不修改 `SessionCoordinator` lease 接口。
- 不实现 lease heartbeat。
- 不改变 HTTP 响应结构，不要求前端提交 row_version。
- 不删除 `updated_at`，它仍用于展示、排序和兼容旧数据。
- 不改变 Graph 节点签名。
