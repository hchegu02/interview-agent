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
