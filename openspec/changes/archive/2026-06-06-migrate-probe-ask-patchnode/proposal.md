## Why

`pick_next` 已通过 patch-on-suspend 迁移到 runner-level `PatchNode`，但 `probe_ask` 仍在节点内部直接修改当前轮追问列表和工作记忆。这样主问题出题路径已经收口，追问出题路径还没有收口，状态写入边界不一致。

## What Changes

- 扩展 `domain.StatePatch`，支持追加当前轮 `FollowUp`，以及替换当前轮 `CriticResult`。
- 新增 `NewProbeAskPatchNode`，让 `probe_ask` 成功路径返回 `AppendCurrentFollowUp + WorkingMemory` patch，并通过 `SuspendWithPatch` 暂停。
- 让 `probe_ask` 降级路径通过 `CurrentCriticResult + WorkingMemory` patch 关闭追问信号。
- `probe_eval` 本阶段不迁移，避免一次性扩大到追答评分字段。
