## MODIFIED Requirements

### Requirement: 题库导入支持缺失元数据补全

系统 MUST 在本地题库导入时支持对缺失元数据的题目进行补全，并保留审核后再提交的流程。系统 SHOULD expose Agent-generated quality review decisions so high-confidence generated items can be batch-confirmed while risky items remain blocked from formal question-bank commit.

#### Scenario: Agent 质量审核建议不绕过发布策略

- **WHEN** Agent 对导入或生成的题目给出 `auto_approved`、`needs_human_review` 或 `rejected` 建议
- **THEN** 系统 MUST 保存该建议和理由
- **AND** `rejected` 题目 MUST NOT 被提交到正式题库
- **AND** `auto_approved` 题目 MUST 仍受当前导入任务的发布策略控制
- **AND** `needs_human_review` 题目 MUST NOT 被提交到正式题库，除非人工审核接受该题

#### Scenario: 人工接受源文档生成题后允许提交

- **WHEN** 源文档导入生成的有效题目处于 `needs_human_review`
- **AND** 人工审核接受该题
- **THEN** 系统 MUST 将该题的 Agent 审核状态推进为 `auto_approved`
- **AND** 后续 commit MUST 将该题写入正式题库

#### Scenario: 人工拒绝源文档生成题后阻止提交

- **WHEN** 源文档导入生成的有效题目处于 `needs_human_review`
- **AND** 人工审核拒绝该题
- **THEN** 系统 MUST 将该题的 Agent 审核状态推进为 `rejected`
- **AND** 后续 commit MUST NOT 将该题写入正式题库
