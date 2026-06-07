# 06-07 生成题去重与提交门禁

## 1. 变更概述

本次变更为 RAG 题库生成和导入提交增加保守的题干去重门禁，避免 LLM 生成题与正式 active 题库或同一导入 job 内已有题重复后继续进入正式 `question_bank`。

去重规则是 exact-normalized content key：只对题干做 trim、lowercase、空白压缩/移除，不做语义相似去重，不改数据库 schema，不改前端。

影响范围：`internal/questionbank` 的生成质量门禁、生成服务、导入 commit 路径和相关测试。

## 2. 变更文件

- `internal/questionbank/generation_quality.go`
  - 新增共享题干去重 key helper。
  - 扩展候选题质量门禁，识别正式题库已有 active 题干重复。
- `internal/questionbank/generation_service.go`
  - 生成进入 gating 前读取正式 active 题库内容 key。
  - 读取失败时降级为 warning，不阻断生成。
- `internal/questionbank/imports_commit.go`
  - commit 前再次读取 active 题库内容 key。
  - 拦截同一 job 内重复题和正式题库已有重复题。
  - 为被拦截 item 写入 agent review reason。
- `internal/questionbank/generation_test.go`
  - 增加 normalized key、existing active duplicate、GenerationService duplicate rejection 测试。
  - 更新 `gateQuestionCandidates` 调用签名。
- `internal/questionbank/imports_test.go`
  - 增加 commit 同 job 重复、existing active 重复、active key 读取失败测试。
  - 调整旧 ID 唯一性测试数据，使其不再用重复题干误测导入数量。

## 3. 函数级说明

### `questionContentDedupeKey`

位置：`internal/questionbank/generation_quality.go`

作用：把题干转换为保守的去重 key。输入是原始题干字符串，输出是 lowercase、trim、移除空白后的 key。空字符串输入会返回空 key。无外部副作用。

行为变化：原有 `normalizeCandidateContent` 改为调用该 helper，保持旧同批重复检测语义不变。

### `normalizeCandidateContent`

位置：`internal/questionbank/generation_quality.go`

作用：兼容 wrapper。输入题干字符串，输出 `questionContentDedupeKey`。保留该函数是为了不扩大改动范围。

### `gateQuestionCandidates`

位置：`internal/questionbank/generation_quality.go`

作用：候选题质量门禁。新增 `existingContentKeys map[string]struct{}` 参数。输出仍是 passed/rejected 两组候选题。

行为变化：除了原有必填字段、来源引用、同批重复等检查外，现在候选题 normalized content key 命中 existing active key 时，会进入 rejected，并追加 `duplicate_existing_content` 到 `QualityFlags`。

### `candidateQualityFlags`

位置：`internal/questionbank/generation_quality.go`

作用：为单个候选题生成质量问题列表。新增 existing content key 输入。

错误处理：无外部错误返回；nil map 读取安全。主要副作用是调用者会把返回 flags 写入 candidate。

### `GenerationService.Generate`

位置：`internal/questionbank/generation_service.go`

作用：生成题目完整流程。生成 drafts 后、quality gate 前，调用 `activeQuestionContentKeys(ctx, s.writer)` 读取正式 active 题库 key。

行为变化：如果读取 active key 成功，generation gate 会拦截与正式题库重复的候选题。如果读取失败，写入 warning `existing question dedupe skipped: ...` 并继续执行同批去重，避免生成流程因只读诊断失败直接不可用。

### `ImportService.commitReadyJob`

位置：`internal/questionbank/imports_commit.go`

作用：把 ready import job 的 accepted items 写入正式题库。

行为变化：

- commit 前调用 `activeQuestionContentKeys(ctx, s.writer)` 读取当前 active 题库 key。
- 对 `valid && importItemAccepted(item)` 的 item 做同 job `seenContentKeys` 去重。
- 与 active 题库重复时，不写入 `writer.Upsert`，保留 `Status=valid`，写入 `AgentReviewStatus=rejected` 和 `AgentReviewReason=duplicate_existing_content`。
- 同 job 后续重复时，不写入 `writer.Upsert`，保留 `Status=valid`，写入 `AgentReviewReason=duplicate_content`。
- `ImportedItems` 现在等于实际写入正式题库的数量。

