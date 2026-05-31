# Production Readiness Roadmap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the four-stage production-readiness roadmap: verification gate, actionable RAG group quality gates, candidate training loop coverage, and minimal deployment diagnostics.

**Architecture:** Keep each stage as a clean commit boundary. Stage 1 centralizes existing verification commands. Stage 2 extends the existing `cmd/rag-eval` aggregator instead of replacing it. Stage 3 strengthens the current report/drill flow using existing `Report.DrillPlan` data. Stage 4 adds optional tracing through a small internal interface and graph callback, leaving default behavior unchanged.

**Tech Stack:** Go, Gin, existing graph callbacks, React/Vite/Vitest, Makefile with PowerShell-friendly targets, GitHub Actions, existing `cmd/rag-eval`, existing observability package.

---

## File Structure

### Stage 1 Files

- Modify `Makefile`: add `verify-local` phony target that runs dependency-free checks.
- Create `.github/workflows/ci.yml`: dependency-free CI for Go, web tests/build, RAG eval, questionbank lint, mock eval.
- Modify `README.md`: add one Windows-first pre-commit verification command.

### Stage 2 Files

- Modify `cmd/rag-eval/main.go`: add stable `Groups`, `WorstGroups`, grouped threshold options, and report output.
- Modify `cmd/rag-eval/main_test.go`: add tests for group aggregation, minimum case count, and group gate failure diagnostics.
- Modify `Makefile`: pass group gate flags only after tests prove current seed/golden data is stable.
- Modify `README.md`: document group-level RAG diagnostics.

### Stage 3 Files

- Modify `web/src/candidatePages.tsx`: make drill plan actions easier to test and expose a preview summary.
- Modify `web/src/sharedView.test.tsx` or create `web/src/candidatePages.test.tsx`: test report drill and question jump behavior.
- Modify `web/src/draftStore.test.ts`: extend drill JD coverage for recommended question IDs and weak skills.
- Modify `cmd/demo/output.go` and `cmd/demo/main_test.go` only if the existing demo artifacts do not clearly expose drill plan evidence.
- Modify `README.md`: document the candidate loop if user-facing commands or outputs change.

### Stage 4 Files

- Create `internal/observability/tracing.go`: define small tracing interface and no-op implementation.
- Create `internal/observability/tracing_test.go`: cover no-op and fake recorder behavior.
- Create `internal/observability/tracing_callback.go`: graph callback that records node spans.
- Create `internal/observability/tracing_callback_test.go`: cover graph span start/end/error.
- Modify `cmd/server/interview_wiring.go`: append tracing callback when tracing is configured.
- Modify `cmd/server/main.go` and `internal/config`: add optional tracing config only if necessary; default must be no-op.
- Create `deploy/helm/interview-agent/Chart.yaml`, `values.yaml`, `templates/deployment.yaml`, `templates/service.yaml` if Helm is selected during implementation.
- Modify `README.md`: document trace, health, load, and chaos paths.

---

## Stage 1: Production Verification Gate

### Task 1: Add Local Verification Target

**Files:**
- Modify: `Makefile`
- Test: Makefile dry-run and actual target execution

- [ ] **Step 1: Add the failing expectation**

Run:

```powershell
mingw32-make -n verify-local
```

Expected before implementation:

```text
mingw32-make: *** No rule to make target 'verify-local'.  Stop.
```

- [ ] **Step 2: Add `verify-local` to `.PHONY` and target body**

Modify the first line of `Makefile` to include `verify-local`:

```makefile
.PHONY: help tidy build web-build run test test-core test-race lint verify-local eval-rag questionbank-lint questionbank-lint-strict eval-mock migrate-up migrate-down seed real-rag-reindex demo demo-web demo-web-real demo-pg demo-pg-full demo-mock demo-real demo-real-full e2e-smoke load-test chaos-dry-run docker-up docker-up-cluster docker-down clean
```

Add after `lint`:

```makefile
verify-local: ## run dependency-free local quality gate
	$(GO) test ./... -count=1
	npm --prefix web run test
	npm --prefix web run build
	$(MAKE) eval-rag
	$(MAKE) questionbank-lint
	$(MAKE) eval-mock
	git diff --check
```

- [ ] **Step 3: Verify dry-run**

Run:

```powershell
mingw32-make -n verify-local
```

Expected: output includes `go test ./... -count=1`, `npm --prefix web run test`, `npm --prefix web run build`, `mingw32-make eval-rag`, `mingw32-make questionbank-lint`, `mingw32-make eval-mock`, and `git diff --check`.

- [ ] **Step 4: Verify target help**

Run:

```powershell
mingw32-make help
```

