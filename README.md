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

仍未完成：OTel tracing 后端接入、题单预览节点、真实业务规模的 RAG 评估集和长期压测报告；Prometheus 仍使用本项目轻量文本渲染器，没有引入 Prometheus SDK。

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
  +-- internal/retriever     pgvector、BM25、规则召回、RRF 融合和 rerank
  +-- internal/questionbank  题库存储、导入、审核、commit
  +-- internal/parser        PDF/DOCX/TXT/Markdown 简历解析
  +-- internal/config        YAML + 环境变量配置
  +-- internal/observability 日志、trace id、graph/LLM 指标回调
```

前端构建产物会被 Go 服务通过 `internal/httpapi/web_assets.go` 暴露；开发和演示入口仍以 `cmd/server` 为准。

### 运行时依赖装配

`cmd/server/main.go` 是唯一服务入口，启动时按配置装配依赖：

- `config.Load` 读取 `config/config.yaml.example` 或指定 YAML，并用环境变量覆盖敏感项。
- `INTERVIEW_POSTGRES_DSN` 为空时使用内存 session store、内存题库 seed 和 fallback retriever；非空时连接 PostgreSQL/pgvector，启用 PG session store、PG 题库和 `pgvector_pipeline` 检索。
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
| RAG 检索 | seed 题库上的 vector + BM25 + rule + RRF + 本地 rerank pipeline | PGVector 作为 vector stage，PG 题库构建 BM25/rule，本地 rerank |
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

### Agent Tooling 设计

项目保留现有 Interview Graph 作为执行编排层，并新增轻量 Agent Tooling 抽象：

- Skill Registry：描述 JD 分析、简历解析、题库检索、答案评估和报告生成等可复用能力。
- Hooks：在关键节点执行前后记录输入摘要、输出摘要、耗时和错误，用于审计、观测和验证。
- Tool Registry / MCP Client：统一工具调用边界，处理权限、超时、结构化错误和调用审计。
- Verification Loop：提供结构化输出、检索 trace、工具调用和报告完整性的 verifier 原语；后续可接入 RAG eval、questionbank lint 和 mock eval 形成质量门禁。

这不是通用 Coding Agent 平台，也不实现完整 OpenClaw Gateway 或容器 Sandbox；它是围绕模拟面试业务收口的 Agent 工程能力层。

### 代码阅读路线

如果你只是想理解项目，不建议从前端或某个节点文件随机看起。按下面顺序读，能最快建立完整心智模型：

1. `cmd/server/main.go`：服务入口。看配置如何加载，Postgres、Redis、题库、事件总线、Graph runner 如何装配。
2. `cmd/server/interview_wiring.go`：面试核心依赖装配。看 ChatModel、Embedder、Retriever、Graph callback 如何接到 `BuildInterviewGraph`。
3. `internal/domain/`：核心数据结构。重点看 `Session`、`AnswerRound`、`WorkingMemory`、`Report`，它们决定 HTTP、PG、Redis snapshot 的共同数据格式。
4. `internal/graphs/interview.go`：Graph 拓扑。看节点执行顺序、暂停点和循环边界。
5. `internal/httpapi/interview_flow.go`：面试业务入口。`Start` 创建会话并推进首题，`Answer` 写入用户回答并从断点继续。
6. `internal/nodes/`：具体节点。先看 `pick_next`、`evaluate`、`critic`、`reflection_check`、`report`，再看 setup 节点。
7. `internal/questionbank/`：题库导入和提交。先看 `imports_parse.go`、`imports_stage.go`、`imports_commit.go`，再看 memory/PG store。
8. `cmd/rag-eval`、`cmd/questionbank-lint`、`cmd/eval`：工程质量门槛。这里决定 CI 如何发现 RAG、题库和报告结构退化。

当前几个核心打开文件的关系可以这样看：

- `cmd/server/main.go`：把配置、外部依赖、题库、Graph runner 和 `InterviewService` 装起来。
- `internal/httpapi/interview_handler.go`：HTTP 协议边界，只负责请求解析、参数校验和响应转换。
- `internal/httpapi/session_store.go`：会话持久化接口，内存和 PG 实现都必须遵守同一套读写语义。
- `internal/domain/session.go`：会话聚合根，HTTP、Redis snapshot、PG state_json 和报告都共享这份结构。

几个容易误解的边界：

- HTTP handler 只做协议转换和校验，不直接改面试状态；状态推进放在 `InterviewService` 和 Graph 节点里。
- `Session` 是聚合根，不要绕过它新增一套并行状态；否则 PG、Redis snapshot、SSE 和报告会读到不一致数据。
- Redis lease 是多实例写入所有权，失败要阻断；Redis snapshot 是恢复能力，失败只记录降级。
- RAG fallback 是未配置 PG/pgvector 时的明确降级路径，不是长期检索方案。
- `/metrics` 当前是轻量 Prometheus 文本渲染器，不依赖 Prometheus SDK；新增指标时要保持文本格式稳定。

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

本地 seed 模式会跑完整的多路检索 pipeline：`vector`、`bm25`、`rule` 三路召回，经过 `rrf` 融合，再用确定性的本地 `rerank` 精排。`summary.json.stages` 会分别输出每个阶段的 Recall/MRR/nDCG，`summary.json.stage_deltas` 会记录 `rrf_vs_vector` 和 `rerank_vs_rrf` 的变化。阶段门槛 flags 为 `-min-stage-recall-at-5` 和 `-min-stage-mrr-at-k`，格式是逗号分隔的 `stage=threshold`，例如：

```powershell
go run ./cmd/rag-eval `
  -cases testdata/rag/golden_queries.jsonl `
  -config config/config.yaml.example `
  -out tmp/eval/rag `
  -min-stage-recall-at-5 vector=0.70,bm25=0.65,rule=0.60,rrf=0.75,rerank=0.70 `
  -min-stage-mrr-at-k rrf=0.88,rerank=0.90
```

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

