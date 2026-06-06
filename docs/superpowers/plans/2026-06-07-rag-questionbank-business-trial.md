---
change: rag-questionbank-business-trial
design-doc: docs/superpowers/specs/2026-06-07-rag-questionbank-business-trial-design.md
base-ref: 813a1752c462c0b3e7be39adc54b3377641f66dc
---

# RAG Question Bank Business Trial Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Go backend RAG question-bank internal business trial loop with Agent-visible source construction, query rewriting, HyDE shadow diagnostics, and trial gates.

**Architecture:** Keep the existing question-bank staging and commit flow as the formal write boundary. Add additive metadata for source provenance and Agent review, then add retrieval-time rewrite/HyDE diagnostics with fail-open fallback to the existing retrieval path.

**Tech Stack:** Go, PostgreSQL migrations, pgvector question bank, existing `internal/questionbank`, `internal/nodes`, `internal/retriever`, `cmd/rag-eval`, OpenSpec, Markdown runbooks.

---

## File Structure

- Modify `internal/questionbank/imports_types.go`: add source provenance and Agent review constants/types.
- Modify `internal/questionbank/imports_stage.go`: populate default Agent review metadata for generated/staged items.
- Modify `internal/questionbank/imports_commit.go`: ensure rejected Agent review items cannot commit.
- Modify `internal/questionbank/imports_clone.go`: deep-copy new metadata.
- Modify `internal/questionbank/imports_memory_store.go`: preserve new fields in memory tests.
- Modify `internal/questionbank/imports_pg.go`: persist new metadata through `raw_json` or schema-backed columns depending on implementation choice.
- Create migration only if schema-backed fields are required; prefer JSON-compatible additive storage first.
- Modify `internal/questionbank/imports_test.go`: cover Agent review state and provenance behavior.
- Modify `internal/retriever/retriever.go`: extend `RetrievalTrace` with query rewrite and HyDE diagnostics.
- Modify `internal/domain/session.go`: mirror trace additions for HTTP/session serialization.
- Modify `internal/nodes/retrieve_rag.go`: add rewrite orchestration before embedding and HyDE shadow diagnostics.
- Modify `internal/nodes/setup_test.go` or related retrieve tests: cover fallback semantics and trace fields.
- Modify `internal/config/config.go` and `config/config.yaml.example`: add retrieval enhancement config only if needed by runtime wiring.
- Modify `cmd/rag-eval/main.go` and tests if strategy comparison needs CLI support.
- Add or modify `testdata/rag/golden_queries_go_backend.jsonl`.
- Add `docs/ai/internal-trial/rag-questionbank-business-trial-runbook.md`.
- Add `docs/code-changes/06-07-rag-questionbank-business-trial.md` because this plan changes code.

## Task 1: Agent Review Metadata And Source Provenance

**Files:**
- Modify: `internal/questionbank/imports_types.go`
- Modify: `internal/questionbank/imports_clone.go`
- Modify: `internal/questionbank/imports_stage.go`
- Test: `internal/questionbank/imports_test.go`

- [ ] **Step 1: Write failing tests for Agent review defaults and source provenance cloning**

Add tests in `internal/questionbank/imports_test.go`:

```go
func TestStageItemsPreservesAgentReviewAndSourceProvenance(t *testing.T) {
	ctx := context.Background()
	service, store := newImportTestService(t)
	job := ImportJob{
		ID:         "job-agent-review",
		SourceType: ImportSourceDocument,
		Filename:   "go-runtime.md",
		Status:     ImportStatusValidating,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if _, err := store.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	items := []Item{{
		ID:             "go-runtime-001",
		Content:        "Go GMP 调度中 P 的作用是什么？",
		SkillCategory:  "go",
		Difficulty:     3,
		Tags:           []string{"go_concurrency", "scheduler"},
		ExpectedPoints: []string{"P 持有本地队列", "work stealing"},
	}}

	staged, err := service.stageItems(ctx, job, items, map[string]string{
		"source_type": "document",
		"source_hash": "sha256:abc",
	})
	if err != nil {
		t.Fatalf("stageItems: %v", err)
	}
	if len(staged) != 1 {
		t.Fatalf("staged len = %d, want 1", len(staged))
	}
	if staged[0].AgentReviewStatus != ImportAgentReviewNeedsHumanReview {
		t.Fatalf("agent review = %q, want %q", staged[0].AgentReviewStatus, ImportAgentReviewNeedsHumanReview)
	}
	if staged[0].SourceProvenance["source_hash"] != "sha256:abc" {
		t.Fatalf("source provenance = %+v", staged[0].SourceProvenance)
	}

	cloned := cloneImportItem(staged[0])
	cloned.SourceProvenance["source_hash"] = "changed"
	if staged[0].SourceProvenance["source_hash"] != "sha256:abc" {
		t.Fatalf("clone mutated original provenance: %+v", staged[0].SourceProvenance)
	}
}
```

