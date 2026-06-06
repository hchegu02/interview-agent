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

当前节点仍以 `*domain.Session` 作为共享状态。第一阶段已经引入轻量 `StatePatch`，把高风险节点的核心写入收敛到统一 apply 入口，但没有改变 `graph.NodeFunc` 签名。

已落地的 patch 模型：

```go
type StatePatch struct {
    CandidatePool        *[]Question
    RetrievalTrace       *RetrievalTrace
    PendingDecision      *Decision
    ClearPendingDecision bool
    AppendRound          *AnswerRound
    CurrentEvaluation    *Evaluation
    CompleteCurrentRound *time.Time
    Report               *Report
}
```

当前实现：

- `internal/domain.ApplyStatePatch` 统一处理 replace、append、current round 写入和错误返回。
- `retrieve_rag` 通过 patch 写 `CandidatePool` / `RetrievalTrace`。
- `pick_next` 通过 patch 写 `PendingDecision` / append `AnswerRound`，`WorkingMemory.RoundsAsked` 暂时仍直接写。
- `evaluate` 通过 patch 写当前轮 `Evaluation` 并清理 `PendingDecision`。
- `report` 通过 patch 写 `Report` 并清理 `PendingDecision`。

当前边界：

- `NodeFunc` 仍返回 `error`，节点在内部调用 patch helper；Graph runner 还不记录 patch。
- `WorkingMemory`、`Status`、`CriticResult`、`FollowUps` 等字段还未纳入 patch，后续按风险逐步迁移。
- `RetrievalTrace` 和 `Report` 指针保持当前状态写入语义，HTTP 响应层负责 clone。

收益：

- 接近 LangGraph “节点返回 state update”的优点。
- 降低并发写冲突。
- 后续 checkpoint 可以记录 patch，而不是只记录完整 Session。

#### 13.1.3 轻量 Graph Checkpoint

当前持久化主要是保存完整 Session。排障时能看到最终状态，但很难回看每一步 Graph 执行前后的状态变化。第一版轻量 checkpoint 已在 `internal/graph` 落地，用 runner 级 recorder 记录执行证据，不改变业务节点签名。

已落地模型：

```go
type GraphCheckpoint struct {
    Seq       int64
    SessionID string
    Step      int
    Graph     string
    Phase     CheckpointPhase
    Frontier  []string
    Node      string
    Error     string
    Snapshot  []byte
    CreatedAt time.Time
}
```

当前实现：

- 新增 `CheckpointRecorder`，通过 `Graph.WithCheckpointRecorder` 可选注入。
- `internal/graphs.BuildInterviewGraph` 通过 `Deps.CheckpointRecorder` 透传 recorder，业务 Interview Graph 可按需启用。
- 新增 `MemoryCheckpointRecorder`，使用 ring buffer 只保留最近 N 条。
- `Runnable.run` 记录 `frontier_before`、`frontier_after`、`frontier_error` 和 `suspended`。
- `Resume` 记录 `resume_from`，其中 `Node` 是恢复来源，`Frontier` 是下一轮 frontier。
- 线性 frontier 记录 `node_before`、`node_after`、`node_error`。
- 并发 frontier 只记录 batch 级 checkpoint，不伪造节点级快照。
- 并发 frontier 中，runner 不在节点 goroutine 内写 `CurrentNode`；节点返回 suspend 时由主协程统一写入暂停节点。

当前边界：

- 不写 PostgreSQL checkpoint 表。
- 不实现 time travel、回滚或 UI。
- 不改变 HTTP API、SSE、Session JSON 和 `NodeFunc`。
- snapshot 使用 `json.Marshal(Session)`，只适合 debug / test / degraded 场景，不建议默认生产开启。
- recorder 调用有短超时保护；runner 会吞掉 recorder panic，并避免慢 recorder 长时间阻塞业务 Graph。
- 自定义 recorder 必须快速返回并尊重 `context.Context`；runner 无法强制终止完全忽略 `ctx` 的 recorder。
- 并发 frontier 的 checkpoint 不做节点级归因；Session 仍要求并发节点只写 disjoint 字段，这一点要等 13.1.4 并发写保护继续收口。

收益：

- 更接近 LangGraph checkpointer 的排障能力。
- 可以定位“哪个节点把 Session 改坏”。
- 后续 agent-verify 可基于 checkpoint 做更细粒度检查。

#### 13.1.4 并发写保护

当前 frontier 可以并发执行多个节点。runner 已避免在并发节点 goroutine 内写 `CurrentNode`，但业务节点本身仍依赖“并发节点写 disjoint 字段”的约定。第一版并发写保护已在 `internal/graph` 落地，把这个约定变成 runner 可检查规则。

