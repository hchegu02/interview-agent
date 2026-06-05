## 1. Domain And Graph

- [ ] 1.1 在 `internal/domain/session.go` 新增 `Suspension` 结构和 `Session.Suspension` 可选字段。
- [ ] 1.2 在 `internal/graph` 中集中处理暂停写入：保留 `CurrentNode`，同时写入默认 `Suspension`。
- [ ] 1.3 调整 `Runnable.Resume`：优先从 `Suspension.Node` 恢复，缺失时回退 `CurrentNode`。
- [ ] 1.4 恢复成功推进后清理过期 `Suspension`。

## 2. HTTP And Frontend Contract

- [ ] 2.1 在 `internal/httpapi` 响应中深拷贝并返回可选 `suspension`。
- [ ] 2.2 在 `web/src/types.ts` 增加 `Suspension` 类型和 `Session.suspension` 字段。

## 3. Tests And Verification

- [ ] 3.1 增加 Graph 暂停时写入 `Suspension` 的测试。
- [ ] 3.2 增加旧 Session 只有 `CurrentNode` 仍可 Resume 的兼容测试。
- [ ] 3.3 增加 HTTP 响应包含 suspension 深拷贝测试。
- [ ] 3.4 运行 `go test ./internal/graph ./internal/httpapi -count=1`。
- [ ] 3.5 运行 `npm --prefix web run test`。
- [ ] 3.6 更新 `docs/SDD-Backend.md` 中 Session / Graph 后续计划的实现状态。
