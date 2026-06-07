# 06-07 Postgres 连接池配置生效

## 1. 变更概述

修复 Postgres 连接池配置未生效的问题。此前 `internal/config.Config.Postgres` 定义了 `max_conns`、`min_conns`、`max_conn_lifetime`、`health_check_period`，但多个入口直接调用 `pgxpool.New(ctx, dsn)`，实际使用 pgxpool 默认值。

本次改为在 `internal/config` 提供统一 `PostgresPoolConfig`，先 `pgxpool.ParseConfig(cfg.PostgresDSN)`，再把项目配置写入 `pgxpool.Config`。`cmd/server`、`cmd/rag-eval`、`cmd/demo` 和 `cmd/questionbank-explain` 使用 `pgxpool.NewWithConfig` 创建连接池。

## 2. 变更文件

- `internal/config/config.go`：新增 `PostgresPoolConfig`，作为项目统一 PG pool 配置构建入口。
- `internal/config/config.go`：新增 Postgres pool 参数校验，拒绝 0/负数和 `min_conns > max_conns`。
- `internal/config/config_test.go`：新增不连接数据库的配置映射单元测试和非法 pool 参数校验测试。
- `cmd/server/deps.go`：接入统一 Postgres pool 配置。
- `cmd/server/main_test.go`：新增不连接数据库的配置映射单元测试。
- `cmd/rag-eval/main.go`：PGVector eval 连接池改用统一配置。
- `cmd/demo/main.go`：demo PGVector retriever 连接池改用统一配置。
- `cmd/questionbank-explain/main.go`：题库 EXPLAIN 诊断连接池改用统一配置。

## 3. 函数级说明

### `internal/config/config.go`

- `Config.validate() error`
  - 行为变化：新增 PostgreSQL pool 参数校验。
  - 输入：完整项目配置。
  - 输出：配置错误或 nil。
  - 错误处理：`postgres.max_conns <= 0`、`postgres.min_conns < 0`、`postgres.min_conns > postgres.max_conns`、`postgres.max_conn_lifetime <= 0`、`postgres.health_check_period <= 0` 均返回稳定字段名错误。
  - 副作用：无。
  - 兼容性：默认配置不受影响；显式写入非法 pool 值的 YAML 会在启动/命令加载配置时提前失败。

- `PostgresPoolConfig(cfg *Config) (*pgxpool.Config, error)`
  - 作用：把项目配置转换为 pgxpool 配置。
  - 输入：`cfg.PostgresDSN` 和 `cfg.Postgres`。
  - 输出：已写入 `MaxConns`、`MinConns`、`MaxConnLifetime`、`HealthCheckPeriod` 的 `*pgxpool.Config`。
  - 错误处理：`cfg == nil` 返回 `config is nil`；`pgxpool.ParseConfig` 失败时返回包装错误。
  - 副作用：无；不建立网络连接。

### `cmd/server/deps.go`

- `buildAppDeps(ctx context.Context, cfg *config.Config) (appDeps, func(), error)`
  - 行为变化：有 `PostgresDSN` 时不再直接调用 `pgxpool.New`，而是调用 `postgresPoolConfig` 后用 `pgxpool.NewWithConfig` 创建 pool。
  - 输入：应用配置和 context。
  - 输出：包含 `PGPool` 的依赖集合、cleanup 函数和错误。
  - 错误处理：DSN 解析失败返回 `parse postgres dsn`；连接失败返回 `connect postgres`；ping 失败会关闭 pool 并返回 `ping postgres`。
  - 副作用：真实启动时 Postgres 连接池将按项目配置限制连接数和健康检查周期。

- `postgresPoolConfig(cfg *config.Config) (*pgxpool.Config, error)`
  - 行为变化：改为委托 `config.PostgresPoolConfig`，保留 server 包内测试和调用兼容。

### `cmd/rag-eval/main.go`

- `buildRetriever(ctx context.Context, cfg *config.Config, seedPath string, embedder embedding.Embedder) (retriever.Retriever, func(), string, error)`
  - 行为变化：配置了 `cfg.PostgresDSN` 时，先调用 `config.PostgresPoolConfig`，再使用 `pgxpool.NewWithConfig`。
  - 输入：上下文、配置、seed 路径和 embedder。
  - 输出：PGVector 或 seed retriever、cleanup、source 和错误。
  - 错误处理：pool config 解析失败直接返回 `pgvector` source 错误；连接和 ping 错误保持原语义。
  - 副作用：离线 RAG eval 连接池开始遵守 `postgres:` 配置。

### `cmd/demo/main.go`

