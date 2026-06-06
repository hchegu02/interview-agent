# Design: migrate update_memory PatchNode

## 目标

让 `update_memory` 的状态写入由 Graph runner 统一 apply，并确保 `WorkingMemory` 更新和当前轮 `CompletedAt` 在同一个 patch 中生效。

## StatePatch 复用

本阶段不新增字段，复用：

```go
WorkingMemory *WorkingMemory
CompleteCurrentRound *time.Time
```

应用规则：

- `NewUpdateMemoryPatchNode` 读取当前轮 `FinalEvaluation()`。
- 在 `cloneWorkingMemory(sess.WorkingMemory)` 上做聚合更新。
- 返回 `WorkingMemory` patch。
- 同时返回 `CompleteCurrentRound` patch。

## update_memory patch 节点

### 正常评分

- 计算主答和追答加权分。
- 更新 `SkillCoverage`、`ConfirmedSkills`、`WeakSkills`、`ScoredRounds` 和 `AvgScore`。
- 返回完成当前轮的时间戳。

### 降级评分

- `FinalEvaluation.Score < 0` 时不更新覆盖度和均分。
- 只递增 `DegradedRounds`。
- 仍返回完成当前轮的时间戳，避免当前轮一直被视作未完成。

## 非目标

- 不迁移 `update_difficulty`、`reflection_check`。
- 不修改 HTTP/SSE payload。
- 不修改 `PatchNodeFunc` 签名。
- 不修改数据库 schema。
- 不解决重复执行 `update_memory` 的幂等问题；这是既有行为，后续可单独设计。
