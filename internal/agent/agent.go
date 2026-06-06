package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"interview-agent/internal/agentkit"
	"interview-agent/internal/skills"
)

const (
	IntentSkillQuiz          = "skill.quiz"
	IntentSkillExplain       = "skill.explain"
	IntentSkillProjectPolish = "skill.project_polish"
	IntentInterviewStart     = "interview.start"
	IntentChat               = "chat"
)

var ErrEmptyMessage = errors.New("agent message is empty")

type AgentError struct {
	Code    string
	Message string
	Err     error
}

func (e *AgentError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AgentError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type AgentMessage struct {
	UserID  string            `json:"user_id,omitempty"`
	Message string            `json:"message"`
	Context map[string]string `json:"context,omitempty"`
}

type RouteResult struct {
	Intent     string  `json:"intent"`
	Skill      string  `json:"skill,omitempty"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

type AgentResponse struct {
	Intent     string             `json:"intent"`
	Skill      string             `json:"skill,omitempty"`
	Confidence float64            `json:"confidence"`
	Reason     string             `json:"reason"`
	Result     skills.SkillResult `json:"result"`
	ToolTrace  []skills.ToolTrace `json:"tool_trace,omitempty"`
}

type Router interface {
	Route(msg AgentMessage) RouteResult
}

type RuleRouter struct{}

func NewRuleRouter() RuleRouter {
	return RuleRouter{}
}

func (RuleRouter) Route(msg AgentMessage) RouteResult {
	text := strings.ToLower(strings.TrimSpace(msg.Message))
	switch {
	case containsAny(text, "出题", "测验", "quiz", "练习", "考我"):
		return RouteResult{Intent: IntentSkillQuiz, Skill: "quiz", Confidence: 0.9, Reason: "matched quiz keywords"}
	case containsAny(text, "解释", "讲解", "原理", "为什么", "explain"):
		return RouteResult{Intent: IntentSkillExplain, Skill: "explain", Confidence: 0.88, Reason: "matched explain keywords"}
	case containsAny(text, "项目", "简历", "亮点", "润色", "polish"):
		return RouteResult{Intent: IntentSkillProjectPolish, Skill: "project_polish", Confidence: 0.86, Reason: "matched project polish keywords"}
	case containsAny(text, "面试", "开始", "模拟", "interview"):
		return RouteResult{Intent: IntentInterviewStart, Confidence: 0.78, Reason: "matched interview keywords"}
	default:
		return RouteResult{Intent: IntentChat, Confidence: 0.3, Reason: "fallback chat intent"}
	}
}

type Service struct {
	router Router
	skills *skills.Registry
}

func NewService(router Router, registry *skills.Registry) *Service {
	return &Service{router: router, skills: registry}
}

func NewDefaultService() *Service {
	tools := agentkit.NewToolRegistry(agentkit.NoopHook{})
	if err := agentkit.RegisterDefaultMCPTools(tools, agentkit.NewMockMCPClient()); err != nil {
		return NewService(NewRuleRouter(), skills.NewDefaultRegistry())
	}
	return NewService(NewRuleRouter(), skills.NewDefaultRegistryWithTools(tools))
}

func (s *Service) HandleMessage(ctx context.Context, msg AgentMessage) (AgentResponse, error) {
	if strings.TrimSpace(msg.Message) == "" {
		return AgentResponse{}, ErrEmptyMessage
	}
	if s == nil || s.router == nil {
		return AgentResponse{}, fmt.Errorf("agent service is not configured")
	}
	route := s.router.Route(msg)
	resp := AgentResponse{
		Intent:     route.Intent,
		Skill:      route.Skill,
		Confidence: route.Confidence,
		Reason:     route.Reason,
	}
	if route.Skill == "" {
		resp.Result = guidanceResult(route.Intent)
		return resp, nil
	}
	result, err := s.skills.Run(ctx, route.Skill, skills.SkillInput{
		UserID:  msg.UserID,
		Message: msg.Message,
		Context: msg.Context,
	})
	if err != nil {
		if errors.Is(err, skills.ErrSkillNotFound) {
			return AgentResponse{}, &AgentError{
				Code:    "skill_not_found",
				Message: "skill is not registered",
				Err:     err,
			}
		}
		return AgentResponse{}, err
	}
	resp.Result = result
	resp.ToolTrace = append([]skills.ToolTrace(nil), result.ToolTrace...)
	return resp, nil
}

func guidanceResult(intent string) skills.SkillResult {
	if intent == IntentInterviewStart {
		return skills.SkillResult{
			Title:   "开始模拟面试",
			Content: "请通过 /api/interview/start 上传简历和 JD 后进入正式面试流程。",
			Actions: []skills.Action{{
				Type:  "start_interview",
				Label: "进入面试创建",
			}},
		}
	}
	return skills.SkillResult{Title: "普通对话", Content: "当前版本支持测验、知识讲解和项目亮点提炼。"}
}

func containsAny(text string, words ...string) bool {
	for _, word := range words {
		if strings.Contains(text, strings.ToLower(word)) {
			return true
		}
	}
	return false
}
