# 06-07 PGVector RAG 运行证据增强

## 1. 变更概述

本次让 `PGVectorRetriever` 暴露可诊断的 `Search` 结果，并让 `cmd/rag-eval` 在每条 golden case 中写入阶段候选明细。目的是真实观察 PostgreSQL 三路候选召回和 Go 端融合结果，避免只看最终 topK 指标。

影响范围限定在后端 RAG 检索和离线评估输出；不修改数据库 schema、HTTP API、面试 Graph、题库导入流程或前端。

## 2. 变更文件

- `internal/retriever/pgvector.go`
  - 新增 `PGVectorRetriever.Search`。
  - 新增 PG 候选来源证据扫描和 trace 构建。
  - `Retrieve` 改为复用 `Search` 的最终结果。
- `internal/retriever/pgvector_test.go`
  - 覆盖 SQL 是否返回 vector/tag/text 候选来源证据。
  - 覆盖 PGVector trace 是否包含 vector、tag、text 和 fusion 阶段候选。
- `cmd/rag-eval/main.go`
  - `caseResult` 新增 `stage_candidates`。
  - pipeline search 结果会把每个阶段候选 ID 写入 `summary.json`。
- `cmd/rag-eval/main_test.go`
  - 覆盖 per-case stage candidates 输出。

## 3. 函数级说明

### `PGVectorRetriever.Retrieve`

位置：`internal/retriever/pgvector.go`

作用：执行 PGVector 检索并返回最终 topK。

输入：`context.Context`、`retriever.Query`。

输出：`[]retriever.Result`。

副作用：只读查询 PostgreSQL。

错误处理：复用 `Search` 错误。

行为变化：内部改为调用 `Search` 后返回 `PipelineResult.Results`；外部返回语义不变。

### `PGVectorRetriever.Search`

位置：`internal/retriever/pgvector.go`

作用：执行 PGVector 检索并返回最终结果、融合结果和结构化 trace。

输入：`context.Context`、`retriever.Query`。

输出：`retriever.PipelineResult`。

副作用：只读查询 PostgreSQL，并在事务内尝试 `SET LOCAL hnsw.ef_search`。

错误处理：连接池、embedding、SQL 查询、扫描等错误原样包装返回。

主要逻辑：调用 `retrieveCandidates` 获取带来源证据的候选，再调用 `buildPGVectorPipelineResult` 生成最终结果和阶段 trace。

### `retrieveCandidates`

位置：`internal/retriever/pgvector.go`

作用：执行原 PGVector SQL，返回带 vector/tag/text 来源证据的候选。

输入：`context.Context`、`retriever.Query`。

输出：`[]pgCandidate`。

副作用：只读查询 PostgreSQL。

错误处理：保留原有 pool 未初始化、query embedding 缺失、连接、事务、查询和扫描错误。

主要逻辑：保留原过滤条件和 fusion 前特征计算，同时扫描 `vector_hit`、`tag_hit`、`text_hit` 和 `text_score`。

### `buildPGVectorPipelineResult`

位置：`internal/retriever/pgvector.go`

作用：把 PG 候选转换为 `PipelineResult`。

输入：`retriever.Query`、`[]pgCandidate`、`Fusion`、查询耗时。

输出：`retriever.PipelineResult`。

副作用：无。

主要逻辑：调用 fusion 得到 topK；生成 vector、tag、text 和 `rrf` 阶段 trace；最终结果 `Trace.Final` 保留 vector/tag/difficulty 三路分数来源。

### `pgCandidateStageTrace`

位置：`internal/retriever/pgvector.go`

作用：从 PG 候选来源证据生成单个阶段的 `StageTrace`。

输入：阶段名、候选列表、耗时和 include/score 函数。

输出：`StageTrace`。

副作用：无。

主要逻辑：只保留命中该阶段的候选，写入候选 ID、rank、score 和 sources。

### `stageCandidatesFromPipelineResult`

位置：`cmd/rag-eval/main.go`

作用：从 pipeline trace 提取每个阶段候选 ID，写入 per-case 评估结果。

输入：`retriever.PipelineResult`。

输出：`map[string][]string`。

副作用：无。

错误处理：空 trace 返回 nil；当 trace 没有显式 `rrf` 阶段时，用 `RRFResults` 或最终 `Results` 合成 fusion topK。

## 4. 调用链

### 面试运行时 RAG

`retrieve_rag` 节点 -> `retriever.Retriever.Retrieve` -> `PGVectorRetriever.Retrieve` -> `PGVectorRetriever.Search` -> `retrieveCandidates` -> PostgreSQL -> fusion -> 返回 topK

调用链变化：`Retrieve` 内部多了一层 `Search`，外部接口不变。

### 离线 RAG eval

`go run ./cmd/rag-eval ...` -> `evaluate` -> 检测 retriever 是否实现 `Search` -> `PGVectorRetriever.Search` -> `stageCandidatesFromPipelineResult` -> `summary.json`

## 5. 数据流

PG SQL 原本返回候选基础字段和融合特征。本次额外返回：

- `vector_hit`：候选是否来自 `vector_candidates`。
- `tag_hit`：候选是否来自 `tag_candidates`。
- `text_hit`：候选是否来自 `text_candidates`。
- `text_score`：`similarity(content, query)` 分数。

这些字段只进入 trace，不进入业务题目、Session 或 HTTP 响应。

`cmd/rag-eval` 的每条 case 增加：

```json
"stage_candidates": {
  "vector": ["..."],
  "rule": ["..."],
  "bm25": ["..."],
  "rrf": ["..."]
}
```

其中 `rule` 当前对应 tag 候选证据，`bm25` 对应 PG 文本 similarity 候选证据，`rrf` 对应 Go 端 fusion topK。

## 6. 依赖与副作用

- 不新增依赖。
- 不新增迁移。
- 不写数据库。
- PGVector SQL 返回列增加，调用方扫描逻辑已同步。
- `EXPLAIN` 诊断和 `rag-eval` 输出会更大，但只影响离线诊断文件。

## 7. 测试

已执行：

```powershell
go test ./internal/retriever -run "PGVectorSearchBuildsStageTrace|RetrieveSQLExposesCandidatePath" -count=1
go test ./cmd/rag-eval -run "TestEvaluateCollectsPipelineStageMetrics|TestEvaluateDoesNotDoubleCountExplicitRRFStage|TestEvaluateStageMetricsUseAllCasesAsDenominator" -count=1
go test ./internal/retriever -count=1
go test ./cmd/rag-eval -count=1
```

结果：全部通过。

## 8. 风险

- 兼容性：`Retriever` 接口未变；`Search` 是额外方法，`rag-eval` 通过可选接口检测使用。
- 性能：SQL 多返回几个布尔/分数字段，正常检索成本变化很小；trace 构建只在 Go 内存中完成。
- 指标语义：PGVector 的 `rrf` 阶段实际是现有 Go 端 fusion topK，不是 `RetrievalPipeline` 的真正 RRF merge；沿用该 stage 名是为了复用现有 eval stage gate。
- 已知限制：当前 trace 记录候选 ID 和分数来源，不记录完整 SQL plan；查询计划仍需配合 `cmd/questionbank-explain` 查看。
