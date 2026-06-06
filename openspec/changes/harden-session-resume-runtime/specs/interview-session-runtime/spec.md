## ADDED Requirements

### Requirement: HTTP 恢复链路必须优先使用结构化 Suspension

系统 MUST 在用户提交答案时优先根据 `Session.Suspension` 判断当前等待输入位置，并继续兼容旧的 `CurrentNode` 恢复方式。

#### Scenario: 根据 Suspension 写入主问题答案

- **WHEN** Session 的 `suspension.awaiting` 为 `answer`
- **AND** `suspension.node` 为 `pick_next`
- **THEN** 系统 MUST 把用户答案写入当前主问题 round

#### Scenario: 根据 Suspension 写入追问答案

- **WHEN** Session 的 `suspension.awaiting` 为 `answer`
- **AND** `suspension.node` 为 `probe_ask`
- **THEN** 系统 MUST 把用户答案写入当前 round 的最后一个追问

#### Scenario: 旧 Session 回退 CurrentNode

- **WHEN** Session 没有 `suspension`
- **AND** Session 有旧字段 `current_node`
- **THEN** 系统 MUST 按 `current_node` 保持原有答案写入行为

#### Scenario: 非答案暂停拒绝普通答案

- **WHEN** Session 的 `suspension.awaiting` 不是 `answer`
- **THEN** 普通答案提交 MUST 返回 invalid state 错误

### Requirement: Redis lease 必须只覆盖会话 mutation 执行期

系统 MUST 使用 lease 防止同一 Session 被多个实例同时修改，但不应在 Graph 已暂停等待用户输入时继续占用 lease。

#### Scenario: Start 暂停后释放 lease

- **WHEN** Start 执行 Graph 并暂停等待用户回答
- **AND** Session 已保存成功
- **THEN** 系统 MUST 释放该 Session 的 mutation lease

#### Scenario: Answer 再次暂停后释放 lease

- **WHEN** Answer 恢复 Graph 后再次暂停等待用户输入
- **AND** Session 已保存成功
- **THEN** 系统 MUST 释放该 Session 的 mutation lease

#### Scenario: Session 完成后释放 lease

- **WHEN** Answer 推进 Session 到 completed
- **THEN** 系统 MUST 释放该 Session 的 mutation lease

### Requirement: SSE 事件必须携带可恢复运行时状态

系统 MUST 在 SSE 事件中提供恢复和排障所需的兼容字段。

#### Scenario: SSE 事件包含暂停状态

- **WHEN** Session 当前处于暂停等待外部输入状态
- **THEN** SSE 事件 SHOULD 包含 `suspension`
- **AND** `suspension` MUST 与 REST 响应使用兼容 JSON 结构

#### Scenario: SSE 事件包含 trace id

- **WHEN** 请求上下文包含 trace id
- **THEN** 该请求触发的 SSE 事件 SHOULD 包含相同 `trace_id`

#### Scenario: SSE 回放存在缺口

- **WHEN** 客户端请求的 `Last-Event-ID` 已不在服务端可回放窗口内
- **THEN** 系统 SHOULD 通过 `replay_gap` 提示客户端以 snapshot 为准

### Requirement: HTTP 错误响应必须包含稳定 code 和 trace

系统 MUST 在不破坏现有 `error` 字段的前提下，为面试 API 错误响应提供稳定错误码和 trace id。

#### Scenario: Session 不存在

- **WHEN** 请求引用不存在或无权访问的 Session
- **THEN** 响应 SHOULD 包含 `code=session_not_found`

#### Scenario: 当前状态不允许提交答案

- **WHEN** Session 当前不等待普通答案
- **THEN** 响应 SHOULD 包含 `code=invalid_state`

#### Scenario: Lease 冲突

- **WHEN** Session 正被其他实例执行 mutation
- **THEN** 响应 SHOULD 包含 `code=lease_conflict`

#### Scenario: Trace id 贯穿错误响应

- **WHEN** 请求携带或生成了 trace id
- **THEN** 错误响应 SHOULD 包含相同 `trace_id`