## Helm 部署

最小 Helm chart 位于 `deploy/helm/interview-agent`，默认使用 mock LLM 和 mock embedding，不需要 PG、Redis 或 API key：

```powershell
helm template interview-agent "deploy/helm/interview-agent"
```

chart 默认把 readiness probe 指向 `/readyz`，liveness probe 指向 `/healthz`。真实 LLM、PostgreSQL 和 Redis 只通过 values/env 注入，例如 `INTERVIEW_LLM_MODE=real`、`INTERVIEW_LLM_API_KEY`、`INTERVIEW_POSTGRES_DSN`、`INTERVIEW_REDIS_URL`；不要把密钥、DSN 或私有配置写入仓库。

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

## 目录与文件说明

本节只解释项目业务、运行、测试和部署相关文件。`.git/`、`node_modules/`、`.gocache/`、`tmp/`、`.tmp/`、`tools/*/.venv/`、各类编辑器/Agent 配置目录不属于业务架构，阅读项目时可以跳过。

### 根目录

| 路径 | 说明 |
|---|---|
| `README.md` | 项目说明、架构、启动方式、API、测试和目录索引。 |
| `Makefile` | 本地开发、测试、demo、评估、迁移、压测的统一入口。 |
| `Dockerfile` | Go 服务容器构建文件，包含前端构建产物嵌入服务的部署路径。 |
| `docker-compose.yml` | 本地 PostgreSQL、Redis、应用服务等依赖编排。 |
| `go.mod` / `go.sum` | Go 模块依赖声明和校验锁定。 |
| `package.json` / `package-lock.json` | 根级 npm 脚本和依赖锁定，主要服务于前端/工具链。 |
| `.env.example` | 环境变量示例，不包含真实密钥。 |
| `.dockerignore` / `.gitignore` | Docker build 和 Git 的忽略规则。 |
| `.golangci.yml` | Go 静态检查配置。 |
| `readme_makefile_test.go` | 校验 README 中的 Makefile 命令与实际目标保持一致。 |
| `CHANGELOG.md` | 变更记录。 |
| `HANDOFF.md` | 开发交接说明，记录阶段性上下文和未完成事项。 |
| `AGENT.md` / `CLAUDE.md` | 本地 Agent/模型协作约束，不是运行时代码。 |
| `skills-lock.json` | 技能/插件锁定信息，不参与业务运行。 |

