package verify

import (
	"strings"

	"interview-agent/internal/agentkit"
)

const DefaultExpectedTool = "github.project_analyze"

type ToolCallVerifier struct {
	ExpectedTool string
}

func (v ToolCallVerifier) VerifyToolEvents(events []agentkit.HookEvent) []Failure {
	failures := []Failure{}
	pending := map[toolEventKey]int{}
	expectedTool := DefaultExpectedTool
	if v.ExpectedTool != "" {
		expectedTool = v.ExpectedTool
	}
	expectedCalled := false
	for _, ev := range events {
		if ev.Type != agentkit.HookBeforeTool && ev.Type != agentkit.HookAfterTool {
			continue
		}
		if ev.Permission != agentkit.PermissionReadOnly {
			failures = append(failures, Failure{
				Code:    "tool_permission_not_read_only",
				Message: "tool call permission must be read_only",
				Target:  ev.Name,
			})
		}
		key := makeToolEventKey(ev)
		switch ev.Type {
		case agentkit.HookBeforeTool:
			pending[key]++
		case agentkit.HookAfterTool:
			if ev.Name == expectedTool {
				expectedCalled = true
			}
			if pending[key] == 0 {
				failures = append(failures, Failure{
					Code:    "tool_after_without_before",
					Message: "tool after event has no matching before event",
					Target:  ev.Name,
				})
			} else {
				pending[key]--
			}
			status := strings.TrimSpace(ev.Status)
			switch status {
			case "":
				failures = append(failures, Failure{
					Code:    "tool_status_missing",
					Message: "tool after event must include status",
					Target:  ev.Name,
				})
			case "success":
				if ev.Error != "" {
					failures = append(failures, Failure{Code: "tool_status_invalid", Message: "success tool event must not include error", Target: ev.Name})
				}
			case "failed":
				if strings.TrimSpace(ev.ErrorClass) == "" {
					failures = append(failures, Failure{
						Code:    "tool_error_class_missing",
						Message: "failed tool event must include error_class",
						Target:  ev.Name,
					})
				}
			default:
				failures = append(failures, Failure{
					Code:    "tool_status_invalid",
					Message: "tool after event status must be success or failed",
					Target:  ev.Name,
				})
			}
			if ev.Error != "" {
				failures = append(failures, Failure{Code: "tool_call_failed", Message: ev.Error, Target: ev.Name})
			}
		}
	}
	for key, count := range pending {
		for i := 0; i < count; i++ {
			failures = append(failures, Failure{
				Code:    "tool_before_without_after",
				Message: "tool before event has no matching after event",
				Target:  key.name,
			})
		}
	}
	if !expectedCalled {
		failures = append(failures, Failure{
			Code:    "tool_expected_not_called",
			Message: "expected tool was not called",
			Target:  expectedTool,
		})
	}
	return failures
}

type toolEventKey struct {
	traceID   string
	sessionID string
	name      string
}

func makeToolEventKey(ev agentkit.HookEvent) toolEventKey {
	return toolEventKey{traceID: ev.TraceID, sessionID: ev.SessionID, name: ev.Name}
}
