## 1. Graph Checkpoint Model

- [x] 1.1 在 `internal/graph` 新增 `GraphCheckpoint`、`CheckpointPhase`、`CheckpointRecorder`。
- [x] 1.2 新增内存 ring buffer recorder，并保证返回快照为拷贝。
- [x] 1.3 增加 recorder 单元测试，覆盖容量截断、顺序、拷贝语义。

## 2. Runner Instrumentation

- [x] 2.1 在 `Graph` / `Runnable` 中支持可选 checkpoint recorder，不改变 `NodeFunc`。
- [x] 2.2 在 `Runnable.run` 记录 frontier before / after / error / suspended。
- [x] 2.3 在线性 frontier 的 `executeNode` 前后记录 node before / after / error。
- [x] 2.4 在 `Resume` 计算 next frontier 后记录 resume_from。
- [x] 2.5 并发 frontier 只记录 batch 级 checkpoint，不记录节点级 checkpoint。

## 3. Tests And Verification

- [x] 3.1 增加线性图 checkpoint 顺序测试。
- [x] 3.2 增加 suspend / resume checkpoint 测试。
- [x] 3.3 增加并发 frontier 只记录 batch 级 checkpoint 测试。
- [x] 3.4 运行 `go test ./internal/graph -count=1`。
- [x] 3.5 运行 `go test ./... -count=1`。
- [x] 3.6 更新 `docs/SDD-Backend.md` 中 13.1.3 的实现状态和边界。
- [x] 3.7 运行 `openspec validate add-graph-checkpoints --strict`。
