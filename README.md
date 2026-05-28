# Interview Agent

>  Graph 驱动的智能面试评估系统 | Go · Gin · PostgreSQL+pgvector · Redis · LLM/RAG

智能面试训练 Agent，通过 LLM + RAG 解决求职者备战面试时缺乏针对性训练、反馈滞后的痛点。系统围绕岗位画像、个性化出题、动态评估和复盘报告构建完整模拟面试流程。

## 当前状态

当前已经完成“可本地演示”的核心 Agent 闭环：

- 领域模型：`Session`、`AnswerRound`、`WorkingMemory`、`Critic`、`Decision`、`Report`
- Graph：节点、路由、循环、`ErrSuspended` / `Resume`
- Setup 节点：`parse_jd`、`parse_resume`、`gap_analyze`、`analyze_profile`、`retrieve_rag`
- Agent 节点：`pick_next`、`evaluate`、`critic`、`refine`、`probe_ask`、`probe_eval`、`update_memory`、`reflection_check`、`report`
- 整图组装：`internal/graphs.BuildInterviewGraph`
- HTTP 骨架：`POST /api/interview/start`、`POST /api/interview/answer`
- 会话 API：`GET /api/interview/sessions`、`GET /api/interview/sessions/:session_id`
- SSE 流式响应：`GET /api/interview/stream?session_id=...`，支持 snapshot、heartbeat 和 `Last-Event-ID`
- Web 前端：支持输入 JD/简历、JD-简历匹配分析、题库预览、历史会话、SSE 事件时间线、回答诊断、训练计划和底部回答输入栏
- 简历文档解析入口：`POST /api/documents/parse-resume`，支持 PDF、DOCX、TXT、Markdown，并可在 Web 端上传后填充简历文本
- 题库预览 API：`GET /api/question-bank`、`GET /api/question-bank/:id`、`GET /api/question-bank/facets`，支持技能、场景、难度、标签和关键词过滤
- Redis Streams 事件总线：设置 `INTERVIEW_REDIS_URL` 后启用
- PG session store：配置 `INTERVIEW_POSTGRES_DSN` 后保存到 `sessions.state_json`
- Redis session snapshot / lease / takeover：设置 `INTERVIEW_REDIS_URL` 后启用跨实例恢复和租约冲突保护
- Real LLM 可靠性保护：进程内并发 limiter、熔断器、`/readyz` degraded 状态
- HTTP 背压：`/api/interview/start`、`/api/interview/answer` 使用 `server.max_in_flight` 限制突发并发；SSE 流使用 `server.max_streams`
- Prometheus metrics 文本指标：`GET /metrics` 暴露 HTTP、简历解析、SSE、Graph 节点、LLM 调用 / token、熔断器状态和背压计数
- 离线题库工具：`cmd/reindex`
- Smoke 脚本：`make demo` 调用 `scripts/smoke.sh`，覆盖健康检查与 start/get/answer/list；`scripts/smoke_sse.ps1` 覆盖 SSE 服务级路径
- 结构化 demo CLI：`make demo-mock` / `make demo-real` 会生成 `tmp/demos/<timestamp>/run.json` 和 `report.md`

仍未完成：OTel tracing、Chaos 故障注入脚本、压测报告、Helm chart、题单预览节点和 RAG 评估指标；Prometheus 目前用轻量 summary / counter / gauge，还没接 Prometheus SDK histogram。

## 快速启动

```bash
# 1. 拉取依赖
go mod tidy

# 2. 启动 Web 服务（mock LLM，无外部依赖）
make demo-web

# 3. 打开浏览器
# http://localhost:8080

# 4. 基础健康检查
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
curl http://localhost:8080/api/ping
```

Web 页面支持直接输入岗位 JD 和候选人简历，也可以点击“读取简历文档”上传 `.txt` / `.md` / `.markdown` / `.pdf` / `.docx`。开始面试后，页面会展示 JD-简历匹配分、命中/缺失要求、风险点、简历优化建议和项目追问计划；最终报告会展示回答质量诊断和下一轮训练计划，训练计划会优先关联本轮 RAG 候选池里的题库题 ID。面试过程中继续展示 SSE 事件时间线、等待状态、当前题、历史问答和底部回答输入栏。

真实 LLM Web 模式：

```bash
# API key 推荐通过环境变量注入；也支持本地未入仓 YAML fallback，环境变量优先级更高。
export INTERVIEW_LLM_API_KEY="sk-xxx"
make demo-web-real
```

Windows / PowerShell 可用等价命令：

```powershell
$env:INTERVIEW_LLM_API_KEY="sk-xxx"
$env:INTERVIEW_LLM_MODE="real"
$env:INTERVIEW_EMBEDDING_MODE="mock"
go run ./cmd/server -config config/config.yaml.example
```

