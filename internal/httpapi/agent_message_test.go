package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"interview-agent/internal/agent"
	"interview-agent/internal/agentkit"
	"interview-agent/internal/config"
	"interview-agent/internal/skills"
)

func TestAgentMessage_RoutesSkill(t *testing.T) {
	server := NewServer(&config.Config{})
	server.SetAgentService(agent.NewService(agent.NewRuleRouter(), skills.NewDefaultRegistry()))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/message", bytes.NewBufferString(`{"user_id":"u1","message":"解释一下 Redis 缓存击穿"}`))
	req.Header.Set("Content-Type", "application/json")
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp agent.AgentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Intent != agent.IntentSkillExplain || resp.Skill != "explain" {
		t.Fatalf("response = %+v", resp)
	}
	if resp.Confidence <= 0 || resp.Reason == "" || resp.Result.Title == "" || resp.Result.Content == "" {
		t.Fatalf("structured response fields should be populated: %+v", resp)
	}
}

func TestAgentMessage_ProjectPolishUsesDefaultMockToolFixture(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/agent_message/project_polish_mock_request.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	server := NewServer(&config.Config{})
	server.SetAgentService(agent.NewDefaultService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/message", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp agent.AgentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Intent != agent.IntentSkillProjectPolish || resp.Skill != "project_polish" {
		t.Fatalf("response = %+v", resp)
	}
	for _, marker := range []string{
		"项目亮点提炼",
		"interview-agent mock GitHub project analysis",
		"uses layered backend structure",
	} {
		if !strings.Contains(resp.Result.Title+" "+resp.Result.Content, marker) {
			t.Fatalf("response missing marker %q: %+v", marker, resp)
		}
	}
	if len(resp.ToolTrace) != 1 {
		t.Fatalf("tool trace len = %d, want 1", len(resp.ToolTrace))
	}
	if resp.ToolTrace[0].Name != "github.project_analyze" || resp.ToolTrace[0].Status != "success" {
		t.Fatalf("tool trace = %+v, want github success trace", resp.ToolTrace[0])
	}
	for _, want := range []string{`"tool_trace"`, `"name":"github.project_analyze"`, `"status":"success"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("body should contain %s, got %s", want, rec.Body.String())
		}
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode raw response: %v", err)
	}
	if _, ok := body["tool_trace"]; !ok {
		t.Fatalf("top-level tool_trace should be present: %s", rec.Body.String())
	}
	result, ok := body["result"].(map[string]any)
	if !ok {
		t.Fatalf("result should be an object: %s", rec.Body.String())
	}
	if _, ok := result["tool_trace"]; ok {
		t.Fatalf("result.tool_trace should not be serialized: %s", rec.Body.String())
	}
}

func TestAgentMessage_NoToolOmitsToolTrace(t *testing.T) {
	server := NewServer(&config.Config{})
	server.SetAgentService(agent.NewService(agent.NewRuleRouter(), skills.NewDefaultRegistry()))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/message", bytes.NewBufferString(`{"user_id":"u1","message":"帮我润色简历项目亮点"}`))
	req.Header.Set("Content-Type", "application/json")
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"tool_trace"`) {
		t.Fatalf("body should omit tool_trace when no tool is called: %s", rec.Body.String())
	}
}

func TestAgentMessage_ProjectPolishToolFailureReturnsFailedTrace(t *testing.T) {
	tools := agentkit.NewToolRegistry(agentkit.NoopHook{})
	if err := tools.Register(httpFailingTool{
		spec: agentkit.ToolSpec{Name: "github.project_analyze", Permission: agentkit.PermissionReadOnly},
		err:  errors.New("tool down"),
	}); err != nil {
		t.Fatalf("register failing tool: %v", err)
	}
	server := NewServer(&config.Config{})
	server.SetAgentService(agent.NewService(agent.NewRuleRouter(), skills.NewDefaultRegistryWithTools(tools)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/message", bytes.NewBufferString(`{"user_id":"u1","message":"帮我润色 https://github.com/acme/interview-agent 这个项目"}`))
	req.Header.Set("Content-Type", "application/json")
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp agent.AgentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.ToolTrace) != 1 {
		t.Fatalf("tool trace len = %d, want 1", len(resp.ToolTrace))
	}
	trace := resp.ToolTrace[0]
	if trace.Name != "github.project_analyze" || trace.Status != "failed" || trace.ErrorClass != "tool_call_failed" {
		t.Fatalf("tool trace = %+v, want failed github trace", trace)
	}
	if !strings.Contains(resp.Result.Content, "背景问题、你的动作、技术取舍") {
		t.Fatalf("fallback content missing generic advice: %s", resp.Result.Content)
	}
}

func TestAgentMessage_EmptyMessage(t *testing.T) {
	server := NewServer(&config.Config{})
	server.SetAgentService(agent.NewService(agent.NewRuleRouter(), skills.NewDefaultRegistry()))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/message", bytes.NewBufferString(`{"message":" "}`))
	req.Header.Set("Content-Type", "application/json")
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAgentMessage_UnknownSkillReturnsStructuredError(t *testing.T) {
	server := NewServer(&config.Config{})
	server.SetAgentService(agent.NewService(httpStaticRouter{result: agent.RouteResult{
		Intent:     agent.IntentSkillQuiz,
		Skill:      "missing",
		Confidence: 1,
		Reason:     "test route",
	}}, skills.NewRegistry()))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/message", bytes.NewBufferString(`{"message":"出题"}`))
	req.Header.Set("Content-Type", "application/json")
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != "skill_not_found" || body["error"] == "" {
		t.Fatalf("body = %+v, want structured skill error", body)
	}
}

func TestAgentMessage_ServiceNotConfigured(t *testing.T) {
	server := NewServer(&config.Config{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/message", bytes.NewBufferString(`{"message":"解释一下 MVCC"}`))
	req.Header.Set("Content-Type", "application/json")
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

type httpStaticRouter struct {
	result agent.RouteResult
}

func (r httpStaticRouter) Route(agent.AgentMessage) agent.RouteResult {
	return r.result
}

type httpFailingTool struct {
	spec agentkit.ToolSpec
	err  error
}

func (f httpFailingTool) Spec() agentkit.ToolSpec { return f.spec }
func (f httpFailingTool) Call(context.Context, agentkit.ToolCall) (agentkit.ToolResult, error) {
	return agentkit.ToolResult{}, f.err
}
