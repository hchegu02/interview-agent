# 06-07 文档导入生成题 ID 去重

## 1. 变更概述

修复 Markdown / 文档导入时生成题 ID 不可信的问题。真实验证中，mock LLM 对每个 chunk 都返回 `generated-go-001`，导致 PostgreSQL 暂存表按 `question_bank_import_items.id` 冲突覆盖，后续批量审核和 commit 看起来成功但实际导入为 0 或数量不对。

本次只修改后端题库导入 staging、PG 入库边界和文档导入状态流：文档生成题在进入暂存区前，由后端基于 `jobID + chunkID + chunk 内序号 + 原始 ID + content` 生成稳定的 `docq-*` 题目 ID；PG store 写入 `question_bank_import_items.errors` 时把 nil 规范为空数组，避免有效题写入 NOT NULL `text[]` 字段时变成 SQL NULL；文档导入只有所有 chunks 都处理完成后才把 job 标记为 `ready`，避免异步轮询提前审核/提交。本地 JSON 题库导入不受影响，仍保留上传文件中的 ID。

## 2. 变更文件

- `internal/questionbank/imports_stage.go`
  - 文档导入 staging 时重写生成题 ID。
  - 文档 chunk 中间 staging 不再把 job 标记为 `ready`。
  - 新增 `documentGeneratedQuestionID`。
- `internal/questionbank/imports.go`
  - `processDocument` 在所有 chunks 处理完成后统一将 job 标记为 `ready`。
- `internal/questionbank/imports_pg.go`
  - 新增 `pgImportItemErrorsForDB`。
  - `AddItems` / `UpdateItems` 写 PG 时使用空数组表示无错误。
- `internal/questionbank/imports_test.go`
  - 调整文档导入 accept/reject 测试，使用后端 staging 后的题目 ID。
  - 新增跨 chunk 重复生成 ID 的回归测试。
  - 新增文档 chunk 中间 staging 不暴露 `ready` 的回归测试。
  - 新增 PG import item errors 入库规范化测试。

## 3. 函数级说明

### `stageItemsWithOriginalsAndProvenance`

位置：`internal/questionbank/imports_stage.go`

作用：把解析或生成出的 `Item` 转换为 `ImportItem` 并写入导入暂存区。

输入：`ctx`、`ImportJob`、`chunkID`、待导入 `items`、可选原始题目和来源标记。

输出：更新后的 `ImportJob` 和错误。

行为变化：当 `job.SourceType == ImportSourceDocument` 时，不再信任 LLM 返回的 `item.ID`，而是调用 `documentGeneratedQuestionID` 生成唯一题目 ID。随后 `ImportItem.ID = job.ID + ":" + item.ID` 也会随之唯一，避免 PG `ON CONFLICT(id)` 覆盖。

副作用：写入 `question_bank_import_items` 的 `question_id` 和 `item_json.id` 会从 LLM 原始 ID 变为后端生成的 `docq-*`。这只影响文档生成题，不影响上传题库文件。

错误处理：沿用原逻辑，校验失败进入 invalid item，存储失败调用 `failJob`。

### `documentGeneratedQuestionID`

位置：`internal/questionbank/imports_stage.go`

作用：为文档生成题生成稳定、可重复、跨 chunk 唯一的题目 ID。

输入：`jobID`、`chunkID`、chunk 内序号、标准化后的 `Item`。

输出：`docq-*` 格式字符串。

主要逻辑：将 `jobID`、`chunkID`、序号、原始 `item.ID` 和题干内容拼接后交给 `importGeneratedID` 做 SHA1 短 hash。没有引入当前时间，保证同一 job 重跑 staging 时结果稳定。

副作用：无。

### `PGImportStore.AddItems`

位置：`internal/questionbank/imports_pg.go`

作用：批量写入导入暂存题到 `question_bank_import_items`。

行为变化：写入 SQL 参数 `errors` 时调用 `pgImportItemErrorsForDB`，把 nil slice 转成空 slice，避免 PG 接收到 SQL NULL。

副作用：无 schema 变更；有效题的 `errors` 字段按表定义落为 `{}`。

### `processDocument`

位置：`internal/questionbank/imports.go`

作用：读取文档、解析文本、切 chunk、调用 LLM 生成题、暂存生成题。

行为变化：原来每个 chunk 调用 staging 后都会间接把 job 标记为 `ready`，异步 HTTP 轮询可能提前看到 ready 并触发审核/commit。现在循环结束后才统一设置 `job.Status = ImportStatusReady` 并更新 store。

错误处理：任一 chunk 生成或暂存失败仍调用 `failJob`，不会进入 ready。

