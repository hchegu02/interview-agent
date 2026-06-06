---
change: internal-business-trial-stabilization
design-doc: docs/superpowers/specs/2026-06-07-internal-business-trial-stabilization-design.md
base-ref: 701d867b9873504ae1c14c65a2047c91da6ff138
---

# Internal Business Trial Stabilization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a default offline business-trial evidence gate to internal trial smoke so maintainers can safely expand controlled internal business use.

**Architecture:** Keep the gate local and deterministic. Add a compact verifier under `internal/agentkit/verify`, load a non-sensitive fixture from `testdata/internal_trial`, and make `cmd/internal-trial-smoke` fail when business-trial evidence is missing or contradictory.

**Tech Stack:** Go 1.23, standard `encoding/json`, existing `internal/agentkit/verify`, OpenSpec, PowerShell verification commands.

---

## File Structure

- Create `internal/agentkit/verify/business_trial.go`: business feedback data structure and verifier.
- Create `internal/agentkit/verify/business_trial_test.go`: verifier unit tests for pass and failure cases.
- Modify `cmd/internal-trial-smoke/main.go`: load fixture, call verifier, print `business_trial` marker.
- Modify `cmd/internal-trial-smoke/main_test.go`: assert the new marker and missing fixture failure.
- Create `testdata/internal_trial/business_feedback_pass.json`: non-sensitive passing fixture.
- Modify `docs/ai/internal-trial-launch-checklist.md`: add business evidence gate.
- Modify `docs/ai/internal-trial/business-trial-runbook.md`: add stable-version criteria.
- Modify `docs/ai/internal-trial/trial-go-no-go.md`: add business evidence hard pause.
- Create `docs/code-changes/06-07-internal-business-trial-stabilization.md`: final diff documentation after implementation.
- Modify `openspec/changes/internal-business-trial-stabilization/tasks.md`: check completed tasks only after matching verification.

## Task 1: Business Feedback Verifier

**Files:**
- Create: `internal/agentkit/verify/business_trial.go`
- Create: `internal/agentkit/verify/business_trial_test.go`
- Create: `testdata/internal_trial/business_feedback_pass.json`

- [ ] **Step 1: Create the passing fixture**

Create `testdata/internal_trial/business_feedback_pass.json`:

```json
{
  "trial_role": "interviewer",
  "trial_date": "2026-06-07",
  "scenario": "go-backend-internal",
  "completed_fixed_script": true,
  "interview_flow_score": 4,
  "report_usefulness_score": 4,
  "project_polish_score": 4,
  "expand_recommendation": "yes",
  "has_blocker": false,
  "most_valuable": "报告和追问可以辅助内部面试复盘。",
  "top_issue": "题库覆盖仍需继续增加 Go 后端真实场景。",
  "next_priority": "继续收集 Go 后端题库和报告质量反馈。"
}
```

- [ ] **Step 2: Write verifier tests**

Create `internal/agentkit/verify/business_trial_test.go`:

