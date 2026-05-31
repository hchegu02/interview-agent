package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"interview-agent/internal/config"
	"interview-agent/internal/embedding"
	"interview-agent/internal/graph"
	"interview-agent/internal/graphs"
	"interview-agent/internal/httpapi"
	"interview-agent/internal/llm"
	"interview-agent/internal/nodes"
	"interview-agent/internal/observability"
	"interview-agent/internal/retriever"
)

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

func buildInterviewRunner(ctx context.Context, cfg *config.Config, deps appDeps, events httpapi.InterviewEventPublisher, metricsCallback graph.Callback, llmObserver func(llm.CallRecord), retrieverWrapper func(retriever.Retriever, string) retriever.Retriever) (interviewRunnerBundle, func(), error) {
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
	r, cleanup, retrieverKind, err := buildRetriever(ctx, cfg, deps)
	if err != nil {
		return interviewRunnerBundle{}, nil, err
	}
	if retrieverWrapper != nil {
		r = retrieverWrapper(r, retrieverKind)
	}
	callbacks := []graph.Callback{
		httpapi.NewInterviewGraphCallback(events),
		observability.NewTracingGraphCallback(observability.NoopTracer{}),
	}
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

func buildRetriever(ctx context.Context, cfg *config.Config, deps appDeps) (retriever.Retriever, func(), string, error) {
	if deps.PGPool == nil {
		return fallbackRetriever{}, func() {}, "fallback", nil
	}
	return retriever.NewPGVectorRetriever(deps.PGPool, nil), func() {}, "pgvector", nil
}

type fallbackRetriever struct{}

func (fallbackRetriever) Retrieve(ctx context.Context, q retriever.Query) ([]retriever.Result, error) {
	return nil, errors.New("postgres DSN not configured; using fallback question bank")
}
