---
change: harden-rag-import-fact-flow
design-doc: docs/superpowers/specs/2026-06-08-rag-import-fact-flow-design.md
base-ref: 1596104112a70dab9fc96495b58423d9e9d15224
---

# RAG Import Fact Flow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a production-grade backend fact flow for versioned question-bank imports, RAG eval real-query regression, and runtime retrieval decision policy.

**Architecture:** Keep `questionbank.Item` and `question_bank` as the formal fact model. Add a versioned import contract at the import boundary, file-based RAG eval tools under `cmd/rag-eval`, and a nodes-layer Runtime Retrieval Decision Policy that consumes `CandidatePool`, `RetrievalTrace`, WorkingMemory, and used question ids without changing the retriever contract.

**Tech Stack:** Go standard library, existing `internal/questionbank`, `cmd/rag-eval`, `internal/nodes`, `internal/retriever`, OpenSpec, existing Go tests.

---

## File Structure

- Modify `internal/questionbank/imports_parse.go`: parse versioned import packages and field-aware errors.
- Modify `internal/questionbank/imports_test.go`: contract tests for versioned package, legacy JSON, flexible fields, bad field types, and commit summary gates.
- Possibly modify `internal/questionbank/imports*.go`: commit summary and embedding/reindex diagnostic behavior if missing.
- Modify `cmd/rag-eval/main.go`: add mode dispatch for existing golden eval plus new export/candidate/metrics modes.
- Create `cmd/rag-eval/real_queries.go`: sanitized query export types and helpers.
- Create `cmd/rag-eval/candidate_pool.go`: candidate pool merge, dedupe, keyword/random-negative helpers.
- Modify `cmd/rag-eval/main_test.go`: tests for sanitizer, export, candidate pool, metrics compatibility.
- Create `internal/nodes/retrieval_decision.go`: Runtime Retrieval Decision Policy.
- Create `internal/nodes/retrieval_decision_test.go`: policy unit tests.
- Modify `internal/nodes/retrieve_rag.go`: pass used question ids into retrieval query when supported by existing query fields.
- Modify `internal/nodes/pick_next.go`: apply policy and second-layer used question filtering before selection.
- Modify probe-related nodes only if current trace/context path needs policy output.
- Modify `internal/domain/session.go` only if policy diagnostics need persisted fields; prefer `omitempty` and WorkingMemory degraded reasons.
- Update `docs/SDD-Backend.md`.
- Add `docs/code-changes/06-08-rag-import-fact-flow.md`.
- Update `openspec/changes/harden-rag-import-fact-flow/tasks.md` as tasks complete.

---

### Task 1: Versioned Import Contract

**Files:**
- Modify: `internal/questionbank/imports_parse.go`
- Modify: `internal/questionbank/imports_test.go`
- Update: `openspec/changes/harden-rag-import-fact-flow/tasks.md`

- [ ] **Step 1: Add failing tests for versioned import package**

Add tests in `internal/questionbank/imports_test.go`:

```go
func TestParseQuestionBankItemsAcceptsVersionedImportPackage(t *testing.T) {
	raw := []byte(`{
		"schema_version":"question_bank_import.v1",
		"source_ref":"obsidian/go.md",
		"review_policy":{"default_status":"needs_human_review"},
		"items":[{
			"id":"go-contract-001",
			"content":"Go map 并发读写为什么不安全？",
			"skill_category":"go",
			"difficulty":"4",
			"tags":"go；map；concurrency",
			"expected_points":"运行时检测；fatal error；sync.Map 或锁",
			"rubric":["说明 fatal error","说明工程规避方式"],
			"follow_up_hints":"如何定位线上 map 并发写？"
		}]
	}`)
	items, err := parseQuestionBankItems("contract.json", raw)
	if err != nil {
		t.Fatalf("parseQuestionBankItems: %v", err)
	}
	if len(items) != 1 || items[0].ID != "go-contract-001" {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Difficulty != 4 {
		t.Fatalf("Difficulty = %d, want 4", items[0].Difficulty)
	}
	if got := items[0].Rubric["point_1"]; got != "说明 fatal error" {
		t.Fatalf("rubric point_1 = %q", got)
	}
	if got := items[0].Tags; len(got) != 3 || got[1] != "map" {
		t.Fatalf("Tags = %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```powershell
