# 内部试用启动清单

本文档用于把当前 Interview Agent 交给内部团队试用。它不是生产发布手册，也不声明外部可用能力。

## 1. 当前可试用范围

- 候选人工作台：开始面试、提交回答、查看进度、查看报告。
- 后端面试链路：Session、Agent Graph、RAG、评分、追问、报告生成。
- 长期记忆：面试完成后沉淀用户画像；失败不阻断面试完成，通过观测信号诊断。
- Agent 消息入口：`/api/agent/message` 支持 `quiz`、`explain`、`project_polish` 等 skill。
- 工具 trace：`project_polish` 可返回顶层 `tool_trace`，前端只读展示工具名、状态、错误类别和摘要。
- 内部试用 smoke：`go run ./cmd/internal-trial-smoke` 默认离线验证核心链路。

### 试用闭环文档

- [技术试用 Runbook](internal-trial/technical-trial-runbook.md)
- [业务试用 Runbook](internal-trial/business-trial-runbook.md)
- [RAG 题库业务试用 Runbook](internal-trial/rag-questionbank-business-trial-runbook.md)
- [试用问题模板](internal-trial/trial-issue-template.md)
- [试用反馈模板](internal-trial/trial-feedback-template.md)
- [试用 Go/No-Go](internal-trial/trial-go-no-go.md)

## 2. 明确不可试用范围

- 不提供完整 JWT/OIDC 登录。
- 不提供租户、用户中心、计费或生产级权限体系。
- 不提供完整 MCP Gateway、MCP runtime、daemon 或 Sandbox。
- 不提供运行时 sub-agent 调度；Codex sub-agent 只是开发协作方式。
- 不承诺外部生产发布、SLA、审计留存或真实用户数据合规边界。

## 3. 推荐配置

默认内部试用先使用 mock/offline 工具路径：

```yaml
internal_trial:
  enabled: true
  owner_header: "X-Internal-User"
  allow_dev_fallback: false
  github_tool_mode: mock
  github_api_base_url: "https://api.github.com"
```

也可以用环境变量覆盖：

```powershell
$env:INTERVIEW_INTERNAL_TRIAL_ENABLED = "true"
$env:INTERVIEW_INTERNAL_TRIAL_OWNER_HEADER = "X-Internal-User"
$env:INTERVIEW_INTERNAL_TRIAL_ALLOW_DEV_FALLBACK = "false"
$env:INTERVIEW_INTERNAL_TRIAL_GITHUB_TOOL_MODE = "mock"
$env:INTERVIEW_INTERNAL_TRIAL_GITHUB_API_BASE_URL = "https://api.github.com"
```

真实 GitHub 工具只允许内部试用显式切换：

```powershell
$env:INTERVIEW_INTERNAL_TRIAL_GITHUB_TOOL_MODE = "real"
```

当前真实 GitHub 工具只读公开仓库 metadata。缺少 HTTP client 或 API base URL 时必须返回 `config_missing`，不能把失败伪装成真实分析成功。

## 4. 启动前门禁

内部试用前至少执行：

```powershell
go test ./... -count=1
npm --prefix web run test
npm --prefix web run build
go run ./cmd/agent-verify -session testdata/agent_verify/pass_session.json -tool-events testdata/agent_verify/pass_tool_events.json -memory-observations testdata/agent_verify/pass_memory_observations.json
go run ./cmd/internal-trial-smoke
openspec validate --all --strict
```

任一命令失败都不应标记为可试用。

## 5. 试用步骤

1. 启动后端服务，并确认 `/healthz` 和 `/readyz` 正常。
2. 启动前端工作台。
3. 让试用者使用可信内部 owner header 或受控网关进入系统。
4. 完成一轮完整面试：开始、答题、追问、报告。
5. 在 Agent 页输入包含 GitHub 仓库 URL 的项目润色请求。
6. 检查 `tool_trace` 是否显示 `github.project_analyze`、权限、状态和错误类别。
7. 检查长期记忆画像读取是否只允许当前 owner 访问。
8. 记录失败样例、session id、trace 状态和复现步骤，不记录 token、密钥或完整隐私正文。

## 6. 回滚和降级

- 工具异常：把 `internal_trial.github_tool_mode` 改回 `mock`。
- 身份异常：关闭 `allow_dev_fallback`，确认只接受可信 `owner_header`。
- 长期记忆异常：先保留面试完成链路，使用观测状态定位 `skipped`、`failed` 或 `conflict_retry_exhausted`。
- 前端展示异常：后端 `tool_trace` 是事实源；前端不得从正文推断 mock、real 或 failed。

## 7. 试用通过标准

- 核心门禁全部通过。
- 默认配置不访问公网 GitHub。
- 真实 GitHub 模式只能由内部试用配置显式启用。
- 工具失败时有稳定 `tool_trace.status` 或 `tool_trace.error_class`。
- 长期记忆写入失败不阻断面试完成。
- 文档和演示口径不把内部试用描述成生产发布。
