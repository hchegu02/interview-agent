---
comet_change: add-rag-question-generation
role: technical-design
canonical_spec: openspec
---

# Add RAG Question Generation Technical Design

## Goal

Build a backend MVP for task-driven question-bank generation:

```text
source import chunks
-> scoped retrieval
-> concept card extraction
-> evidence-grounded QuestionCandidate generation
-> quality gate
-> import staging
-> human review
-> commit
```

The feature must not add a Top100-specific parser and must not let LLM output bypass staging review.

## Existing Integration Points

- `internal/questionbank` owns import jobs, import chunks, staging items, review, commit, and embedding after commit.
- `internal/httpapi/question_bank.go` owns question-bank HTTP handlers.
- `internal/llm` provides `ChatModel` and schema-validated calls.
- `internal/parser` already extracts Markdown text for source documents.
- `internal/retriever` is for formal question-bank retrieval; MVP generation retrieval can start as a questionbank-local chunk retriever and keep an interface compatible with later hybrid retrieval.

## Service Boundary

Add a generation service under `internal/questionbank` rather than a new top-level package. The service should depend on existing boundaries:

```go
type GenerationService struct {
    imports ImportStore
    writer  Writer
    model   llm.ChatModel
}
```

MVP should keep generation jobs as a backend abstraction backed by existing import job/item metadata where practical. If this becomes awkward during implementation, add a small in-memory and PG-backed generation store in the same package, but do not change `question_bank` formal schema in this change.

## Data Model

### GenerateQuestionRequest

Fields:

- `source_job_id`
- `topic`
- `question_type`
- `count`
- `difficulty`
- `target_dimension`
- `tags`
- `skill_category`

Validation:

- `source_job_id`, `topic`, `question_type`, `count`, and `difficulty` are required.
- `count` must have a conservative maximum, e.g. 20.
- `difficulty` must be 1-5.
- `question_type` and `target_dimension` must be whitelisted.

### ConceptCard

Concept cards are the middle layer between evidence and questions:

```json
{
  "concept_id": "concept-xxx",
  "title": "Redis 缓存击穿",
  "skill": "缓存治理",
  "sub_skill": "高并发保护",
  "difficulty_hint": 3,
  "keywords": ["Redis", "缓存击穿"],
  "question_angles": ["concept", "tradeoff", "debugging"],
  "evidence_refs": [
    {"chunk_id": "imp-xxx:chunk:003", "quote": "..."}
  ]
}
```

IDs are backend-generated. LLM IDs are ignored.

### QuestionCandidate

The LLM output is a candidate, not a formal question:

```json
{
  "concept_id": "concept-xxx",
  "content": "...",
  "question_type": "single_choice",
  "target_dimension": "tradeoff",
  "options": [],
  "answer": "...",
  "explanation": "...",
  "expected_points": [],
  "rubric": {},
  "sample_answer": "...",
  "follow_up_hints": [],
  "source_refs": []
}
```

Question IDs are backend-generated from generation job, concept ID, index, content, and source refs.

## Pipeline

### 1. Validate Request

Reject invalid source job, missing fields, unsupported question type, unsupported target dimension, excessive count, and invalid difficulty.

### 2. Retrieve Source Chunks

Load chunks from `ImportStore.ListChunks(source_job_id)`. Retrieval must be scoped to this job only. MVP can use lexical scoring:

- topic exact/partial match;
- tags/skill/category hints;
- target dimension keywords;
- fallback to top chunks only when the source job exists and query is valid.

If no chunks match, return an explainable empty result and do not call LLM generation.

### 3. Extract Concept Cards

Build an evidence pack from retrieved chunks and ask LLM to extract concept cards using structured JSON. Validate:

- concept title present;
- at least one evidence ref;
- refs belong to retrieved chunks;
- quote appears in the referenced chunk text;
- duplicate concept cards collapsed by normalized title + source refs.

If no valid concepts remain, return an explainable empty result and do not generate questions.

### 4. Generate Candidates

Call LLM with concept cards, evidence pack, and request constraints. Require structured `candidates` JSON. Validate count and schema.

### 5. Quality Gate

Block or flag:

- missing fields;
- missing concept ID;
- unknown concept ID;
- missing source refs;
- quote not grounded in evidence;
- same-batch duplicate content;
- “请总结本文” style low-value questions;
- unsupported question type;
- incomplete single-choice options;
- interview question without follow-up hints;
- difficulty inconsistent with request and concept hint.

Only candidates passing the hard gate may be staged. Passing candidates still default to `needs_human_review`.

### 6. Stage

Map candidates to `questionbank.Item` and call the existing staging path. Store versioned generated metadata:

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
  "concept": {},
  "question": {},
  "source_refs": []
}
```

Do not store unversioned arbitrary metadata.

## HTTP API

MVP endpoints:

```text
POST /api/question-bank/generation-jobs
GET  /api/question-bank/generation-jobs/:id
POST /api/question-bank/generation-jobs/:id/stage
```

Handlers should be added near existing question-bank import handlers. Responses should include `omitempty` fields and should not expose raw prompts.

## Testing Strategy

Unit tests:

- request validation;
- chunk retrieval scoped to source job;
- no evidence means no LLM generation call;
- concept extraction rejects missing/foreign refs;
- concept deduplication;
- candidate parser;
- hard gate for missing refs, low-value questions, invalid choice options, duplicate content;
- metadata schema versioning;
- staged candidates remain blocked from commit until human accept.

Integration-style tests can use memory stores and mock LLM. Real LLM and real external APIs are out of scope for CI.

## Risks And Deferrals

- Lexical retrieval is acceptable for MVP but not the final retrieval strategy.
- Concept cards may deserve a table later; not in this change.
- Formal question type/options schema may be needed for exam UI later; not in this change.
- Quality gates should record reasons because overly strict gates can hide useful content.
