# Design: migrate probe_eval PatchNode

## 目标

让 `probe_eval` 的状态写入由 Graph runner 统一 apply，和 `evaluate`、`probe_ask` 的状态流保持一致。

## StatePatch 扩展

新增字段：

```go
CurrentFollowUpEvaluation *Evaluation
```

应用规则：

- 取 `Session.CurrentRound()`。
- 取当前轮最后一个 `FollowUp`。
- 把 `Evaluation` 写入该 FollowUp。
- 当前轮不存在或没有追问时返回错误。

## probe_eval patch 节点

### 空追答

- 返回 `CurrentFollowUpEvaluation(score=0)`。
- 如果当前轮存在 `CriticResult`，返回 `CurrentCriticProbeSignal(false, "")`。

### LLM 成功

- 返回 `CurrentFollowUpEvaluation(score=shape.Score)`。
- 如果当前轮存在 `CriticResult`，根据 `shape.HasMoreProbe`、`shape.NextProbeTopic` 和 `WorkingMemory.CanProbe()` 返回 `CurrentCriticProbeSignal`。

### LLM 失败

- 返回 `CurrentFollowUpEvaluation(score=-1)`。
- 如果当前轮存在 `CriticResult`，关闭追问信号。
- 返回包含 `DegradedReasons["probe_eval"]` 的 `WorkingMemory` patch。

## 非目标

- 不迁移 `critic`、`refine`、`update_memory`。
- 不修改 HTTP/SSE payload。
- 不修改 `PatchNodeFunc` 签名。
