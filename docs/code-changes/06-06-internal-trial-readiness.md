# 06-06 internal trial readiness

## 1. 变更概述

本次变更把 Interview Agent 收敛到“内部试用可诊断”边界，不是外部生产发布。实现范围包括：

- 增加 `InternalTrialConfig`，把可信 owner header、开发 fallback、GitHub 工具模式和 GitHub API base URL 收到配置层。
- 长期记忆读取 API 支持可替换 owner resolver；内部试用使用可信 header，开发 fallback 只在显式允许时启用。
- Agent 默认工具仍是 deterministic mock/offline；只有内部试用启用且 `github_tool_mode=real` 时，服务装配才显式注册真实只读 GitHub client。
- GitHub 真实 client 缺配置时返回稳定 `config_missing`，并通过 `tool_trace.error_class` 诊断。
- `project_polish` 成功正文不再声称 mock；mock/real/failed/config_missing 状态以顶层 `tool_trace` 为准。
- 新增 `cmd/internal-trial-smoke`，默认离线、in-process，验证 session fixture、报告、检索 trace、graph、memory observations、Agent project polish 和 tool trace。

影响范围是内部试用配置、长期记忆读取授权边界、Agent 工具装配、项目润色文案、验证命令和文档。没有引入 breaking API 字段；新增响应仍依赖既有 `tool_trace,omitempty`。

## 2. 变更文件

- `internal/config/config.go`：新增内部试用配置、环境变量覆盖和 validate 规则。
- `internal/httpapi/user_memory.go`：新增可配置 owner resolver，并保持默认开发 resolver。
- `internal/httpapi/user_memory_test.go`：覆盖可信 header、开发 fallback、缺失 owner、owner mismatch。
- `cmd/server/main.go`：按配置装配 Agent service 和 UserMemory owner resolver。
- `cmd/server/main_test.go`：覆盖配置加载、owner resolver、Agent mock/real/config_missing 装配。
- `internal/agent/agent.go`：新增显式工具 registry 的默认 Agent service 构造入口。
- `internal/agent/service_test.go`：覆盖显式工具 registry 和 nil fallback 行为。
- `internal/agentkit/mcp.go`：新增真实 GitHub 项目工具注册、mock web fetch 单独注册和 GitHub client 配置错误边界。
- `internal/agentkit/mcp_test.go`：覆盖真实 GitHub 注册、nil registry、缺配置 `config_missing`。
- `internal/skills/skills.go`：修正 `project_polish` 成功文案，避免把成功路径固定称为 mock。
- `internal/skills/skills_test.go`：覆盖 mock、fake real、config_missing、failed trace 等项目润色路径。
- `cmd/internal-trial-smoke/main.go`：新增内部试用 smoke CLI。
- `cmd/internal-trial-smoke/main_test.go`：覆盖 smoke 成功输出和失败返回。
- `testdata/agent_verify/pass_memory_observations.json`：补充 success、skipped、failed、conflict_retry_exhausted 观测状态。
- `openspec/changes/internal-trial-readiness/tasks.md`：记录实现任务进度。
- `docs/SDD-Backend.md`、`docs/SDD-Frontend.md`、`docs/code-changes/06-06-internal-trial-readiness.md`：同步内部试用能力边界和本文档。

## 3. 函数级说明

### `internal/config/config.go`

- `InternalTrialConfig`：新增配置结构。输入来自 YAML 和环境变量；输出被 `cmd/server` 装配使用。字段包括 `Enabled`、`OwnerHeader`、`AllowDevFallback`、`GitHubToolMode`、`GitHubAPIBaseURL`。副作用为改变服务启动时的 owner resolver 和 Agent 工具装配。
- `Load(path string)`：新增 `INTERVIEW_INTERNAL_TRIAL_ENABLED`、`INTERVIEW_INTERNAL_TRIAL_OWNER_HEADER`、`INTERVIEW_INTERNAL_TRIAL_ALLOW_DEV_FALLBACK`、`INTERVIEW_INTERNAL_TRIAL_GITHUB_TOOL_MODE`、`INTERVIEW_INTERNAL_TRIAL_GITHUB_API_BASE_URL` 覆盖。布尔值解析失败返回配置错误，不启动服务。
- `defaults()`：新增内部试用默认值：disabled、`X-Internal-User`、不允许开发 fallback、GitHub tool mock、`https://api.github.com`。
- `(*Config).validate()`：新增 `github_tool_mode` 只能为 `mock|real`；内部试用启用时 owner header 不能为空。失败返回明确配置错误。

### `internal/httpapi/user_memory.go`

- `UserMemoryOwnerResolverOptions`：新增 resolver 选项，输入是可信 header 名和是否允许开发 fallback。
- `NewUserMemoryOwnerResolver(opts)`：返回 `UserMemoryOwnerResolver`。先读可信 header，成功时返回 authenticated owner；允许 fallback 时再调用 `defaultUserMemoryOwnerResolver`；否则返回 `trusted user memory owner is required`。不写存储，不改变请求。
- `SetUserMemoryOwnerResolver` / `SetUserMemoryAuthorizer`：既有注入点继续作为身份/授权边界；本次由 server wiring 使用。
- `getUserMemory`：行为边界保持不变，resolver 失败返回 401，owner mismatch 返回 403，store 缺用户返回 404，读取失败返回 500。

