---
change: add-structured-suspension
design-doc: docs/superpowers/specs/2026-06-05-structured-suspension-design.md
base-ref: e951e360411ffd1914e2f24fbaf003a595555ab4
---

# Structured Suspension Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a backward-compatible `Session.Suspension` model so Graph pauses expose structured awaiting state while preserving `CurrentNode` resume behavior.

**Architecture:** Keep the current `NodeFunc(ctx, *Session) error` and `ErrSuspended` model. Add an optional domain field, centralize suspend/resume helpers in `internal/graph`, expose a copied read-only field through HTTP and frontend types, and keep old Session JSON compatible.

**Tech Stack:** Go domain/httpapi/graph tests, React TypeScript types, existing Vitest and Go test commands.

---

### Task 1: Domain Suspension Model

**Files:**
- Modify: `internal/domain/session.go`
- Test: `internal/domain/session_test.go` if existing tests need migration coverage; otherwise graph/httpapi tests cover field behavior.

- [ ] **Step 1: Add suspension constants and struct**

Add near `SessionStatus` or before `Session`:

```go
type SuspensionAwaiting string

const (
    SuspensionAwaitingAnswer     SuspensionAwaiting = "answer"
    SuspensionAwaitingApproval   SuspensionAwaiting = "approval"
    SuspensionAwaitingToolReview SuspensionAwaiting = "tool_review"
)

type Suspension struct {
    Node      string                 `json:"node"`
    Reason    string                 `json:"reason,omitempty"`
    Awaiting  SuspensionAwaiting     `json:"awaiting"`
    Payload   map[string]interface{} `json:"payload,omitempty"`
    CreatedAt time.Time              `json:"created_at"`
}
```

Use `interface{}` instead of `any` to match older Go style if nearby code uses it; otherwise `any` is acceptable with Go 1.23.

- [ ] **Step 2: Add optional field on Session**

In `Session` after `CurrentNode`:

```go
Suspension *Suspension `json:"suspension,omitempty"`
```

- [ ] **Step 3: Run targeted compile**

Run:

```powershell
go test ./internal/domain -count=1
```

Expected: package passes.

- [ ] **Step 4: Commit**

```powershell
git add "internal/domain/session.go"
git commit -m "feat: add session suspension model"
```

### Task 2: Graph Suspend And Resume Semantics

**Files:**
- Modify: `internal/graph/graph.go`
- Test: `internal/graph/graph_test.go`

- [ ] **Step 1: Add failing suspend test**

Add a test that invokes a graph with a node returning `ErrSuspended` and asserts:

```go
if sess.Suspension == nil {
    t.Fatal("suspension should be set")
}
if sess.Suspension.Node != "pick_next" {
    t.Fatalf("suspension node = %q", sess.Suspension.Node)
}
if sess.Suspension.Awaiting != domain.SuspensionAwaitingAnswer {
    t.Fatalf("awaiting = %q", sess.Suspension.Awaiting)
}
if sess.CurrentNode != "pick_next" {
    t.Fatalf("current node = %q", sess.CurrentNode)
}
```

Run:

```powershell
go test ./internal/graph -run Suspension -count=1
```

Expected before implementation: compile failure or nil suspension.

- [ ] **Step 2: Add backward-compatible resume test**

Add a test that sets only `sess.CurrentNode = "wait"` and `sess.Suspension = nil`, calls `Resume`, and confirms downstream node runs. This protects old Session JSON.

- [ ] **Step 3: Add clear-after-resume test**

Add a test that sets both `CurrentNode` and `Suspension`, calls `Resume`, and confirms `sess.Suspension == nil` after downstream execution succeeds.

- [ ] **Step 4: Implement helper functions**

In `graph.go` add unexported helpers:

```go
func suspendedNode(sess *domain.Session) string {
    if sess.Suspension != nil && sess.Suspension.Node != "" {
        return sess.Suspension.Node
    }
    return sess.CurrentNode
}

func ensureSuspension(sess *domain.Session, node string) {
    if sess.Suspension == nil {
        sess.Suspension = &domain.Suspension{}
    }
    if sess.Suspension.Node == "" {
        sess.Suspension.Node = node
    }
    if sess.Suspension.Awaiting == "" {
        sess.Suspension.Awaiting = domain.SuspensionAwaitingAnswer
    }
    if sess.Suspension.CreatedAt.IsZero() {
        sess.Suspension.CreatedAt = time.Now().UTC()
    }
}
```

