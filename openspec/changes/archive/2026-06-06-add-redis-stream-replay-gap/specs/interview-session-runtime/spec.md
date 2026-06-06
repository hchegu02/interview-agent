## ADDED Requirements

### Requirement: Redis Streams 事件回放必须报告裁剪缺口

Redis-backed interview event streams MUST report a replay gap when the requested last event id is no longer retained by the Redis Stream.

#### Scenario: 空 afterID 保持订阅最新事件行为

- **WHEN** a client subscribes without `afterID`
- **THEN** Redis event hub SHOULD start reading after the latest retained stream id
- **AND** it MUST NOT report a replay gap

#### Scenario: afterID 早于当前最小 retained ID

- **WHEN** a client subscribes with an `afterID` older than the first retained Redis Stream id
- **THEN** Redis event hub MUST emit an event with `replay_gap=true`
- **AND** subsequent reads SHOULD continue from a retained stream id

#### Scenario: afterID 仍在保留范围内

- **WHEN** a client subscribes with an `afterID` not older than the first retained Redis Stream id
- **THEN** Redis event hub MUST NOT report a replay gap solely because history exists before `afterID`

#### Scenario: stream 为空但客户端要求回放

- **WHEN** a client subscribes with a non-empty `afterID`
- **AND** the Redis Stream has no retained events
- **THEN** Redis event hub SHOULD report `replay_gap=true`
