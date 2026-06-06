## Why

Interview Agent 已具备面试主流程、长期记忆、Agent 工具 trace、真实 GitHub 只读 client 边界和验证门禁，但当前默认装配仍偏开发和演示。内部团队试用前，需要把身份来源、真实工具开关、长期记忆观测、smoke 验证和能力限制收成明确边界，避免把 mock/demo 行为误当成可对外生产能力。

本 change 的目标是支持内部 3-10 人可控试用，不是外部生产发布。

## What Changes

- 定义内部试用身份模式：后端必须通过统一 resolver/authorizer 读取可信内部身份来源；开发态 `X-User-ID` / `owner_user_id` 只作为 dev fallback。
- 定义真实 GitHub 工具试用开关：默认仍使用 deterministic mock；内部试用环境必须显式配置真实只读 GitHub client，缺配置时返回稳定状态并可在 trace 中诊断。
- 定义 Agent 项目润色的真实/模拟工具口径：`project_polish` 不得把工具失败伪装成真实成功，也不得向前端暴露 token、HTTP body 或底层工具输入。
- 定义长期记忆写入试用观测：内部 smoke 必须能看到成功、跳过、失败或冲突重试耗尽的稳定观测结果。
- 增加内部试用 smoke 门禁：覆盖面试开始、答题、报告、长期记忆沉淀、Agent 项目润色、tool trace 和 memory observation。
- 同步 SDD/试用文档限制：明确内部试用不是完整 JWT/OIDC、租户、完整 MCP gateway/server/client runtime 或运行时 sub-agent。
- 不引入 **BREAKING** API 变更；新增响应字段必须保持可选和向后兼容。

## Capabilities

### New Capabilities

- None

### Modified Capabilities

- `identity-access-boundary`: 增加内部试用身份来源和 dev fallback 的行为边界。
- `agentkit-mcp-tools`: 增加内部试用真实工具显式启用、默认 mock 和可诊断配置缺失要求。
- `project-polish-tools`: 更新项目润色工具能力口径，覆盖真实 GitHub 只读工具试用与失败降级。
- `long-term-memory`: 增加内部试用 smoke 对长期记忆观测信号的要求。
- `quality-gates`: 增加内部试用 smoke 验证门禁要求。
- `system-design-documentation`: 增加内部试用边界和非生产能力声明要求。

## Impact

- 后端影响范围：`cmd/server` 装配、`internal/httpapi` 身份解析和长期记忆入口、`internal/agentkit` 工具注册和真实 GitHub client 装配、`internal/agent` / `internal/skills` 工具 trace 传递、`cmd/agent-verify` 或新增 smoke CLI。
- 前端影响范围：只读展示已有 `tool_trace` 和限制状态；不得新增直接调用外部 GitHub/MCP 的前端逻辑。
- 验证影响范围：Go 测试、前端测试/构建、`cmd/agent-verify` fixtures、内部试用 smoke 命令。
- 文档影响范围：`docs/SDD-Backend.md`、`docs/SDD-Frontend.md` 和必要的内部试用说明。
