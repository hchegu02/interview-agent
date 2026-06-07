---
change: improve-frontend-workbench-layout
design-doc: docs/superpowers/specs/2026-06-07-frontend-workbench-layout-design.md
base-ref: 273ec397532be39785d37375cdf5c8226c52c4ba
archived-with: 2026-06-07-improve-frontend-workbench-layout
---

# Frontend Workbench Layout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Improve the candidate workbench layout so preparation, active answering, report conclusions, and training actions are primary while backend diagnostics remain available as secondary information.

**Architecture:** Keep the existing React/Vite single-page workbench, routes, API client, Session types, and backend contracts. Make scoped JSX structure changes in `main.tsx` and `candidatePages.tsx`, add small shared view helpers if needed, and refine `styles.css` for desktop and narrow-screen layout.

**Tech Stack:** React 19, TypeScript, Vite, Vitest, SSR-style `renderToStaticMarkup` tests, plain CSS.

archived-with: 2026-06-07-improve-frontend-workbench-layout
---

## File Map

- Modify `web/src/main.tsx`: reduce sidebar density, preserve navigation and session loading, pass interview mode/update controls to the preparation page if needed.
- Modify `web/src/candidatePages.tsx`: adjust JD, Interview, and Report page structures; keep existing props and backend data usage compatible unless a local prop extension is required.
- Modify `web/src/sharedView.tsx`: add a minimal reusable secondary/diagnostic section component only if it reduces duplication.
- Modify `web/src/styles.css`: update workbench layout, auxiliary sections, report hierarchy, sidebar density, and narrow-screen behavior.
- Modify `web/src/candidatePages.test.tsx`: add tests for primary/auxiliary page structure and preserve existing diagnostics rendering.
- Create `docs/code-changes/06-07-前端工作台布局优化.md`: document the actual implementation diff after code changes.
- Do not modify Go backend code, `web/src/apiClient.ts`, `web/src/types.ts`, or `internal/httpapi/web/dist` unless explicitly required and separately confirmed.

## Task 1: Add Frontend Layout Test Guardrails

**Files:**
- Modify: `web/src/candidatePages.test.tsx`

- [ ] **Step 1: Add a JD page test for mode controls near start workflow**

Add a test in the existing `describe("candidate jd page", ...)` block that expects the JD page to render a mode/workflow control near preparation actions after implementation.

Suggested assertion shape:

```tsx
it("shows interview mode controls in the preparation workflow", () => {
  const html = renderToStaticMarkup(
    <JDPage
      draft={draft}
      mode="practice"
      busy={false}
      updateDraft={() => undefined}
      analyze={() => Promise.resolve()}
      startInterview={() => Promise.resolve()}
    />,
  );

  expect(html).toContain("面试模式");
  expect(html).toContain("模拟");
  expect(html).toContain("考试");
  expect(html).toContain("开始面试");
});
```

If implementing mode switching requires a new prop, update the test call with `setMode={() => undefined}` in the same step.

- [ ] **Step 2: Add an interview page test for auxiliary diagnostics grouping**

Extend `describe("candidate interview page", ...)` with a practice session that has `working_memory` and events. Assert the main question remains present and diagnostics are grouped under a secondary label.

Suggested assertion shape:

```tsx
it("keeps practice diagnostics available as auxiliary interview information", () => {
  const session: Session = {
    session_id: "s-interview-aux",
    mode: "practice",
    status: "running",
    phase: "answering",
    question: { id: "go-001", content: "讲一下 Go GMP。" },
    working_memory: { difficulty: { current: 1 }, rounds_asked: 1, max_rounds: 8 },
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };

  const html = renderToStaticMarkup(
    <InterviewPage
      session={session}
      events={[{ type: "graph.node", label: "Graph 节点", detail: "score_answer", at: "12:00:00" }]}
      busy={false}
      pendingAnswer=""
      submitAnswer={() => Promise.resolve()}
      goJD={() => undefined}
    />,
  );

  expect(html).toContain("讲一下 Go GMP。");
  expect(html).toContain("辅助诊断");
  expect(html).toContain("Agent 状态");
  expect(html).toContain("Graph 节点");
});
```

- [ ] **Step 3: Add a report page test for conclusions before diagnostics**

