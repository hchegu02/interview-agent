# 题库审核工作台设计

## 背景

当前后端已经具备题库导入、暂存、人工 review、commit 发布事务、embedding 诊断和 RAG eval 工具链。现有 `/questions` 页面已有基础导入审核能力，但更像“列表 + 按钮”：可以 accept/reject/commit，但人工处理 100+ 道题时效率低，缺少上线前需要的审核队列、字段完整度、来源证据、批量选择、发布门禁和 commit summary 可视化。

参考 `nzy0510/AI-Interview` 的方向不是照搬页面，而是学习题库后台的业务流程：导入包先进入草稿/暂存区，经过校验和人工确认后发布；发布后才进入面试/RAG 检索链路，并保留检索、向量和发布状态诊断。

## 目标

把 `/questions` 管理视图升级为“题库审核工作台”，让管理员可以高效完成：

1. 选择 import job。
2. 按状态、字段完整度、技能、难度、关键词筛选题目。
3. 快速查看题干、expected_points、rubric、sample_answer、follow_up_hints、来源证据和错误。
4. 多选批量 accept/reject。
5. 使用 `accept_complete_valid` 安全接受字段完整题。
6. commit 前看发布门禁，commit 后看 summary 和 embedding 结果。

## 非目标

- 不新增后端 API，优先使用现有接口。
- 不做完整 Admin UI 平台。
- 不做额度、运营统计、用户权限系统。
- 不修改候选人面试页。
- 不改变题库导入/commit 后端语义。

## 页面结构

采用三栏工作台：

```text
顶部：全局审核统计 + 发布门禁摘要

左栏：Import Jobs
中栏：Review Queue
右栏：Selected Item Detail / Publish Gate
```

### 顶部状态区

展示当前选中或全部 import jobs 的聚合数据：

- total valid
- accepted
- rejected
- pending human review
- complete valid
- invalid
- imported
- embedding failed

顶部右侧显示发布门禁：

- job status 是否 `ready`
- accepted 是否大于 0
- 是否存在 `needs_human_review`
- 是否存在 agent rejected / dirty content
- commit summary 是否可用

### 左栏：Import Jobs

每个 job 卡片展示：

- 文件名或 source
- job id
- status
- valid / invalid / imported
- accepted / rejected / pending
- 简短进度条

点击 job 后：

- 调 `GET /api/question-bank/imports/:id`
- 中栏切换到该 job 的 items
- 右栏清空或显示 job summary

### 中栏：Review Queue

中栏是主要工作区，支持：

- 搜索题干、ID、tag、skill_category。
- 筛选：
  - 全部
  - 只看待人工确认
  - 字段完整
  - 缺 rubric
  - 缺 expected_points
  - agent rejected
  - invalid
  - accepted
  - rejected
- 技能分类筛选。
- 难度筛选。
- 多选当前筛选结果中的题。

每个题目行展示：

- checkbox
- question_id
- content
- skill_category / difficulty / scenario
- status + review_status + agent_review_status
- 字段完整度 chips：
  - expected_points count
  - rubric
  - sample_answer
  - follow_up_hints count
  - source provenance
- 行内按钮：
  - 接受
  - 拒绝
  - 详情

批量操作：

- 接受选中
- 拒绝选中
- 接受字段完整
- 拒绝当前筛选

为了降低误操作，`accept_all_valid` 不作为主按钮，最多放在折叠菜单或危险操作区。

### 右栏：详情与发布门禁

选中题目时展示 item 详情：

- 题干
- expected_points
- rubric
- sample_answer
- follow_up_hints
- tags / role_tags / scenario / difficulty
- errors
- field_provenance
- original_item vs item diff
- agent_review_status / agent_review_reason

未选中题目时展示 job 发布面板：

- accepted / rejected / pending / invalid 汇总
- commit 按钮
- async commit 选项默认开启
- commit 后展示 `metadata.commit_summary`：
  - matched
  - imported
  - skipped
  - embedding_synced
  - embedding_failed
  - failure_reasons
  - embedding_errors