Expected: output includes:

```text
verify-local      run dependency-free local quality gate
```

### Task 2: Add Dependency-Free CI Workflow

**Files:**
- Create: `.github/workflows/ci.yml`
- Test: YAML shape and local command parity

- [ ] **Step 1: Create CI workflow**

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  pull_request:
  push:
    branches:
      - main
      - "codex/**"

jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Set up Node
        uses: actions/setup-node@v4
        with:
          node-version: 22
          cache: npm
          cache-dependency-path: web/package-lock.json

      - name: Install web dependencies
        run: npm ci --prefix web

      - name: Go tests
        run: go test ./... -count=1

      - name: Web tests
        run: npm --prefix web run test

      - name: Web build
        run: npm --prefix web run build

      - name: RAG eval
        run: go run ./cmd/rag-eval -cases testdata/rag/golden_queries.jsonl -config config/config.yaml.example -out tmp/eval/rag -min-recall-at-5 0.70 -min-recall-at-10 0.80 -min-mrr-at-k 0.90 -min-ndcg-at-k 0.75

      - name: Question bank lint
        run: go run ./cmd/questionbank-lint -seed seeds/question_bank.json -min-expected-points 3 -min-scenario-ratio 0.05

      - name: Mock eval
        run: go run ./cmd/eval -suite testdata/eval -mode mock -out tmp/eval/mock

      - name: Whitespace check
        run: git diff --check
```

- [ ] **Step 2: Verify package lock exists**

Run:

```powershell
Test-Path "web\package-lock.json"
```

Expected: `True`. If it returns `False`, change the CI `Install web dependencies` step to `npm install --prefix web` and remove npm cache config.

- [ ] **Step 3: Verify workflow file is tracked by status**

Run:

```powershell
git status --short ".github/workflows/ci.yml"
```

Expected: `?? .github/workflows/ci.yml`.

### Task 3: Document Verification Gate

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add README section under `## 测试`**

Insert before the existing full test block:

```markdown
提交前本地门禁：

```powershell
mingw32-make verify-local
```

该命令只依赖 mock/default 路径，不需要 PG、Redis、Docker 或 API key；CI 使用同一组无外部依赖检查。
```

- [ ] **Step 2: Verify docs mention the gate once**

Run:

```powershell
rg -n "verify-local" "README.md" "Makefile" ".github/workflows/ci.yml"
```

Expected: hits in all three files.

### Task 4: Stage 1 Verification And Commit

**Files:**
- Stage 1 files only

- [ ] **Step 1: Run verification**

Run:

```powershell
mingw32-make verify-local
```

Expected: exit code 0.

- [ ] **Step 2: Run diff check**

Run:

```powershell
git diff --check
```

Expected: exit code 0, allowing only LF-to-CRLF warnings if Git emits them as warnings.

- [ ] **Step 3: Commit**

Run after user confirms commit:

```powershell
git add -- "Makefile" ".github/workflows/ci.yml" "README.md"
git commit -m "ci: add production readiness verification gate"
```

---

## Stage 2: RAG Quality Group Gates

### Task 5: Add Group Gate Types And Tests

**Files:**
- Modify: `cmd/rag-eval/main_test.go`
- Modify later: `cmd/rag-eval/main.go`

- [ ] **Step 1: Write failing test for stable group output**

Add to `cmd/rag-eval/main_test.go`:

```go
func TestAggregateBuildsStableGroupsAndWorstGroups(t *testing.T) {
	s := aggregate([]caseResult{
		{ID: "go-q1", Tags: []string{"go", "concurrency"}, Skill: "go", RecallAt5: 1, RecallAt10: 1, MRRAtK: 1, NDCGAtK: 1, LatencyMS: 10},
		{ID: "redis-q1", Tags: []string{"redis", "cache"}, Skill: "redis", RecallAt5: 0, RecallAt10: 0.5, MRRAtK: 0.5, NDCGAtK: 0.5, LatencyMS: 20},
	}, 10, "seed")

	if got := s.Groups["skill:redis"].RecallAt5; got != 0 {
		t.Fatalf("skill redis recall@5 = %f, want 0", got)
	}
	if got := s.Groups["tag:cache"].CaseCount; got != 1 {
		t.Fatalf("tag cache cases = %d, want 1", got)
	}
	worst := worstGroups(s.Groups, groupGateOptions{
		MinCases:     1,
		MinRecallAt5: 0.8,
		Limit:        3,
	})
	if len(worst) != 1 || worst[0].Group != "skill:redis" || worst[0].Metric != "recall_at_5" {
		t.Fatalf("worst groups = %+v", worst)
	}
}
```

