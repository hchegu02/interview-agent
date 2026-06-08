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
