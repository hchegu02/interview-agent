# Interview SSE Streaming Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a thin SSE stream for interview session progress without rewriting the graph or session model.

**Architecture:** Keep session state in the existing `InterviewService` and emit events through a small in-memory hub. Hook graph node lifecycle callbacks into the same hub so the stream can show live execution without coupling SSE logic into nodes or routes.

**Tech Stack:** Go, Gin, existing `internal/graph` callbacks, in-memory pub/sub.

---

### Task 1: Add an interview event hub

**Files:**
- Create: `internal/httpapi/interview_events.go`

- [ ] **Step 1: Define the event and hub types**

```go
type InterviewEvent struct {
    Type       string
    SessionID  string
    Node       string
    Status     string
    CurrentNode string
    Question   *domain.Question
    Report     *domain.Report
    Error      string
    At         time.Time
}
```

- [ ] **Step 2: Implement in-memory publish/subscribe**

```go
func (h *MemoryInterviewEventHub) Publish(ctx context.Context, event InterviewEvent)
func (h *MemoryInterviewEventHub) Subscribe(ctx context.Context, sessionID string) (<-chan InterviewEvent, func())
```

### Task 2: Wire service and graph callbacks

**Files:**
- Modify: `internal/httpapi/interview.go`
- Modify: `cmd/server/main.go`
- Modify: `internal/graphs/interview.go`

- [ ] **Step 1: Add an event publisher to `InterviewService`**

```go
type InterviewEventHub interface {
    Publish(context.Context, InterviewEvent)
    Subscribe(context.Context, string) (<-chan InterviewEvent, func())
}
```

- [ ] **Step 2: Publish session lifecycle events after start and answer**

```go
s.events.Publish(ctx, buildInterviewEvent("session.updated", sess, "", ""))
```

- [ ] **Step 3: Pass a graph callback from `cmd/server` into the graph builder**

```go
Callbacks: []graph.Callback{httpapi.NewInterviewGraphCallback(events)}
```

### Task 3: Add the SSE endpoint

**Files:**
- Create: `internal/httpapi/interview_stream.go`
- Modify: `internal/httpapi/router.go`

- [ ] **Step 1: Implement `GET /api/interview/stream`**

```go
func (s *Server) streamInterview(c *gin.Context)
```

- [ ] **Step 2: Send an initial snapshot and then forward live events**

```go
event: snapshot
data: {...}
```

### Task 4: Cover the new path with tests

**Files:**
- Modify: `internal/httpapi/interview_test.go`
- Modify: `cmd/server/main_test.go`

- [ ] **Step 1: Test hub publish/subscribe**
- [ ] **Step 2: Test SSE formatting and snapshot output**
- [ ] **Step 3: Test server wiring still builds with the shared event hub**