- [ ] **Step 2: Run failing test**

Run:

```powershell
go test ./cmd/rag-eval -run TestAggregateBuildsStableGroupsAndWorstGroups -count=1
```

Expected: FAIL because `summary.Groups`, `groupGateOptions`, and `worstGroups` do not exist.

### Task 6: Implement Group Summary And Worst Groups

**Files:**
- Modify: `cmd/rag-eval/main.go`

- [ ] **Step 1: Extend summary types**

Change `summary` to include:

```go
	Groups      map[string]groupMetric `json:"groups,omitempty"`
	WorstGroups []groupFailure         `json:"worst_groups,omitempty"`
```

Keep `BySkill` and `ByTag` during this stage for compatibility.

Add:

```go
type groupFailure struct {
	Group     string  `json:"group"`
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Cases     int     `json:"cases"`
}

type groupGateOptions struct {
	MinCases      int
	MinRecallAt5  float64
	MinRecallAt10 float64
	MinMRRAtK     float64
	MinNDCGAtK    float64
	Limit         int
}
```

- [ ] **Step 2: Build prefixed groups in `aggregate`**

Add after `ByTag` assignment:

```go
	out.Groups = map[string]groupMetric{}
	for skill, metric := range out.BySkill {
		out.Groups["skill:"+skill] = metric
	}
	for tag, metric := range out.ByTag {
		out.Groups["tag:"+tag] = metric
	}
```

- [ ] **Step 3: Add worst group helper**

Add:

```go
func worstGroups(groups map[string]groupMetric, opts groupGateOptions) []groupFailure {
	if opts.Limit <= 0 {
		opts.Limit = 5
	}
	var failures []groupFailure
	for name, g := range groups {
		if opts.MinCases > 0 && g.CaseCount < opts.MinCases {
			continue
		}
		if opts.MinRecallAt5 > 0 && g.RecallAt5 < opts.MinRecallAt5 {
			failures = append(failures, groupFailure{name, "recall_at_5", g.RecallAt5, opts.MinRecallAt5, g.CaseCount})
		}
		if opts.MinRecallAt10 > 0 && g.RecallAt10 < opts.MinRecallAt10 {
			failures = append(failures, groupFailure{name, "recall_at_10", g.RecallAt10, opts.MinRecallAt10, g.CaseCount})
		}
		if opts.MinMRRAtK > 0 && g.MRRAtK < opts.MinMRRAtK {
			failures = append(failures, groupFailure{name, "mrr_at_k", g.MRRAtK, opts.MinMRRAtK, g.CaseCount})
		}
		if opts.MinNDCGAtK > 0 && g.NDCGAtK < opts.MinNDCGAtK {
			failures = append(failures, groupFailure{name, "ndcg_at_k", g.NDCGAtK, opts.MinNDCGAtK, g.CaseCount})
		}
	}
	sort.Slice(failures, func(i, j int) bool {
		if failures[i].Value == failures[j].Value {
			return failures[i].Group < failures[j].Group
		}
		return failures[i].Value < failures[j].Value
	})
	if len(failures) > opts.Limit {
		failures = failures[:opts.Limit]
	}
	return failures
}
```

- [ ] **Step 4: Run target test**

Run:

```powershell
go test ./cmd/rag-eval -run TestAggregateBuildsStableGroupsAndWorstGroups -count=1
```

Expected: PASS.

### Task 7: Add Group Gate Flags And Diagnostics

**Files:**
- Modify: `cmd/rag-eval/main.go`
- Modify: `cmd/rag-eval/main_test.go`

- [ ] **Step 1: Add failing test for group threshold message**

Add:

```go
func TestThresholdFailuresIncludesWorstGroups(t *testing.T) {
	s := summary{
		RecallAt5:  0.9,
		RecallAt10: 0.9,
		MRRAtK:     0.9,
		NDCGAtK:    0.9,
		Groups: map[string]groupMetric{
			"skill:redis": {CaseCount: 2, RecallAt5: 0.25, RecallAt10: 0.5, MRRAtK: 0.5, NDCGAtK: 0.5},
		},
	}
	failures := thresholdFailures(s, options{
		MinGroupCases:     2,
		MinGroupRecallAt5: 0.7,
	})
	if len(failures) != 1 {
		t.Fatalf("failures = %v, want one group failure", failures)
	}
	if failures[0] != "group skill:redis recall_at_5 0.250 below threshold 0.700 cases=2" {
		t.Fatalf("failure = %q", failures[0])
	}
}
```

- [ ] **Step 2: Run failing test**

Run:

```powershell
go test ./cmd/rag-eval -run TestThresholdFailuresIncludesWorstGroups -count=1
```

