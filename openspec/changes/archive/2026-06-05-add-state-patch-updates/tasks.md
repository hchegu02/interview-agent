## 1. Domain Patch Model

- [x] 1.1 新增 `StatePatch` 结构，字段只覆盖 CandidatePool、RetrievalTrace、PendingDecision、Rounds、Evaluation、CompletedAt、Report。
- [x] 1.2 新增 `ApplyStatePatch`，集中实现 replace、append、current round 写入和错误返回。
- [x] 1.3 增加领域层单元测试，覆盖成功路径和无 current round 错误路径。

## 2. Node Migration

- [x] 2.1 迁移 `retrieve_rag` 的 CandidatePool / RetrievalTrace 写入到 StatePatch。
- [x] 2.2 迁移 `pick_next` 的 PendingDecision / AppendRound 写入到 StatePatch，保留 WorkingMemory 计数逻辑。
- [x] 2.3 迁移 `evaluate` 的 CurrentEvaluation / ClearPendingDecision 写入到 StatePatch。
- [x] 2.4 迁移 `report` 的 Report 写入到 StatePatch。

## 3. Verification And Docs

- [x] 3.1 运行 `go test ./internal/domain ./internal/nodes -count=1`。
- [x] 3.2 运行 `go test ./... -count=1`。
- [x] 3.3 更新 `docs/SDD-Backend.md` 中 13.1.2 的实现状态和边界。
- [x] 3.4 归档前运行 `openspec validate add-state-patch-updates --strict`。