go test ./internal/questionbank -run TestParseQuestionBankItemsAcceptsVersionedImportPackage -count=1
```

Expected before implementation: FAIL because `{schema_version, items}` is not handled by flexible package parsing.

- [ ] **Step 3: Implement versioned package parsing**

In `parseFlexibleJSONItems`, after wrapped `Items []json.RawMessage`, accept packages with `schema_version` and `items`. Keep extra metadata ignored at parse layer for now; staging metadata can be added in a later task if required by code shape.

Implementation shape:

```go
var wrapped struct {
	Items []json.RawMessage `json:"items"`
}
if wrappedErr := json.Unmarshal(raw, &wrapped); wrappedErr != nil {
	return nil, wrappedErr
}
records = wrapped.Items
```

If this already works after existing wrapped fallback, keep the code unchanged and add the test as contract coverage.

- [ ] **Step 4: Add field path error test**

Add:

```go
func TestParseQuestionBankItemsReportsFlexibleFieldNameOnBadType(t *testing.T) {
	raw := []byte(`{"items":[{
		"id":"bad-expected-points",
		"content":"Redis 热 key 如何治理？",
		"skill_category":"redis",
		"difficulty":3,
		"expected_points":{"primary":"发现热 key"}
	}]}`)
	_, err := parseQuestionBankItems("generated.json", raw)
	if err == nil {
		t.Fatal("parseQuestionBankItems should reject bad expected_points shape")
	}
	if !strings.Contains(err.Error(), "expected_points") {
		t.Fatalf("error = %v, want expected_points context", err)
	}
}
```

- [ ] **Step 5: Run contract tests**

Run:

```powershell
go test ./internal/questionbank -run "TestParseQuestionBankItems(AcceptsVersionedImportPackage|ToleratesLLMScalarDrift|ToleratesRubricArray|RejectsUnsupportedFlexibleStringList|ReportsFlexibleFieldNameOnBadType)" -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 1**

Stage only relevant files:

```powershell
git add internal/questionbank/imports_parse.go internal/questionbank/imports_test.go openspec/changes/harden-rag-import-fact-flow/tasks.md
git commit -m "feat: define question import contract parsing"
```

---

### Task 2: Publish Commit Transaction Diagnostics

**Files:**
- Inspect/modify: `internal/questionbank/imports*.go`
- Modify: `internal/questionbank/imports_test.go`
- Update: `openspec/changes/harden-rag-import-fact-flow/tasks.md`

- [ ] **Step 1: Locate commit summary types**

Run:

```powershell
rg -n "type .*Commit|Commit\\(|ImportedItems|embedding|Embedding" internal/questionbank
```

Expected: identify existing commit result/job fields before editing.

- [ ] **Step 2: Add failing test for skipped and embedding failed summary**

Use existing memory stores where possible. If the current commit result only exposes job counters, assert those counters and item diagnostic state. Test should prove:

- accepted valid item imports
- duplicate/dirty/rejected item skips
- embedding failure records status/error instead of silent success

Name:

```go
func TestImportCommitRecordsPublishTransactionDiagnostics(t *testing.T)
```

- [ ] **Step 3: Implement minimal summary/status additions**

Prefer existing fields:

- `ImportJob.ImportedItems`
- item `Status`
- item `AgentReviewStatus`
- item errors/reasons
- `question_bank` embedding status/model/error

Only add new struct fields if existing fields cannot represent `skipped` or embedding failure. Any new JSON fields must use `omitempty`.

- [ ] **Step 4: Run commit diagnostics tests**

Run:

