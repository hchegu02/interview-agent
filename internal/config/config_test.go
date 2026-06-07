package config

import (
	"os"
	"strings"
	"testing"
	"time"

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
	if cfg.Rerank.Mode != "lexical" {
		t.Errorf("expected rerank mode lexical by default, got %s", cfg.Rerank.Mode)
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

func TestLoad_YAMLAPIKeysWhenEnvUnset(t *testing.T) {
	t.Setenv("INTERVIEW_LLM_MODE", "")
	t.Setenv("INTERVIEW_LLM_API_KEY", "")
	t.Setenv("INTERVIEW_EMBEDDING_MODE", "")
	t.Setenv("INTERVIEW_EMBEDDING_API_KEY", "")

	path := t.TempDir() + "/config.yaml"
	raw := []byte(`
llm:
  mode: real
  api_key: "sk-yaml"
embedding:
  mode: real
  api_key: "embedding-yaml"
`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.LLMAPIKey != "sk-yaml" {
		t.Fatalf("llm api key = %q, want yaml key", cfg.LLMAPIKey)
	}
	if cfg.EmbeddingAPIKey != "embedding-yaml" {
		t.Fatalf("embedding api key = %q, want yaml key", cfg.EmbeddingAPIKey)
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

func TestLoad_RerankEnvOverrides(t *testing.T) {
	t.Setenv("INTERVIEW_RERANK_MODE", "http")
	t.Setenv("INTERVIEW_RERANK_ENDPOINT", "http://127.0.0.1:9000/rerank")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Rerank.Mode != "http" {
		t.Fatalf("rerank mode = %q", cfg.Rerank.Mode)
	}
	if cfg.Rerank.Endpoint != "http://127.0.0.1:9000/rerank" {
		t.Fatalf("rerank endpoint = %q", cfg.Rerank.Endpoint)
	}
}

func TestValidate_HTTPRerankRequiresEndpoint(t *testing.T) {
	cfg := defaults()
	cfg.Rerank.Mode = "http"
	cfg.Rerank.Endpoint = ""
	if err := cfg.validate(); err == nil {
		t.Fatal("expected http rerank without endpoint to fail")
	}
}

func TestValidate_InvalidPostgresPoolConfigFails(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{
			name: "max conns zero",
			edit: func(cfg *Config) {
				cfg.Postgres.MaxConns = 0
			},
			want: "postgres.max_conns",
		},
		{
			name: "min conns negative",
			edit: func(cfg *Config) {
				cfg.Postgres.MinConns = -1
			},
			want: "postgres.min_conns",
		},
		{
			name: "min greater than max",
			edit: func(cfg *Config) {
				cfg.Postgres.MaxConns = 2
				cfg.Postgres.MinConns = 3
			},
			want: "postgres.min_conns",
		},
		{
			name: "lifetime zero",
			edit: func(cfg *Config) {
				cfg.Postgres.MaxConnLifetime = 0
			},
			want: "postgres.max_conn_lifetime",
		},
		{
			name: "health check zero",
			edit: func(cfg *Config) {
				cfg.Postgres.HealthCheckPeriod = 0
			},
			want: "postgres.health_check_period",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaults()
			tt.edit(cfg)
			err := cfg.validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validate error = %v, want %q", err, tt.want)
			}
		})
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

func TestPostgresPoolConfigAppliesConfiguredPoolSettings(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cfg.PostgresDSN = "postgres://user:pass@localhost:5432/interview_agent?sslmode=disable"
	cfg.Postgres.MaxConns = 17
	cfg.Postgres.MinConns = 3
	cfg.Postgres.MaxConnLifetime = 11 * time.Minute
	cfg.Postgres.HealthCheckPeriod = 13 * time.Second

	poolCfg, err := PostgresPoolConfig(cfg)
	if err != nil {
		t.Fatalf("PostgresPoolConfig: %v", err)
	}
	if poolCfg.ConnConfig.Config.Host != "localhost" {
		t.Fatalf("host = %q, want localhost", poolCfg.ConnConfig.Config.Host)
	}
	if poolCfg.MaxConns != 17 {
		t.Fatalf("max conns = %d, want 17", poolCfg.MaxConns)
	}
	if poolCfg.MinConns != 3 {
		t.Fatalf("min conns = %d, want 3", poolCfg.MinConns)
	}
	if poolCfg.MaxConnLifetime != 11*time.Minute {
		t.Fatalf("max conn lifetime = %v, want 11m", poolCfg.MaxConnLifetime)
	}
	if poolCfg.HealthCheckPeriod != 13*time.Second {
		t.Fatalf("health check period = %v, want 13s", poolCfg.HealthCheckPeriod)
	}
}
