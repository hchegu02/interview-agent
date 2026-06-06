## 1. OpenSpec

- [x] 1.1 创建 `add-session-row-version-fencing` proposal/design/tasks。
- [x] 1.2 增加 `interview-session-runtime` delta spec，覆盖 row_version CAS。
- [x] 1.3 运行 `openspec validate add-session-row-version-fencing --strict`。

## 2. Migration

- [x] 2.1 新增 `010_session_row_version.up.sql`。
- [x] 2.2 新增 `010_session_row_version.down.sql`。
- [x] 2.3 更新 `migrations/README.md` 和 migration 健康测试。

## 3. Domain And Store

- [x] 3.1 在 `domain.Session` 增加 `RowVersion`。
- [x] 3.2 `PGSessionStore.Get/ListByUser` 读取 `row_version` 并回填。
- [x] 3.3 `PGSessionStore.Save` 使用 row_version CAS upsert。
- [x] 3.4 保存成功后把最新 row_version 回填到同一个 Session。
- [x] 3.5 row_version 冲突复用 `ErrStaleSessionWrite`。

## 4. Tests

- [x] 4.1 增加 PG store row_version SQL / 参数测试。
- [x] 4.2 增加保存成功回填版本测试。
- [x] 4.3 增加旧版本写入失败测试。
- [x] 4.4 保留 zero `UpdatedAt` 补齐和 exec/query 错误测试。

## 5. Docs And Verification

- [x] 5.1 更新 `docs/SDD-Backend.md`。
- [x] 5.2 创建 `docs/code-changes/MM-DD-session-row-version-fencing.md`。
- [x] 5.3 运行 `go test ./internal/httpapi -count=1`。
- [x] 5.4 运行 `go test ./... -count=1`。
- [x] 5.5 运行 `openspec validate add-session-row-version-fencing --strict`。
- [x] 5.6 归档后运行 `openspec validate interview-session-runtime --strict`。
