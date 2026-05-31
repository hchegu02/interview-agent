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
- 离线量化工具：`cmd/rag-eval`、`cmd/questionbank-lint`、`cmd/eval`
- Smoke 脚本：`mingw32-make demo` 调用 `scripts/smoke.ps1`，覆盖健康检查与 start/get/answer/list；`scripts/smoke_sse.ps1` 覆盖 SSE 服务级路径
- 结构化 demo CLI：`mingw32-make demo-mock` / `mingw32-make demo-real` 会生成 `tmp/demos/<timestamp>/run.json` 和 `report.md`

仍未完成：OTel tracing、Helm chart、题单预览节点、真实业务规模的 RAG 评估集和长期压测报告；Prometheus 仍使用本项目轻量文本渲染器，没有引入 Prometheus SDK。

## 项目架构

### 总体分层

```text
web/ React + Vite
  |
  | HTTP JSON / SSE
  v
cmd/server
  |
  +-- internal/httpapi       Gin 路由、SSE、背压、metrics、session service
  +-- internal/graphs        面试 Graph 装配
  |     |
  |     +-- internal/graph   通用 frontier-based graph runner
  |     +-- internal/nodes   JD/简历解析、RAG、出题、评估、追问、报告节点
  |
  +-- internal/domain        Session 聚合根、画像、题目、轮次、报告
  +-- internal/llm           mock/real ChatModel、限流、熔断、调用记录
  +-- internal/embedding     mock/real embedding
  +-- internal/retriever     pgvector 检索与 fallback retriever
  +-- internal/questionbank  题库存储、导入、审核、commit
  +-- internal/parser        PDF/DOCX/TXT/Markdown 简历解析
  +-- internal/config        YAML + 环境变量配置
  +-- internal/observability 日志、trace id、graph/LLM 指标回调
```

前端构建产物会被 Go 服务通过 `internal/httpapi/web_assets.go` 暴露；开发和演示入口仍以 `cmd/server` 为准。

### 运行时依赖装配

`cmd/server/main.go` 是唯一服务入口，启动时按配置装配依赖：

- `config.Load` 读取 `config/config.yaml.example` 或指定 YAML，并用环境变量覆盖敏感项。
- `INTERVIEW_POSTGRES_DSN` 为空时使用内存 session store、内存题库 seed 和 fallback retriever；非空时连接 PostgreSQL/pgvector，启用 PG session store、PG 题库和 pgvector retriever。
- `INTERVIEW_REDIS_URL` 为空时使用内存事件总线；非空时使用 Redis Streams 事件总线，并启用 Redis session snapshot / lease / takeover。
- `llm.mode=mock|real` 控制 ChatModel；real 模式会套 `LimitedChatModel` 和 `BreakingChatModel`，`/readyz` 通过 breaker state 暴露 degraded 状态。
- `embedding.mode=mock|real` 控制题库检索 query embedding 和题库导入 embedding。

### HTTP/API 层

`internal/httpapi.Server` 持有路由和业务 service。核心路由分三类：

- 只读/健康：`/healthz`、`/readyz`、`/metrics`、`/api/ping`、会话查询、题库查询。
- 长连接：`GET /api/interview/stream`，使用独立 `server.max_streams` 背压，支持 snapshot、heartbeat 和 `Last-Event-ID`。
- 会触发 LLM/Graph 的写入口：`POST /api/interview/start`、`POST /api/interview/answer`，使用 `server.max_in_flight` 背压。

HTTP 层不直接实现面试决策，只负责请求校验、归属校验、调用 `InterviewService`、转换响应和发布/订阅事件。

### 面试 Graph 流程

`internal/graphs.BuildInterviewGraph` 把节点装成一个可恢复的 Graph：

```text
parse_jd
  -> parse_resume
  -> gap_analyze
  -> analyze_profile
  -> retrieve_rag
  -> pick_next
      -> evaluate
      -> critic
      -> refine 或 probe_ask
      -> probe_eval
      -> update_memory
      -> reflection_check
      -> pick_next 或 report
```

`internal/graph` 的执行模型是 frontier-based runner：每轮执行当前 frontier，按静态边或 router 计算下一轮。节点返回 `ErrSuspended` 时，Graph 正常暂停并把 `Session.CurrentNode` 留作断点；用户通过 `answer` 提交答案后，服务调用 `Resume` 从断点后续节点继续跑。