### `cmd/server/main.go`

- `main()`：服务创建后调用 `server.SetAgentService(buildAgentService(cfg))` 和 `server.SetUserMemoryOwnerResolver(buildUserMemoryOwnerResolver(cfg))`。副作用是启动时按配置选择 mock/real 工具和 owner resolver。
- `buildAgentService(cfg)`：默认或非内部试用返回 `agent.NewDefaultService()`，因此保持 mock/offline。内部试用且 `GitHubToolMode=="real"` 时创建 `ToolRegistry`，先注册 mock `web.fetch`，再注册真实只读 `github.project_analyze`。注册失败回退默认服务。真实 client 使用 `http.DefaultClient` 和配置的 `GitHubAPIBaseURL`。
- `buildUserMemoryOwnerResolver(cfg)`：内部试用时返回可信 header resolver，并传入 `AllowDevFallback`；其他模式返回允许开发 fallback 的 resolver。

### `internal/agent/agent.go`

- `NewDefaultService()`：保持原默认行为，创建 mock MCP tool registry 并注册默认 mock 工具。
- `NewDefaultServiceWithTools(tools)`：新增构造入口。`tools==nil` 时回到 `NewDefaultService()`；非 nil 时使用外部显式 registry 创建默认 router + skill registry。用于 server 注入真实 GitHub 工具，也用于测试。

### `internal/agentkit/mcp.go`

- `RegisterGitHubProjectTool(reg, client)`：新增真实 GitHub 项目工具注册 helper。输入是 registry 和 `GitHubProjectClient`；输出是注册错误。nil registry 返回 `ErrInvalidSpec` 包装错误；注册的工具权限为 `read_only`，超时 2 秒。
- `RegisterMockWebFetchTool(reg, client)`：新增单独注册 mock `web.fetch` 的 helper，避免 real GitHub 模式下丢掉默认 web mock。nil client 时使用 `NewMockMCPClient()`。
- `GitHubProjectClient.CallTool(ctx, call)`：只支持 `github.project_analyze`。解析 GitHub URL，构造 `/repos/{owner}/{repo}` 请求，读取公开 metadata，输出 summary、language、highlights、risk_points 等安全摘要。缺 HTTP client 或 BaseURL 通过 helper 返回 `config_missing`；网络、HTTP 状态、响应读取和 JSON 解析分别返回稳定错误类别。
- `mockGitHubProjectAnalyze` / `mockWebFetch`：继续提供 deterministic mock 输出，不访问公网。

### `internal/skills/skills.go`

- `ProjectPolishSkill` 相关逻辑：工具成功文案从“基于 mock GitHub 项目分析”改为“基于 GitHub 项目分析”。输入仍来自上下文或消息中的 GitHub URL；输出仍是 `SkillResult`。工具成功时 `ToolTrace.Status=success`；工具失败或结果非法时返回通用项目建议并保留 failed trace。错误分类通过 `toolErrorClass` 从 `MCPToolError.Code` 提取。
- `compactToolTraceSummary(value)`：继续压缩工具摘要，避免把长内容透给前端。

### `cmd/internal-trial-smoke/main.go`

- `main()`：解析 `-session` 和 `-real-github`。`-real-github` 当前只输出保留说明，不执行公网 GitHub smoke。
- `run(opts, stdout, stderr)`：核心 smoke。读取 session fixture，验证 report、retrieval trace、graph structure；调用 `verifyTrialMemoryObservations` 覆盖 memory 状态；用 `agent.NewDefaultService()` 执行 `project_polish`，要求 `github.project_analyze` tool trace success。失败写 stderr 并返回 1；成功写 interview/memory/project_polish/tool_trace 摘要并返回 0。
- `sessionFixturePath(opts)`：优先使用显式 `-session`，否则尝试仓库根路径和命令包测试路径。
- `loadSessionFixture(path)`：读取 JSON 并反序列化为 `domain.Session`；读取或解析失败返回错误。
- `appendFailures(dst, label, failures)`：把 verifier failures 加上标签后合并到失败列表。
- `verifyTrialMemoryObservations()`：构造 success、skipped、failed、conflict_retry_exhausted 四类观测，调用 `verify.MemoryPersistVerifier` 校验。

## 4. 调用链

### 服务启动到身份边界

`cmd/server.main`
-> `config.Load`
-> `(*Config).validate`
-> `httpapi.NewServer`
-> `buildUserMemoryOwnerResolver`
-> `httpapi.NewUserMemoryOwnerResolver`
-> `Server.SetUserMemoryOwnerResolver`
-> `GET /api/users/:user_id/memory`
-> `Server.getUserMemory`
-> `Server.resolveUserMemoryOwner`
-> `Server.authorizeUserMemory`
-> `InterviewService.GetUserMemory`

### 服务启动到 Agent 工具

