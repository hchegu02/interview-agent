# 05-28 question bank import lease

## Overview

为题库导入 job 增加 `owner_id` 和 `lease_until`，避免多实例同时恢复同一个导入任务。恢复或后台执行前必须先抢 lease，抢不到就跳过。

## Files

- `migrations/006_question_bank_import_lease.*.sql`
- `internal/questionbank/imports.go`
- `internal/questionbank/imports_pg.go`
- `internal/questionbank/imports_test.go`
- `cmd/server/main.go`

## Function Notes

- `ImportStore.TryAcquireJob`：原子获取 job lease。
- `MemoryImportStore.TryAcquireJob`：测试和无 PG 模式的租约实现。
- `PGImportStore.TryAcquireJob`：使用条件 `UPDATE ... WHERE owner_id = '' OR lease_until < now()` 抢占。
- `ImportService.RecoverPendingJobs`：只有抢到 lease 的实例会调度恢复。
- `cmd/server`：使用 `hostnameOwnerID()` 作为 import worker owner。

## Call Chain

恢复：

`RecoverPendingJobs`
-> `TryAcquireJob`
-> `resumeImportJob` / `commitReadyJob`

异步执行：

`EnqueueImport` / `EnqueueCommit`
-> background worker
-> `TryAcquireJob`
-> process

## Data Flow

`question_bank_import_jobs`
-> `owner_id`
-> `lease_until`
-> single owner executes job

## Tests

- `RecoverPendingJobsUsesLease` 覆盖两个 service 共享同一个 store 时，只有一个 worker 能恢复 job。
- migration 测试覆盖 006 lease 字段和索引。

## Risks

- 当前 lease 不续约。单个导入任务如果超过默认 2 分钟，另一个实例可能接管。
- 下一阶段应加入 heartbeat/lease refresh，或把长任务拆成更小阶段。
