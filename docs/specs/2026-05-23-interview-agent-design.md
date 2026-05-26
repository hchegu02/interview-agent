# 智能面试评估系统 — 设计文档

> 日期：2026-05-23  
> 状态：实现中，已完成核心 Agent 闭环  
> 对应项目：基于自研轻量 Graph 的智能面试评估系统

---

## 1. 项目目标

围绕岗位画像、个性化出题、动态评估、复盘报告四个能力，构建一个从 JD/简历输入到模拟面试再到结构化报告的训练系统。

当前不做：用户系统、支付、多租户、完整 SaaS 后台、Web 前端。

## 2. 当前实现状态

已经完成：

| 能力 | 状态 | 主要位置 |
|---|---|---|
| 配置、日志、HTTP 基础骨架 | 已完成 | `internal/config/`, `internal/httpapi/`, `cmd/server/` |
| PDF / DOCX 解析 | 已完成 | `internal/parser/` |
| 自研 Graph 框架 | 已完成 | `internal/graph/` |
| Setup 节点 | 已完成 | `internal/nodes/parse_*.go`, `gap_analyze.go`, `retrieve_rag.go` |
| Agent loop 节点 | 已完成 | `internal/nodes/` |
| Report 节点 | 已完成 | `internal/nodes/report.go` |
| 整图组装 | 已完成 | `internal/graphs/interview.go` |
| pgvector Retriever | 已完成 | `internal/retriever/` |
| 题库 reindex CLI | 已完成 | `cmd/reindex/` |
| 最小 HTTP API | 已完成 | `POST /api/interview/start`, `POST /api/interview/answer` |
| 会话列表 API | 已完成 | `GET /api/interview/sessions?user_id=...` |
| 会话详情 API | 已完成 | `GET /api/interview/sessions/:session_id` |
| PG session store | 已完成最小实现 | `internal/httpapi/pg_session_store.go` |
| SSE 流式响应 | 已完成最小实现 | `GET /api/interview/stream?session_id=...`，支持 `Last-Event-ID` 回放和 heartbeat |
| Redis Streams 事件总线 | 已完成最小实现 | 设置 `INTERVIEW_REDIS_URL` 后启用 |
| Mock demo 响应 | 已完成 | `internal/llm/mock.go` |
| Smoke 脚本 | 已完成 | `scripts/smoke.sh` |

尚未完成：

| 能力 | 计划 |
|---|---|
| Redis session coordinator | 已接入 start/answer 主流程 |
| Redis session takeover | 已完成基础接管，冲突重试策略待完善 |
| Redis Streams 真实 smoke | 已完成 |
| 限流 / 熔断 / 背压 | 后续阶段 |
| PG session repository | 后续阶段 |
| 端到端真实 LLM demo | 后续阶段 |
| k6 压测 | 后续阶段 |

## 3. 架构总览

```text
Client
  |
  | HTTP
  v
Gin HTTP API
  |
  | start / answer
  v
InterviewService (memory store or PG sessions.state_json)
  |
  v
internal/graphs.BuildInterviewGraph
  |
  v
自研 Graph Runnable
  |
  +--> Setup: parse_jd -> parse_resume -> gap_analyze -> retrieve_rag
  |
  +--> Agent loop:
       pick_next -> suspend
       answer -> evaluate -> critic -> refine?
       -> probe_ask/probe_eval*
       -> update_memory -> reflection_check
       -> pick_next | report
```

外部依赖：

```text
LLM Provider        RealChatModel / MockChatModel
Embedding Provider  RealEmbedder / MockEmbedder
PostgreSQL+pgvector Shared PG pool -> PGVectorRetriever + PGSessionStore
Redis               Streams event hub implemented; session coordinator implemented; takeover, ratelimit planned
```

## 4. 目录结构

```text
cmd/
  server/       HTTP 服务入口
  reindex/      离线题库 embedding 重建
internal/
  config/       YAML + env 配置
  domain/       Session / AnswerRound / WorkingMemory / Report 等领域模型
  graph/        自研 Graph 框架，含 suspend/resume
  graphs/       业务图组装
  nodes/        setup + agent loop 全部节点
  llm/          ChatModel, Mock, Real, schema self-correction
  embedding/    Embedder, Mock, Real
  retriever/    pgvector Retriever + fusion
  parser/       PDF / DOCX parser
  httpapi/      Gin router + handlers
  observability/ slog logger
pkg/
  traceid/      trace id helper
migrations/     PG schema
seeds/          question bank JSON
scripts/        smoke / demo scripts
docs/           specs and handoff docs
```

## 5. 核心领域模型

当前模型以 `domain.Session` 为聚合根：

```go
type Session struct {
    ID              string
    UserID          string
    Status          SessionStatus
    CurrentNode     string
    JobProfile      *JobProfile
    CandProfile     *CandidateProfile
    GapReport       *GapReport
    CandidatePool   []Question
    WorkingMemory   *WorkingMemory
    Rounds          []AnswerRound
    PendingDecision *Decision
    Report          *Report
}
```

