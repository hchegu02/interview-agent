# 2026-06-07 Validate MD Question Bank Import Flow Verify

## Summary

| Dimension | Status |
| --- | --- |
| Completeness | 5/5 tasks complete |
| Correctness | 3/3 delta spec scenarios covered or bounded |
| Coherence | Design followed; one Comet process deviation recorded |

Final assessment: no critical implementation issues found. Ready for archive with the process deviation below recorded.

## Evidence

Implemented commit:

```text
b18a844 fix: validate markdown question import flow
```

Verification commands:

```powershell
go test ./internal/questionbank -count=1
go test ./... -count=1
openspec validate validate-md-questionbank-import-flow --strict
openspec validate --all --strict
git diff --check
```

Results: all passed.

Live flow evidence:

```text
job_id=imp-03316aae96b3
chunks=18
total_items=18
valid_items=18
accepted_auto_approved=18
imported_items=18
```

PostgreSQL embedding verification:

```text
embedded,BAAI/bge-m3,18
```

Detailed run report:

```text
docs/superpowers/reports/2026-06-07-md-questionbank-import-flow.md
```

## Completeness

All tasks in `openspec/changes/validate-md-questionbank-import-flow/tasks.md` are checked.

The proposal goal was to prove the real local Markdown import path:

```text
Markdown -> document import -> staging -> human approval -> commit
-> local BGE-M3 embedding -> PostgreSQL question_bank -> retrieval evidence
```

The verified flow reached commit and BGE-M3 embedding for all 18 generated items.

## Correctness

### Scenario: 从源文档生成题目草稿

Covered by the real HTTP document import and staging flow. The job saved file metadata, generated chunks, produced 18 valid staging items, and retained source provenance in import item metadata.

### Scenario: 人工确认源文档生成题后进入可检索题库

Covered by `accept_complete_valid` plus commit. The latest successful run imported 18/18 accepted items and PostgreSQL confirmed all 18 matching formal questions were `embedded` with model `BAAI/bge-m3`.

### Scenario: Skill 或 MCP 作为来源适配器

Bounded by design and spec. This change did not implement a new skill or MCP adapter. It verified and fixed the shared document-import path that adapters must feed into. No adapter writes directly to `question_bank`.

## Coherence

Implementation follows the change design:

- local PostgreSQL and BGE-M3 were used;
- Markdown upload used the real HTTP import API;
- generated document items required human review before commit;
- commit wrote to formal `question_bank`;
- embedding status was verified through PostgreSQL.

Backend blockers fixed during verification:

- document-generated question IDs are now backend-generated `docq-*` IDs;
- nil import item errors are normalized before PG writes;
- document import jobs no longer become `ready` before all chunks finish.

## Process Deviation

Comet metadata originally had:

```text
phase=build
build_mode=null
isolation=null
```

The implementation had already been completed and committed on `main` before verify metadata was filled. User selected option 1 to accept this deviation.

For guard compatibility, `.comet.yaml` was updated to:

```text
build_mode=subagent-driven-development
isolation=branch
```

This records the selected workflow intent, not a claim that the already-created commit was produced in an isolated feature branch. Actual Git state at verification time was normal repo on `main`, ahead of `origin/main`.

## Branch Handling

Because the work is already committed on `main`, there is no separate development branch or worktree to merge, push as PR, keep, or discard. Branch handling is considered accepted as-is for this change.

## Residual Risks

- The successful run used mock LLM generation, so it proves backend import mechanics, not generated question quality.
- The local PostgreSQL now contains imported validation rows; no cleanup was performed because the task explicitly validated local import behavior.
- Query API hides embedding fields unless admin view is requested; embedding status was therefore verified directly through PostgreSQL.
