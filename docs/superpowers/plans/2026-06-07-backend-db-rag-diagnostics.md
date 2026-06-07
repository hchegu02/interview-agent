---
archived-with: 2026-06-07-backend-db-rag-diagnostics
status: final
---
# Backend DB/RAG Diagnostics Implementation Plan

change: backend-db-rag-diagnostics
design-doc: docs/superpowers/specs/2026-06-07-backend-db-rag-diagnostics-design.md

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make backend Postgres and RAG diagnostics verifiable before internal trial.

**Architecture:** Reuse existing config, questionbank, retriever, and rag-eval boundaries. Add diagnostic evidence as additive output without changing HTTP API, Session JSON, or database schema.

**Tech Stack:** Go, pgxpool, PostgreSQL/pgvector, OpenSpec, existing RAG eval.

---

### Task 1: Shared Postgres Pool Config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `cmd/server/deps.go`
- Modify: `cmd/rag-eval/main.go`
- Modify: `cmd/demo/main.go`
- Modify: `cmd/questionbank-explain/main.go`

- [x] Add `config.PostgresPoolConfig`.
- [x] Add config validation for invalid Postgres pool settings.
- [x] Update server and backend CLI pool creation to use `pgxpool.NewWithConfig`.
- [x] Run config/server/CLI package tests.

### Task 2: Question Bank Explain Diagnostics

**Files:**
- Modify: `internal/questionbank/pg_store.go`
- Create: `internal/questionbank/pg_store_test.go`
- Create: `cmd/questionbank-explain/main.go`
- Create: `cmd/questionbank-explain/main_test.go`

- [x] Extract list query construction from `PGStore.List`.
- [x] Add `BuildListExplainQuery` and `PGStore.ExplainList`.
- [x] Add `cmd/questionbank-explain`.
- [x] Run questionbank and CLI tests.

### Task 3: PGVector Stage Trace

**Files:**
- Modify: `internal/retriever/pgvector.go`
- Modify: `internal/retriever/pgvector_test.go`
- Modify: `cmd/rag-eval/main.go`
- Modify: `cmd/rag-eval/main_test.go`

- [x] Add PG candidate evidence fields.
- [x] Add `PGVectorRetriever.Search`.
- [x] Build vector/tag/text/fusion stage trace.
- [x] Add `stage_candidates` to rag-eval case output.
- [x] Run retriever and rag-eval tests.

### Task 4: Documentation and Verification

**Files:**
- Modify: `docs/code-changes/06-07-postgres-pool-config.md`
- Create: `docs/code-changes/06-07-questionbank-explain.md`
- Create: `docs/code-changes/06-07-rag-pgvector-trace.md`
- Create: `openspec/changes/backend-db-rag-diagnostics/*`

- [x] Update code-change docs from actual diff.
- [x] Create OpenSpec proposal/design/tasks and delta specs.
- [x] Run `go test ./... -count=1`.
- [x] Run `git diff --check`.
- [x] Run `openspec validate backend-db-rag-diagnostics --strict`.