### `cmd/`

| 路径 | 说明 |
|---|---|
| `cmd/server/main.go` | 服务主入口，加载配置并装配 HTTP、数据库、Redis、Graph、LLM、Embedding、题库等依赖。 |
| `cmd/server/deps.go` | 服务依赖结构和基础装配辅助。 |
| `cmd/server/interview_wiring.go` | 面试 Graph 相关依赖装配，把 ChatModel、Embedder、Retriever、Callback 接到 `BuildInterviewGraph`。 |
| `cmd/server/questionbank_wiring.go` | 题库存储、导入、检索相关依赖装配。 |
| `cmd/server/redis_wiring.go` | Redis event hub、session snapshot、lease coordinator 装配。 |
| `cmd/demo/` | 结构化 demo CLI，生成本地演示 JSON 和 Markdown 报告。 |
| `cmd/demo/main.go` | demo 命令入口。 |
| `cmd/demo/script.go` | demo 流程脚本。 |
| `cmd/demo/answer.go` | demo 回答数据和交互模拟。 |
| `cmd/demo/output.go` | demo 输出文件生成。 |
| `cmd/rag-eval/main.go` | 离线 RAG 召回评估入口，输出 recall、MRR、nDCG、空召回率等指标。 |
| `cmd/questionbank-lint/main.go` | 题库质量检查入口，检查 expected points、scenario、metadata 覆盖。 |
| `cmd/eval/main.go` | 离线评估入口，用 fixture 检查评分、报告结构和引用关系。 |
| `cmd/reindex/main.go` | 题库重建和 embedding 刷新工具。 |
| `*_test.go` | 对应命令的回归测试。 |

### `internal/domain/`

| 路径 | 说明 |
|---|---|
| `internal/domain/session.go` | `Session` 聚合根、`AnswerRound`、画像、报告、题目、评估等核心业务结构。 |
| `internal/domain/agent.go` | Agent 决策、WorkingMemory、Critic、probe/reflection 相关状态。 |
| `internal/domain/migration.go` | 旧状态迁移和兼容逻辑，保证历史 session 可读。 |
| `internal/domain/session_test.go` | 领域模型和迁移行为测试。 |

### `internal/graph/`

| 路径 | 说明 |
|---|---|
| `internal/graph/node.go` | `NodeFunc`、`Router`、`Callback`、`ErrSuspended`、`ErrPermanent` 等 Graph 基础类型和错误。 |
| `internal/graph/graph.go` | `Graph`、`Runnable`、`Invoke`、`Resume`、frontier 执行器、边/路由计算。 |
| `internal/graph/decorators.go` | 节点 timeout、retry、装饰器组合等横切能力。 |
| `internal/graph/graph_test.go` | Graph 编译、执行、暂停恢复、路由、callback、循环保护测试。 |

### `internal/graphs/`

| 路径 | 说明 |
|---|---|
| `internal/graphs/interview.go` | 真实面试 Graph 装配：setup 节点、RAG、出题、评估、critic、refine、probe、memory、reflection、report。 |
| `internal/graphs/interview_test.go` | 面试 Graph 组装和首轮暂停行为测试。 |

### `internal/nodes/`

