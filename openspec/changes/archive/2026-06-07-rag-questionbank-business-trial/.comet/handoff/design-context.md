# Comet Design Handoff

- Change: rag-questionbank-business-trial
- Phase: design
- Mode: compact
- Context hash: 7b0509444c22e099d4a92d1f2f567a4820d0efd0b5247c2bbd9bfe92d5584d66

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/rag-questionbank-business-trial/proposal.md

- Source: openspec/changes/rag-questionbank-business-trial/proposal.md
- Lines: 1-50
- SHA256: 31f2f83d582bcefa6419b444980bb572673b3de6ea6022efff091b6894d19902

```md
# Change: RAG Question Bank Business Trial

## Why

The project already has question-bank import, enrichment, staging review, commit, embedding gates, multi-stage RAG retrieval, trace output, and local eval/lint commands. The next risk is not whether the code has a retriever, but whether a real internal team can build and use a job-specific question bank without polluting production-quality data.

For Go backend interview scenarios, raw source material such as technical articles, interview notes, JD documents, and internal knowledge pages must be converted into high-quality interview questions with traceable provenance. Retrieval quality also needs a controlled improvement path: query rewriting is low-cost and likely useful, while HyDE is useful but higher-risk and should be measured before it affects live question selection.

## Goals

- Build a Go backend single-role internal business trial loop for RAG question-bank construction and interview usage.
- Make Agent behavior visible in the question-bank pipeline: ingest source material, generate questions, enrich metadata, review quality, classify risk, and recommend commit decisions.
- Support source-document based import as a first-class trial flow, including future adapter points for skills or MCP tools that fetch original text from trusted links or documents.
- Add a retrieval enhancement design for Query Rewriting before embedding and a configurable HyDE experiment path.
- Keep generated questions out of the formal question bank unless they pass automated quality gates and the configured approval policy.
- Extend trial documentation and verification gates so the team can decide whether the RAG question-bank business is usable for internal trials.

## Non-Goals

- No fully autonomous production publishing of generated questions in this change.
- No runtime sub-agent framework is required; Agent roles may be implemented as backend orchestration steps with structured traces.
- No public web crawler or broad internet ingestion is required for MVP.
- No replacement of the existing vector/BM25/rule/RRF/rerank retrieval pipeline.
- No default HyDE live ranking until evaluation shows non-regression.

## Scope

In scope:

- Go backend trial package and runbook updates.
- Source-document import model and staged generated-question flow.
- Agent quality review states and audit records.
- Query Rewriting trace and fallback semantics.
- HyDE `off` / `shadow` / `enabled` mode design, with `shadow` as the recommended internal trial default.
- RAG eval additions for Go backend golden queries and retrieval strategy comparison.

Out of scope:

- Production auth or multi-tenant permission redesign.
- A full dashboard for question-bank operations.
- External MCP server runtime implementation beyond documented adapter boundaries.
- Automatic crawling of arbitrary websites.

## Success Criteria

- A Go backend trial operator can import source material, generate/enrich questions, inspect Agent decisions, and commit only approved items.
- Internal trial runs can distinguish source quality issues, Agent generation issues, embedding failures, and retrieval failures.
- Query Rewriting and HyDE behavior are visible in `RetrievalTrace` or equivalent diagnostic output.
- HyDE can be evaluated in shadow mode without changing live question selection.
- Verification commands document the minimum required gates for RAG question-bank internal trial readiness.
```

## openspec/changes/rag-questionbank-business-trial/design.md

- Source: openspec/changes/rag-questionbank-business-trial/design.md
- Lines: 1-122
- SHA256: 74ea3a2560d7320c81d7f5d034080d6859b981f11778dc9546986b9a123444e4

[TRUNCATED]

```md
# Design

## Overview

This change treats the RAG question bank as a business workflow, not just a retriever feature. The system should support an Agent-first construction loop:

```text
source document / trusted link / uploaded file
  -> source text snapshot
  -> Agent extraction and question generation
  -> Agent enrichment
  -> Agent quality review
  -> auto_approved | needs_human_review | rejected
  -> configured publish decision
  -> formal question_bank
  -> RAG retrieval with rewrite / HyDE trace
  -> internal trial feedback
