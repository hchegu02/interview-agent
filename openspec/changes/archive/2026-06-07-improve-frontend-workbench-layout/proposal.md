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
