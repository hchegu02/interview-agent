---
comet_change: question-bank-import-enrichment
role: technical-design
canonical_spec: openspec
---

# Question Bank Import Enrichment Design

## Problem

Question bank import is part of the RAG construction path. In real use, local files may contain only question text. That is not enough for retrieval, interview scope filtering, scoring, or follow-up generation.

The system must enrich incomplete imported questions without breaking existing imports.

## Approach

Keep the import pipeline simple:

```text
parse -> enrich -> normalize -> validate -> stage
```

LLM enrichment runs only for local question-bank imports and only before staging. It is not part of commit, embedding, or retrieval.

## Data Rules

- `id` and `content` are owned by the uploaded file and must not be replaced by the LLM.
- Complete user-provided metadata wins over LLM output.
- Missing metadata can be filled from LLM output:
  - `tags`
  - `skill_category`
  - `difficulty`
  - `expected_points`
  - `rubric`
  - `sample_answer`
  - `follow_up_hints`
- If no LLM model is configured, import must keep the previous default behavior.

## Matching

The enriched response is matched back to the original item by `id`. If `id` is absent or unmatched, matching can fall back to exact `content`.

The merge keeps the original `id` and `content` even when the LLM returns different values.

## Failure Behavior

If no LLM is configured, enrichment is skipped.

If an LLM is configured but returns invalid JSON or fails schema validation, the import job fails instead of silently staging bad enriched metadata. This is the practical choice: silent corruption is worse than an explicit failed import.

## Testing

Regression tests cover:

- question-only local import with mock LLM enrichment
- no-LLM fallback preserving old defaults
- preservation of original `id` and `content`

## Follow-Up Work

- Split large imports into enrichment batches to avoid oversized prompts.
- Enforce returned item count and one-to-one matching more strictly.
- Surface field-level enrichment diff in the import preview UI.
- Track enrichment provenance per staged item.
