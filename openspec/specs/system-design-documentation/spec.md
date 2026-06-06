# system-design-documentation Specification

## Purpose
定义 Interview Agent 前端和后端系统设计文档的维护要求，确保 SDD 描述当前真实实现、模块边界、验证方式和未实现能力边界。

## Requirements
### Requirement: 维护当前前端系统设计文档

项目 MUST 维护一份前端 Software Design Document，用于描述当前已经实现的 Web 工作台架构，并避免声称尚未实现的能力。

#### Scenario: 读者查看当前前端系统设计

- **WHEN** 维护者打开 `docs/SDD-Frontend.md`
- **THEN** 文档说明前端目标、页面结构、路由、草稿状态、API 调用、SSE 事件消费、报告展示、错误提示和前端验证命令
- **AND** 文档区分前端已实现能力、非目标和未来扩展

#### Scenario: 前端 SDD 与仓库实现保持一致

- **WHEN** 文档引用实现模块
- **THEN** 文档使用仓库中真实存在的前端路径，例如 `web/src`、`web/src/main.tsx`、`web/src/candidatePages.tsx`、`web/src/apiClient.ts`、`web/src/types.ts` 和 `web/src/reportView.ts`
- **AND** 文档不引入新的运行时依赖、API 变更或行为变更

### Requirement: 维护当前后端系统设计文档

项目 MUST 维护一份后端 Software Design Document，用于描述当前已经实现的 Go 服务端架构，并避免声称尚未实现的能力。

#### Scenario: 读者查看当前后端系统设计

- **WHEN** 维护者打开 `docs/SDD-Backend.md`
- **THEN** 文档说明后端目标、HTTP API、SSE 服务端、Session 状态模型、Agent Graph 流程、RAG pipeline、LLM/rerank 边界、存储、降级行为、可观测性和后端验证命令
- **AND** 文档区分后端已实现能力、非目标和未来扩展
- **AND** 文档不得把后续 Codex 开发中可能使用的 sub-agent 写成当前项目运行时能力

#### Scenario: 后端 SDD 与仓库实现保持一致

- **WHEN** 文档引用实现模块
- **THEN** 文档使用仓库中真实存在的后端路径，例如 `cmd/server`、`internal/httpapi`、`internal/domain`、`internal/graphs`、`internal/nodes`、`internal/retriever`、`internal/llm`、`internal/parser`、`cmd/rag-eval` 和 `cmd/agent-verify`
- **AND** 文档不引入新的运行时依赖、API 变更或行为变更

### Requirement: SDD 必须声明内部试用边界和非生产限制

项目 MUST 在系统设计文档中明确内部团队试用能力边界，避免把内部试用、mock 工具、开发 fallback 或 Codex 开发协作能力描述成对外生产能力。

#### Scenario: 后端 SDD 描述内部试用边界

- **WHEN** 读者查看 `docs/SDD-Backend.md`
- **THEN** 文档 MUST 说明内部试用支持的身份来源、真实 GitHub 工具显式启用、长期记忆观测和 smoke 门禁
- **AND** 文档 MUST 声明未实现完整 JWT/OIDC、租户体系、完整 MCP runtime 或运行时 sub-agent

#### Scenario: 前端 SDD 描述内部试用边界

- **WHEN** 读者查看 `docs/SDD-Frontend.md`
- **THEN** 文档 MUST 说明前端只展示后端返回的状态、限制和 trace
- **AND** 文档 MUST 声明前端不直接调用 GitHub、MCP 服务、用户中心或长期记忆写接口