```powershell
go test ./internal/questionbank -run "TestImport.*Commit|TestCommit.*Embedding|TestImportCommitRecordsPublishTransactionDiagnostics" -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit Task 2**

```powershell
git add internal/questionbank openspec/changes/harden-rag-import-fact-flow/tasks.md
git commit -m "feat: record question import publish diagnostics"
```

---

### Task 3: RAG Eval Real Query Export and Candidate Pool

**Files:**
- Modify: `cmd/rag-eval/main.go`
- Create: `cmd/rag-eval/real_queries.go`
- Create: `cmd/rag-eval/candidate_pool.go`
- Modify: `cmd/rag-eval/main_test.go`
- Update: `openspec/changes/harden-rag-import-fact-flow/tasks.md`

- [ ] **Step 1: Add tests for sanitizer**

Add tests:

```go
func TestSanitizeEvalTextRedactsSensitiveValues(t *testing.T) {
	in := "我是张三 email a@b.com phone 13800138000 token=abc123 https://internal.local/a"
	got := sanitizeEvalText(in)
	for _, forbidden := range []string{"a@b.com", "13800138000", "abc123", "https://internal.local/a"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitizeEvalText leaked %q in %q", forbidden, got)
		}
	}
}
```

- [ ] **Step 2: Add tests for query JSONL export**

Define `realQueryRecord` with fields:

```go
type realQueryRecord struct {
	QueryID   string `json:"query_id"`
	QueryText string `json:"query_text"`
	Skill     string `json:"skill,omitempty"`
	Phase     string `json:"phase,omitempty"`
	SourceRef string `json:"source_ref,omitempty"`
}
```

Test writing two records to JSONL and reading them back.

- [ ] **Step 3: Implement `real_queries.go`**

Implement:

```go
func sanitizeEvalText(value string) string
func writeRealQueryJSONL(path string, rows []realQueryRecord) error
func readRealQueryJSONL(path string) ([]realQueryRecord, error)
```

Use regexes for email, CN phone, URL, and key-value secrets.

- [ ] **Step 4: Add candidate pool merge tests**

Add:

```go
func TestBuildCandidatePoolMergesSources(t *testing.T)
```

Expected behavior:

- duplicate question id appears once
- sources include `vector` and `keyword`
- source rank/score maps are preserved

- [ ] **Step 5: Implement `candidate_pool.go`**

Define:

```go
type candidatePoolItem struct {
	QuestionID    string             `json:"question_id"`
	Sources       []string           `json:"sources"`
	SourceRanks   map[string]int     `json:"source_ranks,omitempty"`
	SourceScores  map[string]float64 `json:"source_scores,omitempty"`
}
```

Implement merge function with deterministic ordering by best rank then id.

- [ ] **Step 6: Add CLI mode dispatch without breaking current eval**

Extend `options` with `Mode string`, default `eval`. Add flag:

```go
flag.StringVar(&opts.Mode, "mode", "eval", "mode: eval, export-queries, candidate-pool")
```

Keep current behavior under `mode=eval`. For first implementation, `export-queries` can accept an input JSONL of records and sanitize/write output; later DB/session source can be added under same mode.

- [ ] **Step 7: Run rag-eval tests**

```powershell
go test ./cmd/rag-eval -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit Task 3**

```powershell
git add cmd/rag-eval openspec/changes/harden-rag-import-fact-flow/tasks.md
git commit -m "feat: add rag eval real query tooling"
```

---

### Task 4: Runtime Retrieval Decision Policy

**Files:**
- Create: `internal/nodes/retrieval_decision.go`
- Create: `internal/nodes/retrieval_decision_test.go`
- Modify: `internal/nodes/retrieve_rag.go`
- Modify: `internal/nodes/pick_next.go`
- Possibly modify: `internal/domain/session.go`
- Update: `openspec/changes/harden-rag-import-fact-flow/tasks.md`

- [ ] **Step 1: Add policy tests**

Create `internal/nodes/retrieval_decision_test.go` with tests:

```go
func TestRetrievalDecisionRemediesLowInfoWithHighConfidence(t *testing.T)
func TestRetrievalDecisionSwitchesOnLowInfoWeakRetrieval(t *testing.T)
func TestRetrievalDecisionDeepensNormalAnswerWithUsableRetrieval(t *testing.T)
func TestRetrievalDecisionFiltersUsedQuestions(t *testing.T)
func TestRetrievalDecisionFallbacksWhenPoolEmptyAfterFiltering(t *testing.T)
```

- [ ] **Step 2: Implement policy types**

In `retrieval_decision.go`:

