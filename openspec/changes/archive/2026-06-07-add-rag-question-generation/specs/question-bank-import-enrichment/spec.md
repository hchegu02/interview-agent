## MODIFIED Requirements

### Requirement: 源文档导入应保留可追溯原文

系统 MUST 支持把原文材料作为题库构建来源，并在生成题目进入暂存区前保留来源快照和来源引用。

#### Scenario: 从源文档生成题目草稿

- **WHEN** 用户导入原文材料用于构建 Go 后端题库
- **THEN** 系统 MUST 保存原文快照、来源类型和内容 hash
- **AND** 系统 MUST 生成暂存题目而不是直接写入正式题库
- **AND** 每道生成题 MUST 能追溯到源文档或源片段引用

#### Scenario: 按用户任务检索来源后定向生成题目

- **WHEN** 用户基于已导入源文档请求按主题、题型、数量和难度生成题目
- **THEN** 系统 MUST 先在指定来源范围内检索相关源片段
- **AND** 系统 MUST 从检索片段中抽取可出题的 concept cards
- **AND** 系统 MUST 将 concept cards 和检索到的源片段作为 evidence pack 传入 LLM
- **AND** LLM 输出 MUST 是结构化 QuestionCandidate 草稿
- **AND** 每道题 MUST 包含至少一个可回溯到检索片段的来源引用
- **AND** 生成题 MUST 进入暂存审核流程，而不是直接写入正式 question bank

#### Scenario: 无可用能力点时不生成题目

- **WHEN** 用户请求定向生成题目
- **AND** 系统检索到了文本片段但无法抽取可出题的 concept cards
- **THEN** 系统 MUST NOT 调用 LLM 凭空生成 QuestionCandidate
- **AND** 系统 MUST 返回可解释的空结果或失败原因

#### Scenario: 未命中来源证据时拒绝生成或降级为空结果

- **WHEN** 用户请求定向生成题目
- **AND** 指定来源范围内没有检索到可用源片段
- **THEN** 系统 MUST NOT 调用 LLM 直接凭空生成题目
- **AND** 系统 MUST 返回可解释的空结果或失败原因

#### Scenario: 生成题质量门禁阻止重复和无来源题

- **WHEN** LLM 返回题目草稿
- **AND** 草稿缺少必要字段、缺少来源引用、引用无法在 evidence pack 中验证、没有关联 concept card，或与同批生成题重复
- **THEN** 系统 MUST 阻止该题进入可提交状态
- **AND** 系统 SHOULD 在暂存项或生成结果中记录被阻止原因

#### Scenario: 生成题元数据应版本化

- **WHEN** 系统把 QuestionCandidate 写入暂存审核流程
- **THEN** 系统 MUST 写入版本化生成元数据
- **AND** 元数据 MUST 至少包含 generation job、source job、concept、question type、answer/explanation 和 source refs

#### Scenario: 人工确认源文档生成题后进入可检索题库

- **WHEN** 源文档导入生成了字段完整的有效暂存题
- **AND** 人工审核接受这些题目
- **AND** 本地 embedding 服务可用且维度与 `question_bank.embedding` 一致
- **THEN** commit MUST 将接受的题目写入正式 `question_bank`
- **AND** 系统 MUST 为提交题写入 embedded 状态和 embedding 模型
- **AND** 后续题库查询或 RAG 检索 MUST 能召回这些新题

#### Scenario: Skill 或 MCP 作为来源适配器

- **WHEN** 系统通过 skill 或 MCP 获取外部链接、文档或知识库原文
- **THEN** 该能力 MUST 只作为来源适配器进入源文档导入流程
- **AND** 适配器 MUST NOT 直接写入正式 question bank
