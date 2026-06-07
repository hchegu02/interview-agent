---
archived-with: 2026-06-07-add-rag-question-generation
status: final
---
# RAG Question Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build backend MVP for evidence-grounded, concept-first question generation from imported source document chunks.

**Architecture:** Add a focused generation service under `internal/questionbank` that validates requests, retrieves chunks scoped to a source import job, extracts concept cards, generates `QuestionCandidate` drafts, applies quality gates, and stages passing candidates through the existing import review flow. HTTP handlers in `internal/httpapi` expose create/get/stage endpoints without touching frontend code.

**Tech Stack:** Go, existing `questionbank.ImportStore`, existing `questionbank.ImportService` staging/commit flow, existing `llm.ChatModel` and `llm.CallWithSchema`, Gin HTTP handlers, memory-store tests first.

---

## Metadata

```yaml
change: add-rag-question-generation
design-doc: docs/superpowers/specs/2026-06-07-add-rag-question-generation-design.md
base-ref: a58be45bc8dce34445dd87542aa8bb2c311a988b
```

## Files

- Create: `internal/questionbank/generation_types.go`
  - Request/response structs, status constants, metadata structs.
- Create: `internal/questionbank/generation_service.go`
  - Pipeline orchestration: validate, retrieve, extract concepts, generate candidates, gate, stage.
- Create: `internal/questionbank/generation_retrieval.go`
  - Lexical chunk retrieval scoped to source job.
- Create: `internal/questionbank/generation_llm.go`
  - Structured LLM calls and JSON parsing for concept cards and candidates.
- Create: `internal/questionbank/generation_quality.go`
  - Hard gates: required fields, source grounding, concept refs, duplicate content, low-value summary questions, choice option validation.
- Create: `internal/questionbank/generation_test.go`
  - Unit tests for service, retrieval, quality gates, staging behavior.
- Modify: `internal/questionbank/imports_types.go`
  - Add minimal staging helper or metadata support only if needed. Keep interface-compatible.
- Modify: `internal/questionbank/imports_stage.go`
  - Add a service-level path to stage generated candidates only if existing unexported staging blocks reuse.
- Modify: `internal/httpapi/question_bank.go`
  - Add generation request/response handlers.
- Modify: `internal/httpapi/router.go`
  - Register generation endpoints.
- Modify: `internal/httpapi/question_bank_test.go`
  - Handler tests for request validation and happy path with mock generation service.
- Modify: `cmd/server/main.go` or wiring file only if needed to set generation service on server.
- Modify: `docs/SDD-Backend.md`
  - Document generation pipeline.
- Add: `docs/code-changes/06-07-rag-question-generation.md`
  - Required code-change record.

## Task 1: Domain Types And Validation

**Files:**
- Create: `internal/questionbank/generation_types.go`
- Create: `internal/questionbank/generation_test.go`

- [ ] **Step 1: Write failing tests for request validation**

Add tests:

```go
func TestValidateGenerationRequestRejectsMissingRequiredFields(t *testing.T) {
	req := GenerationRequest{Topic: "Go 并发", Count: 5, Difficulty: 3, QuestionType: "interview"}
	if err := validateGenerationRequest(req); err == nil {
		t.Fatal("expected missing source_job_id to fail")
	}
}

func TestValidateGenerationRequestAcceptsConceptFirstMVPFields(t *testing.T) {
	req := GenerationRequest{
		SourceJobID:     "imp-001",
		Topic:           "Go 并发",
		QuestionType:    "interview",
		Count:           5,
		Difficulty:      3,
		TargetDimension: "debugging",
		SkillCategory:   "go",
		Tags:            []string{"go", "concurrency"},
	}
	if err := validateGenerationRequest(req); err != nil {
		t.Fatalf("validateGenerationRequest: %v", err)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```powershell
go test ./internal/questionbank -run GenerationRequest -count=1
```

Expected: build fails because `GenerationRequest` and `validateGenerationRequest` do not exist.

- [ ] **Step 3: Implement minimal types and validation**

Create `generation_types.go` with:

```go
package questionbank

import (
	"errors"
	"fmt"
	"strings"
)

const (
	GenerationStatusCreated    = "created"
	GenerationStatusRetrieving = "retrieving"
	GenerationStatusDrafting   = "drafting"
	GenerationStatusGating     = "gating"
	GenerationStatusStaged     = "staged"
	GenerationStatusFailed     = "failed"

	GeneratedQuestionMetadataVersion = "generated_question_v1"
)

