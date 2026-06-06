# Tasks

- [x] 1. 新增 `SuspendWithPatch` / `IsPatchSuspend` marker，并扩展 runner apply 逻辑。
- [x] 2. 补 Graph 层测试：patch suspend 会 apply；普通 error 不 apply。
- [x] 3. 新增 `NewPickNextPatchNode`，旧 `NewPickNextNode` 作为兼容 wrapper。
- [x] 4. 将 Interview Graph 中 `pick_next` 注册为 `PatchNode`。
- [x] 5. 补 `pick_next` patch/suspend 行为测试和 assembled graph 回归。
- [x] 6. 更新 SDD、OpenSpec、code-changes，并运行验证。
