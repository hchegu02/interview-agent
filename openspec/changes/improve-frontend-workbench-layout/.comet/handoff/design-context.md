# Comet Design Handoff

- Change: improve-frontend-workbench-layout
- Phase: design
- Mode: compact
- Context hash: bab0e681166a618d7aff31b9cb6c66df586d023f0821dabd673520441ede1c17

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/improve-frontend-workbench-layout/proposal.md

- Source: openspec/changes/improve-frontend-workbench-layout/proposal.md
- Lines: 1-31
- SHA256: 90f9b03baf4272656019f42f4f4b5cb74d40cd83ca9eac2bfb19474c78078d99

```md
## Why

当前前端已经能覆盖完整面试训练链路，但候选人工作台的信息层级偏重：侧栏、面试页和报告页同时承担业务操作、调试展示和后端能力展示，用户在答题和复盘时需要过滤太多次要信息。

本变更将前端优化限定在纯界面和信息架构层，目标是在不改变后端 API、Session 数据结构和 Agent Graph 流程的前提下，让候选人更快完成“准备、答题、复盘、再训练”的主路径。

## What Changes

- 优化候选人工作台的导航和页面信息层级，减少全局侧栏的认知负担。
- 调整面试页布局，让当前题目、回答输入和反馈成为默认主视图。
- 调整报告页布局，让总分、关键弱项、下一步训练建议优先展示。
- 将 Agent 状态、SSE 事件、RAG 检索链路等后端可解释信息降级为次级或可折叠展示。
- 改进窄屏和移动端布局，避免侧栏、历史会话和辅助面板挤占主任务区域。
- 不新增后端接口，不修改现有 JSON 字段，不改变面试状态机、题库检索、评分或报告生成逻辑。

## Capabilities

### New Capabilities

- `candidate-workbench-layout`: 候选人工作台的前端布局、信息层级、响应式行为和辅助诊断信息展示约束。

### Modified Capabilities

- 无。

## Impact

- 主要影响 `web/src/main.tsx`、`web/src/candidatePages.tsx`、`web/src/questionBankPage.tsx`、`web/src/sharedView.tsx` 和 `web/src/styles.css`。
- 可能补充或调整前端组件测试，覆盖关键渲染分支和折叠/响应式相关结构。
- 不影响 Go 后端、数据库 schema、API 路径、API 响应字段、SSE 事件契约或嵌入式 web dist 之外的运行时逻辑。
- 构建产物 `internal/httpapi/web/dist` 只有在执行前端构建并需要同步嵌入产物时才应更新；该更新需与现有未提交 dist 改动分开确认。
```

## openspec/changes/improve-frontend-workbench-layout/design.md

- Source: openspec/changes/improve-frontend-workbench-layout/design.md
- Lines: 1-80
- SHA256: a038c63042a94f58321b8b73e477e2477b6fd08637ad1ea3755fbc2f9654ecbe

