# Design: migrate refine PatchNode

## 目标

让 `refine` 的状态写入由 Graph runner 统一 apply，避免节点内部直接修改当前轮 `RefinedEval`。

## StatePatch 扩展

新增字段：

```go
CurrentRefinedEvaluation *Evaluation
```

应用规则：

- 取 `Session.CurrentRound()`。
- 把修正评估写入当前轮 `RefinedEval`。
- 当前轮不存在时返回错误。
- 不覆盖原始 `Evaluation`、`CriticResult`、`FollowUps` 或 `CompletedAt`。

## refine patch 节点

### 不需要 refine

- 当 `CriticResult.NeedRefine == false` 时返回空 patch。
- 不调用 LLM。

### LLM 成功

- 返回 `CurrentRefinedEvaluation` patch。
- 如果 `WorkingMemory` 为空，返回初始化后的 `WorkingMemory` patch，保持旧 wrapper 应用后的兼容语义。

### LLM 失败

- 不向上返回普通错误，保持会话继续。
- 不写 `CurrentRefinedEvaluation`，让 `FinalEvaluation()` 回退到原始 `Evaluation`。
- 返回包含 `DegradedReasons["refine"]` 的 `WorkingMemory` patch。

## 非目标

- 不迁移 `update_memory`。
- 不修改 HTTP/SSE payload。
- 不修改 `PatchNodeFunc` 签名。
- 不修改数据库 schema。
