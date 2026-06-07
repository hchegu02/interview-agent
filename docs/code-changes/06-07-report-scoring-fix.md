# 06-07 报告逐题评分与考试诊断隔离

## 1. 变更概述

本次变更为内部试用修复报告可信度问题：后端报告新增 `round_reviews` 契约，逐题保留题目、原答案、评分、命中点、缺失点、建议和追问复盘；`agent-verify` 增加报告评分一致性门禁；前端报告页优先展示后端报告复盘，并在考试模式隐藏内部 Agent/检索/事件诊断。

影响范围：

- 后端领域模型与报告节点。
- `cmd/agent-verify` 验证门禁。
- 候选人面试页、报告页和前端类型。
- Vite 构建产物 `internal/httpapi/web/dist`。

## 2. 变更文件

- `internal/domain/session.go`：新增报告逐题复盘结构和总分聚合 helper。
- `internal/nodes/report.go`：生成 `Report.RoundReviews`，总分改为从有效主问题 review 聚合。
- `internal/nodes/report_test.go`：覆盖逐题复盘、追问复盘、原答案和总分聚合。
- `internal/agentkit/verify/report_scoring.go`：新增报告评分一致性 verifier。
- `internal/agentkit/verify/report_scoring_test.go`：新增 verifier 通过/失败用例。
- `cmd/agent-verify/main.go`：接入 `ReportScoringVerifier`。
- `cmd/agent-verify/main_test.go`：补 CLI 失败夹具测试和完整 session fixture。
- `testdata/agent_verify/pass_session.json`：补 `report.round_reviews`。
- `testdata/agent_verify/fail_report_scoring_missing_review.json`：新增缺失逐题复盘失败夹具。
- `web/src/types.ts`：新增前端报告复盘类型。
- `web/src/main.tsx`：考试模式启动面试时不发送题库过滤条件，并向 JD 页传入当前模式。
- `web/src/candidatePages.tsx`：报告页渲染 `report.round_reviews`，fallback 渲染旧 rounds，考试隐藏诊断。
- `web/src/candidatePages.test.tsx`：新增报告复盘、fallback、考试诊断隐藏测试。
- `web/src/styles.css`：新增追问复盘子项样式。
- `internal/httpapi/web/dist/*`：前端 build 产物 hash 更新。

## 3. 函数级说明

### `internal/domain/session.go`

- `Report.RoundReviews []RoundReview`
  - 作用：报告 JSON 新增逐题复盘字段。
  - 输入/输出：作为 `Report` 的序列化字段输出到 API 和前端。
  - 兼容性：使用 `omitempty`，旧 session 没有该字段仍可解析。

- `RoundReview`
  - 作用：主问题复盘结构，包含 `round_id`、题号、题目、原答案、分数、评分证据、参考要点、是否计入总分和追问复盘。
  - 副作用：无。
  - 行为变化：`counts_toward_overall` 显式序列化，避免 false 与字段缺失混淆。

- `FollowUpReview`
  - 作用：追问复盘结构，包含追问题、原答案、分数和评分证据。
  - 副作用：无。

- `OverallScoreFromRoundReviews(reviews []RoundReview) int`
  - 作用：统一按 `CountsTowardOverall && Score != nil && *Score >= 0` 聚合有效主问题分数。
  - 输入：报告逐题复盘。
  - 输出：四舍五入后的整数总分；无有效分数时返回 0。
  - 错误处理：无错误返回；忽略未评分、负分或不计入总分的 review。

### `internal/nodes/report.go`

- `NewReportNode()` 返回的节点函数
  - 行为变化：构建报告时先生成 `roundReviews`，再用 `domain.OverallScoreFromRoundReviews` 计算 `overall_score`。
  - 数据流：`Session.Rounds` -> `roundReviews` -> `Report.RoundReviews` 和 `Report.OverallScore`。

- `roundReviews(rounds []domain.AnswerRound) []domain.RoundReview`
  - 作用：从已回答主问题构建逐题复盘。
  - 输入：session rounds。
  - 输出：按已回答主问题顺序编号的 review 列表。
  - 主要逻辑：跳过空答案；使用 `AnswerRound.FinalEvaluation()`；复制原答案、题目、参考要点、评分、命中点、缺失点和建议；追问由 `followUpReviews` 生成。
  - 副作用：无；slice 字段使用 copy，避免别名污染。

- `followUpReviews(followUps []domain.FollowUp) []domain.FollowUpReview`
  - 作用：从已回答追问构建子复盘。
  - 输入：round follow-ups。
  - 输出：只包含有答案的追问 review。
  - 错误处理：无；未评分追问仍可展示题目和原答案。

- `intPtr(v int) *int`
  - 作用：保留 0 分与未评分的区别。

### `internal/agentkit/verify/report_scoring.go`