```go
package verify

import "testing"

func TestBusinessTrialFeedbackVerifierPassesValidFeedback(t *testing.T) {
	feedback := BusinessTrialFeedback{
		TrialRole:                "interviewer",
		Scenario:                 "go-backend-internal",
		CompletedFixedScript:     true,
		InterviewFlowScore:       4,
		ReportUsefulnessScore:    4,
		ProjectPolishScore:       4,
		ExpandRecommendation:     "yes",
		HasBlocker:               false,
		MostValuable:             "报告可用于复盘。",
		TopIssue:                 "题库还要继续扩充。",
		NextPriority:             "继续收集反馈。",
	}
	failures := BusinessTrialFeedbackVerifier{}.Verify(feedback)
	if len(failures) != 0 {
		t.Fatalf("Verify failures = %+v, want none", failures)
	}
}

func TestBusinessTrialFeedbackVerifierRejectsIncompleteScriptExpansion(t *testing.T) {
	feedback := validBusinessTrialFeedback()
	feedback.CompletedFixedScript = false
	failures := BusinessTrialFeedbackVerifier{}.Verify(feedback)
	if len(failures) == 0 {
		t.Fatal("Verify returned no failures for incomplete fixed script")
	}
}

func TestBusinessTrialFeedbackVerifierRejectsScoreOutOfRange(t *testing.T) {
	feedback := validBusinessTrialFeedback()
	feedback.ReportUsefulnessScore = 6
	failures := BusinessTrialFeedbackVerifier{}.Verify(feedback)
	if len(failures) == 0 {
		t.Fatal("Verify returned no failures for score out of range")
	}
}

func TestBusinessTrialFeedbackVerifierRejectsBlockerExpansion(t *testing.T) {
	feedback := validBusinessTrialFeedback()
	feedback.HasBlocker = true
	feedback.ExpandRecommendation = "yes"
	failures := BusinessTrialFeedbackVerifier{}.Verify(feedback)
	if len(failures) == 0 {
		t.Fatal("Verify returned no failures for blocker with expansion recommendation")
	}
}

func TestBusinessTrialFeedbackVerifierRejectsMissingRecommendation(t *testing.T) {
	feedback := validBusinessTrialFeedback()
	feedback.ExpandRecommendation = ""
	failures := BusinessTrialFeedbackVerifier{}.Verify(feedback)
	if len(failures) == 0 {
		t.Fatal("Verify returned no failures for missing expansion recommendation")
	}
}

func validBusinessTrialFeedback() BusinessTrialFeedback {
	return BusinessTrialFeedback{
		TrialRole:                "interviewer",
		Scenario:                 "go-backend-internal",
		CompletedFixedScript:     true,
		InterviewFlowScore:       4,
		ReportUsefulnessScore:    4,
		ProjectPolishScore:       4,
		ExpandRecommendation:     "yes",
		HasBlocker:               false,
		MostValuable:             "报告可用于复盘。",
		TopIssue:                 "题库还要继续扩充。",
		NextPriority:             "继续收集反馈。",
	}
}
```

- [ ] **Step 3: Run tests to confirm failure**

Run:

```powershell
go test ./internal/agentkit/verify -count=1
```

Expected: FAIL because `BusinessTrialFeedback` and `BusinessTrialFeedbackVerifier` do not exist.

- [ ] **Step 4: Implement verifier**

Create `internal/agentkit/verify/business_trial.go`:

```go
package verify

import "strings"

type BusinessTrialFeedback struct {
	TrialRole                string `json:"trial_role"`
	TrialDate                string `json:"trial_date,omitempty"`
	Scenario                 string `json:"scenario"`
	CompletedFixedScript     bool   `json:"completed_fixed_script"`
	InterviewFlowScore       int    `json:"interview_flow_score"`
	ReportUsefulnessScore    int    `json:"report_usefulness_score"`
	ProjectPolishScore       int    `json:"project_polish_score"`
	ExpandRecommendation     string `json:"expand_recommendation"`
	HasBlocker               bool   `json:"has_blocker"`
	MostValuable             string `json:"most_valuable,omitempty"`
	TopIssue                 string `json:"top_issue,omitempty"`
	NextPriority             string `json:"next_priority,omitempty"`
}

type BusinessTrialFeedbackVerifier struct{}

func (BusinessTrialFeedbackVerifier) Verify(feedback BusinessTrialFeedback) []Failure {
	var failures []Failure
	if strings.TrimSpace(feedback.TrialRole) == "" {
		failures = append(failures, Failure{Code: "business_trial_role_missing", Message: "trial_role is required"})
	}
	if strings.TrimSpace(feedback.Scenario) == "" {
		failures = append(failures, Failure{Code: "business_trial_scenario_missing", Message: "scenario is required"})
	}
	if !feedback.CompletedFixedScript {
		failures = append(failures, Failure{Code: "business_trial_script_incomplete", Message: "completed_fixed_script must be true"})
	}
	failures = appendScoreFailure(failures, "interview_flow_score", feedback.InterviewFlowScore)
	failures = appendScoreFailure(failures, "report_usefulness_score", feedback.ReportUsefulnessScore)
	failures = appendScoreFailure(failures, "project_polish_score", feedback.ProjectPolishScore)

	recommendation := strings.ToLower(strings.TrimSpace(feedback.ExpandRecommendation))
	switch recommendation {
	case "yes", "no", "unsure":
	default:
		failures = append(failures, Failure{Code: "business_trial_recommendation_invalid", Message: "expand_recommendation must be yes, no, or unsure"})
	}
	if feedback.HasBlocker && recommendation == "yes" {
		failures = append(failures, Failure{Code: "business_trial_blocker_expansion_conflict", Message: "feedback with blockers cannot recommend expansion"})
	}
	return failures
}

func appendScoreFailure(failures []Failure, field string, score int) []Failure {
	if score < 1 || score > 5 {
		return append(failures, Failure{Code: "business_trial_score_invalid", Message: field + " must be between 1 and 5"})
	}
	return failures
}
```

