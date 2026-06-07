---
comet_change: backend-db-rag-diagnostics
role: technical-design
canonical_spec: openspec
archived-with: 2026-06-07-backend-db-rag-diagnostics
status: final
---

# Backend DB/RAG Diagnostics Design

## Goal

收口后端数据库和 RAG 诊断能力，让内部试用前可以验证 Postgres pool 配置、题库列表查询计划和 PGVector 三路候选贡献。

## Current Problem

数据库相关风险不是单个 bug：

- `cmd/server` 曾经绕过 pool 配置，修复后其他后端 CLI 仍可能绕过。
- 题库列表查询有深分页和搜索条件风险，但缺少真实 EXPLAIN 工具。
- PGVector SQL 已经有 vector/tag/text 三路候选，但 eval 只看最终 topK，不足以定位是哪一路失效。

## Approach

### Shared Pool Config

把 pool config 映射放到 `internal/config.PostgresPoolConfig`：

```text
Config.PostgresDSN + Config.Postgres
  -> pgxpool.ParseConfig
  -> MaxConns / MinConns / MaxConnLifetime / HealthCheckPeriod
  -> pgxpool.NewWithConfig
```

同时在 `Config.validate` 校验 pool 参数，避免非法值进入运行期。

### Query Plan Diagnostics

抽出 `PGStore.List` 的 SQL 构建函数，让正常列表和 EXPLAIN 诊断复用相同 query shape。新增 `cmd/questionbank-explain`，仅输出 `EXPLAIN (ANALYZE, BUFFERS)` 文本，不改数据。

### PGVector Stage Evidence

`PGVectorRetriever` 继续实现原 `Retriever.Retrieve`，同时额外实现可选 `Search` 方法，返回 `PipelineResult`。RAG eval 已经通过可选接口探测 pipeline search，因此无需改公共 retriever interface。

PGVector SQL 额外返回：

- `vector_hit`
- `tag_hit`
- `text_hit`
- `text_score`

这些字段只用于 trace，不进入 Session、HTTP API 或数据库 schema。

## Non-Goals

- 不改题库分页语义。
- 不改题库搜索 SQL。
- 不新增数据库 migration。
- 不处理前端布局。
- 不把 trace 扩散到面试 Session。

## Verification

最小验证：

```powershell
go test ./internal/config -count=1
go test ./internal/questionbank -count=1
go test ./internal/retriever -count=1
go test ./cmd/rag-eval -count=1
go test ./cmd/questionbank-explain -count=1
go test ./... -count=1
openspec validate backend-db-rag-diagnostics --strict
```

真实 PG 验证需要本地 `INTERVIEW_POSTGRES_DSN` 和已导入题库数据，当前不作为本次自动门禁。
