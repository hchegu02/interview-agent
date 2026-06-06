## Why

`refine` 已迁移到 runner-level `PatchNode`，但 `update_memory` 仍直接修改 `WorkingMemory` 和当前轮 `CompletedAt`。这两个字段共同决定后续 `update_difficulty`、`reflection_check` 和下一轮 `pick_next` 的状态视图，应该由 runner 统一 apply，避免后续节点看到半更新状态。

## What Changes

- 新增 `NewUpdateMemoryPatchNode`，复用已有 `StatePatch.WorkingMemory` 和 `StatePatch.CompleteCurrentRound`。
- 旧 `NewUpdateMemoryNode` 保留 wrapper，直接调用行为兼容。
- `update_memory` 在 Interview Graph 中注册为 `PatchNode`。
- 本阶段不迁移 `update_difficulty` 或 `reflection_check`。
