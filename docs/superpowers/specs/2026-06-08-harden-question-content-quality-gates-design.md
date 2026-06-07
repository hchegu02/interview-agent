---
comet_change: harden-question-content-quality-gates
role: technical-design
canonical_spec: openspec
---

# Harden Question Content Quality Gates

## Context

The current question-bank pipeline already checks metadata completeness, source references, duplicate content, review status, commit eligibility, and embedding readiness. That is not enough for real business use: structurally valid but dirty question text can still enter `question_bank`.

The observed example is a Top100 item whose content is an uncleaned interview-note chain rather than a normalized interview question. Because it is `active` and `embedded`, RAG can retrieve it and `pick_next` can show it to a candidate.

## Technical Design

Add a shared deterministic classifier in `internal/questionbank` for question content quality. It returns stable flags and a high-risk decision. The rules focus on obvious dirty content that should not require LLM judgment:

- note residue and inline comments such as `--`, `无法反驳`, `TODO`, `待补充`
- multiple raw questions or follow-up chains collapsed into one `content`
- overly long question text
- answer, explanation, or self-comment leakage into the question stem
- low-value prompt-like questions such as summarization requests

The classifier is deliberately rule-based. LLM cleanup can be added later, but commit and runtime safety gates must be deterministic and testable.

## Integration Points

Generation quality gates should append content-quality flags to `QuestionCandidate.QualityFlags`. High-risk dirty candidates must not become auto-approved.

Import commit should run the same check on accepted staging items before `writer.Upsert`. Dirty accepted items should be marked rejected with a diagnostic reason, not silently skipped.

`pick_next` should filter high-risk dirty candidates before LLM/rule selection when at least one clean candidate remains. If every candidate is dirty, it may continue to avoid ending the interview early, but it must record a degraded reason.

`cmd/questionbank-lint` should report content quality flags and fail on high-risk dirty items. This gives operators a safe way to inspect existing active rows before deciding whether to inactivate or rewrite them.

## Non-Goals

This change does not alter the frontend. It does not delete or batch-update existing database rows. It does not add schema columns unless implementation proves metadata storage is impossible without them.

## Risks

False positives are possible with rule-based checks. The mitigation is to keep rules focused on high-signal residue and make advisory flags visible without blocking unless they are high-risk.

Runtime filtering can hide relevant RAG rank1 candidates. This is intentional when the candidate is dirty and a clean alternative exists. Retrieval trace remains intact for diagnosis.

## Verification

Focused tests should cover classifier flags, generation rejection, commit blocking, `pick_next` clean-candidate preference, all-dirty degraded fallback, and lint reporting.

Targeted commands:

```powershell
go test ./internal/questionbank ./internal/nodes ./cmd/questionbank-lint -count=1
go test ./... -count=1
openspec validate harden-question-content-quality-gates --strict
```
