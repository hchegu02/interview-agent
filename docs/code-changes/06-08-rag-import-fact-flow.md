# 06-08 RAG Import Fact Flow

## 1. 变更概述

本次变更围绕后端事实流收口：题库导入 contract、导入 commit 发布事务诊断、RAG eval 真实 query/candidate pool 工具、运行时检索决策层和已用题排除。

目标是让题库导入、RAG 评测、出题和追问都有可审核、可诊断、可回放的事实边界。未修改前端。

## 2. 变更文件

- `internal/questionbank/imports_parse.go`：版本化导入包、字段兼容解析、字段路径错误语义。
- `internal/questionbank/imports_commit.go`：commit summary、embedding 成功/失败诊断、半提交恢复语义。
- `internal/questionbank/imports_memory_store.go`：内存 store embedding failed 标记。
- `internal/questionbank/imports_pg.go`：PG store embedding failed 标记、nullable embedded_at scan。
- `internal/questionbank/imports_types.go`：commit summary metadata 常量和失败标记接口。
- `internal/questionbank/imports_test.go`：导入 contract、rubric array、坏类型、commit summary、embedding failed 回归。
- `internal/questionbank/testdata/question_bank_import_v1.json`：版本化 contract fixture。
- `cmd/rag-eval/main.go`：新增 `-mode eval|export-queries|build-candidate-pool`。
- `cmd/rag-eval/real_queries.go`：真实 session query 导出、脱敏、JSONL 写入。
- `cmd/rag-eval/candidate_pool.go`：candidate pool 构建、来源合并、可选 live retriever。
- `cmd/rag-eval/main_test.go`：rag-eval 新模式和 metrics 复用测试。
- `internal/retriever/retriever.go`：`Query.ExcludeIDs`。
- `internal/retriever/rule.go`、`bm25.go`、`pgvector.go`：检索阶段排除已用题。
- `internal/retriever/*_test.go`：排除已用题和 SQL 条件回归。
- `internal/nodes/retrieval_decision.go`：Runtime Retrieval Decision Policy。
- `internal/nodes/retrieve_rag.go`：检索 query 传递已用题排除，并在节点返回前二次过滤。
- `internal/nodes/pick_next.go`：选题前执行策略层并记录诊断。
- `internal/nodes/probe.go`：低信息回答 + 弱召回时走固定澄清追问。
- `internal/nodes/*_test.go`：策略层、节点和 graph loop 回归。
- `docs/SDD-Backend.md`：记录后端事实流、工具链和观测边界。
- `openspec/changes/harden-rag-import-fact-flow/tasks.md`：同步任务状态。

## 3. 函数级说明

### 题库导入

- `parseImportPackage` / `parseImportItems`：识别 `question_bank_import.v1`、legacy array、wrapped items，归一化为暂存 import items。
- `parseFlexibleStringList`：解析数组或分隔字符串，字段类型不支持时报错，不再静默返回 nil。
- `parseFlexibleRubric`：兼容 map、array、string rubric，并保留可审核结构。
- `splitImportList`：新增中文分号分隔。
- `commitImportJob` 相关函数：把 commit 明确为发布事务，summary 记录 matched/imported/skipped/embedding synced/failed/failure reasons。
- `MarkEmbeddingsFailed`：embedding 失败时标记正式题库题目为 failed，清理 stale vector/model/embedded_at，保留题目事实。

### RAG eval

- `run`：新增 mode 分发；默认 `eval` 保持旧行为。
- `runExportQueries`：校验 `-sessions`、`-out-file`，读取 session JSON/JSONL，输出脱敏 query JSONL。
- `loadSessions` / `loadExportedQueries`：支持 JSON array、单 session object、JSONL；JSONL scanner 放宽到 4MB 行大小。
- `sanitizeQueryText`：脱敏邮箱、手机号、URL、api_key/token/secret/password。
- `writeJSONLFile`：自动创建 out-file 父目录。
- `runBuildCandidatePool`：从 sessions 或 exported queries 构建 candidate pool，默认离线，不连接外部服务。
- `buildCandidatePoolsFromSessions` / `buildCandidatePoolsFromQueries`：合并 candidate_pool、final、stage、exported_query、keyword、random_negative。
- `addLiveRetrieverCandidates`：仅在 `-live-top-k > 0` 时调用真实 retriever。
- `candidatePoolBuilder.add`：按 ID 合并候选，保留全局 rank/score 和各来源 rank/score。

### Retriever

