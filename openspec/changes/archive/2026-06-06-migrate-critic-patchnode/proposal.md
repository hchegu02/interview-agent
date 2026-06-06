## Why

`probe_eval` 已迁移到 runner-level `PatchNode`，追问 ask/eval 链路已经收口；但 `critic` 仍直接写当前轮 `CriticResult` 和降级记忆。这样 `evaluate -> critic -> refine/probe` 的核心分支点仍绕过 runner 统一 apply，后续 checkpoint、写集审计和并发边界不完整。

## What Changes

- 扩展 `StatePatch`，支持写入当前轮完整 `CriticResult`。
- 新增 `NewCriticPatchNode`，让 `critic` 通过 patch 写 critic 结果和降级 `WorkingMemory`。
- 旧 `NewCriticNode` 保留 wrapper，直接调用行为兼容。
- `refine/update_memory` 本阶段不迁移。
