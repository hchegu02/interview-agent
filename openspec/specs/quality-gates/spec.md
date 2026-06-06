# quality-gates Specification

## Purpose
TBD - created by archiving change close-agent-rag-verification-gates. Update Purpose after archive.
## Requirements
### Requirement: 本地质量门禁应包含 Agent 输出验证

系统 MUST 提供本地质量门禁命令来运行 `cmd/agent-verify` 的通过用例，并将其纳入统一本地验证流程。

#### Scenario: 运行 Agent 验证门禁

- **WHEN** 开发者运行 `verify-agent`
- **THEN** 系统 MUST 执行 `go run ./cmd/agent-verify -session testdata/agent_verify/pass_session.json`
- **AND** 验证失败时命令 MUST 返回非 0

#### Scenario: verify-local 包含 Agent 验证

- **WHEN** 开发者运行 `verify-local`
- **THEN** 系统 MUST 执行 Agent 验证门禁

### Requirement: Agent Message mock tool 链路应有 HTTP fixture 验证

系统 MUST 使用 fixture 验证 `/api/agent/message` 的 `project_polish` mock GitHub 工具链路。

#### Scenario: ProjectPolish HTTP fixture 使用 mock tool

- **WHEN** fixture 请求包含 GitHub 仓库 URL
- **THEN** `/api/agent/message` MUST 返回 `skill.project_polish`
- **AND** 响应内容 MUST 包含 mock GitHub 项目分析 marker

### Requirement: 验证命令文档应与真实 Makefile 和 CLI 保持一致

系统 MUST 在 README、SDD 和 AI commands 中使用当前真实存在的 RAG eval 路径、参数和 Agent verify 命令。

#### Scenario: 文档使用当前 RAG eval 命令

- **WHEN** 文档描述 RAG eval
- **THEN** 命令 MUST 使用 `testdata/rag/golden_queries.jsonl`
- **AND** 命令 MUST 使用当前 `cmd/rag-eval` 支持的参数名

