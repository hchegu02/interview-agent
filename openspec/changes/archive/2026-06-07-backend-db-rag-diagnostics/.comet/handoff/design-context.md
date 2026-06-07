# Comet Design Handoff

- Change: backend-db-rag-diagnostics
- Phase: design
- Mode: compact
- Context hash: acb09cf72a991dafe5b533b8bea5cfef2e8d811d287666ccd0bf99440555e5fd

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/backend-db-rag-diagnostics/proposal.md

- Source: openspec/changes/backend-db-rag-diagnostics/proposal.md
- Lines: 1-40
- SHA256: 6adbfc3fea015356fb2cb72062be64b7a41a09c96e496b65227427e87dac425a

```md
## Why

当前后端已经开始接入真实 PostgreSQL、pgvector 和内部试用 RAG 题库，但数据库相关问题缺少足够的运行证据：

- Postgres pool 配置需要在所有使用项目配置的后端入口中统一生效，不能只有 HTTP server 生效。
- 题库列表搜索存在 `OFFSET` 深翻页、`ILIKE OR` 和 `unnest(tags)` 查询计划风险，需要先通过真实 `EXPLAIN` 观察，而不是盲改 SQL。
- PGVector 检索已有 vector/tag/text 三路候选和 Go 端 fusion，但离线评估之前无法逐 case 看到三路候选贡献。

这些问题会直接影响内部团队试用时的稳定性、性能诊断和 RAG 质量判断。

## What Changes

- 将 Postgres pool config 构建下沉到 `internal/config`，供 server 和后端 CLI 复用。
- 对 Postgres pool 参数增加配置校验，非法值在启动或 CLI 加载配置时提前失败。
- 新增题库列表查询 EXPLAIN 诊断 CLI，用真实列表查询形状输出 `EXPLAIN (ANALYZE, BUFFERS)`。
- 让 PGVector retriever 暴露可选 `Search` 诊断结果，记录 vector/tag/text/fusion 阶段候选。
- 增强 RAG eval per-case 输出，写入 `stage_candidates`，用于分析每个 golden case 的候选来源。

## Scope

包含：

- 后端配置、数据库连接池、题库 PG store、PGVector retriever、RAG eval CLI。
- 对应 Go 测试和代码变更文档。

不包含：

- 前端布局或候选人工作台调整。
- 数据库 schema 迁移。
- 题库搜索 SQL 重写或 keyset pagination。
- Session JSON 拆表。
- events 分区、TTL 或审计表治理。
- 真实生产数据库运行结果提交。

## Impact

- 正常 HTTP API 响应和 Session JSON 结构不变。
- `cmd/server`、`cmd/rag-eval`、`cmd/demo`、`cmd/questionbank-explain` 在配置了 Postgres DSN 时统一使用项目 pool 设置。
- `cmd/rag-eval` 的 `summary.json` 增加可选字段 `stage_candidates`。
- 新增 `cmd/questionbank-explain` 只读诊断命令。
```

## openspec/changes/backend-db-rag-diagnostics/design.md

- Source: openspec/changes/backend-db-rag-diagnostics/design.md
- Lines: 1-87
- SHA256: 09f028caf07b58a497bb237c211970578d635338141087222c4eea77f5fe97ae

[TRUNCATED]

```md
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
```

Full source: openspec/changes/backend-db-rag-diagnostics/design.md

## openspec/changes/backend-db-rag-diagnostics/tasks.md

- Source: openspec/changes/backend-db-rag-diagnostics/tasks.md
- Lines: 1-29
- SHA256: 91e8a015dfd8c79fe7627c54aa33281768d2570c04e6d19ce44b449fc2343d13

```md
## 1. Postgres Pool Config

- [x] Move Postgres pool config mapping to `internal/config`.
- [x] Reuse project pool config in `cmd/server`, `cmd/rag-eval`, `cmd/demo`, and `cmd/questionbank-explain`.
- [x] Add validation for invalid Postgres pool settings.
- [x] Add unit tests for pool config mapping and validation.

## 2. Question Bank Query Diagnostics

- [x] Extract question bank list SQL construction so runtime list and EXPLAIN share one query shape.
- [x] Add `PGStore.ExplainList` and `BuildListExplainQuery`.
- [x] Add `cmd/questionbank-explain` CLI.
- [x] Add tests for EXPLAIN SQL construction and CLI filter mapping.

## 3. PGVector / RAG Eval Trace

- [x] Add PGVector candidate source evidence for vector/tag/text paths.
- [x] Add `PGVectorRetriever.Search` returning `PipelineResult`.
- [x] Add per-stage trace for vector, tag, text, and fusion candidates.
- [x] Add `stage_candidates` to RAG eval case output.
- [x] Add tests for PGVector trace and RAG eval stage candidate output.

## 4. Verification

- [x] Run targeted package tests for config, questionbank, retriever, rag-eval, demo, server.
- [x] Run `go test ./... -count=1`.
- [x] Run `git diff --check`.
- [ ] Run `openspec validate backend-db-rag-diagnostics --strict`.
- [ ] Archive or otherwise close this backend change after validation.
```