type GenerationRequest struct {
	SourceJobID     string   `json:"source_job_id"`
	Topic           string   `json:"topic"`
	QuestionType    string   `json:"question_type"`
	Count           int      `json:"count"`
	Difficulty      int      `json:"difficulty"`
	TargetDimension string   `json:"target_dimension,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	SkillCategory   string   `json:"skill_category,omitempty"`
}

type GenerationJob struct {
	ID         string              `json:"id"`
	Status     string              `json:"status"`
	Request    GenerationRequest   `json:"request"`
	Concepts   []ConceptCard       `json:"concepts,omitempty"`
	Candidates []QuestionCandidate `json:"candidates,omitempty"`
	Error      string              `json:"error,omitempty"`
}

type SourceRef struct {
	ChunkID string `json:"chunk_id"`
	Quote   string `json:"quote"`
}

type ConceptCard struct {
	ID             string      `json:"concept_id"`
	Title          string      `json:"title"`
	Skill          string      `json:"skill,omitempty"`
	SubSkill       string      `json:"sub_skill,omitempty"`
	DifficultyHint int         `json:"difficulty_hint,omitempty"`
	Keywords       []string    `json:"keywords,omitempty"`
	QuestionAngles []string    `json:"question_angles,omitempty"`
	EvidenceRefs   []SourceRef `json:"evidence_refs"`
}

type QuestionCandidate struct {
	ID              string            `json:"candidate_id,omitempty"`
	ConceptID       string            `json:"concept_id"`
	Content         string            `json:"content"`
	QuestionType    string            `json:"question_type"`
	TargetDimension string            `json:"target_dimension,omitempty"`
	Options         []string          `json:"options,omitempty"`
	Answer          string            `json:"answer,omitempty"`
	Explanation     string            `json:"explanation,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	SkillCategory   string            `json:"skill_category,omitempty"`
	Difficulty      int               `json:"difficulty,omitempty"`
	ExpectedPoints  []string          `json:"expected_points,omitempty"`
	Rubric          map[string]string `json:"rubric,omitempty"`
	SampleAnswer    string            `json:"sample_answer,omitempty"`
	FollowUpHints   []string          `json:"follow_up_hints,omitempty"`
	SourceRefs      []SourceRef       `json:"source_refs"`
	QualityFlags    []string          `json:"quality_flags,omitempty"`
}

func validateGenerationRequest(req GenerationRequest) error {
	if strings.TrimSpace(req.SourceJobID) == "" {
		return errors.New("source_job_id is required")
	}
	if strings.TrimSpace(req.Topic) == "" {
		return errors.New("topic is required")
	}
	if !validGenerationQuestionType(req.QuestionType) {
		return fmt.Errorf("unsupported question_type %q", req.QuestionType)
	}
	if req.Count < 1 || req.Count > 20 {
		return errors.New("count must be between 1 and 20")
	}
	if req.Difficulty < 1 || req.Difficulty > 5 {
		return errors.New("difficulty must be between 1 and 5")
	}
	if req.TargetDimension != "" && !validGenerationTargetDimension(req.TargetDimension) {
		return fmt.Errorf("unsupported target_dimension %q", req.TargetDimension)
	}
	return nil
}
```

Add whitelists for `interview`, `single_choice`, `short_answer` and dimensions `concept`, `principle`, `scenario`, `tradeoff`, `debugging`, `project_experience`, `system_design`.

- [ ] **Step 4: Run tests and commit**

Run:

```powershell
go test ./internal/questionbank -run GenerationRequest -count=1
```

Commit:

```powershell
git add -- internal/questionbank/generation_types.go internal/questionbank/generation_test.go
git commit -m "feat: add question generation request types"
```

## Task 2: Scoped Chunk Retrieval

**Files:**
- Create/modify: `internal/questionbank/generation_retrieval.go`
- Modify: `internal/questionbank/generation_test.go`

- [ ] **Step 1: Write failing retrieval tests**

Test that retrieval only uses chunks from the requested import job and returns no evidence when topic does not match.

- [ ] **Step 2: Implement lexical retrieval**

Implement:

```go
type RetrievedChunk struct {
	ID      string
	JobID   string
	Content string
	Score   float64
}

