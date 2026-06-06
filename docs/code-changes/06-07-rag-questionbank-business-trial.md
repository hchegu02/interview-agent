# 06-07 RAG 题库业务试用

## 变更概述

本次变更把 Go 后端 RAG 题库从“可导入、可检索”推进到内部业务试用闭环：题库源文档导入保留来源信息，Agent review 状态进入暂存项，`rejected` 项不能提交到正式题库；RAG trace 增加 Query Rewriting 和 HyDE shadow 诊断字段；新增 Go 后端专用 RAG eval fixture 和内部试用 runbook。

影响范围是题库导入、题库提交过滤、RAG 检索 trace、节点测试、内部试用文档和 RAG eval 测试数据。

## 变更文件

- `internal/questionbank/imports_types.go`：为 `ImportItem` 增加 Agent review 和 source provenance 字段。
- `internal/questionbank/imports_clone.go`：深拷贝 `SourceProvenance`。
- `internal/questionbank/imports_stage.go`：源文档生成题默认标记为 `needs_human_review`，并保存来源信息。
- `internal/questionbank/imports.go`：真实 `ImportDocument` 路径生成 source/chunk hash provenance。
- `internal/questionbank/imports_pg.go`：通过现有 `field_provenance` jsonb 保留 Agent review/source metadata，避免新增迁移。
- `internal/questionbank/imports_commit.go`：阻止 `ImportAgentReviewRejected` 项提交到正式题库。
- `internal/questionbank/imports_test.go`：覆盖 source provenance、PG metadata packing、Agent rejected commit blocking。
- `internal/retriever/retriever.go`：扩展 retriever trace 字段。
- `internal/domain/session.go`：扩展 session 级 `RetrievalTrace` 字段。
- `internal/nodes/retrieve_rag.go`：增加 Query Rewriter / HyDE shadow 接口、fallback 和 trace 注入。
- `internal/nodes/setup_test.go`：覆盖 Query Rewriting 成功、失败、空结果和 HyDE shadow fallback。
- `testdata/rag/golden_queries_go_backend.jsonl`：新增 Go 后端内部试用 RAG eval fixture。
- `docs/ai/internal-trial/rag-questionbank-business-trial-runbook.md`：新增 RAG 题库业务试用 Runbook。
- `docs/ai/internal-trial-launch-checklist.md`：链接 RAG 题库业务试用 Runbook。

## 函数级说明

### `internal/questionbank/imports.go`

- `processDocument`：在文档切片生成题目后，为每个生成题构造 source provenance，再调用 `stageItemsWithOriginalsAndProvenance`。输入是 `ImportFile`、parser 输出切片和 LLM 生成题；输出是更新后的 `ImportJob`。副作用是写入 import chunks/items。
- `sourceProvenanceForChunk`：根据上传文件、原始内容和切片生成 provenance map，包含 source type、filename、content type、source hash、chunk id 和 chunk hash。
- `sha256Hex`：计算来源内容 hash，用于可追溯诊断。

### `internal/questionbank/imports_stage.go`

- `stageItemsWithOriginalsAndProvenance`：新增 `SourceProvenance` 写入，并按 `job.SourceType` 设置 `AgentReviewStatus`。普通题库导入仍保持 human review status 默认 accepted。
- `defaultAgentReviewStatus`：源文档导入返回 `needs_human_review`，其他导入返回空。

### `internal/questionbank/imports_pg.go`

- `AddItems` / `UpdateItems`：写 PG 时把 field provenance、Agent review 和 source provenance 打包到现有 `field_provenance` jsonb。
- `ListItems`：读取 PG item 后拆出 Agent review/source metadata，恢复到 `ImportItem` 字段。
- `packImportItemMetadata`：把新 metadata 编码到保留 key。
- `unpackImportItemMetadata`：从保留 key 还原新 metadata，并避免污染业务字段来源。

### `internal/questionbank/imports_commit.go`

- `importItemAccepted`：增加 Agent review rejected 判断。`ImportAgentReviewRejected` 优先阻止提交，即使 human review status 是 accepted。

### `internal/nodes/retrieve_rag.go`

