# Comet Design Handoff

- Change: harden-rag-import-fact-flow
- Phase: design
- Mode: compact
- Context hash: fd75416a8c57c3da13ccc9bea4f4214684effb424eb44b1ab50ed5af4832686e

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/harden-rag-import-fact-flow/proposal.md

- Source: openspec/changes/harden-rag-import-fact-flow/proposal.md
- Lines: 1-39
- SHA256: 1ae377ce02a88621f3606059333e5dceb124cfa35918a69dd9e3f5b68282461e

```md
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
```

## openspec/changes/harden-rag-import-fact-flow/design.md

- Source: openspec/changes/harden-rag-import-fact-flow/design.md
- Lines: 1-174
- SHA256: b820a78620b6727db04f6e640da1789bd8108cc63e5caa2e51bba1cf829de559

[TRUNCATED]

```md
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
```

Full source: openspec/changes/harden-rag-import-fact-flow/design.md

## openspec/changes/harden-rag-import-fact-flow/tasks.md

- Source: openspec/changes/harden-rag-import-fact-flow/tasks.md
- Lines: 1-32
- SHA256: 7659fa950a93485ea25910d680ed52ba9386afcecb122b18be1a7b6a920f91aa

```md
## 1. 题库导入 Contract 和发布事务

- [ ] 1.1 梳理并补充 `internal/questionbank` JSON 导入 contract 测试，覆盖版本化导入包、legacy 数组、wrapped items、字段兼容、中文分隔符和坏类型报错。
- [ ] 1.2 增加题库导入 contract 文档或测试 fixture，明确 schema version、source ref、review policy、允许字段形态、归一化结果、字段路径错误和原始值摘要。
- [ ] 1.3 将 commit 明确为发布事务，复核或补齐 matched/imported/skipped/embedding synced/embedding failed/failure reasons 的 summary 测试。
- [ ] 1.4 复核 review/commit 门禁测试，确认解析、暂存、人工 review、Agent review、重复/脏题阻止、embedding/reindex retry 边界不被绕过。

## 2. RAG Eval 真实 Query 和 Candidate Pool

- [ ] 2.1 调研当前 session/RetrievalTrace 持久化入口，确定真实 query 导出的最小事实源。
- [ ] 2.2 为 `cmd/rag-eval` 增加真实 query 导出模式，输出脱敏 JSONL，并覆盖邮箱、手机号、URL、secret 片段清洗测试。
- [ ] 2.3 为 `cmd/rag-eval` 增加 candidate pool 构建模式，合并 live/stage/keyword/random-negative 候选并保留来源 rank/score。
- [ ] 2.4 增加标注输入指标计算或复用现有指标计算路径，覆盖 recall@k、hit@k、MRR 或 nDCG 的回归测试。

## 3. Runtime Retrieval Decision Policy

- [ ] 3.1 新增 nodes 层 Runtime Retrieval Decision Policy，输入回答、历史、CandidatePool、RetrievalTrace、已用题、动态难度、技能覆盖和阈值，输出 strategy/include_context/selected/consumed/reason/degraded reason。
- [ ] 3.2 在 `retrieve_rag` 或相关 query 构造中传递已用题排除条件，并在 `pick_next` 前再次过滤已用候选。
- [ ] 3.3 接入 `pick_next` 和追问链路，使低信息回答、弱召回、正常深挖、候选为空 fallback 和 end 策略都记录可诊断原因。
- [ ] 3.4 增加节点级和 graph loop 回归测试，覆盖低信息+高置信、低信息+弱召回、正常回答+可用召回、动态难度/技能覆盖参与、已用题排除、旧 Session 兼容。

## 4. 上线观测和后续运维边界

- [ ] 4.1 为 RAG decision、zero-hit、fallback、embedding failure、schema error 明确 trace/log/eval 输出位置，避免后续 observability 旁路实现。
- [ ] 4.2 在设计和代码边界中预留 quota/cost guard、Admin operations 和脱敏数据保留接入点，但不实现前端 UI。

## 5. 文档和验证

- [ ] 5.1 更新 `docs/SDD-Backend.md`，记录版本化题库导入 contract、发布事务、RAG eval 真实 query 工具链、Runtime Retrieval Decision Policy 和上线运维边界。
- [ ] 5.2 按项目规则新增或更新 `docs/code-changes/MM-DD-*.md`，基于真实 diff 记录运行时行为变化。
- [ ] 5.3 运行最小相关测试：`go test ./internal/questionbank ./internal/nodes ./cmd/rag-eval -count=1`。
- [ ] 5.4 运行全量验证：`go test ./...` 和必要的 `openspec validate harden-rag-import-fact-flow --strict`。
```

