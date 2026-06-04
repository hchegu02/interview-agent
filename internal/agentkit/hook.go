package agentkit

import (
	"context"
	"sync"
	"time"
)

type HookType string

const (
	HookBeforeSkill        HookType = "before_skill"
	HookAfterSkill         HookType = "after_skill"
	HookBeforeTool         HookType = "before_tool"
	HookAfterTool          HookType = "after_tool"
	HookVerificationFailed HookType = "verification_failed"
)

type HookEvent struct {
	Type          HookType
	TraceID       string
	SessionID     string
	Name          string
	InputSummary  string
	OutputSummary string
	Latency       time.Duration
	Error         string
	Permission    Permission
}

type Hook interface {
	HandleHook(context.Context, HookEvent) error
}

type NoopHook struct{}

func (NoopHook) HandleHook(context.Context, HookEvent) error {
	return nil
}

type RecorderHook struct {
	mu     sync.Mutex
	events []HookEvent
}

func NewRecorderHook() *RecorderHook {
	return &RecorderHook{}
}

func (r *RecorderHook) HandleHook(_ context.Context, ev HookEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return nil
}

func (r *RecorderHook) Events() []HookEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]HookEvent, len(r.events))
	copy(out, r.events)
	return out
}
