## Why

`retrieve_rag`、`evaluate`、`report` 已迁移到 runner-level `PatchNode`，但 `pick_next` 仍不能迁移。原因是 `pick_next` 的成功路径需要先写 `PendingDecision`、追加 `AnswerRound`、更新 `WorkingMemory`，再返回 `ErrSuspended` 等用户作答。当前 runner 在 patch 节点返回 error 时不会 apply patch，直接迁移会丢失暂停前状态。

## What Changes

- Graph runner 新增显式 patch-on-suspend 语义：patch 节点返回 `ErrSuspended` 且错误被标记为“允许 apply suspend patch”时，runner 先 apply patch，再进入现有 suspend 流程。
- 普通 error 仍不 apply patch，避免错误节点污染 Session。
- `pick_next` 新增 patch-aware 构造函数并在 Interview Graph 中注册为 `PatchNode`。
- `probe_ask` 本阶段不迁移，因为 `FollowUps`、`CriticResult` 还没有纳入 `StatePatch`。