- `retriever.Query.ExcludeIDs`：新增硬排除字段，用于已问过题。
- `RuleRetriever.Retrieve` / `ruleMatchesHardFilters`：本地规则召回过滤 `ExcludeIDs`。
- `BM25Retriever.Retrieve`：BM25 文本召回过滤 `ExcludeIDs`。
- `PGVectorRetriever.retrieveCandidates`：SQL 参数 `$13` 传 excluded IDs，vector/tag/text 三路 CTE 都排除。

### Nodes

- `decideRuntimeRetrieval`：输入回答、候选池、检索 trace、已用题和 working memory，输出 strategy、include_context、selected、consumed_ids、reason、degraded_reason。
- `filterUsedQuestions` / `usedQuestionIDs`：按已问过题过滤候选。
- `isLowInformationAnswer`：识别空泛、过短、不会/不知道/no idea 等低信息回答。
- `isWeakRetrievalTrace`：根据 nil trace、fallback reasons、空 final、低 top score 判断弱召回。
- `recordRetrievalDecision`：把策略摘要写入 `WorkingMemory.Notes["retrieval_decision"]`，降级原因写入 `DegradedReasons["retrieval_decision"]`。
- `NewRetrieveRAGPatchNode`：构造 `retriever.Query.ExcludeIDs`，返回前用 `filterRetrievedResults` 二次过滤已用题；全被过滤时降级 fallback。
- `filterRetrievedResults` / `resultTraceFromResults`：节点级硬过滤和 trace final 重写。
- `NewPickNextPatchNode`：选题前执行策略层，使用过滤后候选池，候选耗尽时把策略 reason 写入结束原因。
- `NewProbeAskPatchNode`：低信息回答 + 弱召回时不调用 LLM，直接追加澄清追问并 suspend。

## 4. 调用链

- 题库导入：
  `HTTP import API / import service -> parse import JSON -> stage import job -> review accept -> commitImportJob -> question_bank store -> embedding sync/failed marker`
- RAG eval export：
  `cmd/rag-eval main -> run -> runExportQueries -> loadSessionFile -> exportQueriesFromSessions -> writeJSONLFile`
- RAG eval candidate pool：
  `cmd/rag-eval main -> run -> runBuildCandidatePool -> buildCandidatePoolsFromSessions/buildCandidatePoolsFromQueries -> writeJSONLFile`
- 面试检索：
  `BuildInterviewGraph -> retrieve_rag -> NewRetrieveRAGPatchNode -> retriever.Query{ExcludeIDs} -> Retriever.Search/Retrieve -> CandidatePool/RetrievalTrace`
- 出题：
  `pick_next -> decideRuntimeRetrieval -> pickByLLM 或 pickByRule -> PendingDecision + AnswerRound`
- 追问：
  `critic -> RouteAfterCritic -> probe_ask -> decideRuntimeRetrieval -> deterministic clarification 或 probeAskByLLM -> FollowUp`

## 5. 数据流

- JSON import 输入先归一化，坏类型在 parse 阶段失败；成功结果只进入 import staging。
- review/commit 分离：commit 前再次做重复和质量门禁，正式题库写入后 embedding 状态独立记录。
- session 的 `RetrievalTrace` 是真实 query 导出的事实源；导出时脱敏后写 JSONL。
- candidate pool 标注输入保留候选 ID、rank、score 和来源证据，不写回业务库。
- runtime decision 只写 `WorkingMemory.Notes` / `DegradedReasons` 和选题/追问 patch，不改前端响应结构。

## 6. 依赖与副作用

- 未新增第三方依赖。
- `cmd/rag-eval -live-top-k` 默认关闭；开启后会按现有 config 调 embedding/retriever，可能连接 PG。
- `retriever.Query.ExcludeIDs` 是 Go 内部字段，不改变 HTTP API。
- PG SQL 增加 `$13` 参数，三路召回都排除已用题。
- embedding failed 标记会写正式题库状态，但不删除题目。

## 7. 测试

已执行：

```powershell
go test ./internal/questionbank -count=1
go test ./cmd/rag-eval -count=1
go test ./internal/nodes ./internal/retriever -count=1
go test ./... 
```

其中 `go test ./...` 需在最终收口时再次执行，以最终工作区结果为准。

## 8. 风险

- `ExcludeIDs` 新增后，未更新的自定义 retriever 实现可能忽略该字段；`retrieve_rag` 已做节点级二次过滤兜底。
- 弱召回判断是保守启发式；旧 Session 没有 `RetrievalTrace` 时会记录弱召回诊断，但不阻断出题。
- 低信息 + 弱召回追问使用固定澄清问题，牺牲一点灵活性换取不基于弱证据幻觉追问。
- `cmd/rag-eval` 的 live 模式可能触发外部服务连接，默认关闭。
