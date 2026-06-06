# Design: complete Graph state closure

## Approach

采用混合方案：

- 运行时继续使用字段式 `domain.StatePatch`，保持类型安全和简单 apply 规则。
- checkpoint 层记录 `PatchSummary`，用于观测和验证，不参与业务 replay。
- 幂等层使用节点级 idempotency key，优先保护累计型节点。
- Store 层继续利用已有 row version / fencing 语义，不在本 change 中扩 schema。

## Stage 1: complete PatchNode coverage

迁移剩余 legacy 节点：

- `update_difficulty`：返回 `WorkingMemory` patch，不直接修改 `WorkingMemory.Difficulty`。
- `reflection_check`：返回 `PendingDecision + WorkingMemory` patch，不直接修改 `PendingDecision`、`ReflectTopic`、`ReflectionsUsed` 或 `DegradedReasons`。

保留旧 `NewUpdateDifficultyNode` 和 `NewReflectionCheckNode` wrapper，避免破坏现有单测和局部子图。

## Stage 2: checkpoint patch summary

新增观测结构，挂在 checkpoint 上：

```go
type PatchSummary struct {
    Node        string
    Writes      []string
    RoundID     string
    Fields      []string
    Degraded    []string
    Suspended   bool
    Idempotency string
}
```

runner 在 patch apply 成功后生成摘要并交给 checkpoint recorder。摘要只描述写入字段和关键状态，不保存完整 patch 对象。

## Stage 3: node idempotency

对累计型节点引入 idempotency key：

```text
session_id + ":" + round_id + ":" + node_name
```

优先保护：

- `update_memory`
- `update_difficulty`
- `reflection_check`

重复执行时返回空 patch 或保持已有结果，不重复累加 `AvgScore`、`DegradedRounds`、`Difficulty` streak、`ReflectionsUsed`。

## Stage 4: verification gate

扩展 `cmd/agent-verify` 或其内部 verifier：

- 检查核心 Interview Graph 节点是否注册为 `PatchNode`。
- 检查关键节点写集是否声明。
- 检查 checkpoint 是否包含 patch summary。
- 检查累计型节点是否具备 idempotency marker。
- 检查 `update_memory -> update_difficulty -> reflection_check` 的顺序。

## Compatibility

- `Session` JSON 可新增 `omitempty` 运行时幂等 marker；旧 Session 缺字段时必须兼容。
- HTTP/SSE payload 不变化。
- 数据库 schema 不变化。
- `NodeFunc` 不删除，legacy wrapper 保留。

## Risks

- 幂等 marker 如果放入 `WorkingMemory`，会增加运行时状态体积；需要只记录当前面试必要 marker。
- checkpoint patch summary 不能泄露过多 prompt 或答案内容，只记录字段和摘要。
- `reflection_check` 同时涉及 LLM、规则兜底和路由决策，迁移时必须保持降级不中断语义。