```md
## Context

当前前端位于 `web/src`，核心入口为 `main.tsx`，候选人页面集中在 `candidatePages.tsx`，题库管理页在 `questionBankPage.tsx`，全局样式在 `styles.css`。现有布局已经覆盖完整链路：

- `ResumePage`：简历录入和解析结果编辑。
- `JDPage`：JD 输入、简历匹配分析和题库范围配置。
- `InterviewPage`：面试对话、回答提交、反馈、SSE 事件和 WorkingMemory 展示。
- `ReportPage`：总分、逐题评分、训练计划、回答诊断、Agent 状态和 RAG trace。
- `QuestionBankPage`：题库导入、筛选、列表和详情。

主要问题不是功能缺失，而是候选人主任务和后端诊断信息混杂。前端 SDD 明确要求前端作为候选人工作台，业务事实以后端 `Session` 为准，因此本设计只调整 UI 信息架构，不复制后端状态机。

## Goals / Non-Goals

**Goals:**

- 让候选人模式围绕“准备 -> 答题 -> 复盘 -> 再训练”形成更清晰的信息层级。
- 面试页默认突出当前题、回答输入、反馈和历史轮次。
- 报告页默认突出总分、关键弱项、证据和下一步训练动作。
- 将 Agent 状态、SSE 事件、WorkingMemory 和 RAG trace 作为辅助诊断信息展示，避免默认抢占主任务区域。
- 改善窄屏布局，减少侧栏和历史会话对主内容的挤占。
- 保持现有后端 API、SSE、Session JSON 字段和前端类型兼容。

**Non-Goals:**

- 不新增、删除或重命名后端 API。
- 不修改 Go 后端、数据库 schema、RAG 排序、评分、报告生成或 Agent Graph 决策。
- 不引入新的前端框架、UI 组件库或状态管理库。
- 不把当前开发模式用户 ID、owner header 或 Agent trace 包装成生产级身份系统。
- 不默认更新嵌入式 dist 产物，除非后续实现阶段明确需要同步构建产物。

## Decisions

### Decision 1: 保持单页工作台，不改路由契约

继续使用现有 `routes.ts` 的页面集合和 `main.tsx` 的客户端路由，不新增深层路由。布局优化通过组件结构、折叠区和 CSS 完成。

理由：当前路由已经与后端链路对齐。新增复杂路由会增加历史 session、报告跳题和题库跳转的兼容风险，收益不大。

备选方案：把报告页拆成多个路由。拒绝原因是会放大状态同步和导航复杂度，且不解决主次层级问题。

### Decision 2: 侧栏降负，页面内承接局部操作

全局侧栏保留工作区切换、主导航、连接状态和必要会话入口；与当前页面强相关的操作尽量放回页面内，例如面试模式选择应靠近 JD/start 操作，历史会话应可折叠或降低默认占用。

理由：候选人正在填写 JD 或答题时，不需要长期看到所有会话和用户配置。侧栏越重，主任务越不突出。

备选方案：完全移除侧栏改为顶部导航。拒绝原因是桌面端工作台场景下侧栏仍适合承载多模块导航，且现有结构改动更小。

### Decision 3: 面试页采用主任务优先布局

`InterviewPage` 默认优先展示当前题、回答输入、历史轮次和反馈。`AgentStatePanel`、`EventTimeline` 等可解释和调试信息应移动到次级区域或折叠区。

理由：答题时用户关注的是题目和回答质量。后端状态对演示重要，但不应和当前题竞争视觉层级。

备选方案：保留所有信息纵向堆叠，只调样式。拒绝原因是信息架构问题不能靠颜色和间距解决。

### Decision 4: 报告页采用结论先行布局

`ReportPage` 顶部保留总分和技能拆解，但应更早展示关键弱项、下一步训练计划和逐题证据入口。RAG trace、WorkingMemory 等后端证据链默认作为辅助信息。

理由：报告页的用户任务是知道哪里差、为什么差、下一步怎么练。检索链路是可信解释，不是复盘主线。

备选方案：把所有报告模块继续按后端字段顺序展示。拒绝原因是字段顺序不是用户阅读顺序。

### Decision 5: 响应式设计单独处理主任务入口

窄屏下不只把 grid 改成单列，还要保证主内容先出现，侧栏和辅助信息可折叠或降级。底部回答输入框必须避免遮挡内容，并保持足够触控面积。

理由：当前 `@media (max-width: 980px)` 主要做列数切换，移动端会先看到完整侧栏，主任务入口太靠后。

备选方案：只继续扩展现有媒体查询。可作为实现手段，但必须围绕主任务优先重新排序，而不是机械单列。

## Risks / Trade-offs

- [Risk] 调整 JSX 结构可能影响现有组件测试快照或查询方式。→ Mitigation：优先补基于可见文本和语义结构的测试，避免脆弱快照。
- [Risk] 折叠诊断信息可能降低开发演示时的可见性。→ Mitigation：保留入口和内容，不删除数据展示；必要时提供明确标题和展开状态。
- [Risk] 侧栏降负可能影响历史会话加载效率。→ Mitigation：历史会话功能保留，只调整默认层级和展示密度。
- [Risk] 移动端 sticky 输入框可能遮挡对话内容。→ Mitigation：保留 conversation 底部安全留白，并在窄屏下调整输入框布局。
- [Risk] 若实现阶段同步 dist，会碰到当前已有未提交 dist 改动。→ Mitigation：实现前重新检查 git 状态，未经确认不覆盖用户已有构建产物。
```

## openspec/changes/improve-frontend-workbench-layout/tasks.md

- Source: openspec/changes/improve-frontend-workbench-layout/tasks.md
- Lines: 1-37
- SHA256: c11dbd8bfc7740fa9471b277e668c56631c0cdaeec806247f0b5c9388db285da

