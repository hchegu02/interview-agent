# AI Coding 模块入口索引

本文是代码入口索引，不重复解释完整架构。详细设计见 `docs/SDD-Backend.md` 和 `docs/SDD-Frontend.md`。

## 服务入口

| 路径 | 说明 |
|---|---|
| `cmd/server` | HTTP 服务入口和依赖装配 |
| `internal/config` | YAML 和环境变量配置 |

## HTTP 与前端集成

| 路径 | 说明 |
|---|---|
| `internal/httpapi/router.go` | Gin 路由注册 |
| `internal/httpapi/interview_*.go` | 面试 API、Session service、SSE、响应构造 |
| `internal/httpapi/agent_message.go` | `/api/agent/message` Skill 消息入口 |
| `internal/httpapi/documents.go` | 简历文档解析接口 |
| `internal/httpapi/web_assets.go` | 嵌入式前端静态资源 |
| `web/src/apiClient.ts` | 前端 REST 调用 |
| `web/src/useInterviewStream.ts` | 前端 SSE 消费 |
| `web/src/candidatePages.tsx` | 前端核心页面 |
| `web/src/types.ts` | 前后端 JSON 类型对齐 |

## 领域模型与状态

| 路径 | 说明 |
|---|---|
| `internal/domain/session.go` | Session 聚合根、画像、RAG trace、题目和报告字段 |
| `internal/domain/agent.go` | AnswerRound、WorkingMemory、Decision、Critic 等运行时结构 |

## Graph 与节点

| 路径 | 说明 |
|---|---|
| `internal/graph` | 自研 frontier Graph runner、NodeFunc、Router、ErrSuspended、decorator |
| `internal/graphs/interview.go` | 面试业务 Graph 装配 |
| `internal/nodes` | JD/简历解析、RAG、选题、评分、追问、记忆更新、报告节点 |
| `internal/nodes/routers.go` | Agent loop 条件路由 |

## RAG、LLM、Embedding

| 路径 | 说明 |
|---|---|
| `internal/retriever` | vector/BM25/rule/RRF/rerank 检索链路 |
| `internal/embedding` | embedding 抽象和实现 |
| `internal/llm` | ChatModel 抽象、mock/real 模型、限流、熔断 |
| `cmd/rag-eval` | RAG 离线评估 |

## 题库与解析

| 路径 | 说明 |
|---|---|
| `internal/questionbank` | 题库存储、导入、审核、commit |
| `internal/parser` | PDF/DOCX/TXT/Markdown 文档解析 |
| `seeds/question_bank.json` | 本地 seed 题库 |

## Agent 工程能力

| 路径 | 说明 |
|---|---|
| `internal/agentkit` | Skill、Hook、Tool/MCP adapter、Verification 原语 |
| `internal/agent` | Intent Router 和 AgentService |
| `internal/skills` | quiz、explain、project_polish 等 Skill 实现 |
| `cmd/agent-verify` | Agent 输出验证门禁 |

## 后续开发优先入口

| 方向 | 优先入口 |
|---|---|
| Session / Graph 优化 | `internal/domain/session.go`、`internal/graph` |
| Intent Router + Skill | `internal/agent`、`internal/skills`，接口从 `internal/httpapi/agent_message.go` 接入 |
| Long-term Memory | `internal/memory`、`internal/httpapi/interview_memory.go`，不要把长期画像塞进 `Session` |
| 动态难度 | `internal/nodes/evaluate.go`、`internal/nodes/update_memory.go`、`internal/nodes/pick_next.go` |
| Tool / MCP Adapter | `internal/agentkit`，避免再新建重复 `internal/tools` 抽象 |
