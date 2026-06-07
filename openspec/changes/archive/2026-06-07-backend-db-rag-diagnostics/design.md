## Overview

本变更采用“先统一配置入口，再补诊断证据”的方式处理数据库/RAG 风险。核心原则是保持运行时行为兼容，不在没有真实计划证据前重写题库分页或搜索 SQL。

## Design Decisions

### 1. Postgres pool config 下沉到 `internal/config`

原 `cmd/server` 本地 helper 只能保证 HTTP server 使用配置，但 `cmd/rag-eval`、`cmd/demo` 和诊断 CLI 也会连接 PostgreSQL。将构建逻辑放到 `internal/config.PostgresPoolConfig` 后，所有读取项目 `Config` 的后端入口都能复用同一套 `pgxpool.Config` 映射。

数据流：

```text
config.Load
  -> Config.validate
  -> config.PostgresPoolConfig
  -> pgxpool.NewWithConfig
```

### 2. 配置校验在 `Config.validate` 早失败

因为 pool 配置现在会真实生效，非法值不能继续沉默进入运行期。校验范围限定在现有字段：

- `postgres.max_conns > 0`
- `postgres.min_conns >= 0`
- `postgres.min_conns <= postgres.max_conns`
- `postgres.max_conn_lifetime > 0`
- `postgres.health_check_period > 0`

### 3. 题库搜索先补 EXPLAIN，不先重写 SQL

`PGStore.List` 的查询构建被抽成 `buildListQuery`，`List` 和 `BuildListExplainQuery` 复用同一 SQL shape。新增 CLI 只执行只读 SELECT 的 `EXPLAIN (ANALYZE, BUFFERS)`，用于判断后续是否需要：

- 将深分页从 OFFSET 改为 keyset cursor。
- 拆分 `ILIKE OR` 查询路径。
- 避免 `unnest(tags)` 破坏索引利用。
- 增加或调整索引。

### 4. PGVector trace 复用现有 `PipelineResult`

不新增新的 retriever 接口。`PGVectorRetriever` 额外实现与 `RetrievalPipeline` 已有形状兼容的 `Search(ctx, Query) (PipelineResult, error)`。`Retrieve` 继续满足原 `Retriever` 接口，外部业务节点不需要修改。

PG SQL 增加只读证据列：

- `vector_hit`
- `tag_hit`
- `text_hit`
- `text_score`

这些字段只用于 trace，不进入 Session 或 HTTP 响应。

## Data Flow

### Question Bank Explain

```text
CLI args
  -> questionbank.Filter
  -> BuildListExplainQuery
  -> PGStore.ExplainList
  -> stdout EXPLAIN plan lines
```

### RAG Eval Stage Candidates

```text
golden case
  -> query embedding
  -> PGVectorRetriever.Search
  -> PipelineResult.Trace.Stages
  -> caseResult.stage_candidates
  -> summary.json
```

## Compatibility

- No HTTP API changes.
- No DB schema changes.
- No Session JSON changes.
- Existing `Retriever.Retrieve` callers remain compatible.
- `cmd/rag-eval` output is additive.

## Risks

- `EXPLAIN ANALYZE` executes the SELECT; deep offset diagnostics should not run against production peak traffic.
- PGVector `StageRRF` label represents existing Go-side fusion topK for compatibility with current eval gates; it is not the same implementation as `RetrievalPipeline.MergeRRF`.
- `cmd/reindex` still uses its own `-dsn` path because it does not load project `Config`; changing it is separate work.