### 核心数据结构

`internal/domain.Session` 是一次面试的聚合根，同一份 JSON 结构被 HTTP 响应、Redis snapshot 和 PG `sessions.state_json` 复用。关键字段：

- `JobProfile` / `CandProfile`：JD 和简历画像。
- `GapReport` / `ProfileAnalysis`：匹配分析和提问策略依据。
- `CandidatePool` / `QuestionBankFilter`：RAG 召回候选池和用户限定范围。
- `WorkingMemory`：Agent 循环里的预算、覆盖度、降级原因等运行时记忆。
- `Rounds`：按时间追加的问答、追问、评估记录。
- `PendingDecision` / `CurrentNode`：暂停等待用户输入和恢复执行所需状态。
- `Report`：终评报告。

设计上避免把所有细节塞进一个巨大状态枚举：`SessionStatus` 只表达 created/running/paused/completed/failed 这种生命周期，细粒度流程由 Graph、`CurrentNode`、`Rounds` 和 `WorkingMemory` 表达。

### 存储、事件和降级路径

| 能力 | 默认本地模式 | 配置外部依赖后 |
|---|---|---|
| 会话存储 | 内存 map，进程重启丢失 | PostgreSQL `sessions.state_json` |
| 会话恢复/租约 | 无跨实例恢复 | Redis snapshot + lease，冲突返回 HTTP 409 |
| 事件总线 | 内存 hub | Redis Streams |
| 题库 | `seeds/question_bank.json` 加载到内存 | PostgreSQL `question_bank` |
| RAG 检索 | fallback retriever，明确记录降级 | pgvector |
| LLM | mock fixture | OpenAI-compatible real LLM + limiter + breaker |
| Embedding | mock vector | OpenAI-compatible embedding |

Redis snapshot 写入失败不会中断主流程，会记录到 `WorkingMemory.DegradedReasons["redis_snapshot"]`；Redis lease 获取/续约失败会阻断，因为它决定多实例下同一 session 的写入所有权。

### 题库导入链路

题库相关 API 位于 `internal/httpapi/question_bank.go`，核心逻辑在 `internal/questionbank/`：

1. 上传题库文件或普通文档到 `POST /api/question-bank/imports`。
2. `parser.Dispatcher` 解析文件文本。
3. LLM 从文档中抽取候选题，或直接解析结构化题库。
4. import job 和 item 进入内存或 PG import store。
5. 人工 review 后调用 commit，写入题库 store。
6. real embedding 模式下写入 embedding 字段，供 pgvector 检索。

## 工程化量化指标

本项目现在把“能跑通 demo”和“质量可回归”分开验证。核心离线命令：

```powershell
go run ./cmd/rag-eval -cases testdata/rag/golden_queries.jsonl -config config/config.yaml.example -out tmp/eval/rag
go run ./cmd/questionbank-lint -seed seeds/question_bank.json -min-expected-points 3 -min-scenario-ratio 0.8
go run ./cmd/eval -suite testdata/eval -mode mock -out tmp/eval/mock
```

Makefile 等价入口（Windows 使用 `mingw32-make`；有 GNU Make 的环境可把命令里的 `mingw32-make` 替换为 `make`）：

```powershell
mingw32-make eval-rag
mingw32-make questionbank-lint
mingw32-make questionbank-lint-strict
mingw32-make eval-mock
mingw32-make e2e-smoke
mingw32-make chaos-dry-run
mingw32-make load-test
```

`mingw32-make e2e-smoke` 会启动 mock server，覆盖 health/ready、题库 facets/list、interview start、SSE snapshot、answer/report、session detail/list 和 `/metrics`，并输出 `tmp/e2e/<timestamp>/summary.json`。

`cmd/rag-eval` 输出 `summary.json` 和 `report.md`，指标包括：

- `Recall@5` / `Recall@10`：相关题是否进入前 K 个召回结果。
- `MRR@K`：第一个相关题越靠前分数越高。
- `nDCG@K`：多个相关题按排序位置折损后的质量。
- `empty_rate`：没有任何召回结果的比例。
- `fallback_rate`：embedding/retriever 失败导致降级的比例。
- `avg_latency_ms` / `p95_latency_ms`：离线检索耗时。

