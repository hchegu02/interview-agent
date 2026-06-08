## Why

当前题库导入、RAG 评测和面试追问决策已经能跑通，但后端事实边界还不够硬：JSON 导入兼容规则散落在实现里，RAG eval 主要依赖静态 golden case，出题/追问节点还缺少统一的“低信息回答、弱召回、已用题排除”决策层。

这会导致三个现实问题：坏导入 schema 容易变成 review 噪声，RAG 策略优化缺少真实查询证据，面试追问容易把检索结果机械塞进下一问。面向业务上线，目标不是最小工程量，而是建立可运营、可回滚、可诊断、可评测的题库/RAG 事实流闭环。

## What Changes

- 增加版本化题库导入 contract，明确 JSON 输入形态、字段兼容转换、错误语义、review/commit 边界、audit/retry 事实和回归测试要求。
- 将题库 commit 明确为发布事务：staged item、human review、Agent review、重复/质量门禁、embedding sync、reindex/retry 和 commit summary 必须形成可诊断事实。
- 扩展 RAG eval，支持从真实检索 trace / session 事实中导出脱敏 query，并构建 candidate pool 供人工标注和指标计算。
- 在出题/追问链路引入 Runtime Retrieval Decision Policy，根据低信息回答、弱召回、已用题排除、动态难度、技能覆盖和召回分数决定是否注入上下文、补救追问、切换知识点、fallback 或结束。
- 为上线运维预留后端事实边界：quota/cost guard、admin operations、observability、脱敏导出和数据保留作为后续阶段接入点；本 change 仍优先实现后端事实流，不做前端美化。

## Capabilities

### New Capabilities

无。本 change 不引入新的顶层 capability，避免把已有题库导入和 RAG 检索边界拆散。

### Modified Capabilities

- `question-bank-import-enrichment`: 增加版本化题库导入 contract、字段兼容和错误语义、review/commit 发布事务、embedding/reindex 诊断边界要求。
- `rag-retrieval-enhancement`: 增加真实 query 导出、candidate pool 标注脚本、Runtime Retrieval Decision Policy、已用题排除和上线观测边界要求。

## Impact

- 后端代码：
  - `internal/questionbank`：导入 contract、校验、review/commit 发布事务、embedding/reindex 诊断边界测试。
  - `cmd/rag-eval` 或新增后端 CLI：真实 query 导出、candidate pool 构建、标注输入输出。
  - `internal/retriever`、`internal/nodes/retrieve_rag.go`、`internal/nodes/pick_next.go`、追问相关节点：Runtime Retrieval Decision Policy 和已用题排除。
- 文档和规格：
  - 更新 OpenSpec delta spec。
  - 如运行时行为变化，同步 `docs/SDD-Backend.md` 和 `docs/code-changes/*`。
- 接口兼容：
  - 不修改前端。
  - 新增 JSON/trace 字段必须 `omitempty` 或仅用于 CLI/内部诊断，避免破坏旧 Session 和 HTTP 响应。
- 依赖：
  - 优先复用现有 Go 标准库和项目已有 RAG eval 结构；不得为 candidate pool 引入重量级外部服务。
