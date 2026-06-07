# Comet Design Handoff

- Change: validate-md-questionbank-import-flow
- Phase: design
- Mode: compact
- Context hash: b8feb7dd23499a17b663c9fadd125ac728a66ee5f36b8052f78314966cde2ada

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/validate-md-questionbank-import-flow/proposal.md

- Source: openspec/changes/validate-md-questionbank-import-flow/proposal.md
- Lines: 1-29
- SHA256: 73a1182a58daed92ce2645b82c6f212f63f861b0650a547c89df643cfa56745a

```md
# Validate MD Question Bank Import Flow

## Why

The project needs proof that local Markdown source material can build a usable question bank through the real backend path:

Markdown document -> document import -> LLM-generated staging items -> human approval -> commit -> local BGE-M3 embedding -> PostgreSQL question_bank -> RAG retrieval.

The document-import approval bug has been fixed, but the full operational flow has not yet been exercised with a real local Markdown file.

## What Changes

- Run the existing import APIs against a real local Markdown document.
- Record operational evidence: job status, generated item counts, approval behavior, commit result, embedding status, and retrieval/eval result.
- If the flow exposes backend defects, fix only those defects in this change.
- Produce a run report / runbook for repeating the import.

## Scope

Input document:

`D:\Documents\Mirrorfiles\Obsidian\CODEX\raw\project-experience\AI 模拟面试项目 Top100 高频追问回答.md`

## Non-Goals

- No frontend changes.
- No schema changes unless a verified blocker requires a separate confirmation.
- No production API calls.
- No broad database cleanup.
```

## openspec/changes/validate-md-questionbank-import-flow/design.md

- Source: openspec/changes/validate-md-questionbank-import-flow/design.md
- Lines: 1-29
- SHA256: b40f64708ce2895c5e51be596f28c888aff18fcce7f4885fb26a9b2c53f8f471

```md
# Design

## Approach

Use the live backend import path rather than a synthetic unit test:

1. Ensure local PostgreSQL is reachable and has required migrations.
2. Ensure local BGE-M3 OpenAI-compatible embedding endpoint is reachable.
3. Start the Go server with `config/config.yaml`.
4. Upload the Markdown file as `source_type=document`.
5. Poll the import job until it reaches `ready` or `failed`.
6. Review generated items with `accept_complete_valid`.
7. Commit the import.
8. Verify `question_bank` rows and `embedding_status=embedded`.
9. Run retrieval evidence with question-bank query and RAG eval where applicable.

## Expected Behavior

Document-generated valid items initially require human review. After `accept_complete_valid`, complete valid items should become publishable and commit should import them into `question_bank`. With `embedding.mode=real` and local BGE-M3 configured, committed items should receive embeddings during commit.

## Failure Handling

- If PostgreSQL or BGE-M3 is unavailable, record the environment blocker and stop before changing code.
- If import fails because of parser / LLM / review / commit logic, capture the error and fix the backend defect in this change.
- If generated items are low quality but the pipeline works, do not change code; record quality risks for the next quality-gate change.

## Data Safety

This change writes only to the local configured PostgreSQL database. It does not delete existing rows. It may upsert formal question-bank rows if generated question IDs collide.
```

## openspec/changes/validate-md-questionbank-import-flow/tasks.md

- Source: openspec/changes/validate-md-questionbank-import-flow/tasks.md
- Lines: 1-7
- SHA256: dfad70b6311ab09469a09b96461b3c3665d948d3e587542b7aeda1e14b6c72a0

```md
# Tasks

- [ ] Check local PostgreSQL, BGE-M3 embedding endpoint, and server configuration.
- [ ] Run Markdown document import through the HTTP API.
- [ ] Review and commit complete valid generated items.
- [ ] Verify question_bank rows, embedding status, and retrieval evidence.
- [ ] Record results and fix backend blockers if discovered.
```

## openspec/changes/validate-md-questionbank-import-flow/specs/question-bank-import-enrichment/spec.md

- Source: openspec/changes/validate-md-questionbank-import-flow/specs/question-bank-import-enrichment/spec.md
- Lines: 1-27
- SHA256: 6a0e916f7c24144156f3da75486e6fdcc9186ec9ceb6bf228bfcb82bc829d082

```md
## MODIFIED Requirements

### Requirement: 源文档导入应保留可追溯原文

系统 MUST 支持把原文材料作为题库构建来源，并在生成题目进入暂存区前保留来源快照和来源引用。

#### Scenario: 从源文档生成题目草稿

- **WHEN** 用户导入原文材料用于构建 Go 后端题库
- **THEN** 系统 MUST 保存原文快照、来源类型和内容 hash
- **AND** 系统 MUST 生成暂存题目而不是直接写入正式题库
- **AND** 每道生成题 MUST 能追溯到源文档或源片段引用

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