- [ ] **Step 5: Run verifier tests**

Run:

```powershell
go test ./internal/agentkit/verify -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 1**

Stage precise files:

```powershell
git add "internal/agentkit/verify/business_trial.go" "internal/agentkit/verify/business_trial_test.go" "testdata/internal_trial/business_feedback_pass.json"
git commit -m "feat: add business trial feedback verifier"
```

## Task 2: Internal Trial Smoke Integration

**Files:**
- Modify: `cmd/internal-trial-smoke/main.go`
- Modify: `cmd/internal-trial-smoke/main_test.go`

- [ ] **Step 1: Extend smoke options and fixture path**

In `cmd/internal-trial-smoke/main.go`, add `BusinessFeedbackPath string` to `smokeOptions`, register flag `-business-feedback`, and add:

```go
const defaultBusinessFeedbackFixturePath = "testdata/internal_trial/business_feedback_pass.json"
```

- [ ] **Step 2: Add fixture path and loader functions**

Add functions near `loadSessionFixture`:

```go
func businessFeedbackFixturePath(opts smokeOptions) string {
	if opts.BusinessFeedbackPath != "" {
		return opts.BusinessFeedbackPath
	}
	for _, candidate := range []string{
		defaultBusinessFeedbackFixturePath,
		filepath.Join("..", "..", defaultBusinessFeedbackFixturePath),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return defaultBusinessFeedbackFixturePath
}

func loadBusinessFeedbackFixture(path string) (verify.BusinessTrialFeedback, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return verify.BusinessTrialFeedback{}, err
	}
	var feedback verify.BusinessTrialFeedback
	if err := json.Unmarshal(raw, &feedback); err != nil {
		return verify.BusinessTrialFeedback{}, err
	}
	return feedback, nil
}
```

- [ ] **Step 3: Call verifier inside run**

After memory observation verification in `run`, add:

```go
feedback, err := loadBusinessFeedbackFixture(businessFeedbackFixturePath(opts))
if err != nil {
	failures = append(failures, fmt.Sprintf("load business feedback fixture: %v", err))
} else if feedbackFailures := verify.BusinessTrialFeedbackVerifier{}.Verify(feedback); len(feedbackFailures) > 0 {
	failures = append(failures, fmt.Sprintf("business trial feedback failed: %+v", feedbackFailures))
}
```

After existing success markers, add:

```go
fmt.Fprintln(stdout, "business_trial: feedback evidence verified")
```

- [ ] **Step 4: Update smoke tests**

In `TestRunPassesOfflineInternalTrialSmoke`, include `business_trial` in the expected marker list.

Add:

```go
func TestRunFailsWhenBusinessFeedbackFixtureIsMissing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(smokeOptions{BusinessFeedbackPath: filepath.Join(t.TempDir(), "missing.json")}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("run exit = %d, want non-zero; stdout = %s", code, stdout.String())
	}
	if !strings.Contains(stderr.String(), "load business feedback fixture") {
		t.Fatalf("stderr missing business feedback reason: %s", stderr.String())
	}
}
```

- [ ] **Step 5: Run smoke tests**

Run:

```powershell
go test ./cmd/internal-trial-smoke -count=1
```

Expected: PASS.

- [ ] **Step 6: Run default smoke**

Run:

```powershell
go run ./cmd/internal-trial-smoke
```

Expected output contains:

```text
business_trial: feedback evidence verified
```

- [ ] **Step 7: Commit Task 2**

Stage precise files:

```powershell
git add "cmd/internal-trial-smoke/main.go" "cmd/internal-trial-smoke/main_test.go"
git commit -m "feat: include business trial evidence in smoke"
```

## Task 3: Documentation

**Files:**
- Modify: `docs/ai/internal-trial-launch-checklist.md`
- Modify: `docs/ai/internal-trial/business-trial-runbook.md`
- Modify: `docs/ai/internal-trial/trial-go-no-go.md`

- [ ] **Step 1: Update launch checklist**

Add a bullet under current trial scope:

```markdown
- 业务试用稳定版门禁：`go run ./cmd/internal-trial-smoke` 默认校验业务反馈 fixture，并输出 `business_trial` marker。
```

Update required gates to state that smoke covers business feedback evidence.

- [ ] **Step 2: Update business trial runbook**

Add a short section after feedback scoring:

```markdown
## 8. 扩大内部业务试用条件

