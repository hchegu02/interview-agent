package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"interview-agent/internal/config"
	"interview-agent/internal/llm"
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

func TestQuestionBankGeneration_CreateGetAndStage(t *testing.T) {
	importStore := questionbank.NewMemoryImportStore()
	if err := importStore.AddChunks(context.Background(), []questionbank.ImportChunk{{
		ID:      "imp-001:chunk:001",
		JobID:   "imp-001",
		Index:   1,
		Content: "Go 并发治理需要识别 goroutine 泄漏，并用 pprof 和 context 取消定位问题。",
	}}); err != nil {
		t.Fatalf("AddChunks: %v", err)
	}
	store := questionbank.NewMemoryStore(nil)
	server := NewServer(&config.Config{})
	server.SetQuestionBankGenerationService(questionbank.NewGenerationService(questionbank.GenerationServiceDeps{
		Imports: importStore,
		Writer:  store,
		Model: &httpQuestionGenerationModel{responses: []func([]llm.Message) string{
			func([]llm.Message) string {
				return `{"concepts":[{"title":"goroutine 泄漏","skill":"go","difficulty_hint":3,"keywords":["goroutine"],"question_angles":["debugging"],"evidence_refs":[{"chunk_id":"imp-001:chunk:001","quote":"context 取消"}]}]}`
			},
			func(messages []llm.Message) string {
				conceptID := httpConceptIDFromPrompt(messages)
				return `{"candidates":[{"concept_id":"` + conceptID + `","content":"如何排查 goroutine 泄漏？","question_type":"interview","target_dimension":"debugging","answer":"看 pprof 和 context。","explanation":"题目基于原文证据。","tags":["go"],"skill_category":"go","difficulty":3,"expected_points":["说明 goroutine 泄漏现象"],"rubric":{"good":"能说明 pprof 和 context"},"sample_answer":"通过 pprof 定位阻塞点，并检查 context 取消。","follow_up_hints":["如何避免再次泄漏？"],"source_refs":[{"chunk_id":"imp-001:chunk:001","quote":"context 取消"}]}]}`
			},
		}},
	}))

	body := strings.NewReader(`{"source_job_id":"imp-001","topic":"goroutine 泄漏","question_type":"interview","count":1,"difficulty":3,"target_dimension":"debugging","skill_category":"go"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/question-bank/generation-jobs", body)
	req.Header.Set("Content-Type", "application/json")
	server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Job questionbank.GenerationJob `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Job.ID == "" || len(created.Job.Candidates) != 1 {
		t.Fatalf("created job = %+v", created.Job)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/question-bank/generation-jobs/"+created.Job.ID, nil)
	server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/question-bank/generation-jobs/"+created.Job.ID+"/stage", nil)
	server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stage status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"agent_review_status":"needs_human_review"`) {
		t.Fatalf("stage response should require human review: %s", rec.Body.String())
	}
}

type httpQuestionGenerationModel struct {
	responses []func([]llm.Message) string
	calls     int
}

func (m *httpQuestionGenerationModel) Generate(_ context.Context, messages []llm.Message, _ llm.Options) (*llm.Response, error) {
	if m.calls >= len(m.responses) {
		return &llm.Response{Content: `{"candidates":[]}`}, nil
	}
	content := m.responses[m.calls](messages)
	m.calls++
	return &llm.Response{Content: content}, nil
}

func (m *httpQuestionGenerationModel) Stream(context.Context, []llm.Message, llm.Options) (<-chan llm.Chunk, error) {
	ch := make(chan llm.Chunk)
	close(ch)
	return ch, nil
}

func (m *httpQuestionGenerationModel) Name() string { return "http-question-generation-model" }

func httpConceptIDFromPrompt(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		content := messages[i].Content
		idx := strings.Index(content, `"concept_id":"concept-`)
		if idx < 0 {
			continue
		}
		start := idx + len(`"concept_id":"`)
		end := strings.Index(content[start:], `"`)
		if end > 0 {
			return content[start : start+end]
		}
	}
	return "concept-missing"
}