`cmd/rag-eval` 支持可选全局质量门槛：`-min-recall-at-5`、`-min-recall-at-10`、`-min-mrr-at-k`、`-min-ndcg-at-k`。默认 0 表示只统计不拦截；显式设置后，低于门槛会在 `summary.json` 写入 `gate_failures` 并以非 0 退出。

`summary.json.groups` 按 `skill:*` 和 `tag:*` 输出分组召回质量；分组质量门槛 flags 为 `-min-group-cases`、`-min-group-recall-at-5`、`-min-group-recall-at-10`、`-min-group-mrr-at-k`、`-min-group-ndcg-at-k`。`-min-group-cases` 为 0 时分组门槛不启用；设置为正数后，`worst_groups` 会列出达到该 case 数且低于分组阈值的组，避免小样本误伤。

`cmd/questionbank-lint` 用于暴露题库元数据质量，不自动修数据。`mingw32-make questionbank-lint` 使用当前 seed 基线阈值，保证本地回归能通过；`mingw32-make questionbank-lint-strict` 使用目标阈值，要求每题至少 3 个 expected points，且 `scenario` 覆盖率不低于 80%。当前 seed 已补齐核心元数据，strict 目标应保持通过；后续扩题时如果失败，说明新增题没有按同一结构治理。

`cmd/eval` 读取 `testdata/eval/` 下的 profile、scoring、report fixture，做结构一致性和报告引用关系检查。scoring fixture 可声明 `cases`，输出 `scoring_range_hit_rate`、`expected_point_hit_rate`、`expected_miss_hit_rate`，用于检查分数区间和命中/缺失要点是否符合金标。默认 `mock` 模式只做稳定离线回归；real LLM 评估需要人工设置 API key 后手动跑，不进入默认 CI。

`/metrics` 继续保持无 SDK 的 Prometheus 文本输出，新增/补齐的口径包括：

- HTTP request duration histogram：`interview_http_request_duration_seconds_bucket`
- Graph node duration histogram：`interview_graph_node_duration_seconds_bucket`
- LLM call duration histogram：`interview_llm_call_duration_seconds_bucket`
- RAG retrieval：`interview_rag_retrieve_total`、`interview_rag_retrieve_duration_seconds_bucket`、`interview_rag_candidates_total`、`interview_rag_empty_total`、`interview_rag_fallback_total`
- Event hub counters：`interview_event_hub_publish_errors_total`、`interview_event_hub_dropped_events_total`

压测入口：

```powershell
$env:BASE_URL="http://127.0.0.1:8080"
mingw32-make load-test
```

`mingw32-make load-test` 会创建 `tmp/chaos/<timestamp>/summary.json`。k6 summary 统一输出 `status`、`checks`、`failures`、`sessions_started`、`answers_completed`、`http_req_failed_rate`、`http_req_duration_p95_ms`、`sse_first_packet_p95_ms` 等字段。SSE 是长连接，k6 会把按 timeout 结束的 stream 请求计入 `http_req_failed_rate`，所以该字段只做观测；通过/失败门禁看 503 率、409 背压率、answer 完成数和 SSE 首包耗时。默认目标是 `K6_TARGET_USERS=1000`，本地小规模验证可先设置：

```powershell
$env:K6_TARGET_USERS="5"
$env:K6_RAMP_UP="10s"
$env:K6_HOLD="20s"
$env:K6_RAMP_DOWN="5s"
mingw32-make load-test
```

Chaos smoke 入口：

```powershell
mingw32-make chaos-dry-run
./scripts/chaos_redis_restart.ps1
./scripts/chaos_pg_restart.ps1
```

`mingw32-make chaos-dry-run` 只验证脚本和 summary 结构，不执行 Docker restart，也不访问 `/readyz`。两个真实 chaos 脚本会把摘要写到 `tmp/chaos/<timestamp>/summary.json`，字段包括 `status`、`duration_ms`、`recovery_ms`、`checks`、`failures`、`dry_run`。真实脚本会重启 Docker Compose 里的 Redis/PostgreSQL，只应在本地或测试环境运行。

## 快速启动

