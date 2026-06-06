## Context

当前仓库已经有几条相关基础能力：

- `internal/httpapi/user_memory.go` 提供 `UserMemoryOwnerResolver` / `UserMemoryAuthorizer` 注入点，默认开发模式从 `X-User-ID` 或 `owner_user_id` 解析 owner。
- `internal/httpapi/interview_memory.go` 在面试完成后沉淀长期记忆，CAS 冲突会重试，但写入失败不阻断面试完成响应，当前可观测性不足。
- `internal/agentkit` 已有 `ToolRegistry`、权限、hook、mock MCP client 和默认 mock tool 注册。
- `/api/agent/message` 已返回结构化 intent / skill / result，但没有稳定 `tool_trace` 响应字段。
- `cmd/agent-verify` 已覆盖 RAG / Agent 输出和 mock tool event 的一部分门禁。

这次设计把上述能力推到生产接入边界：身份来源可替换、长期记忆写入可观测、真实工具 MVP 不绕过 ToolRegistry、trace 结构化返回、验证门禁能抓住回归。

## Goals / Non-Goals

**Goals:**

- 定义真实身份来源接入边界，避免业务 handler 直接耦合 JWT、cookie 或第三方认证细节。
- 给长期记忆写入增加结构化观测信号，覆盖成功、跳过、失败和 CAS 冲突重试耗尽。
- 在现有 ToolRegistry 内接入一个低风险真实工具 MVP，同时保留 deterministic mock。
- 为 `/api/agent/message` 增加兼容的 `tool_trace,omitempty` 响应字段，并让前端只读展示。
- 扩展 `cmd/agent-verify` / fixture / 前端测试，覆盖真实工具事件、tool trace 和长期记忆观测信号。

**Non-Goals:**

- 不实现完整 JWT 登录、用户中心、RBAC 或多租户权限模型。
- 不实现完整 MCP Gateway、daemon、Sandbox、外部工具市场或 runtime sub-agent。
- 不改变现有面试 Session JSON 的兼容性，不让前端复制后端状态机。
- 不让长期记忆写入失败阻断面试完成响应。

## Decisions

### 1. 身份边界放在 HTTP 层 resolver / authorizer，不下沉到业务模型

推荐方案：保留并推广 `UserMemoryOwnerResolver` / `UserMemoryAuthorizer` 模式，把真实身份来源接在 `cmd/server` 或 HTTP middleware 装配层。业务 handler 只接收“当前用户是谁”和“是否允许访问目标资源”的结果。

备选方案 A：在每个 handler 里直接解析 JWT。问题是认证细节会扩散，后续换身份提供方会改一堆业务代码。

备选方案 B：把 owner 直接写入 `domain.Session` 并让领域层判断权限。问题是当前要保护的是 HTTP 用户资源入口，强行把认证概念塞进领域模型会污染 Session 事实源。

### 2. 长期记忆观测使用小结构事件/日志，不保存正文

推荐方案：在长期记忆沉淀函数周围形成明确结果状态：`success`、`skipped`、`failed`、`conflict_retry_exhausted`。观测字段只包含 `user_id`、`session_id`、状态、错误类别、重试次数和耗时，不记录完整回答、完整报告或画像正文。

备选方案 A：直接把错误 `slog` 出去。问题是不可测试，也容易丢关键状态。

备选方案 B：把长期记忆写入失败变成 HTTP 错误。问题是破坏现有“面试完成优先”的用户流程，和现有 spec 冲突。

### 3. 真实工具 MVP 只能走 ToolRegistry

推荐方案：真实工具实现包装成现有 `Tool` / `MCPClient` 边界，所有调用仍通过 `ToolRegistry.Call`。这能复用 schema、权限、超时、before/after hook 和 mock fixture。第一版真实工具选低风险只读能力，例如 GitHub README / 仓库元数据分析。

备选方案 A：Skill 直接调 GitHub API。问题是绕过权限、超时和审计，等于废掉 AgentKit 边界。

备选方案 B：直接上完整 MCP Gateway。问题是范围过大，会引入生命周期、凭据、沙箱和网络安全问题，不符合 MVP。

### 4. Tool trace 是后端事实，前端只读展示

推荐方案：`/api/agent/message` 增加可选 `tool_trace` 字段。trace 项从 ToolRegistry hook 或 Skill 执行结果中收集，包含工具名、权限、状态、错误类别和耗时。前端只展示该字段；字段缺失时保持现有展示。

备选方案 A：前端从 `result.content` 文案推断工具状态。问题是脆弱且不可验证。

备选方案 B：把完整 raw tool input/output 全量返回前端。问题是暴露隐私、token 和外部响应细节，风险不值。

### 5. 验证门禁先覆盖边界，不追求端到端真实网络

推荐方案：新增 fixture 覆盖真实工具事件形状、tool trace 响应、长期记忆观测状态；真实网络调用用接口和 mock/fake 验证边界，避免 CI 依赖外部服务。

备选方案 A：CI 直接调用 GitHub 或真实 MCP 服务。问题是慢、不稳定，还会引入凭据管理。

备选方案 B：只跑 Go 单测不扩展 `agent-verify`。问题是无法覆盖跨模块输出契约，后续很容易破坏前端和验证链路。

## Risks / Trade-offs

- [Risk] 单个 change 覆盖身份、记忆、工具、trace、验证，范围偏大。 → Mitigation: 实现时按任务顺序小步提交，先身份和观测，再工具 MVP，再 trace 展示，最后门禁收口；不在同一任务里混改无关模块。
- [Risk] `tool_trace` 字段可能泄露工具输入或外部响应。 → Mitigation: trace 只暴露摘要字段和错误类别，不返回 raw payload、token、header 或完整正文。
- [Risk] 真实工具引入网络不稳定。 → Mitigation: 真实 client 必须有超时和错误分类；测试默认使用 fake/mock，不依赖外网。
- [Risk] 身份边界被误解为完整生产鉴权。 → Mitigation: SDD 明确本阶段只提供后端接入边界，不声明完整登录、RBAC 或用户中心。
- [Risk] 长期记忆观测和 HTTP 响应耦合过深。 → Mitigation: 观测信号由沉淀流程内部产生，不改变面试完成响应语义。

## Migration Plan

1. 保持现有开发模式 owner header 可用，新增真实身份 resolver 装配点和测试。
2. 为长期记忆沉淀路径加观测接口或可测试 recorder，先覆盖现有成功、跳过、失败和 CAS 冲突路径。
3. 在 `internal/agentkit` 增加真实工具 client 适配，默认配置仍使用 mock，真实工具需要显式配置启用。
4. 在 AgentService / Skill 执行结果中收集 tool trace，并通过 `/api/agent/message` 以可选字段返回。
5. 前端补 `tool_trace` 类型和只读展示，缺字段时保持当前页面。
6. 扩展 `cmd/agent-verify`、fixture、Go 测试、前端测试和 SDD。

Rollback 策略：真实工具保持配置开关；禁用真实工具后回退 mock。`tool_trace` 是可选字段，回滚后旧前端和旧响应都能兼容。长期记忆观测只影响日志/metrics/test recorder，不改变业务响应。

## Open Questions

- 第一版真实工具是否固定为 GitHub README / 仓库元数据分析，还是选择更小的本地 HTTP fetch 只读工具。
- 观测信号第一版落点采用 `slog` + 测试 recorder，还是同时暴露 Prometheus counter。
- 身份 resolver 的真实来源第一版是否只接上游 middleware 注入的 header/context，还是同步设计 JWT 验证配置。
