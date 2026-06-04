# Agent Tooling Upgrade Design

## 目标

本次升级的目标不是把项目改成通用 Coding Agent 平台，而是在现有 AI 模拟面试官业务主线之上，补齐更符合 Agent 工程岗位要求的能力层：

- Skill Registry：把现有 JD 分析、简历解析、题库检索、答案评估、报告生成沉淀为可描述能力。
- Hook：在关键 Agent 执行点记录输入摘要、输出摘要、耗时和错误，支撑审计、观测和验证。
- Tool Registry / MCP Client：把题库检索、会话读取、报告写回等外部能力封装为可审计工具调用。
- Verification Loop：对结构化输出、检索 trace、工具调用和报告完整性做自动校验。

这次升级保留现有 `internal/graph`、`internal/graphs` 和 `internal/nodes` 的执行模型，不重写面试流程，不引入大型第三方 Agent 框架。

## 当前基础

项目当前已经具备以下基础：

- `internal/graph` 提供可恢复 Graph runner。
- `internal/graphs.BuildInterviewGraph` 组装面试流程。
- `internal/nodes` 覆盖 JD/简历解析、RAG、出题、评估、追问、记忆更新和报告。
- `internal/domain.Session` 保存面试上下文、候选题池、运行时记忆、轮次和报告。
- `internal/retriever` 已有 pgvector、BM25、规则召回、RRF、rerank 和 retrieval trace。
- `cmd/rag-eval`、`cmd/questionbank-lint`、`cmd/eval` 已具备离线质量评估入口。

缺口是：这些能力目前主要体现为业务节点和离线命令，还没有形成清晰的 Agent Tooling 层。面试时能讲业务闭环，但对 Skill、Hook、Tool/MCP、Verification Loop 的工程抽象还不够明确。

## 非目标

以下内容本次不做：

- 不做完整 OpenClaw 风格 Gateway。
- 不做本地 daemon 和云端 runtime 双模式。
- 不做完整容器 Sandbox。
- 不把项目改成 Coding Agent 平台。
- 不把所有 graph node 重写成 Skill 执行。
- 不强依赖真实 MCP Server 或真实外部工具服务。

## 方案选择

采用轻量 Agent Tooling 层：

```text
现有 Interview Graph
  |
  +-- Skill Registry: 描述可复用能力
  +-- Hook Chain: 记录执行审计和验证信号
  +-- Tool Registry: 统一工具调用、权限、超时和错误
  +-- MCP Client: 预留外部工具协议接入
  +-- Verification Loop: 校验 Agent 输出质量
```

这个方案的好处是改动可控，不破坏当前可运行链路，又能把项目从普通 LLM/RAG 应用提升到 Agent 工程项目。

## 阶段 1：Skill Registry

新增 `internal/agentkit`，定义 Agent 能力目录。

核心类型：

- `SkillSpec`：描述 skill 名称、版本、说明、输入摘要、输出摘要、权限和超时。
- `Skill`：持有 `SkillSpec` 和可选 handler。
- `Registry`：支持注册、查找、列出 skills。
- `Permission`：表达 `read_only`、`write_session`、`write_report`、`external_tool` 等权限。

第一阶段只注册元数据，不强制改变现有节点执行方式。

首批 skill：

- `jd.analyze`
- `resume.parse`
- `profile.match`
- `question.retrieve`
- `answer.evaluate`
- `report.generate`

验收：

- 重复注册返回错误。
- 查找不存在 skill 返回结构化错误。
- 列表输出稳定排序。
- 不影响现有 `BuildInterviewGraph`。

## 阶段 2：Hook 审计

在 `internal/agentkit` 增加 Hook 抽象，先服务审计和验证，不做复杂插件系统。

Hook 事件：

- `before_skill`
- `after_skill`
- `before_tool`
- `after_tool`
- `verification_failed`

事件字段：

- `trace_id`
- `session_id`
- `name`
- `input_summary`
- `output_summary`
- `latency_ms`
- `error`
- `permission`

默认实现为 no-op。审计实现可以先写入内存 recorder，后续再接 metrics 或日志。

接入点优先选择：

- `retrieve_rag`
- `evaluate`
- `report`

验收：

- no-op hook 不改变业务行为。
- 成功和失败路径都能记录事件。
- 节点错误时仍能触发 after hook 并记录 error。

## 阶段 3：Tool Registry / MCP Client

新增工具调用抽象，目标是让 Agent 调用外部能力时有统一边界，而不是在节点里散写逻辑。

核心类型：

- `ToolSpec`：名称、说明、权限、超时、输入 schema 摘要。
- `Tool`：具体工具接口。
- `ToolCall`：工具调用请求。
- `ToolResult`：工具调用结果。
- `ToolRegistry`：注册、查找和调用工具。
- `MCPClient`：预留外部 MCP 工具调用接口。

第一批本地工具：

- `questionbank.search`
- `session.read`
- `report.write`

工具调用必须经过：

