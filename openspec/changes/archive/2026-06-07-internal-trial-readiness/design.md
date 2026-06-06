## Context

当前系统已经有一批生产边界基础：可替换 owner resolver/authorizer、长期记忆结构化观测、真实 GitHub 只读 client、Agent `tool_trace` 和 agent-verify fixture。缺口不在核心抽象，而在试用装配和验收口径：默认服务仍偏 mock，本地开发 header 容易被误读为鉴权，真实工具启用路径没有内部试用门禁，完整业务 smoke 尚未把面试、长期记忆和 Agent 工具串成一条可重复验证链路。

## Goals / Non-Goals

Goals:

- 支持内部团队可控试用完整业务链路。
- 保持默认本地/CI 离线可运行，不默认联网。
- 让真实工具、身份来源和长期记忆写入状态可诊断。
- 用 smoke 和现有验证命令定义“可试用”的最低门槛。

Non-goals:

- 不实现完整 JWT/OIDC、公开登录、租户、计费或生产级权限体系。
- 不实现完整 MCP gateway/server/client 生命周期。
- 不引入运行时 sub-agent。
- 不把长期记忆写入失败变成面试完成阻断条件。

## High-Level Approach

采用“内部试用配置包”而不是直接生产化：

1. 身份保持统一 resolver/authorizer 边界。内部试用环境只允许从可信内部来源解析用户，例如上游网关注入 header 或明确 allowlist；开发 fallback 保留但必须被配置和文档标记为 dev-only。
2. 工具注册保持显式装配。默认服务仍注册 deterministic mock；内部试用配置显式注入真实 `GitHubProjectClient`，缺 `HTTPClient`、`BaseURL` 或必要配置时返回稳定 `config_missing`，并通过 `tool_trace` 暴露状态摘要。
3. `project_polish` 继续通过 `ToolRegistry.Call` 调工具。Skill 只消费结构化 `ToolResult.Summary` 级别信息；失败时可以降级生成通用建议，但 trace 必须保留失败类别，不能让用户误以为真实仓库分析成功。
4. 长期记忆继续在面试完成后异步容错式沉淀。内部 smoke 必须能验证成功/跳过/失败/冲突耗尽观测信号，失败不阻断 completed session。
5. 试用门禁复用现有验证命令，再补一条业务 smoke，把关键 HTTP/CLI 路径串起来。

## Data Flow

```text
内部试用请求
  -> 可信身份来源 / dev fallback
  -> OwnerResolver
  -> Authorizer
  -> InterviewService start/answer
  -> Session completed + Report
  -> LongTermMemory observation
  -> Agent message project_polish
  -> ToolRegistry
  -> mock 或显式 GitHubProjectClient
  -> AgentResponse.tool_trace
  -> 前端只读展示
```

## Error Handling

- 身份缺失或不匹配：受保护用户资源返回结构化错误。
- 真实 GitHub 工具未配置：返回 `config_missing`，不默认联网。
- GitHub 调用失败或超时：记录 failed tool trace，Skill 不伪装成真实成功。
- 长期记忆写入失败：记录稳定错误类别，面试完成响应保持不变。
- smoke 失败：内部试用不得标记为通过。

## Testing Strategy

- 保留现有全量验证：`go test ./...`、`npm --prefix web run test`、`npm --prefix web run build`、`go run ./cmd/agent-verify ... -tool-events ... -memory-observations ...`、`openspec validate --all --strict`。
- 新增或扩展内部 smoke，覆盖完整试用链路和关键 trace 字段。
- 对配置缺失、mock 默认、真实工具显式启用、身份 fallback 限制分别增加最小测试或 fixture。

## Risks

- 最大风险是内部试用被误解为外部生产。缓解方式是配置名、SDD、trace 和 smoke 输出都明确 `internal trial` / `dev fallback` / `mock` / `config_missing`。
- 真实 GitHub API 可能引入网络不稳定。默认离线 mock，真实 client 只在显式配置下启用。
- 旧 `project-polish-tools` spec 仍有 mock-only 口径；本 change 必须同步更新，避免规格和实现继续漂移。
