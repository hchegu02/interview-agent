# Backend DB/RAG Diagnostics Verification Report

change: backend-db-rag-diagnostics
verified-at: 2026-06-07

## Scope

Verified backend-only changes:

- Shared Postgres pool config and validation.
- Question bank list EXPLAIN diagnostics.
- PGVector stage trace and RAG eval per-case stage candidates.
- OpenSpec delta specs for `server-runtime` and `rag-retrieval-enhancement`.

Excluded from this verification:

- Frontend workbench layout active change.
- `internal/httpapi/web/dist` line-ending/stat-only dirtiness.
- Real PostgreSQL runtime EXPLAIN output, because `INTERVIEW_POSTGRES_DSN` is not configured in this shell.

## Commands

```powershell
go test ./... -count=1
openspec validate backend-db-rag-diagnostics --strict
openspec validate --all --strict
git diff --check
```

## Results

- `go test ./... -count=1`: passed via Comet build guard.
- `openspec validate backend-db-rag-diagnostics --strict`: passed, output: `Change 'backend-db-rag-diagnostics' is valid`.
- `openspec validate --all --strict`: passed after archive/spec sync repair, output: `18 passed, 0 failed`.
- `git diff --check`: passed; only line-ending warnings were printed.

## Requirement Check

- Server runtime pool config:
  - `internal/config.PostgresPoolConfig` maps configured pool settings.
  - `Config.validate` rejects invalid pool values.
  - `cmd/server`, `cmd/rag-eval`, `cmd/demo`, and `cmd/questionbank-explain` use `pgxpool.NewWithConfig`.

- Question bank diagnostics:
  - Runtime list query and EXPLAIN query share `buildListQuery`.
  - `cmd/questionbank-explain` maps CLI filters to `questionbank.Filter`.
  - Invalid cursor is rejected by shared query construction.

- RAG / PGVector diagnostics:
  - PGVector SQL returns vector/tag/text candidate path evidence.
  - `PGVectorRetriever.Search` returns `PipelineResult`.
  - `cmd/rag-eval` writes `stage_candidates` in per-case results.

## Remaining Risk

- Real PG performance still requires running `cmd/questionbank-explain` and `cmd/rag-eval` against a populated PostgreSQL database.
- `cmd/reindex` still uses standalone `-dsn` and does not load project `Config`.
- Query SQL has not yet been optimized; this change intentionally adds evidence before rewriting.
