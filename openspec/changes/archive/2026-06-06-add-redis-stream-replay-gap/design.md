## Context

`RedisInterviewEventHub.Publish` 使用：

```text
XADD stream MAXLEN ~ <MaxLen> * event <json>
```

Redis 会近似裁剪旧事件。`Subscribe` 在 `afterID` 非空时直接：

```text
XREAD BLOCK ... STREAMS stream afterID
```

如果 `afterID` 已经被裁剪，Redis 仍可能返回当前保留的后续事件，但客户端不知道中间缺了事件。

## Decision 1: Compare afterID with the first retained stream ID

新增 helper 查询当前最小 retained ID：

```text
XRANGE stream - + COUNT 1
```

判断规则：

- `afterID == ""`：保持当前行为，从最新 ID 后开始读，不报 gap。
- stream 为空且 `afterID != ""`：报 gap，并从 `0-0` 等待后续事件。
- `afterID < firstRetainedID`：报 gap，并从 `firstRetainedID` 开始继续读后续事件。
- `afterID >= firstRetainedID`：不报 gap，继续从 `afterID` 读。

## Decision 2: Represent gap as a snapshot event

保持内存事件总线语义：

```go
InterviewEvent{
    ID:        afterID,
    Type:      interviewEventSnapshot,
    SessionID: sessionID,
    ReplayGap: true,
}
```

HTTP stream handler 本来会先写当前 Session snapshot。Redis gap 事件会作为后续 snapshot-like 事件出现，提示客户端“事件回放不完整，请以 snapshot 为准”。

## Decision 3: Keep Redis protocol code local

项目当前没有引入 Redis client，`redis_event_hub.go` 使用轻量 RESP helpers。为保持依赖边界，本阶段继续复用现有 `redisDo` / `redisStreamEvents` 解析函数，只新增最小 ID 提取和 stream ID 比较。

## Risks

- Redis Stream ID 解析必须严格处理非法 ID；非法 `afterID` 不应 panic，应保守报 gap。
- gap 后起点不能继续用过旧 `afterID` 阻塞，应使用当前最小 retained ID 或 `0-0`。
- `MAXLEN ~` 是近似裁剪，检测只能基于 Redis 当前实际最小 ID，不能基于配置的 `MaxLen` 推断。
