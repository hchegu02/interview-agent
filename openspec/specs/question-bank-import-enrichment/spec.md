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
- **AND** 草稿缺少必要字段、缺少来源引用、引用无法在 evidence pack 中验证、没有关联 concept card，与同批生成题重复，或与正式题库中已有 active 题目重复
- **THEN** 系统 MUST 阻止该题进入可提交状态
- **AND** 系统 SHOULD 在暂存项或生成结果中记录被阻止原因

#### Scenario: 提交阶段再次阻止重复生成题

- **WHEN** 源文档生成题已经进入暂存审核流程
- **AND** 暂存题与同一 import job 中其他暂存题重复，或与正式题库中已有 active 题目重复
- **THEN** commit MUST NOT 将重复题写入正式 `question_bank`
- **AND** 系统 SHOULD 保留重复题未提交的可诊断原因

#### Scenario: 生成题元数据应版本化

- **WHEN** 系统把 QuestionCandidate 写入暂存审核流程
- **THEN** 系统 MUST 写入版本化生成元数据
- **AND** 元数据 MUST 至少包含 generation job、source job、concept、question type、answer/explanation 和 source refs

#### Scenario: 人工确认源文档生成题后进入可检索题库

- **WHEN** 源文档导入生成了字段完整的有效暂存题
- **AND** 人工审核接受这些题目
- **AND** 这些题目不与正式题库中已有 active 题目重复
- **AND** 本地 embedding 服务可用且维度与 `question_bank.embedding` 一致
- **THEN** commit MUST 将接受的题目写入正式 `question_bank`
- **AND** 系统 MUST 为提交题写入 embedded 状态和 embedding 模型
- **AND** 后续题库查询或 RAG 检索 MUST 能召回这些新题

#### Scenario: Skill 或 MCP 作为来源适配器

- **WHEN** 系统通过 skill 或 MCP 获取外部链接、文档或知识库原文
- **THEN** 该能力 MUST 只作为来源适配器进入源文档导入流程
- **AND** 适配器 MUST NOT 直接写入正式 question bank

### Requirement: 定向题库生成支持异步任务

系统 MUST support asynchronous targeted question generation so long-running real LLM calls do not require the HTTP request to remain open until generation completes.

#### Scenario: 异步创建生成任务

- **WHEN** caller posts to `/api/question-bank/generation-jobs?async=true` with a valid generation request
- **THEN** system MUST create a generation job with status `queued`
- **AND** system MUST return HTTP 202 with the queued job
- **AND** system MUST execute generation in a backend worker

#### Scenario: 轮询异步生成任务状态

- **WHEN** caller gets `/api/question-bank/generation-jobs/:id`
- **THEN** system MUST return the latest persisted job status
- **AND** status MUST eventually become `created` or `failed` after worker completion

#### Scenario: 未完成任务不能暂存

- **WHEN** caller posts `/api/question-bank/generation-jobs/:id/stage`
- **AND** the generation job is not `created`
- **THEN** system MUST reject staging
- **AND** system MUST NOT create import review items

### Requirement: 题库题干质量门禁

系统 MUST apply deterministic content-quality checks to generated, imported, committed, and runtime-selected question-bank items so dirty source notes are not silently promoted into candidate-facing interview questions.

#### Scenario: 生成题题干包含笔记残留时阻止自动通过

- **WHEN** LLM 生成的 QuestionCandidate content contains high-risk note residue, inline self-comments, answer/comment leakage, or a multi-question raw interview chain
- **THEN** the system MUST attach stable quality flags to the candidate
- **AND** the candidate MUST NOT become auto-approved for formal question-bank commit
- **AND** the reason SHOULD be visible in staging or generation diagnostics

#### Scenario: 提交阶段再次阻止脏题进入正式题库

- **WHEN** an accepted import item is about to be committed to the formal question bank
- **AND** the item content has high-risk content-quality flags
- **THEN** commit MUST NOT write that item to `question_bank`
- **AND** the staging item SHOULD be marked rejected with a diagnostic reason

#### Scenario: 运行时选题优先跳过高风险脏题

- **WHEN** RAG returns a candidate pool that contains both clean and high-risk dirty questions
- **THEN** `pick_next` MUST prefer the clean candidate subset before LLM or rule selection
- **AND** high-risk dirty candidates MUST NOT be selected while a clean candidate remains

#### Scenario: 全部候选题均为脏题时不中断面试

- **WHEN** all remaining RAG candidates have high-risk content-quality flags
- **THEN** `pick_next` MAY continue with the dirty candidates to avoid dead-ending the session
- **AND** the session MUST record a degraded reason explaining that only dirty question candidates were available

#### Scenario: 题库 lint 报告已有脏题

- **WHEN** question-bank lint scans seed or stored question-bank items
- **AND** an item content has high-risk content-quality flags
- **THEN** lint MUST include the item id and flags in its report
- **AND** lint SHOULD fail when high-risk content issues are present

