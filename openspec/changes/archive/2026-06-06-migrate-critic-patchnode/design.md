# Design: migrate critic PatchNode

## 目标

让 `critic` 的状态写入由 Graph runner 统一 apply，和 `evaluate`、`probe_ask`、`probe_eval` 的状态流保持一致。

## StatePatch 扩展

新增字段：

```go
CurrentCriticResult *Critic
```

应用规则：

- 取 `Session.CurrentRound()`。
- 把完整 `Critic` 写入当前轮 `CriticResult`。
- 当前轮不存在时返回错误。
- 不覆盖 `Evaluation`、`FollowUps`、`RefinedEvaluation` 或 `CompletedAt`。

## critic patch 节点

### 评估已降级

- 当 `round.Evaluation.Score < 0` 时不调用 LLM。
- 返回放行 `CriticResult`，关闭 refine/probe 信号。
- 如果 `WorkingMemory` 为空，返回初始化后的 `WorkingMemory` patch，保持旧行为兼容。

### LLM 成功

- 返回完整 `CriticResult`。
- `NeedRefine` 由 LLM 输出或 `GroundedScore < RefineThreshold` 共同决定。
- `HasProbeSignal` 受 `WorkingMemory.CanProbe()` 预算约束。

### LLM 失败

- 不向上返回普通错误，保持会话继续。
- 返回降级 `CriticResult`。
- 返回包含 `DegradedReasons["critic"]` 的 `WorkingMemory` patch。

## 非目标

- 不迁移 `refine`、`update_memory`。
- 不修改 HTTP/SSE payload。
- 不修改 `PatchNodeFunc` 签名。
- 不修改数据库 schema。
