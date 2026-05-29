package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
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
	if !strings.Contains(raw, "embedding_status") {
		t.Fatalf("admin response should include embedding_status: %s", raw)
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

func TestQuestionBankImport_LocalJSONThenCommit(t *testing.T) {
	store := questionbank.NewMemoryStore(nil)
	importStore := questionbank.NewMemoryImportStore()
	server := NewServer(&config.Config{})
	server.SetQuestionBankStore(store)
	server.SetQuestionBankImportService(questionbank.NewImportService(questionbank.ImportServiceDeps{
		Imports: importStore,
		Writer:  store,
	}))

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("source_type", "question_bank"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "questions.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(`[{"id":"go-import-001","content":"Go channel close 语义是什么？","skill_category":"go","difficulty":3}]`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/question-bank/imports", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Job questionbank.ImportJob `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Job.Status != questionbank.ImportStatusReady || created.Job.ValidItems != 1 {
		t.Fatalf("job = %+v", created.Job)
	}
	if _, err := store.Get(req.Context(), "go-import-001"); err == nil {
		t.Fatal("uploaded item should not be visible before commit")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/question-bank/imports/"+created.Job.ID+"/commit", nil)
	server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("commit status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if _, err := store.Get(req.Context(), "go-import-001"); err != nil {
		t.Fatalf("committed item should be visible: %v", err)
	}
}

func TestQuestionBankImport_ReviewRejectsItemBeforeCommit(t *testing.T) {
	store := questionbank.NewMemoryStore(nil)
	importStore := questionbank.NewMemoryImportStore()
	server := NewServer(&config.Config{})
	server.SetQuestionBankStore(store)
	server.SetQuestionBankImportService(questionbank.NewImportService(questionbank.ImportServiceDeps{
		Imports: importStore,
		Writer:  store,
	}))

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("source_type", "question_bank"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "questions.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(`[{"id":"go-reject-http-001","content":"Go map 并发读写？","skill_category":"go","difficulty":3}]`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/question-bank/imports", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Job questionbank.ImportJob `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	reviewBody := strings.NewReader(`{"action":"reject","item_ids":["` + created.Job.ID + `:go-reject-http-001"]}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/question-bank/imports/"+created.Job.ID+"/items/review", reviewBody)
	req.Header.Set("Content-Type", "application/json")
	server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("review status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"review_status":"rejected"`) {
		t.Fatalf("review response should include rejected item: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/question-bank/imports/"+created.Job.ID+"/commit", nil)
	server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("commit status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if _, err := store.Get(req.Context(), "go-reject-http-001"); err == nil {
		t.Fatal("rejected item should not be visible after commit")
	}
}

func TestQuestionBankImport_AsyncLocalJSON(t *testing.T) {
	store := questionbank.NewMemoryStore(nil)
	importStore := questionbank.NewMemoryImportStore()
	server := NewServer(&config.Config{})
	server.SetQuestionBankStore(store)
	server.SetQuestionBankImportService(questionbank.NewImportService(questionbank.ImportServiceDeps{
		Imports: importStore,
		Writer:  store,
	}))

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("source_type", "question_bank"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "questions.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(`[{"id":"async-http-001","content":"Redis 慢查询如何定位？","skill_category":"redis","difficulty":3}]`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/question-bank/imports?async=true", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"queued"`) {
		t.Fatalf("async response should return queued job: %s", rec.Body.String())
	}
}
