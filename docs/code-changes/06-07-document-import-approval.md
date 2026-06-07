# 06-07 Document Import Approval

## 1. 变更概述

修复源文档 / Markdown 生成题库的审核状态断点：文档生成题默认进入 `needs_human_review`，人工接受后现在会推进为 `auto_approved`，从而允许后续 commit 写入正式 `question_bank`；人工拒绝后会推进为 `rejected`，继续阻止入库。

影响范围限定在后端题库导入暂存审核和提交逻辑；不修改 HTTP API、数据库 schema、前端、embedding 配置或正式题库表结构。

## 2. 变更文件

- `internal/questionbank/imports_commit.go`
  - 新增人工审核后 Agent 审核状态推进函数。
- `internal/questionbank/imports_memory_store.go`
  - 内存导入 store 更新 review 时同步推进 `AgentReviewStatus`。
- `internal/questionbank/imports_pg.go`
  - PG 导入 store 更新 review 时同步更新 `field_provenance` 中保存的 Agent 审核元数据。
- `internal/questionbank/imports_test.go`
  - 增加文档生成题接受后可 commit、拒绝后不可 commit 的回归测试。

## 3. 函数级说明

### `agentReviewStatusAfterHumanReview`

位置：`internal/questionbank/imports_commit.go`

作用：根据人工审核动作推进 Agent 审核状态。

输入：当前 `AgentReviewStatus`、人工 `reviewStatus`。

输出：新的 Agent 审核状态。

副作用：无。

错误处理：无错误返回；非法 review status 通过 `normalizedImportReviewStatus` 归一为 accepted。

主要逻辑：人工接受时，如果当前存在非 `auto_approved` 的 Agent 状态，则推进为 `auto_approved`；人工拒绝时，如果当前存在 Agent 状态，则推进为 `rejected`；结构化导入空状态保持为空。

### `MemoryImportStore.UpdateItemReviews`

位置：`internal/questionbank/imports_memory_store.go`

作用：更新暂存题目的人工审核状态。

输入：`jobID`、item IDs、review status。

输出：error。

副作用：修改内存 store 中有效 item 的 `ReviewStatus`、`AgentReviewStatus` 和 `UpdatedAt`。

错误处理：当前内存实现不返回业务错误；无匹配 item 时保持幂等。

行为变化：人工接受 / 拒绝时同步推进 `AgentReviewStatus`，使内存模式与 PG 模式一致。

### `PGImportStore.UpdateItemReviews`

位置：`internal/questionbank/imports_pg.go`

作用：更新 PG 暂存题目的人工审核状态。

输入：`jobID`、item IDs、review status。

输出：error。

副作用：更新 `question_bank_import_items.review_status`、`field_provenance` 中的 `__agent_review_status` 和 `updated_at`。

错误处理：PG 执行失败时包装为 `update question bank import item reviews` 错误。

行为变化：如果暂存项已有 Agent 审核元数据，人工接受会写为 `auto_approved`，人工拒绝会写为 `rejected`。没有 Agent 审核元数据的结构化导入项不新增该元数据，保持兼容。

### `TestReviewAcceptsDocumentGeneratedItemForCommit`

位置：`internal/questionbank/imports_test.go`

作用：覆盖文档生成题默认需要人工审核，人工接受后可提交入正式题库。

### `TestReviewRejectsDocumentGeneratedItemForCommit`

位置：`internal/questionbank/imports_test.go`

作用：覆盖文档生成题人工拒绝后仍不可提交入正式题库。

### `completeImportTestItem`

位置：`internal/questionbank/imports_test.go`

作用：构造字段完整的测试题目，避免测试失败来自导入字段校验。

## 4. 调用链

### 人工接受文档生成题

`POST /api/question-bank/imports/:id/items/review`
-> `httpapi.reviewQuestionBankImportItems`
-> `ImportService.ReviewItems` 或 `ReviewAllValidItems`
-> `ImportStore.UpdateItemReviews`
-> `agentReviewStatusAfterHumanReview`
-> `ImportService.Commit`
-> `importItemAccepted`
-> `Writer.Upsert`
-> 可选 `embedCommittedItems`

### 人工拒绝文档生成题

`POST /api/question-bank/imports/:id/items/review`
-> `httpapi.reviewQuestionBankImportItems`
-> `ImportStore.UpdateItemReviews`
-> `agentReviewStatusAfterHumanReview`
-> `ImportService.Commit`
-> `importItemAccepted` 返回 false
-> 不写入正式 `question_bank`

## 5. 数据流

文档导入生成题先进入 `question_bank_import_items`。Agent 审核状态在 PG 中打包到 `field_provenance.__agent_review_status`，读取时由 `unpackImportItemMetadata` 还原到 `ImportItem.AgentReviewStatus`。

本次修复让人工 review 动作同时更新：

- `review_status`
- `__agent_review_status` / `AgentReviewStatus`
- `updated_at`

正式提交仍只读取暂存项，不新增旁路。

## 6. 依赖与副作用

- 不新增依赖。
- 不新增迁移。
- 不写前端。
- PG 更新使用现有 `field_provenance jsonb` 元数据列。
- embedding 写入链路不变，仍在 commit 后由已有 `embedCommittedItems` 执行。

## 7. 测试

已执行：

```powershell
go test ./internal/questionbank -run "TestReviewAcceptsDocumentGeneratedItemForCommit|TestReviewRejectsDocumentGeneratedItemForCommit" -count=1
go test ./internal/questionbank -count=1
```

结果：全部通过。

## 8. 风险

- 兼容性：HTTP API 和 DB schema 不变；结构化导入空 Agent 状态不会被强行写入 Agent 元数据。
- 业务语义：人工接受会覆盖已有 `rejected` / `needs_human_review` Agent 状态为 `auto_approved`，这是为了支持审核反悔和重新发布。
- PG 覆盖范围：只更新 `status='valid'` 的暂存项，invalid item 仍不能通过审核动作发布。
