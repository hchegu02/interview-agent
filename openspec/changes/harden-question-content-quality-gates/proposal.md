# Change: harden-question-content-quality-gates

## Problem

The question-bank pipeline can currently accept structurally complete but semantically dirty question content. A real example is an imported Top100 item whose `content` contains a raw interview-note chain, inline comments, and self-evaluation text such as `--（无法反驳..`. The item has valid metadata and embeddings, so it can become `active`, be retrieved by RAG, and be selected by `pick_next`.

This is a data-quality problem, not a frontend display problem. If dirty content enters the formal question bank, embedding and RAG make it easier to surface during interviews.

## Goals

- Add a shared backend content-quality gate for interview question text.
- Reuse the gate in generation, import commit, question-bank lint, and runtime question selection fallback.
- Prevent high-risk dirty content from silently entering or being selected from the formal question bank.
- Keep historical data governance explicit: detect and report dirty existing questions, but do not delete or batch-update database rows in this change.

## Non-Goals

- No frontend changes.
- No automatic deletion of existing question-bank rows.
- No database schema change unless implementation proves it is strictly necessary.
- No LLM-only quality gate. Rules must be deterministic and testable.

## Scope

- `internal/questionbank`: shared content quality checks and import/generation commit integration.
- `internal/nodes`: `pick_next` runtime guard against high-risk dirty candidate content.
- `cmd/questionbank-lint`: report content quality issues for seed or loaded items.
- Backend documentation and focused tests.
