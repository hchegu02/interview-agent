# 05-28 题库范围影响面试

## 1. 变更概述

本次变更把准备阶段选择的题库范围接入面试启动与 RAG 检索链路。用户可在 JD 页面选择技能、场景、难度和标签范围；前端把范围保存到草稿并随 `/api/interview/start` 发送；后端把范围保存到 `domain.Session.QuestionBankFilter`；`retrieve_rag` 将范围传给 retriever；PG retriever 在 HNSW/tag 两路候选召回 SQL 中增加硬过滤条件。

影响范围：前端 JD 准备页、草稿 localStorage、面试启动 API、Session JSON 状态、RAG 检索 Query 和 pgvector SQL。

## 2. 变更文件

- `internal/domain/session.go`：新增 `QuestionBankFilter` 领域结构，并挂到 `Session`。
- `internal/httpapi/interview.go`：`startInterviewRequest` 支持 `question_bank_filter`，`InterviewService.Start` 写入 Session。
- `internal/nodes/retrieve_rag.go`：将 Session 里的题库范围传给 `retriever.Query`；fallback 题库也按可用字段过滤。
- `internal/retriever/retriever.go`：`Query` 增加硬过滤字段。
- `internal/retriever/pgvector.go`：SQL 两路候选召回增加 skill/scenario/difficulty/tag 硬过滤。
- `web/src/types.ts`：新增 `QuestionBankFilter` 类型，Draft 支持保存范围。
- `web/src/draftStore.ts`：加载、规范化和摘要展示题库范围。
- `web/src/apiClient.ts`：`startInterview` payload 支持 `question_bank_filter`。
- `web/src/main.tsx`：JD 页面新增题库范围控件，启动面试时带上范围。
- `web/src/styles.css`：新增 `.scope-panel` 样式。
- 测试文件：`internal/httpapi/interview_test.go`、`internal/nodes/setup_test.go`、`internal/retriever/pgvector_test.go`、`web/src/draftStore.test.ts`。

## 3. 函数级说明

- `domain.QuestionBankFilter`：描述一次面试允许使用的题库范围。空字段表示不限制；数组字段表示命中任意值即可。
- `InterviewService.Start`：从 `startInterviewRequest.QuestionBankFilter` 克隆规范化后的范围到 `Session.QuestionBankFilter`。不改变旧请求；不传该字段时行为保持原样。
- `cloneQuestionBankFilter`：复制并清理空字符串、非法难度和反向难度区间；空过滤返回 `nil`。
- `compactInterviewStrings`：去掉空白字符串，避免无意义过滤进入 Session。
- `normalizeScopeDifficulty`：只接受 1 到 5 的难度，其余值按未设置处理。
- `NewRetrieveRAGNode` 返回的节点函数：构造 `retriever.Query` 时追加技能、场景、难度和标签硬过滤。
- `filterSkillCategories` / `filterScenarios` / `filterDifficultyMin` / `filterDifficultyMax` / `filterTags`：从可空 `QuestionBankFilter` 安全取值，并复制 slice，避免下游修改 Session 内部数据。
- `cloneFallback`：在 RAG 降级时先按目标难度和可用的 scope 字段筛选 fallback 题；如果过滤后为空，则退回难度过滤结果，避免会话无题可问。
- `matchesFallbackFilter`：fallback 的硬过滤实现。当前 fallback 题没有 scenario 字段，因此只应用 skill、difficulty、tags。
- `containsAnyString`：大小写不敏感匹配字符串集合，用于 fallback skill/tag 判断。
- `retriever.Query`：新增 `SkillCategories`、`Scenarios`、`DifficultyMin`、`DifficultyMax`、`FilterTags`。
- `PGVectorRetriever.Retrieve`：规范化硬过滤参数，执行 `retrieveSQL` 时追加 `$6` 到 `$10`。
- `compactQueryStrings` / `normalizeHardDifficultyRange` / `normalizeHardDifficulty`：PG 查询参数清理，避免空字符串和非法难度造成错误过滤。
- `loadDraft`：读取草稿时规范化 `question_bank_filter`。
- `saveDraft`：继续支持按 localStorage 合并 patch 的旧调用方式。
- `buildDraft`：按当前内存 draft 合并 patch，避免用户直接进入 JD 页时，选择 scope 把默认 JD/简历覆盖为空。
- `draftScopeSummary`：生成 JD 页的题库范围摘要文本。
- `normalizeQuestionBankFilter`：前端过滤结构规范化，去重、裁剪、交换反向难度区间，空过滤返回 `undefined`。
- `compact` / `normalizeDifficulty`：前端过滤辅助函数。
- `JDPage`：加载题库 facets，显示范围选择控件，更新草稿中的 `question_bank_filter`。
- `App.startInterview`：调用 `apiClient.startInterview` 时带上规范化后的 `question_bank_filter`。

