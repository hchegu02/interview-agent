---
change: harden-question-content-quality-gates
design-doc: docs/superpowers/specs/2026-06-08-harden-question-content-quality-gates-design.md
base-ref: 782833e8aa12ba474993ec805a676cbed905cbbd
---

# Plan: Harden Question Content Quality Gates

## Scope

Implement deterministic backend content quality gates for question-bank data. The implementation must not change frontend code and must not mutate existing database rows automatically.

## Tasks

### 1. Shared Content Quality Classifier

- Add a reusable classifier under `internal/questionbank`.
- Return stable flags and a high-risk decision.
- Cover dirty note markers, multi-question raw chains, overly long content, comment/answer leakage, and low-value summarization prompts.
- Add focused unit tests with a dirty Top100-style example and a normal normalized question.

### 2. Generation and Commit Gates

- Reuse the classifier in generation quality gates.
- Reuse the classifier during import commit as a final backend guard.
- Dirty accepted items must be marked rejected with diagnostic reason and must not be written to formal `question_bank`.
- Preserve duplicate and human-review behavior already implemented.
- Add focused tests for generation rejection and commit blocking.

### 3. Runtime Selection Guard

- In `pick_next`, filter high-risk dirty candidates before LLM/rule selection when clean candidates remain.
- If all candidates are dirty, continue with degraded reason instead of ending the session.
- Keep retrieval trace unchanged.
- Add focused tests for clean-candidate preference and all-dirty fallback.

### 4. Lint and Diagnostics

- Extend `cmd/questionbank-lint` to report dirty content issues.
- Keep seed lint behavior compatible.
- Add optional PG active-row scan for local diagnostics.
- Add tests for dirty-content reporting.

### 5. Documentation and Verification

- Update `docs/SDD-Backend.md` with the new quality gate behavior.
- Add `docs/code-changes/06-08-question-content-quality-gates.md`.
- Run targeted tests:
  - `go test ./internal/questionbank ./internal/nodes ./cmd/questionbank-lint -count=1`
- Run full backend verification:
  - `go test ./... -count=1`
- Run OpenSpec validation:
  - `openspec validate harden-question-content-quality-gates --strict`

## Review Strategy

Use subagent-driven development in this session:

- Main agent owns integration and final verification.
- Spec-review subagent checks implementation against OpenSpec and this plan.
- Code-quality subagent checks false positives, compatibility, and runtime risks.
- Workers may only be used for disjoint follow-up fixes if review finds bounded issues.