Add a report test with `drill_plan`, `improvements`, `working_memory`, and `retrieval_trace`. Assert conclusion/training text appears before retrieval trace text in the rendered string.

Suggested assertion shape:

```tsx
expect(html).toContain("训练计划");
expect(html).toContain("检索链路");
expect(html.indexOf("训练计划")).toBeLessThan(html.indexOf("检索链路"));
expect(html).toContain("辅助诊断");
```

- [ ] **Step 4: Run the focused frontend tests and verify they fail for the intended missing UI**

Run:

```powershell
npm --prefix web run test -- candidatePages.test.tsx
```

Expected before implementation: at least the newly added tests fail because labels such as `面试模式` or `辅助诊断` are not implemented yet. Existing unrelated tests should not start failing from syntax/type errors.

## Task 2: Reduce Sidebar Burden and Move Mode Context to JD Page

**Files:**
- Modify: `web/src/main.tsx`
- Modify: `web/src/candidatePages.tsx`
- Modify: `web/src/styles.css`
- Modify: `web/src/candidatePages.test.tsx` if `JDPage` props change

- [ ] **Step 1: Extend JDPage props to accept mode setter**

Change `JDPage` props from:

```tsx
export function JDPage({ draft, mode, busy, updateDraft, analyze, startInterview }: {
  draft: Draft;
  mode: Mode;
  busy: boolean;
  updateDraft: (patch: Partial<Draft>) => void;
  analyze: () => Promise<void>;
  startInterview: () => Promise<void>;
}) {
```

to:

```tsx
export function JDPage({ draft, mode, busy, updateDraft, setMode, analyze, startInterview }: {
  draft: Draft;
  mode: Mode;
  busy: boolean;
  updateDraft: (patch: Partial<Draft>) => void;
  setMode: (mode: Mode) => void;
  analyze: () => Promise<void>;
  startInterview: () => Promise<void>;
}) {
```

- [ ] **Step 2: Pass setMode from App to JDPage**

In `main.tsx`, update the JD route render:

```tsx
<JDPage
  draft={draft}
  mode={mode}
  busy={busy}
  updateDraft={updateDraft}
  setMode={setMode}
  analyze={analyze}
  startInterview={startInterview}
/>
```

- [ ] **Step 3: Move the visible mode switch from sidebar into JD control panel**

In `main.tsx`, remove or visually demote the global candidate/exam `.mode-switch` that calls `setMode`. Keep the workspace switch.

In `JDPage`, add a mode block near `准备检查` before the practice-only `scope-panel`:

```tsx
<div className="mode-panel" aria-label="面试模式">
  <div>
    <h3>面试模式</h3>
    <p>{mode === "practice" ? "模拟模式会展示训练策略和诊断信息。" : "考试模式隐藏内部诊断信息。"}</p>
  </div>
  <div className="segmented mode-segmented">
    <button type="button" className={mode === "practice" ? "active" : ""} onClick={() => setMode("practice")}>模拟</button>
    <button type="button" className={mode === "exam" ? "active" : ""} onClick={() => setMode("exam")}>考试</button>
  </div>
</div>
```

- [ ] **Step 4: Lower sidebar session density without removing functionality**

In `main.tsx`, keep `refreshSessions`, `loadSession`, and `deleteSession`, but wrap the history section in a lower-density container. If using native disclosure, use:

```tsx
<details className="side-section session-section" open={!sidebarCollapsed}>
  <summary className="section-title">
    <span>历史会话</span>
    <button type="button" onClick={(evt) => { evt.preventDefault(); refreshSessions(); }} className="ghost-icon">↻</button>
  </summary>
  <div className="session-list">...</div>
</details>
```

If the nested refresh button causes invalid interaction in tests or browser behavior, keep the current `section` but style it as secondary with `.session-section`.

- [ ] **Step 5: Update CSS for mode panel and sidebar density**

Add styles:

```css
.mode-panel {
  display: grid;
  gap: 10px;
  padding: 12px;
  background: #f8fafc;
  border: 1px solid var(--line);
  border-radius: var(--radius);
}

.mode-panel p {
  margin: 4px 0 0;
  color: var(--muted);
  font-size: 12px;
  line-height: 1.45;
}

.mode-segmented {
  width: 100%;
}

.session-section {
  opacity: 0.92;
}
```

