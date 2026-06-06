# Interview Agent 前端 SDD

## 1. 文档定位

本文描述 Interview Agent 当前前端实现。前端不是项目核心复杂度所在，它的职责是作为候选人工作台，对齐后端 API、SSE 事件和 Session 数据结构，把后端 Agent Graph、RAG、评分和报告结果稳定展示出来。

本文只描述当前仓库已有实现和可验证的后续演进方向，不声明尚未实现的运行时能力。

## 2. 前端目标

前端目标是让用户完成一条完整面试训练链路：

```text
录入简历
  -> 输入 JD
  -> 分析 JD / 简历
  -> 开始面试
  -> 提交回答
  -> 接收实时事件
  -> 查看报告、逐题反馈、训练计划和 RAG 检索链路
```

前端必须保持简单，避免把后端状态机复制到浏览器里。业务事实以服务端 `Session` 为准。

## 3. 代码边界

当前前端主要位于 `web/src`：

| 路径 | 职责 |
|---|---|
| `web/src/main.tsx` | 应用入口、路由切换、Session 拉取和页面组合 |
| `web/src/candidatePages.tsx` | 简历页、JD 页、面试页、报告页、Agent Skill 入口、长期画像展示 |
| `web/src/apiClient.ts` | REST API 调用封装 |
| `web/src/useInterviewStream.ts` | SSE 事件订阅 |
| `web/src/types.ts` | 前后端共享 JSON 响应的 TypeScript 类型 |
| `web/src/reportView.ts` | 报告和训练计划展示辅助函数 |
| `web/src/draftStore.ts` | 本地草稿和题库范围归一化 |
| `web/src/styles.css` | 页面样式 |

前端构建产物由 Vite 生成，并嵌入到 `internal/httpapi/web/dist`，由 Go 服务统一暴露。

## 4. 页面设计

### 4.1 ResumePage

`ResumePage` 负责简历输入：

- 支持粘贴简历文本。
- 支持上传 PDF、DOCX、TXT、Markdown。
- 调用 `POST /api/documents/parse-resume` 后，把解析结果写回草稿。
- 将简历拆为概况、技能栈、项目经历、亮点和原文补充，便于用户二次编辑。

前端只做输入和展示，不在浏览器侧实现复杂文档解析。文件解析属于后端 `internal/parser` 和 `internal/httpapi/documents.go` 的职责。

### 4.2 JDPage

`JDPage` 负责岗位输入和面试准备：

- 录入 JD。
- 触发 JD / 简历匹配分析。
- 展示 `profile_analysis`。
- 配置题库范围，例如技能分类、场景、难度和标签。
- 启动面试，调用 `POST /api/interview/start`。

题库范围会跟随 start 请求写入后端 `Session.QuestionBankFilter`，最终传给 RAG 检索链路。

### 4.3 InterviewPage

`InterviewPage` 负责多轮面试交互：

- 展示当前题目。
- 提交用户回答。
- 展示历史轮次、评分和追问。
- 展示 SSE 事件流。
- 在用户提交回答后等待后端恢复 Graph 并推进下一步。

前端不自行判断是否该追问、换题或生成报告。这些决策由后端 Agent Graph 和 Session 状态决定。

### 4.4 ReportPage

`ReportPage` 负责最终报告：

- 展示总分和技能拆解。
- 展示简历 / JD 匹配分析。
- 展示回答诊断。
- 展示训练计划。
- 展示逐题评分。
- 展示 `retrieval_trace`，解释 RAG 检索链路。

`retrieval_trace` 是后端写入 Session 的检索证据。前端只展示 query、阶段统计、降级原因、最终候选和来源分数，不参与排序和 rerank。

### 4.5 AgentPage

`AgentPage` 是 `/api/agent/message` 的轻量前端入口：

- 输入自然语言任务。
- 调用后端 Intent Router + Skill。
- 展示 `intent`、`skill`、`confidence`、`reason` 和 `result`。
- 当后端返回 `tool_trace` 时，只读展示工具名、权限、状态、错误类别、耗时和摘要。
- 对 `start_interview` 动作跳转到 JD 页面。
- 对 `start_drill` 动作把训练主题写入 JD 草稿。

前端不实现意图判断、Skill 执行、工具调用、MCP 访问或权限判断；`tool_trace` 只是后端响应的展示字段。

### 4.6 UserMemoryPage

`UserMemoryPage` 是长期用户画像的只读页面：

- 调用 `GET /api/users/:user_id/memory`。
- 请求携带 `X-User-ID` 作为当前开发模式 owner 标识，让后端可以拒绝读取非本人画像。
- 展示跨 Session 沉淀出的优势、弱项、技能分数、最近建议和更新时间。
- 找不到画像时展示空状态。

前端不编辑长期画像，也不把长期画像塞回当前 Session。当前后端只提供最小 owner resolver / authorizer，不是完整 JWT 登录体系；生产接入前仍应替换为真实身份来源。

## 5. 状态设计

前端状态分两类：

| 状态 | 来源 | 说明 |
|---|---|---|
| Draft | 浏览器本地 | 简历、JD、题库过滤范围等未提交草稿 |
| Session | 后端 | 面试状态、当前题目、轮次、报告和检索 trace |

核心原则：

- 草稿可以在前端本地维护。
- 进入面试后，以后端 Session 为唯一事实源。
- 前端类型必须跟后端 JSON 字段对齐。
- 不在前端复制 Agent Graph 状态机。

当前关键类型在 `web/src/types.ts`：