## 数据接口

复用现有 API：

- `GET /api/question-bank/imports`
- `GET /api/question-bank/imports/:id`
- `POST /api/question-bank/imports/:id/items/review`
- `POST /api/question-bank/imports/:id/commit?async=true`
- `GET /api/question-bank`
- `GET /api/question-bank/:id?view=admin`

前端已有 `apiClient` 方法：

- `questionImports`
- `questionImport`
- `reviewQuestionImportItems`
- `commitQuestionImport`

本次不新增 API。

## 前端组件拆分

在现有 `web/src/questionBankPage.tsx` 基础上拆出或局部新增：

- `QuestionBankPage`
  - 保留题库查询和 adminDefault 入口。
- `ImportReviewWorkbench`
  - 管理 import jobs、选中 job、选中 item、批量选择状态。
- `ImportJobRail`
  - 左栏 job 列表。
- `ImportReviewQueue`
  - 中栏筛选、搜索、题目列表、批量操作。
- `ImportItemDetail`
  - 右栏题目详情。
- `ImportPublishPanel`
  - 右栏发布门禁和 commit summary。
- `CompletenessChips`
  - 复用字段完整度判断。

如果文件过大，优先新增 `web/src/questionBankReviewWorkbench.tsx`，避免继续膨胀 `questionBankPage.tsx`。

## 状态管理

局部 React state 足够，不引入全局状态库：

- `imports`
- `selectedImportId`
- `importItems`
- `selectedItemId`
- `selectedItemIds`
- `filters`
- `importBusy`
- `notice`

状态刷新规则：

- 页面进入时加载 imports。
- 选择 job 时加载 job details。
- review 成功后用响应中的 items 更新当前队列，同时刷新 imports。
- commit 成功后刷新 imports 和当前 job。
- 有 running import/commit 时保留现有轮询逻辑。

## 错误处理

- review/commit 失败显示顶部 notice。
- commit 被后端拒绝时保留后端错误原文，例如 requires human review。
- invalid items 不允许 accept。
- job 非 ready 状态禁用 review 主操作。
- commit 按钮在 accepted 为 0 时禁用。
- `accept_all_valid` 使用二次确认或移入危险操作区。

## 视觉方向

风格：安静、密集、工程后台。避免营销式卡片堆叠。

特点：

- 顶部是状态和门禁，不是 hero。
- 三栏稳定布局，适合反复操作。
- 颜色只表达状态：
  - accepted：绿色
  - rejected/error：红色
  - pending/warning：琥珀色
  - selected/focus：蓝色
- 字段完整度使用 chips。
- 详情区用分组面板，不把长文本塞进列表。

## 测试计划

前端测试：

- `QuestionBankPage` 能渲染 import jobs。
- 选择 job 后显示 review queue。
- 筛选待确认、字段完整、缺 rubric。
- 多选后调用 `reviewQuestionImportItems`，payload 正确。
- accept/reject 单项按钮 payload 正确。
- commit 按钮在 ready + accepted > 0 时可用。
- commit summary 能正确展示。

验证命令：

```powershell
npm --prefix web run test
npm --prefix web run build
```

浏览器验证：

- 打开 `http://127.0.0.1:5173/questions`
- 管理后台模式能看到三栏审核工作台。
- 模拟 5 个 import jobs 能快速筛选、选择、accept/reject。
- 页面在桌面宽度不重叠；移动宽度退化为纵向布局。

## 风险

- 现有 `questionBankPage.tsx` 可能已经偏大，继续堆逻辑会降低可维护性；实现时应拆工作台组件。
- 后端当前没有“待改”状态，前端不要伪造新状态；只能用 accepted/rejected/pending 展示。
- 真实 103 道题的文本可能很长，列表必须截断，详情区完整显示。
- commit summary 在未 commit 前不存在，UI 要区分“未发布”和“已发布”。