- 权限校验。
- context timeout。
- 结构化错误。
- hook 审计。

MCP 第一版只实现接口和 mock/local adapter，不依赖真实 MCP 服务，避免测试不稳定。

验收：

- unknown tool、permission denied、timeout 都返回结构化错误。
- mock MCP client 可在测试中模拟成功和失败。
- 工具调用可被 hook 记录。

## 阶段 4：Verification Loop

新增 `internal/agentkit/verify`，把当前离线评估扩展成 Agent 输出验证回路。

验证器：

- `StructuredOutputVerifier`：检查 LLM 输出是否符合结构化要求。
- `RetrievalTraceVerifier`：检查 retrieval trace 是否缺失、空召回或 fallback 异常。
- `ToolCallVerifier`：检查工具调用是否越权或失败率异常。
- `ReportCompletenessVerifier`：检查报告是否包含评分、薄弱点、复习计划和用户回答引用。

第一版只记录 verification failure，不做自动重试。自动 retry 容易扩大复杂度，后续再评估。

验收：

- report 缺少关键字段时 verifier 返回失败。
- retrieval trace 缺失或空召回时 verifier 返回失败。
- 越权工具调用能被 verifier 捕获。
- `cmd/eval` 或新增测试能覆盖 verification failure 输出。

## 阶段 5：文档与简历收口

README 增加 `Agent Tooling 设计` 小节，说明：

- Graph 是流程编排。
- Skill Registry 是能力目录。
- Tool Registry / MCP Client 是外部能力接入边界。
- Hook 是审计和观测入口。
- Verification Loop 是质量门槛。

同时更新项目讲解文档，避免写成通用 Coding Agent 平台，而是强调：

- 业务主线仍然是 AI 模拟面试官。
- Agent Tooling 是围绕面试任务做的工程增强。
- MCP 目前是轻量 client 抽象和 mock/local adapter，不夸大为完整 MCP 平台。

## 测试计划

最小验证：

```powershell
go test ./internal/agentkit ./internal/nodes ./internal/httpapi ./cmd/eval -count=1
go test ./... -count=1
```

回归验证：

```powershell
go run ./cmd/rag-eval -cases testdata/rag/golden_queries.jsonl -config config/config.yaml.example -out tmp/eval/rag -min-recall-at-5 0.70 -min-recall-at-10 0.80 -min-mrr-at-k 0.90 -min-ndcg-at-k 0.75 -min-group-cases 3 -min-group-recall-at-5 0.50 -min-stage-recall-at-5 vector=0.70,bm25=0.65,rule=0.60,rrf=0.75,rerank=0.70 -min-stage-mrr-at-k rrf=0.88,rerank=0.90
go run ./cmd/questionbank-lint -seed seeds/question_bank.json -min-expected-points 3 -min-scenario-ratio 0.8
go run ./cmd/eval -suite testdata/eval -mode mock -out tmp/eval/mock
```

如果验证命令生成 `tmp/eval/*` 临时输出，任务结束时清理。

## 风险

- Agentkit 如果直接侵入所有节点，会造成大范围重构。本设计先做轻量注册和关键节点接入。
- MCP 如果第一版接真实外部服务，会导致测试依赖环境。第一版只做接口、mock 和本地工具。
- Verification Loop 如果直接自动重试，会增加状态复杂度。第一版只记录失败并进入离线评估。
- 任何新增字段如果进入 `Session` JSON，需要考虑 PG state_json、Redis snapshot、HTTP 响应兼容性。

## 实施顺序

1. 新增 `internal/agentkit` 的 Skill Registry 和测试。
2. 增加 Hook 事件、no-op hook、recorder hook 和测试。
3. 在 `retrieve_rag`、`evaluate`、`report` 三个关键节点接入 hook。
4. 新增 Tool Registry、权限、超时、结构化错误和测试。
5. 新增 MCP Client 接口和 mock/local adapter。
6. 新增 Verification Loop 验证器和测试。
7. 接入 `cmd/eval` 或专项测试输出 verification failures。
8. 更新 README 和项目讲解文档。
9. 运行完整 Go 测试和离线评估命令。

## 简历表达边界

实现完成后可以写：

- 设计 Skill Registry 与 Hook 机制，将 JD 分析、简历解析、题库检索、答案评估和报告生成沉淀为可复用 Agent 能力，并记录关键节点输入摘要、输出摘要、耗时和错误信息。
- 构建轻量 Tool Registry / MCP Client 抽象，将题库检索、会话读取和报告写回封装为可审计工具调用，通过 schema 校验、超时控制、权限校验和结构化错误保证工具链路稳定。
- 建立 Agent Verification Loop，对结构化输出、检索 trace、工具调用和报告完整性进行自动校验，并结合 RAG eval、questionbank lint 和 mock eval 防止质量退化。

未实现完整本地 daemon、云端 runtime、容器 Sandbox 和通用 Coding Agent Gateway 前，不在简历里写这些能力。