`make demo` 会构建并启动本地服务，然后检查 `/healthz`、`/readyz`、`/api/ping`，并用 mock 模式跑一轮 `interview/start`、`sessions/:id`、`interview/answer`、`sessions?user_id=...`。

核心回归可用：

```bash
make test-core
```

结构化端到端 demo 可直接跑 CLI，不启 HTTP，也不依赖 Redis。设置 `INTERVIEW_POSTGRES_DSN` 时会使用 PG/pgvector 题库；未设置时会降级到 fallback 题库，`run.json.config.retriever` 会明确记录实际路径：

```powershell
$env:INTERVIEW_POSTGRES_DSN="postgres://interview:interview@localhost:5432/interview?sslmode=disable"
go run ./cmd/demo -config config/config.yaml -script testdata/demo/example.yaml
```

如果服务已经手动启动，可只跑 HTTP 检查：

```bash
USE_EXISTING_SERVER=1 sh ./scripts/smoke.sh
```

Windows / PowerShell 可用：

```powershell
$env:USE_EXISTING_SERVER="1"; ./scripts/smoke.ps1
```

SSE 服务级 smoke 可用：

```powershell
go build -o .\server-smoke.exe ./cmd/server
$env:SERVER_BIN=".\server-smoke.exe"; ./scripts/smoke_sse.ps1
```

如果要验证 Redis Streams 事件总线，先设置 Redis URL：

```powershell
go build -o .\server-smoke.exe ./cmd/server
$env:INTERVIEW_REDIS_URL="redis://localhost:6379/0"
$env:SERVER_BIN=".\server-smoke.exe"; ./scripts/smoke_sse.ps1
```

真实 PG smoke：

```bash
export INTERVIEW_POSTGRES_DSN="postgres://interview:interview@localhost:5432/interview?sslmode=disable"
make demo-pg
```

如果还没迁移和 seed：

```bash
export INTERVIEW_POSTGRES_DSN="postgres://interview:interview@localhost:5432/interview?sslmode=disable"
make demo-pg-full
```

## API

解析简历文档：

```bash
curl -X POST http://localhost:8080/api/documents/parse-resume \
  -F "file=@./resume.pdf"
```

返回字段包括：

```json
{
  "filename": "resume.pdf",
  "text": "...解析后的简历文本...",
  "page_count": 1,
  "metadata": {
    "format": "pdf"
  }
}
```

题库预览：

```bash
curl "http://localhost:8080/api/question-bank?skill_category=go&scenario=fundamentals&limit=20"
curl "http://localhost:8080/api/question-bank/facets"
curl "http://localhost:8080/api/question-bank/go-001?view=admin"
```

默认候选人视图不会返回 `expected_points`、`rubric`、`sample_answer` 和 `follow_up_hints`。`view=admin` 会返回完整题目元数据，用于管理/演示。

启动一场面试：

```bash
curl -X POST http://localhost:8080/api/interview/start \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "demo-1",
    "user_id": "u1",
    "jd_text": "需要 Go 后端工程师，熟悉 Redis 和并发编程",
    "resume_text": "两年 Go 开发经验，做过 Redis 缓存和后端服务"
  }'
```

提交当前题答案：

```bash
curl -X POST http://localhost:8080/api/interview/answer \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "demo-1",
    "user_id": "u1",
    "answer": "G 是 goroutine，M 是系统线程，P 负责调度本地队列"
  }'
```

列出用户会话：

```bash
curl "http://localhost:8080/api/interview/sessions?user_id=u1&limit=20"
```

`limit` 默认 20，最大 100。

查看单个会话：

```bash
curl "http://localhost:8080/api/interview/sessions/demo-1"
```

可追加 `user_id` 做轻量归属校验：

```bash
curl "http://localhost:8080/api/interview/sessions/demo-1?user_id=u1"
```

注意：未配置 `INTERVIEW_POSTGRES_DSN` 时，HTTP 会话存储仍是内存 map，服务重启后会丢失；配置 DSN 后会保存到 PG `sessions.state_json`。Mock LLM 内置了一组 demo 响应，用于无外部依赖地跑通最小 interview 流程；真实评估质量仍需要 real LLM 或专门 fixture。

所有 interview 响应都会返回 `created_at` / `updated_at`，会话列表按 `updated_at` 倒序。
`answer` 和会话详情都支持可选 `user_id` 归属校验；不传时保持兼容。
`GET /api/interview/stream?session_id=...` 可直接订阅单个会话的 SSE 流；支持 `Last-Event-ID` 做断线回放，并会发送 heartbeat 保活。默认使用内存事件总线；设置 `INTERVIEW_REDIS_URL=redis://localhost:6379/0` 后会改用 Redis Streams。Redis ACL 用户可使用 `redis://username:password@localhost:6379/0`。

