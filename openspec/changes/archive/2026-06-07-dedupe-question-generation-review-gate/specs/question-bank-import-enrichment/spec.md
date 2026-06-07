## MODIFIED Requirements

### Requirement: 源文档导入应保留可追溯原文

系统 MUST 支持把原文材料作为题库构建来源，并在生成题目进入暂存区前保留来源快照和来源引用。

#### Scenario: 生成题质量门禁阻止重复和无来源题

- **WHEN** LLM 返回题目草稿
- **AND** 草稿缺少必要字段、缺少来源引用、引用无法在 evidence pack 中验证、没有关联 concept card，与同批生成题重复，或与正式题库中已有 active 题目重复
- **THEN** 系统 MUST 阻止该题进入可提交状态
- **AND** 系统 SHOULD 在暂存项或生成结果中记录被阻止原因

#### Scenario: 提交阶段再次阻止重复生成题

- **WHEN** 源文档生成题已经进入暂存审核流程
- **AND** 暂存题与同一 import job 中其他暂存题重复，或与正式题库中已有 active 题目重复
- **THEN** commit MUST NOT 将重复题写入正式 `question_bank`
- **AND** 系统 SHOULD 保留重复题未提交的可诊断原因

#### Scenario: 人工确认源文档生成题后进入可检索题库

- **WHEN** 源文档导入生成了字段完整的有效暂存题
- **AND** 人工审核接受这些题目
- **AND** 这些题目不与正式题库中已有 active 题目重复
- **AND** 本地 embedding 服务可用且维度与 `question_bank.embedding` 一致
- **THEN** commit MUST 将接受的题目写入正式 `question_bank`
- **AND** 系统 MUST 为提交题写入 embedded 状态和 embedding 模型
- **AND** 后续题库查询或 RAG 检索 MUST 能召回这些新题
