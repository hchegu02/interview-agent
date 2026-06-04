# Interview Agent

> 基于 Go + Agent Graph + RAG 的智能模拟面试系统。

Interview Agent 面向求职面试训练场景：用户输入岗位 JD 和简历后，系统自动分析岗位要求与候选人画像，结合题库检索生成个性化面试问题，并在多轮问答中完成追问、评分、记忆更新和复盘报告。

本项目的重点不是简单调用大模型生成问题，而是把模拟面试拆成可恢复的 Agent Graph，并围绕 RAG、会话状态、流式交互、题库治理、质量评估和工程可靠性做完整后端实现。

## 核心能力

- **Agent Graph 编排**：将 JD 解析、简历分析、匹配分析、RAG 召回、出题、评估、追问、记忆更新和报告生成拆成独立节点，支持暂停和恢复。
- **个性化出题**：根据 JD 技能、简历项目、能力缺口和题库过滤条件构造检索 query，生成候选题池。
- **多路 RAG 检索**：支持 pgvector、BM25、规则召回、RRF 融合和本地 rerank，并保留 retrieval trace。
- **多轮面试闭环**：支持主题题、追问、答案评分、critic、refine、reflection 和最终报告。
- **会话与流式交互**：提供 REST API、SSE 事件流、历史会话查询和前端页面。
- **存储与恢复**：本地默认内存模式；配置 PostgreSQL 后持久化 session 和题库；配置 Redis 后启用 Streams、snapshot 和 lease。
- **可靠性保护**：LLM 调用支持并发 limiter、熔断器、超时、降级和 `/readyz` degraded 状态。
- **质量评估**：提供 RAG eval、questionbank lint、mock eval，以及 Agent Tooling verifier 原语。

## 架构概览

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
  +-- internal/agentkit      Skill、Hook、Tool/MCP、Verification 原语
  +-- internal/llm           mock/real ChatModel、限流、熔断、调用记录
  +-- internal/embedding     mock/real embedding
  +-- internal/retriever     pgvector、BM25、规则召回、RRF、rerank
  +-- internal/questionbank  题库存储、导入、审核、commit
  +-- internal/parser        PDF/DOCX/TXT/Markdown 简历解析
  +-- internal/config        YAML + 环境变量配置
  +-- internal/observability 日志、trace id、graph/LLM 指标回调
```

核心流程：

```text
JD + 简历
  -> parse_jd
  -> parse_resume
  -> gap_analyze
  -> analyze_profile
  -> retrieve_rag
  -> pick_next
  -> evaluate / critic / refine / probe
  -> update_memory
  -> reflection_check
  -> report
```

`internal/graph` 使用 frontier-based runner。节点返回 `ErrSuspended` 时，Graph 暂停并记录 `Session.CurrentNode`；用户提交答案后，服务从断点恢复执行。

## RAG 设计

RAG 用于回答“该问什么”，LLM 用于理解、评分、追问和总结。

检索链路：

```text
query embedding
  -> vector recall
  -> BM25 recall
  -> rule recall
  -> RRF fusion
  -> local rerank
  -> candidate pool
