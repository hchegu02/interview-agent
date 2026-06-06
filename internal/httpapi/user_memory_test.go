package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"interview-agent/internal/config"
	"interview-agent/internal/memory"
)

func TestGetUserMemory_ReturnsPublicProfile(t *testing.T) {
	store := memory.NewMemoryStore()
	updatedAt := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	if err := store.UpsertUserMemory(context.Background(), &memory.UserMemory{
		UserID:      "u-memory",
		Strengths:   []string{"Go 基础清楚"},
		Weaknesses:  []memory.Weakness{{Topic: "redis", Evidence: "缓存击穿回答不完整", Severity: 2, UpdatedAt: updatedAt}},
		SkillScores: map[string]float64{"go": 82, "redis": 48},
		LastAdvice:  []string{"复习 Redis 高可用"},
		UpdatedAt:   updatedAt,
	}); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}
	svc := NewInterviewService(fakeInterviewRunner{})
	svc.SetMemoryStore(store)
	server := NewServerWithInterview(&config.Config{}, svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users/u-memory/memory", nil)
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got userMemoryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.UserID != "u-memory" || got.SkillScores["go"] != 82 {
		t.Fatalf("memory response = %+v", got)
	}
	if len(got.Weaknesses) != 1 || got.Weaknesses[0].Topic != "redis" {
		t.Fatalf("weaknesses = %+v", got.Weaknesses)
	}
	if strings.Contains(rec.Body.String(), "memory_json") {
		t.Fatalf("response leaked storage details: %s", rec.Body.String())
	}
}

func TestGetUserMemory_NotFound(t *testing.T) {
	svc := NewInterviewService(fakeInterviewRunner{})
	svc.SetMemoryStore(memory.NewMemoryStore())
	server := NewServerWithInterview(&config.Config{}, svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users/missing/memory", nil)
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}
