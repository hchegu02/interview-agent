## 1. Graph 框架

- [x] 1.1 新增 `NodeSpec`、写集 key 和 patch-aware 节点类型。
- [x] 1.2 保留 `AddNode` 兼容入口，新增 `AddNodeSpec` / patch helper。
- [x] 1.3 runner 执行 patch-aware 节点后统一 `ApplyStatePatch`。
- [x] 1.4 并发 frontier 前检查 legacy 节点和写集冲突。

## 2. 业务装配

- [x] 2.1 在 `internal/graphs` 给关键节点声明写集。
- [x] 2.2 在 Graph 层验证 patch-aware 节点模型，业务节点先保持兼容函数并声明写集。
- [x] 2.3 保持未迁移节点线性兼容。

## 3. 测试

- [x] 3.1 覆盖 `AddNode` 兼容。
- [x] 3.2 覆盖 patch-aware 节点 apply patch。
- [x] 3.3 覆盖并发 frontier 写集冲突拒绝。
- [x] 3.4 覆盖 legacy 节点无写集时不得参与并发 frontier。
- [x] 3.5 覆盖业务 Interview Graph 仍可运行。

## 4. 文档与验证

- [x] 4.1 更新 `docs/SDD-Backend.md`。
- [x] 4.2 更新 `docs/code-changes/06-06-graph-state-patch-runner.md`。
- [x] 4.3 运行 Go 测试和 OpenSpec strict 校验。
