# Interview Agent 后端 SDD

## 1. 文档定位

本文描述 Interview Agent 当前后端实现。项目重点在后端工程：Go HTTP API、Session 聚合根、可恢复 Agent Graph、RAG 多路检索、LLM 调用边界、SSE 事件流、存储恢复、降级保护、验证门禁和后续可扩展接口。

本文基于当前仓库实现编写，不把未实现能力写成当前运行时能力。

## 2. 后端目标

后端负责把“上传简历 + 输入 JD + 多轮模拟面试 + 评分报告”组织成稳定、可恢复、可验证的业务链路：

```text
REST 请求
  -> HTTP API
  -> Session 创建 / 加载
  -> Agent Graph 执行
  -> RAG 检索题库
  -> LLM 生成 / 评分 / 总结
  -> Session 持久化
  -> SSE 事件推送
  -> 前端展示
```

后端设计优先级：

1. 状态清晰。
2. 接口兼容。
3. 失败可降级。
4. 链路可验证。
5. 模块边界便于后续优化。

## 3. 代码边界

| 路径 | 职责 |
|---|---|
| `cmd/server` | 服务入口和依赖装配 |
| `internal/httpapi` | HTTP API、SSE、middleware、metrics、session service |
| `internal/domain` | Session 聚合根、题目、轮次、报告、RAG trace 等领域模型 |
| `internal/graph` | 通用 frontier-based graph runner |
| `internal/graphs` | 面试业务 Graph 装配 |
| `internal/nodes` | JD/简历解析、RAG、出题、评分、追问、报告等节点 |
| `internal/retriever` | 多路 RAG、BM25、规则召回、RRF、rerank |
| `internal/llm` | ChatModel 抽象、mock/real 模型、限流、熔断和调用记录 |
| `internal/embedding` | embedding 抽象和实现 |
| `internal/questionbank` | 题库存储、导入、审核、commit |
| `internal/parser` | PDF/DOCX/TXT/Markdown 简历解析 |
| `internal/agentkit` | Skill、Hook、Tool/MCP adapter、Verification 原语 |
| `cmd/rag-eval` | RAG 离线评估 |
| `cmd/agent-verify` | Agent 输出验证门禁 |

## 4. HTTP API 设计

核心接口由 `internal/httpapi/router.go` 装配：

| 方法 | 路径 | 职责 |
|---|---|---|
| `GET` | `/healthz` | 存活检查 |
| `GET` | `/readyz` | 就绪检查和 degraded 状态 |
| `GET` | `/metrics` | Prometheus 指标 |
| `POST` | `/api/documents/parse-resume` | 简历文档解析 |
| `POST` | `/api/interview/start` | 创建面试并推进到首题 |
| `POST` | `/api/interview/answer` | 提交回答并恢复 Graph |
| `GET` | `/api/interview/stream` | SSE 事件流 |
| `GET` | `/api/interview/sessions` | 会话列表 |
| `GET` | `/api/interview/sessions/:session_id` | 会话详情 |
| `GET` | `/api/question-bank` | 题库查询 |
| `GET` | `/api/question-bank/:id` | 题目详情 |
| `GET` | `/api/question-bank/facets` | 题库筛选项 |

接口兼容原则：

- 新增响应字段使用 `omitempty`。
- 老 Session JSON 必须能继续反序列化。
- 前端可依赖字段名，但不能依赖未声明的内部状态。
- 长任务通过 Session 和 SSE 体现进度，不阻塞前端猜测状态。

## 5. Session 状态模型

`internal/domain.Session` 是后端核心聚合根。它保存一次面试的业务事实：

- `ID`
- `UserID`
- `Mode`
- `Status`
- `CurrentNode`
- `JobProfile`
- `CandProfile`
- `ProfileAnalysis`
- `QuestionBankFilter`
- `CandidatePool`
- `RetrievalTrace`
- `Question`
- `Rounds`
- `WorkingMemory`
- `Report`
- `CreatedAt`
- `UpdatedAt`

设计原则：

- `Session.Status` 表示粗粒度生命周期。
- `CurrentNode` 表示 Graph 暂停和恢复位置。
- `Rounds` 保存已发生的问答事实。
- `WorkingMemory` 保存当前会话内的策略状态。
- `RetrievalTrace` 保存 RAG 检索证据，用于报告、排障和验证。

Session 是后端事实源，前端只是读取和展示。

## 6. Agent Graph 设计

当前运行时不是 sub-agent runtime，而是业务 Agent Graph。

核心流程：

```text
parse_jd
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

`internal/graph` 提供通用 runner，`internal/graphs` 负责业务装配，`internal/nodes` 负责具体节点。

Graph 暂停机制：

```text
节点需要用户回答
  -> 返回 ErrSuspended
  -> Session.CurrentNode 记录暂停点
  -> 前端提交回答
  -> 后端加载 Session
  -> Graph 从暂停点恢复