- `ReportScoringVerifier.VerifyReportScoring(sess *domain.Session) []Failure`
  - 作用：检查报告逐题评分是否完整且和真实 evaluation 一致。
  - 输入：完整 session。
  - 输出：失败列表。
  - 主要逻辑：跳过 nil session/report；按 `round_id` 建立 review 索引；对每个已回答主问题检查 review、原答案、score、`counts_toward_overall`、评分证据；检查追问；最后用 `domain.OverallScoreFromRoundReviews` 校验总分。
  - 错误处理：通过 `Failure{Code, Message, Target}` 返回，不 panic。

- `verifyFollowUpReviews(round domain.AnswerRound, review domain.RoundReview) []Failure`
  - 作用：检查已回答追问是否进入主问题 review 的 `follow_ups`。
  - 输入：一个原始 round 和对应 report review。
  - 输出：追问缺失、缺答案、缺分数、分数不一致、证据缺失或错配的失败列表。
  - 限制：当前按追问文本匹配；如果同一主问题下追问文本重复，无法区分。

- `verifyScoringEvidence(...) []Failure`
  - 作用：校验 hit/missed/suggestion 是否缺失或与 evaluation 漂移。
  - 输入：源 evaluation 证据和 report review 证据。
  - 输出：缺失或 mismatch failure。
  - 行为：slice 使用严格顺序相等；当前 report 节点是原样复制 evaluation，因此严格匹配符合防漂移目标。

### `cmd/agent-verify/main.go`

- `run(opts options, stdout, stderr io.Writer) int`
  - 行为变化：在 `ReportCompletenessVerifier` 后追加 `ReportScoringVerifier`。
  - 数据流：读取 session JSON 后，报告完整性和评分一致性都进入同一 failure 汇总。

### `cmd/agent-verify/main_test.go`

- `TestRunFailsWithReportScoringMissingReviewFixture`
  - 作用：确保缺失 `report.round_reviews` 的已答 session 使 CLI 返回失败。

- `completeSession() *domain.Session`
  - 行为变化：测试 session fixture 补齐 `RoundReviews`，让新增 verifier 不误杀既有通过用例。

### `web/src/candidatePages.tsx`

- `JDPage(...)`
  - 行为变化：接收当前 `mode`；仅 `practice` 模式展示题库范围筛选控件。
  - 兼容性：模拟模式保留题库范围控制；考试模式不向候选人暴露题库选择入口。

- `InterviewPage(...)`
  - 行为变化：题目/对话优先展示；`AgentStatePanel` 和 `EventTimeline` 只在 `practice` 模式、且位于对话之后展示。
  - 兼容性：practice 保留训练诊断；exam 候选人侧隐藏内部状态。

- `ReportPage(...)`
  - 行为变化：报告顺序调整为总分、画像、逐题复盘、回答诊断、训练计划、摘要、practice-only 诊断。
  - 兼容性：exam 报告隐藏 `AgentStatePanel` 和 `RetrievalTracePanel`。

- `ReportRoundReviews({ session })`
  - 作用：报告页逐题复盘入口。
  - 数据流：优先读取 `session.report.round_reviews`；缺失时 fallback 到 `session.rounds`。

- `ReportRoundReviewCard({ review, fallbackNumber })`
  - 作用：渲染后端报告主问题 review。
  - 输出：题号、题目、原答案、分数、评分证据和追问子项。

- `ReportFollowUpReviewCard({ follow, index })`
  - 作用：渲染后端报告追问 review。
  - 输出：追问编号、追问题、原答案、分数和评分证据。

- `ReviewEvidence({ review })`
  - 作用：复用命中点、遗漏点、参考要点和建议渲染。
  - 输入：主问题 review 或追问 review。

- `RoundReview({ round })`
  - 行为变化：旧 session fallback 时也渲染 `round.follow_ups`，避免追问复盘丢失。

### `web/src/main.tsx`

- `startInterview()`
  - 行为变化：`practice` 模式继续发送 `question_bank_filter`；`exam` 模式发送 `undefined`，避免考试链路带入候选人侧题库选择。
  - 数据流：侧栏 `mode` 状态 -> `startInterview` payload -> `/api/interview/start`。

- `App()` 路由渲染
  - 行为变化：渲染 `JDPage` 时传入当前 `mode`。

### `web/src/types.ts`

- `Report.round_reviews?: ReportRoundReview[]`
  - 作用：前端接收后端报告复盘字段。

- `ReportRoundReview`
  - 作用：前端主问题复盘类型，对齐后端 JSON 字段。

- `ReportFollowUpReview`
  - 作用：前端追问复盘类型。

### `web/src/styles.css`

- `.follow-review-list`
  - 作用：追问子项列表布局。

- `.follow-review`
  - 作用：追问子项视觉层级。

## 4. 调用链

### 报告生成链路