`cmd/server.main`
-> `buildAgentService`
-> 默认：`agent.NewDefaultService`
-> mock registry：`agentkit.RegisterDefaultMCPTools`

内部试用 real GitHub：

`cmd/server.main`
-> `buildAgentService`
-> `agentkit.NewToolRegistry`
-> `agentkit.RegisterMockWebFetchTool`
-> `agentkit.RegisterGitHubProjectTool`
-> `agent.NewDefaultServiceWithTools`

### Agent 消息到 tool_trace

`POST /api/agent/message`
-> `AgentService.HandleMessage`
-> `RuleRouter.Route`
-> `skills.Registry.Run("project_polish")`
-> `ProjectPolishSkill`
-> `ToolRegistry.Call("github.project_analyze")`
-> mock `MockMCPClient` 或真实 `GitHubProjectClient`
-> `SkillResult.ToolTrace`
-> `AgentResponse.tool_trace`
-> 前端只读展示

### Smoke CLI

`go run ./cmd/internal-trial-smoke`
-> `main`
-> `run`
-> `sessionFixturePath`
-> `loadSessionFixture`
-> `ReportCompletenessVerifier.VerifyReport`
-> `RetrievalTraceVerifier.VerifyRetrieval`
-> `GraphStructureVerifier.VerifyInterviewGraph`
-> `verifyTrialMemoryObservations`
-> `agent.NewDefaultService().HandleMessage`
-> stdout/stderr + exit code

## 5. 数据流

- 配置数据：YAML/default/env 进入 `config.Config.InternalTrial`，经 validate 后只在装配层使用，不写入响应。
- 身份数据：HTTP request header/query 进入 owner resolver；内部试用优先可信 owner header；开发 fallback 只在允许时读取 `X-User-ID` 或 `owner_user_id`；handler 只接收解析后的 owner。
- 工具数据：用户消息或 context 中的 GitHub URL 进入 `ProjectPolishSkill`，再进入 `ToolRegistry.Call`。mock 返回 deterministic 摘要；真实 client 只读取公开 repository metadata。前端只收到 compact `tool_trace` 和业务建议，不收到 token、HTTP body、headers、完整工具输入或原始响应正文。
- 长期记忆数据：面试完成后的长期记忆写入仍是非阻塞；观测只记录 status、reason、error_class、attempts、elapsed_ms 等摘要。smoke 使用 fixture 覆盖状态，不写真实用户画像。

## 6. 依赖与副作用

- 新增/使用 `net/http` 默认 client 仅在内部试用 real GitHub 装配路径使用；默认 mock 不联网。
- `cmd/internal-trial-smoke` 读取本地 `testdata/agent_verify/pass_session.json`，不启动 HTTP server，不访问公网，不写文件。
- `GitHubProjectClient` live 路径会访问配置的 GitHub API base URL，但只有内部试用显式 real 模式才装配。
- 新增环境变量均为非敏感配置；没有新增 token、密钥或私有配置。
- 长期记忆写失败仍不阻断面试完成响应；这保持兼容，但需要依赖观测/验证发现问题。

## 7. 测试

实现提交中新增或修改了以下测试覆盖点：

- `cmd/server/main_test.go`：配置 env override、无效 GitHub tool mode、缺 owner header、owner resolver、Agent mock/default/real missing config。
- `internal/httpapi/user_memory_test.go`：可信 header、dev fallback、缺身份、owner mismatch。
- `internal/agentkit/mcp_test.go`：真实 GitHub 注册、nil registry、缺配置 `config_missing`。
- `internal/agent/service_test.go`：显式 registry 和 nil fallback。
- `internal/skills/skills_test.go`：项目润色 mock、fake real、config_missing、failed trace 语义。
- `cmd/internal-trial-smoke/main_test.go`：离线 smoke 成功和失败行为。

本次文档任务已运行：

```powershell
openspec validate internal-trial-readiness --strict
```

结果：通过，输出 `Change 'internal-trial-readiness' is valid`。沙箱内首次运行因 `C:\Users\hchegu` EPERM 失败，随后按权限规则提权运行通过。

本次文档任务不重新运行 `go test ./...`、`npm --prefix web run test`、`npm --prefix web run build`、`go run ./cmd/agent-verify ...` 或 `go run ./cmd/internal-trial-smoke`。`openspec/changes/internal-trial-readiness/tasks.md` 中 5.3 保持未勾选，表示全量 Go、前端、agent-verify、smoke 和 OpenSpec closeout 尚未完成。

## 8. 风险

- 兼容性：新增配置默认 disabled/mock，不改变默认本地和 CI 行为；新增 `tool_trace` 仍是 omitempty 兼容字段。
- 安全：内部试用 trusted header 只在可信上游边界后才有意义；不能把它当成 JWT/OIDC 或生产用户中心。
- 网络：真实 GitHub client 只在显式 real 模式装配；默认 smoke 和默认 Agent 不访问公网。
- 可观测性：长期记忆失败不阻断面试，必须依赖 memory observation、agent-verify 或 smoke 发现问题。
- 产品口径：内部试用不等于外部生产就绪；未实现租户、完整 MCP runtime、runtime sub-agent、生产认证和外部发布。