```

这个设计保证面试流程可以跨 HTTP 请求恢复，而不是依赖内存中的长调用栈。

## 7. RAG 检索设计

RAG 用于回答“下一题该问什么”，不是直接替代面试官逻辑。

当前检索链路：

```text
query
  -> embedding
  -> vector recall
  -> BM25 recall
  -> rule recall
  -> RRF fusion
  -> lexical/http rerank
  -> candidate pool
  -> Session.RetrievalTrace
```

关键模块：

- `internal/retriever`
- `internal/nodes/retrieve_rag.go`
- `cmd/rag-eval`

RAG 输出应写入：

- `Session.CandidatePool`
- `Session.RetrievalTrace`
- `WorkingMemory.DegradedReasons`

如果 RAG 为空、错误或 rerank 失败，系统应降级到 fallback 题，不应直接中断面试。

## 8. LLM 与 rerank 边界

LLM 用于：

- JD / 简历理解。
- 问题生成。
- 回答评分。
- critic / refine。
- 报告总结。

RAG 用于：

- 从题库和项目上下文中召回候选题。
- 提供可解释证据。
- 限制问题生成不要完全脱离材料。

rerank 用于：

- 对多路召回候选进行精排。
- 可用本地 lexical reranker。
- 可配置 HTTP reranker 调用本地模型服务。

边界原则：

- 不让 LLM 自己决定所有流程。
- 工具调用、RAG、评分和报告都应保留结构化输入输出。
- HTTP rerank 服务失败时返回错误，由 RAG pipeline 记录 fallback 并回退。

## 9. 存储设计

当前支持三类运行模式：

| 能力 | 默认模式 | 外部依赖模式 |
|---|---|---|
| Session | 内存存储 | PostgreSQL session store |
| 事件流 | 本地 event hub | Redis Streams |
| 题库 | seed JSON | PostgreSQL question bank |
| embedding | mock | real embedding + pgvector |

存储设计要求：

- Session JSON 字段新增必须兼容老数据。
- PostgreSQL 和 Redis 是增强能力，不应破坏默认本地模式。
- Redis lease 和事件流服务于多实例协调，但不能替代 Session 事实源。

## 10. 文档解析设计

简历解析入口：

- `POST /api/documents/parse-resume`
- `internal/httpapi/documents.go`
- `internal/parser`

支持类型：

- PDF
- DOCX
- TXT
- Markdown

解析边界：

- 后端负责提取文本。
- 前端负责展示和允许用户修正。
- 解析失败返回结构化错误，不进入面试 Graph。

## 11. 可观测性与降级

后端需要记录：

- HTTP 请求指标。
- SSE 连接指标。
- Graph 节点指标。
- LLM 调用指标。
- RAG 检索指标。
- breaker degraded 状态。
- retrieval trace。
- hook/tool/verification 事件。

降级策略：

- LLM 失败：按配置重试、熔断或降级。
- RAG 失败：回退 fallback 题。
- rerank 失败：回退 RRF 结果。
- SSE 断开：不影响 Session，前端可重新拉取。
- 文档解析失败：返回错误，不创建错误 Session。

## 12. 验证体系

后端验证命令：

```powershell
go test ./...
go run ./cmd/rag-eval -config testdata/rag_eval/config.json -min-recall-at-5 0.6 -min-mrr 0.4 -min-ndcg-at-5 0.5
go run ./cmd/questionbank-lint -input seeds/question_bank.json
go run ./cmd/agent-verify -session testdata/agent_verify/pass_session.json
```

前端和嵌入式页面验证：

```powershell
npm --prefix web run test
npm --prefix web run build
```

验证原则：

- RAG 修改必须跑 RAG eval。
- Agent 输出结构修改必须跑 agent-verify。
- HTTP API 修改必须跑相关 Go 测试。
- 前端类型和页面修改必须跑前端测试和 build。

## 13. 后续演进计划

以下内容来自后续规划，不代表当前已经实现。

### 13.1 基础架构优化：对齐 LangGraph 的可取点

当前系统不需要迁移到 LangGraph。项目是 Go 后端，核心价值在 `Session` 聚合根、业务 Graph、RAG、LLM 边界和验证门禁；直接引入 LangGraph 会带来跨语言 runtime、部署复杂度和框架黑盒。

但 LangGraph 的几个设计点值得吸收：

- 用结构化 interrupt 表达暂停原因和等待输入。
- 用 checkpoint 保存每个执行步的状态快照。
- 用 state update / reducer 思路减少并发写冲突。
- 区分单次 thread state 和跨会话 long-term memory。

建议按兼容方式渐进改造。

#### 13.1.1 结构化 Suspension

当前暂停依赖 `ErrSuspended + Session.CurrentNode`。这个设计简单，但只知道“停在哪个节点”，不知道“为什么停、等待什么输入、要给前端展示什么 payload”。

第一步已落地结构化断点字段：

```go
type Suspension struct {
    Node      string         `json:"node"`
    Reason    string         `json:"reason,omitempty"`
    Awaiting  string         `json:"awaiting"` // answer | approval | tool_review
    Payload   map[string]any `json:"payload,omitempty"`
    CreatedAt time.Time      `json:"created_at"`
}
```

兼容策略：

- 保留 `CurrentNode`，避免破坏老 Session。
- 新逻辑同时写 `Session.Suspension`。
- HTTP 响应已返回可选 `suspension`，前端类型已对齐；SSE 事件复用同一响应结构。
- `Resume` 优先读取 `Suspension.Node`，没有则回退 `CurrentNode`。

当前边界：

- 默认暂停类型先使用 `answer`，人工确认、工具审批和题目确认后续按节点语义写入。
- `Payload` 当前只做 HTTP 响应层 map 拷贝；如果后续放入嵌套结构，需要补深层 clone 或改成明确 schema。
- `CurrentNode` 仍是兼容字段，不能立即删除，否则会破坏旧 Session 恢复。

收益：

- 暂停/恢复语义更接近 LangGraph interrupt。
- 后续支持人工确认、工具审批、题目确认时不需要重写 Graph。
- 前端可以知道当前是在等答案、等审批还是等工具 review。

#### 13.1.2 StatePatch / 局部状态更新

当前节点直接修改 `*domain.Session`。这对单线程线性流程足够简单，但节点增多、fan-out 并发增多后，容易出现多个节点写同一字段的问题。

建议先不全量重写 NodeFunc，而是在高风险节点逐步引入 `StatePatch`：

```go
type StatePatch struct {
    CandidatePool   *[]domain.Question
    RetrievalTrace  *domain.RetrievalTrace
    PendingDecision *domain.Decision
    AppendRound     *domain.AnswerRound
}
```

演进方式：

- 第一阶段只覆盖 `retrieve_rag`、`pick_next`、`evaluate`、`report` 等关键节点。
- Runner 或 service 层统一 apply patch。
- append、overwrite、merge 规则写清楚，不让节点随意改大对象。

收益：

- 接近 LangGraph “节点返回 state update”的优点。
- 降低并发写冲突。
- 后续 checkpoint 可以记录 patch，而不是只记录完整 Session。

#### 13.1.3 轻量 Graph Checkpoint

当前持久化主要是保存完整 Session。排障时能看到最终状态，但很难回看每一步 Graph 执行前后的状态变化。

建议新增轻量 checkpoint，不做完整 time travel UI：

```go
type GraphCheckpoint struct {
    SessionID string
    Step      int
    Frontier  []string
    Node      string
    Snapshot  []byte
    CreatedAt time.Time
}
```

落地策略：

- 内存模式使用 ring buffer。
- PostgreSQL 模式可以单独建 checkpoint 表。
- 只在 debug / test / degraded 场景开启，避免默认成本过高。
- 先用于失败排障和回归测试，不承诺完整回滚能力。

收益：

- 更接近 LangGraph checkpointer 的排障能力。
- 可以定位“哪个节点把 Session 改坏”。
- 后续 agent-verify 可基于 checkpoint 做更细粒度检查。

#### 13.1.4 并发写保护

当前 frontier 可以并发执行多个节点，但依赖节点作者遵守“并发节点写 disjoint 字段”的约定。这个约定需要变成可检查的结构。

建议扩展节点注册信息：

```go
type NodeSpec struct {
    Name   string
    Fn     NodeFunc
    Writes []string
}
```

Compile 阶段或测试阶段检查同一 frontier 中是否存在相同写字段。短期可以只作为测试工具，不一定进入生产 runner。

收益：

- 把并发写约定从注释变成检查。
- 比单纯依赖 `go test -race` 更早发现设计冲突。
- 后续新增 Router、Memory、Difficulty 节点时风险更低。

#### 13.1.5 状态分层原则

后续新增 Memory 时不能把所有东西都塞进 `Session`。

建议分层：

```text
Session
  单次面试事实：题目、轮次、报告、retrieval_trace、当前暂停点