| 路径 | 说明 |
|---|---|
| `doc.go` | nodes 包说明。 |
| `parse_jd.go` | 解析 JD，生成岗位画像。 |
| `parse_resume.go` | 解析简历，生成候选人画像。 |
| `gap_analyze.go` | 结合 JD 和简历生成差距分析。 |
| `analyze_profile.go` | 生成可解释的画像匹配分析。 |
| `retrieve_rag.go` | 调用 Embedder 和 Retriever 召回题库候选题。 |
| `pick_next.go` | 根据 WorkingMemory、候选池和决策选择下一题，出题后返回 `ErrSuspended` 等用户回答。 |
| `evaluate.go` | 对用户主回答评分，写入当前 round 的 evaluation。 |
| `critic.go` | 校验评分是否可靠，决定是否需要 refine 或 probe。 |
| `refine.go` | 根据 critic 反馈重评，写入 `RefinedEval`。 |
| `probe.go` | 生成追问、评估追问回答，并控制多轮追问预算。 |
| `update_memory.go` | 根据当前轮最终评估更新 WorkingMemory 的覆盖度、弱点和预算。 |
| `reflection_check.go` | 判断继续出题、反思补题还是结束生成报告。 |
| `report.go` | 聚合 rounds、critic、refine、probe、coverage 生成最终报告。 |
| `routers.go` | Graph 条件路由：`RouteAfterPickNext`、`RouteAfterCritic`、`RouteAfterRefine`、`RouteAfterProbeEval`、`RouteAfterReflection`。 |
| `prompts.go` | 各 LLM 节点的 system/user prompt 模板。 |
| `degradation.go` | 节点降级原因写入 WorkingMemory 的辅助函数。 |
| `*_test.go` | 各节点和 agent loop 的行为回归测试。 |

### `internal/httpapi/`

| 路径 | 说明 |
|---|---|
| `doc.go` | httpapi 包说明。 |
| `router.go` | Gin 路由注册、健康检查、API 分组、背压中间件接入。 |
| `interview.go` | `InterviewService` 结构、runner/store/events 等依赖定义。 |
| `interview_handler.go` | `/api/interview/start` 和 `/api/interview/answer` HTTP handler。 |
| `interview_flow.go` | 面试主流程：Start 创建 session 并 Invoke，Answer 写答案并 Resume。 |
| `interview_session.go` | session 查询、订阅等 service 方法。 |
| `interview_response.go` | domain session 到 HTTP response 的转换。 |
| `interview_errors.go` | 面试相关错误到 HTTP 状态码的映射。 |
| `interview_stream.go` | SSE 流式接口，输出 snapshot、heartbeat 和事件流。 |
| `interview_events.go` | 事件模型、内存 event hub、Graph callback 事件发布。 |
| `redis_event_hub.go` | Redis Streams event hub 实现。 |
| `session_store.go` | session store 接口和内存实现。 |
| `pg_session_store.go` | PostgreSQL session store，把完整 session 写入 `sessions.state_json`。 |
| `redis_session_coordinator.go` | Redis session snapshot、lease、跨实例恢复协调。 |
| `interview_lease.go` | lease 获取、续约、释放和 snapshot 降级处理。 |
| `question_bank.go` | 题库查询、导入、审核、提交 API。 |
| `documents.go` | 简历文档解析 API。 |
| `profile_analysis.go` | JD/简历画像分析 API 响应转换。 |
| `middleware_trace.go` | trace id 注入中间件。 |
| `middleware_inflight.go` | HTTP 并发背压中间件。 |
| `metrics*.go` | HTTP、SSE、Graph、LLM、RAG、Prometheus 文本指标实现。 |
| `web_assets.go` | 前端构建产物嵌入和静态资源服务。 |
| `server_metrics.go` | 服务级指标聚合。 |
| `*_test.go` | HTTP、store、SSE、metrics、Redis/PG 集成行为测试。 |

### `internal/llm/`

| 路径 | 说明 |
|---|---|
| `chat_model.go` | ChatModel 接口和消息/选项/响应结构。 |
| `mock.go` | mock LLM，实现稳定本地测试和 demo。 |
| `real.go` | OpenAI-compatible real LLM 客户端。 |
| `factory.go` | 按配置创建 mock/real ChatModel。 |
| `limited.go` | 进程内 LLM 并发 limiter。 |
| `breaker.go` | LLM 熔断器，保护 real 模式下游不稳定。 |
| `recording.go` | LLM 调用记录包装，用于 metrics/token 统计。 |
| `tokens.go` | token 统计和累计结构。 |
| `schema.go` | LLM JSON schema/结构化输出辅助。 |
| `errors.go` | LLM 错误分类。 |
| `*_test.go` | mock、real、limiter、breaker、schema、token 统计测试。 |