```powershell
# 1. 拉取依赖
go mod tidy

# 2. 启动 Web 服务（mock LLM，无外部依赖）
mingw32-make demo-web

# 3. 打开浏览器
# http://localhost:8080

# 4. 基础健康检查
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
curl http://localhost:8080/api/ping
```

Web 页面支持直接输入岗位 JD 和候选人简历，也可以点击“读取简历文档”上传 `.txt` / `.md` / `.markdown` / `.pdf` / `.docx`。开始面试后，页面会展示 JD-简历匹配分、命中/缺失要求、风险点、简历优化建议和项目追问计划；最终报告会展示回答质量诊断和下一轮训练计划，训练计划会优先关联本轮 RAG 候选池里的题库题 ID。面试过程中继续展示 SSE 事件时间线、等待状态、当前题、历史问答和底部回答输入栏。

真实 LLM Web 模式：

```powershell
# API key 推荐通过环境变量注入；也支持本地未入仓 YAML fallback，环境变量优先级更高。
$env:INTERVIEW_LLM_API_KEY="sk-xxx"
mingw32-make demo-web-real
```

Windows / PowerShell 可用等价命令：

```powershell
$env:INTERVIEW_LLM_API_KEY="sk-xxx"
$env:INTERVIEW_LLM_MODE="real"
$env:INTERVIEW_EMBEDDING_MODE="mock"
go run ./cmd/server -config config/config.yaml.example
```

## Docker

应用镜像会先构建 Web 静态资源，再把 Go server 编译成单二进制。镜像不包含 `.env`、本地 `config/config.yaml` 或 API key；真实 LLM / embedding 只能通过运行时环境变量注入。

```powershell
docker build -t interview-agent:local .
```

只启动本地依赖：

```powershell
docker compose up -d postgres redis
```

启动三实例 mock 集群和 nginx 轮询入口：

```powershell
docker compose --profile cluster up -d --build
Invoke-RestMethod http://127.0.0.1:8080/healthz
Invoke-RestMethod http://127.0.0.1:8080/readyz
```

Web 演示入口是 `http://127.0.0.1:8080`。compose 默认使用 `INTERVIEW_LLM_MODE=mock` 和 `INTERVIEW_EMBEDDING_MODE=mock`，同时连接容器内 PostgreSQL/pgvector 与 Redis；如果切真实模式，只改运行时环境变量，不要把密钥写入镜像或仓库。

## 真实完整演示

真实完整演示会跑 **PostgreSQL + pgvector + Redis Streams + real LLM + real embedding**，覆盖 CLI 和 Web/SSE 两个入口：

```powershell
$env:INTERVIEW_LLM_API_KEY="sk-xxx"
$env:INTERVIEW_EMBEDDING_API_KEY="dummy"
$env:INTERVIEW_EMBEDDING_BASE_URL="http://127.0.0.1:8000/v1"
$env:INTERVIEW_EMBEDDING_MODEL="BAAI/bge-m3"
$env:INTERVIEW_EMBEDDING_DIMENSION="1024"
$env:INTERVIEW_POSTGRES_DSN="postgres://interview:interview@localhost:5432/interview?sslmode=disable"
$env:INTERVIEW_REDIS_URL="redis://localhost:6379/0"

./scripts/real_e2e.ps1
```

默认真实演示使用本地 OpenAI-compatible embedding 服务，启动方式见 `tools/bge_server/README.md`。如果改用云端 embedding，把 `INTERVIEW_EMBEDDING_BASE_URL`、`INTERVIEW_EMBEDDING_MODEL` 和 `INTERVIEW_EMBEDDING_API_KEY` 换成对应 provider 的值。

脚本会依次执行：

1. `docker compose up -d postgres redis`
2. 应用 `001` 到 `008` 全部 migration。
3. 运行 `cmd/reindex`，用 real embedding 写入 `question_bank.embedding`。
4. 校验 active 且 embedded 的题目数量大于 0。
5. 运行 `cmd/demo`，产出 `tmp/demos/real-*/run.json` 和 `report.md`。
6. 在 `http://127.0.0.1:18080` 启动真实 server，跑 Web/SSE smoke，验证 start、answer、SSE snapshot/progress、completed session detail、report 和 sessions list。