WorkingMemory
  当前面试内策略状态：已问数量、追问预算、临时弱点、降级原因

UserMemory
  跨 session 长期画像：历史薄弱点、技能分数、复习建议、常错知识点
```

这个分层和 LangGraph 的 thread state / long-term memory 思路一致，但实现保持 Go 项目自己的存储和领域模型。

### 13.2 第一阶段：Intent Router + Skill

目标：在固定 Interview Graph 之外，增加用户意图路由和专项 skill 能力，让系统从“单一模拟面试流程”扩展为“面试训练 Agent”。

建议能力：

```text
interview.start
interview.answer
skill.quiz
skill.explain
skill.project_polish
skill.tech_compare
chat
```

建议目录：

```text
internal/agent
  router.go
  intent.go
  message.go

internal/skills
  registry.go
  quiz.go
  explain.go
  project_polish.go
  tech_compare.go
```

第一版建议使用规则 Router，不急着做 LLM intent classifier。Router 只负责分流，Skill 负责专项能力，Graph 继续负责正式面试主流程。

可能新增接口：

```text
POST /api/agent/message
```

接口必须返回结构化 intent、skill、confidence、reason 和结果，便于前端展示和后续验证。

### 13.3 第二阶段：Long-term Memory

目标：把单次面试报告沉淀为长期用户画像。

当前 `WorkingMemory` 适合当前 Session。后续可以新增长期记忆模块：

```text
internal/memory
  memory.go
  short_term.go
  long_term.go
  store.go
  postgres_store.go
  inmem_store.go
