package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"interview-agent/internal/agent"
	"interview-agent/internal/config"
	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
	"interview-agent/internal/httpapi"
	"interview-agent/internal/llm"
	"interview-agent/internal/questionbank"
	"interview-agent/internal/retriever"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBuildInterviewRunner_MockModeStartsInterview(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.LLM.Mode = "mock"
	cfg.Embedding.Mode = "mock"
	cfg.PostgresDSN = ""
	events := httpapi.NewMemoryInterviewEventHub(16)

	deps, cleanupDeps, err := buildAppDeps(context.Background(), cfg)
	if err != nil {
		t.Fatalf("build deps: %v", err)
	}
	defer cleanupDeps()
	if deps.PGPool != nil {
		t.Fatal("expected nil PG pool without DSN")
	}

	runner, cleanup, err := buildInterviewRunner(context.Background(), cfg, deps, events, nil, nil, nil)
	if err != nil {
		t.Fatalf("build runner: %v", err)
	}
	defer cleanup()

	sess := &domain.Session{
		ID:     "server-wiring-test",
		Status: domain.StatusRunning,
		JobProfile: &domain.JobProfile{
			JDRawText: "需要 Go 后端工程师，熟悉 Redis 和并发编程",
		},
		CandProfile: &domain.CandidateProfile{
			ResumeRawText: "两年 Go 后端经验，做过 Redis 缓存服务",
		},
		WorkingMemory: domain.NewWorkingMemory(),
	}

	if err := runner.runner.Invoke(context.Background(), sess); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if sess.CurrentNode != "pick_next" {
		t.Fatalf("current node = %q, want pick_next", sess.CurrentNode)
	}
	if len(sess.Rounds) != 1 {
		t.Fatalf("rounds = %d, want 1", len(sess.Rounds))
	}
	if sess.Rounds[0].Question.ID == "" {
		t.Fatalf("question was not selected: %+v", sess.Rounds[0].Question)
	}
}

