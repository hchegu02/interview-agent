---
change: question-bank-import-enrichment
design-doc: docs/superpowers/specs/2026-05-28-question-bank-import-enrichment-design.md
base-ref: 6e3ec2efa2668b54948702a9377f43dd194437eb
---

# Question Bank Import Enrichment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Local question-bank imports with only question text should be enriched by LLM before staging, while no-LLM imports keep legacy defaults.

**Architecture:** Keep the existing import pipeline and insert enrichment between parsing and staging. Enrichment only fills missing metadata and never replaces uploaded `id` or `content`.

**Tech Stack:** Go, existing `questionbank.ImportService`, existing `llm.ChatModel`, Go unit tests.

---

### Task 1: Regression Tests

**Files:**
- Modify: `internal/questionbank/imports_test.go`
- Test: `internal/questionbank/imports_test.go`

- [x] **Step 1: Add LLM enrichment test**

Add a test named `TestImportService_ImportLocalQuestionOnlyUsesLLMEnrichment` that creates an `ImportService` with `llm.NewMockChatModel("")`, imports:

```json
[
  {"id":"question-only-001","content":"Go map 并发读写为什么会 panic？"}
]
```

Expected assertions:

```go
if item.ID != "question-only-001" || item.Content != "Go map 并发读写为什么会 panic？" {
	t.Fatalf("enrichment should preserve id/content: %+v", item)
}
if item.SkillCategory == "" || item.SkillCategory == "general" {
	t.Fatalf("SkillCategory = %q, want LLM-enriched category", item.SkillCategory)
}
if len(item.Tags) == 0 || len(item.ExpectedPoints) == 0 || len(item.Rubric) == 0 || len(item.FollowUpHints) == 0 || item.SampleAnswer == "" {
	t.Fatalf("item was not enriched enough: %+v", item)
}
```

- [x] **Step 2: Add no-LLM fallback test**

Add a test named `TestImportService_ImportLocalQuestionOnlyWithoutLLMFallsBackToDefaults` that creates an `ImportService` without `Model`, imports:

```json
[
  {"id":"question-only-002","content":"Redis 缓存击穿怎么处理？"}
]
```

Expected assertions:

```go
if item.SkillCategory != "general" || item.Difficulty != 3 {
	t.Fatalf("fallback item = %+v, want old default metadata", item)
}
if len(item.ExpectedPoints) != 0 || len(item.Rubric) != 0 || len(item.FollowUpHints) != 0 {
	t.Fatalf("fallback should not invent rich metadata without LLM: %+v", item)
}
```

- [x] **Step 3: Run red test**

Run:

```powershell
go test ./internal/questionbank -run "TestImportService_ImportLocalQuestionOnly" -count=1
```

Expected before implementation: enrichment test fails with `SkillCategory = "general"`.

### Task 2: Import Enrichment Implementation

**Files:**
- Modify: `internal/questionbank/imports.go`

- [x] **Step 1: Call enrichment after parsing**

In `processLocalQuestionBank`, call:

```go
items, err = s.enrichLocalItems(ctx, items)
if err != nil {
	return s.failJob(ctx, job, err)
}
```

before `stageItems`.

- [x] **Step 2: Add enrichment detector**

Add:

```go
func needsEnrichment(item Item) bool {
	return strings.TrimSpace(item.SkillCategory) == "" ||
		strings.TrimSpace(item.SkillCategory) == "general" ||
		item.Difficulty == 0 ||
		len(item.Tags) == 0 ||
		len(item.ExpectedPoints) == 0 ||
		len(item.Rubric) == 0 ||
		strings.TrimSpace(item.SampleAnswer) == "" ||
		len(item.FollowUpHints) == 0
}
```

- [x] **Step 3: Add conservative merge**

Add:

```go
func mergeEnrichedItem(base, enriched Item) Item {
	if strings.TrimSpace(base.SkillCategory) == "" || strings.TrimSpace(base.SkillCategory) == "general" {
		base.SkillCategory = enriched.SkillCategory
	}
	if base.Difficulty == 0 {
		base.Difficulty = enriched.Difficulty
	}
	if len(base.Tags) == 0 {
		base.Tags = enriched.Tags
	}
	if len(base.ExpectedPoints) == 0 {
		base.ExpectedPoints = enriched.ExpectedPoints
	}
	if len(base.Rubric) == 0 {
		base.Rubric = enriched.Rubric
	}
	if strings.TrimSpace(base.SampleAnswer) == "" {
		base.SampleAnswer = enriched.SampleAnswer
	}
	if len(base.FollowUpHints) == 0 {
		base.FollowUpHints = enriched.FollowUpHints
	}
	return base
}
```

- [x] **Step 4: Add `enrichLocalItems`**

Implement `enrichLocalItems` so it:

- returns original items when `s.model == nil`
- sends only incomplete items to LLM
- asks for JSON-only `items`
- validates with `validateItemsJSON`
- parses with `parseQuestionBankItems`
- matches enriched items by `id`, then exact `content`
- preserves original `id` and `content`

### Task 3: Mock LLM Support

**Files:**
- Modify: `internal/llm/mock.go`

- [x] **Step 1: Add metadata enrichment fixture**

In `builtinDemoResponse`, add a case matching:

```go
strings.Contains(prompt, "题库元数据补全助手") || strings.Contains(prompt, "补齐题库元数据")
```

Return one enriched item for `question-only-001` with:

- `tags`: `go`, `map`, `concurrency`
- `skill_category`: `go`
- `difficulty`: `3`
- `expected_points`
- `rubric`
- `sample_answer`
- `follow_up_hints`

### Task 4: Documentation And Verification

**Files:**
- Create: `docs/code-changes/05-28-question-bank-import-enrichment.md`
- Modify: `internal/questionbank/imports.go`
- Modify: `internal/questionbank/imports_test.go`
- Modify: `internal/llm/mock.go`

- [x] **Step 1: Format Go files**

Run:

```powershell
gofmt -w internal/questionbank/imports.go internal/questionbank/imports_test.go internal/llm/mock.go
```

- [x] **Step 2: Run targeted tests**

Run:

```powershell
go test ./internal/questionbank ./internal/httpapi -count=1
```

Expected: both packages pass.

- [x] **Step 3: Run full Go test suite**

Run:

```powershell
go test ./... -count=1
```

Expected: all Go packages pass.

- [x] **Step 4: Run frontend typecheck and build**

Run:

```powershell
npm run typecheck
npm run build
```

Expected: TypeScript build and Vite production build pass.

- [x] **Step 5: Run diff whitespace check**

Run:

```powershell
git diff --check
```

Expected: no whitespace errors. CRLF warnings are acceptable on this Windows workspace.
