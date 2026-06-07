## MODIFIED Requirements

### Requirement: 源文档导入应保留可追溯原文

系统 MUST 支持把原文材料作为题库构建来源，并在生成题目进入暂存区前保留来源快照和来源引用。

#### Scenario: 从源文档生成题目草稿

- **WHEN** 用户导入原文材料用于构建 Go 后端题库
- **THEN** 系统 MUST 保存原文快照、来源类型和内容 hash
- **AND** 系统 MUST 生成暂存题目而不是直接写入正式题库
- **AND** 每道生成题 MUST 能追溯到源文档或源片段引用

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
