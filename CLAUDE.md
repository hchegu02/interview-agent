# CLAUDE.md — Interview Agent 长期协作规则

本文件给所有未来的 Claude / 协作者读。**只写长期有效的规则、架构约定、命令、测试方式、代码风格**。临时任务、进度、bug list 一律放在 `HANDOFF.md`。

---

## 项目概览

Go 写的智能面试评估系统，核心是一个**自研轻量决策图（Graph）框架**驱动的多节点 LLM Agent。两层子图：

```
setup 子图       parse_jd → parse_resume → gap_analyze → retrieve_rag
                                     ↓
agent 子图       pick_next ⇄ evaluate → critic → (refine?) → (probe_ask ↔ probe_eval)+
                                     ↓
                              update_memory → reflection_check → (loop or end)
                                     ↓
                                  report
```

支持 **Mock / Real 双模式**：LLM、Embedder 都有 mock 实现，所有节点单测都用 stub 驱动，**不依赖外部网络**。

---

## 仓库结构

```
cmd/
  server/       HTTP 入口
  reindex/      离线工具：重建题库 embedding
internal/
  config/       YAML + 环境变量配置加载与校验
  domain/       领域模型（Session / AnswerRound / WorkingMemory / Critic / Decision / JobProfile / CandidateProfile / Question 等）
  graph/        自研图框架（NodeFunc / Router / Runnable，含 suspend/resume）
  llm/          ChatModel 接口 + Real (OpenAI 兼容 HTTP) + Mock + schema 自校正 + TokenTracker
  embedding/    Embedder 接口 + Real (DashScope) + Mock
  retriever/    Retriever 接口 + pgvector + LinearFusion
  parser/       PDF / DOCX 简历解析
  nodes/        所有图节点 + prompt 模板 + router
  httpapi/      HTTP handler
  observability/  日志、metric、trace
migrations/      PG schema + seed
seeds/           题库 JSON
config/          YAML 模板
```

---

## 架构约定

### Graph 框架
- 节点签名固定：`type NodeFunc func(ctx context.Context, sess *domain.Session) error`。
- Router 签名固定：`type Router func(*domain.Session) string`，**纯函数，禁止副作用**。所有副作用（写 PendingDecision、Notes 等）必须在节点里完成。
- 挂起靠 `ErrSuspended` 哨兵 + `sess.CurrentNode`：节点 return `ErrSuspended` 后 runtime 暂停，外部填好用户输入再 `Resume`。
- 永久错误用 `ErrPermanent`（包内 helper）抛，会终止整张图；可重试错误正常返回。
- **不要往 graph 包加业务逻辑**，它只该懂"节点 / 边 / suspend"。

### 节点设计
- 每个节点一个文件，文件头写**节点契约**注释（输入 / 输出 / 返回值约定）。
- 节点内部分层：`run` 顶层 → `XxxByLLM`（调 LLM）→ `ruleBased Xxx`（降级 fallback）→ 各 helper。
- LLM 调用一律走 `llm.CallWithSchema(ctx, model, msgs, opts, validator, retries)`，配 schema validator 处理 hallucination。
- 失败降级**必须显式**：写 `WorkingMemory.DegradedReasons[component]=reason` + 走规则 fallback；禁止静默兜底。
- 预算约束**在节点内做硬约束**：probe / reflection / round 预算检查由节点拦截，router 不重判预算。

### Session 与状态
- `domain.Session` 是图运行的唯一状态载体。
- 通过 `sess.CurrentRound()` 取当前轮，节点不要自己 index `sess.Rounds[len-1]`。
- `WorkingMemory.SkillCoverage` 是 `map[string]float64`，**归一化加权累加**（`+= score/100`），不是计数。
- `WorkingMemory.ScoredRounds` / `DegradedRounds` 是强类型计数器；不要再放回 `Notes`。
- `WorkingMemory.ReflectTopic` 是 reflection_check → pick_next 的强类型补漏信号；pick_next 消费后必须清空。
- `WorkingMemory.DegradedReasons` 是降级原因表，key 使用稳定 component 名（如 `pick`、`eval`、`rag`）。
- `WorkingMemory.Notes` 只保留给旧状态兼容和极少数非核心元数据；不要新增主流程协议。当前代码只兼容消费旧 `Notes["reflect_topic"]`。
- `Score = -1` 是"评估失败"哨兵；任何统计前判 `< 0` 跳过。`Score = 0` 是"答得很差"，要计入。
- `AnswerRound.FinalEvaluation()` 在有 RefinedEval 时优先返回它；下游聚合用这个方法，不要直接读 `Evaluation`。

### Prompt
- 所有 prompt 模板放 `internal/nodes/prompts.go`，常量命名 `promptXxx`。
- 强制 `**只返回 JSON 对象**, 不要 markdown` + schema 字段列表 + 字段类型 + 一句话语义。
- enum 字段必须显式列出可选值。
- 优先抽**可被追问的事实**（量化指标 / 技术细节），不抽形容词。