Adjust existing `.sidebar-collapsed` rules so removed/demoted mode switch selectors do not hide the wrong controls.

- [ ] **Step 6: Run focused tests**

Run:

```powershell
npm --prefix web run test -- candidatePages.test.tsx routes.test.ts
```

Expected: JD page tests and route tests pass.

## Task 3: Make Interview Diagnostics Secondary

**Files:**
- Modify: `web/src/candidatePages.tsx`
- Modify: `web/src/sharedView.tsx` if adding a reusable helper
- Modify: `web/src/styles.css`
- Modify: `web/src/candidatePages.test.tsx`

- [ ] **Step 1: Add a small auxiliary section wrapper**

If avoiding duplication, add this to `candidatePages.tsx` near helper components or to `sharedView.tsx`:

```tsx
function AuxiliarySection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="auxiliary-section">
      <details open>
        <summary>{title}</summary>
        <div className="auxiliary-content">{children}</div>
      </details>
    </section>
  );
}
```

Keep it local to `candidatePages.tsx` if it is only used there.

- [ ] **Step 2: Wrap practice diagnostics in InterviewPage**

Replace:

```tsx
{session.mode === "practice" && (
  <>
    <AgentStatePanel memory={session.working_memory} />
    <EventTimeline events={events} />
  </>
)}
```

with:

```tsx
{session.mode === "practice" && (
  <AuxiliarySection title="辅助诊断">
    <AgentStatePanel memory={session.working_memory} />
    <EventTimeline events={events} />
  </AuxiliarySection>
)}
```

Keep exam behavior unchanged: exam must not show Agent state or event trace.

- [ ] **Step 3: Preserve pending answer and busy state**

Confirm existing JSX remains in the conversation flow:

```tsx
{pendingAnswer && <article className="bubble answer pending">...</article>}
{busy && <div className="system-line">正在评估回答，准备下一题...</div>}
```

Do not move these into the auxiliary section.

- [ ] **Step 4: Add CSS for auxiliary section**

Add styles:

```css
.auxiliary-section {
  margin: 0 0 14px;
}

.auxiliary-section details {
  background: rgba(255, 255, 255, 0.78);
  border: 1px solid var(--line);
  border-radius: var(--radius);
  box-shadow: var(--shadow-tight);
}

.auxiliary-section summary {
  cursor: pointer;
  padding: 11px 14px;
  color: #334155;
  font-size: 13px;
  font-weight: 900;
}

.auxiliary-content {
  display: grid;
  gap: 12px;
  padding: 0 14px 14px;
}

.auxiliary-content .agent-state,
.auxiliary-content .retrieval-trace {
  margin: 0;
  box-shadow: none;
}
```

- [ ] **Step 5: Run focused tests**

Run:

```powershell
npm --prefix web run test -- candidatePages.test.tsx
```

Expected: interview diagnostics tests pass; exam diagnostics tests still pass.

## Task 4: Make Report Conclusions First

**Files:**
- Modify: `web/src/candidatePages.tsx`
- Modify: `web/src/styles.css`
- Modify: `web/src/candidatePages.test.tsx`

- [ ] **Step 1: Reorder ReportPage sections**

In `ReportPage`, keep `report-hero` first. Immediately after it, render high-level action sections before deep diagnostics:

Recommended order:

```tsx
<div className="report-hero">...</div>
<div className="report-summary">...</div>
<DrillPlanPanel ... />
<ReportRoundReviews session={session} />
<TranscriptPanel analysis={report.transcript_analysis} />
<ProfileAnalysisPanel analysis={session.profile_analysis} />
{session.mode === "practice" && (
  <AuxiliarySection title="辅助诊断">
    <AgentStatePanel memory={session.working_memory} />
    <RetrievalTracePanel trace={session.retrieval_trace} />
  </AuxiliarySection>
)}
```

This puts conclusions and training before backend diagnostics. If transcript analysis is more useful before round reviews after implementation review, keep the same principle: action/conclusion before trace.

- [ ] **Step 2: Preserve drill plan question jumps**

Do not change this call:

```tsx
<DrillPlanPanel plan={report.drill_plan || []} startDrill={startDrill} jumpQuestion={jumpQuestion} />
```

Confirm recommended question buttons still call `jumpQuestion(id)` inside `DrillPlanPanel`.