func TestBuildInterviewRunner_WiresOperationalMetrics(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.LLM.Mode = "mock"
	cfg.Embedding.Mode = "mock"
	cfg.PostgresDSN = ""

	deps, cleanupDeps, err := buildAppDeps(context.Background(), cfg)
	if err != nil {
		t.Fatalf("build deps: %v", err)
	}
	defer cleanupDeps()
	events := httpapi.NewMemoryInterviewEventHub(16)
	server := httpapi.NewServer(cfg)
	runner, cleanupRunner, err := buildInterviewRunner(context.Background(), cfg, deps, events, server.GraphMetricsCallback(), server.ObserveLLMCall, server.WrapRetriever)
	if err != nil {
		t.Fatalf("build runner: %v", err)
	}
	defer cleanupRunner()
	svc, cleanupSvc, err := buildInterviewService(context.Background(), cfg, deps, runner.runner, events)
	if err != nil {
		t.Fatalf("build service: %v", err)
	}
	defer cleanupSvc()
	server.SetInterviewService(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/interview/start", bytes.NewBufferString(`{
		"session_id":"metrics-wiring",
		"user_id":"u1",
		"jd_text":"需要 Go 后端工程师，熟悉 Redis 和并发编程",
		"resume_text":"两年 Go 后端经验，做过 Redis 缓存服务"
	}`))
	req.Header.Set("Content-Type", "application/json")
	router := server.Router()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", rec.Code, rec.Body.String())
	}

	metrics := httptest.NewRecorder()
	router.ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	got := metrics.Body.String()
	for _, marker := range []string{
		`interview_graph_node_total{node="pick_next",status="suspended"} 1`,
		`interview_llm_calls_total{model="mock",err_class="ok"}`,
		`interview_llm_tokens_total{model="mock",type="prompt"}`,
	} {
		if !strings.Contains(got, marker) {
			t.Fatalf("metrics missing %q\n--- metrics ---\n%s", marker, got)
		}
	}
}

func TestBuildInterviewService_NoPostgresUsesMemoryStore(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.PostgresDSN = ""
	deps, cleanupDeps, err := buildAppDeps(context.Background(), cfg)
	if err != nil {
		t.Fatalf("build deps: %v", err)
	}
	defer cleanupDeps()

	events := httpapi.NewMemoryInterviewEventHub(16)
	runner, cleanup, err := buildInterviewRunner(context.Background(), cfg, deps, events, nil, nil, nil)
	if err != nil {
		t.Fatalf("build runner: %v", err)
	}
	defer cleanup()

	svc, cleanupSvc, err := buildInterviewService(context.Background(), cfg, deps, runner.runner, events)
	if err != nil {
		t.Fatalf("build service: %v", err)
	}
	defer cleanupSvc()
	if svc == nil {
		t.Fatal("expected service")
	}
	if !interviewServiceMemoryStoreConfigured(svc) {
		t.Fatal("expected long-term memory store to be configured")
	}
}

func TestBuildInterviewService_PostgresUsesPGMemoryStore(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.PostgresDSN = "postgres://example"
	deps := appDeps{PGPool: &pgxpool.Pool{}}
	events := httpapi.NewMemoryInterviewEventHub(16)

	svc, cleanupSvc, err := buildInterviewService(context.Background(), cfg, deps, &graph.Runnable{}, events)
	if err != nil {
		t.Fatalf("build service: %v", err)
	}
	defer cleanupSvc()
	if got := interviewServiceMemoryStoreType(svc); !strings.Contains(got, "PGStore") {
		t.Fatalf("memory store type = %q, want PGStore", got)
	}
}

func interviewServiceMemoryStoreConfigured(svc *httpapi.InterviewService) bool {
	if svc == nil {
		return false
	}
	field := reflect.ValueOf(svc).Elem().FieldByName("memoryStore")
	return field.IsValid() && !field.IsNil()
}

func interviewServiceMemoryStoreType(svc *httpapi.InterviewService) string {
	if svc == nil {
		return ""
	}
	field := reflect.ValueOf(svc).Elem().FieldByName("memoryStore")
	if !field.IsValid() || field.IsNil() {
		return ""
	}
	return field.Elem().Type().String()
}

func TestBuildQuestionBankStore_NoPostgresLoadsSeed(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.PostgresDSN = ""
	deps, cleanupDeps, err := buildAppDeps(context.Background(), cfg)
	if err != nil {
		t.Fatalf("build deps: %v", err)
	}
	defer cleanupDeps()

	store, err := buildQuestionBankStore(deps)
	if err != nil {
		t.Fatalf("build question bank store: %v", err)
	}
	got, err := store.List(context.Background(), questionbank.Filter{Limit: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(got.Items))
	}
}

func TestBuildRetrievalPipelineUsesQuestionBankStoreForLocalStages(t *testing.T) {
	vector := fixedServerRetriever{results: []retriever.Result{{ID: "vector", Content: "Go runtime", Score: 0.9}}}
	store := questionbank.NewMemoryStore([]questionbank.Item{
		{
			ID:             "bm25",
			Content:        "Redis AOF rewrite 期间新写入怎么处理？",
			Tags:           []string{"redis_persistence"},
			SkillCategory:  "redis",
			Difficulty:     3,
			ExpectedPoints: []string{"AOF rewrite"},
			Status:         "active",
		},
	})

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	got, err := buildRetrievalPipeline(context.Background(), cfg, vector, store)
	if err != nil {
		t.Fatal(err)
	}
	searcher, ok := got.(interface {
		Search(context.Context, retriever.Query) (retriever.PipelineResult, error)
	})
	if !ok {
		t.Fatalf("pipeline does not expose Search")
	}
	result, err := searcher.Search(context.Background(), retriever.Query{Text: "Redis AOF rewrite", K: 5})
	if err != nil {
		t.Fatal(err)
	}
	foundBM25 := false
	for _, stage := range result.Trace.Stages {
		if stage.Stage == retriever.StageBM25 && stage.Count > 0 {
			foundBM25 = true
		}
	}
	if !foundBM25 {
		t.Fatalf("trace stages = %+v, want BM25 candidates from question bank store", result.Trace.Stages)
	}
}

func TestBuildRetrievalPipelineUsesHTTPRerankerWhenConfigured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"scores":[{"id":"bm25","score":0.95},{"id":"vector","score":0.10}]}`))
	}))
	defer server.Close()

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Rerank.Mode = "http"
	cfg.Rerank.Endpoint = server.URL

	vector := fixedServerRetriever{results: []retriever.Result{{ID: "vector", Content: "Go runtime", Score: 0.9}}}
	store := questionbank.NewMemoryStore([]questionbank.Item{
		{
			ID:            "bm25",
			Content:       "Redis AOF rewrite 期间新写入怎么处理？",
			SkillCategory: "redis",
			Difficulty:    3,
			Status:        "active",
		},
	})

	got, err := buildRetrievalPipeline(context.Background(), cfg, vector, store)
	if err != nil {
		t.Fatal(err)
	}
	searcher := got.(interface {
		Search(context.Context, retriever.Query) (retriever.PipelineResult, error)
	})
	result, err := searcher.Search(context.Background(), retriever.Query{Text: "Redis AOF rewrite", K: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) < 2 || result.Results[0].ID != "bm25" {
		t.Fatalf("results = %+v, want http reranker to promote bm25", result.Results)
	}
}

