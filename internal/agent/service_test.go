package agent

import (
	"context"
	"errors"
	"testing"

	"interview-agent/internal/skills"
)

func TestService_RunsRoutedSkill(t *testing.T) {
	service := NewService(NewRuleRouter(), skills.NewDefaultRegistry())

	resp, err := service.HandleMessage(context.Background(), AgentMessage{
		UserID:  "u1",
		Message: "帮我讲解一下 Redis 缓存击穿",
	})
	if err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if resp.Intent != IntentSkillExplain || resp.Skill != "explain" {
		t.Fatalf("response route = %+v", resp)
	}
	if resp.Result.Title == "" || resp.Result.Content == "" {
		t.Fatalf("result should be populated: %+v", resp.Result)
	}
}

func TestService_RejectsEmptyMessage(t *testing.T) {
	service := NewService(NewRuleRouter(), skills.NewDefaultRegistry())

	_, err := service.HandleMessage(context.Background(), AgentMessage{Message: "  "})
	if !errors.Is(err, ErrEmptyMessage) {
		t.Fatalf("error = %v, want ErrEmptyMessage", err)
	}
}

func TestService_UnknownSkillReturnsStructuredError(t *testing.T) {
	service := NewService(staticRouter{result: RouteResult{
		Intent:     IntentSkillQuiz,
		Skill:      "missing",
		Confidence: 1,
		Reason:     "test route",
	}}, skills.NewRegistry())

	_, err := service.HandleMessage(context.Background(), AgentMessage{Message: "出题"})
	if !errors.Is(err, skills.ErrSkillNotFound) {
		t.Fatalf("error = %v, want ErrSkillNotFound", err)
	}
	var agentErr *AgentError
	if !errors.As(err, &agentErr) {
		t.Fatalf("error = %T, want AgentError", err)
	}
	if agentErr.Code != "skill_not_found" {
		t.Fatalf("code = %q, want skill_not_found", agentErr.Code)
	}
}

func TestService_InterviewIntentReturnsGuidance(t *testing.T) {
	service := NewService(NewRuleRouter(), skills.NewDefaultRegistry())

	resp, err := service.HandleMessage(context.Background(), AgentMessage{Message: "开始模拟面试"})
	if err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if resp.Intent != IntentInterviewStart || resp.Skill != "" {
		t.Fatalf("response route = %+v", resp)
	}
	if len(resp.Result.Actions) == 0 {
		t.Fatalf("interview guidance should include actions: %+v", resp.Result)
	}
}

type staticRouter struct {
	result RouteResult
}

func (r staticRouter) Route(AgentMessage) RouteResult {
	return r.result
}