Expected: FAIL because group flags do not exist.

- [ ] **Step 3: Extend `options` and flags**

Add to `options`:

```go
	MinGroupCases      int
	MinGroupRecallAt5  float64
	MinGroupRecallAt10 float64
	MinGroupMRRAtK     float64
	MinGroupNDCGAtK    float64
```

Add in `main`:

```go
	flag.IntVar(&opts.MinGroupCases, "min-group-cases", 0, "minimum cases required before group gates apply; 0 disables group gates")
	flag.Float64Var(&opts.MinGroupRecallAt5, "min-group-recall-at-5", 0, "fail when any eligible group recall@5 is below this threshold; 0 disables")
	flag.Float64Var(&opts.MinGroupRecallAt10, "min-group-recall-at-10", 0, "fail when any eligible group recall@10 is below this threshold; 0 disables")
	flag.Float64Var(&opts.MinGroupMRRAtK, "min-group-mrr-at-k", 0, "fail when any eligible group MRR@K is below this threshold; 0 disables")
	flag.Float64Var(&opts.MinGroupNDCGAtK, "min-group-ndcg-at-k", 0, "fail when any eligible group nDCG@K is below this threshold; 0 disables")
```

- [ ] **Step 4: Connect group gates**

In `thresholdFailures`, append:

```go
	groupFailures := worstGroups(s.Groups, groupGateOptions{
		MinCases:      opts.MinGroupCases,
		MinRecallAt5:  opts.MinGroupRecallAt5,
		MinRecallAt10: opts.MinGroupRecallAt10,
		MinMRRAtK:     opts.MinGroupMRRAtK,
		MinNDCGAtK:    opts.MinGroupNDCGAtK,
		Limit:         10,
	})
	for _, failure := range groupFailures {
		failures = append(failures, fmt.Sprintf("group %s %s %.3f below threshold %.3f cases=%d",
			failure.Group, failure.Metric, failure.Value, failure.Threshold, failure.Cases))
	}
```

In `run`, before `writeOutputs`:

```go
	result.WorstGroups = worstGroups(result.Groups, groupGateOptions{
		MinCases:      opts.MinGroupCases,
		MinRecallAt5:  opts.MinGroupRecallAt5,
		MinRecallAt10: opts.MinGroupRecallAt10,
		MinMRRAtK:     opts.MinGroupMRRAtK,
		MinNDCGAtK:    opts.MinGroupNDCGAtK,
		Limit:         10,
	})
```

- [ ] **Step 5: Run tests**

Run:

```powershell
go test ./cmd/rag-eval -count=1
```

Expected: PASS.

### Task 8: Wire Stable Group Gates Into Makefile And Docs

**Files:**
- Modify: `Makefile`
- Modify: `README.md`

- [ ] **Step 1: Check current group distribution**

Run:

```powershell
mingw32-make eval-rag
Get-Content "tmp\eval\rag\summary.json" | Select-String -Pattern '"groups"|"worst_groups"|"skill:|"tag:'
```

Expected: `summary.json` contains `groups`. `worst_groups` can be absent or empty if group gates are not enabled.

- [ ] **Step 2: Add conservative group flags**

Modify `Makefile` `eval-rag` command to include only stable gates:

```makefile
	$(GO) run ./cmd/rag-eval -cases testdata/rag/golden_queries.jsonl -config $(CONFIG) -out tmp/eval/rag -min-recall-at-5 0.70 -min-recall-at-10 0.80 -min-mrr-at-k 0.90 -min-ndcg-at-k 0.75 -min-group-cases 3 -min-group-recall-at-5 0.50
```

If current data fails this gate, lower only `-min-group-recall-at-5` to the highest value that passes with at least one weak-group diagnostic test still present in unit tests.

- [ ] **Step 3: Document RAG group diagnostics**

In README RAG eval section, add:

```markdown
`summary.json.groups` 按 `skill:*` 和 `tag:*` 输出分组召回质量；`worst_groups` 在启用分组门槛时列出低于阈值的组。分组门槛只对达到 `-min-group-cases` 的组生效，避免小样本误伤。
```

- [ ] **Step 4: Verify Stage 2**

Run:

```powershell
go test ./cmd/rag-eval -count=1
mingw32-make eval-rag
git diff --check
```

Expected: all exit 0.

- [ ] **Step 5: Commit Stage 2**

Run after user confirms commit:

```powershell
git add -- "cmd/rag-eval/main.go" "cmd/rag-eval/main_test.go" "Makefile" "README.md"
git commit -m "test: add rag group quality gates"
```

---

## Stage 3: Candidate Training Product Loop

