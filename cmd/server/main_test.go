package main

import (
	"context"
	"os"
	"testing"

	"interview-agent/internal/config"
	"interview-agent/internal/domain"
	"interview-agent/internal/httpapi"
	"interview-agent/internal/llm"
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

	runner, cleanup, err := buildInterviewRunner(context.Background(), cfg, deps, events)
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
	runner, cleanup, err := buildInterviewRunner(context.Background(), cfg, deps, events)
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