## 4. 调用链

用户操作链路：

`JDPage` 选择题库范围 -> `updateDraft` -> `buildDraft` -> `localStorage(interview_agent_draft_v1)` -> 点击“开始面试” -> `App.startInterview` -> `apiClient.startInterview` -> `POST /api/interview/start` -> `Server.startInterview` -> `InterviewService.Start` -> `domain.Session.QuestionBankFilter` -> Graph `retrieve_rag` -> `retriever.Query` -> `PGVectorRetriever.Retrieve` -> `retrieveSQL` 硬过滤候选题。

降级链路：

`retrieve_rag` -> embedder/retriever 失败或空结果 -> `cloneFallback(targetDiff, sess.QuestionBankFilter)` -> `matchesFallbackFilter` -> `sess.CandidatePool`。

## 5. 数据流

来源：用户在 JD 页面选择 skill/scenario/difficulty/tag，或草稿从 localStorage 恢复。

校验与转换：前端 `normalizeQuestionBankFilter` 去空、去重、限制难度 1-5。后端 `cloneQuestionBankFilter` 再做一次清理，保证直接调用 API 也不会把坏 scope 写入 Session。

存储：范围存入 `Draft.question_bank_filter` 和 `Session.question_bank_filter`。PG session store、Redis snapshot 使用 Session JSON，因此自动包含该字段。

传递：`Session.QuestionBankFilter` 在 `retrieve_rag` 转换为 `retriever.Query` 的硬过滤字段。

返回：当前 HTTP response 不额外展示 scope；面试出题结果通过候选池和题目体现范围影响。

## 6. 依赖与副作用

- 前端新增一次 `apiClient.questionFacets()` 调用，用于 JD 页范围选择项。
- PG SQL 继续使用现有 `question_bank` 表和已有 `skill_category`、`difficulty`、`tags` 索引；新增 scenario 条件会利用已有 scenario 索引。
- 不新增数据库 migration。
- 不改旧 start payload；未传 `question_bank_filter` 时旧客户端兼容。
- fallback 过滤只应用 fallback 题实际拥有的字段；scenario 在 fallback 中不生效。

## 7. 测试

新增或修改测试：

- `TestInterviewService_StartStoresQuestionBankFilter`
- `TestRetrieveRAG_PassesQuestionBankFilterToRetriever`
- `TestRetrieveRAG_FallbackHonorsQuestionBankFilter`
- `TestRetrieveSQLIncludesQuestionBankHardFilters`
- `draftStore helpers > summarizes question bank scope for the JD page`
- `draftStore helpers > merges scope changes with the current in-memory draft`

已执行 focused 验证：

- `go test ./internal/httpapi -run TestInterviewService_StartStoresQuestionBankFilter -count=1`
- `go test ./internal/nodes -run TestRetrieveRAG_PassesQuestionBankFilterToRetriever -count=1`
- `go test ./internal/nodes -run TestRetrieveRAG_FallbackHonorsQuestionBankFilter -count=1`
- `go test ./internal/retriever -run TestRetrieveSQLIncludesQuestionBankHardFilters -count=1`
- `npm run test -- draftStore.test.ts`
- `npm run typecheck`
- `npm run test`
- `npm run build`
- `go test ./internal/httpapi ./internal/nodes ./internal/retriever -count=1`
- `go test ./... -count=1`
- `git diff --check`

浏览器验收：

- 使用临时 `INTERVIEW_LLM_MODE=mock`、`INTERVIEW_EMBEDDING_MODE=mock`、`INTERVIEW_SERVER_ADDR=:19081` 服务。
- 在真实 headless Edge 中打开 `/jd`，选择 `redis` + 难度 `3`。
- 确认 `localStorage.interview_agent_draft_v1` 保留默认 JD/简历且包含 `question_bank_filter`。
- 点击“开始面试”，捕获 start payload：`question_bank_filter.skill_categories=["redis"]`、`difficulty_min=3`、`difficulty_max=3`。
- 进入 `/interview` 后第一题为 Redis fallback 题：`Redis 的 AOF 和 RDB 持久化方式各自的取舍？`。

## 8. 风险

- PG SQL 使用可选条件会让 HNSW 候选集更贴范围，但过滤过窄可能导致 0 结果；`retrieve_rag` 已保留 fallback。
- fallback 题没有 scenario 字段，因此 scenario 过滤只在真实题库/PG 检索中完整生效。
- JD 页当前每类只支持单选，数据结构支持多选，后续可扩展 UI 而不改后端契约。
- 如果用户选择极窄范围且真实题库缺题，最终可能退到 fallback；报告中会保留 rag 降级原因。
