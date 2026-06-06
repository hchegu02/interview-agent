## Context

当前 `Makefile verify-local` 已执行：

```text
go test ./...
npm --prefix web run test
npm --prefix web run build
make eval-rag
make questionbank-lint
make eval-mock
git diff --check
```

但 `cmd/agent-verify` 已经存在且可基于 `testdata/agent_verify/pass_session.json` 做报告和 retrieval trace 门禁，没有纳入统一流程。

同时 SDD / commands 中的 RAG eval 示例还使用不存在的 `testdata/rag_eval/config.json` 和旧参数，和 Makefile 当前真实命令不一致。

## Goals / Non-Goals

**Goals:**

- 增加 `verify-agent` Makefile 目标。
- `verify-local` 执行 `verify-agent`。
- 文档中的 RAG eval 命令统一到 `testdata/rag/golden_queries.jsonl` 和当前 CLI 参数。
- 增加 `/api/agent/message` fixture 测试，锁住 ProjectPolish mock tool HTTP 链路。

**Non-Goals:**

- 不改变 `cmd/agent-verify` 语义。
- 不新增 `cmd/agent-message-verify`。
- 不新增真实外部依赖。
- 不改变 HTTP API 字段。

## Decisions

### 1. `agent-verify` 作为 Makefile 门禁

`agent-verify` 已经用退出码表达失败，适合被 Makefile 直接调用。第一步只用现有 pass fixture，不把可选 `-tool-events` 纳入硬门槛。

### 2. HTTP fixture 用关键字段断言

ProjectPolish mock 输出是可读文案，全文断言会让文案调整变脆。测试只用 request fixture，并断言 intent、skill、title 和关键 marker。

### 3. 文档以 Makefile 为真实来源

RAG eval 的参数较多，容易在多个文档漂移。SDD、commands 和 README 应指向同一条当前可执行命令。

## Tests

- `go test ./internal/httpapi -run TestAgentMessage_ProjectPolishUsesDefaultMockToolFixture -count=1`
- `go run ./cmd/agent-verify -session testdata/agent_verify/pass_session.json`
- `make verify-agent`
- `go test ./... -count=1`
- `openspec validate close-agent-rag-verification-gates --strict`

## Risks / Trade-offs

- [Risk] `verify-local` 更慢。Mitigation：`agent-verify` 使用本地 JSON fixture，开销很小。
- [Risk] HTTP fixture 被 mock 文案变化影响。Mitigation：只断言关键 marker，不做全文匹配。
- [Risk] 文档命令再次漂移。Mitigation：README / SDD / commands 都引用 Makefile 目标和同一命令。
