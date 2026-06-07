## 1. Baseline Review

- [ ] 1.1 Re-read `docs/SDD-Frontend.md` and current frontend entry points to confirm API and Session boundaries before editing.
- [ ] 1.2 Inspect current rendered structure for candidate pages and identify which elements are primary workflow, secondary controls, and diagnostics.
- [ ] 1.3 Check git status and isolate any pre-existing `internal/httpapi/web/dist` changes before implementation.

## 2. Candidate Navigation and Shell

- [ ] 2.1 Reduce candidate sidebar density while preserving workspace switch, route navigation, connection state, user identity input, and session loading.
- [ ] 2.2 Move or visually associate interview mode selection with the preparation/start workflow instead of treating it as an unrelated global setting.
- [ ] 2.3 Keep admin question bank navigation and candidate navigation behavior compatible with existing `routes.ts`.

## 3. Interview Page Layout

- [ ] 3.1 Rework `InterviewPage` structure so current question, answer input, conversation history, and feedback are the default primary reading path.
- [ ] 3.2 Move `AgentStatePanel` and `EventTimeline` into auxiliary or collapsible sections without deleting their data display.
- [ ] 3.3 Preserve pending-answer and backend-processing states during answer submission.

## 4. Report Page Layout

- [ ] 4.1 Rework `ReportPage` top section so overall score, skill breakdown, weak points, and next training actions appear before diagnostics.
- [ ] 4.2 Keep round reviews and drill-plan question jumps functional after layout changes.
- [ ] 4.3 Move WorkingMemory and retrieval trace display into secondary or collapsible sections while retaining all existing fields.

## 5. Responsive and Visual Refinement

- [ ] 5.1 Update `styles.css` so narrow screens prioritize page content before full desktop-style sidebar content.
- [ ] 5.2 Verify interview answer input does not overlap conversation content on narrow screens.
- [ ] 5.3 Adjust spacing, typography scale, card density, and contrast for a restrained workbench style without introducing new dependencies.

## 6. Tests and Documentation

- [ ] 6.1 Add or update frontend tests for changed render structure and auxiliary diagnostics visibility.
- [ ] 6.2 Run `npm --prefix web run test`.
- [ ] 6.3 Run `npm --prefix web run build`.
- [ ] 6.4 Create `docs/code-changes/MM-DD-前端工作台布局优化.md` after implementation, based on actual code diff.
- [ ] 6.5 Decide whether to synchronize `internal/httpapi/web/dist` build output separately, accounting for pre-existing dist modifications.