### Task 9: Extract Testable Report Drill Helpers

**Files:**
- Create: `web/src/reportView.ts`
- Create: `web/src/reportView.test.ts`
- Modify: `web/src/candidatePages.tsx`

- [ ] **Step 1: Write failing helper tests**

Create `web/src/reportView.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { drillPlanSummary, drillQuestionIds } from "./reportView";
import type { DrillPlanItem } from "./types";

describe("report drill view helpers", () => {
  const plan: DrillPlanItem[] = [
    { practice_order: 1, skill: "redis", reason: "缓存一致性薄弱", target_score: 80, recommended_question_ids: ["redis-001", "redis-001"] },
    { practice_order: 2, skill: "go", reason: "并发细节不足", target_score: 75, recommended_question_ids: ["go-003"] },
  ];

  it("summarizes the next drill plan for the report page", () => {
    expect(drillPlanSummary(plan)).toBe("2 个弱项 · 2 道题 · redis / go");
  });

  it("deduplicates recommended question ids in display order", () => {
    expect(drillQuestionIds(plan)).toEqual(["redis-001", "go-003"]);
  });
});
```

- [ ] **Step 2: Run failing test**

Run:

```powershell
npm --prefix web run test -- reportView.test.ts
```

Expected: FAIL because `web/src/reportView.ts` does not exist.

- [ ] **Step 3: Implement helper**

Create `web/src/reportView.ts`:

```ts
import type { DrillPlanItem } from "./types";

export function drillQuestionIds(plan: DrillPlanItem[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const item of plan) {
    for (const id of item.recommended_question_ids || []) {
      const value = id.trim();
      if (!value || seen.has(value)) continue;
      seen.add(value);
      out.push(value);
    }
  }
  return out;
}

export function drillPlanSummary(plan: DrillPlanItem[]): string {
  const skills = plan.map((item) => item.skill?.trim()).filter((skill): skill is string => Boolean(skill));
  const ids = drillQuestionIds(plan);
  return `${plan.length} 个弱项 · ${ids.length} 道题 · ${skills.slice(0, 3).join(" / ") || "综合表达"}`;
}
```

- [ ] **Step 4: Use helper in report page**

Modify `web/src/candidatePages.tsx` import:

```ts
import { drillPlanSummary } from "./reportView";
```

Change `DrillPlanPanel` body from one-line return to include a summary:

```tsx
function DrillPlanPanel({ plan, startDrill, jumpQuestion }: { plan: DrillPlanItem[]; startDrill: (plan: DrillPlanItem[]) => void; jumpQuestion: (id: string) => void }) {
  if (!plan.length) return null;
  return (
    <section className="drill-plan">
      <div className="analysis-head">
        <div>
          <p className="eyebrow">训练计划</p>
          <h2>下一轮按弱项顺序练，不再泛刷题。</h2>
          <p>{drillPlanSummary(plan)}</p>
        </div>
        <button className="secondary drill-start" onClick={() => startDrill(plan)}>按此计划训练</button>
      </div>
      <div className="drill-list">
        {plan.map((item) => <article className="drill-card" key={`${item.practice_order}-${item.skill}`}><div className="drill-order">{item.practice_order}</div><div><div className="drill-head"><strong>{item.skill || "综合表达"}</strong><span>目标 {item.target_score || 75} 分</span></div><p>{item.reason}</p><div className="recommended-question-ids">{item.recommended_question_ids?.map((id) => <button key={id} onClick={() => jumpQuestion(id)}>题库题 {id}</button>)}</div><ul>{item.recommended_questions?.map((q) => <li key={q}>{q}</li>)}</ul></div></article>)}
      </div>
    </section>
  );
}
```

- [ ] **Step 5: Run tests**

Run:

```powershell
npm --prefix web run test -- reportView.test.ts
npm --prefix web run test
```

Expected: PASS.

### Task 10: Cover Drill JD And Question Jump Behavior

**Files:**
- Modify: `web/src/draftStore.test.ts`
- Modify: `web/src/routes.test.ts`

- [ ] **Step 1: Extend drill JD test**

In `web/src/draftStore.test.ts`, ensure the existing drill test asserts:

```ts
expect(text).toContain("缓存一致性薄弱");
expect(text).toContain("redis-001");
expect(text).toContain("go-003");
```

If variable names differ, use the local result variable already returned by `drillJDText`.

- [ ] **Step 2: Extend route test for question URL**

In `web/src/routes.test.ts`, import `questionURL` and add:

```ts
it("builds encoded question bank jump URLs", () => {
  expect(questionURL("redis hot/key")).toBe("/questions?q=redis%20hot%2Fkey");
});
```