## openspec/changes/harden-rag-import-fact-flow/specs/question-bank-import-enrichment/spec.md

- Source: openspec/changes/harden-rag-import-fact-flow/specs/question-bank-import-enrichment/spec.md
- Lines: 1-80
- SHA256: d9d1ec963acea3c422618e52e921e35dda687fc9d4eb161871e07b66202870c0

```md
## ADDED Requirements

### Requirement: 题库 JSON 导入 contract 必须版本化且稳定可验证

系统 MUST 明确定义本地题库 JSON 导入的版本化输入契约、字段兼容规则和错误语义，并用自动化测试覆盖该 contract。

#### Scenario: 接受版本化导入包

- **WHEN** 用户导入包含 `schema_version`、`source_ref`、`items`、`validation_report` 或 `review_policy` 的题库导入包
- **THEN** 系统 MUST 按声明版本解析导入包
- **AND** 系统 MUST 保留 source、validation 和 review policy 相关事实用于暂存 review 和后续诊断
- **AND** 系统 MUST NOT 因额外 contract 元数据破坏正式 `Item` 写入格式

#### Scenario: 接受标准数组和 wrapped items

- **WHEN** 用户导入 JSON 题库文件
- **AND** JSON 顶层是题目数组或 `{ "items": [...] }`
- **THEN** 系统 MUST 将 legacy 输入按当前导入包 contract 语义归一化
- **AND** 后续 normalize、暂存、review 和 commit 流程 MUST 不依赖顶层包装差异

#### Scenario: 兼容真实 LLM 输出的标量漂移

- **WHEN** 导入 JSON 中 `difficulty` 是数字字符串
- **AND** `rubric` 是 object、string array 或 string
- **AND** `tags`、`expected_points`、`role_tags` 或 `follow_up_hints` 是 string array 或分隔字符串
- **THEN** 系统 MUST 将这些字段归一化为正式 `Item` 字段类型
- **AND** 分隔字符串 MUST 至少支持英文逗号、英文分号、竖线、中文逗号、中文分号和顿号

#### Scenario: 不支持的字段类型必须报错

- **WHEN** JSON 字段存在
- **AND** 该字段不是 contract 允许的类型
- **THEN** 系统 MUST 拒绝本次解析
- **AND** 错误信息 MUST 包含出错字段名
- **AND** 错误信息 SHOULD 包含字段路径和原始值摘要
- **AND** 系统 MUST NOT 将坏字段静默转换为空值

#### Scenario: contract 变化必须有 golden 或回归测试

- **WHEN** 题库 JSON 导入 contract 新增兼容格式、错误语义或字段转换规则
- **THEN** 系统 MUST 增加或更新对应自动化测试
- **AND** 测试 MUST 覆盖成功归一化和失败报错两类路径

### Requirement: Review 和 commit 边界必须形成发布事务

系统 MUST 保持导入解析、暂存 review 和正式 commit 的职责边界，并将 commit 作为可诊断发布事务处理。任何 JSON 输入、LLM 输出、脚本产物或来源适配器都不得直接写入正式题库。

#### Scenario: 导入解析只产生暂存项

- **WHEN** 用户导入本地题库 JSON 或源文档生成题
- **THEN** 系统 MUST 先创建 import job 和暂存 import items
- **AND** 系统 MUST NOT 在解析阶段写入正式 `question_bank`

#### Scenario: commit 只写入满足门禁的题目

- **WHEN** 用户提交 import job
- **THEN** 系统 MUST 只写入 valid、人工 accepted、Agent review 非阻塞、非重复且质量门禁通过的题目
- **AND** 系统 MUST 跳过 rejected、needs_human_review、重复或高风险脏题
- **AND** 被跳过题目 SHOULD 保留可诊断原因

#### Scenario: commit summary 记录发布事务结果

- **WHEN** commit 完成、部分完成或失败
- **THEN** 系统 MUST 返回或持久化 commit summary
- **AND** summary MUST 至少区分 matched、imported、skipped、embedding synced、embedding failed 和 failure reasons
- **AND** summary MUST 能支持维护者判断是否需要重试、reindex 或人工处理

#### Scenario: embedding 失败不得静默分叉

- **WHEN** 题目已写入正式 `question_bank`
- **AND** embedding 写入或维度校验失败
- **THEN** 系统 MUST 保留正式题库事实
- **AND** 系统 MUST 标记 embedding 状态和错误原因
- **AND** 系统 MUST 提供后续 reindex 或 retry 的事实入口

#### Scenario: 来源适配器不得直接发布

- **WHEN** 系统通过脚本、Skill、MCP、外部链接或本地文档获得题库来源
- **THEN** 该来源 MUST 进入现有导入暂存流程
- **AND** 适配器 MUST NOT 直接写入正式 `question_bank`
```

