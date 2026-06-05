## Why

长期记忆基础层已经能从 `Session.Report` 构建用户画像，但当前面试完成后不会自动写入长期记忆。需要在服务层接入这条沉淀链路，让报告生成后的结果能进入跨 Session 用户画像，为后续动态难度和复习建议提供数据基础。

## What Changes

- 在 `InterviewService` 增加可选 `memory.Store` 依赖。
- 面试 `Answer` 流程在 Session 完成并保存后，将报告沉淀到长期记忆 Store。
- 长期记忆写入失败不影响面试完成响应、Session 保存、SSE 完成事件和 lease 释放。
- `cmd/server` 默认注入内存长期记忆 Store。
- 第一版不新增 HTTP API、不接数据库、不修改 Graph 节点。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `long-term-memory`: 增加“面试完成后自动沉淀报告到长期记忆 Store”的行为要求。

## Impact

- 影响代码：`internal/httpapi` 的 `InterviewService` 装配和完成流程，`cmd/server` 服务装配测试。
- 影响文档：更新 `docs/SDD-Backend.md` 中 Long-term Memory 阶段状态。
- 不改变现有 HTTP 响应结构、Session JSON、数据库 schema 和前端类型。
- 无新增第三方依赖。