```

The current import staging and commit flow remains the boundary that protects the formal question bank. The new work should extend that flow instead of bypassing it.

## Source Document Import

The MVP should represent source material explicitly before it becomes questions. A source document should preserve enough provenance for review and rollback:

- source type: uploaded file, pasted text, trusted link, future skill adapter, future MCP adapter
- source URI or filename when available
- captured text snapshot
- content hash
- extraction status and error
- generated question IDs

Skills and MCP tools should be treated as source adapters. They may fetch or normalize original text, but they must not directly write formal question-bank rows.

## Agent-First Question Construction

The Agent pipeline should produce structured outputs that match the existing question-bank item shape:

- content
- skill category
- tags and role tags
- difficulty
- scenario
- expected points
- rubric
- sample answer
- follow-up hints
- provenance back to source excerpts

Quality review should classify each generated item:

- `auto_approved`: complete, source-grounded, non-duplicate, and low risk
- `needs_human_review`: useful but ambiguous, incomplete, or medium-risk
- `rejected`: unsupported by source, duplicate, too generic, malformed, or unsafe

For the first internal trial, `auto_approved` can be committed only by an explicit batch confirmation or controlled trial policy. This keeps Agent value visible while preventing silent data pollution.

## Query Rewriting

Query Rewriting should run after `retrieve_rag` builds the base query and before embedding. The rewrite input should include:

- job title
- key skills and must-have skills
- missing skills from gap analysis
- current target difficulty
- question-bank filters
- locale

The output should include:

- original query
- rewritten query
- normalized tags
- rewrite reason
- error or fallback reason

If rewrite fails, retrieval must continue with the original query and record fallback diagnostics.

## HyDE

```

Full source: openspec/changes/rag-questionbank-business-trial/design.md

## openspec/changes/rag-questionbank-business-trial/tasks.md

- Source: openspec/changes/rag-questionbank-business-trial/tasks.md
- Lines: 1-36
- SHA256: 64a05ce1ca60bff9b749fcdc75c080ddfed5e659d3de9c1711ef5911b13b40b8

```md
# Tasks

## 1. Design And Spec

- [ ] Add OpenSpec requirements for Agent-first source-document question-bank construction.
- [ ] Add OpenSpec requirements for Query Rewriting and HyDE retrieval diagnostics.
- [ ] Add OpenSpec requirements for Go backend RAG question-bank internal trial gates.
- [ ] Produce Superpowers technical design doc from the Comet design phase.

## 2. Source-Document Question Construction

- [ ] Model source-document provenance for trial imports without bypassing existing staging.
- [ ] Add Agent-generated question review states: `auto_approved`, `needs_human_review`, `rejected`.
- [ ] Preserve source excerpts or source references for generated questions.
- [ ] Keep formal question-bank commit behind configured approval policy.

## 3. Retrieval Enhancement

- [ ] Add Query Rewriting before query embedding with fallback to original query.
- [ ] Record original query, rewritten query, and rewrite fallback reason in retrieval diagnostics.
- [ ] Add HyDE mode configuration: `off`, `shadow`, `enabled`.
- [ ] Implement HyDE shadow diagnostics without changing live candidate selection.

## 4. Business Trial Package

- [ ] Add Go backend golden queries for RAG eval.
- [ ] Document Go backend source-material import trial steps.
- [ ] Document Agent review state interpretation and commit policy.
- [ ] Document minimum verification gates for RAG question-bank internal trial readiness.

## 5. Verification

- [ ] Run targeted Go tests for changed packages.
- [ ] Run `go run ./cmd/questionbank-lint ...` for trial seed data when applicable.
- [ ] Run `go run ./cmd/rag-eval ...` with strategy comparison when applicable.
- [ ] Run `openspec validate rag-questionbank-business-trial --strict`.
```

## openspec/changes/rag-questionbank-business-trial/specs/question-bank-import-enrichment/spec.md

- Source: openspec/changes/rag-questionbank-business-trial/specs/question-bank-import-enrichment/spec.md
- Lines: 1-63
- SHA256: 19779669e1f9def207cbd155f60148bceb1a2044a485ae0dc031d3ee9d74c09d

