# 05-28 question bank import spool

## Overview

异步题库导入不再把上传 payload 只放在内存里。`EnqueueImport` 创建 job 后会把上传文件写入本地 durable spool，并把 `spool_path/content_type/size` 写入 job metadata。后台 worker 再从 spool 重新打开文件执行解析或文档生成。

## Files

- `internal/questionbank/imports.go`
- `internal/questionbank/imports_test.go`
- `internal/config/config.go`
- `cmd/server/main.go`

## Function Notes

- `ImportSpool`：上传 payload 的保存、打开、删除接口。
- `LocalImportSpool.Save`：写入 root 下的临时文件，成功后原子 rename 为 `{job_id}.upload`。
- `LocalImportSpool.Open`：只允许打开 spool root 内路径，防止 metadata 路径逃逸。
- `LocalImportSpool.Delete`：后台导入完成后删除 payload。
- `ImportService.EnqueueImport`：保存 spool 后再标记 job 为 `queued`。

## Call Chain

`POST /api/question-bank/imports?async=true`
-> `ImportService.EnqueueImport`
-> `ImportSpool.Save`
-> `job.metadata.spool_path`
-> background `ImportSpool.Open`
-> `processLocalQuestionBank` / `processDocument`
-> `ImportSpool.Delete`

## Data Flow

`multipart file`
-> `data/import-spool/{job_id}.upload`
-> `job.metadata`
-> background worker
-> staging items
-> delete spool payload

## Dependencies

- 默认 spool 目录：`data/import-spool`
- 环境变量覆盖：`INTERVIEW_IMPORT_SPOOL_DIR`

## Tests

- `LocalImportSpool_SaveOpenDelete` 覆盖保存、读取、删除。
- `EnqueueImportRunsInBackground` 覆盖 queued job 记录 spool path，并在导入完成后清理 payload。

## Risks

- 现在 payload 已 durable，但服务启动时还不会扫描 `queued/parsing` job 自动恢复。
- 下一阶段应实现 recover loop：启动时扫描未完成 job，用 spool payload 继续执行。
