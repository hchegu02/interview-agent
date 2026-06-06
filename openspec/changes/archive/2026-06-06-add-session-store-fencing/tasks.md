## 1. OpenSpec

- [x] 1.1 创建 `add-session-store-fencing` proposal/design/tasks。
- [x] 1.2 增加 delta spec，覆盖 PG stale write guard。
- [x] 1.3 运行 `openspec validate add-session-store-fencing --strict`。

## 2. PG Session Store Guard

- [x] 2.1 在 `session_store.go` 增加 `ErrStaleSessionWrite`。
- [x] 2.2 调整 `PGSessionStore.Save`：zero `UpdatedAt` 自动补当前时间。
- [x] 2.3 调整 PG upsert：更新分支只允许已持久化 Session `updated_at <= incoming UpdatedAt`。
- [x] 2.4 `RowsAffected == 0` 时返回 `ErrStaleSessionWrite`。

## 3. HTTP Error Mapping

- [x] 3.1 `writeInterviewError` 增加 `stale_session_write` code。
- [x] 3.2 stale write 返回 HTTP 409，保留 `error` 和 `trace_id`。

## 4. Tests

- [x] 4.1 增加 PG store stale write guard 测试。
- [x] 4.2 增加 zero `UpdatedAt` 补齐测试。
- [x] 4.3 增加 HTTP stale write 错误映射测试。

## 5. Docs And Verification

- [x] 5.1 更新 `docs/SDD-Backend.md`。
- [x] 5.2 创建 `docs/code-changes/MM-DD-session-store-fencing.md`。
- [x] 5.3 运行 `go test ./internal/httpapi -count=1`。
- [x] 5.4 运行 `go test ./... -count=1`。
- [x] 5.5 运行 `openspec validate add-session-store-fencing --strict`。
