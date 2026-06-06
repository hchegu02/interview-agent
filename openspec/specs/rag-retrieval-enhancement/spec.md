# rag-retrieval-enhancement Specification

## Purpose

定义 RAG 检索增强能力，包括 Query Rewriting、HyDE 实验模式和 Go 后端题库试用的检索评估门禁，确保增强策略可观测、可回退，并不破坏正式候选选择流程。

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

系统 MUST 为 Go 后端单岗位内部试用提供 RAG eval 对比，至少覆盖 baseline、query rewrite 和 HyDE shadow 诊断路径。

#### Scenario: 运行 Go 后端 RAG 题库试用 eval

- **WHEN** 维护者准备发布 Go 后端题库试用包
- **THEN** 验证 MUST 运行 RAG eval golden queries
- **AND** 验证 MUST 输出 baseline 与 query rewrite 的对比结果
- **AND** HyDE shadow 结果 MUST 可用于人工判断是否升级到 enabled