用户完成面试 -> 后端 Graph 进入 report node -> `NewReportNode()` -> `roundReviews(Session.Rounds)` -> `followUpReviews(round.FollowUps)` -> `domain.OverallScoreFromRoundReviews(reviews)` -> 写入 `Session.Report.RoundReviews` 和 `Session.Report.OverallScore` -> HTTP session/report 响应返回前端。

### 验证链路

命令行执行 `go run ./cmd/agent-verify ...` -> `cmd/agent-verify.run` 读取 session fixture -> `ReportCompletenessVerifier.VerifyReport` -> `ReportScoringVerifier.VerifyReportScoring` -> `verifyFollowUpReviews` / `verifyScoringEvidence` -> 汇总 JSON failure 输出和进程退出码。

### 前端展示链路

用户进入 JD 页 -> `main.tsx` 将当前 `mode` 传入 `JDPage` -> `JDPage` 在 practice 模式展示题库范围，在 exam 模式隐藏题库范围。

用户开始面试 -> `main.tsx` 的 `startInterview` 构造 payload -> practice 模式携带 `question_bank_filter`，exam 模式不携带 -> `/api/interview/start`。

用户进入报告页 -> `main.tsx` 根据路由渲染 `ReportPage` -> `ReportRoundReviews` -> 优先 `ReportRoundReviewCard` / `ReportFollowUpReviewCard`；无 `round_reviews` 时进入 `RoundReview` fallback。

用户进入面试页 -> `main.tsx` 渲染 `InterviewPage` -> conversation 优先展示题目和回答 -> practice 模式才渲染 `AgentStatePanel` / `EventTimeline`。

## 5. 数据流

- 原始数据来源：`Session.Rounds[].Question`、`Answer`、`Evaluation`、`FollowUps`。
- 校验：report verifier 对已回答主问题和已回答追问逐项校验；分数和评分证据必须与 `FinalEvaluation()` / `FollowUp.Evaluation` 一致。
- 转换：report node 将 round/follow-up 转换为报告专用 review 结构。
- 存储：仍写入 session 的 `Report` 对象；没有新增数据库 schema。
- 返回：HTTP session/report JSON 多出兼容字段 `report.round_reviews`。
- 前端：优先渲染 `report.round_reviews`，旧数据 fallback 到 `session.rounds`。

## 6. 依赖与副作用

- 新增 Go import：`internal/agentkit/verify/report_scoring.go` 使用 `fmt`、`reflect`、`strings`。
- 无新增外部依赖。
- 无数据库 schema 变更。
- 无环境变量变更。
- `npm --prefix web run build` 更新 `internal/httpapi/web/dist` 静态资源 hash。
- 安全边界：exam 模式隐藏内部 Agent 状态、事件流和检索 trace，减少候选人侧暴露后端运行细节。

## 7. 测试

已执行：

```powershell
go test ./internal/nodes -count=1
go test ./internal/domain ./internal/nodes -count=1
go test ./internal/agentkit/verify ./cmd/agent-verify -count=1
go run ./cmd/agent-verify -session testdata/agent_verify/pass_session.json -tool-events testdata/agent_verify/pass_tool_events.json -memory-observations testdata/agent_verify/pass_memory_observations.json
go run ./cmd/agent-verify -session testdata/agent_verify/fail_report_scoring_missing_review.json
npm --prefix web run test -- --run src/candidatePages.test.tsx
npm --prefix web run test
npm --prefix web run build
go test ./... -count=1
go run ./cmd/internal-trial-smoke
openspec validate internal-trial-report-scoring-fix --strict
```

结果：

- Go 相关单测通过。
- pass fixture 输出 `pass: true`、`failure_count: 0`。
- fail fixture 按预期非零退出，包含 `report_round_review_missing`。
- 前端单文件测试 13 passed。
- 前端全量测试 37 passed。
- 前端 build 通过。
- `go test ./... -count=1` 通过。
- `go run ./cmd/internal-trial-smoke` 输出 `business_trial: feedback evidence verified` 等通过信息。
- `openspec validate internal-trial-report-scoring-fix --strict` 通过；首次沙箱内因 `EPERM lstat C:\Users\hchegu` 失败，提升权限重跑通过。

未完成：

- 浏览器可视检查未完成。尝试访问 `http://127.0.0.1:5174/` 和 `/report` 返回 502，未将其计为通过验证。

## 8. 风险

- `verifyScoringEvidence` 使用严格顺序相等；如果后续报告层对评分证据排序、去重或改写文案，会触发误报。当前 report node 是原样复制 evaluation，因此合理。
- 追问 review 当前按追问文本匹配；重复追问文本无法区分。
- 前端 fallback 会继续支持旧 `session.rounds`，但旧数据若本身没有追问 feedback，就只能展示题目和答案。
- 构建产物 hash 更新需要和源码同批提交，否则 Go 嵌入式前端会落后于源码。
