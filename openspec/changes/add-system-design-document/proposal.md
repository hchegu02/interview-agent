## 为什么

当前 README 已经覆盖项目能力、启动方式和验证命令，但缺少一份稳定的 Software Design Document。前端页面流、后端 API、Session 状态、Agent Graph、RAG、SSE、降级和验证体系散落在 README、历史变更文档和代码中，不利于面试讲解、后续维护和工程交接。

新增前端和后端两份 SDD，把当前实现按职责边界收敛成正式设计文档，明确页面状态、接口协作、后端模块、关键数据流和非目标。

## 变更内容

- 新增前端软件设计文档 `docs/SDD-Frontend.md`。
- 新增后端软件设计文档 `docs/SDD-Backend.md`。
- 前端 SDD 覆盖页面结构、路由、草稿状态、API 调用、SSE 消费、报告展示和前端验证。
- 后端 SDD 覆盖 HTTP API、Session 数据模型、Agent Graph、RAG、LLM/rerank、错误降级、可观测性和后端验证。
- 在 OpenSpec change 中记录本次文档变更的 proposal、spec、design 和 tasks。
- 不修改业务代码、不修改 API、不引入新依赖。

## 能力范围

### 新增能力

- `frontend-system-design-documentation`: 项目维护一份正式前端 SDD，用于说明 Web 工作台设计、状态流、接口协作和页面边界。
- `backend-system-design-documentation`: 项目维护一份正式后端 SDD，用于说明 API、Session、Agent Graph、RAG、LLM、降级、观测和验证边界。

### 修改能力

<!-- 不修改已有 spec 需求。 -->

## 影响范围

- `docs/SDD-Frontend.md`: 新增正式前端软件设计文档。
- `docs/SDD-Backend.md`: 新增正式后端软件设计文档。
- `openspec/changes/add-system-design-document/`: 新增本次 OpenSpec 变更产物。
