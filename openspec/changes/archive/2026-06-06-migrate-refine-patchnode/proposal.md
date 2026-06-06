## Why

`critic` 已迁移到 runner-level `PatchNode`，但其后续分支 `refine` 仍直接写当前轮 `RefinedEval` 和降级记忆。这样 `critic -> refine -> route` 这段仍有一处绕过 runner apply，状态写入边界不一致。

## What Changes

- 扩展 `StatePatch`，支持写入当前轮 `RefinedEval`。
- 新增 `NewRefinePatchNode`，让 `refine` 通过 patch 写修正评估和降级 `WorkingMemory`。
- 旧 `NewRefineNode` 保留 wrapper，直接调用行为兼容。
- `update_memory` 本阶段不迁移。
