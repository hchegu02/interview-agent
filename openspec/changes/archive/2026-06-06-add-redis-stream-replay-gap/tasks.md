## 1. OpenSpec

- [x] 1.1 创建 `add-redis-stream-replay-gap` proposal/design/tasks。
- [x] 1.2 增加 `interview-session-runtime` delta spec。
- [x] 1.3 运行 `openspec validate add-redis-stream-replay-gap --strict`。

## 2. Tests First

- [x] 2.1 增加 Redis Stream ID 比较测试。
- [x] 2.2 增加最小 retained ID 解析测试。
- [x] 2.3 增加 `Subscribe` 在 `afterID` 被裁剪时返回 `ReplayGap` 的测试。
- [x] 2.4 先运行目标测试并确认 RED。

## 3. Implementation

- [x] 3.1 实现 Redis Stream ID 解析和比较。
- [x] 3.2 实现当前最小 retained ID 查询。
- [x] 3.3 在 Redis `Subscribe` 中注入 gap snapshot 事件。
- [x] 3.4 保持空 `afterID` 行为不变。

## 4. Docs And Verification

- [x] 4.1 更新 `docs/SDD-Backend.md`。
- [x] 4.2 创建 `docs/code-changes/06-06-redis-stream-replay-gap.md`。
- [x] 4.3 运行 `go test ./internal/httpapi -count=1`。
- [x] 4.4 运行 `go test ./... -count=1`。
- [x] 4.5 归档并验证 `interview-session-runtime`。
