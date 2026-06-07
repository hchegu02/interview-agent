---
comet_change: validate-md-questionbank-import-flow
role: technical-design
canonical_spec: openspec
status: draft
archived-with: 2026-06-07-validate-md-questionbank-import-flow
status: final
---

# Validate MD Question Bank Import Flow Design

## Goal

Validate the real backend path for building a question bank from a local Markdown document:

`Markdown -> document import -> staging -> human approval -> commit -> BGE-M3 embedding -> PostgreSQL question_bank -> retrieval evidence`.

## Input

`D:\Documents\Mirrorfiles\Obsidian\CODEX\raw\project-experience\AI 模拟面试项目 Top100 高频追问回答.md`

## Execution Plan

1. Check local PostgreSQL connectivity and required question-bank import tables.
2. Check local OpenAI-compatible BGE-M3 embedding endpoint.
3. Start `cmd/server` with `config/config.yaml` and explicit local env overrides.
4. Upload the Markdown file through `POST /api/question-bank/imports?async=true`.
5. Poll the job until it reaches `ready` or `failed`.
6. Apply `accept_complete_valid` review.
7. Commit the import.
8. Verify formal question-bank rows, embedding status, and retrieval visibility.
9. Record exact commands, results, and blockers.

## Failure Handling

If an environment dependency is unavailable, stop and record it as an environment blocker. If backend logic blocks the flow after dependencies are available, fix the backend defect in this change.

## Non-Goals

No frontend changes, no database schema changes, no production API calls, and no broad data cleanup.
