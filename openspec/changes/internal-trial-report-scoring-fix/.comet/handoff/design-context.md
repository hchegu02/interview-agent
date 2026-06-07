# Comet Design Handoff

- Change: internal-trial-report-scoring-fix
- Phase: design
- Mode: compact
- Context hash: 5c4cedf365c57c30d2ccbc8adee4b836983e6fa43e19ab5f446da5fb5662a963

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/internal-trial-report-scoring-fix/proposal.md

- Source: openspec/changes/internal-trial-report-scoring-fix/proposal.md
- Lines: 1-33
- SHA256: 977269286007b96111723b948e5b40e7838378b52e24b2e6fd734485910f528a

```md
## Why

内部业务试用已经能跑完整轮面试，但最终报告目前暴露出三个直接破坏可信度的问题：报告只展示部分题目、缺少候选人原始回答、逐题评分和总评之间缺少可验证的一致性。只要报告不能解释“每道题怎么评、依据是什么、总分怎么来”，HR 和面试官就无法把试用反馈当成有效产品证据。

## What Changes

- 报告必须覆盖本轮所有已回答题目，包括主问题和已回答追问。
- 报告和前端逐题复盘必须展示每题原始回答，不能只展示摘要或只展示评分。
- 每题必须有独立评分、命中点、缺失点和改进建议；未回答题不得生成高分或参与有效总分。
- 总分必须由有效逐题评分聚合，并能与逐题分数复核一致。
- 前端报告页应把逐题复盘作为核心内容展示，调试性后端状态不得压在题目和回答前面。
- 考试模式的候选人界面不应展示题库选择、Agent 状态、Graph/事件调试状态；模拟模式可保留训练辅助，但调试信息必须降级到诊断区域。
- 使用 `D:/Downloads/云智研发公司 - 校园招聘 JD.md` 作为内部试用真实 JD 样例输入，不把完整招聘申请表单改造纳入本 change。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `interview-session-runtime`：补充最终报告、逐题复盘、原始回答、逐题评分和模式化前端展示边界要求。
- `quality-gates`：补充报告一致性验证门禁，防止报告缺题、缺原答案、缺逐题评分或总分与逐题分不一致。

## Impact

- 后端领域模型：可能需要扩展 `domain.Report` 或增加报告明细结构，保持 JSON 兼容，新增字段使用 `omitempty`。
- 后端报告节点：`internal/nodes/report.go` 需要从 `Session.Rounds` 构建可追溯逐题报告明细，并修正聚合规则。
- HTTP 响应：`internal/httpapi/interview_response.go` 需要确保 completed session 的 rounds/report 能暴露原始回答和最终评分证据。
- 前端：`web/src/candidatePages.tsx`、`web/src/types.ts`、`web/src/reportView.ts` 需要调整报告页和模拟/考试展示边界。
- 验证：新增或扩展 Go/前端测试，必要时扩展 `cmd/agent-verify` 或内部试用 smoke fixture。
- 非目标：不做完整招聘申请表单系统，不接真实生产鉴权，不上线生产，不引入真实 LLM/Redis/Postgres 作为本 change 的必需条件。
```

## openspec/changes/internal-trial-report-scoring-fix/design.md

- Source: openspec/changes/internal-trial-report-scoring-fix/design.md
- Lines: 1-54
- SHA256: 37dcbc77d48833c43cedae32474f1a3d20581da733095fb7376228deaa4ee728