关键取舍：

- 不再使用 `Answers map` / `Evaluations map`，改成 `Rounds []AnswerRound` 保留时序。
- `AnswerRound.FinalEvaluation()` 优先返回 `RefinedEval`，再回退到初评。
- `WorkingMemory.SkillCoverage` 是 `map[string]float64`，使用归一化分数累加。
- `Score = -1` 暂时作为评估失败哨兵，下游统计必须跳过 `<0`。
- `WorkingMemory.ScoredRounds` / `DegradedRounds` 是强类型计数器。
- `WorkingMemory.ReflectTopic` 是 reflection_check 到 pick_next 的强类型补漏信号，pick_next 消费后清空。
- `WorkingMemory.DegradedReasons` 是强类型降级原因表，用稳定 component 名记录 fallback。
- `WorkingMemory.Notes` 不再承载主流程协议；当前只为旧 `reflect_topic` 状态保留兼容消费入口。
- `Report.NextSteps` 会汇总降级组件名，提示用户复测这些环节以提高报告可信度。
- `Session.MigrateLegacyState()` 会把旧 `Notes` 协议迁移到 typed fields，Graph `Invoke` / `Resume` 入口会自动调用。
- PG 相关命令和集成测试统一优先读取 `INTERVIEW_POSTGRES_DSN`，兼容旧 `PG_DSN`。

## 6. Graph 设计

项目使用自研轻量 Graph，而不是 Eino。核心 API：

```go
type NodeFunc func(ctx context.Context, sess *domain.Session) error
type Router func(sess *domain.Session) string
```

执行语义：

- `Graph` 注册节点、静态边和条件 router。
- `Runnable.Invoke` 从入口启动。
- 节点返回 `graph.ErrSuspended` 表示等待用户输入，不算失败。
- 外部填入答案后调用 `Runnable.Resume`，从 `sess.CurrentNode` 下游继续。
- Router 必须是纯函数，只返回下一节点名，不写状态。

当前整图入口在 `internal/graphs/interview.go`：

```text
parse_jd
  -> parse_resume
  -> gap_analyze
  -> retrieve_rag
  -> pick_next
```

Agent loop：

```text
pick_next -> evaluate -> critic
critic -> refine | probe_ask | update_memory
refine -> probe_ask | update_memory
probe_ask -> probe_eval -> probe_ask | update_memory
update_memory -> reflection_check -> pick_next | report
```

## 7. LLM 输出稳定性

所有 LLM 节点统一走：

```go
llm.CallWithSchema(ctx, model, messages, opts, validator, maxFixAttempts)
```

稳定性策略：

- 请求设置 `ResponseFormat = "json_object"`。
- 每个节点提供手写 validator。
- JSON 校验失败后，把错误反馈给 LLM 自纠一次。
- 节点内明确处理降级，不静默吞错。

Mock 模式：

- 优先读取 fixture。
- 没有 fixture 时，根据 prompt 类型返回一组内置 demo JSON。
- 未识别 prompt 仍返回 `ErrFixtureMissing`，避免测试误吞错误。

## 8. RAG 检索

`retrieve_rag` 节点流程：

```text
GapReport + JobProfile
  -> build query tags/text
  -> Embedder.Embed
  -> Retriever.Retrieve
  -> CandidatePool
```

`PGVectorRetriever` 使用两阶段检索：

- PG 端召回 vector candidates 和 tag candidates。
- Go 端使用 `LinearFusion` 融合 vector/topic/difficulty 三路分数。

没有 PG DSN 时，`cmd/server` 使用 `fallbackRetriever`，让 `retrieve_rag` 进入已有 fallback question bank。

## 9. HTTP API

当前最小 API：

```text
POST /api/interview/start
POST /api/interview/answer
GET  /api/interview/sessions?user_id=u1&limit=20
GET  /api/interview/sessions/demo-1
GET  /api/interview/sessions/demo-1?user_id=u1
```

`start` 请求：

```json
{
  "session_id": "demo-1",
  "user_id": "u1",
  "jd_text": "需要 Go 后端工程师",
  "resume_text": "两年 Go 开发经验"
}
```

`answer` 请求：

```json
{
  "session_id": "demo-1",
  "user_id": "u1",
  "answer": "G 是 goroutine，M 是线程，P 负责调度"
}
```

限制：

