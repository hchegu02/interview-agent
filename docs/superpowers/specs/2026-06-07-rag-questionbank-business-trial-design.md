---
comet_change: rag-questionbank-business-trial
role: technical-design
canonical_spec: openspec
---

# RAG Question Bank Business Trial Design

## Context

OpenSpec change `rag-questionbank-business-trial` defines a Go backend single-role internal trial for building and using a RAG question bank. Existing code already has question-bank import, LLM enrichment, staging review, commit, embedding status gates, multi-stage retrieval, RAG trace, `questionbank-lint`, and `rag-eval`.

The design goal is not to replace these foundations. The goal is to make the RAG question-bank construction loop usable by an internal team, with Agent-visible generation and quality review, source provenance, query rewriting, HyDE shadow diagnostics, and measurable trial gates.

## Technical Approach

Use the existing question-bank staging and commit flow as the write boundary. Add capabilities around it:

```text
source material
  -> source document snapshot
  -> generated question drafts
  -> Agent enrichment and quality review
  -> staged import items
  -> approval policy
  -> formal question_bank
  -> retrieve_rag query rewrite
  -> retrieval pipeline
  -> HyDE shadow diagnostics
  -> trial feedback
```

The important constraint is that Agent output does not directly write formal question-bank rows. Agent output creates or annotates staged items, and formal commit remains controlled by review status, quality state, embedding success, and trial policy.

## Components

### Source Document Import

Add a source-document layer for raw materials used to build questions. This layer should store:

- source type: pasted text, uploaded file, trusted link, future skill adapter, future MCP adapter
- source URI or filename when available
- captured text snapshot
- content hash
- extraction status and error
- generated item references

This should be modeled as trial import provenance, not as a second formal knowledge base. If a skill or MCP tool fetches content, it is only an adapter that returns text plus metadata into this source-document layer.

### Agent Question Construction

Agent construction should be treated as a deterministic backend workflow around LLM calls, not as a new runtime sub-agent system.

Logical roles:

- Source ingest: normalize source material and preserve provenance.
- Question generator: produce question drafts from source excerpts.
- Enrichment: fill tags, skill category, difficulty, scenario, expected points, rubric, sample answer, and follow-up hints.
- Quality review: classify each item as `auto_approved`, `needs_human_review`, or `rejected`.
- Commit adviser: summarize whether a batch is safe to publish under the configured trial policy.

The existing staging model remains the shared data structure. New review metadata should be additive and backward-compatible.

### Approval Policy

First internal trial policy:

- `rejected`: never committed.
- `needs_human_review`: requires explicit item or batch approval.
- `auto_approved`: can be batch-confirmed, but should not silently commit by default.

Later policies may allow direct commit for `auto_approved`, but only with audit records, source provenance, and rollback guidance.

### Query Rewriting

Introduce query rewriting inside `retrieve_rag`, after the base query is built and before embedding.

Input:

- job title
- key skills and must-have skills
- missing skills
- target difficulty
- question-bank filter
- locale

Output:

- original query
- rewritten query
- normalized tags
- rewrite reason
- fallback reason on failure

Failure must degrade to original query. The interview must not fail because rewrite failed.

### HyDE

HyDE should generate a hypothetical question-bank entry, not a candidate answer. The generated text should look like:

```text
Question: ...
Assessed skills: ...
Expected points: ...
Difficulty: ...
```

Modes:

- `off`: no HyDE.
- `shadow`: generate HyDE diagnostics without changing live candidate selection.
- `enabled`: HyDE embedding can participate in live vector retrieval.

The internal trial default should be `shadow`. Moving to `enabled` requires eval non-regression and explicit configuration.

### Retrieval Trace

Retrieval diagnostics should expose:

- original query
- rewritten query
- rewrite status and fallback reason
- HyDE mode
- HyDE status and fallback reason
- stage-level result IDs
- final candidate IDs

Avoid storing long raw source text or full generated HyDE answers in user-facing trace by default. Store summaries or hashes where enough for diagnosis.

## Data Flow

### Import Flow

```text
operator provides source
  -> source snapshot saved
  -> Agent generates drafts
  -> drafts normalized into import items
  -> Agent review annotates quality state
  -> staging UI/API shows source diff and review reason
  -> approval policy filters commit candidates
  -> commit embeds accepted items
  -> formal question_bank only contains active embedded items
```

### Retrieval Flow

```text
Session.JobProfile + GapReport + QuestionBankFilter
  -> buildQueryTags/buildQueryText
  -> optional Query Rewriting
  -> embed selected query text
  -> vector/BM25/rule retrieval
  -> RRF
  -> rerank
  -> optional HyDE shadow comparison
  -> CandidatePool + RetrievalTrace
```

HyDE shadow should not alter `CandidatePool`; it should attach diagnostics for eval and manual review.

## Error Handling

- Source fetch fails: source import fails with stable error state; no staged questions.
- Agent generation returns malformed JSON: import fails or item is marked rejected, depending on batch granularity.
- Agent misses input/source references: item is rejected or the batch fails; do not silently create source-less questions.
- Query rewrite fails: continue with original query and record fallback.
- HyDE fails: continue non-HyDE path and record fallback.
- Embedding fails during commit: existing behavior should keep the item out of RAG-ready state.

## Testing Strategy

Backend tests should cover:

- source provenance persisted into staged generated items
- Agent review state filtering during commit
- rejected items cannot enter formal question bank
- query rewrite success and fallback semantics
- HyDE shadow does not change live candidate pool
- retrieval trace includes rewrite and HyDE diagnostics

Evaluation should cover:

- Go backend golden queries
- baseline vs query rewrite
- baseline vs rewrite plus HyDE shadow diagnostics
- no regression on existing RAG thresholds

Documentation checks should cover:

- trial runbook includes source import steps
- trial runbook explains Agent review states
- verification checklist includes `questionbank-lint`, `rag-eval`, and OpenSpec strict validation

## Implementation Boundaries

Likely code areas:

- `internal/questionbank`: source provenance, import review state, commit filtering
- `internal/nodes/retrieve_rag.go`: rewrite orchestration and trace propagation
- `internal/retriever`: trace shape and HyDE diagnostics if pipeline-level comparison is needed
- `cmd/rag-eval`: strategy comparison flags or fixtures
- `docs/ai/internal-trial`: Go backend question-bank trial runbook
- `openspec/specs`: archive target after validation

Avoid changing auth, session storage semantics, or the formal question-bank table contract unless the implementation proves a schema addition is required.

## Risks

- HyDE can improve semantic recall while making failures harder to diagnose. Keep shadow as the default.
- Agent-generated questions can pollute the bank if review state and provenance are optional. Make both visible in staging.
- Skill/MCP source adapters can blur trust boundaries. Treat them as text providers only.
- If eval only measures aggregate recall, bad difficulty or weak expected points may slip through. Keep questionbank lint and trial feedback in the gate.
