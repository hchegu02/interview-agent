// Command server 是 interview-agent 的入口。
//
// 阶段 0 提供：
//   - 配置加载
//   - 日志初始化（slog + trace id）
//   - Gin 路由 + /healthz、/readyz、/api/ping
//   - 优雅停机骨架（监听 SIGTERM/SIGINT，drain 30s）
//
// 后续阶段在 wireup 处加入 PG / Redis / Eino Graph runner。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"interview-agent/internal/config"
	"interview-agent/internal/embedding"
	"interview-agent/internal/graph"
	"interview-agent/internal/graphs"
	"interview-agent/internal/httpapi"
	"interview-agent/internal/llm"
	"interview-agent/internal/nodes"
	"interview-agent/internal/observability"
	"interview-agent/internal/parser"
	"interview-agent/internal/questionbank"
	"interview-agent/internal/retriever"
)

func main() {
	cfgPath := flag.String("config", "config/config.yaml", "path to YAML config (optional)")
	flag.Parse()

	logger := observability.NewLogger(os.Stdout, slog.LevelInfo)
	slog.SetDefault(logger)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}
	logger.Info("config loaded",
		"addr", cfg.Server.Addr,
		"llm_mode", cfg.LLM.Mode,
		"embedding_mode", cfg.Embedding.Mode,
		"ratelimit_backend", cfg.RateLimit.Backend,
	)

	ctx := context.Background()
	deps, cleanupDeps, err := buildAppDeps(ctx, cfg)
	if err != nil {
		logger.Error("app deps setup failed", "err", err)
		os.Exit(1)
	}
	defer cleanupDeps()

	events, cleanupEvents, err := buildInterviewEventHub(ctx, cfg)
	if err != nil {
		logger.Error("event hub setup failed", "err", err)
		os.Exit(1)
	}
	defer cleanupEvents()

	sessionCoordinator, err := buildRedisSessionCoordinator(ctx)
	if err != nil {
		logger.Error("session coordinator setup failed", "err", err)
		os.Exit(1)
	}

	server := httpapi.NewServer(cfg)
	profileAnalyzer, err := buildProfileAnalyzer(cfg)
	if err != nil {
		logger.Error("profile analyzer setup failed", "err", err)
		os.Exit(1)
	}
	server.SetProfileAnalyzer(profileAnalyzer)
	questionBankStore, err := buildQuestionBankStore(deps)
	if err != nil {
		logger.Error("question bank setup failed", "err", err)
		os.Exit(1)
	}
	server.SetQuestionBankStore(questionBankStore)
	questionImportService, err := buildQuestionBankImportService(cfg, deps, questionBankStore)
	if err != nil {
		logger.Error("question bank import setup failed", "err", err)
		os.Exit(1)
	}
	server.SetQuestionBankImportService(questionImportService)
	go func() {
		n, err := questionImportService.RecoverPendingJobs(context.Background())
		if err != nil {
			logger.Error("question bank import recovery failed", "err", err)
			return
		}
		if n > 0 {
			logger.Info("question bank import recovery scheduled", "jobs", n)
		}
	}()

	graphRunner, cleanup, err := buildInterviewRunner(ctx, cfg, deps, events, server.GraphMetricsCallback(), server.ObserveLLMCall)
	if err != nil {
		logger.Error("interview graph setup failed", "err", err)
		os.Exit(1)
	}
	defer cleanup()

	interviewService, serviceCleanup, err := buildInterviewService(ctx, cfg, deps, graphRunner.runner, events, sessionCoordinator)
	if err != nil {
		logger.Error("interview service setup failed", "err", err)
		os.Exit(1)
	}
	defer serviceCleanup()

	server.SetInterviewService(interviewService)
	if graphRunner.breakerState != nil {
		server.SetBreakerState(graphRunner.breakerState)
	}
	srv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      server.Router(),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// 启动 HTTP 服务
	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server starting", "addr", cfg.Server.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// 优雅停机：监听 SIGTERM / SIGINT
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err := <-errCh:
		logger.Error("server error", "err", err)
		os.Exit(1)
	case sig := <-stopCh:
		logger.Info("shutdown signal received", "signal", sig.String())
	}

	// drain：停止接受新请求，等 in-flight 请求完成（阶段 7 完善）
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownGrace)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
	}

	logger.Info("server stopped",
		"grace", cfg.Server.ShutdownGrace,
		"ts", time.Now().Format(time.RFC3339))
}