## openspec/changes/harden-rag-import-fact-flow/specs/rag-retrieval-enhancement/spec.md

- Source: openspec/changes/harden-rag-import-fact-flow/specs/rag-retrieval-enhancement/spec.md
- Lines: 1-117
- SHA256: 5c736e0c93ac71f9ed201fd5bb612927193f60018d34152fe6c0a1c72f108d9c

[TRUNCATED]

```md
## ADDED Requirements

### Requirement: RAG eval 必须支持真实查询导出

系统 MUST 提供后端可运行的 RAG eval 导出能力，从真实会话或检索 trace 中生成脱敏 query 数据集，用于离线评测和策略回归。

#### Scenario: 导出真实 RAG query

- **WHEN** 维护者运行 RAG eval query 导出命令
- **AND** 输入事实源包含 Session、RetrievalTrace 或等价检索记录
- **THEN** 系统 MUST 输出 JSONL query 数据集
- **AND** 每条记录 MUST 至少包含稳定 query id、query text、岗位或技能范围、阶段信息和来源引用
- **AND** 命令 MUST NOT 修改 session、question bank 或 embedding 数据

#### Scenario: 导出时脱敏敏感内容

- **WHEN** query text 或来源字段包含邮箱、手机号、URL、token、api key、password 或 secret 片段
- **THEN** 系统 MUST 在导出数据中替换为占位符
- **AND** 脱敏 MUST 在写入输出文件前完成

#### Scenario: 无有效 query 时返回可诊断结果

- **WHEN** 输入事实源不存在有效技术面试 query
- **THEN** 命令 MUST 输出空数据集或返回明确错误
- **AND** 结果 MUST 说明过滤原因或输入统计

### Requirement: RAG eval 必须支持 candidate pool 标注输入

系统 MUST 支持为导出的真实 query 构建 candidate pool，合并多路候选，供人工标注和指标计算使用。

#### Scenario: 构建候选池

- **WHEN** 维护者基于 query 数据集运行 candidate pool 构建命令
- **AND** 可用 retriever 或离线题库输入存在
- **THEN** 系统 MUST 为每个 query 输出候选 question ids
- **AND** 每个候选 MUST 记录来源，例如 vector、text、rule、fusion、keyword 或 random_negative
- **AND** 输出 MUST 是可人工标注的稳定 JSONL 格式

#### Scenario: 候选池去重并保留来源证据

- **WHEN** 同一 question id 被多个候选来源命中
- **THEN** 系统 MUST 在 candidate pool 中只保留一个候选项
- **AND** 系统 MUST 保留所有命中来源和各来源 rank 或 score

#### Scenario: 标注数据可计算检索指标

- **WHEN** candidate pool 已经带有人工 relevance 标注
- **THEN** RAG eval MUST 能计算 recall@k、hit@k、MRR 或 nDCG 中至少一组排序指标
- **AND** 指标计算 MUST NOT 改变 live retriever 或面试运行时行为

### Requirement: 面试追问必须使用 Runtime Retrieval Decision Policy

系统 MUST 在出题和追问链路中使用后端 Runtime Retrieval Decision Policy，基于回答信息量、召回强度、已用题、动态难度、技能覆盖和候选证据决定下一问策略。

#### Scenario: 决策层输出稳定策略事实

- **WHEN** 决策层处理一次出题或追问决策
- **THEN** 输出 MUST 包含 strategy、include_context、selected_candidate_ids、consumed_candidate_ids 和 reason
- **AND** strategy MUST 至少支持 `deepen`、`remedy`、`switch_topic`、`fallback` 和 `end`
- **AND** 降级时输出 SHOULD 包含 degraded reason

#### Scenario: 低信息回答且召回可靠时补救追问

- **WHEN** 候选人当前回答被判定为低信息回答
- **AND** RAG 候选存在高置信命中
- **THEN** 决策层 MUST 允许注入最相关题库上下文
- **AND** 下一问策略 MUST 倾向低难度补救追问
- **AND** 系统 MUST 记录补救追问原因

#### Scenario: 低信息回答且召回弱时切换知识点

- **WHEN** 候选人当前回答被判定为低信息回答
- **AND** RAG 候选为空或最高置信度低于最低上下文阈值
- **THEN** 决策层 MUST NOT 强行注入题库上下文
- **AND** 下一问策略 MUST 倾向切换知识点或使用 fallback
- **AND** 系统 MUST 记录弱召回或切换原因

#### Scenario: 正常回答且召回可用时深挖

- **WHEN** 候选人当前回答不是低信息回答
```

Full source: openspec/changes/harden-rag-import-fact-flow/specs/rag-retrieval-enhancement/spec.md

