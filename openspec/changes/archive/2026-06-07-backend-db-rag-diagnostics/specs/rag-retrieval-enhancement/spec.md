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
