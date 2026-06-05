package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"interview-agent/internal/agent"
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