- `Session`
- `WorkingMemory`
- `DifficultyState`
- `PendingDecision`
- `InterviewQuestion`
- `InterviewRound`
- `InterviewFeedback`
- `Report`
- `RetrievalTrace`
- `QuestionBankFilter`
- `AgentResponse`
- `ToolTrace`

## 6. API 对齐

前端通过 `web/src/apiClient.ts` 调用后端 REST API。核心接口包括：

| 方法 | 路径 | 前端用途 |
|---|---|---|
| `POST` | `/api/documents/parse-resume` | 上传并解析简历 |
| `POST` | `/api/interview/start` | 创建面试 Session |
| `POST` | `/api/interview/answer` | 提交回答 |
| `GET` | `/api/interview/sessions` | 获取会话列表 |
| `GET` | `/api/interview/sessions/:session_id` | 获取会话详情 |
| `GET` | `/api/users/:user_id/memory` | 获取只读长期用户画像，前端通过 `X-User-ID` 声明当前 owner |
| `POST` | `/api/agent/message` | 后端 Intent Router + Skill 消息入口 |
| `GET` | `/api/question-bank` | 题库预览 |
| `GET` | `/api/question-bank/facets` | 题库筛选项 |

`GET /api/users/:user_id/memory` 不只依赖 path `user_id`；前端请求会携带 `X-User-ID`，后端在 owner 与 path 用户不一致时拒绝读取。前端不得把当前开发模式 owner header 包装成生产级用户中心能力。

接口对齐要求：

- 前端新增字段前，先确认后端响应是否真实存在。
- 后端新增字段后，前端在 `types.ts` 中补类型。
- 老 Session 没有新字段时，前端必须能容忍 `undefined`。

## 7. SSE 事件消费

前端通过 `GET /api/interview/stream?session_id=...` 订阅服务端事件。

SSE 的职责是展示执行过程和状态变化，不替代 REST：

```text
REST：提交命令、获取当前 Session 快照
SSE：接收执行进度、节点事件、评分进度、报告生成进度
```

前端处理 SSE 时必须考虑：

- 连接断开后可以重新拉取 Session。
- SSE 事件只作为增量提示，最终状态以后端 Session 快照为准。
- 不因某个事件缺失而破坏页面渲染。

## 8. 错误处理

前端错误处理保持轻量：

- API 错误展示为用户可理解的 notice。
- API client 的错误对象保留 HTTP `status`，页面判断 404 等状态码时不得依赖错误文案。
- 文件上传失败后清空 input，允许重新选择。
- 面试提交失败时保留当前页面状态。
- 缺少报告或 Session 时展示空状态页面。

错误根因、降级原因和 trace 记录由后端负责。

## 9. 前端验证

当前前端验证命令：

```powershell
npm --prefix web run test
npm --prefix web run build
```

测试覆盖重点：

- API client 路径和参数。
- 草稿归一化。
- 路由辅助。
- 共享展示组件。
- 报告 helper。
- 报告页 `retrieval_trace` 展示。
- 面试页 / 报告页 `working_memory` 只读展示。
- Agent 页 `tool_trace` 只读展示和缺字段兼容。

## 10. 后续演进计划

后续前端演进应服务后端能力，不做独立复杂平台。

### 10.1 Intent Router + Skill 前端入口

后端已经提供 `/api/agent/message`，用于接收自然语言请求并路由到不同 skill。前端已接入轻量 `/agent` 页面：

```text
用户输入自然语言
  -> 调用 /api/agent/message
  -> 后端返回 intent、skill、结果或后续动作
  -> 前端按响应展示测验、讲解、项目润色或面试入口
```

前端不实现 intent 判断，只展示后端路由结果和可选 `tool_trace`。

### 10.2 Long-term Memory 展示

后端已有长期记忆基础层，并会在新 Session 启动时把历史弱点注入当前 `WorkingMemory`。前端已展示两类记忆信号：

- 面试页 / 报告页展示当前 Session 响应中的 `working_memory` 白名单字段，例如弱项、已确认技能、平均分、剩余轮次和降级原因。
- 画像页调用 `GET /api/users/:user_id/memory`，展示跨 Session 的长期强项、弱项、技能分数和最近建议。

前端只展示用户可见的画像摘要，不直接编辑底层 memory 结构，也不展示 `applied_nodes`、原始 `notes` 等后端内部运行标记。

### 10.3 动态难度展示

后端已有 `WorkingMemory.Difficulty`，并已影响 RAG 目标难度、`pick_next` prompt 和规则兜底。前端已在面试页和报告页展示当前难度、连续高分 / 低分 streak、追问预算、反思预算和技能覆盖摘要。

难度更新策略由后端负责，前端不自行根据分数调整题目难度。

### 10.4 MCP 工具结果展示

后端已有 mock MCP tool adapter 和显式注入的真实 GitHub 只读 client 边界，`project_polish` 可以调用 `github.project_analyze` 并返回项目润色建议。当前 `/api/agent/message` 响应通过 `result.content/actions` 表达业务结果，并通过顶层 `tool_trace` 返回工具调用摘要。

前端已在 Agent 页只读展示 `tool_trace`，字段包括工具名、权限、状态、错误类别、耗时和摘要。缺少 `tool_trace` 时保持原有页面行为，不从文案里反推工具状态。

前端不直接调用外部 MCP 服务。

## 11. 非目标

- 不在前端实现 Agent Graph。
- 不在前端实现 RAG、rerank 或 LLM 调用。
- 不在前端直接访问 PostgreSQL、Redis、向量库或 MCP 服务。
- 不声明当前系统支持 sub-agent runtime。
- 不把前端设计成独立低代码平台或通用 Agent 控制台。