- [ ] **Step 3: Keep practice/exam diagnostic behavior unchanged**

Practice reports may show `AgentStatePanel` and `RetrievalTracePanel` inside `AuxiliarySection`.

Exam reports must still hide both panels:

```tsx
{session.mode === "practice" && (...)}
```

- [ ] **Step 4: Adjust report CSS density**

Add or adjust:

```css
.report-page .auxiliary-section {
  margin-top: 18px;
}

.report-summary {
  margin-bottom: 18px;
}

.report-summary section {
  min-width: 0;
}
```

Keep existing score cards and meters compatible.

- [ ] **Step 5: Run focused tests**

Run:

```powershell
npm --prefix web run test -- candidatePages.test.tsx
```

Expected: report round review fallback, exam hiding, retrieval trace, working memory, and new ordering tests pass.

## Task 5: Responsive Polish, Verification, and Change Documentation

**Files:**
- Modify: `web/src/styles.css`
- Modify: `openspec/changes/improve-frontend-workbench-layout/tasks.md`
- Create: `docs/code-changes/06-07-前端工作台布局优化.md`
- Do not modify: `internal/httpapi/web/dist` unless separately confirmed

- [ ] **Step 1: Update narrow-screen app shell rules**

In `styles.css`, refine the `@media (max-width: 980px)` block so the sidebar becomes compact and does not dominate the page:

```css
@media (max-width: 980px) {
  .app-shell {
    grid-template-columns: 1fr;
  }

  .sidebar {
    position: static;
    height: auto;
    padding: 14px;
  }

  .session-list {
    max-height: 180px;
  }

  .main {
    padding: 16px;
  }
}
```

If the current sidebar still appears too tall because user ID and sessions remain expanded, add CSS to reduce session density rather than removing controls.

- [ ] **Step 2: Update narrow-screen answer dock**

Inside the same media query, add:

```css
.answer-dock {
  grid-template-columns: 1fr;
  bottom: 10px;
}

.send {
  width: 100%;
}

.conversation {
  padding-bottom: 220px;
}
```

Verify this does not regress desktop layout.

- [ ] **Step 3: Update OpenSpec task checkboxes as work is completed**

After implementation tasks are complete, update `openspec/changes/improve-frontend-workbench-layout/tasks.md` from `- [ ]` to `- [x]` for completed items. Do not mark verification tasks done until commands have actually run.

- [ ] **Step 4: Create code change documentation from actual diff**

Create `docs/code-changes/06-07-前端工作台布局优化.md` with these sections:

```md
# 06-07 前端工作台布局优化

## 1. 变更概述

## 2. 变更文件

## 3. 函数级说明

## 4. 调用链

## 5. 数据流

## 6. 依赖与副作用

## 7. 测试

## 8. 风险
```

Fill it from the real implementation diff only. Do not claim browser verification unless it was performed.

- [ ] **Step 5: Run frontend tests**

Run:

```powershell
npm --prefix web run test
```

Expected: all Vitest tests pass.

- [ ] **Step 6: Run frontend build**

Run:

```powershell
npm --prefix web run build
```

Expected: TypeScript build and Vite build pass.

If build modifies `internal/httpapi/web/dist`, inspect with:

```powershell
git status --short
git diff --stat -- internal/httpapi/web/dist
```

Do not overwrite or stage pre-existing dist changes without explicit confirmation.

- [ ] **Step 7: Run OpenSpec validation**

Run:

```powershell
openspec validate improve-frontend-workbench-layout --strict
```

If OpenSpec CLI fails inside sandbox with Node `EPERM`, rerun with the same command under approved elevated permissions.

- [ ] **Step 8: Final diff review**

Run:

```powershell
git status --short
git diff -- web/src/main.tsx web/src/candidatePages.tsx web/src/sharedView.tsx web/src/styles.css web/src/candidatePages.test.tsx docs/code-changes/06-07-前端工作台布局优化.md openspec/changes/improve-frontend-workbench-layout docs/superpowers/specs/2026-06-07-frontend-workbench-layout-design.md docs/superpowers/plans/2026-06-07-frontend-workbench-layout.md
```

Confirm the diff does not modify backend contracts, Go code, `apiClient.ts`, `types.ts`, or unrelated documentation.
