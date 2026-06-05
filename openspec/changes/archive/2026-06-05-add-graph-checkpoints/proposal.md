## Why

当前 Graph 执行只保存最终 `Session`。排障时能看到最终状态，但很难回答三个关键问题：

- 哪个节点开始前 Session 是什么样？
- 哪个节点结束后改了哪些关键字段？
- suspend、resume、error 发生在第几步、哪个 frontier？

已经落地的 `Suspension` 和 `StatePatch` 解决了暂停语义和局部写入收口，但还缺少可回放的执行证据。下一步需要轻量 Graph checkpoint，用于排障、回归测试和后续 agent-verify 细粒度验证。

## What Changes

- 在 `internal/graph` 增加轻量 checkpoint 数据结构和 recorder 接口。
- Runner 在执行过程中记录 batch / node / suspend / resume / error 事件。
- 第一版提供内存 ring buffer recorder，只用于 debug、测试和后续装配。
- 保持 `graph.NodeFunc(ctx, *Session) error` 不变。
- 不把 checkpoint 写入 Session JSON，不改变 HTTP API、SSE、数据库 schema。
- 并发 frontier 第一版只记录 batch 级 checkpoint，不伪造稳定的节点级快照。

## Capabilities

### New Capabilities

- `graph-checkpoints`: 系统可以记录 Graph 执行过程中的轻量 checkpoint，用于排障和回归验证。

### Modified Capabilities

<!-- No existing main spec capability is modified yet. -->

## Impact

- `internal/graph`: 新增 checkpoint 类型、recorder、内存 ring buffer 和 runner 插桩。
- `internal/graphs`: 透传可选 checkpoint recorder。
- 测试：覆盖线性图、suspend/resume、并发 frontier batch 级记录。
- 文档：更新 `docs/SDD-Backend.md` 中 13.1.3 的实现状态和边界。
- 不新增外部依赖。
- 不改数据库 schema。
