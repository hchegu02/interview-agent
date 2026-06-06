## Why

当前系统已经根据每轮评分更新 `WorkingMemory`，但下一题难度仍主要由固定 `RetrieveRAGOptions.TargetDifficulty` 和岗位匹配策略决定。需要先补一个确定性的动态难度状态机，让后续出题可以基于连续高低分调整难度，而不是让 LLM 单独决定。

## What Changes

- 在运行时记忆中新增动态难度状态。
- 新增 `update_difficulty` 节点，根据当前轮最终评分和上一轮状态更新难度、连对/连错计数。
- 将 Interview Graph 流程从 `update_memory -> reflection_check` 改为 `update_memory -> update_difficulty -> reflection_check`。
- 新增单元测试覆盖升难、降难、维持难度、降级评分跳过。
- 第一版不读取长期记忆，不改 RAG 检索目标难度，不改 HTTP 响应结构。

## Capabilities

### New Capabilities

- `dynamic-difficulty`: 支持在面试过程中用规则状态机维护下一题难度倾向。

### Modified Capabilities

无。

## Impact

- 影响代码：`internal/domain`、`internal/nodes`、`internal/graphs`。
- 影响文档：更新 `docs/SDD-Backend.md` 动态难度阶段状态。
- 不改变现有 API、数据库 schema、Session 顶层字段和前端类型。
- 无新增第三方依赖。
