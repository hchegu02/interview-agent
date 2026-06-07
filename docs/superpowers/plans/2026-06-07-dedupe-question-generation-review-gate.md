---
change: dedupe-question-generation-review-gate
design-doc: docs/superpowers/specs/2026-06-07-dedupe-question-generation-review-gate-design.md
base-ref: 715e8e5e426295f4e91e31674500c4f222b3be10
archived-with: 2026-06-07-dedupe-question-generation-review-gate
---

# Dedupe Question Generation Review Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent generated question duplicates from entering active question bank content through generation, staging, or commit.

**Architecture:** Add one shared normalized content-key boundary in `internal/questionbank`, then reuse it in generation gates and import commit guards. Keep the behavior conservative, exact-normalized, schema-free, and backend-only.

**Tech Stack:** Go, existing `internal/questionbank` memory/PG store abstractions, OpenSpec change `dedupe-question-generation-review-gate`.

## File Structure

- Modify `internal/questionbank/generation_quality.go`: add shared dedupe constants/helpers and extend gate logic to accept existing active content keys.
- Modify `internal/questionbank/generation_service.go`: load existing active question keys from `Writer` when it also implements `Store`; pass keys into generation gates.
- Modify `internal/questionbank/imports_commit.go`: add commit-time same-job and existing-active dedupe before `Writer.Upsert`.
- Modify `internal/questionbank/generation_test.go`: cover generation rejection against existing question-bank content and preserve same-batch behavior.
- Modify `internal/questionbank/imports_test.go`: cover commit duplicate skipping and review policy compatibility.
- Modify `openspec/changes/dedupe-question-generation-review-gate/tasks.md`: check off tasks as implementation lands.
- Create or update `docs/code-changes/06-07-dedupe-question-generation-review-gate.md`: record the real code diff after implementation.

## Task 1: Shared Normalized Content-Key Boundary

**Files:**
- Modify: `internal/questionbank/generation_quality.go`
- Test: `internal/questionbank/generation_test.go`

- [ ] **Step 1: Add focused helper tests**

Add tests near existing generation quality tests:

```go
func TestQuestionContentDedupeKeyNormalizesWhitespaceAndCase(t *testing.T) {
	got := questionContentDedupeKey(" Agent 效果如何评估？ ")
	want := questionContentDedupeKey("agent 效果如何评估？")
	if got == "" || got != want {
		t.Fatalf("dedupe key mismatch: got %q want %q", got, want)
	}
}

func TestQuestionContentDedupeKeyDoesNotMergeDistinctQuestions(t *testing.T) {
	first := questionContentDedupeKey("Go 服务如何设计超时？")
	second := questionContentDedupeKey("Go 服务如何设计熔断？")
	if first == "" || second == "" || first == second {
		t.Fatalf("distinct questions collapsed: first=%q second=%q", first, second)
	}
}
```

- [ ] **Step 2: Run the narrow tests and confirm failure**

Run:

```powershell
go test ./internal/questionbank -run "TestQuestionContentDedupeKey" -count=1
```

Expected before implementation: compile failure because `questionContentDedupeKey` does not exist.

- [ ] **Step 3: Implement the shared helper**

In `generation_quality.go`, add duplicate reason constants and replace direct use of `normalizeCandidateContent` with the shared helper:

```go
const (
	qualityFlagDuplicateContent         = "duplicate_content"
	qualityFlagDuplicateExistingContent = "duplicate_existing_content"
)

func questionContentDedupeKey(content string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(content))), "")
}

func normalizeCandidateContent(content string) string {
	return questionContentDedupeKey(content)
}
```

Keep `normalizeCandidateContent` as a compatibility wrapper if existing tests or callers reference it.

- [ ] **Step 4: Run helper tests**

Run:

```powershell
go test ./internal/questionbank -run "TestQuestionContentDedupeKey" -count=1
```

Expected: PASS.

## Task 2: Generation Gate Blocks Existing Active Questions

**Files:**
- Modify: `internal/questionbank/generation_quality.go`
- Modify: `internal/questionbank/generation_service.go`
- Test: `internal/questionbank/generation_test.go`

- [ ] **Step 1: Add gate-level failing test**

Add a test near `TestGateQuestionCandidatesRejectsDuplicateAndLowValueContent`:

```go
func TestGateQuestionCandidatesRejectsExistingQuestionBankDuplicate(t *testing.T) {
	req := GenerationRequest{QuestionType: "interview", Difficulty: 3}
	concepts := []ConceptCard{completeConceptCard("concept-001")}
	chunks := []RetrievedChunk{completeRetrievedChunk("chunk-001")}
	candidate := completeGenerationCandidate("concept-001", "Go 服务如何设计超时、重试和熔断，避免级联故障？")
	existing := map[string]struct{}{
		questionContentDedupeKey(" go 服务如何设计超时、重试和熔断，避免级联故障？ "): {},
	}

	passed, rejected := gateQuestionCandidates(req, concepts, chunks, []QuestionCandidate{candidate}, existing)
	if len(passed) != 0 || len(rejected) != 1 {
		t.Fatalf("passed=%d rejected=%d, want 0/1", len(passed), len(rejected))
	}
	if !contains(rejected[0].QualityFlags, qualityFlagDuplicateExistingContent) {
		t.Fatalf("flags = %+v, want %q", rejected[0].QualityFlags, qualityFlagDuplicateExistingContent)
	}
}
```

