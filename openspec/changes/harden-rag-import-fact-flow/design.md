## Context

Interview Agent 当前后端已有题库导入暂存、LLM 补全、Agent 审核建议、commit 写入正式题库、embedding 写入、RAG 检索 trace、`cmd/rag-eval` 和 `pick_next/probe_*` 面试节点。最近一次收口已经修复了真实 LLM JSON schema 漂移，但这些规则仍主要存在于代码和测试中，没有形成可验证 contract。

参考项目 InterWise 的可取点不是技术栈，而是边界设计：题库导入先生成可审查包，发布走后端事实源；RAG 检索结果不是直接拼 prompt，而是进入下一问决策；离线评测以真实 query、候选池和人工标注闭环推动策略变更。上线项目还需要可运营边界：失败可重试、发布可审计、策略可回归、成本可保护、数据可脱敏导出。

本项目约束：

- 后端是事实源；前端只是读取和触发动作。
- 本 change 不修改前端，除非后续用户明确要求。
- Session JSON、HTTP 响应和现有题库格式必须兼容；新增字段优先 `omitempty` 或限于 CLI 输出。
- 正式题库仍是 `question_bank`，embedding 是可重建派生数据。
- RAG 用于决定下一题/追问，不是知识库问答。

## Goals / Non-Goals

**Goals:**

- 把题库 JSON 导入兼容规则固化为版本化 contract 和 golden tests。
- 将题库 commit 设计为发布事务，覆盖 review 状态、质量门禁、embedding sync、reindex/retry、commit summary 和审计事实。
- 让 RAG eval 能使用真实运行 trace 导出脱敏 query，并生成 candidate pool 供人工标注。
- 在出题/追问节点增加 Runtime Retrieval Decision Policy，处理低信息回答、弱召回、已用题排除、动态难度、技能覆盖和上下文注入策略。
- 记录可诊断事实：为什么补救追问、为什么切换知识点、为什么不注入题库上下文。
- 为后续 quota/cost guard、Admin operations、observability 和数据保留策略预留后端事实字段和边界。

**Non-Goals:**

- 第一阶段不实现额度、用户配额、Admin UI、运营统计页面；但设计必须保留后端事实边界，避免后续上线能力无处接入。
- 不切换到 Qdrant、不替换 BGE-M3/pgvector。
- 不重写前端题库工作台。
- 不让 LLM 直接绕过 review/commit 写正式题库。
- 不把参考项目的 Java/Vue 代码移植到本项目。

## Decisions

### 1. 题库导入 contract 以现有 `Item` 为正式题库格式，但导入包必须版本化

保留 `questionbank.Item` 当前字段作为正式题库写入格式。新增 `Import Package v1` contract 文档和测试。导入包应能表达：

- `schema_version`
- `source_ref`
- `items`
- `validation_report`
- `review_policy`
- 字段路径级错误
- 原始值摘要或原始 item 快照

为了兼容现有用户工作流，直接上传 `[]Item` 或 `{ "items": [...] }` 仍然是合法的 legacy package，系统在解析层归一化到 v1 语义。contract 测试覆盖：

- 顶层 `[]Item` 或 `{ "items": [...] }`
- `difficulty`: number 或数字字符串
- `rubric`: object、string array、string
- `tags`、`expected_points`、`role_tags`、`follow_up_hints`: string array 或分隔字符串
- 字段存在但类型不支持时必须报错，不能静默转空

理由：现有导入、review、commit、embedding 已围绕 `Item` 打通。另起 atom 表会扩大数据库和前端影响面；版本化导入包能获得上线所需契约，而不破坏正式题库事实格式。

备选方案：新增独立 import package schema。暂不采用；可作为后续对外导入包能力，但当前先把运行中已有 JSON 行为固化。

### 2. Review/commit 边界升级为发布事务

导入解析只负责归一化和报错，不负责发布。commit 是发布事务，进入正式题库必须满足：

- 暂存项 valid
- 人工 review accepted
- Agent review 非 rejected/needs_human_review 阻塞态
- commit 阶段重复和脏题质量门禁通过
- embedding 写入可记录状态
- commit summary 记录 matched/imported/skipped/embedded/failed 和原因
- embedding/reindex 失败可重试，不让正式题库事实和派生索引静默分叉

理由：避免脚本、LLM 或外部来源绕过后端门禁。

### 3. RAG eval 新增真实 query 导出和 candidate pool，但不改变 live retriever

新增 CLI 子命令或现有 `cmd/rag-eval` 模式：

```text
export-queries -> build-candidate-pool -> 人工标注 -> calculate metrics
```

导出来源优先使用后端已持久化或可读取的 `RetrievalTrace` / session 事实。导出时脱敏 query 文本，去除邮箱、手机号、URL、明显 secret 片段。candidate pool 合并多个来源：

