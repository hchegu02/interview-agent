package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"interview-agent/internal/config"
	"interview-agent/internal/embedding"
)

// TestRun_MockMode 把 demo 的 main 流程跑一遍。
// 目的：验证 wiring 正确，run.json / report.md 落盘格式可读。
// 不强校验报告内容（mock LLM 可能给 end，跑 0 轮也算成功），只断言基础结构。
func TestRun_MockMode(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "testdata", "demo", "example.yaml")
	cfgPath := filepath.Join("..", "..", "config", "config.yaml.example")
	outDir := t.TempDir()

	var stdout, stderr bytes.Buffer
	// 显式设置 mock 模式，避免环境变量泄漏到测试。
	t.Setenv("INTERVIEW_LLM_MODE", "mock")
	t.Setenv("INTERVIEW_EMBEDDING_MODE", "mock")
	t.Setenv("INTERVIEW_LLM_API_KEY", "")
	t.Setenv("INTERVIEW_EMBEDDING_API_KEY", "")
	t.Setenv("INTERVIEW_POSTGRES_DSN", "")

	exit := run(cfgPath, scriptPath, outDir, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("run exit=%d; stderr=%s", exit, stderr.String())
	}

	// 1. run.json 存在且可解码
	runPath := filepath.Join(outDir, "run.json")
	raw, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatalf("read run.json: %v", err)
	}
	var art RunArtifact
	if err := json.Unmarshal(raw, &art); err != nil {
		t.Fatalf("decode run.json: %v", err)
	}

	// 2. 基础结构断言
	if art.Config.LLMMode != "mock" {
		t.Errorf("Config.LLMMode = %q, want mock", art.Config.LLMMode)
	}
	if art.Config.Retriever != "fallback" {
		t.Errorf("Config.Retriever = %q, want fallback", art.Config.Retriever)
	}
	if art.Config.PostgresConfigured {
		t.Errorf("Config.PostgresConfigured = true, want false")
	}
	if len(art.LLMCalls) == 0 {
		t.Errorf("LLMCalls is empty, want >0")
	}
	if len(art.Nodes) == 0 {
		t.Errorf("Nodes is empty, want >0")
	}
	if art.Session == nil {
		t.Fatalf("Session is nil")
	}
	if art.FatalError != "" {
		t.Errorf("FatalError = %q, want empty", art.FatalError)
	}

	// 3. report.md 存在且非空
	reportPath := filepath.Join(outDir, "report.md")
	reportRaw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report.md: %v", err)
	}
	rstr := string(reportRaw)
	if !strings.Contains(rstr, "# Interview Agent Demo Run") {
		t.Errorf("report.md missing header; got %q", rstr[:min(120, len(rstr))])
	}
	if !strings.Contains(rstr, "## LLM call stats") {
		t.Errorf("report.md missing LLM call stats section")
	}
}

func TestBuildDemoRetriever_NoPostgresUsesFallback(t *testing.T) {
	cfg := &config.Config{}

	r, cleanup, kind, err := buildDemoRetriever(context.Background(), cfg)
	if err != nil {
		t.Fatalf("buildDemoRetriever: %v", err)
	}
	defer cleanup()
	if kind != "fallback" {
		t.Fatalf("kind = %q, want fallback", kind)
	}
	if _, ok := r.(fallbackRetriever); !ok {
		t.Fatalf("retriever = %T, want fallbackRetriever", r)
	}
}

func TestBuildDemoEmbedder_RealUsesConfig(t *testing.T) {
	cfg := &config.Config{
		EmbeddingAPIKey: "dummy",
		Embedding: config.EmbeddingConfig{
			Mode:      "real",
			BaseURL:   "http://127.0.0.1:8000/v1",
			Model:     "BAAI/bge-m3",
			Dimension: 1024,
		},
	}

	got, err := buildDemoEmbedder(cfg)
	if err != nil {
		t.Fatalf("buildDemoEmbedder: %v", err)
	}
	real, ok := got.(*embedding.RealEmbedder)
	if !ok {
		t.Fatalf("embedder = %T, want *embedding.RealEmbedder", got)
	}
	if real.BaseURL != "http://127.0.0.1:8000/v1" || real.Model != "BAAI/bge-m3" || real.Dimension() != 1024 {
		t.Fatalf("unexpected real embedder config: base=%q model=%q dim=%d", real.BaseURL, real.Model, real.Dimension())
	}
}

func TestBuildRunConfig_RecordsRetriever(t *testing.T) {
	cfg := &config.Config{
		PostgresDSN: "postgres://example",
		LLM:         config.LLMConfig{Mode: "real", Model: "qwen-plus"},
		Embedding:   config.EmbeddingConfig{Mode: "real", Model: "BAAI/bge-m3"},
	}

	got := buildRunConfig(cfg, "cfg.yaml", "script.yaml", "out", "pgvector")
	if got.Retriever != "pgvector" {
		t.Fatalf("Retriever = %q, want pgvector", got.Retriever)
	}
	if !got.PostgresConfigured {
		t.Fatalf("PostgresConfigured = false, want true")
	}
	if got.EmbeddingModel != "BAAI/bge-m3" {
		t.Fatalf("EmbeddingModel = %q", got.EmbeddingModel)
	}
}

// TestLoadScript_ValidatesRequired 确保 LoadScript 对必填字段报错。
func TestLoadScript_ValidatesRequired(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"empty_jd", "job_profile:\n  jd_text: \"\"\ncandidate:\n  resume_text: x\nanswers:\n  - y\n", "jd_text is empty"},
		{"empty_resume", "job_profile:\n  jd_text: x\ncandidate:\n  resume_text: \"\"\nanswers:\n  - y\n", "resume_text is empty"},
		{"empty_answers", "job_profile:\n  jd_text: x\ncandidate:\n  resume_text: y\n", "answers is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "s.yaml")
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := LoadScript(path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("LoadScript err = %v, want contains %q", err, tc.want)
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
