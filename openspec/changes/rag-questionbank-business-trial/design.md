# Design

## Overview

This change treats the RAG question bank as a business workflow, not just a retriever feature. The system should support an Agent-first construction loop:

```text
source document / trusted link / uploaded file
  -> source text snapshot
  -> Agent extraction and question generation
  -> Agent enrichment
  -> Agent quality review
  -> auto_approved | needs_human_review | rejected
  -> configured publish decision
  -> formal question_bank
  -> RAG retrieval with rewrite / HyDE trace
  -> internal trial feedback
```

The current import staging and commit flow remains the boundary that protects the formal question bank. The new work should extend that flow instead of bypassing it.

## Source Document Import

The MVP should represent source material explicitly before it becomes questions. A source document should preserve enough provenance for review and rollback:

- source type: uploaded file, pasted text, trusted link, future skill adapter, future MCP adapter
- source URI or filename when available
- captured text snapshot
- content hash
- extraction status and error
- generated question IDs

Skills and MCP tools should be treated as source adapters. They may fetch or normalize original text, but they must not directly write formal question-bank rows.

## Agent-First Question Construction

The Agent pipeline should produce structured outputs that match the existing question-bank item shape:

- content
- skill category
- tags and role tags
- difficulty
- scenario
- expected points
- rubric
- sample answer
- follow-up hints
- provenance back to source excerpts

Quality review should classify each generated item:

- `auto_approved`: complete, source-grounded, non-duplicate, and low risk
- `needs_human_review`: useful but ambiguous, incomplete, or medium-risk
- `rejected`: unsupported by source, duplicate, too generic, malformed, or unsafe

For the first internal trial, `auto_approved` can be committed only by an explicit batch confirmation or controlled trial policy. This keeps Agent value visible while preventing silent data pollution.

## Query Rewriting

Query Rewriting should run after `retrieve_rag` builds the base query and before embedding. The rewrite input should include:

- job title
- key skills and must-have skills
- missing skills from gap analysis
- current target difficulty
- question-bank filters
- locale

The output should include:

- original query
- rewritten query
- normalized tags
- rewrite reason
- error or fallback reason

If rewrite fails, retrieval must continue with the original query and record fallback diagnostics.

## HyDE

HyDE should generate a hypothetical question-bank style document, not a candidate answer. The generated text should look like a high-quality question-bank entry: interview question, assessed skills, expected points, and difficulty.

HyDE modes:

- `off`: no HyDE generation
- `shadow`: generate HyDE and run comparison diagnostics without affecting final live selection
- `enabled`: allow HyDE embedding to participate in vector retrieval

The recommended MVP default is `shadow`. `enabled` should require eval non-regression and explicit configuration.

## Retrieval Trace

Diagnostics should make retrieval behavior explainable:

- base query
- rewritten query
- HyDE mode
- HyDE generated text summary or hash
- stage-level result IDs
- fallback reasons
- final selected candidates

Trace output should avoid leaking long source text or full generated answers by default.

## Verification

Verification should compare retrieval strategies:

- baseline
- query rewrite
- query rewrite plus HyDE shadow
- query rewrite plus HyDE enabled when explicitly configured

Go backend golden queries should cover concurrency, channel, runtime scheduler, Redis, database transactions, distributed systems, observability, and performance debugging.

The trial runbook should require:

- questionbank lint
- rag eval
- Go backend tests for changed packages
- OpenSpec strict validation
- manual or scripted inspection of Agent-generated review states
