# Tasks

## 1. Design And Spec

- [x] Add OpenSpec requirements for Agent-first source-document question-bank construction.
- [x] Add OpenSpec requirements for Query Rewriting and HyDE retrieval diagnostics.
- [x] Add OpenSpec requirements for Go backend RAG question-bank internal trial gates.
- [x] Produce Superpowers technical design doc from the Comet design phase.

## 2. Source-Document Question Construction

- [x] Model source-document provenance for trial imports without bypassing existing staging.
- [x] Add Agent-generated question review states: `auto_approved`, `needs_human_review`, `rejected`.
- [x] Preserve source excerpts or source references for generated questions.
- [x] Keep formal question-bank commit behind configured approval policy.

## 3. Retrieval Enhancement

- [x] Add Query Rewriting before query embedding with fallback to original query.
- [x] Record original query, rewritten query, and rewrite fallback reason in retrieval diagnostics.
- [x] Add HyDE mode configuration: `off`, `shadow`, `enabled`.
- [x] Implement HyDE shadow diagnostics without changing live candidate selection.

## 4. Business Trial Package

- [x] Add Go backend golden queries for RAG eval.
- [x] Document Go backend source-material import trial steps.
- [x] Document Agent review state interpretation and commit policy.
- [x] Document minimum verification gates for RAG question-bank internal trial readiness.

## 5. Verification

- [x] Run targeted Go tests for changed packages.
- [x] Run `go run ./cmd/questionbank-lint ...` for trial seed data when applicable.
- [x] Run `go run ./cmd/rag-eval ...` with strategy comparison when applicable.
- [x] Run `openspec validate rag-questionbank-business-trial --strict`.