- `buildDemoRetriever(ctx context.Context, cfg *config.Config) (retriever.Retriever, func(), string, error)`
  - 行为变化：配置了 `cfg.PostgresDSN` 时，使用统一 pool config 创建 PG 连接池。
  - 输入：上下文和配置。
  - 输出：PGVector 或 fallback retriever、cleanup、retriever kind 和错误。
  - 错误处理：pool config 解析失败、连接失败、ping 失败分别返回错误。
  - 副作用：demo 使用 PGVector 时连接池开始遵守 `postgres:` 配置。

### `cmd/questionbank-explain/main.go`

- `run(opts options) int`
  - 行为变化：连接 PG 前先调用 `config.PostgresPoolConfig`，再使用 `pgxpool.NewWithConfig`。
  - 输入：CLI options。
  - 输出：进程退出码。
  - 错误处理：pool config 解析失败输出 `ERROR: postgres pool config` 并返回 2。
  - 副作用：题库 EXPLAIN 诊断连接池开始遵守 `postgres:` 配置。

### `cmd/server/main_test.go`

### `internal/config/config_test.go`

- `TestPostgresPoolConfigAppliesConfiguredPoolSettings`
  - 作用：验证 helper 会保留 DSN 解析结果，并把四个连接池字段写入 pgxpool config。
  - 输入：测试构造的 DSN 和 pool 配置。
  - 输出：测试断言。
  - 副作用：无数据库连接。

- `TestValidate_InvalidPostgresPoolConfigFails`
  - 作用：验证非法 Postgres pool 参数会在配置校验阶段失败。
  - 输入：测试构造的非法 `PostgresConfig`。
  - 输出：测试断言错误信息包含对应字段名。
  - 副作用：无数据库连接。

## 4. 调用链

服务启动 -> `cmd/server/main.go` 调用 `buildAppDeps(ctx, cfg)` -> `buildAppDeps` 检查 `cfg.PostgresDSN` -> `postgresPoolConfig(cfg)` -> `config.PostgresPoolConfig(cfg)` -> `pgxpool.ParseConfig` -> 写入 `cfg.Postgres` 四个字段 -> `pgxpool.NewWithConfig` -> `pool.Ping` -> app deps 注入后续 interview/questionbank/retriever 组件。

RAG eval -> `go run ./cmd/rag-eval ...` -> `buildRetriever` -> `config.PostgresPoolConfig` -> `pgxpool.NewWithConfig` -> `PGVectorRetriever`。

Demo -> `go run ./cmd/demo ...` -> `buildDemoRetriever` -> `config.PostgresPoolConfig` -> `pgxpool.NewWithConfig` -> `PGVectorRetriever`。

题库诊断 -> `go run ./cmd/questionbank-explain ...` -> `run` -> `config.PostgresPoolConfig` -> `pgxpool.NewWithConfig` -> `PGStore.ExplainList`。

测试入口 -> `go test ./internal/config -count=1` -> `TestPostgresPoolConfigAppliesConfiguredPoolSettings` -> `config.PostgresPoolConfig`。

## 5. 数据流

- 配置来源：`internal/config.defaults()`、YAML `postgres:` 配置、环境变量注入的 `INTERVIEW_POSTGRES_DSN`。
- 校验：`Config.validate` 确认 pool 参数为正数、非负数且 `min_conns <= max_conns`。
- 转换：`Config.Postgres` -> `pgxpool.Config`。
- 使用：`pgxpool.NewWithConfig` 创建服务、RAG eval、demo 和题库诊断 pool。
- 存储：无数据库 schema 变化。

## 6. 依赖与副作用

- 无新增外部依赖。
- `internal/config/config.go` 新增 `github.com/jackc/pgx/v5/pgxpool` import；项目已有依赖。
- `internal/config/config_test.go` 新增 `time` import。
- 运行时副作用：Postgres 最大连接数、最小连接数、连接最大生命周期、健康检查周期开始在服务和后端诊断/评估 CLI 中按配置生效。
- 兼容性：DSN 解析仍由 pgxpool 负责；无 HTTP/API/JSON/DB schema 变化。

## 7. 测试

已执行：

```powershell
go test ./cmd/server -count=1
go test ./internal/config -run TestPostgresPoolConfigAppliesConfiguredPoolSettings -count=1
go test ./internal/config -count=1
go test ./cmd/questionbank-explain ./cmd/rag-eval ./cmd/demo ./cmd/server -count=1
```

结果：通过。

未连接真实 Postgres 做运行时验证；当前验证覆盖配置映射和不需要外部数据库的包测试。

## 8. 风险

- 如果 YAML 将 `postgres.max_conns` 等字段配置为 0、负数或 `min_conns > max_conns`，现在会在配置加载阶段失败。这是有意的早失败，避免坏 pool 配置进入运行期。
- `cmd/reindex` 仍使用独立 `-dsn` 参数，不读取项目 `config.Config`，本次没有强行改造。
- 题库 OFFSET、搜索 OR/ILIKE、session JSON 拆表、events 膨胀风险本次没有处理，需要单独用数据和 EXPLAIN 证据推进。