Remember to import `time`.

- [ ] **Step 5: Wire helpers into run and Resume**

In `run`, when `ErrSuspended` is caught:

```go
ensureSuspension(sess, sess.CurrentNode)
return nil
```

In `Resume`, replace direct `sess.CurrentNode` reads with `node := suspendedNode(sess)`. After computing the next frontier and before running downstream, clear:

```go
sess.Suspension = nil
```

If `run` suspends again, it will set a fresh suspension.

- [ ] **Step 6: Run graph tests**

```powershell
go test ./internal/graph -count=1
```

Expected: pass.

- [ ] **Step 7: Commit**

```powershell
git add "internal/graph/graph.go" "internal/graph/graph_test.go"
git commit -m "feat: structure graph suspension state"
```

### Task 3: HTTP Response And Frontend Type Contract

**Files:**
- Modify: `internal/httpapi/interview_response.go`
- Modify: `internal/httpapi/interview_test.go`
- Modify: `web/src/types.ts`

- [ ] **Step 1: Add failing HTTP response test**

In `internal/httpapi/interview_test.go`, add a test similar to retrieval trace copy:

```go
sess := &domain.Session{
    ID: "suspension-response",
    Status: domain.StatusRunning,
    CurrentNode: "pick_next",
    Suspension: &domain.Suspension{
        Node: "pick_next",
        Awaiting: domain.SuspensionAwaitingAnswer,
        Reason: "waiting for answer",
        Payload: map[string]interface{}{"round_id": "r1"},
        CreatedAt: now,
    },
    CreatedAt: now,
    UpdatedAt: now,
}
got := buildInterviewResponse(sess)
if got.Suspension == nil { t.Fatal("suspension should be included") }
sess.Suspension.Payload["round_id"] = "changed"
if got.Suspension.Payload["round_id"] != "r1" { t.Fatal("payload should be copied") }
```

Run:

```powershell
go test ./internal/httpapi -run Suspension -count=1
```

Expected before implementation: compile failure or missing field.

- [ ] **Step 2: Add response field and clone helper**

In `interviewResponse` add:

```go
Suspension *domain.Suspension `json:"suspension,omitempty"`
```

In `buildInterviewResponse`, set:

```go
Suspension: cloneSuspension(sess.Suspension),
```

Implement deep copy of `Payload` map.

- [ ] **Step 3: Add frontend types**

In `web/src/types.ts`, add:

```ts
export type Suspension = {
  node: string;
  reason?: string;
  awaiting: "answer" | "approval" | "tool_review" | string;
  payload?: Record<string, unknown>;
  created_at: string;
};
```

Add `suspension?: Suspension;` to `Session`.

- [ ] **Step 4: Run targeted tests**

```powershell
go test ./internal/httpapi -run Suspension -count=1
npm --prefix web run test
```

Expected: pass.

- [ ] **Step 5: Commit**

```powershell
git add "internal/httpapi/interview_response.go" "internal/httpapi/interview_test.go" "web/src/types.ts"
git commit -m "feat: expose session suspension state"
```

### Task 4: Documentation, Final Verification, And Change Tasks

**Files:**
- Modify: `docs/SDD-Backend.md`
- Modify: `openspec/changes/add-structured-suspension/tasks.md`

- [ ] **Step 1: Update backend SDD**

In `docs/SDD-Backend.md`, update the Session / Graph improvement plan to state structured Suspension is now implemented as the first compatibility step, while StatePatch and GraphCheckpoint remain future work.

- [ ] **Step 2: Check OpenSpec tasks**

Mark completed items in `openspec/changes/add-structured-suspension/tasks.md`.

- [ ] **Step 3: Run final verification**

```powershell
go test ./internal/domain ./internal/graph ./internal/httpapi -count=1
npm --prefix web run test
go test ./... -count=1
```

Expected: all pass.

- [ ] **Step 4: Commit**

```powershell
git add "docs/SDD-Backend.md" "openspec/changes/add-structured-suspension/tasks.md"
git commit -m "docs: document structured suspension rollout"
```

## Self-Review

- Covers all OpenSpec requirements: structured suspension, legacy CurrentNode resume, HTTP optional field, cleanup after resume.
- Keeps CurrentNode compatibility.
- Does not implement LangGraph, checkpoint, or sub-agent runtime.
- Includes targeted and full verification commands.
