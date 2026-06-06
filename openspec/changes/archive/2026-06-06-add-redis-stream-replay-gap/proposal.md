## Why

SSE 已经支持 `Last-Event-ID` / `last_event_id`，内存事件总线也能在历史事件被裁剪后返回 `replay_gap=true`。但 Redis Streams 模式目前只把客户端传入的 `afterID` 直接交给 `XREAD`，没有判断这个 ID 是否已经早于 Redis 当前保留的最小事件 ID。

结果是：Redis Stream 被 `MAXLEN ~` 裁剪后，客户端可能漏掉事件却不知道，只能继续读到后续事件。这和内存事件总线行为不一致，也让前端无法明确用 snapshot 兜底。

## What Changes

- Redis event hub 在 `Subscribe` 收到非空 `afterID` 时，读取当前 stream 最小 retained ID。
- 如果 stream 为空，或 `afterID` 小于最小 retained ID，订阅通道先返回 `ReplayGap=true` 的 snapshot 事件。
- gap 后从合理起点继续 `XREAD`，避免阻塞在已经被裁剪的旧 ID 上。
- 新增 Redis Stream ID 比较和最小 ID 解析测试。
- 更新 SDD，说明 Redis Streams 已具备最小 ID 裁剪检测。

## Capabilities

### Modified Capabilities

- `interview-session-runtime`: Redis Streams event hub must report replay gaps when the requested last event id is no longer retained.

## Impact

- `internal/httpapi/redis_event_hub.go`: 增加最小 stream ID 查询、ID 比较和 gap 事件注入。
- `internal/httpapi/redis_event_hub_test.go`: 增加单元测试。
- `docs/SDD-Backend.md`: 更新 Redis Streams replay gap 边界。
- `docs/code-changes/06-06-redis-stream-replay-gap.md`: 记录代码变更。

## Non-Goals

- 不修改 HTTP/SSE 响应结构。
- 不修改前端 EventSource 逻辑。
- 不实现 Redis lease heartbeat 或 fencing token。
- 不改变内存事件总线行为。
- 不引入第三方 Redis client。
