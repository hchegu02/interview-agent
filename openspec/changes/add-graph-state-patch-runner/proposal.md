## Why

当前 Graph 已有 `StatePatch` 和轻量 checkpoint，但 runner 仍主要执行 `NodeFunc(ctx, *Session) error`。部分节点在内部调用 `ApplyStatePatch`，部分节点仍直接修改 `Session`。这会导致两个问题：

- 并发 frontier 只能靠约定避免写同一块 Session 字段，缺少结构化检查。
- checkpoint 只能记录完整 Session 快照，不能自然记录“本节点写了哪些字段”。

下一步需要把节点写入从“随手改 Session”收口到 `NodeSpec + StatePatch`。这不是迁移到 LangGraph，而是在 Go Graph 中吸收“节点返回 state update、runner 统一合并”的可取点。

## What Changes

- 在 `internal/graph` 引入 `NodeSpec` 和写集声明。
- 保留现有 `AddNode(name, fn)` 和 `NodeFunc`，作为 legacy 兼容入口。
- 新增 patch-aware 节点注册方式，节点可以返回 `domain.StatePatch`，由 runner 统一 apply。
- 并发 frontier 执行前检查写集冲突；冲突或未声明写集的 legacy 节点不得在并发 frontier 中执行。
- 在 Graph 层验证 patch-aware 节点模型；业务图先为关键节点声明写集，业务节点函数保持兼容迁移，不一次性改完整业务链路。
- 更新 SDD 和过程文档，明确当前能力边界。

## Capabilities

### New Capabilities

- `graph-state-patch-runner`: Graph runner 支持节点写集声明、patch-aware 节点执行和并发写冲突检测。

### Modified Capabilities

- `graph-checkpoints`: 后续 checkpoint 可以基于写集和 patch 记录更细粒度执行证据；本变更只打通 runner 模型，不实现 patch checkpoint 持久化。

## Impact

- `internal/graph`: 新增 `NodeSpec`、patch-aware 节点类型、写集冲突检测和 runner apply patch。
- `internal/domain`: 复用现有 `StatePatch` / `ApplyStatePatch`，必要时补充 patch 字段。
- `internal/nodes`: 保持节点函数兼容；后续再逐步把适合的节点改为 patch-aware。
- `internal/graphs`: 使用 `NodeSpec` 标注关键节点写集。
- 测试：覆盖写集冲突、legacy 并发拒绝、patch apply、兼容 `AddNode`。
- 不改 HTTP API、SSE、数据库 schema。
