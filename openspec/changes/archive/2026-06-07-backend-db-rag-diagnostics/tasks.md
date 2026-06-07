## 1. Postgres Pool Config

- [x] Move Postgres pool config mapping to `internal/config`.
- [x] Reuse project pool config in `cmd/server`, `cmd/rag-eval`, `cmd/demo`, and `cmd/questionbank-explain`.
- [x] Add validation for invalid Postgres pool settings.
- [x] Add unit tests for pool config mapping and validation.

## 2. Question Bank Query Diagnostics

- [x] Extract question bank list SQL construction so runtime list and EXPLAIN share one query shape.
- [x] Add `PGStore.ExplainList` and `BuildListExplainQuery`.
- [x] Add `cmd/questionbank-explain` CLI.
- [x] Add tests for EXPLAIN SQL construction and CLI filter mapping.

## 3. PGVector / RAG Eval Trace

- [x] Add PGVector candidate source evidence for vector/tag/text paths.
- [x] Add `PGVectorRetriever.Search` returning `PipelineResult`.
- [x] Add per-stage trace for vector, tag, text, and fusion candidates.
- [x] Add `stage_candidates` to RAG eval case output.
- [x] Add tests for PGVector trace and RAG eval stage candidate output.

## 4. Verification

- [x] Run targeted package tests for config, questionbank, retriever, rag-eval, demo, server.
- [x] Run `go test ./... -count=1`.
- [x] Run `git diff --check`.
- [x] Run `openspec validate backend-db-rag-diagnostics --strict`.
