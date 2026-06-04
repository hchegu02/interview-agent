package verify

import "interview-agent/internal/agentkit"

type ToolCallVerifier struct{}

func (ToolCallVerifier) VerifyToolEvents(events []agentkit.HookEvent) []Failure {
	failures := []Failure{}
	for _, ev := range events {
		if ev.Type != agentkit.HookAfterTool || ev.Error == "" {
			continue
		}
		failures = append(failures, Failure{Code: "tool_call_failed", Message: ev.Error, Target: ev.Name})
	}
	return failures
}
