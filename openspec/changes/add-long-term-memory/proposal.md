## Why

当前 `WorkingMemory` 只服务单次面试 Session，面试结束后的强项、弱点、技能分数和复习建议没有稳定的长期画像承载。先补长期记忆基础层，可以让后续动态难度、复习计划和个性化 skill 有统一数据边界，而不是继续把跨会话信息塞进 `Session`。

## What Changes

- 新增长期记忆领域模型，表达用户强项、弱点、技能分数、最近建议和更新时间。
- 新增长期记忆 Store 接口和内存实现，作为后续数据库实现的兼容边界。
- 新增从 `domain.Session` / `domain.Report` 生成或合并 `UserMemory` 的规则函数。
- 第一版不自动接入 Interview Graph，不写数据库，不新增 HTTP API。
- 第一版不实现 LLM 总结长期记忆，全部使用可测试的本地规则。

## Capabilities

### New Capabilities

- `long-term-memory`: 支持把单次面试报告沉淀为跨会话用户画像，并通过 Store 接口读写长期记忆。

### Modified Capabilities

无。

## Impact

- 影响代码：新增 `internal/memory` 包及测试。
- 影响文档：更新 `docs/SDD-Backend.md` 中 Long-term Memory 阶段状态。
- 不改变现有 HTTP API、Graph 节点、Session JSON、数据库 schema 和前端类型。
- 无新增第三方依赖。
