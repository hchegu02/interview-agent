# Comet Design Handoff

- Change: add-rag-question-generation
- Phase: design
- Mode: compact
- Context hash: 4ec4ca9e2cf7644a859e0eedd97bc1c31901d1ff05c1b20f0e1e8bc995dc0126

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/add-rag-question-generation/proposal.md

- Source: openspec/changes/add-rag-question-generation/proposal.md
- Lines: 1-40
- SHA256: ea4af03514c2e155f70d563bcbab488f57e93e1fe60f3d7c44e505f43f3bd81f

```md
# Add RAG Question Generation

## Why

当前文档导入链路已经能把 Markdown 原文生成题目、暂存审核、commit 到 `question_bank` 并写入 BGE-M3 embedding。但现有生成方式更像“按 chunk 批量出题”：文档被切片后，每个 chunk 直接交给 LLM 生成题。它能验证链路，却不适合真实题库构建，因为用户常见需求是定向生成：

```text
基于某份材料/某个章节/某个知识点，生成 N 道指定题型、指定难度的题，并附解析和来源引用。
```

如果继续让 LLM 对所有 chunk 自由生成，容易出现重复题、偏题、无法解释来源、题型不受控和质量门禁不足。

## What Changes

- Add a backend RAG-driven question generation workflow.
- Allow callers to request generation by topic, count, difficulty, question type, and source scope.
- Retrieve relevant source chunks before calling LLM.
- Require structured LLM output with question fields, answer/explanation, expected points, rubric, follow-up hints, and source references.
- Stage generated questions through the existing question-bank import review flow.
- Add deduplication and quality gates before items become commit-eligible.
- Record generation evidence for observability and debugging.

## Scope

Initial MVP focuses on backend capability:

- HTTP API for creating a generation job.
- Service layer for query rewriting / retrieval / evidence packing / LLM generation / validation.
- Reuse existing document import chunks and import staging tables where practical.
- Reuse existing `questionbank.ImportService` commit path.
- Tests for request validation, generation quality gates, deduplication, source references, and staging behavior.

## Non-Goals

- No frontend changes in this change.
- No dedicated Top100 Markdown parser.
- No production LLM or production external API calls in tests.
- No direct write from LLM output to formal `question_bank`.
- No broad database cleanup or destructive migration.
- No runtime sub-agent implementation.
```

## openspec/changes/add-rag-question-generation/design.md

- Source: openspec/changes/add-rag-question-generation/design.md
- Lines: 1-212
- SHA256: a4487b16268af73fdbd9ead14ae020412a73be0156900a0678d785ed89b34439

[TRUNCATED]

```md
# Design

## Approach

Build an evidence-grounded, concept-first generation pipeline on top of the existing document import and question-bank staging model:

```text
GenerateQuestionRequest
  -> validate request
  -> resolve source scope
  -> rewrite query
  -> retrieve relevant source chunks
  -> extract concept cards
  -> build evidence pack from concepts and chunks
  -> LLM structured QuestionCandidate generation
  -> validate / deduplicate / quality gate
  -> stage generated candidates as document import items
  -> human review
  -> commit
```

The important boundary is that retrieved chunks are evidence, not questions. The system first normalizes evidence into concept cards, then asks the LLM to generate candidates from those cards. LLM output remains draft content until validation and human review pass.

## API Shape

Add backend endpoints under the question-bank generation namespace:

```text
POST /api/question-bank/generation-jobs
GET  /api/question-bank/generation-jobs/:id
POST /api/question-bank/generation-jobs/:id/stage
```

Initial request fields:

```json
{
  "source_job_id": "imp-xxx",
  "topic": "误差传播",
  "question_type": "single_choice",
  "count": 5,
  "difficulty": 3,
  "target_dimension": "tradeoff",
  "tags": ["error-propagation"],
  "skill_category": "math"
}
```

`source_job_id` points to an existing document import job with chunks. Later versions may support uploaded source collections or MCP/skill source adapters.

## Retrieval

MVP retrieval starts from stored import chunks:

- query text = topic + optional tags + question type + target dimension + source hints;
- query rewriting produces a clearer retrieval query;
- retrieval selects top source chunks from the specified job only;
- evidence pack preserves chunk IDs, hashes, filename, and excerpt text.

If pgvector chunk embeddings are not available yet, MVP can use lexical retrieval over stored chunks and keep the interface ready for vector retrieval. Do not block the feature on a new chunk-vector schema unless evidence shows lexical retrieval is insufficient.

## Concept Cards

MVP introduces an intermediate `ConceptCard` in service code and staging metadata, not a new table:

```json
{
  "concept_id": "concept-xxx",
  "source_job_id": "imp-xxx",
  "chunk_ids": ["imp-xxx:chunk:003"],
  "title": "Redis 缓存击穿",
  "skill": "缓存治理",
  "sub_skill": "高并发保护",
  "difficulty_hint": 3,
  "keywords": ["Redis", "缓存击穿", "热点 key"],
  "question_angles": ["concept", "tradeoff", "debugging"],
  "evidence_refs": [
    {"chunk_id": "imp-xxx:chunk:003", "quote": "..."}
  ]
}
```

Full source: openspec/changes/add-rag-question-generation/design.md

## openspec/changes/add-rag-question-generation/tasks.md

- Source: openspec/changes/add-rag-question-generation/tasks.md
- Lines: 1-14
- SHA256: f704ec0b4b4a94dd8f2ef81ec9a803b019c8f0b685c7bc8a135e941d249024f3

```md
# Tasks