If `stageItems` is not public or has a different signature, put the test next to the existing staging tests and adapt only the call site, not the assertion intent.

- [ ] **Step 2: Run the failing test**

Run:

```powershell
go test ./internal/questionbank -run TestStageItemsPreservesAgentReviewAndSourceProvenance -count=1
```

Expected: FAIL because `AgentReviewStatus`, `ImportAgentReviewNeedsHumanReview`, `SourceProvenance`, or the new `stageItems` metadata parameter does not exist.

- [ ] **Step 3: Add additive fields and constants**

In `internal/questionbank/imports_types.go`, add:

```go
const (
	ImportAgentReviewAutoApproved     = "auto_approved"
	ImportAgentReviewNeedsHumanReview = "needs_human_review"
	ImportAgentReviewRejected         = "rejected"
)
```

Extend `ImportItem`:

```go
AgentReviewStatus string            `json:"agent_review_status,omitempty"`
AgentReviewReason string            `json:"agent_review_reason,omitempty"`
SourceProvenance  map[string]string `json:"source_provenance,omitempty"`
```

- [ ] **Step 4: Deep-copy new fields**

In `internal/questionbank/imports_clone.go`, update `cloneImportItem`:

```go
item.SourceProvenance = cloneStringMap(item.SourceProvenance)
```

If no `cloneStringMap` helper exists, add a local helper in the same file:

```go
func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
```

- [ ] **Step 5: Populate default review state during staging**

In `internal/questionbank/imports_stage.go`, set generated document imports to `needs_human_review` by default. If the function currently has no metadata parameter, add a private helper instead of changing public API widely:

```go
func defaultAgentReviewStatus(sourceType string) string {
	if sourceType == ImportSourceDocument {
		return ImportAgentReviewNeedsHumanReview
	}
	return ""
}
```

When constructing `ImportItem`, set:

```go
AgentReviewStatus: defaultAgentReviewStatus(job.SourceType),
SourceProvenance:  cloneStringMap(sourceProvenance),
```

- [ ] **Step 6: Run test**

Run:

