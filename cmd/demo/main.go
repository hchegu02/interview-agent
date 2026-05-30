// Command demo 是一个 standalone 的端到端 CLI——按 YAML 脚本驱动 graph 跑完
// 一次面试，把 LLM 调用 / 节点 timeline / 最终报告落到 {out}/run.json + report.md。
//
// 用法：
//
//	go run ./cmd/demo -config config/config.yaml.example -script testdata/demo/example.yaml
//
// 不启 HTTP，不依赖 Redis：
//   - 设置 INTERVIEW_POSTGRES_DSN 时 retriever 走 PGVectorRetriever
//   - 未设置 INTERVIEW_POSTGRES_DSN 时 retriever 走 fallback
//   - embedding 按 config 构造；真实 pgvector demo 要和 reindex 使用同一 embedding 模型
//   - session 状态保存在内存里，跑完即丢
//
// 操作者跑真实 LLM：用 `make demo-real`，前置校验 INTERVIEW_LLM_API_KEY。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"interview-agent/internal/config"
	"interview-agent/internal/domain"
	"interview-agent/internal/embedding"
	"interview-agent/internal/graph"
	"interview-agent/internal/graphs"
	"interview-agent/internal/llm"
	"interview-agent/internal/observability"
	"interview-agent/internal/retriever"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfgPath := flag.String("config", "config/config.yaml.example", "path to YAML config")
	scriptPath := flag.String("script", "", "path to demo script YAML (required)")
	outDir := flag.String("out", "", "output directory (default tmp/demos/{timestamp})")
	flag.Parse()

	if *scriptPath == "" {
		fmt.Fprintln(os.Stderr, "ERROR: -script is required")
		flag.Usage()
		os.Exit(2)
	}

	out := *outDir
	if out == "" {
		out = filepath.Join("tmp", "demos", time.Now().Format("20060102-150405"))
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: mkdir %s: %v\n", out, err)
		os.Exit(1)
	}

	exitCode := run(*cfgPath, *scriptPath, out, os.Stdout, os.Stderr)
	os.Exit(exitCode)
}

