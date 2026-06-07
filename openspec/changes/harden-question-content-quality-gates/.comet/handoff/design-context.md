# Comet Design Handoff

- Change: harden-question-content-quality-gates
- Phase: design
- Mode: compact
- Context hash: 02e7fca50f56c615e01c7d27583db3eea02176a0ded4663c67a4426c189145f1

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/harden-question-content-quality-gates/proposal.md

- Source: openspec/changes/harden-question-content-quality-gates/proposal.md
- Lines: 1-28
- SHA256: 5bdbc03b9d9a4106a68cab2332b6d507de3f33a9e7fbd6545d8899a149e3be59

```md
# Change: harden-question-content-quality-gates

## Problem

The question-bank pipeline can currently accept structurally complete but semantically dirty question content. A real example is an imported Top100 item whose `content` contains a raw interview-note chain, inline comments, and self-evaluation text such as `--（无法反驳..`. The item has valid metadata and embeddings, so it can become `active`, be retrieved by RAG, and be selected by `pick_next`.

This is a data-quality problem, not a frontend display problem. If dirty content enters the formal question bank, embedding and RAG make it easier to surface during interviews.

## Goals

- Add a shared backend content-quality gate for interview question text.
- Reuse the gate in generation, import commit, question-bank lint, and runtime question selection fallback.
- Prevent high-risk dirty content from silently entering or being selected from the formal question bank.
- Keep historical data governance explicit: detect and report dirty existing questions, but do not delete or batch-update database rows in this change.

## Non-Goals

- No frontend changes.
- No automatic deletion of existing question-bank rows.
- No database schema change unless implementation proves it is strictly necessary.
- No LLM-only quality gate. Rules must be deterministic and testable.

## Scope

- `internal/questionbank`: shared content quality checks and import/generation commit integration.
- `internal/nodes`: `pick_next` runtime guard against high-risk dirty candidate content.
- `cmd/questionbank-lint`: report content quality issues for seed or loaded items.
- Backend documentation and focused tests.
```

## openspec/changes/harden-question-content-quality-gates/design.md

- Source: openspec/changes/harden-question-content-quality-gates/design.md
- Lines: 1-61
- SHA256: 92f744e23cf84b20bf63a3939288cd4e9d80b04d4ee25b0a9498773393192538

```md
# Design: harden-question-content-quality-gates

## Approach

Create one deterministic content-quality classifier in `internal/questionbank` and make existing pipeline stages consume it instead of duplicating ad hoc string checks.

The classifier should return stable flags rather than a boolean. Example flags:

- `dirty_note_marker`
- `multiple_question_chain`
- `content_too_long`
- `answer_or_comment_leak`
- `low_value_question`

Each flag is classified as either high-risk or advisory. High-risk flags block commit and make generated candidates rejected or human-review-only. Advisory flags are reported by lint but do not block by themselves.

## Data Flow

```text
LLM QuestionCandidate
  -> content quality classifier
  -> generation quality flags
  -> staging item review status / reason
  -> commit re-check
  -> formal question_bank only when accepted and clean enough

existing active question_bank rows
  -> questionbank-lint
  -> operator-visible report

RAG candidate_pool
  -> pick_next quality filter
  -> clean candidates preferred
  -> dirty fallback only when no clean candidate remains
```

## Runtime Guard

`pick_next` should not become a cleaner. It should only protect the candidate experience:

- Remove high-risk dirty candidates before LLM/rule picking when at least one clean candidate remains.
- If all remaining candidates are dirty, keep them to avoid dead-ending the session and record a degraded reason.
- The retrieval trace remains unchanged; this guard is about display selection, not retrieval scoring.

## Import and Generation Guard

Generation quality gates should include content-quality flags. Commit must re-check accepted items before `Upsert`, because commit is the last backend boundary before formal `question_bank`.

The commit re-check should mark blocked items as rejected with a diagnostic reason. It must not silently skip without updating staging state.

## Existing Data Governance

This change only reports existing dirty active rows through lint or direct diagnostics. Batch inactivation is intentionally excluded because it mutates user data and needs a separate explicit approval.

## Testing

- Unit tests for content-quality classification.
- Generation gate tests for dirty content rejection.
- Import commit tests for dirty accepted items being blocked.
- `pick_next` tests showing dirty candidate skip and all-dirty fallback.
- `questionbank-lint` tests reporting dirty question content.
```

## openspec/changes/harden-question-content-quality-gates/tasks.md

- Source: openspec/changes/harden-question-content-quality-gates/tasks.md
- Lines: 1-10
- SHA256: 0dfa461d88cf064c0d0adada419277649761dbec03e993963379bced56cc25cd

```md
# Tasks

- [ ] Add shared deterministic question content quality classifier.
- [ ] Extend generation quality gates to flag dirty question text.
- [ ] Add import commit re-check so accepted dirty items do not enter formal question bank.
- [ ] Add `pick_next` runtime guard to prefer clean candidates and degrade only when all candidates are dirty.
- [ ] Extend `cmd/questionbank-lint` to report dirty content issues.
- [ ] Add focused tests for classifier, generation, commit, pick_next, and lint.
- [ ] Update backend SDD and code-change documentation.
- [ ] Run targeted Go tests and OpenSpec validation.
```

## openspec/changes/harden-question-content-quality-gates/specs/question-bank-import-enrichment/spec.md

- Source: openspec/changes/harden-question-content-quality-gates/specs/question-bank-import-enrichment/spec.md
- Lines: 1-38
- SHA256: c79f3d06edd712fdb21f720e5ca5e3bf6eda3660fd1222aebc5687a8db0ad536

```md
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
```