```md
## Context

当前后端 `Report` 只保存总分、技能拆分、转录分析、训练计划和摘要列表；前端报告页的“逐题评分”主要从 `session.rounds` 临时拼出来。这个结构有两个坏味道：报告本身没有声明逐题明细契约；UI 可以显示 rounds，但报告一致性门禁无法验证“报告覆盖了所有有效回答、原答案没有丢、总分来自逐题分”。

本 change 优先修业务试用可信度，不做完整表单改版。真实 JD 只作为试用样例输入，招聘申请信息采集和大布局改造后续另开 change。

## Approach

采用“报告明细成为后端报告契约，前端只展示契约”的方式。

1. 后端在 `domain.Report` 增加兼容字段，例如 `round_reviews,omitempty`。
2. `report` 节点从 `Session.Rounds` 构建逐题明细：
   - 主问题：题号、题目 ID、题目正文、题型、原始回答、最终评分、命中点、缺失点、建议、是否参与总分。
   - 追问：作为主问题下的子明细，包含追问正文、原始回答和独立评分；已回答追问必须显示。
3. 总分只基于有效主问题最终评分聚合；追问评分用于单题证据和工作记忆，不默认重复计入总分，除非现有工作记忆规则明确合成。报告中必须说明聚合口径。
4. HTTP completed session 返回时继续保留 `rounds`，并返回 `report.round_reviews`；旧前端仍可读 `rounds`，新前端优先读报告明细。
5. 前端报告页顶部展示总分和摘要，随后立即展示逐题复盘；Agent/Graph/检索 trace 等诊断信息移到靠后区域。
6. 面试页按 `session.mode` 做最小展示隔离：
   - `exam`：隐藏题库范围、Agent 状态、Graph/事件调试状态。
   - `practice`：保留训练辅助，但调试信息不放在题目前面。

## Data Flow

```text
Session.Rounds
  -> report node
  -> Report.RoundReviews
  -> HTTP Session.report
  -> web ReportPage
  -> 逐题复盘卡片
```

每个 `RoundReview` 的分数来自 `AnswerRound.FinalEvaluation()`，即优先使用 refined evaluation。原始回答来自 `AnswerRound.Answer` 和 `FollowUp.Answer`，不做摘要、不改写。

## Error Handling

- 已回答但缺评分：报告明细必须标记为未评分，不参与有效总分；验证门禁应能捕获。
- 未回答题：可显示为未回答，但不得参与总分，不得生成高分。
- 总分和逐题有效分不一致：报告一致性验证失败。
- 旧 Session 缺少新字段：HTTP 仍兼容；新报告字段只在新生成报告后出现。

## Testing

- Go：覆盖 report node 生成所有主问题和已回答追问明细。
- Go：覆盖原始回答保留、未回答不计分、总分由有效逐题分聚合。
- Go：覆盖 HTTP completed session 返回 report round reviews。
- 前端：覆盖报告页展示原答案、逐题评分、追问评分，以及 exam 模式隐藏调试状态。
- 门禁：扩展 agent-verify 或 smoke，让缺题、缺答案、缺评分理由或总分不一致时失败。

## Trade-offs

- 不把完整招聘申请表单塞进本 change。它会影响开始页信息架构、数据模型和更多 UI 流程，应该单独设计。
- 不把追问默认计入总分。现有 `overallScore` 按主问题 `FinalEvaluation()` 聚合，改成追问混合会改变评分语义；本 change 先把追问作为独立证据展示。
- 不强制真实 LLM/Embedding。内部试用可信度先靠结构正确性和可复跑 fixture 保证。
```

## openspec/changes/internal-trial-report-scoring-fix/tasks.md

- Source: openspec/changes/internal-trial-report-scoring-fix/tasks.md
- Lines: 1-39
- SHA256: f77788bf5d252f845f4d31a673725ff8075acc7eb1088f03c80214817582738e

```md
## 1. 后端报告契约

- [ ] 1.1 扩展报告领域模型，增加兼容的逐题复盘结构。
- [ ] 1.2 从 `Session.Rounds` 构建主问题和已回答追问明细，保留题目、原始回答、评分和评分证据。
- [ ] 1.3 明确总分聚合口径，未回答或未评分题不得计入有效总分。
- [ ] 1.4 保持旧 JSON 响应兼容，新字段使用 `omitempty`。

## 2. 报告一致性验证

- [ ] 2.1 增加报告一致性 verifier，检查缺题、缺原答案、缺逐题评分和总分不一致。
- [ ] 2.2 将 verifier 接入 `cmd/agent-verify` 或 `cmd/internal-trial-smoke` 的内部试用门禁。
- [ ] 2.3 增加通过和失败 fixture。

## 3. 前端报告页

- [ ] 3.1 更新 `web/src/types.ts` 的报告逐题复盘类型。
- [ ] 3.2 报告页优先展示 `report.round_reviews`，每题显示题目、原答案、单题评分、命中点、缺失点和建议。
- [ ] 3.3 已回答追问显示为题目明细下的子项。
- [ ] 3.4 将 Agent/Graph/检索 trace 等诊断信息移到报告后部或诊断区域。

## 4. 模拟 / 考试最小展示隔离

- [ ] 4.1 考试模式隐藏题库选择入口和 Agent/Graph/事件调试状态。
- [ ] 4.2 模拟模式保留训练辅助，但后端状态不得出现在题目前面。
- [ ] 4.3 前端测试覆盖 practice/exam 展示差异。

## 5. 真实 JD 内部试用样例

- [ ] 5.1 将 `D:/Downloads/云智研发公司 - 校园招聘 JD.md` 的岗位要求整理为内部试用样例或 fixture。
- [ ] 5.2 不导入完整申请表单字段；招聘信息采集改造留到后续 change。

## 6. 验证和文档

- [ ] 6.1 增加或更新 Go 测试。
- [ ] 6.2 运行 `go test ./... -count=1`。
- [ ] 6.3 运行 `npm --prefix web run test`。
- [ ] 6.4 运行 `npm --prefix web run build`。
- [ ] 6.5 运行相关 `agent-verify` / `internal-trial-smoke` 门禁。
- [ ] 6.6 更新 `docs/code-changes/MM-DD-*.md`。
```

