# rag-retrieval-enhancement Specification

## Purpose

定义 RAG 检索增强能力，包括 Query Rewriting、HyDE 实验模式、PGVector 运行诊断、题库查询计划诊断和 Go 后端题库试用的检索评估门禁，确保增强策略可观测、可回退，并不破坏正式候选选择流程。
## Requirements
### Requirement: RAG 检索应支持查询改写

系统 MUST 支持在 RAG query embedding 前进行可配置 Query Rewriting，以把岗位、技能缺口、动态难度和题库过滤条件转成更适合题库检索的查询文本。

#### Scenario: 查询改写成功

- **WHEN** `retrieve_rag` 已构造基础查询
- **AND** Query Rewriting 已启用
- **THEN** 系统 MUST 在 embedding 前生成 rewritten query
- **AND** embedding 和文本召回应使用 rewritten query
- **AND** RetrievalTrace MUST 记录 original query 和 rewritten query

#### Scenario: 查询改写失败时降级

- **WHEN** Query Rewriting 调用失败、超时或返回空结果
- **THEN** 系统 MUST 使用 original query 继续检索
- **AND** RetrievalTrace MUST 记录 rewrite fallback reason
- **AND** 面试流程 MUST NOT 因查询改写失败而中断

### Requirement: RAG 检索应支持 HyDE 实验模式

系统 MUST 提供 HyDE 配置模式，用于生成题库条目风格的假设文档并评估其对召回质量的影响。

#### Scenario: HyDE shadow 模式不影响正式候选选择

- **WHEN** HyDE mode 为 `shadow`
- **THEN** 系统 MAY 生成 HyDE 文本和对应 embedding
- **AND** 系统 MUST 记录 HyDE 诊断信息
- **AND** 正式 CandidatePool MUST 仍由非 HyDE live path 决定

#### Scenario: HyDE enabled 模式参与检索

- **WHEN** HyDE mode 为 `enabled`
- **AND** 配置明确允许 HyDE 影响 live retrieval
- **THEN** 系统 MAY 使用 HyDE embedding 参与 vector recall
- **AND** RetrievalTrace MUST 标记 HyDE 参与了最终检索

#### Scenario: HyDE 失败时回退

- **WHEN** HyDE 生成或 embedding 失败
- **THEN** 系统 MUST 回退到 non-HyDE retrieval path
- **AND** RetrievalTrace MUST 记录 HyDE fallback reason

### Requirement: Go 后端题库试用应有 RAG 策略对比门禁

系统 MUST 为 Go 后端单岗位内部试用提供 RAG eval 对比，至少覆盖 baseline、query rewrite 和 HyDE shadow 诊断路径，并能输出检索阶段候选证据用于人工排查。

#### Scenario: 运行 Go 后端 RAG 题库试用 eval

- **WHEN** 维护者准备发布 Go 后端题库试用包
- **THEN** 验证 MUST 运行 RAG eval golden queries
- **AND** 验证 MUST 输出 baseline 与 query rewrite 的对比结果
- **AND** HyDE shadow 结果 MUST 可用于人工判断是否升级到 enabled

#### Scenario: RAG eval 输出阶段候选证据

- **WHEN** RAG eval 使用支持 pipeline search 的 retriever
- **THEN** 每个 case 的评估结果 SHOULD include per-stage candidate IDs
- **AND** stage evidence SHOULD include vector, text, tag/rule, and fusion candidates when those stages execute
- **AND** the evidence MUST be additive and MUST NOT change recall, MRR, nDCG, or gate calculation semantics

### Requirement: PGVector 检索应提供运行诊断 trace

系统 MUST make PGVector retrieval diagnosable by exposing candidate evidence for its internal recall paths without changing the existing `Retriever.Retrieve` contract.

#### Scenario: PGVector search returns internal stage trace

- **WHEN** PGVector retrieval executes vector, tag, and text candidate paths
- **THEN** the diagnostic search result MUST record which candidates came from each path
- **AND** the final fusion candidates MUST be recorded in the trace
- **AND** existing `Retrieve` callers MUST continue receiving only final topK results

#### Scenario: PGVector trace does not leak into session state

- **WHEN** PGVector diagnostic fields are produced
- **THEN** vector/tag/text hit flags and text scores MUST remain internal trace evidence
- **AND** they MUST NOT require HTTP API, Session JSON, or database schema changes

### Requirement: 题库列表查询应支持查询计划诊断

系统 MUST provide a read-only way to inspect the PostgreSQL execution plan for the question bank list query used by the backend.

#### Scenario: Maintainer runs question bank EXPLAIN

- **WHEN** a maintainer runs the question bank explain command with application config and filters
- **THEN** the command MUST build the same WHERE, ORDER BY, LIMIT, and OFFSET query shape as the runtime list path
- **AND** it MUST output `EXPLAIN (ANALYZE, BUFFERS)` lines
- **AND** it MUST NOT modify question bank data

#### Scenario: Invalid list cursor fails before query execution

- **WHEN** the explain command receives an invalid cursor
- **THEN** the command MUST fail before executing the database query

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
- **AND** RAG 候选最高置信度达到最低上下文阈值
- **THEN** 决策层 MUST 允许注入有限数量的题库上下文
- **AND** 下一问策略 MUST 结合动态难度和技能覆盖围绕缺失点深挖

#### Scenario: 已用题必须被排除

- **WHEN** RAG 候选或 CandidatePool 包含本场面试已经问过的主问题或追问题
- **THEN** 系统 MUST 在选择下一题或构造追问上下文前排除这些题
- **AND** 如果排除后候选池为空，系统 MUST 记录 degraded reason 并走 fallback 或结束策略

#### Scenario: 决策层不得破坏旧 Session 兼容

- **WHEN** 系统加载没有新决策字段的旧 Session
- **THEN** 面试流程 MUST 继续运行
- **AND** 新增诊断字段 MUST 使用兼容序列化策略或仅存在于运行时/trace 中

### Requirement: RAG 事实流必须预留上线观测和成本保护边界

系统 MUST 为后续上线运维预留后端事实边界，使 quota/cost guard、Admin operations、observability 和脱敏数据保留可以接入同一事实流，而不是旁路实现。

#### Scenario: 决策和失败必须可观测

- **WHEN** RAG 检索为空、弱召回、fallback、embedding 失败、query rewrite 失败或决策层切换策略
- **THEN** 系统 MUST 在 trace、degraded reasons、日志或 eval 输出中记录可诊断事实
- **AND** 记录 MUST 不泄露 token、api key、password 或 secret

#### Scenario: 成本保护入口不绕过业务事实

- **WHEN** 后续引入真实 LLM、embedding 或 RAG eval 导出额度限制
- **THEN** quota/cost guard MUST 在后端入口执行
- **AND** 被拒绝或降级的请求 MUST 记录业务可诊断原因

#### Scenario: Admin operations 只能调用后端事实接口

- **WHEN** 后续引入 Admin UI 或运营面板
- **THEN** UI MUST 只调用后端 import、review、publish、reindex、audit 或 eval 接口
- **AND** UI MUST NOT 直接写入题库、embedding 或检索索引

