# 06-09 题库导入审核工作台

## 1. 变更概述

将 `/questions` 页面里的导入审核区从简单列表升级为三栏题库审核工作台：左侧 import job 批次，中间 review queue，右侧题目详情与发布门禁。目标是让 103 道暂存题能在前端高效人工审核、批量接受/拒绝，并在提交入正式题库前看到字段完整度、Agent 拒绝、无效题和最近 commit summary。

本次不新增后端 API，不改候选人面试页。前端继续复用现有接口：

- `GET /api/question-bank/imports`
- `GET /api/question-bank/imports/:id`
- `POST /api/question-bank/imports/:id/items/review`
- `POST /api/question-bank/imports/:id/commit?async=true`

## 2. 变更文件

- `web/src/types.ts`
  - 为导入任务补充 `metadata.commit_summary`。
  - 为导入条目补充 `agent_review_status`、`agent_review_reason`。
- `web/src/questionBankImportView.ts`
  - 新增导入审核 contract helper：字段完整度、筛选、指标、commit summary。
- `web/src/questionBankImportView.test.ts`
  - 增加 helper 回归测试。
- `web/src/questionBankReviewWorkbench.tsx`
  - 新增三栏审核工作台组件。
- `web/src/questionBankPage.tsx`
  - 接入 `QuestionBankReviewWorkbench`，移除旧的内联 `ImportDetail`/`ImportDiff`。
- `web/src/styles.css`
  - 新增工作台布局、状态、发布门禁和响应式样式。
- `internal/httpapi/web/dist/*`
  - `npm --prefix web run build` 生成的嵌入式前端产物。

## 3. 函数级说明

### `web/src/questionBankImportView.ts`

- `hasImportAnswerCompleteness(importItem)`
  - 输入：`QuestionBankImportItem`。
  - 输出：布尔值。
  - 逻辑：要求题干、`expected_points`、`rubric`、`sample_answer`、`follow_up_hints` 都非空。
  - 副作用：无。

- `importItemReviewFlags(importItem)`
  - 输入：导入条目。
  - 输出：标准化状态 flags，包括 valid/invalid、accepted/rejected、complete/incomplete、missingRubric、missingExpectedPoints、agentRejected。
  - 作用：避免组件里重复拼状态判断。

- `buildImportReviewMetrics(job, items, selectedIds)`
  - 输入：当前 job、导入条目、选中条目集合。
  - 输出：工作台统计数据。
  - 逻辑：优先使用 job 上的 total/valid/invalid/imported 计数，接受/拒绝/完整度基于当前 items 计算。

- `filterImportItems(items, filter, query)`
  - 输入：条目列表、筛选类型、搜索词。
  - 输出：筛选后的条目。
  - 搜索字段：导入 item id、question id、题目 id、题干、技能、场景、来源、tags、role_tags。

- `commitSummary(job)`
  - 输入：导入任务。
  - 输出：`job.metadata.commit_summary` 或 `undefined`。

### `web/src/questionBankReviewWorkbench.tsx`

- `QuestionBankReviewWorkbench(props)`
  - 输入：jobs、当前 job id、items、busy 状态、导入来源、上传/选择/提交/审核回调。
  - 输出：React JSX。
  - 主要逻辑：
    - 左栏渲染 import jobs。
    - 中栏支持搜索、状态筛选、当前结果全选、批量接受/拒绝。
    - 右栏渲染发布门禁、危险区和选中题详情。
    - `accept_complete_valid` 是主批量操作。
    - `accept_all_valid` 放在危险区，必须勾选确认。
  - 副作用：无直接网络副作用；通过 props 回调交给 `QuestionBankPage` 调用 API。

- `Metric(props)`
  - 渲染顶部统计小块。

- `GateLine(props)`
  - 渲染发布门禁行。

- `ReviewQueueItem(props)`
  - 渲染中栏单题摘要和选择框。

- `ReviewItemDetail(props)`
  - 渲染右栏单题详情、单题接受/拒绝、Agent 审核、校验错误、评分要点、rubric、参考回答、追问提示和 diff。

### `web/src/questionBankPage.tsx`

- `QuestionBankPage`
  - 行为变化：导入审核区域改为 `QuestionBankReviewWorkbench`。
  - 保留原有数据流：上传文件、轮询 import jobs、加载 selected import items、review、commit、刷新正式题库。

## 4. 调用链

### 页面加载

用户打开 `/questions`
-> `QuestionBankPage`
-> `apiClient.listQuestionImports()`
-> `apiClient.getQuestionImport(selectedImportId)`
-> `QuestionBankReviewWorkbench`
-> helper 计算筛选、指标和门禁。

### 上传导入

用户点击“上传导入”
-> `QuestionBankReviewWorkbench.onUploadClick`
-> `QuestionBankPage` 触发隐藏 file input
-> `uploadImport(file)`
-> `apiClient.createQuestionImport(source, file)`
-> `refreshImports()`。

### 人工审核

用户选择单题或批量条目
-> `QuestionBankReviewWorkbench`
-> `onReview(job.id, action, itemIds)`
-> `QuestionBankPage.reviewImport`
-> `apiClient.reviewQuestionImportItems`
-> 更新 `importItems`
-> 刷新 jobs。

### 发布入库

用户点击“提交入库并生成向量”
-> `QuestionBankReviewWorkbench.onCommit`
-> `QuestionBankPage.commitImport`
-> `apiClient.commitQuestionImport(id)`
-> `refreshImports()`
-> `load()` 刷新正式题库列表。

## 5. 数据流

后端返回 import job 和 items。前端不会修改数据结构，只在 UI 层派生：

- `review_status` 决定接受/拒绝。
- `status` 决定 valid/invalid。
- `agent_review_status` 和 `agent_review_reason` 作为审核风险提示。
- `item.expected_points`、`item.rubric`、`item.sample_answer`、`item.follow_up_hints` 组合成“字段完整”判断。
- `metadata.commit_summary` 展示最近一次 commit 结果。

人工动作仍通过后端 review API 写回，不在前端本地伪造审核结果。

## 6. 依赖与副作用

- 没有新增 npm 依赖。
- 没有新增后端接口。
- 没有修改数据库 schema。
- 构建会更新 `internal/httpapi/web/dist`，用于 Go 后端嵌入静态前端资源。
- `accept_all_valid` 仍可调用，但前端要求勾选确认，降低误操作风险。

## 7. 测试

已执行：

```powershell
npm --prefix web run test
npm --prefix web run build
```

结果：

- Vitest：8 个测试文件，43 个测试通过。
- Build：`tsc -b && vite build` 通过，并生成 `internal/httpapi/web/dist`。

访问验证：

```powershell
Invoke-WebRequest -Uri "http://127.0.0.1:5173/questions" -UseBasicParsing -TimeoutSec 8
```

结果：HTTP 200。

## 8. 风险

- 前端字段完整度是工程门禁，不替代后端 schema 校验；后端仍是最终事实源。
- `metadata.commit_summary` 是可选字段，旧 job 不带该字段时 UI 不展示 summary。
- `agent_review_status` 是可选字段，旧导入项不带该字段时不会影响审核。
- 当前没有新增 React 组件级 DOM 测试；主要用 helper 测试覆盖审核规则，用 build 覆盖类型与 JSX。
- 本轮未使用浏览器截图工具做像素级检查，因为当前会话没有可用的 in-app browser 控制工具，且本地 Playwright 未安装。