```

默认 seed 模式和 PostgreSQL 模式都走多路 pipeline：

| 能力 | 默认本地模式 | 外部依赖模式 |
|---|---|---|
| 题库 | `seeds/question_bank.json` | PostgreSQL `question_bank` |
| 向量召回 | mock embedding + seed retriever | PostgreSQL + pgvector |
| BM25 / rule | seed 题库构建 | 启动时从 PG active 题库构建 |
| rerank | 本地 lexical reranker | 本地 lexical reranker |

检索失败不会直接中断面试。系统会回退到 fallback 题，并在 `WorkingMemory.DegradedReasons["rag"]` 和 hook event 中记录降级原因。

## Agent Tooling

项目保留 Interview Graph 作为业务执行主线，并增加轻量 Agent Tooling 层：

- `Skill Registry`：描述 `jd.analyze`、`resume.parse`、`question.retrieve`、`answer.evaluate`、`report.generate` 等能力。
- `Hooks`：在关键节点执行前后记录 skill 名称、输入摘要、输出摘要、耗时、错误和权限。
- `Tool Registry`：统一工具调用边界，处理权限、超时、结构化错误和审计事件。
- `MCP Client Adapter`：提供 MCP 工具调用抽象和 mock/local adapter，不强依赖真实外部 MCP 服务。
- `Verification`：提供 report、retrieval trace、tool event 的 verifier 原语，后续可接入质量门禁。

这不是通用 Coding Agent 平台，也没有实现完整 OpenClaw Gateway、本地 daemon 或容器 Sandbox；当前 Agent Tooling 是围绕模拟面试业务收口的工程能力层。

## HTTP API

核心接口：

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/healthz` | 存活检查 |
| `GET` | `/readyz` | 就绪检查，LLM breaker degraded 时会体现状态 |
| `GET` | `/metrics` | Prometheus 文本指标 |
| `POST` | `/api/interview/start` | 创建面试并推进到首题 |
| `POST` | `/api/interview/answer` | 提交答案并恢复 Graph |
| `GET` | `/api/interview/stream?session_id=...` | SSE 事件流 |
| `GET` | `/api/interview/sessions` | 会话列表 |
| `GET` | `/api/interview/sessions/:session_id` | 会话详情 |
| `POST` | `/api/documents/parse-resume` | 解析 PDF/DOCX/TXT/Markdown 简历 |
| `GET` | `/api/question-bank` | 题库查询 |
| `GET` | `/api/question-bank/:id` | 题目详情 |
| `GET` | `/api/question-bank/facets` | 题库筛选项 |

## 快速启动

前提：

- Go 1.23+
- Node.js 18+
- Windows 推荐 PowerShell 7
- 可选：PostgreSQL + pgvector、Redis、Docker Compose

安装前端依赖并构建：

```powershell
cd web
npm install
npm run build
cd ..
```

启动 mock 服务：

```powershell
go run ./cmd/server -config config/config.yaml.example
```

访问：

```text
http://127.0.0.1:8080
```

本地 smoke：

```powershell
.\scripts\smoke.ps1
.\scripts\smoke_sse.ps1
```

如果本机安装了 `mingw32-make`，也可以使用：

```powershell
mingw32-make demo
mingw32-make demo-mock
```

### Web 前端

前端位于 `web/`，使用 React + Vite。开发时可以单独启动前端，也可以构建后由 Go 服务通过 `internal/httpapi/web_assets.go` 暴露静态资源。

```powershell
cd web
npm install
npm run dev
```

通过 Makefile 运行 Web 演示：

```powershell
mingw32-make demo-web
```

有 GNU Make 的环境可以使用等价命令：

```powershell
make demo-web
```

Web 页面支持输入 JD、粘贴简历、上传简历文档、查看题库预览、订阅 SSE 事件流和查看最终报告。简历解析接口为 `POST /api/documents/parse-resume`。

### 真实完整演示

真实链路会使用 PostgreSQL/pgvector、Redis、real LLM 和 real embedding。运行前需要准备以下环境变量：

```powershell
$env:INTERVIEW_POSTGRES_DSN="postgres://interview:interview@localhost:5432/interview?sslmode=disable"
$env:INTERVIEW_REDIS_URL="redis://localhost:6379/0"
$env:INTERVIEW_LLM_API_KEY="sk-..."
$env:INTERVIEW_LLM_BASE_URL="https://..."
$env:INTERVIEW_EMBEDDING_API_KEY="sk-..."
$env:INTERVIEW_EMBEDDING_BASE_URL="https://..."
```

真实端到端脚本：

```powershell
.\scripts\real_e2e.ps1
```

脚本路径：`scripts/real_e2e.ps1`。

Makefile 入口：

```powershell
mingw32-make demo-real-full
mingw32-make demo-web-real
```

脚本会执行题库 reindex、启动服务、运行 demo，并检查 `run.json.config.retriever`、`report`、`llm_calls` 和 `nodes` 等字段，确认 real RAG 与 Agent 链路实际跑通。

## 配置

配置文件示例位于：

```text
config/config.yaml.example
```

常用环境变量：