```

建议结构：

```go
type UserMemory struct {
    UserID      string
    Strengths   []string
    Weaknesses  []Weakness
    SkillScores map[string]float64
    LastAdvice  []string
    UpdatedAt   time.Time
}

type Weakness struct {
    Topic     string
    Evidence  string
    Severity  int
    UpdatedAt time.Time
}
```

接口边界：

```go
type MemoryStore interface {
    GetUserMemory(ctx context.Context, userID string) (*UserMemory, error)
    UpsertUserMemory(ctx context.Context, memory *UserMemory) error
}
```

长期记忆应影响后续题目权重和复习建议，但不能覆盖 Session 内已经发生的事实。

### 13.4 第三阶段：动态难度

目标：根据用户连续答题表现和历史薄弱点调整下一题难度。

建议先做简单状态机：

```go
type Difficulty int

const (
    DifficultyEasy   Difficulty = 1
    DifficultyMedium Difficulty = 2
    DifficultyHard   Difficulty = 3
)

type DifficultyState struct {
    Current        Difficulty
    CorrectStreak int
    WrongStreak   int
}
```

建议流程：

```text
evaluate
  -> update_memory
  -> update_difficulty
  -> reflection_check
  -> retrieve_rag / pick_next
```

动态难度要依赖评分、当前题目难度、历史薄弱点和目标岗位，不要让 LLM 单独决定难度。

### 13.5 第四阶段：MCP Adapter

目标：提供统一工具调用边界，为后续 GitHub 项目分析、网页抓取等外部工具接入做准备。

建议先做抽象和 mock，不急着接真实 MCP Server、容器 sandbox 或完整 Gateway。

建议目录：

```text
internal/tools
  registry.go
  tool.go
  mcp_adapter.go
  mock_mcp.go
  github_project.go
  web_fetch.go
```

建议接口：

```go
type Tool interface {
    Name() string
    Description() string
    Call(ctx context.Context, input ToolInput) (ToolOutput, error)
}

type ToolInput struct {
    Arguments map[string]any
}

type ToolOutput struct {
    Result map[string]any
    Error  string
}

type MCPAdapter interface {
    CallTool(ctx context.Context, name string, args map[string]any) (ToolOutput, error)
}
```

推荐先接两个工具场景：

- `github.project_analyze`
- `web.fetch`

用途：

```text
用户输入 GitHub 项目地址
  -> Tool 拉取 README / 项目结构
  -> ResumeProfilerAgent 分析项目亮点
  -> ProjectPolishSkill 生成简历项目描述
  -> InterviewerAgent 针对项目追问
```

### 13.6 MVP 闭环

推荐后续 MVP：

```text
/api/agent/message
  -> Intent Router
  -> Skill Registry
  -> QuizSkill / ExplainSkill / ProjectPolishSkill
  -> 读写 Long-term Memory
  -> 动态难度影响下一题
  -> Tool Registry 预留 MCP Adapter
```

第一版目标是链路跑通，不追求功能铺满。

### 13.7 Codex sub-agent 开发说明

后续开发中可以使用 Codex sub-agent 拆分任务，例如让不同开发代理分别处理 Router、Memory、Difficulty、MCP Adapter、前端展示和验证。但这是开发协作方式，不是 Interview Agent 当前运行时能力。

当前系统不能写成已经支持 sub-agent runtime、sub-agent 调度或分布式多代理执行。

## 14. 非目标

- 不实现通用 Coding Agent 平台。
- 不实现完整 OpenClaw Gateway。
- 不实现 daemon runtime、云端 runtime 或容器 Sandbox。
- 不把 Codex sub-agent 开发方式写成当前服务能力。
- 不在没有评估和测试前把 LLM Router 替换为核心控制面。
- 不让 LLM 绕过后端 schema、权限、超时和验证边界直接调用工具。