- [ ] **Step 3: Run targeted tests**

Run:

```powershell
npm --prefix web run test -- draftStore.test.ts routes.test.ts reportView.test.ts
```

Expected: PASS.

### Task 11: Verify Demo Artifacts Expose Drill Plan

**Files:**
- Inspect: `cmd/demo/output.go`
- Modify only if missing: `cmd/demo/main_test.go`

- [ ] **Step 1: Run mock demo**

Run:

```powershell
mingw32-make demo-mock
```

Expected: command exits 0 and writes a `tmp/demos/<timestamp>/run.json`.

- [ ] **Step 2: Inspect latest run for drill plan**

Run:

```powershell
$latest = Get-ChildItem "tmp\demos" -Directory | Sort-Object LastWriteTime -Descending | Select-Object -First 1
Select-String -Path (Join-Path $latest.FullName "run.json") -Pattern '"drill_plan"|"recommended_question_ids"'
```

Expected: output contains `drill_plan` and either `recommended_question_ids` or recommended question text. If absent, add an assertion to `cmd/demo/main_test.go` that generated reports include a non-empty drill plan, then update `cmd/demo/output.go` to serialize the existing report field.

- [ ] **Step 3: Verify Stage 3**

Run:

```powershell
npm --prefix web run test
npm --prefix web run build
mingw32-make demo-mock
git diff --check
```

Expected: all exit 0.

- [ ] **Step 4: Commit Stage 3**

Run after user confirms commit:

```powershell
git add -- "web/src/reportView.ts" "web/src/reportView.test.ts" "web/src/candidatePages.tsx" "web/src/draftStore.test.ts" "web/src/routes.test.ts" "internal/httpapi/web/dist"
git commit -m "test: cover candidate training loop"
```

---

## Stage 4: Deployment And Diagnostics Loop

### Task 12: Add Minimal Tracing Interface

**Files:**
- Create: `internal/observability/tracing.go`
- Create: `internal/observability/tracing_test.go`

- [ ] **Step 1: Write failing tracing tests**

Create `internal/observability/tracing_test.go`:

```go
package observability

import (
	"context"
	"testing"
)

func TestNoopTracerReturnsContextAndEndFunc(t *testing.T) {
	ctx := context.Background()
	next, end := NoopTracer{}.Start(ctx, "graph.node", map[string]string{"node": "pick_next"})
	if next == nil {
		t.Fatal("Start returned nil context")
	}
	end(nil)
}

func TestRecordingTracerRecordsSpanLifecycle(t *testing.T) {
	tracer := NewRecordingTracer()
	_, end := tracer.Start(context.Background(), "graph.node", map[string]string{"node": "evaluate"})
	end(assertErr("boom"))

	spans := tracer.Spans()
	if len(spans) != 1 {
		t.Fatalf("spans len = %d, want 1", len(spans))
	}
	if spans[0].Name != "graph.node" || spans[0].Attrs["node"] != "evaluate" || spans[0].Err == "" {
		t.Fatalf("span = %+v", spans[0])
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
```

- [ ] **Step 2: Run failing tests**

Run:

```powershell
go test ./internal/observability -run TestNoopTracerReturnsContextAndEndFunc -count=1
```

Expected: FAIL because tracing types do not exist.

- [ ] **Step 3: Implement tracing interface**

Create `internal/observability/tracing.go`:

```go
package observability

import (
	"context"
	"sync"
	"time"
)

type SpanEnd func(error)

type Tracer interface {
	Start(ctx context.Context, name string, attrs map[string]string) (context.Context, SpanEnd)
}

type NoopTracer struct{}

func (NoopTracer) Start(ctx context.Context, _ string, _ map[string]string) (context.Context, SpanEnd) {
	return ctx, func(error) {}
}

type SpanRecord struct {
	Name      string            `json:"name"`
	Attrs     map[string]string `json:"attrs,omitempty"`
	StartedAt time.Time         `json:"started_at"`
	Duration  time.Duration     `json:"duration_ns"`
	Err       string            `json:"err,omitempty"`
}

type RecordingTracer struct {
	now func() time.Time
	mu  sync.Mutex
	out []SpanRecord
}

func NewRecordingTracer() *RecordingTracer {
	return &RecordingTracer{now: time.Now}
}

func (r *RecordingTracer) Start(ctx context.Context, name string, attrs map[string]string) (context.Context, SpanEnd) {
	start := r.now()
	copied := map[string]string{}
	for k, v := range attrs {
		copied[k] = v
	}
	return ctx, func(err error) {
		rec := SpanRecord{Name: name, Attrs: copied, StartedAt: start, Duration: r.now().Sub(start)}
		if err != nil {
			rec.Err = err.Error()
		}
		r.mu.Lock()
		r.out = append(r.out, rec)
		r.mu.Unlock()
	}
}

func (r *RecordingTracer) Spans() []SpanRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]SpanRecord, len(r.out))
	copy(out, r.out)
	return out
}
```