type appDeps struct {
	PGPool *pgxpool.Pool
}

func buildAppDeps(ctx context.Context, cfg *config.Config) (appDeps, func(), error) {
	if cfg.PostgresDSN == "" {
		return appDeps{}, func() {}, nil
	}
	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		return appDeps{}, nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return appDeps{}, nil, fmt.Errorf("ping postgres: %w", err)
	}
	return appDeps{PGPool: pool}, pool.Close, nil
}

func buildInterviewService(ctx context.Context, cfg *config.Config, deps appDeps, runner *graph.Runnable, events httpapi.InterviewEventHub, coordinators ...httpapi.SessionCoordinator) (*httpapi.InterviewService, func(), error) {
	coordinator := firstNonNilCoordinator(coordinators)
	ownerID := hostnameOwnerID()
	if deps.PGPool == nil {
		return httpapi.NewInterviewServiceWithStoreEventsAndCoordinator(runner, httpapi.NewMemorySessionStore(), events, coordinator, ownerID), func() {}, nil
	}
	store := httpapi.NewPGSessionStore(deps.PGPool, 24*time.Hour)
	return httpapi.NewInterviewServiceWithStoreEventsAndCoordinator(runner, store, events, coordinator, ownerID), func() {}, nil
}

func firstNonNilCoordinator(coordinators []httpapi.SessionCoordinator) httpapi.SessionCoordinator {
	for _, coordinator := range coordinators {
		if coordinator == nil {
			continue
		}
		v := reflect.ValueOf(coordinator)
		if v.Kind() == reflect.Pointer && v.IsNil() {
			continue
		}
		return coordinator
	}
	return nil
}

func buildQuestionBankStore(deps appDeps) (questionbank.Store, error) {
	if deps.PGPool != nil {
		return questionbank.NewPGStore(deps.PGPool), nil
	}
	items, err := questionbank.LoadSeedFile("seeds/question_bank.json")
	if err != nil {
		return nil, err
	}
	return questionbank.NewMemoryStore(items), nil
}

func buildQuestionBankImportService(cfg *config.Config, deps appDeps, store questionbank.Store) (*questionbank.ImportService, error) {
	writer, ok := store.(questionbank.Writer)
	if !ok {
		return nil, fmt.Errorf("question bank store does not support writes")
	}
	var importStore questionbank.ImportStore
	if deps.PGPool != nil {
		importStore = questionbank.NewPGImportStore(deps.PGPool)
	} else {
		importStore = questionbank.NewMemoryImportStore()
	}
	model, _, err := buildChatModel(cfg)
	if err != nil {
		return nil, err
	}
	embedder, err := buildEmbedder(cfg)
	if err != nil {
		return nil, err
	}
	return questionbank.NewImportService(questionbank.ImportServiceDeps{
		Imports:  importStore,
		Writer:   writer,
		Parser:   parser.NewDispatcher(),
		Model:    model,
		Embedder: embedder,
		Spool:    questionbank.NewLocalImportSpool(cfg.Server.ImportSpoolDir),
		OwnerID:  hostnameOwnerID(),
	}), nil
}

func buildInterviewEventHub(ctx context.Context, cfg *config.Config) (httpapi.InterviewEventHub, func(), error) {
	if rawURL := os.Getenv("INTERVIEW_REDIS_URL"); rawURL != "" {
		opts, err := httpapi.ParseRedisEventHubOptions(rawURL)
		if err != nil {
			return nil, nil, err
		}
		hub, err := httpapi.NewRedisInterviewEventHub(opts)
		if err != nil {
			return nil, nil, err
		}
		return hub, func() {
			_ = hub.Close()
		}, nil
	}
	hub := httpapi.NewMemoryInterviewEventHub(128)
	return hub, func() {
		_ = hub.Close()
	}, nil
}

func buildRedisSessionCoordinator(ctx context.Context) (*httpapi.RedisSessionCoordinator, error) {
	rawURL := os.Getenv("INTERVIEW_REDIS_URL")
	if rawURL == "" {
		return nil, nil
	}
	opts, err := httpapi.ParseRedisSessionCoordinatorOptions(rawURL)
	if err != nil {
		return nil, err
	}
	return httpapi.NewRedisSessionCoordinator(opts)
}