### LLM 抽象
- `llm.ChatModel` 接口：`Chat(ctx, messages, opts) (*Response, error)`。
- Real / Mock / Stub 三实现，测试默认用 `stubChatModel`（按 `responses[]` 顺序返回）。
- API key **永远从环境变量取**，配置 struct 上敏感字段 `yaml:"-"`，YAML 配置不能含 key；`config.Validate()` 在 `mode=real` 缺 key 时必须 fail。

---

## 安全约定

- API key、DB password 等敏感字段：只接受环境变量，YAML 用 `yaml:"-"` 屏蔽。
- 不在 commit / log / 错误信息里打印 key。
- 如果对外文档需要给 key 示例，写 `sk-xxx` 占位。
- 任何人不慎把 key 提到 git，**立刻吊销**再换新 key，不要靠 force-push 清。

---

## 代码风格

- Go 1.25+，遵循 gofmt + goimports 默认风格，`golangci-lint run ./...` 必须过。
- 错误用 `fmt.Errorf("... : %w", err)` 包装；不裸 panic（除 `init` 阶段必要时）。
- 中文注释 + 英文标识符，注释优先解释 **why**，代码已经说了 **what**。
- 节点 / router / helper 一律包级函数，禁止包级可变状态（TokenTracker 例外，但要 thread-safe）。
- 测试文件就近放 `xxx_test.go`，集成类测试加 `_test` 包名时谨慎（当前都是同包）。
- 表驱动测试优先；多场景的集成测试用"预装 LLM 响应序列 + 多次 Resume"模式（见 `agent_loop_test.go`）。

---

## 测试约定

- 单测必须独立、可并行、无网络、无 PG（除明确标注的集成测试）。
- 节点测试用 `stubChatModel{responses: [...], errs: [...]}` 驱动，顺序硬编码 LLM 调用次序 — **改节点 LLM 调用次数前先数一遍受影响的测试**。
- `agent_loop_test.go` 是 agent 子图的集成测试，组装节点 + router 跑完整路径，是修改节点逻辑后最重要的回归套件。
- 用 PG 的测试（`internal/retriever/pgvector_test.go`）受 `INTERVIEW_POSTGRES_DSN` 控制，没 DSN 自动 skip，不要在没 PG 的机器上强跑。
- 命名约定：`TestXxx_Scenario`（例：`TestReflection_LLMReflect_NoBudget_Downgrades`）。

---

## 常用命令

```bash
# 构建
make build

# 启服务（开发）
make run

# 全量测试
make test

# 含 race detector
make test-race

# 单包
go test ./internal/nodes/ -v -count=1

# 单 scenario
go test ./internal/nodes/ -run AgentLoop_SingleProbe -v

# Lint
make lint

# DB
make docker-up        # 起 pg + redis
make migrate-up
make seed
make docker-down

# 题库 embedding 重建
go run ./cmd/reindex
```

---

## 环境变量

| 变量 | 用途 | 必填 |
|---|---|---|
| `INTERVIEW_LLM_API_KEY` | LLM (Real 模式) | mode=real 时是 |
| `INTERVIEW_EMBED_API_KEY` | Embedder (Real 模式) | mode=real 时是 |
| `INTERVIEW_POSTGRES_DSN` | PG 连接串 | 用 retriever / migrate 时是 |
| `INTERVIEW_CONFIG` | 自定义配置路径 | 否，默认 `config/config.yaml` |

---

## 修改规则

- 改 `internal/graph/`：必须跑全部 nodes 包测试 + agent_loop_test。
- 改 `internal/domain/` 的结构体：必然牵连一堆节点 + 测试，先 grep 用法。
- 改 prompt：本地 stub 测试基本不受影响，但 real 模式表现可能变化；改完跑一次手工 curl 验证。
- 新增节点：(1) 写节点 + 单测；(2) 在 `routers.go` 加节点名常量；(3) 如有新分支，加 router 函数 + 编译期 `_ graph.Router = RouteAfterXxx`；(4) 在 agent 子图组装处挂上。
- 原则上不要加 `WorkingMemory.Notes` 新 key；确实需要时先证明不能建强类型字段，再 grep 全仓确认不冲突。

---

## 不要做

- 不要把 API key 写进 YAML 或 commit 到仓库。
- 不要在 router 里写副作用。
- 不要在节点里静默 swallow LLM 错误。
- 不要把 `SkillCoverage` 改回 `int`。
- 不要直接读 `AnswerRound.Evaluation` 做最终聚合（用 `FinalEvaluation()`）。
- 不要在 `internal/graph/` 包里塞业务逻辑。
- 不要为了让测试过而调测试期望（先理解为什么不过）。
