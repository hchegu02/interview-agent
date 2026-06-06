# Tasks

- [x] 1. 迁移 `update_difficulty` 到 runner-level `PatchNode`，保留 wrapper 和现有行为。
- [x] 2. 迁移 `reflection_check` 到 runner-level `PatchNode`，保留 wrapper 和降级语义。
- [x] 3. 为 checkpoint 增加 patch summary 记录，不改变业务 replay 语义。
- [x] 4. 为累计型节点增加节点级 idempotency marker 和重复执行保护。
- [x] 5. 扩展 agent-verify 门禁，检查 patch 注册、写集、顺序和幂等风险。
- [x] 6. 更新 `docs/SDD-Backend.md`、OpenSpec 主 spec 和 `docs/code-changes`。
- [x] 7. 运行 `go test ./...`、OpenSpec strict 校验和 agent-verify。
