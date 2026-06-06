## Why

项目已经具备多条验证能力：Go 测试、RAG eval、questionbank lint、mock eval 和 agent-verify。但当前流程还有两个问题：

- `agent-verify` 可作为硬门禁，却没有纳入 `Makefile verify-local`。
- `docs/SDD-Backend.md` 和 `docs/ai/commands.md` 中的 RAG eval 命令仍引用过期路径和参数，和 Makefile / README 不一致。
- `/api/agent/message` 的 ProjectPolish mock tool 链路已有单元测试，但缺少 HTTP 请求级 fixture 测试。

需要把已有能力收口成可重复执行的本地质量门禁，不扩展业务行为。

## What Changes

- 新增 Makefile 目标 `verify-agent`，执行 `cmd/agent-verify` 的 pass fixture。
- 将 `verify-agent` 纳入 `verify-local`。
- 修正 SDD 和 AI commands 中过期的 RAG eval 命令。
- 新增 `/api/agent/message` project polish mock tool 请求 fixture 和 HTTP 测试。
- 更新 README 质量门禁说明，保持当前能力边界准确。

## Non-Goals

- 不重写 `cmd/rag-eval` 指标逻辑。
- 不调整 RAG eval 阈值。
- 不新增 CI 平台配置。
- 不扩展 `agent-verify` 为万能 Agent Message verifier。
- 不接真实 GitHub / MCP / Web 工具。
- 不改变 HTTP API 响应结构。

## Impact

- 影响文件：`Makefile`、`internal/httpapi` 测试、`testdata/agent_message`、`docs/SDD-Backend.md`、`docs/ai/commands.md`、`README.md`。
- 不改变生产代码业务行为。
- 不改变 Session JSON、数据库 schema、前端类型。