- live retriever topK
- stage trace 中 vector/text/rule/fusion 候选
- 简单关键词候选
- 少量固定随机负例

理由：真实 query 能暴露静态 golden case 看不到的问题；candidate pool 让人工标注覆盖“模型认为相关”和“关键词可能相关”的候选。

### 4. Runtime Retrieval Decision Policy 放在 nodes 层，使用 retriever trace 但不污染 retriever contract

新增或抽取一个后端策略组件，例如 `internal/nodes/retrieval_decision.go`。输入：

- 当前用户回答
- 历史回答/当前 round
- `Session.CandidatePool`
- `Session.RetrievalTrace`
- 已问主问题和追问题 ID
- WorkingMemory 中的 skill coverage、dynamic difficulty、degraded reasons
- 配置阈值：高置信、最低上下文分数、上下文数量

输出：

- `strategy`: deepen/remedy/switch_topic/fallback/end
- `include_context`
- `selected_candidate_ids`
- `consumed_candidate_ids`
- `degraded_reason`
- `reason`

理由：`internal/retriever` 应继续只负责召回和排序；“下一问该怎么问”是面试业务决策，应留在 nodes/domain 边界。

### 5. 已用题排除必须在检索前和选择前双层执行

- `retrieve_rag` 构造 query/filter 时应尽量把已用 question IDs 传给 retriever。
- `pick_next` 和追问决策前再次过滤 `CandidatePool`，防止旧 Session、fallback pool 或 retriever 不支持 exclude 时重复出题。

理由：排除是用户体验和评测事实，不应依赖单个 retriever 实现完全正确。

### 6. 低信息回答和弱召回先用确定性规则

先实现轻量 deterministic signal：

- 低信息短语：不会、不知道、不清楚、没做过、不了解等
- 极短回答且无技术词
- 召回 top score 低于最低上下文阈值
- 连续低信息回答

理由：这个判断必须稳定、可测、可解释。不要先引入 LLM 判断，否则调试困难、成本高。

### 7. 上线运维先设计边界，分阶段实现

本 change 的实现优先级仍是后端事实流，但设计必须预留上线能力：

- quota/cost guard：未来在真实 LLM、embedding、RAG eval 导出入口接入。
- admin operations：未来 UI 只调用后端 import/review/publish/reindex/audit API，不绕过事实源。
- observability：RAG decision、zero-hit、fallback、embedding failure、schema error 都应能落入 trace 或日志。
- data retention：真实 query 导出默认脱敏，不自动进入 Git。

理由：上线能力不是 UI 美化，而是后端边界。如果现在设计不留入口，后续会把运营能力补成旁路。

## Risks / Trade-offs

- [Risk] 真实 query 导出可能泄露用户内容。  
  Mitigation: 默认脱敏；导出文件不自动提交；测试覆盖邮箱、手机号、URL、secret 片段清洗。

- [Risk] candidate pool 标注流程增加工具复杂度。  
  Mitigation: 先做 JSONL 文件输入输出，不引入服务端 UI；保持脚本可独立测试。

- [Risk] 追问决策阈值不适合所有岗位。  
  Mitigation: 阈值配置化，默认保守；trace 记录决策理由，后续用 eval 调整。

- [Risk] 已用题排除可能导致候选池为空。  
  Mitigation: 记录 degraded reason；允许 fallback 题继续面试，但不得重复已明确使用的题。

- [Risk] contract 收紧会让部分宽松 JSON 导入失败。  
  Mitigation: 失败信息带字段名；文档列清兼容格式；字段不支持时失败比静默丢数据更正确。

## Migration Plan

1. 先补 OpenSpec delta specs 和后端设计文档。
2. 实现题库导入 contract 测试和文档，不改变数据库 schema。
3. 扩展 `cmd/rag-eval` 的文件型工具链，先用测试 fixture 验证。
4. 引入 nodes 层决策组件，并接入 `retrieve_rag` / `pick_next` / 追问链路。
5. 更新 `docs/SDD-Backend.md` 和 `docs/code-changes/*`。
6. 运行 `go test ./...`，必要时补 `go run ./cmd/rag-eval ...` fixture 验证。

Rollback：各步骤应保持小提交；若决策层影响运行时，可通过配置关闭上下文决策增强，回退到现有 candidate pool 选择行为。

## Open Questions

- 真实 query 导出优先读取哪个事实源：session JSON、Postgres sessions 表，还是 RAG eval 现有输入文件？实现前需要根据当前存储代码确定最小入口。
- 追问决策的 score 来源是否总能从 `RetrievalTrace` 映射回 `CandidatePool`？如果不能，第一版应允许缺 score 时按弱召回处理。
