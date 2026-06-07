# Fix Document Import Approval

## Problem

Markdown / document question-bank import can generate valid staging items, but accepted items are still skipped during commit.

Root cause: document imports set `ImportItem.AgentReviewStatus` to `needs_human_review`. The review API only updates `review_status`; it does not advance the agent review state. `ImportService.Commit` intentionally skips any item whose agent review status is neither empty nor `auto_approved`, so human-accepted document items never reach the formal `question_bank`.

## Goal

- Preserve the existing safety rule that generated document questions require human approval.
- When a human accepts valid generated items, advance their agent review state to `auto_approved`.
- When a human rejects valid generated items, advance their agent review state to `rejected`.
- Keep structured local question-bank imports compatible.

## Non-Goals

- No new HTTP endpoint.
- No database schema change.
- No frontend change.
- No change to embedding model selection.
