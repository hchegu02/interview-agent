package config

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoad_DefaultsMockMode(t *testing.T) {
	t.Setenv("INTERVIEW_LLM_API_KEY", "")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.LLM.Mode != "mock" {
		t.Errorf("expected mock mode by default, got %s", cfg.LLM.Mode)
	}
	if cfg.LLM.MaxConcurrency != 4 {
		t.Errorf("expected llm max concurrency default 4, got %d", cfg.LLM.MaxConcurrency)
	}
	if cfg.Embedding.Model != "text-embedding-v4" {
		t.Errorf("expected embedding model text-embedding-v4, got %s", cfg.Embedding.Model)
	}
	if cfg.Embedding.Dimension != 1024 {
		t.Errorf("expected embedding dimension default 1024, got %d", cfg.Embedding.Dimension)
	}
	if cfg.Server.Addr != ":8080" {
		t.Errorf("default addr wrong: %s", cfg.Server.Addr)
	}
	if cfg.Server.MaxStreams != 100 {
		t.Errorf("expected max streams default 100, got %d", cfg.Server.MaxStreams)
	}
}

func TestLoad_RealModeRequiresAPIKey(t *testing.T) {
	t.Setenv("INTERVIEW_LLM_MODE", "real")
	os.Unsetenv("INTERVIEW_LLM_API_KEY")
	_, err := Load("")
	if err == nil {
		t.Fatal("expected error for real mode without key")
	}
}

func TestLoad_RealModeWithKey(t *testing.T) {
	t.Setenv("INTERVIEW_LLM_MODE", "real")
	t.Setenv("INTERVIEW_LLM_API_KEY", "sk-test")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.LLM.Mode != "real" || cfg.LLMAPIKey != "sk-test" {
		t.Errorf("env override not applied: mode=%s key=%s", cfg.LLM.Mode, cfg.LLMAPIKey)
	}
}

func TestValidate_InvalidLLMMaxConcurrencyFails(t *testing.T) {
	cfg := defaults()
	cfg.LLM.MaxConcurrency = 0
	if err := cfg.validate(); err == nil {
		t.Fatal("expected invalid max concurrency error")
	}
}

func TestLoad_EmbeddingEnvOverrides(t *testing.T) {
	t.Setenv("INTERVIEW_EMBEDDING_MODE", "real")
	t.Setenv("INTERVIEW_EMBEDDING_API_KEY", "dummy")
	t.Setenv("INTERVIEW_EMBEDDING_BASE_URL", "http://localhost:8000/v1")
	t.Setenv("INTERVIEW_EMBEDDING_MODEL", "BAAI/bge-m3")
	t.Setenv("INTERVIEW_EMBEDDING_DIMENSION", "1024")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Embedding.BaseURL != "http://localhost:8000/v1" {
		t.Fatalf("base url = %q", cfg.Embedding.BaseURL)
	}
	if cfg.Embedding.Model != "BAAI/bge-m3" {
		t.Fatalf("model = %q", cfg.Embedding.Model)
	}
	if cfg.Embedding.Dimension != 1024 {
		t.Fatalf("dimension = %d", cfg.Embedding.Dimension)
	}
}

func TestLoad_InvalidEmbeddingDimensionFails(t *testing.T) {
	t.Setenv("INTERVIEW_EMBEDDING_DIMENSION", "bad")
	_, err := Load("")
	if err == nil {
		t.Fatal("expected invalid embedding dimension error")
	}
}

// 关键测试：验证序列化 Config 不会泄漏 API key。
// 这是吸取了原项目把 key 写进 interview.json 的教训。
func TestSensitiveFieldsNotInYAML(t *testing.T) {
	t.Setenv("INTERVIEW_LLM_API_KEY", "sk-super-secret")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(b)
	if strings.Contains(out, "sk-super-secret") {
		t.Errorf("yaml marshal leaked secret:\n%s", out)
	}
}
