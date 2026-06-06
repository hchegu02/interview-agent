# 技术试用 Runbook

本文档用于技术团队执行 Interview Agent 内部试用前的启动、验证、诊断和回滚。内部试用不是生产鉴权，不是外部发布；不要把可信 header、开发 fallback、mock 工具或 smoke 结果描述成 JWT、OIDC、生产用户中心或 SLA。

## 1. 前提

- 试用范围仅限内部技术验证，业务、HR 或面试官试用必须等技术试用通过后再放开。
- 默认工具路径必须使用 mock/offline；默认 mock/offline 不访问公网，也不访问真实 GitHub API。
- 真实 GitHub 工具只能在内部试用配置中显式启用，且当前只读公开仓库 metadata。
- 长期记忆访问必须有明确 owner 边界；内部试用应使用可信 `owner_header`，不能把开发 fallback 当成生产身份来源。
- 任何记录、日志摘录和问题单都不要记录 token、密钥、完整回答、完整报告、私有配置。

## 2. 启动前验证

在扩大到业务试用前，按顺序执行以下门禁命令。任一失败都不能标记为可试用。

```powershell
go test ./... -count=1
npm --prefix web run test
npm --prefix web run build
go run ./cmd/agent-verify -session testdata/agent_verify/pass_session.json -tool-events testdata/agent_verify/pass_tool_events.json -memory-observations testdata/agent_verify/pass_memory_observations.json
go run ./cmd/internal-trial-smoke
openspec validate --all --strict
```

检查重点：

- Go 测试必须覆盖后端配置、Agent、Graph、RAG、长期记忆和工具 trace 的既有门禁。
- 前端测试和构建必须通过，避免 `tool_trace` 展示或类型变更破坏候选人工作台。
- `agent-verify` 必须同时读取 session、tool events 和 memory observations fixture。
- `internal-trial-smoke` 必须保持离线 in-process 验证，不启动真实服务，不访问公网，不写入外部系统。
- `openspec validate --all --strict` 必须通过，保证内部试用文档、规格和现有能力边界没有明显冲突。

## 3. 默认 mock/offline 工具检查

默认配置应保持：

```yaml
internal_trial:
  enabled: true
  owner_header: "X-Internal-User"
  allow_dev_fallback: false
  github_tool_mode: mock
  github_api_base_url: "https://api.github.com"
```

技术试用启动时先确认：

- `github_tool_mode` 是 `mock`，除非本轮明确验证真实 GitHub 工具。
- mock/offline 路径不访问公网，不依赖 GitHub token，不依赖外部 MCP Gateway。
- 如果真实工具缺少 HTTP client 或 API base URL，必须暴露 `config_missing`，不能伪装成成功。
- 前端和文档只能根据结构化字段判断 mock、real、failed 或 `config_missing`，不能从自然语言回答中猜。

## 4. Agent project_polish 与 tool_trace 检查

在 Agent 消息入口输入包含 GitHub 仓库 URL 的 `project_polish` 请求，检查返回结构：

- skill 或 intent 能路由到 `project_polish`。
- 响应包含顶层 `tool_trace` 时，至少能看到工具名 `github.project_analyze`、权限、状态、错误类别和摘要。
- mock 成功时 `tool_trace.status` 应能表达成功状态，正文不能硬编码声称真实 GitHub 分析。
- 真实 GitHub 工具失败时必须记录 `tool_trace.status/error_class`，例如网络、HTTP、解析或 `config_missing`；必要时回滚 mock。
- 前端只读展示后端返回的 `tool_trace`，不得根据回答正文反推工具状态。

失败时记录最小可复现信息：请求入口、session id 或命令、GitHub URL 是否为公开仓库、`tool_trace.status`、`tool_trace.error_class`、期望结果、实际结果。不要记录 token、请求 header 原文、完整回答或完整报告。

## 5. 长期记忆 owner 和观测检查

长期记忆检查分两块：owner 边界和写入观测。

owner 边界：

- 内部试用应使用可信 `X-Internal-User` 或配置的 `owner_header`。
- `allow_dev_fallback` 默认必须为 `false`；只有本地开发诊断才允许显式打开。
- 读取 `/api/users/:user_id/memory` 时，当前 owner 必须和目标 user id 匹配；不匹配应拒绝。
- 这不是 JWT/OIDC，也不是生产鉴权模型；它只适合受控内部入口之后的试用验证。

观测检查：

- 完整面试结束后，长期记忆写入失败不能阻断面试完成。
- memory observation 应能区分 success、skipped、failed 和 conflict retry exhausted 等状态。
- `go run ./cmd/agent-verify -session testdata/agent_verify/pass_session.json -tool-events testdata/agent_verify/pass_tool_events.json -memory-observations testdata/agent_verify/pass_memory_observations.json` 必须验证 memory observations。
- `go run ./cmd/internal-trial-smoke` 必须输出或校验长期记忆相关摘要，作为扩大试用前的最低诊断证据。

## 6. 失败记录要求

技术失败必须可复现，至少记录：

- 失败阶段：启动、验证命令、API、`project_polish`、`tool_trace`、memory、owner 或前端展示。
- 复现命令或用户操作步骤。
- session id、请求入口或测试 fixture 名称。
- 期望结果和实际结果。
- 结构化错误类别，例如 `tool_trace.status/error_class`、`config_missing`、memory observation 状态或 HTTP status。
- 当前是否为 mock/offline，是否显式切到 real GitHub。

禁止记录：

- token、密钥、cookie、完整 Authorization header。
- 完整回答、完整报告、完整简历或完整私有配置。
- 私有仓库内容、内部 API base URL 细节或无法脱敏的原始 payload。

## 7. 回滚

触发以下任一情况，暂停扩大试用范围：

- 任一启动前验证命令失败。
- 真实 GitHub 工具失败被伪装成成功，或缺少稳定 `tool_trace.status/error_class`。
- 默认 mock/offline 路径访问公网。
- owner 边界混乱，开发 fallback 被当成内部试用默认身份。
- 长期记忆失败不可观测，或 memory failure 阻断面试完成。

回滚步骤：

1. 将 `internal_trial.github_tool_mode` 改回 `mock`，必要时移除真实 GitHub 工具相关环境变量。
2. 确认 `allow_dev_fallback` 为 `false`，并只接受可信 `owner_header`。
3. 重新执行 `go run ./cmd/internal-trial-smoke` 和 `go run ./cmd/agent-verify -session testdata/agent_verify/pass_session.json -tool-events testdata/agent_verify/pass_tool_events.json -memory-observations testdata/agent_verify/pass_memory_observations.json`。
4. 若回滚后 smoke 或 agent verify 仍失败，停止业务试用，按失败记录要求建单。
5. 回滚完成后重新运行 `openspec validate --all --strict`，确认文档和规格仍然一致。