- 固定脚本已完成。
- 面试流程、报告可用性和项目润色评分均在 1-5 范围内，并有明确扩大建议。
- 存在阻断项时，不得标记为适合扩大内部试用。
- 维护者必须运行 `go run ./cmd/internal-trial-smoke`，并确认输出 `business_trial: feedback evidence verified`。
```

- [ ] **Step 3: Update Go/No-Go**

Add business feedback evidence to `Enter Business Trial` and hard pause conditions:

```markdown
- 业务反馈证据校验必须通过；如果存在阻断项但仍标记适合扩大试用，必须暂停 rollout。
```

- [ ] **Step 4: Commit Task 3**

Stage precise files:

```powershell
git add "docs/ai/internal-trial-launch-checklist.md" "docs/ai/internal-trial/business-trial-runbook.md" "docs/ai/internal-trial/trial-go-no-go.md"
git commit -m "docs: add business trial stabilization gate"
```

## Task 4: Code Change Documentation And Tasks

**Files:**
- Create: `docs/code-changes/06-07-internal-business-trial-stabilization.md`
- Modify: `openspec/changes/internal-business-trial-stabilization/tasks.md`

- [ ] **Step 1: Create code-change document from actual diff**

Create `docs/code-changes/06-07-internal-business-trial-stabilization.md` with these sections:

```markdown
# 06-07 内部业务试用稳定版

## 变更概述

说明新增业务反馈 fixture 校验、internal-trial-smoke 集成和内部试用文档同步。

## 变更文件

列出本次新增和修改文件。

## 函数级说明

逐个说明新增或修改的 Go 类型、函数和测试。

## 调用链

从 `go run ./cmd/internal-trial-smoke` 到 `BusinessTrialFeedbackVerifier.Verify`。

## 数据流

从 `testdata/internal_trial/business_feedback_pass.json` 到 smoke 输出 marker。

## 依赖与副作用

说明只使用本地文件和标准 JSON，不访问网络、不写数据库。

## 测试

记录实际执行命令和结果。

## 风险

说明该门禁不自动评价真实内容质量，也不是生产上线批准。
```

- [ ] **Step 2: Check off completed OpenSpec tasks**

After Tasks 1-3 verification passes, update `openspec/changes/internal-business-trial-stabilization/tasks.md` checkboxes for completed items.

- [ ] **Step 3: Commit Task 4**

Stage precise files:

```powershell
git add "docs/code-changes/06-07-internal-business-trial-stabilization.md" "openspec/changes/internal-business-trial-stabilization/tasks.md"
git commit -m "chore: document business trial stabilization"
```

## Task 5: Final Verification

**Files:**
- Modify: `openspec/changes/internal-business-trial-stabilization/tasks.md`

- [ ] **Step 1: Run focused Go tests**

Run:

```powershell
go test ./cmd/internal-trial-smoke ./internal/agentkit/verify -count=1
```

Expected: PASS.

- [ ] **Step 2: Run default smoke**

Run:

```powershell
go run ./cmd/internal-trial-smoke
```

Expected output contains:

```text
business_trial: feedback evidence verified
```

- [ ] **Step 3: Run OpenSpec strict validation**

Run:

```powershell
openspec validate internal-business-trial-stabilization --strict
```

Expected: PASS.

- [ ] **Step 4: Mark verification tasks complete**

Update `openspec/changes/internal-business-trial-stabilization/tasks.md` for Task 5 items after command output confirms pass.

- [ ] **Step 5: Commit final task status**

Stage precise file:

```powershell
git add "openspec/changes/internal-business-trial-stabilization/tasks.md"
git commit -m "chore: complete business trial stabilization tasks"
```

## Self-Review

- Spec coverage: covered business feedback evidence, smoke marker, blocker conflict, internal rollout docs, and strict validation.
- Placeholder scan: no implementation step uses TBD/TODO/fill-later language.
- Type consistency: `BusinessTrialFeedback`, `BusinessTrialFeedbackVerifier`, and JSON field names are consistent across fixture, tests, implementation, and smoke integration.
