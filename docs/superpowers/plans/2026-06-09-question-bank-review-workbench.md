# Question Bank Review Workbench Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a production-oriented question import review workbench that lets reviewers inspect, filter, batch review, and publish import jobs from `/questions`.

**Architecture:** Keep the backend API contract unchanged and move review presentation/decision logic into focused frontend helpers and a dedicated React component. `QuestionBankPage` remains the page orchestrator for upload, polling, API calls, and the existing question-bank browse area.

**Tech Stack:** React 19, TypeScript, Vite, Vitest, existing CSS system in `web/src/styles.css`.

---

### File Structure

- Create: `web/src/questionBankReviewWorkbench.tsx`
  - Owns the three-column import review workbench UI: job queue, review queue, selected item detail, publish panel, filters, search, selection, and batch actions.
- Modify: `web/src/questionBankImportView.ts`
  - Add deterministic helper functions for completeness, review metrics, filtering, search matching, and commit summary extraction.
- Modify: `web/src/types.ts`
  - Add optional backend diagnostic fields: `agent_review_status`, `agent_review_reason`, and `metadata.commit_summary`.
- Modify: `web/src/questionBankPage.tsx`
  - Replace inline `ImportDetail` UI with `QuestionBankReviewWorkbench` while preserving upload, polling, refresh, commit, and item review callbacks.
- Modify: `web/src/styles.css`
  - Add dense, production-grade workbench layout and responsive behavior.
- Modify: `web/src/questionBankImportView.test.ts`
  - Cover helper behavior for completeness, filtering, metrics, and commit summary.
- Add: `docs/code-changes/06-09-question-bank-review-workbench.md`
  - Record runtime/UI behavior change, affected files, tests, risk, and data flow.

### Task 1: Types And Import Helper Contract

**Files:**
- Modify: `web/src/types.ts`
- Modify: `web/src/questionBankImportView.ts`
- Modify: `web/src/questionBankImportView.test.ts`

- [ ] **Step 1: Add failing helper tests**

Add tests for:
- field completeness requires non-empty `content`, `expected_points`, `rubric`, `sample_answer`, and `follow_up_hints`;
- agent rejected items are filterable;
- invalid items are filterable;
- metrics count valid, invalid, accepted, rejected, complete, incomplete, agent rejected, selected;
- commit summary is read from `job.metadata.commit_summary`.

- [ ] **Step 2: Run helper tests**

Run: `npm --prefix web run test -- questionBankImportView.test.ts`
Expected: FAIL because helper functions do not exist yet.

- [ ] **Step 3: Implement helper functions**

Add exported helpers:
- `hasImportAnswerCompleteness(item)`
- `importItemReviewFlags(item)`
- `buildImportReviewMetrics(job, items, selectedIds)`
- `filterImportItems(items, filter, query)`
- `commitSummary(job)`

- [ ] **Step 4: Run helper tests**

Run: `npm --prefix web run test -- questionBankImportView.test.ts`
Expected: PASS.

### Task 2: Dedicated Review Workbench Component

**Files:**
- Create: `web/src/questionBankReviewWorkbench.tsx`

- [ ] **Step 1: Create component contract**

Props:
- `jobs`
- `selectedId`
- `items`
- `busy`
- `source`
- `onSourceChange`
- `onSelectJob`
- `onUploadClick`
- `onCommit`
- `onReview`

- [ ] **Step 2: Implement workbench state**

Internal state:
- text search query
- selected filter
- selected item id
- selected item id set for batch review
- local confirm checkbox for dangerous accept-all action

- [ ] **Step 3: Implement layout**

Render:
- top metrics and upload controls;
- left import job rail;
- middle review queue with search, filters, multi-select, batch accept/reject;
- right selected item detail with diff, provenance, validation errors, agent review, and publish panel.

### Task 3: Page Integration

**Files:**
- Modify: `web/src/questionBankPage.tsx`

- [ ] **Step 1: Import workbench component**

Replace imports from `questionBankImportView` so the page only needs `QuestionBankReviewWorkbench`.

- [ ] **Step 2: Replace inline import UI**

Replace `<section className="import-workbench">...</section>` with `QuestionBankReviewWorkbench`.

- [ ] **Step 3: Remove obsolete inline components**

Remove `ImportDetail` and `ImportDiff` from `questionBankPage.tsx`.

### Task 4: Production Workbench Styling

**Files:**
- Modify: `web/src/styles.css`

- [ ] **Step 1: Add dense workbench styles**

Add layout classes for:
- `.review-workbench`
- `.review-command-bar`
- `.review-metrics`
- `.review-shell`
- `.review-job-rail`
- `.review-queue`
- `.review-inspector`
- `.review-filter-bar`
- `.review-item-row`
- `.review-publish-panel`

- [ ] **Step 2: Add responsive behavior**

At desktop, use three columns. Below 1280px, collapse to two rows. Below 980px, use a single column and prevent button/text overflow.

### Task 5: Documentation And Verification

**Files:**
- Add: `docs/code-changes/06-09-question-bank-review-workbench.md`

- [ ] **Step 1: Document behavior change**

Include:
- changed runtime behavior;
- file-level and function-level changes;
- user/API data flow;
- tests;
- risks.

- [ ] **Step 2: Run frontend tests**

Run: `npm --prefix web run test`
Expected: PASS.

- [ ] **Step 3: Run frontend build**

Run: `npm --prefix web run build`
Expected: PASS.

- [ ] **Step 4: Browser verification**

Start dev server if needed and open `/questions`.
Expected:
- page loads;
- review workbench is visible;
- no obvious overlap at desktop viewport;
- upload/review controls are disabled/enabled according to job state.