```go
type RetrievalStrategy string

const (
	RetrievalStrategyDeepen      RetrievalStrategy = "deepen"
	RetrievalStrategyRemedy      RetrievalStrategy = "remedy"
	RetrievalStrategySwitchTopic RetrievalStrategy = "switch_topic"
	RetrievalStrategyFallback    RetrievalStrategy = "fallback"
	RetrievalStrategyEnd         RetrievalStrategy = "end"
)

type RetrievalDecisionPolicyOptions struct {
	HighConfidenceScore float64
	MinContextScore     float64
	ContextLimit         int
}

type RetrievalDecisionResult struct {
	Strategy             RetrievalStrategy
	IncludeContext       bool
	SelectedCandidateIDs []string
	ConsumedCandidateIDs []string
	Reason               string
	DegradedReason       string
	Pool                 []domain.Question
}
```

- [ ] **Step 3: Implement low-info and used-id helpers**

Implement deterministic helpers:

```go
func lowInformationAnswer(answer string) bool
func usedQuestionIDs(sess *domain.Session) map[string]struct{}
func filterUsedQuestions(pool []domain.Question, used map[string]struct{}) []domain.Question
```

Include main round question IDs and follow-up question IDs if represented as IDs; if follow-ups only store text, do not invent IDs.

- [ ] **Step 4: Wire second-layer filtering into `pick_next`**

Replace local asked map logic with shared helper if cleaner. Ensure existing tests still pass:

```powershell
go test ./internal/nodes -run "Test.*Pick|TestRetrievalDecision" -count=1
```

- [ ] **Step 5: Pass used ids into retrieval query where supported**

Inspect `retriever.Query`. If it already has exclude fields, populate them in `retrieve_rag.go`. If not, do not expand retriever interface in this task; rely on pick-time filter and note follow-up task.

- [ ] **Step 6: Record degraded reasons**

Use existing `markDegradedReason(mem, "pick", "...")` or add `rag_decision` key. Do not add public HTTP fields unless necessary.

- [ ] **Step 7: Run node tests**

```powershell
go test ./internal/nodes -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit Task 4**

```powershell
git add internal/nodes internal/domain openspec/changes/harden-rag-import-fact-flow/tasks.md
git commit -m "feat: add runtime retrieval decision policy"
```

---

### Task 5: Docs, SDD, and Verification

**Files:**
- Modify: `docs/SDD-Backend.md`
- Create: `docs/code-changes/06-08-rag-import-fact-flow.md`
- Modify: `openspec/changes/harden-rag-import-fact-flow/tasks.md`

- [ ] **Step 1: Update SDD**

In `docs/SDD-Backend.md`, update:

- `internal/questionbank` responsibility
- `cmd/rag-eval` responsibility
- RAG retrieval design section
- Agent Graph section for decision policy

- [ ] **Step 2: Add code-change document**

Create `docs/code-changes/06-08-rag-import-fact-flow.md` with:

- change overview
- changed files
- function-level notes
- call chain
- data flow
- dependencies and side effects
- tests
- risks

Use real diff only; do not claim unimplemented behavior.

- [ ] **Step 3: Run targeted tests**

```powershell
go test ./internal/questionbank ./internal/nodes ./cmd/rag-eval -count=1
```

Expected: PASS.

- [ ] **Step 4: Run full tests**

```powershell
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Validate OpenSpec**

```powershell
openspec validate harden-rag-import-fact-flow --strict
```

Expected: PASS.

- [ ] **Step 6: Commit Task 5**

Use `git add -f` for ignored docs:

```powershell
git add docs/SDD-Backend.md openspec/changes/harden-rag-import-fact-flow/tasks.md
git add -f docs/code-changes/06-08-rag-import-fact-flow.md
git commit -m "docs: record rag import fact flow behavior"
```

---

## Final Verification

- [ ] Run `go test ./...`.
- [ ] Run `openspec validate harden-rag-import-fact-flow --strict`.
- [ ] Run `git status --short --branch`.
- [ ] Confirm all `openspec/changes/harden-rag-import-fact-flow/tasks.md` tasks are checked.
- [ ] Run Comet build guard and proceed to verify phase.