- [ ] **Step 4: Run tests**

Run:

```powershell
go test ./internal/observability -run "TestNoopTracer|TestRecordingTracer" -count=1
```

Expected: PASS.

### Task 13: Add Graph Tracing Callback

**Files:**
- Create: `internal/observability/tracing_callback.go`
- Create: `internal/observability/tracing_callback_test.go`

- [ ] **Step 1: Write failing callback test**

Create `internal/observability/tracing_callback_test.go`:

```go
package observability

import (
	"context"
	"errors"
	"testing"

	"interview-agent/internal/domain"
)

func TestTracingGraphCallbackRecordsNodeSpan(t *testing.T) {
	tracer := NewRecordingTracer()
	cb := NewTracingGraphCallback(tracer)
	sess := &domain.Session{ID: "s1"}

	cb.OnNodeStart(context.Background(), "pick_next", sess)
	cb.OnNodeEnd(context.Background(), "pick_next", sess)

	spans := tracer.Spans()
	if len(spans) != 1 {
		t.Fatalf("spans len = %d, want 1", len(spans))
	}
	if spans[0].Name != "graph.node" || spans[0].Attrs["node"] != "pick_next" || spans[0].Attrs["session_id"] != "s1" {
		t.Fatalf("span = %+v", spans[0])
	}
}

func TestTracingGraphCallbackRecordsNodeError(t *testing.T) {
	tracer := NewRecordingTracer()
	cb := NewTracingGraphCallback(tracer)
	sess := &domain.Session{ID: "s1"}

	cb.OnNodeStart(context.Background(), "evaluate", sess)
	cb.OnNodeError(context.Background(), "evaluate", sess, errors.New("llm failed"))

	spans := tracer.Spans()
	if len(spans) != 1 || spans[0].Err != "llm failed" {
		t.Fatalf("spans = %+v", spans)
	}
}
```

- [ ] **Step 2: Run failing test**

Run:

```powershell
go test ./internal/observability -run TestTracingGraphCallback -count=1
```

Expected: FAIL because `NewTracingGraphCallback` does not exist.

- [ ] **Step 3: Implement callback**

Create `internal/observability/tracing_callback.go`:

```go
package observability

import (
	"context"
	"sync"

	"interview-agent/internal/domain"
)

type tracingGraphCallback struct {
	tracer Tracer
	mu     sync.Mutex
	ends   map[string]SpanEnd
}

func NewTracingGraphCallback(tracer Tracer) *tracingGraphCallback {
	if tracer == nil {
		tracer = NoopTracer{}
	}
	return &tracingGraphCallback{tracer: tracer, ends: map[string]SpanEnd{}}
}

func (c *tracingGraphCallback) OnNodeStart(ctx context.Context, node string, sess *domain.Session) {
	attrs := map[string]string{"node": node}
	if sess != nil {
		attrs["session_id"] = sess.ID
	}
	_, end := c.tracer.Start(ctx, "graph.node", attrs)
	c.mu.Lock()
	c.ends[node] = end
	c.mu.Unlock()
}

func (c *tracingGraphCallback) OnNodeEnd(_ context.Context, node string, _ *domain.Session) {
	c.complete(node, nil)
}

func (c *tracingGraphCallback) OnNodeError(_ context.Context, node string, _ *domain.Session, err error) {
	c.complete(node, err)
}

func (c *tracingGraphCallback) complete(node string, err error) {
	c.mu.Lock()
	end := c.ends[node]
	delete(c.ends, node)
	c.mu.Unlock()
	if end != nil {
		end(err)
	}
}
```

- [ ] **Step 4: Run observability tests**

Run:

```powershell
go test ./internal/observability -count=1
```

Expected: PASS.

### Task 14: Wire No-Op Tracing Into Server Graph

**Files:**
- Modify: `cmd/server/interview_wiring.go`
- Modify: `cmd/server/main.go` if dependency injection needs a tracer field
- Modify: `cmd/server/main_test.go`

- [ ] **Step 1: Add callback in wiring**

Modify `cmd/server/interview_wiring.go` callback construction:

```go
	callbacks := []graph.Callback{httpapi.NewInterviewGraphCallback(events)}
	callbacks = append(callbacks, observability.NewTracingGraphCallback(observability.NoopTracer{}))
	if metricsCallback != nil {
		callbacks = append(callbacks, metricsCallback)
	}
```