- [ ] Add generation request/response domain types and validation.
- [ ] Implement source chunk retrieval scoped to a document import job.
- [ ] Add query rewriting / evidence retrieval boundary with mock-safe tests.
- [ ] Implement concept card extraction, backend-generated concept IDs, and concept deduplication.
- [ ] Build evidence packs from concept cards and source chunks.
- [ ] Implement structured QuestionCandidate parser and validation gates.
- [ ] Add duplicate, low-value-question, and source-grounding checks before staging.
- [ ] Add versioned generated-question metadata for concept, generation, question type, answer, and source refs.
- [ ] Stage generated questions through existing import review flow.
- [ ] Add HTTP endpoints for generation job create/get/stage.
- [ ] Add focused tests for quality gates and commit blocking.
- [ ] Update backend SDD / code-change docs for the new generation workflow.
```

## openspec/changes/add-rag-question-generation/specs/question-bank-import-enrichment/spec.md

- Source: openspec/changes/add-rag-question-generation/specs/question-bank-import-enrichment/spec.md
- Lines: 1-64
- SHA256: 050d3f789af8eec89cf391c90ac4201cc65d888e8a087e6f14df2d8e40b30222

```md
## MODIFIED Requirements

### Requirement: 源文档导入应保留可追溯原文

系统 MUST 支持把原文材料作为题库构建来源，并在生成题目进入暂存区前保留来源快照和来源引用。

#### Scenario: 从源文档生成题目草稿

- **WHEN** 用户导入原文材料用于构建 Go 后端题库
- **THEN** 系统 MUST 保存原文快照、来源类型和内容 hash
- **AND** 系统 MUST 生成暂存题目而不是直接写入正式题库
- **AND** 每道生成题 MUST 能追溯到源文档或源片段引用

#### Scenario: 按用户任务检索来源后定向生成题目

- **WHEN** 用户基于已导入源文档请求按主题、题型、数量和难度生成题目
- **THEN** 系统 MUST 先在指定来源范围内检索相关源片段
- **AND** 系统 MUST 从检索片段中抽取可出题的 concept cards
- **AND** 系统 MUST 将 concept cards 和检索到的源片段作为 evidence pack 传入 LLM
- **AND** LLM 输出 MUST 是结构化 QuestionCandidate 草稿
- **AND** 每道题 MUST 包含至少一个可回溯到检索片段的来源引用
- **AND** 生成题 MUST 进入暂存审核流程，而不是直接写入正式 question bank

#### Scenario: 无可用能力点时不生成题目

- **WHEN** 用户请求定向生成题目
- **AND** 系统检索到了文本片段但无法抽取可出题的 concept cards
- **THEN** 系统 MUST NOT 调用 LLM 凭空生成 QuestionCandidate
- **AND** 系统 MUST 返回可解释的空结果或失败原因

#### Scenario: 未命中来源证据时拒绝生成或降级为空结果

- **WHEN** 用户请求定向生成题目
- **AND** 指定来源范围内没有检索到可用源片段
- **THEN** 系统 MUST NOT 调用 LLM 直接凭空生成题目
- **AND** 系统 MUST 返回可解释的空结果或失败原因

#### Scenario: 生成题质量门禁阻止重复和无来源题

- **WHEN** LLM 返回题目草稿
- **AND** 草稿缺少必要字段、缺少来源引用、引用无法在 evidence pack 中验证、没有关联 concept card，或与同批生成题重复
- **THEN** 系统 MUST 阻止该题进入可提交状态
- **AND** 系统 SHOULD 在暂存项或生成结果中记录被阻止原因

#### Scenario: 生成题元数据应版本化

- **WHEN** 系统把 QuestionCandidate 写入暂存审核流程
- **THEN** 系统 MUST 写入版本化生成元数据
- **AND** 元数据 MUST 至少包含 generation job、source job、concept、question type、answer/explanation 和 source refs

#### Scenario: 人工确认源文档生成题后进入可检索题库

- **WHEN** 源文档导入生成了字段完整的有效暂存题
- **AND** 人工审核接受这些题目
- **AND** 本地 embedding 服务可用且维度与 `question_bank.embedding` 一致
- **THEN** commit MUST 将接受的题目写入正式 `question_bank`
- **AND** 系统 MUST 为提交题写入 embedded 状态和 embedding 模型
- **AND** 后续题库查询或 RAG 检索 MUST 能召回这些新题

#### Scenario: Skill 或 MCP 作为来源适配器

- **WHEN** 系统通过 skill 或 MCP 获取外部链接、文档或知识库原文
- **THEN** 该能力 MUST 只作为来源适配器进入源文档导入流程
- **AND** 适配器 MUST NOT 直接写入正式 question bank
```