### `internal/embedding/`

| 路径 | 说明 |
|---|---|
| `embedder.go` | Embedding 接口定义。 |
| `mock.go` | mock embedding，支持无外部依赖的本地检索。 |
| `real.go` | OpenAI-compatible embedding 客户端。 |
| `*_test.go` | embedding 客户端和 mock 行为测试。 |

### `internal/retriever/`

| 路径 | 说明 |
|---|---|
| `retriever.go` | Retriever 查询结构、结果结构和接口定义。 |
| `pgvector.go` | PostgreSQL + pgvector 向量召回实现，是线上 RAG pipeline 的 vector stage。 |
| `fusion.go` | 多路召回/排序融合逻辑。 |
| `aliases.go` | 技能、标签、别名归一化。 |
| `*_test.go` | pgvector、fusion、alias 行为测试。 |

### `internal/questionbank/`

| 路径 | 说明 |
|---|---|
| `store.go` | 题库 store 接口和内存实现。 |
| `pg_store.go` | PostgreSQL 题库存储实现。 |
| `errors.go` | 题库领域错误。 |
| `imports.go` | 题库导入 service 聚合入口。 |
| `imports_types.go` | import job、item、review status、file ref 等类型。 |
| `imports_parse.go` | 导入文件解析和题目抽取。 |
| `imports_stage.go` | 导入 item 暂存、校验、状态推进。 |
| `imports_commit.go` | 审核通过的 item 提交到正式题库。 |
| `imports_enrichment.go` | 导入题目的元数据补全和 embedding 处理。 |
| `imports_normalize.go` | 题库字段归一化。 |
| `imports_clone.go` | import 数据深拷贝，避免外部修改污染内部状态。 |
| `imports_id.go` | import job/item ID 生成。 |
| `imports_spool.go` | 本地导入文件暂存。 |
| `imports_memory_store.go` | 内存 import store。 |
| `imports_pg.go` | PostgreSQL import store。 |
| `imports_async.go` | 异步导入/提交任务封装。 |
| `*_test.go` | store、import、commit、解析和状态流测试。 |

### `internal/parser/`

| 路径 | 说明 |
|---|---|
| `parser.go` | 文档解析接口、Source、Hint、ParseLimit 等类型。 |
| `dispatcher.go` | 根据文件类型分发到具体 parser。 |
| `text.go` | TXT/Markdown 文本解析。 |
| `pdf.go` | PDF 解析。 |
| `docx.go` | DOCX 解析。 |
| `mock.go` | 测试用 mock parser。 |
| `parser_test.go` | 文档解析回归测试。 |

### `internal/config/`

| 路径 | 说明 |
|---|---|
| `config.go` | YAML + 环境变量配置加载、默认值和校验。 |
| `config_test.go` | 配置加载、环境变量覆盖和默认值测试。 |
| `marshal_test_helper.go` | 配置测试辅助。 |

### `internal/observability/`

| 路径 | 说明 |
|---|---|
| `logger.go` | 日志基础封装。 |
| `tracing.go` | trace id 上下文读写。 |
| `tracing_callback.go` | Graph 节点 tracing callback。 |
| `recording_callback.go` | Graph 节点耗时/错误记录 callback。 |
| `*_test.go` | tracing 和 recording callback 测试。 |

### `internal/migrations/`

| 路径 | 说明 |
|---|---|
| `migrations_test.go` | 校验迁移文件命名、顺序和 up/down 配对。 |

### `migrations/`