```md
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
```

## openspec/changes/improve-frontend-workbench-layout/specs/candidate-workbench-layout/spec.md

- Source: openspec/changes/improve-frontend-workbench-layout/specs/candidate-workbench-layout/spec.md
- Lines: 1-76
- SHA256: 290cbb2285486c0147340236273e6d940b62064307b90ecf11745f37d9c87037

```md
## ADDED Requirements

### Requirement: Candidate workflow navigation preserves task focus
The frontend SHALL present candidate workflow navigation in a way that keeps preparation, interview, report, agent, and memory pages reachable without allowing global controls to dominate the main task area.

#### Scenario: Candidate workspace shows workflow navigation
- **WHEN** the user is in the candidate workspace
- **THEN** the interface MUST expose navigation to resume, JD analysis, interview, report, Agent, and memory views

#### Scenario: Page-specific actions stay near their page context
- **WHEN** the user configures an interview mode or starts an interview
- **THEN** the relevant controls MUST be visually associated with the preparation/start workflow rather than presented only as unrelated global settings

#### Scenario: Historical sessions do not block the main workflow
- **WHEN** historical sessions are available
- **THEN** the interface MUST keep session loading reachable without forcing the session list to consume primary screen space needed for the current workflow

### Requirement: Interview page prioritizes active answering
The frontend SHALL make the current question, answer input, prior answers, and feedback the primary default content of the interview page.

#### Scenario: Active question is the primary interview content
- **WHEN** an interview session has a current question
- **THEN** the current question and answer input MUST be easier to find than Agent state, SSE event details, or retrieval diagnostics

#### Scenario: Auxiliary execution state remains available
- **WHEN** a practice session includes WorkingMemory or stream events
- **THEN** the interface MUST keep those details accessible as auxiliary information without removing or changing the underlying data

#### Scenario: Submitting an answer preserves task continuity
- **WHEN** the user submits an answer and the backend is processing
- **THEN** the interface MUST show the pending answer or processing state without hiding the conversation context

### Requirement: Report page presents conclusions before diagnostics
The frontend SHALL present report conclusions and next training actions before low-level backend diagnostics.

#### Scenario: Report summary appears first
- **WHEN** a completed session has a report
- **THEN** the page MUST present overall score and skill breakdown before detailed diagnostics

#### Scenario: Training actions are surfaced before trace details
- **WHEN** the report contains a drill plan or next steps
- **THEN** the page MUST surface actionable training guidance before RAG retrieval trace details

#### Scenario: Backend explainability remains available
- **WHEN** a practice report contains WorkingMemory or retrieval trace data
- **THEN** the interface MUST keep that explainability data accessible without making it the dominant default reading path

### Requirement: Responsive layout preserves primary tasks
The frontend SHALL adapt to narrow screens by preserving access to the primary task before secondary navigation and diagnostics.

#### Scenario: Narrow candidate page keeps main task reachable
- **WHEN** the viewport is narrow
- **THEN** resume editing, JD editing, active answering, or report review content MUST remain reachable without first scrolling through a full desktop-style sidebar

#### Scenario: Narrow interview input remains usable
- **WHEN** the viewport is narrow and the user is on the interview page
- **THEN** the answer input and send action MUST remain usable without overlapping conversation content

#### Scenario: Narrow report modules remain readable
- **WHEN** the viewport is narrow and the report has multiple modules
- **THEN** report sections MUST stack or collapse in a way that avoids horizontal overflow and preserves readable text

### Requirement: Frontend UI optimization preserves backend contracts
The frontend SHALL NOT require backend contract changes for this layout optimization.

#### Scenario: Existing API responses remain sufficient
- **WHEN** the optimized frontend renders resume, JD, interview, report, Agent, memory, or question bank pages
- **THEN** it MUST use existing API responses and tolerate optional fields according to existing frontend types

#### Scenario: Session remains the source of truth
- **WHEN** a session is loaded, updated through REST, or updated through SSE
- **THEN** the frontend MUST continue to treat the backend Session snapshot as the authoritative workflow state

#### Scenario: No frontend-only graph decisions
- **WHEN** the user answers a question or views a report
- **THEN** the frontend MUST NOT decide whether to ask follow-ups, change questions, generate reports, or alter scoring outside the existing backend flow
```

