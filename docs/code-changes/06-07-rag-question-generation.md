# 06-07 RAG 题库生成

## 变更概述

新增后端 MVP：基于已导入文档切片生成证据约束的题库候选题。链路为 source chunks -> scoped retrieval -> concept cards -> QuestionCandidate -> quality gate -> import review staging。生成题不会直接写正式题库，必须进入现有导入审核流程并由人工 accept 后才能 commit。

影响范围：`internal/questionbank` 生成服务、`internal/httpapi` 题库生成 API、`cmd/server` 装配、OpenSpec tasks、后端 SDD。

## 变更文件

- `internal/questionbank/generation_types.go`：扩展 `GenerationJob`，加入 rejected candidates、warnings、staged import job 和时间字段。
- `internal/questionbank/generation_service.go`：新增生成 orchestration、进程内 job store、staging 到 import review flow、generated metadata。
- `internal/questionbank/generation_llm.go`：新增 QuestionCandidate LLM 调用和 JSON schema 解析。
- `internal/questionbank/generation_quality.go`：新增候选题质量门禁。
- `internal/questionbank/generation_test.go`：新增生成、质量门禁、stage 和人工审核提交测试。
- `internal/questionbank/imports.go`：新增 `ImportService.Store()`，让 generation service 复用同一个 import store。
- `internal/httpapi/router.go`：注册 generation job create/get/stage 路由。
- `internal/httpapi/question_bank.go`：新增 generation HTTP handlers。
- `internal/httpapi/server_metrics.go`：新增 generation service setter。
- `internal/httpapi/question_bank_test.go`：新增 HTTP create/get/stage 测试。
- `cmd/server/questionbank_wiring.go`：新增 generation service 装配。
- `cmd/server/main.go`：启动时注入 generation service。
- `docs/SDD-Backend.md`：记录题库生成链路和 MVP 限制。
- `openspec/changes/add-rag-question-generation/tasks.md`：标记任务完成。

## 函数级说明

- `GenerationService.Generate(ctx, req)`：校验请求，按 `source_job_id` 检索 chunks，抽取 concept cards，调用 LLM 生成候选题，执行质量门禁，保存 generation job。输入为 `GenerationRequest`，输出为 `GenerationJob`。错误时 job 状态为 `failed`。
- `GenerationService.Get(ctx, id)`：从进程内 job map 查询生成任务。未找到返回 `ErrImportNotFound`。
- `GenerationService.Stage(ctx, id)`：创建新的 document import job 和对应 generated chunk，把通过门禁的 candidates 转成 import items，写入现有 ImportStore，并把 import job 状态置为 `ready`。副作用是新增 import job/chunk/items。
- `generateQuestionCandidates(ctx, req, concepts, chunks)`：通过 `llm.CallWithSchema` 要求 LLM 输出 `{"candidates":[...]}`，并复用 JSON validator。
- `parseQuestionCandidatesJSON` / `validateQuestionCandidatesJSON`：校验和解析候选题 JSON envelope。
- `gateQuestionCandidates`：按 concept、chunk、source quote、重复内容、低价值题、必填字段、题型约束和难度偏差拆分 passed/rejected。
- `generationCandidatesToImportItems`：将 `QuestionCandidate` 转为正式 import `Item` 草稿。
- `generatedQuestionProvenance`：写入 `generated_question_v1`、generation/source/candidate/concept/source refs 元数据。
- `ImportService.Store()`：返回底层 `ImportStore`，用于 server 装配 generation service。
- `createQuestionBankGenerationJob`：HTTP `POST /api/question-bank/generation-jobs`，创建 generation job。
- `getQuestionBankGenerationJob`：HTTP `GET /api/question-bank/generation-jobs/:id`，查询 generation job。
- `stageQuestionBankGenerationJob`：HTTP `POST /api/question-bank/generation-jobs/:id/stage`，把生成候选题放入 import 审核区。
- `buildQuestionBankGenerationService`：server wiring，复用 import service 的 store、题库 writer 和 LLM model。

## 调用链

- API 创建：`POST /api/question-bank/generation-jobs` -> `Router` -> `createQuestionBankGenerationJob` -> `GenerationService.Generate` -> `retrieveGenerationChunks` -> `extractConceptCards` -> `generateQuestionCandidates` -> `gateQuestionCandidates`。
- API 查询：`GET /api/question-bank/generation-jobs/:id` -> `getQuestionBankGenerationJob` -> `GenerationService.Get`。
- API 暂存：`POST /api/question-bank/generation-jobs/:id/stage` -> `stageQuestionBankGenerationJob` -> `GenerationService.Stage` -> `ImportStore.CreateJob` -> `ImportStore.AddChunks` -> `ImportService.stageItemsWithOriginalsAndProvenance` -> `ImportStore.AddItems` -> `ImportStore.UpdateJob`。
- 人工审核提交：现有 `POST /api/question-bank/imports/:id/items/review` -> `ImportService.ReviewItems`；`POST /api/question-bank/imports/:id/commit` -> `ImportService.Commit` -> `Writer.Upsert`。
- 启动装配：`main` -> `buildQuestionBankImportService` -> `buildQuestionBankGenerationService` -> `Server.SetQuestionBankGenerationService`.

## 数据流

输入 JSON 包含 `source_job_id/topic/question_type/count/difficulty/target_dimension/skill_category/tags`。服务只从指定 `source_job_id` 的 import chunks 检索证据。LLM 输出 concept cards 和 question candidates 后，后端覆盖 concept/candidate ID，并校验 source quote 必须落在 chunk 原文中。通过门禁的题转为 import item，带 `generated_question_v1` provenance，默认 `needs_human_review`。人工 accept 后，现有 commit 链路才写入正式题库。

## 依赖与副作用

- 依赖现有 `ImportStore`、`Writer`、`llm.ChatModel`。
- 不新增数据库 schema。
- 不修改前端。
- generation job 状态是进程内内存，重启后不可查询；已 stage 的 import job/items 仍由 ImportStore 保留。
- stage 会新增 import job、generated import chunk 和 import items。

## 测试

已执行：

```powershell
go test ./internal/questionbank ./internal/httpapi ./cmd/server -run 'Generation|QuestionBankGeneration|QuestionCandidate|QualityGate|GateQuestionCandidates|ParseQuestionCandidates|QuestionBankImport|QuestionBankList|QuestionBankGet|QuestionBankFacets' -count=1
```

结果：通过。

## 风险

- 进程内 generation job store 不适合多实例和重启恢复；后续如果要线上长期使用，应增加 PG generation_jobs 或直接以 import job 作为任务事实源。
- scoped retrieval 目前是轻量词项匹配，适合 MVP，不等同于最终 RAG pipeline。
- LLM 输出质量仍依赖 prompt 和模型；后端质量门禁只能挡明显坏题，不能替代人工审核。
- server wiring 目前会为 import 和 generation 各构建一次 ChatModel，后续可合并以减少 limiter/breaker 状态分裂。