type fixedServerRetriever struct {
	results []retriever.Result
}

func (f fixedServerRetriever) Retrieve(context.Context, retriever.Query) ([]retriever.Result, error) {
	return append([]retriever.Result(nil), f.results...), nil
}

func TestBuildInterviewService_IgnoresTypedNilCoordinator(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.PostgresDSN = ""
	cfg.LLM.Mode = "mock"
	cfg.Embedding.Mode = "mock"

	events := httpapi.NewMemoryInterviewEventHub(16)
	deps, cleanupDeps, err := buildAppDeps(context.Background(), cfg)
	if err != nil {
		t.Fatalf("build deps: %v", err)
	}
	defer cleanupDeps()

	runner, cleanupRunner, err := buildInterviewRunner(context.Background(), cfg, deps, events, nil, nil, nil)
	if err != nil {
		t.Fatalf("build runner: %v", err)
	}
	defer cleanupRunner()

	var nilCoordinator *httpapi.RedisSessionCoordinator
	svc, cleanupSvc, err := buildInterviewService(context.Background(), cfg, deps, runner.runner, events, nilCoordinator)
	if err != nil {
		t.Fatalf("build service: %v", err)
	}
	defer cleanupSvc()

	server := httpapi.NewServerWithInterview(cfg, svc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/interview/start", bytes.NewBufferString(`{
		"session_id":"typed-nil-coordinator",
		"user_id":"u1",
		"jd_text":"需要 Go 后端工程师",
		"resume_text":"两年 Go 后端经验"
	}`))
	req.Header.Set("Content-Type", "application/json")
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("start should ignore typed nil coordinator: status=%d body=%s", rec.Code, rec.Body.String())
	}
	sess, err := svc.Get(context.Background(), "typed-nil-coordinator")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.WorkingMemory.DegradedReasons["redis_snapshot"] != "" {
		t.Fatalf("typed nil coordinator should not mark redis degraded: %+v", sess.WorkingMemory.DegradedReasons)
	}
}

func TestBuildInterviewEventHub_DefaultsToMemoryHub(t *testing.T) {
	t.Setenv("INTERVIEW_REDIS_URL", "")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	hub, cleanup, err := buildInterviewEventHub(context.Background(), cfg)
	if err != nil {
		t.Fatalf("build event hub: %v", err)
	}
	if hub == nil {
		t.Fatal("expected event hub")
	}

	ch, unsubscribe, err := hub.Subscribe(context.Background(), "event-hub-test", "")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer unsubscribe()
	cleanup()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("subscriber should close after cleanup")
		}
	default:
		t.Fatal("subscriber should be closed synchronously")
	}

	if _, _, err := hub.Subscribe(context.Background(), "event-hub-test", ""); err == nil {
		t.Fatal("subscribe after cleanup should fail")
	}
}

func TestBuildInterviewEventHub_RedisURLSelectsRedisHub(t *testing.T) {
	t.Setenv("INTERVIEW_REDIS_URL", "redis://localhost:6379/1")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	hub, cleanup, err := buildInterviewEventHub(context.Background(), cfg)
	if err != nil {
		t.Fatalf("build event hub: %v", err)
	}
	defer cleanup()
	if _, ok := hub.(*httpapi.RedisInterviewEventHub); !ok {
		t.Fatalf("hub type = %T, want RedisInterviewEventHub", hub)
	}
}

func TestBuildInterviewEventHub_InvalidRedisURLFails(t *testing.T) {
	t.Setenv("INTERVIEW_REDIS_URL", "http://localhost:6379")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	_, _, err = buildInterviewEventHub(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected invalid redis URL error")
	}
	if os.Getenv("INTERVIEW_REDIS_URL") == "" {
		t.Fatal("test env should be set")
	}
}

func TestBuildRedisSessionCoordinator_RedisURLSelectsCoordinator(t *testing.T) {
	t.Setenv("INTERVIEW_REDIS_URL", "redis://localhost:6379/1")

	coord, err := buildRedisSessionCoordinator(context.Background())
	if err != nil {
		t.Fatalf("build coordinator: %v", err)
	}
	if coord == nil {
		t.Fatal("expected redis session coordinator")
	}
}

func TestBuildRedisSessionCoordinator_NoRedisURLReturnsNil(t *testing.T) {
	t.Setenv("INTERVIEW_REDIS_URL", "")

	coord, err := buildRedisSessionCoordinator(context.Background())
	if err != nil {
		t.Fatalf("build coordinator: %v", err)
	}
	if coord != nil {
		t.Fatalf("coordinator = %T, want nil", coord)
	}
}

func TestBuildChatModel_RealModeWrapsLimiterAndBreaker(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.LLM.Mode = "real"
	cfg.LLMAPIKey = "sk-test"
	cfg.LLM.MaxConcurrency = 2

	model, breakerState, err := buildChatModel(cfg)
	if err != nil {
		t.Fatalf("build chat model: %v", err)
	}
	if _, ok := model.(*llm.BreakingChatModel); !ok {
		t.Fatalf("model type = %T, want BreakingChatModel (outermost)", model)
	}
	if breakerState == nil {
		t.Fatal("expected breakerState fn for real mode")
	}
	if got := breakerState(); got != "closed" {
		t.Fatalf("breakerState() = %q, want closed", got)
	}
}

func TestBuildChatModel_MockModeNoBreaker(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.LLM.Mode = "mock"

	model, breakerState, err := buildChatModel(cfg)
	if err != nil {
		t.Fatalf("build chat model: %v", err)
	}
	if model == nil {
		t.Fatal("expected non-nil model")
	}
	if breakerState != nil {
		t.Fatal("breakerState should be nil for mock mode")
	}
}

func TestBuildAgentService_ProjectPolishUsesMockTool(t *testing.T) {
	service := buildAgentService()
	resp, err := service.HandleMessage(context.Background(), agent.AgentMessage{
		UserID:  "u1",
		Message: "帮我润色项目 https://github.com/acme/interview-agent",
	})
	if err != nil {
		t.Fatalf("handle message: %v", err)
	}
	if !strings.Contains(resp.Result.Content, "interview-agent mock GitHub project analysis") {
		t.Fatalf("content should include mock tool output: %s", resp.Result.Content)
	}
}

func TestBuildUserMemoryOwnerResolverUsesInternalTrialHeader(t *testing.T) {
	cfg := &config.Config{}
	cfg.InternalTrial.Enabled = true
	cfg.InternalTrial.OwnerHeader = "X-Internal-User"

	resolver := buildUserMemoryOwnerResolver(cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/users/u1/memory", nil)
	req.Header.Set("X-Internal-User", "u1")

	owner, err := resolver(req)
	if err != nil {
		t.Fatalf("resolve owner: %v", err)
	}
	if owner.UserID != "u1" || !owner.Authenticated {
		t.Fatalf("owner = %+v, want authenticated u1", owner)
	}
}

func TestBuildUserMemoryOwnerResolverRejectsDevFallbackInInternalTrial(t *testing.T) {
	cfg := &config.Config{}
	cfg.InternalTrial.Enabled = true
	cfg.InternalTrial.OwnerHeader = "X-Internal-User"

	resolver := buildUserMemoryOwnerResolver(cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/users/u1/memory?owner_user_id=u1", nil)
	req.Header.Set("X-User-ID", "u1")

	if _, err := resolver(req); err == nil {
		t.Fatal("resolve owner succeeded with dev fallback in internal trial, want error")
	}
}

func TestBuildUserMemoryOwnerResolverAllowsExplicitDevFallbackInInternalTrial(t *testing.T) {
	cfg := &config.Config{}
	cfg.InternalTrial.Enabled = true
	cfg.InternalTrial.OwnerHeader = "X-Internal-User"
	cfg.InternalTrial.AllowDevFallback = true

	resolver := buildUserMemoryOwnerResolver(cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/users/u1/memory?owner_user_id=u1", nil)

	owner, err := resolver(req)
	if err != nil {
		t.Fatalf("resolve owner: %v", err)
	}
	if owner.UserID != "u1" || !owner.Authenticated {
		t.Fatalf("owner = %+v, want dev fallback owner u1", owner)
	}
}

func TestConfigLoadInternalTrialEnvOverrides(t *testing.T) {
	t.Setenv("INTERVIEW_INTERNAL_TRIAL_ENABLED", "true")
	t.Setenv("INTERVIEW_INTERNAL_TRIAL_OWNER_HEADER", "X-Trial-Owner")
	t.Setenv("INTERVIEW_INTERNAL_TRIAL_ALLOW_DEV_FALLBACK", "true")
	t.Setenv("INTERVIEW_INTERNAL_TRIAL_GITHUB_TOOL_MODE", "real")
	t.Setenv("INTERVIEW_INTERNAL_TRIAL_GITHUB_API_BASE_URL", "https://github.example/api")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.InternalTrial.Enabled {
		t.Fatal("internal trial should be enabled")
	}
	if cfg.InternalTrial.OwnerHeader != "X-Trial-Owner" {
		t.Fatalf("owner header = %q", cfg.InternalTrial.OwnerHeader)
	}
	if !cfg.InternalTrial.AllowDevFallback {
		t.Fatal("allow dev fallback should be true")
	}
	if cfg.InternalTrial.GitHubToolMode != "real" {
		t.Fatalf("github tool mode = %q", cfg.InternalTrial.GitHubToolMode)
	}
	if cfg.InternalTrial.GitHubAPIBaseURL != "https://github.example/api" {
		t.Fatalf("github api base url = %q", cfg.InternalTrial.GitHubAPIBaseURL)
	}
}

func TestConfigLoadRejectsInvalidInternalTrialGitHubToolMode(t *testing.T) {
	t.Setenv("INTERVIEW_INTERNAL_TRIAL_GITHUB_TOOL_MODE", "live")

	if _, err := config.Load(""); err == nil || !strings.Contains(err.Error(), "internal_trial.github_tool_mode") {
		t.Fatalf("load error = %v, want internal trial github tool mode validation", err)
	}
}

func TestConfigLoadRejectsEnabledInternalTrialWithoutOwnerHeader(t *testing.T) {
	t.Setenv("INTERVIEW_INTERNAL_TRIAL_ENABLED", "true")
	t.Setenv("INTERVIEW_INTERNAL_TRIAL_OWNER_HEADER", " ")

	if _, err := config.Load(""); err == nil || !strings.Contains(err.Error(), "internal_trial.owner_header") {
		t.Fatalf("load error = %v, want owner header validation", err)
	}
}

func TestShutdownServerClosesQuestionBankImportService(t *testing.T) {
	imports := questionbank.NewImportService(questionbank.ImportServiceDeps{
		Imports: questionbank.NewMemoryImportStore(),
		Writer:  questionbank.NewMemoryStore(nil),
		Spool:   questionbank.NewLocalImportSpool(t.TempDir()),
	})
	srv := &http.Server{Handler: http.NewServeMux()}

	if err := shutdownServer(context.Background(), srv, imports); err != nil {
		t.Fatalf("shutdownServer: %v", err)
	}
	_, err := imports.EnqueueImport(context.Background(), questionbank.ImportSourceQuestionBank, questionbank.ImportFile{
		Filename: "after-shutdown.json",
		Reader:   strings.NewReader(`[]`),
	})
	if !errors.Is(err, questionbank.ErrImportServiceShutdown) {
		t.Fatalf("EnqueueImport error = %v, want ErrImportServiceShutdown", err)
	}
}