func retrieveGenerationChunks(ctx context.Context, store ImportStore, req GenerationRequest, limit int) ([]RetrievedChunk, error)
```

Use `ImportStore.ListChunks(req.SourceJobID)`. Score by normalized term matches from topic, tags, skill category, question type, and target dimension. Keep it deterministic.

- [ ] **Step 3: Run tests and commit**

Run:

```powershell
go test ./internal/questionbank -run GenerationRetrieval -count=1
```

Commit retrieval files.

## Task 3: Concept Card Extraction

**Files:**
- Create: `internal/questionbank/generation_llm.go`
- Modify: `internal/questionbank/generation_test.go`

- [ ] **Step 1: Write failing tests for concept parsing and grounding**

Cover:

- valid concept with quote in chunk passes;
- concept with foreign chunk ID fails;
- concept with quote not present in chunk fails;
- duplicate concept titles collapse.

- [ ] **Step 2: Implement concept extraction helpers**

Implement:

```go
func validateConceptCards(cards []ConceptCard, chunks []RetrievedChunk) ([]ConceptCard, []string)
func conceptID(jobID string, index int, card ConceptCard) string
```

Use backend-generated IDs.

- [ ] **Step 3: Add LLM call wrapper**

Add a method on `GenerationService`:

```go
func (s *GenerationService) extractConceptCards(ctx context.Context, req GenerationRequest, chunks []RetrievedChunk) ([]ConceptCard, []string, error)
```

Use `llm.CallWithSchema` only when `s.model != nil`. For nil model in tests, allow deterministic fallback concepts from chunks to keep service testable.

- [ ] **Step 4: Run tests and commit**

Run:

```powershell
go test ./internal/questionbank -run Concept -count=1
```

Commit concept extraction files.

## Task 4: QuestionCandidate Parsing And Quality Gates

**Files:**
- Create: `internal/questionbank/generation_quality.go`
- Modify: `internal/questionbank/generation_llm.go`
- Modify: `internal/questionbank/generation_test.go`

- [ ] **Step 1: Write failing tests**

Cover:

- missing source refs blocked;
- unknown concept blocked;
- quote not grounded blocked;
- duplicate content blocked;
- `请总结本文` blocked;
- `single_choice` without four options or single answer blocked;
- `interview` without follow-up hints blocked.

- [ ] **Step 2: Implement quality gate**

Implement:

```go
func gateQuestionCandidates(req GenerationRequest, concepts []ConceptCard, chunks []RetrievedChunk, candidates []QuestionCandidate) ([]QuestionCandidate, []QuestionCandidate)
```

Return passing and rejected candidates. Rejected candidates carry `QualityFlags`.

- [ ] **Step 3: Implement structured candidate parsing**

Use `llm.CallWithSchema` with JSON validation. The parser must accept only `{"candidates":[...]}`.

- [ ] **Step 4: Run tests and commit**

Run:

```powershell
go test ./internal/questionbank -run 'QuestionCandidate|QualityGate' -count=1
```

Commit quality gate files.

## Task 5: Generation Service And Staging

**Files:**
- Create: `internal/questionbank/generation_service.go`
- Modify: `internal/questionbank/imports_stage.go` only if an exported staging wrapper is required.
- Modify: `internal/questionbank/generation_test.go`

- [ ] **Step 1: Write failing service tests**

Cover:

- no retrieved chunks means no candidates and no LLM call;
- valid generation produces candidates;
- stage creates import items with `needs_human_review`;
- commit remains blocked until human accept.

- [ ] **Step 2: Implement `GenerationService.Generate`**

Pipeline:

```go
validateGenerationRequest
retrieveGenerationChunks
extractConceptCards
generateQuestionCandidates
gateQuestionCandidates
return GenerationJob
```

- [ ] **Step 3: Implement staging**

Map passing candidates to `Item`, build versioned generated metadata, and stage through the existing import review flow. If existing `stageItemsWithOriginalsAndProvenance` is unexported but in the same package, use it inside `GenerationService`.

- [ ] **Step 4: Run tests and commit**

Run:

```powershell
go test ./internal/questionbank -run GenerationService -count=1
```

Commit service and staging files.

## Task 6: HTTP API

**Files:**
- Modify: `internal/httpapi/question_bank.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/question_bank_test.go`
- Modify: server wiring files if required.

- [ ] **Step 1: Write failing handler tests**

Cover:

- invalid request returns 400;
- missing service returns 501;
- valid create returns generation job response;
- stage endpoint returns staged import job/items.

- [ ] **Step 2: Add server field and setter**

Add `questionGeneration` service field and `SetQuestionGenerationService`.

- [ ] **Step 3: Add routes and handlers**

Register:

```go
api.POST("/question-bank/generation-jobs", s.createQuestionGenerationJob)
api.GET("/question-bank/generation-jobs/:id", s.getQuestionGenerationJob)
api.POST("/question-bank/generation-jobs/:id/stage", s.stageQuestionGenerationJob)
```

- [ ] **Step 4: Wire server dependency**

In `cmd/server`, create the service with existing imports, writer, and model config. Keep mock-safe behavior in tests.

- [ ] **Step 5: Run tests and commit**

Run:

```powershell
go test ./internal/httpapi ./cmd/server -run 'Generation|QuestionBank' -count=1
```

Commit HTTP files.

## Task 7: Documentation And Final Verification

**Files:**
- Modify: `docs/SDD-Backend.md`
- Create: `docs/code-changes/06-07-rag-question-generation.md`
- Modify: `openspec/changes/add-rag-question-generation/tasks.md`

- [ ] **Step 1: Update docs**

Document the generation pipeline, API boundary, concept cards, quality gates, and staging review policy.

- [ ] **Step 2: Check off completed tasks**

Update `openspec/changes/add-rag-question-generation/tasks.md` only after implementation tasks are actually complete.

- [ ] **Step 3: Run full verification**

Run:

```powershell
go test ./internal/questionbank -count=1
go test ./internal/httpapi ./cmd/server -count=1
go test ./... -count=1
openspec validate add-rag-question-generation --strict
git diff --check
```

- [ ] **Step 4: Commit docs and task updates**

Use precise add, including forced add for ignored docs:

```powershell
git add -- openspec/changes/add-rag-question-generation/tasks.md docs/SDD-Backend.md
git add -f -- docs/code-changes/06-07-rag-question-generation.md
git commit -m "docs: record rag question generation workflow"
```

## Self-Review

- Spec coverage: all OpenSpec scenarios map to tasks 2-6.
- No frontend files are part of this plan.
- No schema migration is planned for MVP.
- Concept cards and question metadata are versioned.
- LLM output remains draft-only and must pass staging/human review before formal commit.
