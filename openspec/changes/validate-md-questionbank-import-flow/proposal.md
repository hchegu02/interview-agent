# Validate MD Question Bank Import Flow

## Why

The project needs proof that local Markdown source material can build a usable question bank through the real backend path:

Markdown document -> document import -> LLM-generated staging items -> human approval -> commit -> local BGE-M3 embedding -> PostgreSQL question_bank -> RAG retrieval.

The document-import approval bug has been fixed, but the full operational flow has not yet been exercised with a real local Markdown file.

## What Changes

- Run the existing import APIs against a real local Markdown document.
- Record operational evidence: job status, generated item counts, approval behavior, commit result, embedding status, and retrieval/eval result.
- If the flow exposes backend defects, fix only those defects in this change.
- Produce a run report / runbook for repeating the import.

## Scope

Input document:

`D:\Documents\Mirrorfiles\Obsidian\CODEX\raw\project-experience\AI 模拟面试项目 Top100 高频追问回答.md`

## Non-Goals

- No frontend changes.
- No schema changes unless a verified blocker requires a separate confirmation.
- No production API calls.
- No broad database cleanup.
