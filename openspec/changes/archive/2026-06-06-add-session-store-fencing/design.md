## Context

`InterviewService.Answer` 的流程是：

```text
Get Session -> 获取/续租 lease -> 写入答案 -> Resume Graph -> Save Session
```

如果旧实例在获取 lease 后卡住，lease TTL 过期后新实例可能接管并保存了更新状态。旧实例随后恢复执行并调用 `PGSessionStore.Save`，当前无条件 upsert 会覆盖新状态。

完整方案是 fencing token 或 row version，但这需要扩展 SessionStore 接口、Redis lease 返回 token、PG schema 增加版本列，改动面较大。本阶段先做无 schema 的 updated_at guard，解决最常见的旧快照覆盖。

## Goals / Non-Goals

**Goals:**

- PG 保存时拒绝 `Session.UpdatedAt` 落后于数据库行 `updated_at` 的更新。
- 保持 insert 新 Session 不受影响。
- 保持 MemorySessionStore 语义不变。
- stale write 通过稳定错误码返回，便于客户端重拉 Session。

**Non-Goals:**

- 不实现强 fencing token。
- 不新增 migration。
- 不改 Redis coordinator。
- 不让所有 store 都必须支持 CAS。

## Decision 1: Use updated_at guard without schema change

PG upsert 更新分支增加条件：

```sql
ON CONFLICT (id) DO UPDATE SET ...
WHERE sessions.updated_at <= EXCLUDED.updated_at
```

同时 insert/update 都显式写入 `updated_at`，让 DB 行的排序字段和 Session JSON 中的 `UpdatedAt` 对齐。

如果 `RowsAffected() == 0`，说明出现 conflict 但 guard 拒绝了更新，返回 `ErrStaleSessionWrite`。

## Decision 2: Zero UpdatedAt remains compatible

旧测试或手工调用可能传入 `UpdatedAt.IsZero()` 的 Session。PG store 在保存前应使用当前时间补齐 zero `UpdatedAt`，避免生成无法比较的零时间，也让列表排序稳定。

## Decision 3: HTTP exposes stale write as invalid state style conflict

stale write 不是普通 bad request，也不是用户输入错误。HTTP 层应返回稳定错误码：

```json
{
  "code": "stale_session_write",
  "error": "会话状态已被其他请求更新，请重新拉取后再试",
  "trace_id": "..."
}
```

HTTP status 建议使用 `409 Conflict`。

## Risks / Trade-offs

- `updated_at` guard 不是完整 fencing token：如果两个 writer 的时间异常接近或系统时钟不可靠，仍不如单调版本强。
- 本阶段不改 MemorySessionStore，所以本地 demo 不具备 stale write protection。
- 显式写 `updated_at` 会改变 PG 行时间来源，从 DB trigger 为主变成应用时间为主；但 `InterviewService` 已维护单调 `UpdatedAt`，这和 Session JSON 更一致。

## Validation

- `go test ./internal/httpapi -count=1`
- `go test ./... -count=1`
- `openspec validate add-session-store-fencing --strict`