Update existing `gateQuestionCandidates` test calls to pass `nil` for the new existing-key argument.

- [ ] **Step 2: Run the gate test and confirm failure**

Run:

```powershell
go test ./internal/questionbank -run "TestGateQuestionCandidatesRejectsExistingQuestionBankDuplicate|TestGateQuestionCandidates" -count=1
```

Expected before implementation: compile failure because the function signature still has four arguments.

- [ ] **Step 3: Extend gate signature and flag logic**

Change signatures:

```go
func gateQuestionCandidates(req GenerationRequest, concepts []ConceptCard, chunks []RetrievedChunk, candidates []QuestionCandidate, existingContentKeys map[string]struct{}) ([]QuestionCandidate, []QuestionCandidate)

func candidateQualityFlags(req GenerationRequest, candidate QuestionCandidate, concepts map[string]ConceptCard, chunks map[string]RetrievedChunk, seenContent map[string]struct{}, existingContentKeys map[string]struct{}) []string
```

Inside `candidateQualityFlags`, after same-batch check:

```go
if key != "" {
	if _, ok := seenContent[key]; ok {
		flags = append(flags, qualityFlagDuplicateContent)
	}
	if _, ok := existingContentKeys[key]; ok {
		flags = append(flags, qualityFlagDuplicateExistingContent)
	}
}
```

Nil maps are safe for reads, so no special case is needed.

- [ ] **Step 4: Load existing active keys in generation service**

In `GenerationService.Generate`, before calling `gateQuestionCandidates`, derive keys:

```go
existingKeys, err := activeQuestionContentKeys(ctx, s.writer)
if err != nil {
	job.Warnings = append(job.Warnings, "existing question dedupe skipped: "+err.Error())
}
passed, rejected := gateQuestionCandidates(req, concepts, chunks, drafts, existingKeys)
```

Add helper in `generation_service.go` or a small questionbank file:

```go
func activeQuestionContentKeys(ctx context.Context, source any) (map[string]struct{}, error) {
	store, ok := source.(Store)
	if !ok || store == nil {
		return nil, nil
	}
	result, err := store.List(ctx, Filter{Status: "active", Limit: 100})
	if err != nil {
		return nil, err
	}
	keys := make(map[string]struct{}, len(result.Items))
	for _, item := range result.Items {
		if key := questionContentDedupeKey(item.Content); key != "" {
			keys[key] = struct{}{}
		}
	}
	return keys, nil
}
```

If implementation needs full pagination, add a cursor loop using `result.NextCursor` until empty.

- [ ] **Step 5: Run generation tests**

Run:

```powershell
go test ./internal/questionbank -run "TestGateQuestionCandidates|TestGenerationService" -count=1
```

Expected: PASS.

## Task 3: Commit Guard Blocks Same-Job And Existing Active Duplicates

**Files:**
- Modify: `internal/questionbank/imports_commit.go`
- Test: `internal/questionbank/imports_test.go`

- [ ] **Step 1: Add commit duplicate tests**

Add two tests near existing commit/review tests:

```go
func TestImportCommitSkipsDuplicateAcceptedItemsWithinJob(t *testing.T) {
	ctx := context.Background()
	imports := NewMemoryImportStore()
	writer := NewMemoryStore(nil)
	service := &ImportService{imports: imports, writer: writer}
	job, err := imports.CreateJob(ctx, newImportJob(ImportSourceDocument, "generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	job.Status = ImportStatusReady
	job, _ = imports.UpdateJob(ctx, job)
	items := []ImportItem{
		completeImportItem(job.ID, "q1", "Go 服务如何设计超时、重试和熔断，避免级联故障？"),
		completeImportItem(job.ID, "q2", " go 服务如何设计超时、重试和熔断，避免级联故障？ "),
	}
	for i := range items {
		items[i].ReviewStatus = ImportReviewStatusAccepted
		items[i].AgentReviewStatus = ImportAgentReviewAutoApproved
	}
	if err := imports.UpdateItems(ctx, items); err != nil {
		t.Fatal(err)
	}

	committed, err := service.Commit(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if committed.ImportedItems != 1 {
		t.Fatalf("ImportedItems = %d, want 1", committed.ImportedItems)
	}
	listed, err := writer.List(ctx, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 1 {
		t.Fatalf("active writer items = %d, want 1", len(listed.Items))
	}
}

func TestImportCommitSkipsExistingActiveQuestionDuplicate(t *testing.T) {
	ctx := context.Background()
	imports := NewMemoryImportStore()
	writer := NewMemoryStore([]Item{{
		ID:             "existing-1",
		Content:        "Go 服务如何设计超时、重试和熔断，避免级联故障？",
		SkillCategory:  "go",
		Difficulty:     3,
		Tags:           []string{"go"},
		ExpectedPoints: []string{"timeout"},
		Rubric:         map[string]string{"timeout": "提到超时"},
		SampleAnswer:   "设置超时、重试和熔断。",
		FollowUpHints:  []string{"如何避免重试风暴？"},
		Status:         "active",
	}})
	service := &ImportService{imports: imports, writer: writer}
	job, err := imports.CreateJob(ctx, newImportJob(ImportSourceDocument, "generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	job.Status = ImportStatusReady
	job, _ = imports.UpdateJob(ctx, job)
	item := completeImportItem(job.ID, "new-1", " go 服务如何设计超时、重试和熔断，避免级联故障？ ")
	item.ReviewStatus = ImportReviewStatusAccepted
	item.AgentReviewStatus = ImportAgentReviewAutoApproved
	if err := imports.UpdateItems(ctx, []ImportItem{item}); err != nil {
		t.Fatal(err)
	}

	committed, err := service.Commit(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if committed.ImportedItems != 0 {
		t.Fatalf("ImportedItems = %d, want 0", committed.ImportedItems)
	}
	listed, err := writer.List(ctx, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 1 || listed.Items[0].ID != "existing-1" {
		t.Fatalf("writer items = %+v, want only existing-1", listed.Items)
	}
}
```

