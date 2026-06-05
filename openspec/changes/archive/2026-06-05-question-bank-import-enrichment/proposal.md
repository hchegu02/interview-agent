# Question Bank Import Enrichment

## Why

Question bank import is now part of the RAG construction path. Real imported files often contain only question text, while retrieval and interview scoring need structured metadata such as skill category, tags, difficulty, expected points, rubric, sample answer, and follow-up hints.

Leaving those fields as `general` defaults makes the RAG path technically present but operationally weak.

## Goals

- Support local question imports with incomplete metadata.
- Use LLM enrichment to fill missing question-bank fields before staging.
- Preserve existing behavior when no LLM model is configured.
- Keep imported `id` and `content` stable.
- Stage enriched items for review before commit.

## Scope

- Local JSON, CSV, and Markdown question-bank imports.
- Metadata enrichment before staging.
- Mock LLM support for deterministic tests.
- Regression tests for enrichment and fallback behavior.

## Non-Goals

- Replacing human review of staged imports.
- Bulk asynchronous re-enrichment of existing question-bank rows.
- Changing the committed question-bank schema beyond existing import support.
- Making LLM output trusted without validation.