- `QueryRewriter` / `QueryRewriteInput` / `QueryRewriteResult`：定义检索前 query 改写接口。输入包含 query、岗位、tags、目标难度、题库 filter 快照和 locale。
- `HyDEGenerator` / `HyDEInput`：定义 HyDE shadow 文本生成接口。
- `NewRetrieveRAGPatchNode`：在 embedding 前执行可选 rewrite；失败或空结果回退原 query。HyDE shadow 只生成 hash/status，不改 live candidate pool。
- `toDomainRetrievalTrace`：复制 retriever trace 的 rewrite/HyDE 字段到 domain trace。
- `annotateRetrievalTrace` / `retrievalDiagnosticsTrace`：为 searcher trace 或 fallback trace 注入诊断字段。
- `runHyDEShadow`：仅在 `HyDEMode == "shadow"` 时调用 HyDE generator，失败时记录 fallback。
- `shortTextHash`：生成 12 位 HyDE 文本 hash，避免 trace 暴露长文本。
- `cloneQuestionBankFilter`：给 rewriter 传 filter 快照，避免下游修改 session filter。

## 调用链

### 源文档题库导入

`ImportService.ImportDocument`
-> `processDocument`
-> `parser.Parse`
-> `buildImportChunks`
-> `generateItems`
-> `sourceProvenanceForChunk`
-> `stageItemsWithOriginalsAndProvenance`
-> `ImportStore.AddItems`

PG 模式下：

`PGImportStore.AddItems`
-> `packImportItemMetadata`
-> `question_bank_import_items.field_provenance`
-> `PGImportStore.ListItems`
-> `unpackImportItemMetadata`

### 题库提交

`ImportService.Commit`
-> `ImportStore.ListItems`
-> `importItemAccepted`
-> `Writer.Upsert`
-> `embedCommittedItems`

### RAG 检索

`BuildInterviewGraph`
-> `retrieve_rag` PatchNode
-> `buildQueryTags`
-> `buildQueryText`
-> optional `QueryRewriter.RewriteQuery`
-> optional `HyDEGenerator.GenerateHyDE` in shadow mode
-> `Embedder.Embed`
-> `retrieveWithTrace`
-> `RetrievalPipeline.Search` or `Retriever.Retrieve`
-> `annotateRetrievalTrace`
-> `StatePatch{CandidatePool, RetrievalTrace}`

## 数据流

源文档数据从上传文件进入 `ImportFile.Reader`，parser 生成文本，chunk 生成题目。系统用原始文件内容和 chunk 内容计算 hash，并把 provenance 写到暂存项。Agent review 状态跟随 `ImportItem` 在内存 store 和 PG store 中保存。提交时 `ImportAgentReviewRejected` 阻断正式题库写入。

RAG query 数据从 `JobProfile`、`GapReport` 和 `QuestionBankFilter` 生成基础 query。可选 rewriter 返回 rewritten query 和 normalized tags；失败时保留原 query。HyDE shadow 生成的文本只写 hash 和状态，不参与候选池排序。

## 依赖与副作用

- 新增 `crypto/sha256`、`encoding/hex` 用于 provenance/hash。
- 没有新增数据库迁移；PG import metadata 复用 `field_provenance` jsonb 保持兼容。
- 没有新增外部 API 调用；Query Rewriter 和 HyDE Generator 是可选接口。
- `RetrievalTrace` JSON 响应新增 `omitempty` 字段，兼容旧客户端。
- `tmp/eval/rag-go-backend` 是验证输出目录，不纳入提交。

## 测试

已执行：

```powershell
go test ./internal/questionbank -count=1
go test ./internal/nodes -run "TestRetrieveRAGRecordsRewriteAndHyDEShadowTrace|TestRetrieveRAGRewriteFailureFallsBackToOriginalQuery|TestRetrieveRAGEmptyRewriteFallsBackToOriginalQuery|TestRetrieveRAGHyDEShadowFailureRecordsFallback" -count=1
go test ./internal/nodes ./internal/retriever ./internal/domain -count=1
go test ./internal/questionbank ./internal/nodes ./internal/retriever -count=1
go run ./cmd/questionbank-lint -seed seeds/question_bank.json -min-expected-points 3 -min-scenario-ratio 0.8
go run ./cmd/rag-eval -cases testdata/rag/golden_queries_go_backend.jsonl -config config/config.yaml.example -out tmp/eval/rag-go-backend
```

`rag-eval` 结果：`cases=8 recall@5=0.750 recall@10=0.875 mrr@10=1.000 ndcg@10=0.858 source=seed`。

## 风险

- HyDE 当前实现为 shadow 诊断，不实现 live enabled ranking。
- Query Rewriter / HyDE Generator 目前是可选接口，尚未接真实 LLM wiring。
- PG metadata 复用 `field_provenance` 保留 key，避免迁移但需要避免外部直接依赖这些内部 key。
- 新 trace 字段为 additive，但前端若要展示仍需后续 UI 支持。