```powershell
go test ./internal/questionbank -run TestStageItemsPreservesAgentReviewAndSourceProvenance -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit Task 1**

```powershell
git add "internal/questionbank/imports_types.go" "internal/questionbank/imports_clone.go" "internal/questionbank/imports_stage.go" "internal/questionbank/imports_test.go"
git commit -m "feat: add agent review provenance for question imports"
```

## Task 2: Commit Filtering For Agent-Rejected Items

**Files:**
- Modify: `internal/questionbank/imports_commit.go`
- Test: `internal/questionbank/imports_test.go`

- [ ] **Step 1: Write failing test for rejected Agent review commit blocking**

Add:

```go
func TestCommitSkipsAgentRejectedItems(t *testing.T) {
	ctx := context.Background()
	service, _ := newImportTestService(t)
	job, err := service.Start(ctx, ImportFile{
		Filename:    "questions.json",
		ContentType: "application/json",
		Reader: strings.NewReader(`[{
			"id":"reject-agent-001",
			"content":"Go channel 关闭后接收行为是什么？",
			"skill_category":"go",
			"difficulty":3,
			"tags":["channel"],
			"expected_points":["zero value","ok flag","panic on send"]
		}]`),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	items, err := service.imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	items[0].AgentReviewStatus = ImportAgentReviewRejected
	items[0].AgentReviewReason = "not grounded in source"
	if err := service.imports.UpdateItems(ctx, items); err != nil {
		t.Fatalf("UpdateItems: %v", err)
	}

	committed, committedItems, err := service.Commit(ctx, job.ID)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if committed.ImportedItems != 0 {
		t.Fatalf("imported = %d, want 0", committed.ImportedItems)
	}
	for _, item := range committedItems {
		if item.QuestionID == "reject-agent-001" && item.Status == ImportItemStatusImported {
			t.Fatalf("agent rejected item was imported: %+v", item)
		}
	}
}
```

- [ ] **Step 2: Run failing test**

```powershell
go test ./internal/questionbank -run TestCommitSkipsAgentRejectedItems -count=1
```

Expected: FAIL because commit currently only checks validity and human review status.

- [ ] **Step 3: Implement Agent review filtering**

In `internal/questionbank/imports_commit.go`, update `importItemAccepted`:

```go
func importItemAccepted(item ImportItem) bool {
	if item.AgentReviewStatus == ImportAgentReviewRejected {
		return false
	}
	return item.ReviewStatus == "" || item.ReviewStatus == ImportReviewStatusAccepted
}
```

- [ ] **Step 4: Run test**

```powershell
go test ./internal/questionbank -run TestCommitSkipsAgentRejectedItems -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit Task 2**

```powershell
git add "internal/questionbank/imports_commit.go" "internal/questionbank/imports_test.go"
git commit -m "fix: block agent rejected question imports"
```

## Task 3: Retrieval Trace For Query Rewriting And HyDE

**Files:**
- Modify: `internal/retriever/retriever.go`
- Modify: `internal/domain/session.go`
- Modify: `internal/nodes/retrieve_rag.go`
- Test: `internal/nodes/setup_test.go`

- [ ] **Step 1: Write failing retrieve trace test**

Add a test near existing `retrieve_rag` tests:

```go
func TestRetrieveRAGRecordsRewriteAndHyDEShadowTrace(t *testing.T) {
	ctx := context.Background()
	sess := newRetrieveRAGSession()
	r := &fakePipelineRetriever{
		searchResult: retriever.PipelineResult{
			Results: []retriever.Result{{
				ID: "go-scheduler-001",
				Content: "Go GMP 调度中 P 的作用是什么？",
				Tags: []string{"go_concurrency", "scheduler"},
				Difficulty: 3,
				Category: "go",
			}},
			Trace: retriever.RetrievalTrace{
				Query: "Go 后端岗位中关于 GMP 调度的中级面试题",
			},
		},
	}
	node := NewRetrieveRAGPatchNode(embedding.NewMockEmbedder(8), r, RetrieveRAGOptions{
		TopK: 5,
		QueryRewriter: fixedQueryRewriter{
			out: QueryRewriteResult{
				RewrittenQuery: "Go 后端岗位中关于 GMP 调度的中级面试题",
				Reason: "normalize interview question style",
			},
		},
		HyDEMode: "shadow",
		HyDEGenerator: fixedHyDEGenerator{
			out: "Question: Go GMP 调度中 P 的作用是什么？\nExpected points: local queue, work stealing",
		},
	})

	patch, err := node(ctx, sess)
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	if patch.RetrievalTrace == nil {
		t.Fatal("retrieval trace missing")
	}
	if patch.RetrievalTrace.RewrittenQuery == "" {
		t.Fatalf("rewritten query missing: %+v", patch.RetrievalTrace)
	}
	if patch.RetrievalTrace.HyDEMode != "shadow" {
		t.Fatalf("hyde mode = %q, want shadow", patch.RetrievalTrace.HyDEMode)
	}
}
```

Define tiny fakes in the test file:

```go
type fixedQueryRewriter struct {
	out QueryRewriteResult
	err error
}

func (f fixedQueryRewriter) RewriteQuery(context.Context, QueryRewriteInput) (QueryRewriteResult, error) {
	return f.out, f.err
}

type fixedHyDEGenerator struct {
	out string
	err error
}

func (f fixedHyDEGenerator) GenerateHyDE(context.Context, HyDEInput) (string, error) {
	return f.out, f.err
}
```

- [ ] **Step 2: Run failing test**

```powershell
go test ./internal/nodes -run TestRetrieveRAGRecordsRewriteAndHyDEShadowTrace -count=1
```

Expected: FAIL because rewrite/HyDE interfaces and trace fields do not exist.

- [ ] **Step 3: Add trace fields**

In `internal/retriever/retriever.go` extend `RetrievalTrace`:

```go
OriginalQuery          string `json:"original_query,omitempty"`
RewrittenQuery         string `json:"rewritten_query,omitempty"`
QueryRewriteReason     string `json:"query_rewrite_reason,omitempty"`
QueryRewriteFallback   string `json:"query_rewrite_fallback,omitempty"`
HyDEMode               string `json:"hyde_mode,omitempty"`
HyDEStatus             string `json:"hyde_status,omitempty"`
HyDEFallback           string `json:"hyde_fallback,omitempty"`
HyDETextHash           string `json:"hyde_text_hash,omitempty"`
```

Mirror compatible fields in `internal/domain/session.go` `RetrievalTrace`.

- [ ] **Step 4: Add small interfaces and options in retrieve_rag**

In `internal/nodes/retrieve_rag.go`, add:

```go
type QueryRewriter interface {
	RewriteQuery(context.Context, QueryRewriteInput) (QueryRewriteResult, error)
}

type QueryRewriteInput struct {
	Query string
	JobTitle string
	Tags []string
	TargetDifficulty int
}

type QueryRewriteResult struct {
	RewrittenQuery string
	NormalizedTags []string
	Reason string
}

type HyDEGenerator interface {
	GenerateHyDE(context.Context, HyDEInput) (string, error)
}

type HyDEInput struct {
	Query string
	Tags []string
	TargetDifficulty int
}
```

Extend `RetrieveRAGOptions`:

```go
QueryRewriter QueryRewriter
HyDEMode string
HyDEGenerator HyDEGenerator
```

- [ ] **Step 5: Wire rewrite before embedding and HyDE shadow after query selection**

In `NewRetrieveRAGPatchNode`, preserve `originalQueryText := queryText`. If `opts.QueryRewriter != nil`, call it before embedding. On success, replace `queryText`; on failure, keep original.

HyDE shadow should compute a hash only and not change `query.QueryEmbedding`:

```go
hydeStatus := ""
hydeHash := ""
if opts.HyDEMode == "shadow" && opts.HyDEGenerator != nil {
	text, hydeErr := opts.HyDEGenerator.GenerateHyDE(ctx, HyDEInput{Query: queryText, Tags: queryTags, TargetDifficulty: targetDiff})
	if hydeErr != nil {
		hydeStatus = "fallback"
	} else {
		hydeStatus = "shadow"
		hydeHash = shortTextHash(text)
	}
}
```

Add `shortTextHash` using `crypto/sha256` and 12 hex chars.

- [ ] **Step 6: Copy trace fields into domain trace conversion**

Update `toDomainRetrievalTrace` to copy rewrite and HyDE fields.

- [ ] **Step 7: Run targeted node test**

```powershell
go test ./internal/nodes -run TestRetrieveRAGRecordsRewriteAndHyDEShadowTrace -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit Task 3**

```powershell
git add "internal/retriever/retriever.go" "internal/domain/session.go" "internal/nodes/retrieve_rag.go" "internal/nodes/setup_test.go"
git commit -m "feat: trace rag query rewrite and hyde shadow"
```

## Task 4: Eval Fixtures And Trial Runbook

**Files:**
- Add: `testdata/rag/golden_queries_go_backend.jsonl`
- Add: `docs/ai/internal-trial/rag-questionbank-business-trial-runbook.md`
- Modify: `docs/ai/internal-trial-launch-checklist.md`

- [ ] **Step 1: Add Go backend golden query fixture**

Create `testdata/rag/golden_queries_go_backend.jsonl` with at least these cases:

```jsonl
{"id":"go-scheduler","query":"Go GMP 调度模型里 P 的作用是什么","tags":["go_concurrency","scheduler"],"expected_ids":["go-scheduler-001"],"group":"go-runtime"}
{"id":"go-channel-close","query":"channel 关闭后接收和发送分别会发生什么","tags":["channel","go_concurrency"],"expected_ids":["go-channel-001"],"group":"go-runtime"}
{"id":"redis-aof-rewrite","query":"Redis AOF rewrite 期间新写入命令怎么处理","tags":["redis_persistence","aof"],"expected_ids":["redis-aof-001"],"group":"redis"}
{"id":"mysql-transaction","query":"MySQL 可重复读隔离级别如何避免幻读","tags":["mysql","transaction"],"expected_ids":["mysql-tx-001"],"group":"database"}
```

If current seed IDs differ, use IDs that exist in `seeds/question_bank.json`; do not invent expected IDs that eval cannot match.

- [ ] **Step 2: Run fixture sanity check**

```powershell
go run ./cmd/rag-eval -cases testdata/rag/golden_queries_go_backend.jsonl -config config/config.yaml.example -out tmp/eval/rag-go-backend
```

Expected: command exits 0 if fixture IDs and config are valid. If thresholds fail because the seed lacks matching IDs, update expected IDs to real seed IDs or add seed items under a separate approved task.

- [ ] **Step 3: Add trial runbook**

Create `docs/ai/internal-trial/rag-questionbank-business-trial-runbook.md` with:

```md
# RAG Question Bank Business Trial Runbook

## Scope

This trial validates Go backend question-bank construction and retrieval. It is an internal trial, not production release.

## Operator Flow

1. Import Go backend source material.
2. Let Agent generate and enrich question drafts.
3. Review `auto_approved`, `needs_human_review`, and `rejected` states.
4. Batch-confirm safe items only.
5. Commit approved items.
6. Run questionbank lint.
7. Run RAG eval.
8. Start internal interview trial with the Go backend question-bank filter.

## Required Gates

- `go test ./internal/questionbank ./internal/nodes ./internal/retriever -count=1`
- `go run ./cmd/questionbank-lint -seed seeds/question_bank.json -min-expected-points 3 -min-scenario-ratio 0.8`
- `go run ./cmd/rag-eval -cases testdata/rag/golden_queries_go_backend.jsonl -config config/config.yaml.example -out tmp/eval/rag-go-backend`
- `openspec validate rag-questionbank-business-trial --strict`

## Go/No-Go

Go only if generated questions are source-grounded, rejected items stay out of commit, retrieval trace explains rewrite/HyDE status, and eval does not regress.
```

- [ ] **Step 4: Link runbook from launch checklist**

Add one line to `docs/ai/internal-trial-launch-checklist.md` under trial documents:

```md
- `docs/ai/internal-trial/rag-questionbank-business-trial-runbook.md`
```

- [ ] **Step 5: Commit Task 4**

```powershell
git add "testdata/rag/golden_queries_go_backend.jsonl" "docs/ai/internal-trial/rag-questionbank-business-trial-runbook.md" "docs/ai/internal-trial-launch-checklist.md"
git commit -m "docs: add rag question bank trial runbook"
```

## Task 5: Code Change Documentation And Final Verification

**Files:**
- Add: `docs/code-changes/06-07-rag-questionbank-business-trial.md`
- Modify: `openspec/changes/rag-questionbank-business-trial/tasks.md`

- [ ] **Step 1: Write code-change document from real diff**

Create `docs/code-changes/06-07-rag-questionbank-business-trial.md` after implementation, using actual `git diff base-ref..HEAD` facts. Include:

```md
# 06-07 RAG 题库业务试用

## 变更概述

说明 Agent-first 题库构建、Query Rewriting、HyDE shadow 和试用门禁的真实代码改动。

## 变更文件

列出每个修改/新增代码文件和作用。

## 函数级说明

逐个说明新增或修改的函数、方法、接口、配置字段和测试。

## 调用链

从题库导入 API、ImportService、retrieve_rag 节点和 rag-eval CLI 写到目标函数。

## 数据流

说明 source provenance、Agent review state、query rewrite、HyDE diagnostics 的来源、转换、存储和返回。

## 依赖与副作用

说明 LLM、embedding、PG、配置、日志、trace 和文件系统影响。

## 测试

记录实际执行命令和结果。

## 风险

说明兼容性、安全、性能、并发和已知限制。
```

- [ ] **Step 2: Run targeted tests**

```powershell
go test ./internal/questionbank ./internal/nodes ./internal/retriever -count=1
```

Expected: PASS.

- [ ] **Step 3: Run RAG/question-bank gates**

```powershell
go run ./cmd/questionbank-lint -seed seeds/question_bank.json -min-expected-points 3 -min-scenario-ratio 0.8
go run ./cmd/rag-eval -cases testdata/rag/golden_queries_go_backend.jsonl -config config/config.yaml.example -out tmp/eval/rag-go-backend
```

Expected: PASS, or document exact failure and fix before proceeding.

- [ ] **Step 4: Run OpenSpec validation**

```powershell
openspec validate rag-questionbank-business-trial --strict
```

Expected: PASS.

- [ ] **Step 5: Mark OpenSpec tasks complete**

Update `openspec/changes/rag-questionbank-business-trial/tasks.md` checkboxes to `- [x]` only after corresponding work and verification are complete.

- [ ] **Step 6: Commit final docs and task status**

```powershell
git add "docs/code-changes/06-07-rag-questionbank-business-trial.md" "openspec/changes/rag-questionbank-business-trial/tasks.md"
git commit -m "chore: document rag question bank business trial"
```

## Self-Review

- Spec coverage: covered source-document construction, Agent review states, Query Rewriting, HyDE shadow/enabled boundary, Go backend eval gates, and trial runbook.
- Placeholder scan: no TBD/TODO placeholders are intentionally left in the plan.
- Type consistency: proposed names use existing `ImportItem`, `RetrievalTrace`, and `RetrieveRAGOptions` extension points. If implementation discovers better local names, update this plan and the OpenSpec delta spec before coding.