// run 是可单测的主流程——所有 IO 通过参数注入，main 只负责 flag + os.Exit。
//
// 返回值：0 = success（report 生成），1 = fatal error（仍会写 partial run.json）。
func run(cfgPath, scriptPath, outDir string, stdout, stderr io.Writer) int {
	// 1. 装一个能统计 schema_retry 计数的 slog handler，再把它设为 default。
	//    schema.go 在每次 validation 失败时打 event=llm_schema_validation_failed，
	//    我们在 handler 里 atomic.AddInt64 就行——比 grep 干净。
	schemaRetries := new(atomic.Int64)
	baseLogger := observability.NewLogger(stderr, slog.LevelInfo)
	logger := slog.New(&schemaRetryCounter{
		inner:   baseLogger.Handler(),
		counter: schemaRetries,
	})
	slog.SetDefault(logger)

	startedAt := time.Now()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		writeFatal(outDir, startedAt, nil, nil, nil, "", "", schemaRetries.Load(), fmt.Sprintf("config load: %v", err), cfgPath, scriptPath)
		fmt.Fprintf(stderr, "ERROR: config load: %v\n", err)
		return 1
	}

	script, err := LoadScript(scriptPath)
	if err != nil {
		writeFatal(outDir, startedAt, cfg, nil, nil, "", "", schemaRetries.Load(), fmt.Sprintf("script load: %v", err), cfgPath, scriptPath)
		fmt.Fprintf(stderr, "ERROR: script load: %v\n", err)
		return 1
	}

	// 2. 构造 LLM 链路 → 最外层包 RecordingChatModel。
	//    breakerState 仅 real 模式非 nil。
	innerModel, breakerState, err := llm.BuildChatModel(cfg)
	if err != nil {
		writeFatal(outDir, startedAt, cfg, nil, nil, "", "", schemaRetries.Load(), fmt.Sprintf("build chat model: %v", err), cfgPath, scriptPath)
		fmt.Fprintf(stderr, "ERROR: build chat model: %v\n", err)
		return 1
	}
	recordModel := llm.NewRecordingChatModel(innerModel)

	// 3. 构造 embedder + retriever。
	//    真实 pgvector demo 必须让查询 embedding 和 reindex embedding 来自同一模型。
	embedder, err := buildDemoEmbedder(cfg)
	if err != nil {
		writeFatal(outDir, startedAt, cfg, nil, nil, "", "", schemaRetries.Load(), fmt.Sprintf("build embedder: %v", err), cfgPath, scriptPath)
		fmt.Fprintf(stderr, "ERROR: build embedder: %v\n", err)
		return 1
	}
	r, cleanupRetriever, retrieverKind, err := buildDemoRetriever(context.Background(), cfg)
	if err != nil {
		writeFatal(outDir, startedAt, cfg, nil, nil, "", retrieverKind, schemaRetries.Load(), fmt.Sprintf("build retriever: %v", err), cfgPath, scriptPath)
		fmt.Fprintf(stderr, "ERROR: build retriever: %v\n", err)
		return 1
	}
	defer cleanupRetriever()

	// 4. 装 RecordingCallback；不用 InterviewGraphCallback（demo 不需要 SSE）。
	cb := observability.NewRecordingCallback()

	runner, err := graphs.BuildInterviewGraph(graphs.Deps{
		Model:     recordModel,
		Embedder:  embedder,
		Retriever: r,
		Callbacks: []graph.Callback{cb},
	})
	if err != nil {
		writeFatal(outDir, startedAt, cfg, nil, nil, "", retrieverKind, schemaRetries.Load(), fmt.Sprintf("build interview graph: %v", err), cfgPath, scriptPath)
		fmt.Fprintf(stderr, "ERROR: build interview graph: %v\n", err)
		return 1
	}

	// 5. 构造 Session 并跑首轮。
	ctx := context.Background()
	now := time.Now()
	sess := &domain.Session{
		ID:        "demo-" + now.Format("150405"),
		UserID:    "demo-user",
		Status:    domain.StatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
		JobProfile: &domain.JobProfile{
			JDRawText: script.JobProfile.JDText,
		},
		CandProfile: &domain.CandidateProfile{
			ResumeRawText: script.Candidate.ResumeText,
		},
		WorkingMemory: domain.NewWorkingMemory(),
	}

	fmt.Fprintf(stdout, "demo: starting session %s\n", sess.ID)
	if err := runner.Invoke(ctx, sess); err != nil {
		writeFatal(outDir, startedAt, cfg, recordModel.Snapshot(), cb.Snapshot(), breakerStateValue(breakerState), retrieverKind, schemaRetries.Load(), fmt.Sprintf("invoke: %v", err), cfgPath, scriptPath)
		fmt.Fprintf(stderr, "ERROR: invoke: %v\n", err)
		return 1
	}

	// 6. fill-answer 循环：直到 session completed 或 answers 耗尽。
	answerIdx := 0
	for sess.Status != domain.StatusCompleted && sess.Status != domain.StatusFailed {
		if sess.CurrentNode == "" {
			// graph 已退出但 status 没 finalize——不应该发生，但兜底。
			fmt.Fprintln(stderr, "WARN: graph idle but session not completed")
			break
		}
		if answerIdx >= len(script.Answers) {
			fmt.Fprintf(stderr, "ERROR: script answers exhausted at node %q (rounds=%d)\n", sess.CurrentNode, len(sess.Rounds))
			endedAt := time.Now()
			writeArtifacts(outDir, startedAt, endedAt, cfg, sess, recordModel.Snapshot(), cb.Snapshot(), breakerStateValue(breakerState), retrieverKind, schemaRetries.Load(), "answers exhausted", cfgPath, scriptPath)
			return 1
		}
		ans := script.Answers[answerIdx]
		answerIdx++
		if err := fillPendingAnswer(sess, ans); err != nil {
			writeFatal(outDir, startedAt, cfg, recordModel.Snapshot(), cb.Snapshot(), breakerStateValue(breakerState), retrieverKind, schemaRetries.Load(), fmt.Sprintf("fill answer #%d: %v", answerIdx, err), cfgPath, scriptPath)
			fmt.Fprintf(stderr, "ERROR: fill answer: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "demo: filled answer #%d at node=%s (round=%d)\n", answerIdx, sess.CurrentNode, len(sess.Rounds))
		if err := runner.Resume(ctx, sess); err != nil {
			writeFatal(outDir, startedAt, cfg, recordModel.Snapshot(), cb.Snapshot(), breakerStateValue(breakerState), retrieverKind, schemaRetries.Load(), fmt.Sprintf("resume #%d: %v", answerIdx, err), cfgPath, scriptPath)
			fmt.Fprintf(stderr, "ERROR: resume: %v\n", err)
			return 1
		}
	}

	endedAt := time.Now()
	writeArtifacts(outDir, startedAt, endedAt, cfg, sess, recordModel.Snapshot(), cb.Snapshot(), breakerStateValue(breakerState), retrieverKind, schemaRetries.Load(), "", cfgPath, scriptPath)

	if sess.Report != nil {
		fmt.Fprintf(stdout, "demo: completed, report overall_score=%d, rounds=%d, llm_calls=%d, schema_retries=%d\n",
			sess.Report.OverallScore, len(sess.Rounds), len(recordModel.Snapshot()), schemaRetries.Load())
	} else {
		fmt.Fprintf(stdout, "demo: finished without report, status=%s\n", sess.Status)
	}
	fmt.Fprintf(stdout, "demo: artifacts in %s\n", outDir)
	return 0
}

func writeArtifacts(outDir string, startedAt, endedAt time.Time, cfg *config.Config, sess *domain.Session, llmCalls []llm.CallRecord, nodes []observability.NodeRecord, breakerStateFinal, retrieverKind string, schemaRetries int64, fatalErr string, cfgPath, scriptPath string) {
	art := &RunArtifact{
		StartedAt:         startedAt,
		EndedAt:           endedAt,
		Config:            buildRunConfig(cfg, cfgPath, scriptPath, outDir, retrieverKind),
		Session:           sess,
		LLMCalls:          llmCalls,
		Nodes:             nodes,
		BreakerStateFinal: breakerStateFinal,
		Summary:           BuildSummary(startedAt, endedAt, llmCalls, sess, int(schemaRetries)),
		FatalError:        fatalErr,
	}
	if err := WriteRunArtifact(outDir, art); err != nil {
		slog.Error("write run.json failed", "err", err)
	}
	if err := WriteReportMarkdown(outDir, art); err != nil {
		slog.Error("write report.md failed", "err", err)
	}
}

// writeFatal 是 writeArtifacts 的早返版本：cfg / sess 等可能为 nil，
// 仍尽量把已采集的数据落盘，方便操作者排查。
func writeFatal(outDir string, startedAt time.Time, cfg *config.Config, llmCalls []llm.CallRecord, nodes []observability.NodeRecord, breakerStateFinal, retrieverKind string, schemaRetries int64, fatalErr string, cfgPath, scriptPath string) {
	writeArtifacts(outDir, startedAt, time.Now(), cfg, nil, llmCalls, nodes, breakerStateFinal, retrieverKind, schemaRetries, fatalErr, cfgPath, scriptPath)
}

func buildRunConfig(cfg *config.Config, cfgPath, scriptPath, outDir, retrieverKind string) RunConfig {
	rc := RunConfig{
		ScriptPath: scriptPath,
		OutputDir:  outDir,
		Retriever:  retrieverKind,
	}
	if cfg != nil {
		rc.LLMMode = cfg.LLM.Mode
		rc.LLMModel = cfg.LLM.Model
		rc.EmbeddingMode = cfg.Embedding.Mode
		rc.EmbeddingModel = cfg.Embedding.Model
		rc.PostgresConfigured = cfg.PostgresDSN != ""
	}
	return rc
}

func buildDemoEmbedder(cfg *config.Config) (embedding.Embedder, error) {
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

func buildDemoRetriever(ctx context.Context, cfg *config.Config) (retriever.Retriever, func(), string, error) {
	if cfg.PostgresDSN == "" {
		return fallbackRetriever{}, func() {}, "fallback", nil
	}
	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		return nil, func() {}, "pgvector", fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, func() {}, "pgvector", fmt.Errorf("ping postgres: %w", err)
	}
	return retriever.NewPGVectorRetriever(pool, nil), pool.Close, "pgvector", nil
}

func breakerStateValue(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}

// fallbackRetriever 与 cmd/server 同名类型功能等价——返回错误让
// retrieve_rag 节点降级到静态题库。复制一份避免 cmd/demo 反向依赖
// cmd/server 包（不能 import）。
type fallbackRetriever struct{}

func (fallbackRetriever) Retrieve(ctx context.Context, q retriever.Query) ([]retriever.Result, error) {
	return nil, errors.New("postgres DSN not configured; using fallback question bank")
}

// schemaRetryCounter 包装一个 slog.Handler，对 event=llm_schema_validation_failed
// 的 record 做 atomic 计数。其他 record 直接透传给 inner handler。
//
// 设计：用 handler-level 拦截比 logger middleware 简单——schema.go 已经发了
// 结构化日志，CLI 不需要再改代码，也不需要 grep run.log 反推次数。
type schemaRetryCounter struct {
	inner   slog.Handler
	counter *atomic.Int64
}

func (h *schemaRetryCounter) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *schemaRetryCounter) Handle(ctx context.Context, r slog.Record) error {
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "event" && a.Value.String() == "llm_schema_validation_failed" {
			h.counter.Add(1)
			return false
		}
		return true
	})
	return h.inner.Handle(ctx, r)
}

func (h *schemaRetryCounter) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &schemaRetryCounter{inner: h.inner.WithAttrs(attrs), counter: h.counter}
}

func (h *schemaRetryCounter) WithGroup(name string) slog.Handler {
	return &schemaRetryCounter{inner: h.inner.WithGroup(name), counter: h.counter}
}