## openspec/changes/backend-db-rag-diagnostics/specs/rag-retrieval-enhancement/spec.md

- Source: openspec/changes/backend-db-rag-diagnostics/specs/rag-retrieval-enhancement/spec.md
- Lines: 1-52
- SHA256: 8eb22a652b83bfa8a7de3556f7072fc788972c6b7065f6d54cc511d1d0a756c7

```md
## MODIFIED Requirements

### Requirement: Go 后端题库试用应有 RAG 策略对比门禁

系统 MUST 为 Go 后端单岗位内部试用提供 RAG eval 对比，至少覆盖 baseline、query rewrite 和 HyDE shadow 诊断路径，并能输出检索阶段候选证据用于人工排查。

#### Scenario: 运行 Go 后端 RAG 题库试用 eval

- **WHEN** 维护者准备发布 Go 后端题库试用包
- **THEN** 验证 MUST 运行 RAG eval golden queries
- **AND** 验证 MUST 输出 baseline 与 query rewrite 的对比结果
- **AND** HyDE shadow 结果 MUST 可用于人工判断是否升级到 enabled

#### Scenario: RAG eval 输出阶段候选证据

- **WHEN** RAG eval 使用支持 pipeline search 的 retriever
- **THEN** 每个 case 的评估结果 SHOULD include per-stage candidate IDs
- **AND** stage evidence SHOULD include vector, text, tag/rule, and fusion candidates when those stages execute
- **AND** the evidence MUST be additive and MUST NOT change recall, MRR, nDCG, or gate calculation semantics

### Requirement: PGVector 检索应提供运行诊断 trace

系统 MUST make PGVector retrieval diagnosable by exposing candidate evidence for its internal recall paths without changing the existing `Retriever.Retrieve` contract.

#### Scenario: PGVector search returns internal stage trace

- **WHEN** PGVector retrieval executes vector, tag, and text candidate paths
- **THEN** the diagnostic search result MUST record which candidates came from each path
- **AND** the final fusion candidates MUST be recorded in the trace
- **AND** existing `Retrieve` callers MUST continue receiving only final topK results

#### Scenario: PGVector trace does not leak into session state

- **WHEN** PGVector diagnostic fields are produced
- **THEN** vector/tag/text hit flags and text scores MUST remain internal trace evidence
- **AND** they MUST NOT require HTTP API, Session JSON, or database schema changes

### Requirement: 题库列表查询应支持查询计划诊断

系统 MUST provide a read-only way to inspect the PostgreSQL execution plan for the question bank list query used by the backend.

#### Scenario: Maintainer runs question bank EXPLAIN

- **WHEN** a maintainer runs the question bank explain command with application config and filters
- **THEN** the command MUST build the same WHERE, ORDER BY, LIMIT, and OFFSET query shape as the runtime list path
- **AND** it MUST output `EXPLAIN (ANALYZE, BUFFERS)` lines
- **AND** it MUST NOT modify question bank data

#### Scenario: Invalid list cursor fails before query execution

- **WHEN** the explain command receives an invalid cursor
- **THEN** the command MUST fail before executing the database query
```

## openspec/changes/backend-db-rag-diagnostics/specs/server-runtime/spec.md

- Source: openspec/changes/backend-db-rag-diagnostics/specs/server-runtime/spec.md
- Lines: 1-38
- SHA256: b8891636ab90ac9575506e3534cae40dad8542f5be91c079773f94411833f8b8

```md
## MODIFIED Requirements

### Requirement: Postgres Pool Configuration

When a Postgres DSN is configured, backend entry points that load application `Config` SHALL apply the configured Postgres pool controls to the runtime pgxpool before opening the pool.

#### Scenario: Pool config maps from application config

- **GIVEN** `Config.PostgresDSN` is non-empty
- **AND** `Config.Postgres` sets `MaxConns`, `MinConns`, `MaxConnLifetime`, and `HealthCheckPeriod`
- **WHEN** backend code constructs the Postgres pool configuration through the shared config helper
- **THEN** the resulting pgxpool config contains the configured values
- **AND** the runtime pool is opened with that config

#### Scenario: Server startup uses configured pool

- **GIVEN** the HTTP server is started with a non-empty Postgres DSN
- **WHEN** server dependencies are built
- **THEN** the Postgres pool is opened with the shared configured pgxpool config

#### Scenario: Backend diagnostics and eval use configured pool

- **GIVEN** `cmd/rag-eval`, `cmd/demo`, or `cmd/questionbank-explain` loads application config with a non-empty Postgres DSN
- **WHEN** the command opens a Postgres pool
- **THEN** it MUST use the shared configured pgxpool config

#### Scenario: Invalid pool config fails early

- **GIVEN** application config has an invalid Postgres pool setting
- **WHEN** config validation runs
- **THEN** validation MUST fail before a Postgres pool is opened

#### Scenario: No DSN keeps Postgres disabled

- **GIVEN** `Config.PostgresDSN` is empty
- **WHEN** server dependencies are built
- **THEN** no Postgres pool is created
- **AND** in-memory stores remain available for local/mock operation
```