| 路径 | 说明 |
|---|---|
| `001_init.*.sql` | 初始表结构。 |
| `002_question_bank_expected_points.*.sql` | 题库 expected points 字段迁移。 |
| `003_question_bank_metadata.*.sql` | 题库 metadata 扩展。 |
| `004_question_bank_imports.*.sql` | 题库导入任务表。 |
| `005_question_bank_embedding_status.*.sql` | embedding 状态字段。 |
| `006_question_bank_import_lease.*.sql` | 导入任务 lease。 |
| `007_question_bank_import_review_status.*.sql` | 导入审核状态。 |
| `008_question_bank_import_field_provenance.*.sql` | 字段来源追踪。 |
| `009_question_bank_content_trgm.*.sql` | 题库文本 trigram 检索索引。 |
| `seed_question_bank.sql` | SQL 形式题库 seed。 |
| `README.md` | 数据库迁移说明。 |

### `config/`

| 路径 | 说明 |
|---|---|
| `config/config.yaml.example` | 可提交的配置模板。 |
| `config/config.yaml` | 本地真实配置，通常不应提交。 |

### `web/`

| 路径 | 说明 |
|---|---|
| `web/index.html` | Vite 入口 HTML。 |
| `web/package.json` / `web/package-lock.json` | 前端依赖和脚本。 |
| `web/vite.config.ts` | Vite 构建配置。 |
| `web/tsconfig.json` | TypeScript 配置。 |
| `web/src/main.tsx` | 前端入口。 |
| `web/src/apiClient.ts` | HTTP API client。 |
| `web/src/useInterviewStream.ts` | SSE 订阅 hook。 |
| `web/src/interviewView.ts` | 面试主界面逻辑。 |
| `web/src/candidatePages.tsx` | 候选人页面入口和组合。 |
| `web/src/questionBankPage.tsx` | 题库预览页面。 |
| `web/src/questionBankImportView.ts` | 题库导入/审核界面逻辑。 |
| `web/src/reportView.ts` | 报告展示逻辑。 |
| `web/src/sharedView.tsx` | 前端共享 UI 组件。 |
| `web/src/draftStore.ts` | 本地草稿保存。 |
| `web/src/routes.ts` | 前端路由。 |
| `web/src/types.ts` | 前端共享类型。 |
| `web/src/styles.css` | 全局样式。 |
| `web/src/*test*` | 前端 API、路由、视图和状态测试。 |

### 数据、脚本、部署和测试资源

| 路径 | 说明 |
|---|---|
| `seeds/` | 本地题库 seed 数据。 |
| `testdata/` | RAG、评估、fixture 等离线测试数据。 |
| `scripts/smoke.ps1` | Windows smoke 测试脚本。 |
| `scripts/smoke_sse.ps1` | SSE 路径 smoke 测试脚本。 |
| `scripts/e2e_smoke.ps1` | 端到端 smoke 脚本。 |
| `scripts/real_e2e.ps1` | real LLM/embedding 环境下的端到端脚本。 |
| `scripts/smoke.sh` | Bash smoke 脚本。 |
| `scripts/chaos_redis_restart.ps1` | Redis 重启混沌脚本。 |
| `scripts/chaos_pg_restart.ps1` | PostgreSQL 重启混沌脚本。 |
| `chaos/` | k6/混沌测试脚本和压测配置。 |
| `deploy/` | 部署配置，包含 Helm/Kubernetes 相关资源。 |
| `docs/` | 项目文档、设计记录和补充说明。 |
| `openspec/` | OpenSpec 变更规范和任务文档。 |
| `pkg/` | 可被外部复用的公共包；当前核心业务仍主要在 `internal/`。 |
| `tools/` | 本地辅助工具，例如 embedding 服务脚手架；其中虚拟环境和依赖缓存不属于业务源码。 |
| `bin/` | 本地构建产物或工具输出。 |
| `data/` | 本地运行数据。 |
| `CK/` | 本地检查/资料目录，不是服务运行主路径。 |

## 安全说明

- API key 推荐走环境变量；`config.Load` 也支持从本地未入仓 YAML 读取 fallback key，且环境变量优先级更高。
- `.env` 和 `config.yaml` 不入仓，`config/config.yaml.example` 不包含真实密钥。
- 示例 key 只使用 `sk-xxx` 占位。

## License

仅用于学习与求职演示。