已落地模型：

```go
type NodeSpec struct {
    Name   string
    Fn     NodeFunc
    Writes []string
}

type PatchNodeFunc func(context.Context, *domain.Session) (domain.StatePatch, error)
```

当前实现：

- `AddNode(name, fn)` 继续保留，作为 legacy 线性兼容入口。
- `AddNodeSpec(spec)` 支持声明节点写集。
- `PatchNode(name, writes, fn)` 支持 patch-aware 节点，由 runner 统一调用 `domain.ApplyStatePatch`。
- 并发 frontier 执行前检查写集：
  - legacy 节点没有 `Writes` 时，不允许参与并发 frontier。
  - 两个节点 `Writes` 有交集时，不允许并发执行。
  - 写集 disjoint 时，允许并发执行。
- `internal/graphs.BuildInterviewGraph` 已给 `retrieve_rag`、`pick_next`、`evaluate`、`report` 等关键节点声明写集；其中 `report` 声明了 `pending_decision/report/status/working_memory`。

当前边界：

- 未删除 `NodeFunc`，未强制所有节点一次性迁移。
- 未改变 HTTP API、SSE、Session JSON 或数据库 schema。
- patch-aware 能力已在 Graph 层可用，但业务节点仍以兼容迁移为主；不是完整 LangGraph runtime。
- 写集是粗粒度 key，宁可保守拒绝一部分并发组合，也不放过明显冲突。

收益：

- 把并发写约定从注释变成检查。
- 比单纯依赖 `go test -race` 更早发现设计冲突。
- 为 checkpoint 记录 patch、agent-verify 检查节点写入边界打基础。
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

目标：在固定 Interview Graph 之外，增加用户意图路由和专项 skill 能力，让系统从“单一模拟面试流程”扩展为“面试训练 Agent”。第一版规则 Router + Skill Registry 已落地，作为后续 LLM Router 和更多 Skill 的稳定入口。

已落地能力：

```text
skill.quiz
skill.explain
skill.project_polish
interview.start
chat
```

已落地目录：

```text
internal/agent
  agent.go

internal/skills
  skills.go
```

当前实现：

- `RuleRouter` 根据关键词输出 `intent`、`skill`、`confidence` 和 `reason`。
- `Skill Registry` 注册并执行 `quiz`、`explain`、`project_polish`。
- `AgentService` 统一处理消息，skill 请求执行对应 skill。
- `interview.start` 只返回引导结果，不绕过 `/api/interview/start` 自动创建 session。
- `cmd/server` 默认注入规则 AgentService。

新增接口：

```text
POST /api/agent/message
```

接口返回结构化 intent、skill、confidence、reason 和 result，便于前端展示和后续验证。

当前边界：

- 不使用 LLM intent classifier。
- 不实现运行时 sub-agent。
- 不写数据库，不改变现有 Interview Graph。
- 不把用户消息作为工具调用权限来源。

### 13.3 第二阶段：Long-term Memory

目标：把单次面试报告沉淀为长期用户画像。

当前 `WorkingMemory` 适合当前 Session。长期记忆基础层已落地，用于承载跨 Session 的用户画像，但第一版尚未自动接入 Interview Graph、数据库或 HTTP API。

```text
internal/memory
  memory.go
  memory_test.go
```

已落地结构：

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

已落地接口边界：

```go
type Store interface {
    GetUserMemory(ctx context.Context, userID string) (*UserMemory, error)
    UpsertUserMemory(ctx context.Context, memory *UserMemory) error
}
```

当前实现：

- `MemoryStore` 是线程安全内存实现，读写时做 defensive copy，避免外部引用污染内部状态。
- `BuildUpdateFromSession` 从 `domain.Session.Report` 提取 highlights、improvements、skill breakdown、next steps 和 drill plan，生成 `UserMemoryUpdate`。
- `ApplyUpdate` 将一次面试报告增量合并到 `UserMemory`，字符串集合去重保序，weakness 按主题和证据去重，同名技能分数用新旧均值保守更新，`UpdatedAt` 不接受零值或旧时间回退。
- `InterviewService.Answer` 在 Session 完成并保存后，会把 Report 非阻塞沉淀到长期记忆 Store；服务层串行化 `Get -> Apply -> Upsert`，避免当前进程内并发完成同一用户多场面试时丢更新。
- `cmd/server` 默认注入内存长期记忆 Store，用于本地演示和测试闭环。
- 缺少 `user_id`、Session 或 Report 时返回结构化错误，不生成残缺画像。