Metrics：

```bash
curl http://localhost:8080/metrics
```

当前包含 HTTP request counter、parser document counter、SSE active/total、Graph node total / duration summary、LLM calls / duration summary / tokens、breaker state gauge、in-flight gauge、backpressure rejection counter。

Redis session coordinator 已接入 `InterviewService`：设置 `INTERVIEW_REDIS_URL` 后，`start` 会获取 session lease 并写 snapshot，`get/answer` 在本地 store miss 时会从 Redis snapshot 恢复，`answer` 会续约或重新获取过期 lease，成功后更新 snapshot；会话完成后释放 lease。Redis lease 冲突会返回 HTTP 409，并带 `Retry-After` / `retry_after_seconds`。
Redis snapshot 写入失败不会打断 start/answer 主流程；服务会在 `WorkingMemory.DegradedReasons["redis_snapshot"]` 记录原因，并继续依赖 PG / 内存 session store。Redis lease acquire / renew 失败仍会阻断，因为它关系到多实例所有权。
Redis / memory event hub 会记录 publish 失败和慢消费者丢事件计数；事件推送失败不阻断 interview 主流程。
Real LLM 模式会套进程内并发限制，配置项为 `llm.max_concurrency`，默认 4；等待并发闸门时会尊重请求 context。

## Roadmap（按当前 ROI）

1. 可演示性闭环：README、`make demo-web`、真实浏览器流程验收、demo 产物说明。
2. OTel tracing：贯穿 HTTP → Graph → LLM / Parser / Retriever，支持按 `trace_id` 定位慢节点。
3. 可靠性演示：熔断器半开期渐进放行、Chaos 故障注入脚本和恢复验证。
4. 工程化：GitHub Actions CI 矩阵、Docker 镜像、k8s Helm chart 和 probes。
5. 产品增强：`preview_plan` 题单预览、候选人档案复用、项目背景库和简化版 RAGAS 指标。

## 测试

```bash
# 全量测试
make test

# 关键回归
go test ./internal/nodes/ -run AgentLoop -v -count=1
go test ./internal/graphs -v -count=1
go test ./internal/httpapi -v -count=1
```

需要 PG 的 pgvector / session store 集成测试受 `INTEGRATION=1` + `INTERVIEW_POSTGRES_DSN` 控制；旧 `PG_DSN` 仍兼容。

Redis Streams 集成测试受 `INTEGRATION=1` + `INTERVIEW_REDIS_URL` 控制：

```bash
INTERVIEW_REDIS_URL=redis://localhost:6379/0 INTEGRATION=1 go test ./internal/httpapi -run TestRedisInterviewEventHub_IntegrationPublishSubscribe -count=1 -v
```

Redis session coordinator 集成测试：

```bash
INTERVIEW_REDIS_URL=redis://localhost:6379/0 INTEGRATION=1 go test ./internal/httpapi -run TestRedisSessionCoordinator -count=1 -v
```

## 数据库与题库

```bash
# 启动 PG + Redis
make docker-up

# 设置 DSN 后迁移
export INTERVIEW_POSTGRES_DSN="postgres://interview:interview@localhost:5432/interview?sslmode=disable"
make migrate-up

# 重建题库 embedding
go run ./cmd/reindex -seed seeds/question_bank.json -mode mock
```

Real embedding 模式需要设置 `INTERVIEW_EMBEDDING_API_KEY`，且 embedding 维度必须和 `question_bank.embedding` 的 `vector(N)` 一致。
题库 seed 支持 `scenario`、`role_tags`、`rubric`、`sample_answer`、`follow_up_hints`、`locale`、`status` 等元数据；重跑 `cmd/reindex` 会同步这些字段并刷新 `updated_at`。

## 目录索引

| 能力 | 主要位置 |
|---|---|
| Graph 执行框架 | `internal/graph/` |
| Graph 组装 | `internal/graphs/` |
| Agent 节点 | `internal/nodes/` |
| 领域模型 | `internal/domain/` |
| LLM 抽象 | `internal/llm/` |
| Embedding 抽象 | `internal/embedding/` |
| pgvector 检索 | `internal/retriever/` |
| HTTP API | `internal/httpapi/` |
| 服务入口 | `cmd/server/` |
| 题库重建 | `cmd/reindex/` |
| 迁移 | `migrations/` |

## 安全说明

- API key 推荐走环境变量；`config.Load` 也支持从本地未入仓 YAML 读取 fallback key，且环境变量优先级更高。
- `.env` 和 `config.yaml` 不入仓，`config/config.yaml.example` 不包含真实密钥。
- 示例 key 只使用 `sk-xxx` 占位。

## License

仅用于学习与求职演示。
