## Why

`probe_ask` 已迁移到 runner-level `PatchNode`，但 `probe_eval` 仍直接修改最后一个 `FollowUp.Evaluation`、`CriticResult` 追问信号和 `WorkingMemory.DegradedReasons`。这让追问链路的“提问”已收口，但“追答评分”仍绕过 runner 统一 apply。

## What Changes

- 扩展 `StatePatch`，支持写入当前轮最后一个 `FollowUp` 的 `Evaluation`。
- 新增 `NewProbeEvalPatchNode`，让 `probe_eval` 通过 patch 写追答评分、Critic 追问信号和降级记忆。
- 旧 `NewProbeEvalNode` 保留 wrapper，直接调用行为兼容。
- `critic/refine/update_memory` 本阶段不迁移。