func hostnameOwnerID() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "local"
	}
	return name
}

func buildInterviewRunner(ctx context.Context, cfg *config.Config, deps appDeps, events httpapi.InterviewEventPublisher, metricsCallback graph.Callback, llmObserver func(llm.CallRecord)) (interviewRunnerBundle, func(), error) {
	model, breakerState, err := buildChatModel(cfg)
	if err != nil {
		return interviewRunnerBundle{}, nil, err
	}
	if llmObserver != nil {
		recordModel := llm.NewRecordingChatModel(model)
		recordModel.SetObserver(llmObserver)
		model = recordModel
	}
	embedder, err := buildEmbedder(cfg)
	if err != nil {
		return interviewRunnerBundle{}, nil, err
	}
	r, cleanup, err := buildRetriever(ctx, cfg, deps)
	if err != nil {
		return interviewRunnerBundle{}, nil, err
	}
	callbacks := []graph.Callback{httpapi.NewInterviewGraphCallback(events)}
	if metricsCallback != nil {
		callbacks = append(callbacks, metricsCallback)
	}
	runner, err := graphs.BuildInterviewGraph(graphs.Deps{
		Model:     model,
		Embedder:  embedder,
		Retriever: r,
		Callbacks: callbacks,
	})
	if err != nil {
		cleanup()
		return interviewRunnerBundle{}, nil, err
	}
	return interviewRunnerBundle{runner: runner, breakerState: breakerState}, cleanup, nil
}

// interviewRunnerBundle 把 graph runner 和可选的熔断器状态查询函数捆在一起返回。
// breakerState 仅在 real 模式下非 nil；mock 模式 /readyz 返回 ready 不带降级位。
type interviewRunnerBundle struct {
	runner       *graph.Runnable
	breakerState func() string
}

// buildChatModel 是 cmd/server 包内对 llm.BuildChatModel 的轻量 wrapper。
// 复用同一份装配逻辑——把它放到 internal/llm/factory.go 后，cmd/demo 也能直接调。
//
// real 模式链路（外→内）：BreakingChatModel → LimitedChatModel → RealChatModel。
// 熔断器放最外层的原因：open 时要直接 fail-fast，不能先去抢 limiter 槽位。
// 第二个返回值 breakerState 是熔断器状态查询函数，给 /readyz 用；mock 模式为 nil。
func buildChatModel(cfg *config.Config) (llm.ChatModel, func() string, error) {
	return llm.BuildChatModel(cfg)
}

func buildProfileAnalyzer(cfg *config.Config) (*httpapi.NodeProfileAnalyzer, error) {
	model, _, err := buildChatModel(cfg)
	if err != nil {
		return nil, err
	}
	return httpapi.NewNodeProfileAnalyzer(
		nodes.NewParseJDNode(model),
		nodes.NewParseResumeNode(model),
		nodes.NewGapAnalyzeNode(model),
		nodes.NewAnalyzeProfileNode(),
	), nil
}

func buildEmbedder(cfg *config.Config) (embedding.Embedder, error) {
	switch cfg.Embedding.Mode {
	case "mock":
		return embedding.NewMockEmbedder(cfg.Embedding.Dimension), nil
	case "real":
		return embedding.NewRealEmbedder(
			cfg.Embedding.BaseURL,
			cfg.EmbeddingAPIKey,
			cfg.Embedding.Model,
			cfg.Embedding.Dimension,
			cfg.Embedding.Timeout,
		), nil
	default:
		return nil, fmt.Errorf("unsupported embedding mode %q", cfg.Embedding.Mode)
	}
}

func buildRetriever(ctx context.Context, cfg *config.Config, deps appDeps) (retriever.Retriever, func(), error) {
	if deps.PGPool == nil {
		return fallbackRetriever{}, func() {}, nil
	}
	return retriever.NewPGVectorRetriever(deps.PGPool, nil), func() {}, nil
}

type fallbackRetriever struct{}

func (fallbackRetriever) Retrieve(ctx context.Context, q retriever.Query) ([]retriever.Result, error) {
	return nil, errors.New("postgres DSN not configured; using fallback question bank")
}
