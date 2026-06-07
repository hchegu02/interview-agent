# 06-08 题干内容质量门禁

## 1. 变更概述

本次变更为题库构建和运行时选题增加确定性的题干内容质量门禁。目标是阻止类似“原始面经追问串 + 笔记备注 + 自我评价”的脏题干进入正式题库或优先展示给候选人。

影响范围是后端题库导入/生成/commit、`pick_next` 选题、题库 lint 和相关文档。不修改前端，不删除或批量更新数据库已有题。

## 2. 变更文件

- `internal/questionbank/content_quality.go`：新增共享题干质量检查器。
- `internal/questionbank/generation_quality.go`：生成候选题质量门禁复用题干质量检查。
- `internal/questionbank/imports_commit.go`：commit 前二次检查题干质量。
- `internal/nodes/pick_next.go`：运行时选题优先跳过高风险脏题。
- `cmd/questionbank-lint/main.go`：lint 报告脏题干，并支持显式 PG active 题扫描。
- `docs/SDD-Backend.md`：更新题库生成、commit 和 pick_next 行为说明。
- `openspec/changes/harden-question-content-quality-gates/*`：新增 OpenSpec change。
- `docs/superpowers/specs/2026-06-08-harden-question-content-quality-gates-design.md`：技术设计。
- `docs/superpowers/plans/2026-06-08-harden-question-content-quality-gates.md`：实现计划。

## 3. 函数级说明

### `EvaluateQuestionContentQuality`

位置：`internal/questionbank/content_quality.go`

输入题干文本，输出 `ContentQualityResult`，包含稳定 flags、`HighRiskFlags`、`AdvisoryFlags` 和 `HighRisk`。当前规则覆盖：

- `dirty_note_marker`
- `multiple_question_chain`
- `content_too_long`
- `answer_or_comment_leak`
- `low_value_question`

副作用：无。错误处理：无外部错误，空文本由上游字段完整性检查处理。

### `HasHighRiskQuestionContent`

位置：`internal/questionbank/content_quality.go`

用于运行时快速判断题干是否高风险。内部调用 `EvaluateQuestionContentQuality`。

### `candidateQualityFlags`

位置：`internal/questionbank/generation_quality.go`

行为变化：在原有字段、来源、重复、题型检查基础上追加题干质量 flags。命中高风险题干时，该候选进入 rejected 列表。

### `commitReadyJob`

位置：`internal/questionbank/imports_commit.go`

行为变化：accepted item 在写入正式 `question_bank` 前会再次执行题干质量检查。高风险 item 会被标记为 `agent_review_status=rejected`，`agent_review_reason` 写入 flags，不执行 `writer.Upsert`。

### `filterDirtyQuestionContent`

位置：`internal/nodes/pick_next.go`

输入候选池，返回干净候选和脏题数量。`NewPickNextPatchNode` 在 LLM/rule 选题前调用它。若存在干净候选，则脏题不会进入 LLM prompt；若全是脏题，则保留候选并写 degraded reason。

### `loadLintItems`

位置：`cmd/questionbank-lint/main.go`

行为变化：默认仍读取 seed 文件；显式传 `-postgres-dsn` 时扫描 PG `question_bank` active 题。只读查询，不修改数据库。

### `lintItems`

位置：`cmd/questionbank-lint/main.go`

行为变化：新增 `dirty_content_items` 和 `advisory_content_items` 统计。High-risk flags 会进入 `issues` 并导致 lint 失败；advisory-only flags 会进入 `warnings`，只提示不阻断。

## 4. 调用链

生成题：

`GenerationService.Generate` -> `gateQuestionCandidates` -> `candidateQualityFlags` -> `EvaluateQuestionContentQuality`

导入提交：

`POST /api/question-bank/imports/:id/commit` -> `ImportService.Commit` -> `commitReadyJob` -> `EvaluateQuestionContentQuality` -> `writer.Upsert`

运行时选题：

`POST /api/interview/start` 或 `POST /api/interview/answer` -> Graph `pick_next` -> `NewPickNextPatchNode` -> `filterDirtyQuestionContent` -> `pickByLLM` 或 `pickByRule`

题库 lint：

`go run ./cmd/questionbank-lint` -> `run` -> `loadLintItems` -> `lintItems` -> `EvaluateQuestionContentQuality`

## 5. 数据流

题干文本来自 LLM 生成候选、导入暂存 item、正式题库候选池或 lint 输入。质量检查只读取文本并生成 flags，不改变题干正文。只有 high-risk flags 会阻断生成、commit 或运行时优先选择；advisory flags 只用于诊断，避免普通多角度面试题被误伤。

生成阶段 flags 写入 `QuestionCandidate.QualityFlags`。commit 阶段 flags 写入 `ImportItem.AgentReviewReason`。运行时阶段只过滤候选池视图，不修改 `RetrievalTrace`。

## 6. 依赖与副作用

新增依赖：

- `cmd/questionbank-lint` 增加 `pgxpool` 用于显式 PG 扫描。

外部副作用：

- `questionbank-lint -postgres-dsn` 只读 PG。
- commit 阶段可能少导入高风险题，并更新 staging item 的审核状态。
- `pick_next` 可能跳过 RAG rank1 脏题，但会保留检索 trace 供排查。

## 7. 测试

新增或修改测试覆盖：

- 题干质量 flags。
- 生成候选题脏题拒绝。
- commit 阻止 accepted 脏题入库。
- `pick_next` 跳过脏题和全脏降级。
- lint 脏题报告。

已执行：

```powershell
go test ./internal/questionbank ./internal/nodes ./cmd/questionbank-lint -count=1
go test ./... -count=1
openspec validate harden-question-content-quality-gates --strict
```

结果：通过。

本地 PG 只读诊断：

```powershell
go run ./cmd/questionbank-lint -postgres-dsn "postgres://interview:interview@localhost:5432/interview?sslmode=disable" -min-expected-points 1 -min-scenario-ratio 0
```

结果：`total=84 dirty=1 advisory=3`。High-risk 命中 `codex-top100-agent-014`；另外 3 道只作为 advisory warning，不阻断。

## 8. 风险

- 规则过严会误伤正常多角度题干。当前已收紧多问题串判断，避免普通两段式面试题被判高风险。
- 历史 active 脏题不会自动下线，需要单独人工确认后再做数据库批量更新。
- `pick_next` 跳过脏题可能导致不选 RAG rank1；这是候选体验优先的显式取舍。
