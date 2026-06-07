# Design

## Approach

Build an evidence-grounded, concept-first generation pipeline on top of the existing document import and question-bank staging model:

```text
GenerateQuestionRequest
  -> validate request
  -> resolve source scope
  -> rewrite query
  -> retrieve relevant source chunks
  -> extract concept cards
  -> build evidence pack from concepts and chunks
  -> LLM structured QuestionCandidate generation
  -> validate / deduplicate / quality gate
  -> stage generated candidates as document import items
  -> human review
  -> commit
```

The important boundary is that retrieved chunks are evidence, not questions. The system first normalizes evidence into concept cards, then asks the LLM to generate candidates from those cards. LLM output remains draft content until validation and human review pass.

## API Shape

Add backend endpoints under the question-bank generation namespace:

```text
POST /api/question-bank/generation-jobs
GET  /api/question-bank/generation-jobs/:id
POST /api/question-bank/generation-jobs/:id/stage
```

Initial request fields:

```json
{
  "source_job_id": "imp-xxx",
  "topic": "误差传播",
  "question_type": "single_choice",
  "count": 5,
  "difficulty": 3,
  "target_dimension": "tradeoff",
  "tags": ["error-propagation"],
  "skill_category": "math"
}
```

`source_job_id` points to an existing document import job with chunks. Later versions may support uploaded source collections or MCP/skill source adapters.

## Retrieval

MVP retrieval starts from stored import chunks:

- query text = topic + optional tags + question type + target dimension + source hints;
- query rewriting produces a clearer retrieval query;
- retrieval selects top source chunks from the specified job only;
- evidence pack preserves chunk IDs, hashes, filename, and excerpt text.

If pgvector chunk embeddings are not available yet, MVP can use lexical retrieval over stored chunks and keep the interface ready for vector retrieval. Do not block the feature on a new chunk-vector schema unless evidence shows lexical retrieval is insufficient.

## Concept Cards

MVP introduces an intermediate `ConceptCard` in service code and staging metadata, not a new table:

```json
{
  "concept_id": "concept-xxx",
  "source_job_id": "imp-xxx",
  "chunk_ids": ["imp-xxx:chunk:003"],
  "title": "Redis 缓存击穿",
  "skill": "缓存治理",
  "sub_skill": "高并发保护",
  "difficulty_hint": 3,
  "keywords": ["Redis", "缓存击穿", "热点 key"],
  "question_angles": ["concept", "tradeoff", "debugging"],
  "evidence_refs": [
    {"chunk_id": "imp-xxx:chunk:003", "quote": "..."}
  ]
}
```

Concept extraction can be deterministic plus LLM-assisted:

- MVP may ask LLM to extract concept cards from the evidence pack.
- If no chunks are retrieved, do not call LLM.
- If no concept cards are extracted, return an explainable empty result.
- Concept IDs are backend-generated from generation job, title, source refs, and index.
- Concept cards are deduplicated before question generation.

This layer gives the system skill coverage, deduplication, and future reporting hooks without adding a concept table yet.

## LLM Output Contract

The LLM must return structured JSON with `candidates`, not formal questions:

```json
{
  "candidates": [
    {
      "concept_id": "concept-xxx",
      "content": "...",
      "question_type": "single_choice",
      "target_dimension": "tradeoff",
      "options": ["A", "B", "C", "D"],
      "answer": "A",
      "explanation": "...",
      "tags": [],
      "skill_category": "...",
      "difficulty": 3,
      "expected_points": [],
      "rubric": {},
      "sample_answer": "...",
      "follow_up_hints": [],
      "source_refs": [
        {"chunk_id": "imp-xxx:chunk:003", "quote": "..."}
      ]
    }
  ]
}
```

For existing `questionbank.Item`, map:

- `content` -> `Item.Content`
- `expected_points`, `rubric`, `sample_answer`, `follow_up_hints` -> existing fields
- `question_type`, `target_dimension`, `options`, `answer`, `explanation`, `source_refs`, `concept_id` -> staging metadata first.

Prefer metadata for MVP to avoid broad schema changes. Use the name `QuestionCandidate` for LLM output. It must not be treated as a formal question until staging review and commit complete.

## Quality Gates

Before staging:

- generated count must be between 1 and requested count, unless the system reports insufficient evidence;
- each candidate must have content, skill category, difficulty, tags, expected points, rubric, and sample answer/explanation;
- each candidate must reference at least one retrieved source chunk;
- each candidate must reference a known concept card;
- repeated question content in the same generation job must be rejected or collapsed;
- content too similar to existing formal `question_bank` items should be marked with a duplicate warning;
- source quotes must appear in retrieved evidence or be rejected as ungrounded;
- low-value summary questions such as “请总结本文” must be rejected;
- single-choice questions must have complete, mutually exclusive options and one answer;
- interview questions must include follow-up hints;
- difficulty must be consistent with concept difficulty hint where available.

All generated candidates that reach staging still default to `needs_human_review`.

## Metadata Contract

Generated staging metadata must be versioned:

```json
{
  "schema_version": "generated_question_v1",
  "generation": {
    "job_id": "gen-xxx",
    "source_job_id": "imp-xxx",
    "retrieval_method": "lexical",
    "prompt_version": "v1",
    "quality_gate_version": "v1"
  },
  "concept": {
    "concept_id": "concept-xxx",
    "title": "...",
    "skill": "...",
    "target_dimension": "tradeoff"
  },
  "question": {
    "type": "single_choice",
    "options": [],
    "answer": "...",
    "explanation": "..."
  },
  "source_refs": []
}
```

Do not store unversioned arbitrary JSON blobs.

## Storage

MVP should avoid schema churn:

- use existing import jobs/items for staging generated candidates;
- record generation metadata in import job metadata and import item metadata;
- record source refs in item metadata/source provenance;
- keep concept cards in generation result metadata for MVP;
- if generation job lifecycle cannot fit import jobs cleanly, add a small generation job abstraction in Go first and persist through import job metadata before adding tables.

## Testing

Tests should cover:

- request validation;
- retrieval restricted to `source_job_id`;
- no evidence means no LLM generation call;
- concept card extraction and deduplication;
- structured output parsing;
- duplicate generated questions;
- missing source refs;
- source quote grounding;
- low-value summary question rejection;
- staging into existing import review flow;
- commit remains blocked until human acceptance.

## Risks

- LLM quality is still variable. The backend must treat output as drafts.
- Without chunk embeddings, lexical retrieval may miss semantically relevant passages. Keep retrieval interface replaceable.
- If metadata becomes too overloaded, a later change should introduce explicit generation tables.
- Concept extraction can over-normalize or miss useful angles. Keep rejected/empty concept reasons visible.
