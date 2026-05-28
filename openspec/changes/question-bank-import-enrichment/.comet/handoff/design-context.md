# Comet Design Context

source: openspec/changes/question-bank-import-enrichment/proposal.md
lines: 1-31
sha256: 77076e1d312fbbc818f0f92ce8cb9dbe9a384881ee0adaaa209313f557e6405f
mode: compact

## Proposal Excerpt

Question bank import is now part of the RAG construction path. Real imported files often contain only question text, while retrieval and interview scoring need structured metadata.

Goals:
- Support local question imports with incomplete metadata.
- Use LLM enrichment to fill missing question-bank fields before staging.
- Preserve existing behavior when no LLM model is configured.
- Keep imported `id` and `content` stable.
- Stage enriched items for review before commit.

source: openspec/changes/question-bank-import-enrichment/design.md
lines: 1-27
sha256: 77076e1d312fbbc818f0f92ce8cb9dbe9a384881ee0adaaa209313f557e6405f
mode: compact

## Design Excerpt

The import pipeline keeps the existing parse -> validate -> stage shape. LLM enrichment is inserted between parse and stage for local question-bank imports.

Merge enriched data conservatively:
- never replace `id`
- never replace `content`
- only fill missing metadata fields

source: openspec/changes/question-bank-import-enrichment/tasks.md
lines: 1-7
sha256: 77076e1d312fbbc818f0f92ce8cb9dbe9a384881ee0adaaa209313f557e6405f
mode: compact

## Task Excerpt

- Add local import LLM enrichment for question-only uploads.
- Preserve no-LLM fallback behavior.
- Add deterministic mock LLM response.
- Add regression tests for enrichment and fallback.
- Document the code change.
