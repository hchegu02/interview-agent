## Why

Interview Agent 已经具备长期记忆、Agent Router、mock MCP Tool、SSE trace 和本地验证门禁，但这些能力仍停留在开发边界：画像读取依赖开发模式 owner header，长期记忆写入失败不可见，工具调用只有 mock foundation，前端也缺少结构化 tool trace 展示。下一阶段需要把这些边界收紧到可替换、可观测、可验证的生产接入形态。

本 change 的目标不是一次性实现完整用户中心、完整 MCP runtime 或 runtime sub-agent，而是把真实身份来源、低风险真实工具接入、工具 trace、长期记忆可观测性和验证门禁设计成稳定增量，避免继续在 mock 和自然语言结果上堆功能。

## What Changes

- 定义真实身份和访问边界：保留现有 `UserMemoryOwnerResolver` / `UserMemoryAuthorizer` 可注入点，设计从开发 header 迁移到真实身份来源的后端边界。
- 增强长期记忆写入可观测性：长期记忆写入失败、跳过、CAS 冲突重试和最终结果必须产生结构化可观测信号，但仍不阻断面试完成响应。
- 推进真实 MCP / Tool MVP：在现有 `internal/agentkit` ToolRegistry 边界内接入一个低风险真实工具，保留 schema、权限、超时、hook、错误码和 mock fixture。
- 增加结构化 Agent / Tool trace：后端返回工具调用 trace，前端只读展示工具状态，不从自然语言结果里反推工具执行。
- 扩展验证门禁：`cmd/agent-verify` 和本地验证 fixture 覆盖真实 tool event、长期记忆观测信号和结构化 trace。
- 不引入 **BREAKING** API 变更；新增 JSON 字段必须使用兼容的 `omitempty` / 可选字段。

## Capabilities

### New Capabilities

- `identity-access-boundary`: 定义真实身份来源、owner 解析和授权边界，覆盖从开发 header 到生产身份注入的替换路径。

### Modified Capabilities

- `long-term-memory`: 增加长期记忆写入可观测性要求，包括跳过、失败、CAS 冲突重试和成功写入的结构化信号。
- `agentkit-mcp-tools`: 将当前 mock MCP Tool foundation 扩展为可接入低风险真实工具的 MVP，同时继续复用 ToolRegistry 权限、超时和 hook 边界。
- `agent-router-skills`: 为 Agent 消息入口增加结构化 tool trace 响应要求，并保持现有 intent / skill / result 响应兼容。
- `quality-gates`: 增加真实 tool event fixture、tool trace 和长期记忆可观测性相关验证门禁。

## Impact

- 后端影响范围：`internal/httpapi` 身份解析和长期记忆写入观测、`internal/memory` 写入结果表达、`internal/agentkit` ToolRegistry / MCP adapter、`internal/agent` 和 `internal/skills` 的工具调用返回结构。
- 前端影响范围：`web/src/apiClient.ts`、`web/src/types.ts`、`web/src/candidatePages.tsx` 只读展示结构化 tool trace；不得直接访问 MCP 服务或自行判断工具状态。
- 验证影响范围：`cmd/agent-verify`、`testdata/agent_verify`、现有 Go 测试、前端测试和构建。
- 文档影响范围：`docs/SDD-Backend.md`、`docs/SDD-Frontend.md` 需要同步真实身份、真实工具 MVP、trace 和验证边界；不得把 Codex sub-agent 或完整 MCP Gateway 写成项目运行时能力。
