---
comet_change: improve-frontend-workbench-layout
role: technical-design
canonical_spec: openspec
---

# Frontend Workbench Layout Design

## Context

Interview Agent 前端当前是一个候选人工作台，主要代码位于 `web/src`：

- `main.tsx` 负责应用壳、侧栏、工作区切换、路由分发、Session 拉取和共享状态。
- `candidatePages.tsx` 承载简历、JD、面试、报告、Agent 和长期画像页面。
- `questionBankPage.tsx` 承载题库管理页。
- `styles.css` 承载全局视觉、布局和响应式规则。

现有实现的问题不是功能不通，而是信息层级偏重。候选人在答题和复盘时同时看到业务操作、后端诊断信息、状态 trace 和历史会话入口，主任务不够突出。本设计只优化前端布局和信息架构，不修改后端 API、Session JSON、SSE 契约、RAG、评分或 Agent Graph。

## Technical Approach

采用“轻结构调整 + CSS 重排”的保守方案：

1. 保留现有路由集合和前端状态流。
2. 调整候选人页面的 JSX 分区，让主任务、辅助控制和诊断信息有明确层级。
3. 通过少量共享 UI 结构承载辅助/诊断内容，例如可折叠或次级说明区。
4. 在 `styles.css` 中重写关键区域的密度、间距和响应式规则。
5. 不引入新的 UI 库、图标库、状态管理库或 CSS 框架。

该方案改动范围比纯 CSS 大，但能解决真正的问题：页面顺序和 DOM 层级本身已经把诊断信息放在主阅读流里，单靠颜色无法修正。

## Component Decisions

### App Shell

`main.tsx` 保持现有 `app-shell` 和 `sidebar` 架构，但降低侧栏默认负担：

- 工作区切换和路由导航保留在侧栏。
- 用户 ID、历史会话、连接状态继续可达。
- 历史会话和用户配置应降低默认视觉权重，避免和当前页面主任务竞争。
- 面试模式选择应靠近 JD/start 操作，而不是只作为全局设置孤立存在。

不拆分路由，不新增复杂导航状态。原因是当前 `routes.ts` 已经承载了报告跳题和题库跳转，新增深层路由会扩大兼容风险。

### Interview Page

`InterviewPage` 改为主任务优先：

- 顶部保留 session、模式和状态。
- 当前题、对话历史、回答反馈和 pending answer 保持默认可见。
- 回答输入继续使用 sticky dock，但窄屏下需要改成单列并保留底部安全留白。
- `AgentStatePanel` 和 `EventTimeline` 进入辅助诊断区。内容不删除，字段不改名，展示权重降低。

这样后端 WorkingMemory 和 SSE 仍可用于演示和排查，但不会抢答题主线。

### Report Page

`ReportPage` 改为结论先行：

- 顶部继续展示总分和技能拆解。
- 训练计划、关键改进项、逐题证据入口前置到诊断信息之前。
- `AgentStatePanel` 和 `RetrievalTracePanel` 保留，但作为辅助解释区后置或折叠。
- `jumpQuestion` 行为保持不变，题库题号按钮仍跳转到 `/questions?q=...`。

报告页的用户任务是“哪里差、为什么差、下一轮练什么”。RAG trace 是可信解释，不是默认阅读主线。

### JD and Resume Pages

`ResumePage` 和 `JDPage` 现有两栏结构基本合理：

- 主输入区继续占据主要宽度。
- 右侧控制面板继续承载上传、分析、启动等动作。
- JD 页应承接模式选择，使用户在启动面试前确认模拟/考试模式。
- 题库范围配置仍只在 practice 模式显示，不改变请求 payload 规则。

### Question Bank Page

题库页属于管理后台，保留列表 + 详情结构。只做必要的响应式和密度协调，不把候选人布局规则强行套进管理页。

## Data Flow

数据流保持不变：

```text
Draft(localStorage)
  -> ResumePage / JDPage
  -> apiClient.startInterview
  -> backend Session
  -> InterviewPage / ReportPage
```

SSE 仍只作为增量提示：

```text
REST Session snapshot = authoritative state
SSE event stream       = progress and trace display
```

本变更不新增前端派生状态来判断追问、换题、生成报告或评分。所有业务决策继续由后端 Session 和 Agent Graph 负责。

## Styling Direction

视觉方向是克制的工作台，而不是营销页：

- 减少装饰性大卡片，优先信息密度和扫描效率。
- 保持 8px 左右圆角和现有蓝/青/深色状态体系，避免大范围换肤。
- 主任务区域用更清晰的宽度、间距和标题层级。
- 诊断信息用较低权重、折叠区或后置分组。
- 移动端优先保证主任务先出现，避免完整桌面侧栏挡在内容前。

## Testing Strategy

最小验证包括：

- 更新或新增组件测试，确认候选人导航、面试诊断区、报告诊断区仍可渲染。
- 验证 answer pending/processing 状态仍保留。
- 验证 drill plan 的题库跳转按钮仍调用 `jumpQuestion`。
- 运行 `npm --prefix web run test`。
- 运行 `npm --prefix web run build`。

实现后如果同步嵌入式 dist，必须先重新检查 `internal/httpapi/web/dist` 的既有未提交改动，不能覆盖用户已有产物。

## Risks

- JSX 结构调整可能使现有测试查询失效。测试应转向语义文本和可见结构，避免脆弱 DOM 细节。
- 诊断区降级可能影响开发演示。内容必须保留，只调整默认权重和位置。
- 侧栏降负可能降低历史会话入口显眼程度。必须保留刷新和加载会话能力。
- sticky answer dock 在窄屏下容易遮挡内容。需要同步调整 conversation 底部留白。
- dist 产物已有未提交改动，构建同步需要单独处理。

## Spec Patches

无。当前 OpenSpec delta spec 已覆盖前端布局、报告优先级、响应式和后端契约保持不变的验收场景，不需要补充。
