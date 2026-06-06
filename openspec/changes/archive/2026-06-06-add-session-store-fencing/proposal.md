## Why

上一阶段已经把 Redis lease 收窄到 mutation 执行期，并在 Graph 暂停后释放 lease。但仍有一个现实风险：如果某个旧实例在 lease 过期后才完成长耗时 Resume，它仍可能把旧 Session JSON 无条件写回 PG，覆盖另一个实例已经保存的新状态。

当前 `PGSessionStore.Save` 是无条件 upsert：

- 没有版本检查。
- 没有 fencing token。
- 没有 CAS。

本阶段先做最小可落地的 stale write guard：使用 `sessions.updated_at` 与 `Session.UpdatedAt` 防止明显旧快照覆盖新快照。这样不改数据库 schema，也不影响内存 demo store。

## What Changes

- 新增 stale write 错误，用于表示 Session 写入版本落后。
- `PGSessionStore.Save` 在 upsert conflict update 时增加 `WHERE sessions.updated_at <= incoming_updated_at`。
- 当 update 因 guard 未命中时，返回 stale write 错误。
- `InterviewService.Answer` / `Start` 维持现有保存流程，但遇到 stale write 时通过结构化错误返回。
- 补充 PG store SQL 单元级测试或可替代的最小测试，确保 SQL 和错误语义稳定。

## Capabilities

### Modified Capabilities

- `interview-session-runtime`: Session store 在 PG 路径下必须防止旧 Session 快照覆盖新状态。

## Impact

- `internal/httpapi/session_store.go`: 增加 stale write 错误。
- `internal/httpapi/pg_session_store.go`: 增加条件 upsert 和 rows affected 检查。
- `internal/httpapi/interview_errors.go`: 增加 `stale_session_write` 错误码。
- `internal/httpapi/pg_session_store_test.go` 或相关测试：覆盖 stale write guard。
- `docs/SDD-Backend.md`: 更新后续边界，说明本阶段是 updated_at guard，不是完整 fencing token。

## Non-Goals

- 不新增数据库列。
- 不新增 migration。
- 不实现 lease token / fencing token。
- 不修改 Redis lease 协议。
- 不改变 `MemorySessionStore` demo 语义。
- 不改变 HTTP 响应已有字段。
