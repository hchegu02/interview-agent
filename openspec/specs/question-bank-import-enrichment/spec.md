# question-bank-import-enrichment Specification

## Purpose

定义题库导入、缺失元数据补全、Agent 审核建议和源文档追溯的正式行为边界，确保生成题目先进入暂存审核流程，再按发布策略进入正式题库。

## Requirements

### Requirement: 题库导入支持缺失元数据补全

系统 MUST 在本地题库导入时支持对缺失元数据的题目进行补全，并保留审核后再提交的流程。系统 SHOULD expose Agent-generated quality review decisions so high-confidence generated items can be batch-confirmed while risky items remain blocked from formal question-bank commit.

#### Scenario: 使用 LLM 补全只有题干的导入项

- **WHEN** 用户导入本地题库文件，且题目只有 `id` 和 `content`
- **AND** 导入服务配置了 LLM 模型
- **THEN** 系统应请求 LLM 补全缺失的 `skill_category`、`difficulty`、`tags`、`expected_points`、`rubric`、`sample_answer` 和 `follow_up_hints`
- **AND** 系统必须保留原始 `id` 和 `content`
- **AND** 系统必须先把补全后的题目暂存为待审核导入项，而不是直接写入正式题库

#### Scenario: 未配置 LLM 时保持旧默认行为

- **WHEN** 用户导入缺失元数据的本地题库文件
- **AND** 导入服务没有配置 LLM 模型
- **THEN** 系统应继续使用默认元数据
- **AND** 系统不应凭空生成 `expected_points`、`rubric`、`sample_answer` 或 `follow_up_hints`

#### Scenario: LLM 漏返回输入题目时导入失败

- **WHEN** LLM 补全返回结果缺少某个输入题目
- **THEN** 系统必须拒绝本次导入
- **AND** 导入任务状态应标记为失败

#### Scenario: 批量补全题目

- **WHEN** 导入文件包含多道需要补全的题目
- **THEN** 系统可以按批次请求 LLM 补全
- **AND** 每个批次都必须校验返回项覆盖输入项

#### Scenario: 暂存项保留字段来源

- **WHEN** 系统暂存导入项
- **THEN** 系统应记录关键字段来源
- **AND** 上传字段、默认字段、LLM 补全字段和生成字段应可区分
- **AND** 暂存项应保留原始上传内容，供审核和 diff 预览使用

#### Scenario: Agent 质量审核建议不绕过发布策略

- **WHEN** Agent 对导入或生成的题目给出 `auto_approved`、`needs_human_review` 或 `rejected` 建议
- **THEN** 系统 MUST 保存该建议和理由
- **AND** `rejected` 题目 MUST NOT 被提交到正式题库
- **AND** `auto_approved` 题目 MUST 仍受当前导入任务的发布策略控制

### Requirement: 源文档导入应保留可追溯原文

系统 MUST 支持把原文材料作为题库构建来源，并在生成题目进入暂存区前保留来源快照和来源引用。

#### Scenario: 从源文档生成题目草稿

- **WHEN** 用户导入原文材料用于构建 Go 后端题库
- **THEN** 系统 MUST 保存原文快照、来源类型和内容 hash
- **AND** 系统 MUST 生成暂存题目而不是直接写入正式题库
- **AND** 每道生成题 MUST 能追溯到源文档或源片段引用

#### Scenario: Skill 或 MCP 作为来源适配器

- **WHEN** 系统通过 skill 或 MCP 获取外部链接、文档或知识库原文
- **THEN** 该能力 MUST 只作为来源适配器进入源文档导入流程
- **AND** 适配器 MUST NOT 直接写入正式 question bank
