# Change: RAG Question Bank Business Trial

## Why

The project already has question-bank import, enrichment, staging review, commit, embedding gates, multi-stage RAG retrieval, trace output, and local eval/lint commands. The next risk is not whether the code has a retriever, but whether a real internal team can build and use a job-specific question bank without polluting production-quality data.

For Go backend interview scenarios, raw source material such as technical articles, interview notes, JD documents, and internal knowledge pages must be converted into high-quality interview questions with traceable provenance. Retrieval quality also needs a controlled improvement path: query rewriting is low-cost and likely useful, while HyDE is useful but higher-risk and should be measured before it affects live question selection.

## Goals

- Build a Go backend single-role internal business trial loop for RAG question-bank construction and interview usage.
- Make Agent behavior visible in the question-bank pipeline: ingest source material, generate questions, enrich metadata, review quality, classify risk, and recommend commit decisions.
- Support source-document based import as a first-class trial flow, including future adapter points for skills or MCP tools that fetch original text from trusted links or documents.
- Add a retrieval enhancement design for Query Rewriting before embedding and a configurable HyDE experiment path.
- Keep generated questions out of the formal question bank unless they pass automated quality gates and the configured approval policy.
- Extend trial documentation and verification gates so the team can decide whether the RAG question-bank business is usable for internal trials.

## Non-Goals

- No fully autonomous production publishing of generated questions in this change.
- No runtime sub-agent framework is required; Agent roles may be implemented as backend orchestration steps with structured traces.
- No public web crawler or broad internet ingestion is required for MVP.
- No replacement of the existing vector/BM25/rule/RRF/rerank retrieval pipeline.
- No default HyDE live ranking until evaluation shows non-regression.

## Scope

In scope:

- Go backend trial package and runbook updates.
- Source-document import model and staged generated-question flow.
- Agent quality review states and audit records.
- Query Rewriting trace and fallback semantics.
- HyDE `off` / `shadow` / `enabled` mode design, with `shadow` as the recommended internal trial default.
- RAG eval additions for Go backend golden queries and retrieval strategy comparison.

Out of scope:

- Production auth or multi-tenant permission redesign.
- A full dashboard for question-bank operations.
- External MCP server runtime implementation beyond documented adapter boundaries.
- Automatic crawling of arbitrary websites.

## Success Criteria

- A Go backend trial operator can import source material, generate/enrich questions, inspect Agent decisions, and commit only approved items.
- Internal trial runs can distinguish source quality issues, Agent generation issues, embedding failures, and retrieval failures.
- Query Rewriting and HyDE behavior are visible in `RetrievalTrace` or equivalent diagnostic output.
- HyDE can be evaluated in shadow mode without changing live question selection.
- Verification commands document the minimum required gates for RAG question-bank internal trial readiness.
