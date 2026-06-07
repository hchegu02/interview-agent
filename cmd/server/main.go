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
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"interview-agent/internal/agent"
	"interview-agent/internal/agentkit"
	"interview-agent/internal/config"
	"interview-agent/internal/httpapi"
	"interview-agent/internal/observability"
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
	// 基础依赖只负责“外部资源”：Postgres、Redis 等。后面的业务服务只拿组合后的 deps，
	// 避免在 main 里到处散落连接池和清理逻辑。
	deps, cleanupDeps, err := buildAppDeps(ctx, cfg)
	if err != nil {
		logger.Error("app deps setup failed", "err", err)
		os.Exit(1)
	}
	defer cleanupDeps()

	// EventHub 是面试过程的事件出口：SSE、metrics 和后续审计都从这里拿节点状态。
	events, cleanupEvents, err := buildInterviewEventHub(ctx, cfg)
	if err != nil {
		logger.Error("event hub setup failed", "err", err)
		os.Exit(1)
	}
	defer cleanupEvents()

	// Redis coordinator 只解决“同一个 session 被多个进程同时推进”的问题；
	// 没有 Redis 时会退化成本进程内执行，接口层仍然保持同一套服务调用。
	sessionCoordinator, err := buildRedisSessionCoordinator(ctx)
	if err != nil {
		logger.Error("session coordinator setup failed", "err", err)
		os.Exit(1)
	}

	server := httpapi.NewServer(cfg)
	server.SetEventHubMetrics(eventHubMetricsProvider(events))
	server.SetAgentService(buildAgentService(cfg))
	server.SetUserMemoryOwnerResolver(buildUserMemoryOwnerResolver(cfg))

	// ProfileAnalyzer 是“开始面试前的 JD/简历解释”能力。
	// 它和真正的面试 Graph 分开装配，避免用户只点分析时也创建 session。
	profileAnalyzer, err := buildProfileAnalyzer(cfg)
	if err != nil {
		logger.Error("profile analyzer setup failed", "err", err)
		os.Exit(1)
	}
	server.SetProfileAnalyzer(profileAnalyzer)

	// 题库 Store 先接到 HTTP 查询/导入，再通过 retriever 接入面试 Graph；
	// 这两个入口共享同一份题库数据，但职责不同：一个面向管理/预览，一个面向 RAG 检索。
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
	questionGenerationService, err := buildQuestionBankGenerationService(cfg, questionImportService, questionBankStore)
	if err != nil {
		logger.Error("question bank generation setup failed", "err", err)
		os.Exit(1)
	}
	server.SetQuestionBankGenerationService(questionGenerationService)
	// 导入任务可能在服务重启时停在 queued/committing 状态，这里异步恢复，避免启动路径被长任务阻塞。
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

	graphRunner, cleanup, err := buildInterviewRunner(ctx, cfg, deps, events, server.GraphMetricsCallback(), server.ObserveLLMCall, server.WrapRetriever)
	if err != nil {
		logger.Error("interview graph setup failed", "err", err)
		os.Exit(1)
	}
	defer cleanup()

	// InterviewService 是 HTTP handler 和 Graph runner 之间的业务门面。
	// handler 不直接调用 runner，也不直接写 store；所有会话状态变更都经过这里。
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
	if err := shutdownServer(ctx, srv, questionImportService); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
	}

	logger.Info("server stopped",
		"grace", cfg.Server.ShutdownGrace,
		"ts", time.Now().Format(time.RFC3339))
}

func shutdownServer(ctx context.Context, srv *http.Server, questionImports interface {
	Shutdown(context.Context) error
}) error {
	var shutdownErr error
	shutdownErr = errors.Join(shutdownErr, srv.Shutdown(ctx))
	if questionImports != nil {
		shutdownErr = errors.Join(shutdownErr, questionImports.Shutdown(ctx))
	}
	return shutdownErr
}

func buildAgentService(cfg *config.Config) *agent.Service {
	if cfg == nil || !cfg.InternalTrial.Enabled || cfg.InternalTrial.GitHubToolMode != "real" {
		return agent.NewDefaultService()
	}
	tools := agentkit.NewToolRegistry(agentkit.NoopHook{})
	if err := agentkit.RegisterMockWebFetchTool(tools, agentkit.NewMockMCPClient()); err != nil {
		return agent.NewDefaultService()
	}
	if err := agentkit.RegisterGitHubProjectTool(tools, agentkit.GitHubProjectClient{
		HTTPClient: http.DefaultClient,
		BaseURL:    cfg.InternalTrial.GitHubAPIBaseURL,
	}); err != nil {
		return agent.NewDefaultService()
	}
	return agent.NewDefaultServiceWithTools(tools)
}

func buildUserMemoryOwnerResolver(cfg *config.Config) httpapi.UserMemoryOwnerResolver {
	if cfg != nil && cfg.InternalTrial.Enabled {
		return httpapi.NewUserMemoryOwnerResolver(httpapi.UserMemoryOwnerResolverOptions{
			TrustedHeader:    cfg.InternalTrial.OwnerHeader,
			AllowDevFallback: cfg.InternalTrial.AllowDevFallback,
		})
	}
	return httpapi.NewUserMemoryOwnerResolver(httpapi.UserMemoryOwnerResolverOptions{
		AllowDevFallback: true,
	})
}
