# 05-28 question bank async import

## Overview

新增题库导入后台执行路径。同步 API 保留，新增 `async=true` 时只读取上传文件、创建 `queued` job，然后由后台 goroutine 执行解析、文档生成、暂存或 commit。

## Files

- `internal/questionbank/imports.go`
- `internal/questionbank/imports_test.go`
- `internal/httpapi/question_bank.go`
- `internal/httpapi/question_bank_test.go`
- `migrations/004_question_bank_imports.up.sql`
- `web/src/apiClient.ts`
- `web/src/main.tsx`

## Function Notes

- `ImportService.EnqueueImport`：复制上传内容，创建 `queued` job，后台执行导入。
- `ImportService.EnqueueCommit`：把 ready job 标记为 `queued`，后台执行 commit + embedding。
- `ImportService.runAsync`：用固定 worker semaphore 限制并发，避免上传高峰把进程打满。
- `createQuestionBankImport`：`async=true` 返回 HTTP 202。
- `commitQuestionBankImport`：`async=true` 返回 queued job。

## Call Chain

异步导入：

`POST /api/question-bank/imports?async=true`
-> `createQuestionBankImport`
-> `ImportService.EnqueueImport`
-> background `processLocalQuestionBank` / `processDocument`
-> staging items

异步提交：

`POST /api/question-bank/imports/:id/commit?async=true`
-> `commitQuestionBankImport`
-> `ImportService.EnqueueCommit`
-> background `commitReadyJob`
-> `question_bank`
-> embedding

## Data Flow

上传请求只负责：

`multipart file -> memory copy -> import job queued`

后台负责：

`queued job -> parse/generate/validate -> ready`

提交后台负责：

`ready -> queued -> committing -> committed`

## Dependencies

- 复用现有 `ImportStore`、`Writer`、`EmbeddingWriter`。
- 没引入外部队列；当前是单进程后台 worker。

## Tests

- `internal/questionbank`：覆盖 `EnqueueImport` 和 `EnqueueCommit` 后台状态推进。
- `internal/httpapi`：覆盖 `async=true` 返回 202 和 queued job。

## Risks

- 当前异步任务的上传文件 payload 只保存在进程内存里，进程重启会丢失 queued 任务上下文。
- 下一阶段应把 upload payload 落到 durable spool（本地文件/对象存储/PG bytea 分块）并支持实例接管。