错误处理：读取 active key 失败会调用 `failJob`，不继续提交，避免最后防线失效后静默写入重复题。

### `activeQuestionContentKeys`

位置：`internal/questionbank/imports_commit.go`

作用：从实现了 `Store` 的 source 中分页读取 active 题库，返回 normalized content key 集合。

输入：`context.Context` 和任意 source。source 不实现 `Store` 时返回 nil map 和 nil error，表示无法做跨正式题库去重但不影响同 job 去重。

错误处理：`Store.List` 失败时返回错误；分页 cursor 不推进时返回错误，避免异常 store 造成死循环。生成阶段将错误降级为 warning；commit 阶段将错误视为失败。

## 4. 调用链

### 生成阶段

用户或 API 触发题目生成
→ `GenerationService.Generate`
→ `retrieveGenerationChunks`
→ `extractConceptCards`
→ `generateQuestionCandidates`
→ `activeQuestionContentKeys`
→ `gateQuestionCandidates`
→ `candidateQualityFlags`
→ `questionContentDedupeKey`
→ `GenerationJob.Candidates` / `GenerationJob.RejectedCandidates`

### 提交阶段

用户审核后提交 import job
→ `ImportService.Commit`
→ `ImportService.commitReadyJob`
→ `activeQuestionContentKeys`
→ 遍历 `ImportItem`
→ `questionContentDedupeKey`
→ `writer.Upsert`
→ `embedCommittedItems`
→ `imports.UpdateItems`
→ `imports.UpdateJob`

## 5. 数据流

生成阶段数据来源是 LLM candidate 和正式 active 题库列表。candidate content 被转换为 normalized key，与同批 `seenContent` 和正式 active key 对比。重复候选不会进入 `Candidates`，而进入 `RejectedCandidates` 并保留 `QualityFlags`。

提交阶段数据来源是 import store 中的 staged items 和 writer/store 中的 active question bank。只处理 `Status=valid` 且审核策略允许的 item。重复 item 不写入正式题库，只更新 import item 的 agent review metadata；非重复 item 写入 `question_bank`，再按原流程写 embedding。

## 6. 依赖与副作用

- 没有新增第三方依赖。
- 没有数据库 schema 变更。
- 没有前端文件变更。
- 对 PostgreSQL/Memory store 的读取通过现有 `Store.List` 接口完成。
- commit 阶段会额外读取 active 题库，题库很大时有分页读取成本。
- 被拦截的 duplicate import item 会更新 `AgentReviewStatus`、`AgentReviewReason`、`UpdatedAt`。
- source 不实现 `Store` 时，跨正式题库去重不可用，但同 job 去重仍有效。

## 7. 测试

新增或修改测试覆盖：

- `TestQuestionContentDedupeKeyNormalizesWhitespaceAndCase`
- `TestQuestionContentDedupeKeyDoesNotMergeDistinctQuestions`
- `TestGateQuestionCandidatesRejectsExistingActiveDuplicateContent`
- `TestGenerationServiceGenerateRejectsExistingActiveDuplicateContent`
- `TestImportCommitSkipsDuplicateContentInSameJob`
- `TestImportCommitSkipsDuplicateExistingActiveContent`
- `TestImportCommitSkipsFailJobWhenActiveContentReadFails`
- `TestActiveQuestionContentKeysRejectsStuckCursor`
- `TestDocumentImportStagesUniqueGeneratedIDsAcrossChunks`

已执行：

```powershell
go test ./internal/questionbank -count=1
```

结果：通过。

## 8. 风险

- 归一化规则仍然会把仅空白和大小写不同的题视为同题，这是预期行为；不会做语义相似误杀。
- 如果运行时 writer 不实现 `Store`，commit 无法读取正式 active 题库，只能执行同 job 去重。
- commit 阶段分页读取 active 题库会增加一次只读成本；当前优先保证不写入重复题，并对异常不推进 cursor 做失败保护。
- 被 duplicate 拦截的 import item 保留 `valid` 状态，依赖 `AgentReviewStatus=rejected` 和 reason 解释未提交原因；前端若不展示该字段，需要后续 UI 对接。
