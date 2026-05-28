# 05-28 question bank import

## Overview

新增题库导入工程化 v1：导入数据先进入 staging，校验通过后再提交到正式 `question_bank`。支持两条入口：

- 本地题库文件上传：JSON / CSV / Markdown 解析为结构化题目。
- 文档上传：复用文档 parser 切片，再由 LLM 生成结构化题目。

## Files

- `migrations/004_question_bank_imports.*.sql`
- `internal/questionbank/imports.go`
- `internal/questionbank/imports_pg.go`
- `internal/questionbank/store.go`
- `internal/questionbank/pg_store.go`
- `internal/httpapi/question_bank.go`
- `internal/httpapi/router.go`
- `cmd/server/main.go`
- `internal/llm/mock.go`
- `web/src/apiClient.ts`
- `web/src/main.tsx`
- `web/src/styles.css`
- `web/src/types.ts`

## Function Notes

- `questionbank.ImportService.ImportLocalQuestionBank`：解析本地题库文件并写入 import item staging。
- `questionbank.ImportService.ImportDocument`：解析文档、切片、调用 LLM 生成题目并 staging。
- `questionbank.ImportService.Commit`：只把 `valid` item upsert 到正式 `question_bank`。
- `questionbank.PGImportStore` / `MemoryImportStore`：分别服务生产 PG 和测试/本地无 PG 模式。
- `httpapi.createQuestionBankImport`：接收 multipart upload，按 `source_type` 分发。
- `httpapi.commitQuestionBankImport`：触发 staged items 提交入库。

## Call Chain

本地题库：

`POST /api/question-bank/imports`
-> `createQuestionBankImport`
-> `ImportLocalQuestionBank`
-> `parseQuestionBankItems`
-> `ImportStore.AddItems`
-> `POST /api/question-bank/imports/:id/commit`
-> `ImportService.Commit`
-> `questionbank.Writer.Upsert`

文档生成：

`POST /api/question-bank/imports`
-> `createQuestionBankImport`
-> `ImportDocument`
-> `parser.DocumentParser.Parse`
-> `buildImportChunks`
-> `llm.CallWithSchema`
-> `ImportStore.AddItems`
-> commit 同上

## Data Flow

`uploaded file`
-> `question_bank_import_jobs`
-> `question_bank_import_chunks`（文档路径）
-> `question_bank_import_items`
-> `question_bank`

LLM 输出和用户上传内容都不会直接写正式题库。

## Dependencies

- 复用现有 `parser.DocumentParser`。
- 复用现有 `llm.ChatModel` / schema 校验工具。
- PG 依赖新增 migration 004。

## Tests

- `internal/migrations`：检查 004 up/down 表和索引。
- `internal/questionbank`：验证 staging-before-commit、只提交 valid items。
- `internal/httpapi`：验证本地 JSON 上传和 commit API。

## Risks

- 文档生成路径当前是同步请求，长文档会占用 HTTP 请求时间；下一阶段应改成后台 job。
- commit 后 embedding 仍依赖后续 reindex，不是异步 embedding 队列。
- 前端 `npm test` 当前被 Vitest suite 初始化错误挡住，`typecheck` 和 `build` 已通过。