- 未配置 PG DSN 时 session store 是内存 map。
- 配置 PG DSN 后可保存到 `sessions.state_json`。
- `sessions` 按 `user_id` 返回会话摘要列表，底层走 store `ListByUser`。
- `sessions` 的 `limit` 默认 20，最大 100。
- `InterviewService` 维护 `CreatedAt` / 单调递增 `UpdatedAt`，列表按更新时间倒序。
- interview 响应返回 `created_at` / `updated_at`，供列表和恢复状态展示。
- `sessions/:id` 返回单个会话摘要，底层走 store `Get`；可选 `user_id` 会做归属匹配校验。
- `answer` 请求可选 `user_id`；传入时会做归属匹配校验。
- SSE 流式接口支持 snapshot、heartbeat 和 `Last-Event-ID` 回放。
- 设置 `INTERVIEW_REDIS_URL` 后，SSE 事件总线使用 Redis Streams。
- Redis session coordinator 已支持 snapshot save/load/delete 和 lease acquire/renew/release，并已接入 `InterviewService` 的 start/get/answer 主流程。
- 基础 takeover 已完成：本地 store miss 时可从 Redis snapshot 恢复；answer 遇到过期租约会重新 acquire；租约仍被其他 owner 持有时返回 HTTP 409，并带 `Retry-After` / `retry_after_seconds`。
- Redis snapshot 写入失败会降级为 `WorkingMemory.DegradedReasons["redis_snapshot"]`，不会打断 start/answer；lease acquire / renew 失败仍然阻断。
- Redis / memory event hub 会记录 publish error 和 dropped events；事件推送失败不阻断 interview 主流程。
- Real LLM 模式会套进程内并发 limiter，配置项为 `llm.max_concurrency`，默认 4；等待 limiter 时尊重 context。

## 10. 测试与验证

关键测试：

```bash
go test ./internal/nodes/ -run AgentLoop -v -count=1
go test ./internal/graphs -v -count=1
go test ./internal/httpapi -v -count=1
go test ./cmd/server -run BuildInterviewRunner -v -count=1
go test ./... -count=1
```

Smoke：

```bash
make demo
```

当前 smoke 覆盖 `/healthz`、`/readyz`、`/api/ping`、`interview/start`、`sessions/:id`、`interview/answer`、`sessions?user_id=...`。
如果服务已经手动启动，可用 `USE_EXISTING_SERVER=1 sh ./scripts/smoke.sh` 或 PowerShell `$env:USE_EXISTING_SERVER="1"; ./scripts/smoke.ps1` 只运行 HTTP 检查。
真实 PG smoke 可先设置 `INTERVIEW_POSTGRES_DSN`，再跑 `make demo-pg`。
完整本地路径可用 `make demo-pg-full`：起数据库、迁移、seed、再 smoke。
Redis Streams 集成测试可设置 `INTERVIEW_REDIS_URL=redis://localhost:6379/0 INTEGRATION=1`，再运行 `go test ./internal/httpapi -run TestRedisInterviewEventHub_IntegrationPublishSubscribe -count=1 -v`。
Redis session coordinator 集成测试可运行 `go test ./internal/httpapi -run TestRedisSessionCoordinator -count=1 -v`，同样需要 `INTERVIEW_REDIS_URL` 和 `INTEGRATION=1`。
Service + Redis coordinator snapshot 集成测试可运行 `go test ./internal/httpapi -run TestIntegration_InterviewService_RedisCoordinatorSnapshots -count=1 -v`。
基础 takeover 集成测试可运行 `go test ./internal/httpapi -run TestIntegration_InterviewService_TakeoverFromRedisSnapshot -count=1 -v`。
SSE 服务级 smoke 可运行 `./scripts/smoke_sse.ps1`；设置 `INTERVIEW_REDIS_URL=redis://localhost:6379/0` 后会验证 Redis Streams hub。

当前环境注意：Windows sandbox 偶发拦截 `gofmt`、`sh`、`go build`，报 `CreateProcessAsUserW failed: 1920`。Go 测试可正常运行。

## 11. 后续路线

推荐顺序：

1. 真实运行 `make demo`，确认 shell 环境下 smoke 可启动服务并跑完 start/answer。
2. 给 `scripts/smoke.sh` 做一次真实本机运行确认（当前 sandbox 会拦截 `sh`）。
3. 规划并实现 lease 冲突自动短重试策略。
4. 接限流、熔断、背压。
5. 做真实 LLM 端到端 demo。
6. 做 k6 压测和 README 演示截图。

## 12. 风险与缓解

| 风险 | 当前缓解 | 后续处理 |
|---|---|---|
| Mock 输出掩盖真实 LLM 行为 | 未识别 prompt 仍报 fixture missing | 增加 real LLM 手工 demo |
| 内存 session 丢失 | 当前仅用于最小闭环 | PG/Redis 持久化 |
| `Score = -1` 污染统计 | 统计前跳过 `<0` | 改成 typed evaluation status |
| `WorkingMemory.Notes` 被重新塞进协议 | 长期规则禁止新增主流程 key | 优先建 typed fields |
| 多实例不共享状态 | 暂不声明 HA | Redis snapshot + Streams |

---

**END OF DESIGN DOC**