## openspec/changes/internal-trial-report-scoring-fix/specs/interview-session-runtime/spec.md

- Source: openspec/changes/internal-trial-report-scoring-fix/specs/interview-session-runtime/spec.md
- Lines: 1-40
- SHA256: fd5a403cec062d661659c6cdc5f83319f3f97062f61c6d3a2b21de16bc8d9689

```md
## MODIFIED Requirements

### Requirement: Final report must expose traceable per-question scoring

Completed interview Sessions MUST expose a final report that can be traced back to every answered main question and every answered follow-up.

#### Scenario: Report includes every answered main question

- **WHEN** a completed Session contains answered main question rounds
- **THEN** the final report MUST include one review item for each answered main question
- **AND** each review item MUST include the question id, question text, original answer, final score, hit points, missed points, and suggestion when available

#### Scenario: Report includes answered follow-ups

- **WHEN** an answered round contains answered follow-ups
- **THEN** the final report MUST expose those follow-ups under the corresponding main question review
- **AND** each answered follow-up review MUST include the follow-up question text, original answer, score, and scoring evidence when available

#### Scenario: Unanswered items are not scored as successful answers

- **WHEN** a question or follow-up has no original answer
- **THEN** the final report MUST NOT assign it a successful score
- **AND** it MUST NOT count as an effective answered item in aggregate scoring

#### Scenario: Overall score matches effective per-question scores

- **WHEN** the final report has effective scored main question reviews
- **THEN** `overall_score` MUST be derived from those effective scores using the documented aggregation rule
- **AND** the aggregate MUST be reproducible from the exposed per-question review data

#### Scenario: Frontend exam mode hides internal diagnostic state

- **WHEN** a Session is in `exam` mode
- **THEN** the candidate-facing interview and report UI MUST NOT place question bank controls, Agent state, Graph state, or event debug timeline ahead of the current question or report content

#### Scenario: Frontend practice mode keeps diagnostics secondary

- **WHEN** a Session is in `practice` mode
- **THEN** training aids MAY be visible
- **BUT** backend diagnostic state SHOULD be presented only as secondary diagnostics, not before the active question or per-question report review
```

## openspec/changes/internal-trial-report-scoring-fix/specs/quality-gates/spec.md

- Source: openspec/changes/internal-trial-report-scoring-fix/specs/quality-gates/spec.md
- Lines: 1-28
- SHA256: eb23aea6c7a0fc4c52f11ccd0033e02b939dd900dbabf8e121ec731aceed02cd

```md
## MODIFIED Requirements

### Requirement: Verification gates must detect incomplete report scoring evidence

Local verification gates for internal trial readiness MUST detect final reports that are missing per-question scoring evidence.

#### Scenario: Missing answered question in report fails verification

- **WHEN** a verification fixture contains an answered main question round
- **AND** the final report omits that answered question from per-question review data
- **THEN** the verification gate MUST fail

#### Scenario: Missing original answer in report review fails verification

- **WHEN** a report review item represents an answered question or answered follow-up
- **AND** the review item does not expose the original answer
- **THEN** the verification gate MUST fail

#### Scenario: Missing per-question score evidence fails verification

- **WHEN** a report review item participates in scoring
- **AND** it lacks score, hit/missed point evidence, or suggestion where the source evaluation has those fields
- **THEN** the verification gate MUST fail

#### Scenario: Aggregate score mismatch fails verification

- **WHEN** `overall_score` cannot be reproduced from the effective per-question review scores
- **THEN** the verification gate MUST fail
```

