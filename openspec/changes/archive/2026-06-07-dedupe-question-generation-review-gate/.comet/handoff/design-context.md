# Comet Design Handoff

- Change: dedupe-question-generation-review-gate
- Phase: design
- Mode: compact
- Context hash: 9e28e45f7b328e0ebe02a1994e6a8821d34e4b8738ca9c44aaaf64c6b44b9e48

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/dedupe-question-generation-review-gate/proposal.md

- Source: openspec/changes/dedupe-question-generation-review-gate/proposal.md
- Lines: 1-43
- SHA256: 2ef2c77aafd44057db3c294929e7d8017ecdb864a75b73ef71cedd30a1dc7f40

```md
# Change: Dedupe Question Generation Review Gate

## Problem

真实业务演练后，本地 `question_bank` 中出现多条 `docq-*` 生成题重复内容，例如多条题干都是“Go 服务如何设计超时、重试和熔断，避免级联故障？”。现有生成质量门禁能拦截同一批 LLM 返回中的重复题，但没有把已存在正式题库、已暂存导入项、以及同一生成 job 的 staged 结果作为全局去重边界。

这会带来三个现实问题：

- 内部试用时题库列表出现大量重复题，业务上不可用。
- RAG 检索结果被重复题污染，影响题目多样性和面试体验。
- Agent review 虽然已有 `auto_approved`、`needs_human_review`、`rejected` 状态，但重复题的阻断原因和提交结果不够可诊断。

## Goals

- 在生成题进入暂存审核流程前，阻止与正式题库或同一暂存 job 中已有题目重复的候选题。
- 在 commit 前再次保护，避免重复题绕过生成阶段进入正式 `question_bank`。
- 对被拦截的重复题保留可解释原因，便于内部团队判断是模型生成问题、来源材料问题还是审核策略问题。
- 保持现有人工审核流程：`needs_human_review` 不经人工接受不得提交，`rejected` 不得提交。

## Non-Goals

- 不做数据库 schema 变更。
- 不改前端页面和交互。
- 不引入新的向量相似度去重服务。
- 不改变现有题库 API 响应结构的必填字段；如需新增诊断字段，必须 `omitempty`。
- 不让 skill/MCP 直接写正式题库。

## Scope

涉及后端模块：

- `internal/questionbank/generation_*`
- `internal/questionbank/imports_*`
- `internal/questionbank/store.go` / `pg_store.go` 现有读写能力
- 相关测试和运行文档

## Success Criteria

- 生成阶段能拦截与正式题库既有题干重复的候选题。
- staged import 内重复题不会同时进入可提交状态。
- commit 阶段不会把重复题写入正式题库。
- 重复题拦截原因能在生成 job 或 import item 上看到。
- `go test ./internal/questionbank -count=1` 通过。
```

## openspec/changes/dedupe-question-generation-review-gate/design.md

- Source: openspec/changes/dedupe-question-generation-review-gate/design.md
- Lines: 1-63
- SHA256: 49dcf0fc517a3c15d1c8b6030d002fa2bee349c24fa82af87b67f6f850c34ade

```md
# Design

## Current Behavior

`gateQuestionCandidates` 当前只使用 `seenContent` 检测同批 LLM 返回中的重复题。`GenerationService.Stage` 会把通过门禁的 candidates 转成 import items，再进入现有人工审核流程。`ImportService.Commit` 只提交 `ImportItemStatusValid` 且 review 策略允许的 item。

现有 `Commit` 已阻止 `needs_human_review` 和 `rejected` 静默进入正式题库，但它不知道“正式题库已有同题干”或“同一 import job 内多个 item 归一化后重复”。

## Approach

采用保守的文本归一化去重，不引入新 schema：

1. **统一归一化函数**
   - 复用或提升 `normalizeCandidateContent` 的语义。
   - 归一化规则只做大小写、空白和常见标点压缩，避免误杀语义相近但不同的问题。

2. **生成阶段去重**
   - 在 `GenerationService.Generate` 或 Stage 前加载正式题库中 active 题目的归一化 content key。
   - `gateQuestionCandidates` 支持传入已有 content keys。
   - 与已有题库重复的 candidate 进入 `RejectedCandidates`，标记 `duplicate_existing_content` 或复用扩展后的 duplicate flag。

3. **暂存/提交阶段保护**
   - 在 import staging 或 commit 前，对同一 job 内 item 做 content key 去重。
   - 重复 item 不写正式题库，并保留 review/agent review reason。
   - commit 前再次读取正式题库 active items，过滤重复内容，避免并发或旧数据绕过生成门禁。

4. **可诊断反馈**
   - 对重复题写入 `AgentReviewStatus=rejected` 或保留 `QualityFlags`，理由包含重复类型。
   - 不要求前端立即展示新字段，但 API 现有 import item / generation job 返回应能看到原因字段。

## Data Flow

```text
source chunks
  -> concept cards
  -> LLM candidates
  -> generation quality gates
       - required fields
       - source refs
       - same batch duplicate
       - existing question bank duplicate
  -> accepted candidates
  -> staged import items
       - same job duplicate guard
       - agent review status/reason
  -> human review
  -> commit
       - review policy
       - existing question bank duplicate guard
  -> question_bank + embedding
```

## Compatibility

- 不改变现有 `question_bank` schema。
- 旧导入项仍按现有 review 规则提交。
- 没有配置 PG store 或无法读取正式题库时，生成阶段至少保留同批去重；commit 阶段仍按已有 writer 行为执行，但测试应覆盖 memory store 路径。

## Risks

- 文本归一化过强会误杀相似但不同的题。第一版只做保守 exact-normalized dedupe。
- commit 前过滤重复题可能导致 import job `ImportedItems` 小于 accepted item 数量，需要测试确认状态和计数语义清楚。
- 如果正式题库已有重复历史数据，本 change 不负责清理历史数据，只防止新增。
```

## openspec/changes/dedupe-question-generation-review-gate/tasks.md

- Source: openspec/changes/dedupe-question-generation-review-gate/tasks.md
- Lines: 1-9
- SHA256: 6c0cee70321c61a50eea1f4902651e2f87791474beeb6179094af5ec70002867

```md
# Tasks

- [ ] Add normalized content-key helpers for question-bank dedupe.
- [ ] Extend generation quality gates to reject candidates duplicated against existing question-bank content.
- [ ] Guard staged import / commit so duplicate content within a job or against existing active questions is not written.
- [ ] Preserve duplicate rejection reasons in generation job or import item review metadata.
- [ ] Add focused tests for generation duplicate rejection, import commit duplicate blocking, and existing review policy compatibility.
- [ ] Run `go test ./internal/questionbank -count=1`.
- [ ] Update change documentation if implementation changes behavior beyond this design.
```

## openspec/changes/dedupe-question-generation-review-gate/specs/question-bank-import-enrichment/spec.md

- Source: openspec/changes/dedupe-question-generation-review-gate/specs/question-bank-import-enrichment/spec.md
- Lines: 1-29
- SHA256: 146aaca4ce18141389a3d4482c56e897d6e43a946733e42e2a2e5d0779b31e3b

```md
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
```

