## Context

`internal/memory` 已提供 `Store`、`BuildUpdateFromSession` 和 `ApplyUpdate`。当前 `InterviewService.Answer` 在 `runner.Resume` 后保存 Session，若 `sess.Status == completed` 会释放 lease 并发布完成事件。这个位置已经持有完整 `Session.Report`，并且属于 HTTP/service 层边界，比在 Graph report 节点中直接写外部 Store 更清楚。

## Goals / Non-Goals

**Goals:**

- `InterviewService` 支持注入长期记忆 Store。
- 当 `Answer` 推进到 completed 且 Session 已保存后，自动把报告沉淀到长期记忆。
- 沉淀失败不破坏面试完成主流程。
- 默认 server 装配内存长期记忆 Store。
- 添加单元测试覆盖成功沉淀、缺少报告不写入、Store 失败不影响完成流程。

**Non-Goals:**

- 不新增用户画像 HTTP API。
- 不实现数据库长期记忆 Store。
- 不改 Graph runner、report 节点和 Session JSON。
- 不让长期记忆影响本次题目选择。

## Decisions

### 1. 在 `InterviewService.Answer` 完成后沉淀

接入点放在 `runner.Resume` 成功、Session 保存成功且 `StatusCompleted` 后。这样只有真正完成并持久化的报告会进入长期记忆，避免 Graph 中间状态写入跨会话画像。

备选方案是在 `nodes.NewReportNode` 中直接写 Store。这个方案会让 Graph 节点依赖服务层存储，破坏当前“Graph 只改 Session，Service 负责持久化和事件”的边界。

### 2. 写入失败降级为非阻塞错误

长期记忆是个性化增强，不应让用户已经完成的面试因为画像写入失败而变成失败。第一版只吞掉错误，不改变 HTTP 响应。后续可加 observability hook 或事件。

备选方案是返回 500。这个会破坏现有面试完成流程，不选。

### 3. 服务层串行化合并

长期记忆合并是 `Get -> Apply -> Upsert`，单次 Store 调用加锁不足以防止同一进程内并发完成同一用户多场面试时丢更新。因此 `InterviewService` 在沉淀画像期间使用服务层 mutex 串行化合并。

备选方案是扩展 Store 接口增加原子 merge。这个更适合后续数据库 Store 设计；当前阶段先不扩大 Store 契约。

### 4. 默认使用内存 Store

`cmd/server` 默认注入 `memory.NewMemoryStore()`，满足本地开发和演示闭环。数据库 Store 后续单独设计 schema 和迁移。

## Risks / Trade-offs

- [Risk] 内存 Store 重启丢失 → Mitigation：明确只是第一版接入；后续数据库 Store 单独实现。
- [Risk] 写入失败被吞掉不易排查 → Mitigation：当前测试锁定“不影响主流程”；后续在 observability change 中加指标或日志。
- [Risk] Start 阶段如果 Graph 直接完成不会沉淀 → Mitigation：当前真实面试通常从 Start 暂停到答题；第一版只覆盖 Answer 完成路径，避免扩大范围。
