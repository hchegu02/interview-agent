package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"interview-agent/internal/config"
	"interview-agent/internal/questionbank"
)

func TestQuestionBankList_FiltersAndHidesAnswerFieldsByDefault(t *testing.T) {
	server := NewServer(&config.Config{})
	server.SetQuestionBankStore(questionbank.NewMemoryStore([]questionbank.Item{
		{
			ID:             "go-001",
			Content:        "Go channel 底层结构",
			Tags:           []string{"go", "channel"},
			SkillCategory:  "go",
			Difficulty:     3,
			ExpectedPoints: []string{"hchan"},
			Rubric:         map[string]string{"good": "讲清楚 hchan"},
			SampleAnswer:   "channel 底层是 hchan",
			FollowUpHints:  []string{"sendq"},
			Status:         "active",
		},
		{ID: "redis-001", Content: "Redis 热 key", SkillCategory: "redis", Difficulty: 3, Status: "active"},
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/question-bank?skill_category=go&q=channel", nil)
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0]["id"] != "go-001" {
		t.Fatalf("items = %+v", got.Items)
	}
	raw := rec.Body.String()
	for _, hidden := range []string{"expected_points", "sample_answer", "rubric", "follow_up_hints"} {
		if strings.Contains(raw, hidden) {
			t.Fatalf("candidate response leaked %q: %s", hidden, raw)
		}
	}
}

func TestQuestionBankGet_AdminViewIncludesAnswerFields(t *testing.T) {
	server := NewServer(&config.Config{})
	server.SetQuestionBankStore(questionbank.NewMemoryStore([]questionbank.Item{{
		ID:             "go-001",
		Content:        "Go channel 底层结构",
		ExpectedPoints: []string{"hchan"},
		Rubric:         map[string]string{"good": "讲清楚 hchan"},
		SampleAnswer:   "channel 底层是 hchan",
		FollowUpHints:  []string{"sendq"},
		Status:         "active",
	}}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/question-bank/go-001?view=admin", nil)
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	for _, marker := range []string{"expected_points", "sample_answer", "rubric", "follow_up_hints"} {
		if !strings.Contains(raw, marker) {
			t.Fatalf("admin response missing %q: %s", marker, raw)
		}
	}
}

func TestQuestionBankFacets(t *testing.T) {
	server := NewServer(&config.Config{})
	server.SetQuestionBankStore(questionbank.NewMemoryStore([]questionbank.Item{
		{ID: "go-001", Tags: []string{"go"}, SkillCategory: "go", Difficulty: 3, Scenario: "fundamentals", Status: "active"},
		{ID: "redis-001", Tags: []string{"redis"}, SkillCategory: "redis", Difficulty: 4, Scenario: "troubleshooting", Status: "active"},
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/question-bank/facets", nil)
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	for _, marker := range []string{`"go":1`, `"redis":1`, `"fundamentals":1`, `"troubleshooting":1`} {
		if !strings.Contains(rec.Body.String(), marker) {
			t.Fatalf("facets missing %q: %s", marker, rec.Body.String())
		}
	}
}
