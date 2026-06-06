---
change: rag-questionbank-business-trial
role: verification
canonical_spec: openspec
---

# RAG Question Bank Business Trial Verification

## Summary

| Dimension | Status |
|---|---|
| Tasks | 20/20 complete |
| OpenSpec | `rag-questionbank-business-trial` strict validation passed |
| Go tests | Targeted changed packages passed |
| Question bank lint | Passed |
| Go backend RAG eval | Passed |
| Security scan | No submitted secrets found |
| Final reviewer | Approved after CRITICAL fix |

## Verified Scope

- Source-document generated import items keep source provenance.
- Agent review states are persisted through memory and PG import stores.
- `rejected` and `needs_human_review` items cannot commit to formal question bank.
- `auto_approved` items can commit under the trial policy.
- Query Rewriting runs before embedding when configured and falls back to original query on error or empty rewrite.
- HyDE shadow records diagnostics without changing live candidate selection.
- Go backend RAG eval fixture uses real seed IDs.

## Commands

```powershell
go test ./internal/questionbank ./internal/nodes ./internal/retriever -count=1
go run ./cmd/questionbank-lint -seed seeds/question_bank.json -min-expected-points 3 -min-scenario-ratio 0.8
go run ./cmd/rag-eval -cases testdata/rag/golden_queries_go_backend.jsonl -config config/config.yaml.example -out tmp/eval/rag-go-backend
openspec validate rag-questionbank-business-trial --strict
git diff --check 813a175..HEAD
```

## Results

- Go tests: passed.
- Question bank lint: passed; `total=52`, `scenario_ratio=1`, `expected_points_pass_ratio=1`, `complete_metadata_ratio=1`.
- RAG eval: passed; `cases=8 recall@5=0.750 recall@10=0.875 mrr@10=1.000 ndcg@10=0.858 source=seed`.
- OpenSpec strict validation: passed.
- Diff check: passed.

## Review Outcome

The first final verification found one CRITICAL issue: `needs_human_review` generated items could still commit because only `rejected` was blocked. The fix changed `importItemAccepted` so only legacy empty Agent review status and `auto_approved` pass; both `needs_human_review` and `rejected` are blocked. New tests cover `needs_human_review`, `rejected`, and `auto_approved`.

After the fix, final reviewer approved the change with no CRITICAL issues.

## Known Limits

- Query Rewriter and HyDE Generator are optional node-level interfaces and are not wired to the production Interview Graph by default.
- `rag-eval` now has a Go backend fixture, but not a full baseline/rewrite/HyDE strategy comparison CLI.
- HyDE live `enabled` ranking is not implemented in this change; this change implements shadow diagnostics.
