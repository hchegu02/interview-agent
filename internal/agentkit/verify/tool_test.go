package verify

import (
	"testing"

	"interview-agent/internal/agentkit"
)

func TestToolCallVerifierRequiresAfterStatus(t *testing.T) {
	failures := ToolCallVerifier{}.VerifyToolEvents([]agentkit.HookEvent{
		{Type: agentkit.HookBeforeTool, TraceID: "tr1", SessionID: "s1", Name: DefaultExpectedTool, Permission: agentkit.PermissionReadOnly},
		{Type: agentkit.HookAfterTool, TraceID: "tr1", SessionID: "s1", Name: DefaultExpectedTool, Permission: agentkit.PermissionReadOnly},
	})

	if !hasFailureCode(failures, "tool_status_missing") {
		t.Fatalf("failures = %+v, want tool_status_missing", failures)
	}
}

func TestToolCallVerifierRequiresFailedErrorClass(t *testing.T) {
	failures := ToolCallVerifier{}.VerifyToolEvents([]agentkit.HookEvent{
		{Type: agentkit.HookBeforeTool, TraceID: "tr1", SessionID: "s1", Name: DefaultExpectedTool, Permission: agentkit.PermissionReadOnly},
		{Type: agentkit.HookAfterTool, TraceID: "tr1", SessionID: "s1", Name: DefaultExpectedTool, Permission: agentkit.PermissionReadOnly, Status: "failed", Error: "bad repo"},
	})

	if !hasFailureCode(failures, "tool_error_class_missing") {
		t.Fatalf("failures = %+v, want tool_error_class_missing", failures)
	}
}

func TestToolCallVerifierAcceptsPairedStatusAndErrorClass(t *testing.T) {
	failures := ToolCallVerifier{}.VerifyToolEvents([]agentkit.HookEvent{
		{Type: agentkit.HookBeforeTool, TraceID: "tr1", SessionID: "s1", Name: DefaultExpectedTool, Permission: agentkit.PermissionReadOnly},
		{Type: agentkit.HookAfterTool, TraceID: "tr1", SessionID: "s1", Name: DefaultExpectedTool, Permission: agentkit.PermissionReadOnly, Status: "success"},
	})

	if len(failures) != 0 {
		t.Fatalf("failures = %+v, want none", failures)
	}
}

func hasFailureCode(failures []Failure, code string) bool {
	for _, failure := range failures {
		if failure.Code == code {
			return true
		}
	}
	return false
}
