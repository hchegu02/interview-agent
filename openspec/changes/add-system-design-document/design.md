## 背景

Interview Agent 已经具备前端工作台、Go HTTP API、SSE 事件流、Session 聚合根、Agent Graph、RAG 检索、LLM gateway、题库治理和验证命令。当前 README 适合快速介绍和启动项目，`docs/code-changes` 与 `docs/superpowers` 记录历史演进，但缺少稳定的前端和后端当前系统设计文档。

SDD 应面向维护者、面试讲解和后续工程交接，描述“当前真实实现是什么”，而不是未来蓝图。前端和后端职责不同，应拆成两份文档，避免一份总文档过长且边界含糊。

## 目标 / 非目标

**目标：**

- 新增 `docs/SDD-Frontend.md`，集中说明当前前端系统设计。
- 新增 `docs/SDD-Backend.md`，集中说明当前后端系统设计。
- 明确前端页面状态、API 协作、SSE 消费、报告展示和前端验证边界。
- 明确后端 API、Session、Agent Graph、RAG、LLM/rerank、降级、观测和后端验证边界。
- 保持内容和现有 README、代码目录一致。
- 让读者能区分已实现能力、非目标和后续扩展。

**非目标：**

- 不修改业务代码。
- 不新增 API、配置、数据库 schema 或依赖。
- 不把项目描述成通用 Coding Agent 平台。
- 不声称已实现完整 OpenClaw Gateway、daemon runtime、云端 runtime、Sandbox 或 Helm 生产部署。
- 不把后续 Codex 开发中可能使用的 sub-agent 写成当前项目运行时能力。

## 设计决策

### 决策 1：拆分创建 `docs/SDD-Frontend.md` 和 `docs/SDD-Backend.md`

SDD 单独成文，不塞进 README。README 保持“快速理解和启动”，两份 SDD 承担“系统设计说明”。前端文档聚焦用户工作台和状态流，后端文档聚焦 API、Session、Graph、RAG 和验证。这样不会让 README 继续膨胀，也便于按角色交接。

备选方案：只写一份 `docs/SDD.md`。放弃原因是前后端关注点差异明显，一份文档容易变成混杂总览；后续修改前端页面或后端 Graph/RAG 时也不好定位维护位置。

### 决策 2：只描述已实现架构

文档只描述当前仓库中已有的模块和命令。前端 SDD 引用 `web/src`、`web/src/apiClient.ts`、`web/src/candidatePages.tsx`、`web/src/types.ts` 等真实路径；后端 SDD 引用 `internal/httpapi`、`internal/domain`、`internal/graphs`、`internal/nodes`、`internal/retriever`、`internal/llm`、`cmd/rag-eval`、`cmd/agent-verify`。

备选方案：同时写未来规划。保留少量“后续扩展”，但必须放在非目标或未来扩展章节，避免和已实现能力混在一起。

### 决策 3：sub-agent 只作为开发协作预留，不作为系统能力

后续如果用 Codex sub-agent 辅助开发，那属于开发流程和任务拆分方式，不属于 Interview Agent 当前运行时架构。SDD 可以在“后续演进”中说明接口设计要保持清晰，便于未来由不同开发代理分别维护前端、HTTP API、RAG、Graph、验证等模块；但不能写成当前服务已经支持 sub-agent 调度。

备选方案：把 sub-agent 写进 Agent Graph 架构。放弃原因是当前项目运行时是 `internal/graph` + `internal/nodes` 的业务 Graph，不是 sub-agent runtime；混写会造成能力夸大。

### 决策 4：按工程边界组织章节

两份 SDD 都按工程边界组织。前端按页面、状态、接口、SSE、报告和验证组织；后端按 HTTP API、Session、Graph、RAG、LLM、存储、降级、观测和验证组织。这样读者可以直接从文档跳到对应代码目录。

备选方案：按用户流程组织。用户流程直观，但容易掩盖模块职责和数据边界；SDD 更需要服务维护。

## 风险 / 取舍

- 文档过细导致后续维护成本高 -> 控制在当前稳定架构和关键数据流，不复制完整代码细节。
- 文档夸大项目能力 -> 明确非目标，尤其是 Gateway、daemon、Sandbox、Helm 等未实现能力。
- 两份文档内容重复 -> 前端只写浏览器侧职责，后端只写服务端职责；REST/SSE 协作可在两边分别从各自视角描述。