If helper names differ in the current test file, adapt to the existing helper style instead of adding duplicate factories.

- [ ] **Step 2: Run commit tests and confirm failure**

Run:

```powershell
go test ./internal/questionbank -run "TestImportCommitSkipsDuplicateAcceptedItemsWithinJob|TestImportCommitSkipsExistingActiveQuestionDuplicate" -count=1
```

Expected before implementation: FAIL because duplicates are still imported.

- [ ] **Step 3: Implement commit filtering**

In `commitReadyJob`, before appending to `items`, load current active keys and track job keys:

```go
existingKeys, err := activeQuestionContentKeys(ctx, s.writer)
if err != nil {
	return s.failJob(ctx, job, fmt.Errorf("load active question dedupe keys: %w", err))
}
seenJobKeys := map[string]struct{}{}
```

Inside the import item loop:

```go
key := questionContentDedupeKey(item.Item.Content)
if key != "" {
	if _, ok := seenJobKeys[key]; ok {
		item.AgentReviewStatus = ImportAgentReviewRejected
		item.AgentReviewReason = qualityFlagDuplicateContent
		item.UpdatedAt = time.Now().UTC()
		updated = append(updated, item)
		continue
	}
	if _, ok := existingKeys[key]; ok {
		item.AgentReviewStatus = ImportAgentReviewRejected
		item.AgentReviewReason = qualityFlagDuplicateExistingContent
		item.UpdatedAt = time.Now().UTC()
		updated = append(updated, item)
		continue
	}
	seenJobKeys[key] = struct{}{}
}
items = append(items, item.Item)
item.Status = ImportItemStatusImported
item.UpdatedAt = time.Now().UTC()
updated = append(updated, item)
```

This keeps duplicate diagnostics in existing metadata and avoids schema changes.

- [ ] **Step 4: Run commit tests**

Run:

```powershell
go test ./internal/questionbank -run "TestImportCommitSkips|TestCommitSkipsAgent|TestReview" -count=1
```

Expected: PASS.

## Task 4: Documentation, Task Checkoff, And Full Package Verification

**Files:**
- Modify: `openspec/changes/dedupe-question-generation-review-gate/tasks.md`
- Create or modify: `docs/code-changes/06-07-dedupe-question-generation-review-gate.md`

- [ ] **Step 1: Update code change documentation**

Create `docs/code-changes/06-07-dedupe-question-generation-review-gate.md` after implementation. It must list the actual changed code files, function-level behavior changes, call chain, data flow, dependencies, tests, and risks based on the final diff.

- [ ] **Step 2: Check off OpenSpec tasks**

Update `openspec/changes/dedupe-question-generation-review-gate/tasks.md` by changing completed items from `- [ ]` to `- [x]` only after tests pass.

- [ ] **Step 3: Run required verification**

Run:

```powershell
go test ./internal/questionbank -count=1
```

Expected: PASS.

- [ ] **Step 4: Run OpenSpec validation**

Run:

```powershell
openspec validate dedupe-question-generation-review-gate --strict
```

Expected: PASS.

If `openspec` is unavailable in PowerShell, use the project’s existing OpenSpec command path and record the exact command and result in the final response.

## Self-Review

- Spec coverage: generation duplicate rejection, staged/commit duplicate protection, diagnostic reason preservation, review policy compatibility, and no schema/frontend change are covered.
- Placeholder scan: no implementation step depends on unspecified TODO work.
- Type consistency: plan uses existing `Store`, `Writer`, `Filter`, `ImportItem`, `ImportAgentReview*`, `ImportReviewStatus*`, and `QualityFlags` naming from `internal/questionbank`.
