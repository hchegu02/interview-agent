package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"
)

// Config 是服务的顶层配置。
// 设计原则：敏感字段运行时不参与 Config YAML 序列化，避免落盘泄漏。
// API key 支持 env > YAML，DB 密码/连接信息仍只从环境变量注入。
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Postgres  PostgresConfig  `yaml:"postgres"`
	Redis     RedisConfig     `yaml:"redis"`
	LLM       LLMConfig       `yaml:"llm"`
	Embedding EmbeddingConfig `yaml:"embedding"`
	Rerank    RerankConfig    `yaml:"rerank"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`

	InternalTrial InternalTrialConfig `yaml:"internal_trial"`

	// 敏感字段：yaml:"-" 防止序列化时泄漏；Load 会按规则单独注入。
	PostgresDSN     string `yaml:"-"`
	RedisAddr       string `yaml:"-"`
	RedisPassword   string `yaml:"-"`
	LLMAPIKey       string `yaml:"-"`
	EmbeddingAPIKey string `yaml:"-"`
}

type ServerConfig struct {
	Addr           string        `yaml:"addr"`
	ReadTimeout    time.Duration `yaml:"read_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout"`
	ShutdownGrace  time.Duration `yaml:"shutdown_grace"`
	MaxInFlight    int           `yaml:"max_in_flight"` // 背压阈值
	MaxStreams     int           `yaml:"max_streams"`   // SSE 长连接背压阈值
	ImportSpoolDir string        `yaml:"import_spool_dir"`
}

type PostgresConfig struct {
	MaxConns          int32         `yaml:"max_conns"`
	MinConns          int32         `yaml:"min_conns"`
	MaxConnLifetime   time.Duration `yaml:"max_conn_lifetime"`
	HealthCheckPeriod time.Duration `yaml:"health_check_period"`
}

type RedisConfig struct {
	DB           int           `yaml:"db"`
	PoolSize     int           `yaml:"pool_size"`
	MinIdleConns int           `yaml:"min_idle_conns"`
	DialTimeout  time.Duration `yaml:"dial_timeout"`
	KeyPrefix    string        `yaml:"key_prefix"`
}

type LLMConfig struct {
	Mode           string        `yaml:"mode"` // mock | real
	BaseURL        string        `yaml:"base_url"`
	Model          string        `yaml:"model"`
	Fallback       string        `yaml:"fallback_model"`
	Timeout        time.Duration `yaml:"timeout"`
	MaxTokens      int           `yaml:"max_tokens"`
	MaxRetries     int           `yaml:"max_retries"`
	MaxConcurrency int           `yaml:"max_concurrency"`

	// 熔断器（仅 real 模式生效）。连续 BreakerFailureThreshold 次 ErrTransient /
	// DeadlineExceeded 失败后熔断器打开，open 持续 BreakerOpenDuration 后进入半开，
	// 单次 probe 成功才回到 closed。
	BreakerFailureThreshold int           `yaml:"breaker_failure_threshold"`
	BreakerOpenDuration     time.Duration `yaml:"breaker_open_duration"`
}

type EmbeddingConfig struct {
	Mode      string        `yaml:"mode"`
	BaseURL   string        `yaml:"base_url"`
	Model     string        `yaml:"model"`
	Dimension int           `yaml:"dimension"`
	Timeout   time.Duration `yaml:"timeout"`
}

type RerankConfig struct {
	Mode     string        `yaml:"mode"` // lexical | http
	Endpoint string        `yaml:"endpoint"`
	Timeout  time.Duration `yaml:"timeout"`
}

type RateLimitConfig struct {
	Backend      string  `yaml:"backend"` // local | redis_lua
	PerUserQPS   float64 `yaml:"per_user_qps"`
	PerUserBurst int     `yaml:"per_user_burst"`
	GlobalQPS    float64 `yaml:"global_qps"`
	GlobalBurst  int     `yaml:"global_burst"`
}

type InternalTrialConfig struct {
	Enabled          bool   `yaml:"enabled"`
	OwnerHeader      string `yaml:"owner_header"`
	AllowDevFallback bool   `yaml:"allow_dev_fallback"`
	GitHubToolMode   string `yaml:"github_tool_mode"` // mock | real
	GitHubAPIBaseURL string `yaml:"github_api_base_url"`
}

