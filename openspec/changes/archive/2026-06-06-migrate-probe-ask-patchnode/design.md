# Design: migrate probe_ask PatchNode

## 目标

把 `probe_ask` 的状态写入收口到 Graph runner，同时保持现有 suspend/resume、HTTP/SSE 和 Session JSON 行为不变。

## StatePatch 扩展

新增两个字段：

```go
AppendCurrentFollowUp *FollowUp
CurrentCriticProbeSignal *CriticProbeSignalPatch

type CriticProbeSignalPatch struct {
    HasProbeSignal bool
    ProbeTopic     string
}
```

应用规则：

- `AppendCurrentFollowUp` 追加到 `Session.CurrentRound().FollowUps`。
- `CurrentCriticProbeSignal` 只更新 `Session.CurrentRound().CriticResult.HasProbeSignal` 和 `ProbeTopic`。
- 当前轮不存在时返回错误。

## probe_ask patch 节点

成功路径：

1. 读取当前轮和 `CriticResult`。
2. 调 LLM 生成追问。
3. 返回 `AppendCurrentFollowUp` 和 `WorkingMemory.ProbesUsed+1` patch。
4. 返回 `graph.SuspendWithPatch(...)`，runner 先 apply patch 再暂停。

降级路径：

1. LLM 失败时不追加追问。
2. 返回关闭后的 `CurrentCriticProbeSignal` 和包含 `DegradedReasons["probe_ask"]` 的 `WorkingMemory` patch。
3. 返回 nil，让 router 走 `update_memory`。

跳过路径：

- 无 probe 信号或预算耗尽时返回空 patch。
- 如果原 Session 缺 `WorkingMemory`，兼容旧行为返回默认 `WorkingMemory` patch。

## 非目标

- 不迁移 `probe_eval`。
- 不新增 HTTP/SSE 字段。
- 不修改 `PatchNodeFunc` 签名。
