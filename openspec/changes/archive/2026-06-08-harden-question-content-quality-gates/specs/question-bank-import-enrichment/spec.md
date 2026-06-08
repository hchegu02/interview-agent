## ADDED Requirements

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