副作用：导入中的文档 job 会在 chunk 处理期间保持 `generating`，前端和调用方需要等最终 ready。

### `PGImportStore.UpdateItems`

位置：`internal/questionbank/imports_pg.go`

作用：更新暂存题状态、审核状态、题目 JSON、字段来源和错误列表。

行为变化：和 `AddItems` 一样，写入 `errors` 时使用 `pgImportItemErrorsForDB`，避免 imported 状态更新时把 nil 写成 SQL NULL。

副作用：无 schema 变更。

### `pgImportItemErrorsForDB`

位置：`internal/questionbank/imports_pg.go`

作用：PG 入库边界的 errors 规范化函数。

输入：`[]string`。

输出：非 nil 的 `[]string`。输入 nil 时返回空 slice。

副作用：无。

### `TestDocumentChunkStagingDoesNotMarkJobReadyBeforeProcessCompletes`

位置：`internal/questionbank/imports_test.go`

作用：锁定异步导入状态流，防止文档 chunk 中间 staging 暴露 `ready`。

覆盖行为：`stageItems` 对 `ImportSourceDocument + chunkID` 的状态更新。

### `TestDocumentImportStagesUniqueGeneratedIDsAcrossChunks`

位置：`internal/questionbank/imports_test.go`

作用：复现并锁定真实问题：两个文档 chunk 都生成同一个 `generated-go-001` 时，暂存 ID 和正式题目 ID 仍必须唯一，批量接受完整题后 commit 应导入两道题。

输入：内存 import store 和内存题库 writer。

输出：测试断言。

覆盖行为：`stageItems`、`ReviewAllValidItems(..., completeOnly=true)`、`Commit` 的闭环。

### `TestPGImportItemErrorsForDBUsesEmptyArray`

位置：`internal/questionbank/imports_test.go`

作用：保证 PG import store 不会把 nil `Errors` 写成 SQL NULL。

## 4. 调用链

HTTP 文档导入链路：

`POST /api/question-bank/imports?async=true`
-> question-bank import handler
-> `ImportService.ImportDocument` 或异步 worker 恢复处理
-> `processDocument`
-> `buildImportChunks`
-> `generateItems`
-> `stageItemsWithOriginalsAndProvenance`
-> `documentGeneratedQuestionID`
-> `PGImportStore.AddItems`
-> `pgImportItemErrorsForDB`
-> `processDocument` 全部 chunks 完成后 `UpdateJob(ready)`

批量审核提交链路：

`POST /api/question-bank/imports/{jobID}/items/review`
-> import review handler
-> `ImportService.ReviewAllValidItems`
-> `ImportStore.UpdateItemReviews`
-> `POST /api/question-bank/imports/{jobID}/commit`
-> `ImportService.Commit`
-> `commitReadyJob`
-> `Writer.Upsert`
-> 可选 `EmbeddingWriter.UpsertEmbeddings`

## 5. 数据流

来源：Markdown / 文档内容经 parser 切 chunk 后交给 LLM 生成题目。

转换：LLM 返回的 `Item.ID` 先经 `normalizeImportedItem` 清理，再在文档导入场景被 `documentGeneratedQuestionID` 替换为后端稳定 ID。

存储：暂存表写入唯一 `ImportItem.ID`、`question_id`、`item_json.id`。commit 时正式 `question_bank.id` 使用同一个后端生成 ID。

PG errors：有效题 `Errors == nil` 时在 PG 参数层转换为空数组，满足 `question_bank_import_items.errors text[] NOT NULL DEFAULT '{}'`。

状态：文档导入 chunk 处理中保持 `generating`，全部 chunks 暂存完成后才进入 `ready`。

返回：前端或 API 查询 import items 时看到的是后端生成后的题目 ID。

## 6. 依赖与副作用

新增标准库 import：`fmt`。

无数据库 schema 变更。无前端变更。无外部 API 变更。

兼容性：文档生成题的 ID 格式发生变化，但这类 ID 原本由 LLM 生成，不应作为外部稳定契约。本地题库 JSON 导入保留原 ID。

## 7. 测试

已执行：

```powershell
go test ./internal/questionbank -count=1
```

结果：通过。

## 8. 风险

- 已经导入到正式题库的旧 `generated-*` 行不会自动迁移；本次修复只影响后续导入。
- 同一 job 的同一 chunk、同一序号、同一原始 ID、同一题干会生成同一个 ID，这是刻意的幂等行为。
- 如果未来要求跨 job 对相同题干自动去重，需要另加内容指纹或审核策略；本次不做。
