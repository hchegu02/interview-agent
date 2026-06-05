## Context

当前 Graph runner 的节点签名是：

```go
type NodeFunc func(ctx context.Context, sess *domain.Session) error
```

节点直接读写 `Session`。例如：

- `retrieve_rag` 写 `CandidatePool` 和 `RetrievalTrace`。
- `pick_next` 写 `PendingDecision`、追加 `Rounds`、更新 `WorkingMemory.RoundsAsked`，并返回 `ErrSuspended`。
- `evaluate` 写当前 round 的 `Evaluation` 并清理 `PendingDecision`。
- `report` 写 `Report`。

这个模式当前可用，但写入规则分散。后续如果加入 checkpoint、并发写保护或更多 Agent Skill，直接写 Session 会让状态边界变差。

## Goals / Non-Goals

**Goals:**

- 新增小而明确的 `StatePatch`，只覆盖当前最需要收口的字段。
- 保持现有 `NodeFunc` 签名和 Graph 编排方式。
- 让关键节点的核心写入通过统一 apply 函数完成。
- 保证迁移后现有业务行为、HTTP 响应和测试语义不变。
- 为后续 checkpoint 记录 patch 做准备。

**Non-Goals:**

- 不把所有节点一次性改成返回 patch。
- 不引入 LangGraph 或外部状态机框架。
- 不实现完整 reducer DSL。
- 不改变 Session JSON、数据库 schema 或前端类型。
- 不在本阶段实现 GraphCheckpoint。

## Decisions

### Decision 1: Put StatePatch in domain layer

`StatePatch` 描述的是 `Session` 聚合根的更新意图，不应该放在 `internal/graph`。Graph 可以不知道业务字段，节点和领域层负责解释 patch。

建议新增：

```go
type StatePatch struct {
    CandidatePool  *[]Question
    RetrievalTrace *RetrievalTrace
    PendingDecision *Decision
    ClearPendingDecision bool
    AppendRound    *AnswerRound
    CurrentEvaluation *Evaluation
    CompleteCurrentRound *time.Time
    Report         *Report
}
```

字段保持少量、显式，不做 `map[string]any`。这样编译器能帮忙发现误用。

### Decision 2: Apply rules are explicit

新增 `ApplyStatePatch(sess *Session, patch StatePatch) error`。

第一阶段规则：

- `CandidatePool`：整体替换，复制 slice。
- `RetrievalTrace`：整体替换指针。
- `PendingDecision`：整体替换；`ClearPendingDecision` 为 true 时清空。
- `AppendRound`：追加到 `Rounds`。
- `CurrentEvaluation`：写入 `CurrentRound().Evaluation`，没有 current round 返回错误。
- `CompleteCurrentRound`：写入 `CurrentRound().CompletedAt`，没有 current round 返回错误。
- `Report`：整体替换。

错误必须返回，不能 panic。节点把错误包装成现有的 `graph.ErrPermanent` 或直接返回，保持链路可观测。

### Decision 3: Use patch helper inside nodes first

第一阶段不改变 `NodeFunc`，节点仍然可以在函数内部构造 patch 并调用 apply：

```go
patch := domain.StatePatch{
    CandidatePool: &pool,
    RetrievalTrace: trace,
}
if err := domain.ApplyStatePatch(sess, patch); err != nil {
    return fmt.Errorf("%w: apply state patch: %v", graph.ErrPermanent, err)
}
```

这能先收拢写入规则，同时避免把 Graph runner 和所有节点一起重写。

### Decision 4: Tests must prove behavior equivalence

新增领域层 patch 测试，覆盖 replace、append、current round 写入和错误分支。

节点迁移测试不追求重写所有断言，但必须保证：

- `retrieve_rag` 仍写入候选池和检索 trace。
- `pick_next` 仍追加 round、更新 pending decision、暂停。
- `evaluate` 仍写入当前 round evaluation 并清理 pending decision。
- `report` 仍生成同样报告。

## Risks / Trade-offs

- 迁移一半时仍存在直接写 Session：这是有意的渐进策略，避免一次大改。
- `StatePatch` 字段过多会变成另一个大杂烩：第一阶段只允许上面列出的字段。
- `RetrievalTrace`、`Report` 指针是否深拷贝：第一阶段保持当前语义，响应层已有 clone；patch apply 只负责状态写入。
- `CurrentEvaluation` 依赖当前 round：没有 current round 时必须返回错误，避免静默丢状态。