Add import:

```go
	"interview-agent/internal/observability"
```

- [ ] **Step 2: Verify server tests**

Run:

```powershell
go test ./cmd/server -count=1
```

Expected: PASS.

### Task 15: Add Minimal Deployment Health Docs Or Helm Template

**Files:**
- Create: `deploy/helm/interview-agent/Chart.yaml`
- Create: `deploy/helm/interview-agent/values.yaml`
- Create: `deploy/helm/interview-agent/templates/deployment.yaml`
- Create: `deploy/helm/interview-agent/templates/service.yaml`
- Modify: `README.md`

- [ ] **Step 1: Create minimal Helm chart**

Create `deploy/helm/interview-agent/Chart.yaml`:

```yaml
apiVersion: v2
name: interview-agent
description: Minimal Interview Agent deployment chart
type: application
version: 0.1.0
appVersion: "0.1.0"
```

Create `deploy/helm/interview-agent/values.yaml`:

```yaml
replicaCount: 1

image:
  repository: interview-agent
  tag: latest
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 8080

env:
  INTERVIEW_LLM_MODE: mock
  INTERVIEW_EMBEDDING_MODE: mock
```

Create `deploy/helm/interview-agent/templates/deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Chart.Name }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      app: {{ .Chart.Name }}
  template:
    metadata:
      labels:
        app: {{ .Chart.Name }}
    spec:
      containers:
        - name: server
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - containerPort: 8080
          env:
            {{- range $name, $value := .Values.env }}
            - name: {{ $name }}
              value: {{ $value | quote }}
            {{- end }}
          readinessProbe:
            httpGet:
              path: /readyz
              port: 8080
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8080
```

Create `deploy/helm/interview-agent/templates/service.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: {{ .Chart.Name }}
spec:
  type: {{ .Values.service.type }}
  selector:
    app: {{ .Chart.Name }}
  ports:
    - port: {{ .Values.service.port }}
      targetPort: 8080
```

- [ ] **Step 2: Document deployment health**

Add to README deployment section:

```markdown
Kubernetes 最小模板位于 `deploy/helm/interview-agent`，默认 mock LLM / mock embedding，只暴露 `/healthz` liveness 和 `/readyz` readiness。真实 LLM、PG、Redis 应通过 values/env 注入，不写入仓库。
```

- [ ] **Step 3: Verify Helm rendering if Helm exists**

Run:

```powershell
helm template interview-agent "deploy/helm/interview-agent"
```

Expected if Helm is installed: rendered Deployment and Service. If Helm is not installed, record "未验证 Helm render：本机未安装 helm"，and still run file-level checks.

- [ ] **Step 4: Verify Stage 4**

Run:

```powershell
go test ./internal/observability ./cmd/server -count=1
git diff --check
```

Expected: PASS.

- [ ] **Step 5: Commit Stage 4**

Run after user confirms commit:

```powershell
git add -- "internal/observability/tracing.go" "internal/observability/tracing_test.go" "internal/observability/tracing_callback.go" "internal/observability/tracing_callback_test.go" "cmd/server/interview_wiring.go" "deploy/helm/interview-agent" "README.md"
git commit -m "chore: add minimal deployment diagnostics"
```

---

## Final Cross-Stage Verification

### Task 16: Full Verification Pass

**Files:**
- No source edits unless verification exposes a real defect

- [ ] **Step 1: Run local gate**

Run:

```powershell
mingw32-make verify-local
```

Expected: PASS.

- [ ] **Step 2: Run smoke**

Run:

```powershell
mingw32-make e2e-smoke
```

Expected: PASS and output summary under `tmp/e2e/<timestamp>/summary.json`.

- [ ] **Step 3: Run explicit frontend checks**

Run:

```powershell
npm --prefix web run test
npm --prefix web run build
```

Expected: PASS.

- [ ] **Step 4: Run explicit Go checks**

Run:

```powershell
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 5: Inspect final status**

Run:

```powershell
git status --short
git log --oneline -8
```

Expected: clean status and four stage commits after the roadmap design commit.

## Self-Review Notes

- Stage 1 covers the design's production verification gate.
- Stage 2 covers grouped RAG metrics, worst groups, and group gate diagnostics.
- Stage 3 covers the candidate report-to-drill loop using existing `Report.DrillPlan`.
- Stage 4 covers no-op tracing, graph tracing callback, and minimal deployment health templates.
- The plan avoids real LLM, PG, Redis, Docker, and secret requirements in default gates.
- Commit steps are listed but must still follow repository confirmation rules before execution.
