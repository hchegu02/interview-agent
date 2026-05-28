# 05-28 question bank auto embedding

## Overview

题库导入 commit 后自动生成 embedding。这样上传题库不只是写入列表页，而是直接进入可向量召回的 RAG 数据面。

## Files

- `migrations/005_question_bank_embedding_status.*.sql`
- `internal/questionbank/imports.go`
- `internal/questionbank/store.go`
- `internal/questionbank/pg_store.go`
- `internal/retriever/pgvector.go`
- `cmd/server/main.go`
- `cmd/reindex/main.go`
- `internal/httpapi/question_bank.go`
- `web/src/main.tsx`
- `web/src/types.ts`

## Function Notes

- `ImportService.Commit`：正式写入题库后调用 embedder 批量生成向量。
- `ImportService.embedCommittedItems`：构造 embedding 文本、校验维度、写回 embedding。
- `PGStore.UpsertEmbeddings`：更新 `question_bank.embedding` 和 embedding 状态字段。
- `MemoryStore.UpsertEmbeddings`：支持单测和无 PG 本地模式。
- `PGVectorRetriever`：召回时过滤 `embedding IS NOT NULL`，避免 pending 题进入向量路径。

## Call Chain

`POST /api/question-bank/imports/:id/commit`
-> `ImportService.Commit`
-> `Writer.Upsert`
-> `Embedder.Embed`
-> `EmbeddingWriter.UpsertEmbeddings`
-> `question_bank.embedding`

## Data Flow

`valid import_items`
-> `question_bank`
-> `Embedder`
-> `question_bank.embedding`
-> `embedding_status = embedded`

## Dependencies

- 复用现有 mock/real embedding 实现。
- `cmd/server` 装配 import service 时注入 embedder。
- `cmd/reindex` 同步维护 embedding 状态字段。

## Tests

- migration 测试覆盖 005 字段和索引。
- import service 测试覆盖 commit 后 embedding 写入。
- retriever SQL 测试覆盖 `embedding IS NOT NULL`。

## Risks

- 当前是同步 embedding，真实 embedding API 慢或失败会让 commit 请求变慢或失败。
- 下一阶段应改成异步队列，支持 pending/failed 重试和批量 worker。