关键验收点：

- `run.json.config.retriever` 必须是 `pgvector`，不能是 fallback。
- `run.json.session.report` 必须包含 `overall_score`、`skill_breakdown`、`transcript_analysis` 和 `drill_plan`。
- `run.json.llm_calls` 和 `run.json.nodes` 必须非空，节点 timeline 至少覆盖 setup、RAG、选题和报告。

如果只想验证 CLI 真实 Agent 链路，不启动 Web 服务：

```powershell
./scripts/real_e2e.ps1 -SkipWeb
```

如果 Docker 里的 PG/Redis 已经启动：

```powershell
./scripts/real_e2e.ps1 -SkipDocker
```

也可以通过 Makefile 调用：

```powershell
mingw32-make demo-real-full
```

`mingw32-make demo` 会构建并启动本地服务，然后检查 `/healthz`、`/readyz`、`/api/ping`，并用 mock 模式跑一轮 `interview/start`、`sessions/:id`、`interview/answer`、`sessions?user_id=...`。

核心回归可用：

```powershell
mingw32-make test-core
```

结构化端到端 demo 可直接跑 CLI，不启 HTTP，也不依赖 Redis。设置 `INTERVIEW_POSTGRES_DSN` 时会使用 PG/pgvector 题库；未设置时会降级到 fallback 题库，`run.json.config.retriever` 会明确记录实际路径：

```powershell
$env:INTERVIEW_POSTGRES_DSN="postgres://interview:interview@localhost:5432/interview?sslmode=disable"
go run ./cmd/demo -config config/config.yaml -script testdata/demo/example.yaml
```

如果服务已经手动启动，可只跑 HTTP 检查：

```powershell
$env:USE_EXISTING_SERVER="1"
./scripts/smoke.ps1
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

```powershell
$env:INTERVIEW_POSTGRES_DSN="postgres://interview:interview@localhost:5432/interview?sslmode=disable"
mingw32-make demo-pg
```

如果还没迁移和 seed：

```powershell
$env:INTERVIEW_POSTGRES_DSN="postgres://interview:interview@localhost:5432/interview?sslmode=disable"
mingw32-make demo-pg-full
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

1. OTel tracing：贯穿 HTTP → Graph → LLM / Parser / Retriever，支持按 `trace_id` 定位慢节点。
2. 可靠性演示：熔断器半开期渐进放行、Chaos 故障注入脚本和恢复验证。
3. 工程化：GitHub Actions CI 矩阵、Docker 镜像、k8s Helm chart 和 probes。
4. 产品增强：`preview_plan` 题单预览、候选人档案复用、项目背景库和简化版 RAGAS 指标。

## 测试

提交前本地门禁：

```powershell
mingw32-make verify-local
```

该命令只依赖 mock/default 路径，不需要 PG、Redis、Docker 或 API key；CI 使用同一组无外部依赖检查。

```powershell
# 全量测试
mingw32-make test

# 关键回归
go test ./internal/nodes/ -run AgentLoop -v -count=1
go test ./internal/graphs -v -count=1
go test ./internal/httpapi -v -count=1
```

需要 PG 的 pgvector / session store 集成测试受 `INTEGRATION=1` + `INTERVIEW_POSTGRES_DSN` 控制；旧 `PG_DSN` 仍兼容。

Redis Streams 集成测试受 `INTEGRATION=1` + `INTERVIEW_REDIS_URL` 控制：

```powershell
$env:INTERVIEW_REDIS_URL="redis://localhost:6379/0"
$env:INTEGRATION="1"
go test ./internal/httpapi -run TestRedisInterviewEventHub_IntegrationPublishSubscribe -count=1 -v
```

Redis session coordinator 集成测试：

```powershell
$env:INTERVIEW_REDIS_URL="redis://localhost:6379/0"
$env:INTEGRATION="1"
go test ./internal/httpapi -run TestRedisSessionCoordinator -count=1 -v
```

## 数据库与题库

```powershell
# 启动 PG + Redis
mingw32-make docker-up

# 设置 DSN 后迁移
$env:INTERVIEW_POSTGRES_DSN="postgres://interview:interview@localhost:5432/interview?sslmode=disable"
mingw32-make migrate-up

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
