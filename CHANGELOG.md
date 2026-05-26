# Interview Agent — Changelog

## stage-0 · 项目骨架（2026-05-23）

### Added
- 项目结构（`cmd/`, `internal/`, `pkg/`, `migrations/`, `chaos/`, `web/`, `docs/`）
- `go.mod` 含 eino / gin / pgx / go-redis / gobreaker / ulid / ledongthuc/pdf / docx / errgroup / time/rate
- `internal/config`：YAML + env 双源加载，敏感字段强制走 env
- `internal/llm`：`ChatModel` 接口 + `MockChatModel`（fixture 驱动）+ `RealChatModel`（阶段 2 完善）
- `internal/embedding`：`Embedder` 接口 + `MockEmbedder`（FNV hash + L2 归一化）+ `RealEmbedder`（阶段 3 完善）
- `internal/observability`：基于 slog 的 ContextHandler 自动注入 `trace_id`
- `internal/httpapi`：TraceID middleware、Router、`/healthz`、`/readyz`、`/api/ping`
- `pkg/traceid`：context 派生 trace id
- `cmd/server/main.go`：服务入口 + 优雅停机骨架
- `config/config.yaml.example`、`.env.example`
- `docker-compose.yml`（单实例 + `cluster` profile）
- `Makefile`、`.golangci.yml`、`.gitignore`
- `docs/specs/2026-05-23-interview-agent-design.md` 完整设计文档

### 亮点
- 配置层防泄漏：敏感字段 `yaml:"-"` + 单测 `TestSensitiveFieldsNotInYAML` 确认 marshal 不会带出 key
- Mock 完全脱离外部依赖，单测 `go test ./...` 可在零依赖环境跑通
- 优雅停机骨架预留：监听 SIGTERM/SIGINT、context-with-timeout、记录 grace 时长

### 难点 & 已规避
- 旧项目把 DashScope key 硬编码在 `interview.json` 已泄漏 → 新项目硬约束 yaml 不准放 key，validate() 强制要求
- 单元测试不依赖网络 / 真实 LLM → Mock 抽象第一阶段就立起来
- 维度选择歧义：Mock embedder 默认 768，与 migration 中 `vector(768)` 锁定