// Load 读取 YAML 并把敏感字段从环境变量注入。
// 优先级：env > yaml > default。
func Load(path string) (*Config, error) {
	cfg := defaults()
	var keys sensitiveYAMLKeys

	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config %s: %w", path, err)
		}
		if err := yaml.Unmarshal(raw, cfg); err != nil {
			return nil, fmt.Errorf("parse yaml: %w", err)
		}
		if err := yaml.Unmarshal(raw, &keys); err != nil {
			return nil, fmt.Errorf("parse yaml keys: %w", err)
		}
	}

	// 敏感字段运行时单独保存；优先级：env > yaml > default。
	cfg.PostgresDSN = os.Getenv("INTERVIEW_POSTGRES_DSN")
	cfg.RedisAddr = os.Getenv("INTERVIEW_REDIS_ADDR")
	cfg.RedisPassword = os.Getenv("INTERVIEW_REDIS_PASSWORD")
	cfg.LLMAPIKey = keys.LLM.APIKey
	cfg.EmbeddingAPIKey = keys.Embedding.APIKey
	if v := os.Getenv("INTERVIEW_LLM_API_KEY"); v != "" {
		cfg.LLMAPIKey = v
	}
	if v := os.Getenv("INTERVIEW_EMBEDDING_API_KEY"); v != "" {
		cfg.EmbeddingAPIKey = v
	}

	// ops-tweakable env overrides
	if v := os.Getenv("INTERVIEW_SERVER_ADDR"); v != "" {
		cfg.Server.Addr = v
	}
	if v := os.Getenv("INTERVIEW_LLM_MODE"); v != "" {
		cfg.LLM.Mode = v
	}
	if v := os.Getenv("INTERVIEW_EMBEDDING_MODE"); v != "" {
		cfg.Embedding.Mode = v
	}
	if v := os.Getenv("INTERVIEW_EMBEDDING_BASE_URL"); v != "" {
		cfg.Embedding.BaseURL = v
	}
	if v := os.Getenv("INTERVIEW_EMBEDDING_MODEL"); v != "" {
		cfg.Embedding.Model = v
	}
	if v := os.Getenv("INTERVIEW_EMBEDDING_DIMENSION"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid INTERVIEW_EMBEDDING_DIMENSION %q: %w", v, err)
		}
		cfg.Embedding.Dimension = n
	}
	if v := os.Getenv("INTERVIEW_RERANK_MODE"); v != "" {
		cfg.Rerank.Mode = v
	}
	if v := os.Getenv("INTERVIEW_RERANK_ENDPOINT"); v != "" {
		cfg.Rerank.Endpoint = v
	}
	if v := os.Getenv("INTERVIEW_RATELIMIT_BACKEND"); v != "" {
		cfg.RateLimit.Backend = v
	}
	if v := os.Getenv("INTERVIEW_IMPORT_SPOOL_DIR"); v != "" {
		cfg.Server.ImportSpoolDir = v
	}
	if v := os.Getenv("INTERVIEW_INTERNAL_TRIAL_ENABLED"); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("invalid INTERVIEW_INTERNAL_TRIAL_ENABLED %q: %w", v, err)
		}
		cfg.InternalTrial.Enabled = enabled
	}
	if v := os.Getenv("INTERVIEW_INTERNAL_TRIAL_OWNER_HEADER"); v != "" {
		cfg.InternalTrial.OwnerHeader = v
	}
	if v := os.Getenv("INTERVIEW_INTERNAL_TRIAL_ALLOW_DEV_FALLBACK"); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("invalid INTERVIEW_INTERNAL_TRIAL_ALLOW_DEV_FALLBACK %q: %w", v, err)
		}
		cfg.InternalTrial.AllowDevFallback = enabled
	}
	if v := os.Getenv("INTERVIEW_INTERNAL_TRIAL_GITHUB_TOOL_MODE"); v != "" {
		cfg.InternalTrial.GitHubToolMode = v
	}
	if v := os.Getenv("INTERVIEW_INTERNAL_TRIAL_GITHUB_API_BASE_URL"); v != "" {
		cfg.InternalTrial.GitHubAPIBaseURL = v
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

type sensitiveYAMLKeys struct {
	LLM struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"llm"`
	Embedding struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"embedding"`
}

func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Addr:           ":8080",
			ReadTimeout:    15 * time.Second,
			WriteTimeout:   60 * time.Second, // SSE 长连接需要大一些
			ShutdownGrace:  30 * time.Second,
			MaxInFlight:    200,
			MaxStreams:     100,
			ImportSpoolDir: "data/import-spool",
		},
		Postgres: PostgresConfig{
			// 算法注释（亮点之一）：
			// MaxConns ≈ 单机最大并发 LLM 调用数 × 1.5
			// LLM 调用期间持有 PG 连接做 state 读写
			MaxConns:          50,
			MinConns:          5,
			MaxConnLifetime:   30 * time.Minute,
			HealthCheckPeriod: 1 * time.Minute,
		},
		Redis: RedisConfig{
			DB: 0,
			// SSE 订阅 + state 读写 + 限流，每个请求最多 ~3 个 Redis 调用，
			// pool 大小约等于 MaxInFlight × 2 留 buffer。
			PoolSize:     400,
			MinIdleConns: 20,
			DialTimeout:  3 * time.Second,
			KeyPrefix:    "interview:",
		},
		LLM: LLMConfig{
			Mode:                    "mock", // 默认 mock，部署时 env 切 real
			Timeout:                 30 * time.Second,
			MaxTokens:               1200,
			MaxRetries:              2,
			MaxConcurrency:          4,
			BreakerFailureThreshold: 5,
			BreakerOpenDuration:     30 * time.Second,
		},
		Embedding: EmbeddingConfig{
			Mode:      "mock",
			BaseURL:   "https://dashscope.aliyuncs.com/compatible-mode/v1",
			Model:     "text-embedding-v4",
			Dimension: 1024,
			Timeout:   10 * time.Second,
		},
		Rerank: RerankConfig{
			Mode:    "lexical",
			Timeout: 5 * time.Second,
		},
		RateLimit: RateLimitConfig{
			Backend:      "local",
			PerUserQPS:   0.5,
			PerUserBurst: 3,
			GlobalQPS:    20,
			GlobalBurst:  40,
		},
		InternalTrial: InternalTrialConfig{
			Enabled:          false,
			OwnerHeader:      "X-Internal-User",
			AllowDevFallback: false,
			GitHubToolMode:   "mock",
			GitHubAPIBaseURL: "https://api.github.com",
		},
	}
}

