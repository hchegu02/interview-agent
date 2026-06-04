package agentkit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrToolNotFound     = errors.New("agentkit: tool not found")
	ErrPermissionDenied = errors.New("agentkit: permission denied")
)

type ToolSpec struct {
	Name          string
	Description   string
	InputSummary  string
	OutputSummary string
	Permission    Permission
	Timeout       time.Duration
}

type ToolCall struct {
	Name         string
	SessionID    string
	TraceID      string
	Input        any
	InputSummary string
	Permission   Permission
}

type ToolResult struct {
	Output  any
	Summary string
}

type Tool interface {
	Spec() ToolSpec
	Call(context.Context, ToolCall) (ToolResult, error)
}

type ToolRegistry struct {
	tools map[string]Tool
	hook  Hook
}

func NewToolRegistry(hook Hook) *ToolRegistry {
	if hook == nil {
		hook = NoopHook{}
	}
	return &ToolRegistry{tools: map[string]Tool{}, hook: hook}
}

func (r *ToolRegistry) Register(tool Tool) error {
	spec := tool.Spec()
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return fmt.Errorf("%w: empty tool name", ErrInvalidSpec)
	}
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("%w: tool %s", ErrDuplicate, name)
	}
	r.tools[name] = tool
	return nil
}

func (r *ToolRegistry) Call(ctx context.Context, call ToolCall) (ToolResult, error) {
	tool, ok := r.tools[strings.TrimSpace(call.Name)]
	if !ok {
		return ToolResult{}, fmt.Errorf("%w: %s", ErrToolNotFound, call.Name)
	}
	spec := tool.Spec()
	if call.Permission != spec.Permission {
		return ToolResult{}, fmt.Errorf("%w: tool %s requires %s, got %s", ErrPermissionDenied, spec.Name, spec.Permission, call.Permission)
	}
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}
	start := time.Now()
	_ = r.hook.HandleHook(ctx, HookEvent{
		Type:         HookBeforeTool,
		TraceID:      call.TraceID,
		SessionID:    call.SessionID,
		Name:         spec.Name,
		InputSummary: call.InputSummary,
		Permission:   spec.Permission,
	})
	result, err := tool.Call(ctx, call)
	ev := HookEvent{
		Type:          HookAfterTool,
		TraceID:       call.TraceID,
		SessionID:     call.SessionID,
		Name:          spec.Name,
		InputSummary:  call.InputSummary,
		OutputSummary: result.Summary,
		Latency:       time.Since(start),
		Permission:    spec.Permission,
	}
	if err != nil {
		ev.Error = err.Error()
	}
	_ = r.hook.HandleHook(ctx, ev)
	if err != nil {
		return ToolResult{}, err
	}
	return result, nil
}
