# 05-28 question bank import recovery

## Overview

新增题库导入 worker recovery。服务启动后会扫描未完成的 import job，并按状态恢复执行：

- `queued/parsing/generating/validating`：从 spool payload 重新执行导入。
- `committing`：重跑 commit，依赖 question_bank upsert 保持幂等。

## Files

- `internal/questionbank/imports.go`
- `internal/questionbank/imports_pg.go`
- `internal/questionbank/imports_test.go`
- `cmd/server/main.go`

## Function Notes

- `ImportService.RecoverPendingJobs`：扫描 `ImportStore.ListJobs`，筛出可恢复状态并重新调度后台任务。
- `ImportService.resumeImportJob`：清理旧 staging 数据和计数，再从 spool 重新解析。
- `ImportStore.ResetJobData`：删除指定 job 的 chunks/items，避免恢复时重复累计。
- `cmd/server`：启动后异步触发恢复扫描，失败只记录日志，不阻塞 HTTP 服务。

## Call Chain

服务启动：

`main`
-> `buildQuestionBankImportService`
-> `RecoverPendingJobs`
-> `resumeImportJob` / `commitReadyJob`
-> staging / question_bank

## Data Flow

导入恢复：

`job.metadata.spool_path`
-> `ImportSpool.Open`
-> `ResetJobData`
-> `processLocalQuestionBank` / `processDocument`
-> `ready`

提交恢复：

`committing job`
-> `commitReadyJob`
-> `question_bank upsert`
-> `embedding`
-> `committed`

## Tests

- `RecoverPendingQueuedImport` 覆盖 queued job 从 spool 恢复为 ready。
- `RecoverPendingCommit` 覆盖 committing job 恢复为 committed。

## Risks

- 当前 recovery 由每个实例启动时触发；多实例同时启动时可能重复调度同一 job。commit 侧 upsert 是幂等的，导入侧通过 reset 降低重复风险。
- 下一阶段应给 import job 加 lease/owner 字段，复用 session coordinator 的租约思想，避免多实例重复恢复。