func (c *Config) validate() error {
	if c.Postgres.MaxConns <= 0 {
		return fmt.Errorf("invalid postgres.max_conns %d (must be positive)", c.Postgres.MaxConns)
	}
	if c.Postgres.MinConns < 0 {
		return fmt.Errorf("invalid postgres.min_conns %d (must be >= 0)", c.Postgres.MinConns)
	}
	if c.Postgres.MinConns > c.Postgres.MaxConns {
		return fmt.Errorf("invalid postgres.min_conns %d (must be <= postgres.max_conns %d)", c.Postgres.MinConns, c.Postgres.MaxConns)
	}
	if c.Postgres.MaxConnLifetime <= 0 {
		return fmt.Errorf("invalid postgres.max_conn_lifetime %v (must be > 0)", c.Postgres.MaxConnLifetime)
	}
	if c.Postgres.HealthCheckPeriod <= 0 {
		return fmt.Errorf("invalid postgres.health_check_period %v (must be > 0)", c.Postgres.HealthCheckPeriod)
	}
	if c.LLM.Mode != "mock" && c.LLM.Mode != "real" {
		return fmt.Errorf("invalid llm.mode %q (must be mock|real)", c.LLM.Mode)
	}
	if c.LLM.Mode == "real" && c.LLMAPIKey == "" {
		return fmt.Errorf("LLM in real mode but INTERVIEW_LLM_API_KEY is empty")
	}
	if c.LLM.MaxConcurrency <= 0 {
		return fmt.Errorf("invalid llm.max_concurrency %d (must be positive)", c.LLM.MaxConcurrency)
	}
	if c.LLM.Mode == "real" {
		if c.LLM.BreakerFailureThreshold < 1 {
			return fmt.Errorf("invalid llm.breaker_failure_threshold %d (must be >= 1)", c.LLM.BreakerFailureThreshold)
		}
		if c.LLM.BreakerOpenDuration <= 0 {
			return fmt.Errorf("invalid llm.breaker_open_duration %v (must be > 0)", c.LLM.BreakerOpenDuration)
		}
	}
	if c.Embedding.Mode == "real" && c.EmbeddingAPIKey == "" {
		return fmt.Errorf("embedding in real mode but INTERVIEW_EMBEDDING_API_KEY is empty")
	}
	if c.Embedding.Dimension <= 0 {
		return fmt.Errorf("invalid embedding.dimension %d (must be positive)", c.Embedding.Dimension)
	}
	if c.Rerank.Mode != "lexical" && c.Rerank.Mode != "http" {
		return fmt.Errorf("invalid rerank.mode %q (must be lexical|http)", c.Rerank.Mode)
	}
	if c.Rerank.Mode == "http" && c.Rerank.Endpoint == "" {
		return fmt.Errorf("rerank.mode is http but rerank.endpoint is empty")
	}
	if c.Rerank.Timeout <= 0 {
		return fmt.Errorf("invalid rerank.timeout %v (must be > 0)", c.Rerank.Timeout)
	}
	if c.RateLimit.Backend != "local" && c.RateLimit.Backend != "redis_lua" {
		return fmt.Errorf("invalid rate_limit.backend %q", c.RateLimit.Backend)
	}
	if c.InternalTrial.GitHubToolMode != "mock" && c.InternalTrial.GitHubToolMode != "real" {
		return fmt.Errorf("invalid internal_trial.github_tool_mode %q (must be mock|real)", c.InternalTrial.GitHubToolMode)
	}
	if c.InternalTrial.Enabled && strings.TrimSpace(c.InternalTrial.OwnerHeader) == "" {
		return fmt.Errorf("internal_trial.owner_header is required when internal trial is enabled")
	}
	return nil
}

func PostgresPoolConfig(cfg *Config) (*pgxpool.Config, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	poolCfg, err := pgxpool.ParseConfig(cfg.PostgresDSN)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}
	poolCfg.MaxConns = cfg.Postgres.MaxConns
	poolCfg.MinConns = cfg.Postgres.MinConns
	poolCfg.MaxConnLifetime = cfg.Postgres.MaxConnLifetime
	poolCfg.HealthCheckPeriod = cfg.Postgres.HealthCheckPeriod
	return poolCfg, nil
}