当前边界：

- 不修改输入 `Session`、`Report` 或 `WorkingMemory`。
- 不自动写入数据库或缓存，当前只用进程内 Store。
- 不新增 HTTP API。
- 不改变现有 Interview Graph 流程。
- 不使用 LLM 总结长期画像。
- 长期记忆写入失败不阻断面试完成响应。

长期记忆后续应影响题目权重和复习建议，但不能覆盖 Session 内已经发生的事实。

### 13.4 第三阶段：动态难度

阶段目标：根据用户连续答题表现和历史薄弱点调整下一题难度。

第一版动态难度基础层已落地，先做单次 Session 内的规则状态机：

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

当前流程：

```text
evaluate
  -> update_memory
  -> update_difficulty
  -> reflection_check
  -> pick_next / report
```

当前实现：

- `WorkingMemory.Difficulty` 保存当前难度、连续高分次数、连续低分次数和最近已消费的评分轮次。
- `update_difficulty` 读取最近一轮最终评分，`score >= 80` 计入高分 streak，`score < 50` 计入低分 streak。
- 连续两轮高分升一档，连续两轮低分降一档，难度限制在 easy/medium/hard。
- 中间分数清空 streak，降级评分 `score < 0` 不影响难度。
- 重放同一个 round 时不会重复累计 streak，避免恢复或重试导致难度误升/误降。
- Graph 已接入 `update_memory -> update_difficulty -> reflection_check`。
- `retrieve_rag` 读取 `WorkingMemory.Difficulty.Current` 推导 RAG 基础目标难度：easy -> 2，medium -> 3，hard -> 4。
- RAG 仍会在动态目标难度上叠加 `GapStrategy` 微调；用户设置的 `QuestionBankFilter.DifficultyMin/DifficultyMax` 继续作为硬过滤条件传给 retriever。

当前边界：

- 不让 LLM 单独决定难度。
- 尚未读取长期记忆中的历史弱点。
- 动态难度只影响 RAG 候选题目标难度，尚未显式写入 `pick_next` 的 LLM prompt。
- 不改变 HTTP 响应结构。

### 13.5 第四阶段：MCP Adapter

目标：提供统一工具调用边界，为后续 GitHub 项目分析、网页抓取等外部工具接入做准备。

当前阶段复用 `internal/agentkit`，不再新建重复的 `internal/tools` 抽象。`agentkit` 已提供：

- `Tool`
- `ToolSpec`
- `ToolCall`
- `ToolResult`
- `ToolRegistry`
- `MCPClient`
- `MCPToolAdapter`
- `Permission`
- Hook 事件

已落地 foundation：

- `ToolRegistry.List` 稳定返回已注册工具清单。
- `MockMCPClient` 提供 deterministic mock 输出，便于本地测试和演示。
- `RegisterDefaultMCPTools` 注册默认 mock MCP 工具。
- `github.project_analyze` 返回项目摘要、主要语言、亮点和风险点。
- `web.fetch` 返回 URL、标题和正文摘要。
- 所有工具调用仍经过 `ToolRegistry.Call` 的权限、超时和 before/after hook。

当前工具只是 mock foundation：

- 不接真实 GitHub API。
- 不接真实网页抓取。
- 不实现完整 MCP Server / Client 协议生命周期。
- 不实现 Gateway、daemon、Sandbox 或 runtime sub-agent。
- 不改变 `/api/agent/message` 响应结构。

已接入 Skill 链路：

- `agent.NewDefaultService` 默认创建 mock MCP tool registry，并注入 `skills.NewDefaultRegistryWithTools`。
- `cmd/server` 通过 `buildAgentService` 装配默认 Agent 服务。
- `ProjectPolishSkill` 优先从 `context.github_url`、`context.github`、`context.repo_url` 读取 GitHub URL，其次从用户消息中识别 `github.com/owner/repo`。
- 有 GitHub URL 且工具可用时，`ProjectPolishSkill` 通过 `ToolRegistry.Call` 调用 `github.project_analyze`。
- 工具成功时，输出融合 mock 项目摘要、亮点和风险点。
- 没有 URL、没有工具或工具失败时，降级到原通用项目亮点提炼建议，不中断 `/api/agent/message`。

后续真实工具接入后的用途：

```text
用户输入 GitHub 项目地址
  -> 后续接入真实工具后拉取 / 分析项目资料
  -> 当前阶段 github.project_analyze 只返回 mock 项目分析结果
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