```md
## MODIFIED Requirements

### Requirement: 题库导入支持缺失元数据补全

系统 MUST 在本地题库导入时支持对缺失元数据的题目进行补全，并保留审核后再提交的流程。系统 SHOULD expose Agent-generated quality review decisions so high-confidence generated items can be batch-confirmed while risky items remain blocked from formal question-bank commit.

#### Scenario: 使用 LLM 补全只有题干的导入项

- **WHEN** 用户导入本地题库文件，且题目只有 `id` 和 `content`
- **AND** 导入服务配置了 LLM 模型
- **THEN** 系统应请求 LLM 补全缺失的 `skill_category`、`difficulty`、`tags`、`expected_points`、`rubric`、`sample_answer` 和 `follow_up_hints`
- **AND** 系统必须保留原始 `id` 和 `content`
- **AND** 系统必须先把补全后的题目暂存为待审核导入项，而不是直接写入正式题库

#### Scenario: 未配置 LLM 时保持旧默认行为

- **WHEN** 用户导入缺失元数据的本地题库文件
- **AND** 导入服务没有配置 LLM 模型
- **THEN** 系统应继续使用默认元数据
- **AND** 系统不应凭空生成 `expected_points`、`rubric`、`sample_answer` 或 `follow_up_hints`

#### Scenario: LLM 漏返回输入题目时导入失败

- **WHEN** LLM 补全返回结果缺少某个输入题目
- **THEN** 系统必须拒绝本次导入
- **AND** 导入任务状态应标记为失败

#### Scenario: 批量补全题目

- **WHEN** 导入文件包含多道需要补全的题目
- **THEN** 系统可以按批次请求 LLM 补全
- **AND** 每个批次都必须校验返回项覆盖输入项

#### Scenario: 暂存项保留字段来源

- **WHEN** 系统暂存导入项
- **THEN** 系统应记录关键字段来源
- **AND** 上传字段、默认字段、LLM 补全字段和生成字段应可区分
- **AND** 暂存项应保留原始上传内容，供审核和 diff 预览使用

#### Scenario: Agent 质量审核建议不绕过发布策略

- **WHEN** Agent 对导入或生成的题目给出 `auto_approved`、`needs_human_review` 或 `rejected` 建议
- **THEN** 系统 MUST 保存该建议和理由
- **AND** `rejected` 题目 MUST NOT 被提交到正式题库
- **AND** `auto_approved` 题目 MUST 仍受当前导入任务的发布策略控制

### Requirement: 源文档导入应保留可追溯原文

系统 MUST 支持把原文材料作为题库构建来源，并在生成题目进入暂存区前保留来源快照和来源引用。

#### Scenario: 从源文档生成题目草稿

- **WHEN** 用户导入原文材料用于构建 Go 后端题库
- **THEN** 系统 MUST 保存原文快照、来源类型和内容 hash
- **AND** 系统 MUST 生成暂存题目而不是直接写入正式题库
- **AND** 每道生成题 MUST 能追溯到源文档或源片段引用

#### Scenario: Skill 或 MCP 作为来源适配器

- **WHEN** 系统通过 skill 或 MCP 获取外部链接、文档或知识库原文
- **THEN** 该能力 MUST 只作为来源适配器进入源文档导入流程
- **AND** 适配器 MUST NOT 直接写入正式 question bank
```

## openspec/changes/rag-questionbank-business-trial/specs/rag-retrieval-enhancement/spec.md

- Source: openspec/changes/rag-questionbank-business-trial/specs/rag-retrieval-enhancement/spec.md
- Lines: 1-55
- SHA256: d1523702a48cd67b4e0e8fe563cf8a67970790839ecb1444cbbfc6b8e823befd

```md
## ADDED Requirements

### Requirement: RAG 检索应支持查询改写

系统 MUST 支持在 RAG query embedding 前进行可配置 Query Rewriting，以把岗位、技能缺口、动态难度和题库过滤条件转成更适合题库检索的查询文本。

#### Scenario: 查询改写成功

- **WHEN** `retrieve_rag` 已构造基础查询
- **AND** Query Rewriting 已启用
- **THEN** 系统 MUST 在 embedding 前生成 rewritten query
- **AND** embedding 和文本召回应使用 rewritten query
- **AND** RetrievalTrace MUST 记录 original query 和 rewritten query

#### Scenario: 查询改写失败时降级

- **WHEN** Query Rewriting 调用失败、超时或返回空结果
- **THEN** 系统 MUST 使用 original query 继续检索
- **AND** RetrievalTrace MUST 记录 rewrite fallback reason
- **AND** 面试流程 MUST NOT 因查询改写失败而中断

### Requirement: RAG 检索应支持 HyDE 实验模式

系统 MUST 提供 HyDE 配置模式，用于生成题库条目风格的假设文档并评估其对召回质量的影响。

#### Scenario: HyDE shadow 模式不影响正式候选选择

- **WHEN** HyDE mode 为 `shadow`
- **THEN** 系统 MAY 生成 HyDE 文本和对应 embedding
- **AND** 系统 MUST 记录 HyDE 诊断信息
- **AND** 正式 CandidatePool MUST 仍由非 HyDE live path 决定

#### Scenario: HyDE enabled 模式参与检索

- **WHEN** HyDE mode 为 `enabled`
- **AND** 配置明确允许 HyDE 影响 live retrieval
- **THEN** 系统 MAY 使用 HyDE embedding 参与 vector recall
- **AND** RetrievalTrace MUST 标记 HyDE 参与了最终检索

#### Scenario: HyDE 失败时回退

- **WHEN** HyDE 生成或 embedding 失败
- **THEN** 系统 MUST 回退到 non-HyDE retrieval path
- **AND** RetrievalTrace MUST 记录 HyDE fallback reason

### Requirement: Go 后端题库试用应有 RAG 策略对比门禁

系统 MUST 为 Go 后端单岗位内部试用提供 RAG eval 对比，至少覆盖 baseline、query rewrite 和 HyDE shadow 诊断路径。

#### Scenario: 运行 Go 后端 RAG 题库试用 eval

- **WHEN** 维护者准备发布 Go 后端题库试用包
- **THEN** 验证 MUST 运行 RAG eval golden queries
- **AND** 验证 MUST 输出 baseline 与 query rewrite 的对比结果
- **AND** HyDE shadow 结果 MUST 可用于人工判断是否升级到 enabled
```
