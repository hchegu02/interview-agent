# 06-07 Postgres 连接池配置生效

## 1. 变更概述

修复服务启动时 Postgres 连接池配置未生效的问题。此前 `internal/config.Config.Postgres` 定义了 `max_conns`、`min_conns`、`max_conn_lifetime`、`health_check_period`，但 `cmd/server/deps.go` 直接调用 `pgxpool.New(ctx, cfg.PostgresDSN)`，实际使用 pgxpool 默认值。

本次改为先 `pgxpool.ParseConfig(cfg.PostgresDSN)`，再把项目配置写入 `pgxpool.Config`，最后使用 `pgxpool.NewWithConfig` 创建连接池。

## 2. 变更文件

- `cmd/server/deps.go`：接入 Postgres pool 配置，新增 `postgresPoolConfig` helper。
- `cmd/server/main_test.go`：新增不连接数据库的配置映射单元测试。

## 3. 函数级说明

### `cmd/server/deps.go`

- `buildAppDeps(ctx context.Context, cfg *config.Config) (appDeps, func(), error)`
  - 行为变化：有 `PostgresDSN` 时不再直接调用 `pgxpool.New`，而是调用 `postgresPoolConfig` 后用 `pgxpool.NewWithConfig` 创建 pool。
  - 输入：应用配置和 context。
  - 输出：包含 `PGPool` 的依赖集合、cleanup 函数和错误。
  - 错误处理：DSN 解析失败返回 `parse postgres dsn`；连接失败返回 `connect postgres`；ping 失败会关闭 pool 并返回 `ping postgres`。
  - 副作用：真实启动时 Postgres 连接池将按项目配置限制连接数和健康检查周期。

- `postgresPoolConfig(cfg *config.Config) (*pgxpool.Config, error)`
  - 作用：把项目配置转换为 pgxpool 配置。
  - 输入：`cfg.PostgresDSN` 和 `cfg.Postgres`。
  - 输出：已写入 `MaxConns`、`MinConns`、`MaxConnLifetime`、`HealthCheckPeriod` 的 `*pgxpool.Config`。
  - 错误处理：`pgxpool.ParseConfig` 失败时返回包装错误。
  - 副作用：无；不建立网络连接。

### `cmd/server/main_test.go`

- `TestPostgresPoolConfigAppliesConfiguredPoolSettings`
  - 作用：验证 helper 会保留 DSN 解析结果，并把四个连接池字段写入 pgxpool config。
  - 输入：测试构造的 DSN 和 pool 配置。
  - 输出：测试断言。
  - 副作用：无数据库连接。

## 4. 调用链

服务启动 -> `cmd/server/main.go` 调用 `buildAppDeps(ctx, cfg)` -> `buildAppDeps` 检查 `cfg.PostgresDSN` -> `postgresPoolConfig(cfg)` -> `pgxpool.ParseConfig` -> 写入 `cfg.Postgres` 四个字段 -> `pgxpool.NewWithConfig` -> `pool.Ping` -> app deps 注入后续 interview/questionbank/retriever 组件。

测试入口 -> `go test ./cmd/server -count=1` -> `TestPostgresPoolConfigAppliesConfiguredPoolSettings` -> `postgresPoolConfig`。

## 5. 数据流

- 配置来源：`internal/config.defaults()`、YAML `postgres:` 配置、环境变量注入的 `INTERVIEW_POSTGRES_DSN`。
- 转换：`Config.Postgres` -> `pgxpool.Config`。
- 使用：`pgxpool.NewWithConfig` 创建服务运行时 pool。
- 存储：无数据库 schema 变化。

## 6. 依赖与副作用

- 无新增外部依赖。
- `cmd/server/main_test.go` 新增 `time` import。
- 运行时副作用：Postgres 最大连接数、最小连接数、连接最大生命周期、健康检查周期开始按配置生效。
- 兼容性：DSN 解析仍由 pgxpool 负责；无 HTTP/API/JSON/DB schema 变化。

## 7. 测试

已执行：

```powershell
go test ./cmd/server -count=1
```

结果：通过。

未执行：

- 未跑 `go test ./... -count=1`，因为本次改动只触及 server deps 和对应测试。
- 未连接真实 Postgres 做运行时验证。

## 8. 风险

- 如果线上 YAML 将 `postgres.max_conns` 等字段配置为 0 或不合理值，现在会真实传给 pgxpool；当前默认值是正数。后续可以在 `config.validate` 增加 Postgres pool 参数约束。
- 题库 OFFSET、搜索 OR/ILIKE、session JSON 拆表、RAG EXPLAIN 证据、events 膨胀风险本次没有处理，需要单独用数据和 EXPLAIN 证据推进。
