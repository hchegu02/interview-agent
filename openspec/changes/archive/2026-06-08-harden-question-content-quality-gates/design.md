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