| 环境变量 | 说明 |
|---|---|
| `INTERVIEW_POSTGRES_DSN` | PostgreSQL DSN；为空时使用内存 session store 和 seed 题库 |
| `INTERVIEW_REDIS_URL` | Redis URL；为空时使用内存事件总线 |
| `INTERVIEW_LLM_API_KEY` | real LLM 模式使用的 API key |
| `INTERVIEW_LLM_BASE_URL` | OpenAI-compatible LLM base URL |
| `INTERVIEW_EMBEDDING_API_KEY` | real embedding 模式使用的 API key |
| `INTERVIEW_EMBEDDING_BASE_URL` | OpenAI-compatible embedding base URL |

不要把 `.env`、token、API key 或私有配置提交到 Git。

## Observability

`GET /metrics` 暴露 Prometheus metrics 文本指标，覆盖 HTTP、SSE、Graph 节点、LLM 调用、RAG 检索、熔断器状态和背压计数。

OTel tracing 后端尚未接入；当前已有 trace id 和 Graph/LLM 指标回调，后续可扩展到 HTTP -> Graph -> LLM/Retriever/Parser 的完整链路追踪。

## Docker

启动依赖：

```powershell
docker compose up -d
```

数据库迁移 SQL 在 `migrations/` 下。题库 seed 位于 `seeds/question_bank.json`。

## 验证

Go 全量测试：

```powershell
go test ./... -count=1
```

RAG 硬门槛：

```powershell
go run ./cmd/rag-eval `
  -cases testdata/rag/golden_queries.jsonl `
  -config config/config.yaml.example `
  -out tmp/eval/rag `
  -min-recall-at-5 0.70 `
  -min-recall-at-10 0.80 `
  -min-mrr-at-k 0.90 `
  -min-ndcg-at-k 0.75 `
  -min-group-cases 3 `
  -min-group-recall-at-5 0.50 `
  -min-stage-recall-at-5 vector=0.70,bm25=0.65,rule=0.60,rrf=0.75,rerank=0.70 `
  -min-stage-mrr-at-k rrf=0.88,rerank=0.90
```

题库质量：

```powershell
go run ./cmd/questionbank-lint -seed seeds/question_bank.json -min-expected-points 3 -min-scenario-ratio 0.8
```

mock 报告评估：

```powershell
go run ./cmd/eval -suite testdata/eval -mode mock -out tmp/eval/mock
```

生成的 `tmp/eval/*` 是临时评估输出，可在验证后删除。

## 目录说明

| 路径 | 说明 |
|---|---|
| `cmd/server` | HTTP 服务入口 |
| `cmd/demo` | CLI demo |
| `cmd/rag-eval` | RAG 离线评估 |
| `cmd/questionbank-lint` | 题库质量检查 |
| `cmd/eval` | mock/real 报告评估 |
| `internal/domain` | 领域模型和 Session 聚合根 |
| `internal/graph` | 通用 Graph runner |
| `internal/graphs` | 面试 Graph 装配 |
| `internal/nodes` | 具体 Agent 节点 |
| `internal/agentkit` | Skill、Hook、Tool/MCP、Verification 原语 |
| `internal/httpapi` | Gin API、SSE、metrics、session service |
| `internal/retriever` | RAG 检索 pipeline |
| `internal/questionbank` | 题库存储、导入、审核和提交 |
| `internal/parser` | 简历文档解析 |
| `web` | React + Vite 前端 |
| `migrations` | PostgreSQL schema 迁移 |
| `deploy/helm` | 可选 Helm chart |
| `scripts` | smoke、e2e、chaos 脚本 |
| `testdata` | RAG 和 eval fixture |

## 当前边界

- MCP 当前是 client adapter 抽象和测试替身，没有接入真实生产 MCP Server。
- Verification 当前是 verifier 原语，尚未接入独立 CI gate。
- rerank 当前是本地 lexical reranker，不是本地深度学习 rerank 模型。
- PG 模式下 BM25/rule 本地阶段在服务启动时从 active 题库加载，题库运行时变更后的热刷新仍需后续完善。
- OTel tracing 后端和真实业务规模压测报告仍未完成。

## 安全说明

- 不提交 `.env`、密钥、token 或私有配置。
- LLM、embedding、工具调用均应设置超时和降级策略。
- Redis lease 失败会阻断写入，避免多实例同时修改同一 session。
- Redis snapshot 失败只记录降级，不中断主流程。

## License

本仓库当前未声明开源许可证。未经作者允许，请勿将代码用于商业发布。
