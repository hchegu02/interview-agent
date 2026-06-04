package agentkit

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubTool struct {
	spec ToolSpec
	run  func(context.Context, ToolCall) (ToolResult, error)
}

func (s stubTool) Spec() ToolSpec { return s.spec }
func (s stubTool) Call(ctx context.Context, call ToolCall) (ToolResult, error) {
	return s.run(ctx, call)
}

func TestToolRegistryCallsToolAndRecordsHooks(t *testing.T) {
	rec := NewRecorderHook()
	reg := NewToolRegistry(rec)
	err := reg.Register(stubTool{
		spec: ToolSpec{Name: "session.read", Permission: PermissionReadOnly, Timeout: time.Second},
		run: func(_ context.Context, call ToolCall) (ToolResult, error) {
			if call.Name != "session.read" {
				t.Fatalf("call name = %s", call.Name)
			}
			return ToolResult{Output: map[string]string{"status": "ok"}, Summary: "read session"}, nil
		},
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	result, err := reg.Call(context.Background(), ToolCall{Name: "session.read", Permission: PermissionReadOnly})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if result.Summary != "read session" {
		t.Fatalf("result = %+v", result)
	}
	if events := rec.Events(); len(events) != 2 || events[0].Type != HookBeforeTool || events[1].Type != HookAfterTool {
		t.Fatalf("hook events = %+v", events)
	}
}

func TestToolRegistryRejectsUnknownAndPermissionDenied(t *testing.T) {
	reg := NewToolRegistry(NoopHook{})
	if _, err := reg.Call(context.Background(), ToolCall{Name: "missing", Permission: PermissionReadOnly}); !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	err := reg.Register(stubTool{
		spec: ToolSpec{Name: "report.write", Permission: PermissionWriteReport},
		run:  func(context.Context, ToolCall) (ToolResult, error) { return ToolResult{}, nil },
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := reg.Call(context.Background(), ToolCall{Name: "report.write", Permission: PermissionReadOnly}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("permission err = %v", err)
	}
}

func TestToolRegistryTimeout(t *testing.T) {
	reg := NewToolRegistry(NoopHook{})
	err := reg.Register(stubTool{
		spec: ToolSpec{Name: "slow", Permission: PermissionReadOnly, Timeout: time.Nanosecond},
		run: func(ctx context.Context, _ ToolCall) (ToolResult, error) {
			<-ctx.Done()
			return ToolResult{}, ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := reg.Call(context.Background(), ToolCall{Name: "slow", Permission: PermissionReadOnly}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout err = %v", err)
	}
}
