---
comet_change: dedupe-question-generation-review-gate
role: technical-design
canonical_spec: openspec
---

# Dedupe Question Generation Review Gate Design

## Technical Approach

This change keeps dedupe conservative: exact match after normalized question content only. It does not add schema, does not introduce semantic similarity, and does not change frontend behavior.

The core implementation should make one normalized content-key helper the shared boundary between generation, staging, and commit. The helper should normalize only content shape that does not change meaning: trim, case fold, whitespace collapse/removal, and common punctuation normalization where already consistent with existing candidate normalization. It must not remove technical terms or rewrite text.

Generation quality gates should accept an optional set of existing active question content keys. `GenerationService` should load active question-bank items before candidates are gated, derive normalized keys from their `content`, and pass them into `gateQuestionCandidates`. A candidate duplicated against an existing active question should be rejected before it becomes a committable staged item. The rejection reason should be explicit, for example `duplicate_existing_content`.

The import path needs a second guard because old staged jobs, manual review, or concurrent generation can bypass generation-time checks. Commit should build a per-job seen-key set and skip repeated accepted items after the first one. It should also load current active question-bank keys immediately before writing and skip any accepted item whose key already exists. Skipped duplicate items should keep diagnostic metadata through existing review or issue fields; no new database columns are required.

## Data Flow

```text
document chunks
  -> concept cards
  -> LLM candidates
  -> candidate gates
       -> required field checks
       -> source reference checks
       -> same-batch content dedupe
       -> existing active question-bank dedupe
  -> staged import items
  -> human review
  -> commit guards
       -> review policy
       -> same-job content dedupe
       -> current active question-bank dedupe
  -> question_bank
```

## Implementation Boundaries

- Do not change PostgreSQL schema.
- Do not change frontend files.
- Do not make skill, MCP, or LLM output write directly to `question_bank`.
- Keep public API compatibility. If any response gains diagnostic fields, they must be optional.
- Keep inactive historical duplicates as data cleanup scope, not application behavior.
- Treat memory store and PG store consistently in tests where practical.

## Testing Strategy

Add focused tests under `internal/questionbank`:

- generation rejects a candidate whose normalized content already exists in active question bank;
- generation still allows distinct questions that merely share similar terms;
- commit skips duplicate accepted items within the same import job;
- commit skips an accepted item if an active question with the same normalized content already exists;
- existing review policy still blocks `needs_human_review` and `rejected` items.

Run:

```powershell
go test ./internal/questionbank -count=1
```

If implementation touches shared storage interfaces or query behavior, also run the smallest affected package tests.

## Risks

The main risk is over-normalization causing false duplicate rejection. Keep normalization intentionally weak and exact. Another risk is commit count semantics: accepted staged items may be fewer than imported items after duplicate skipping, so tests must pin the expected result. A final risk is partial diagnostic visibility if the existing import item metadata cannot represent all duplicate reasons cleanly; prefer reusing existing agent review reason or quality issue fields over adding schema.
