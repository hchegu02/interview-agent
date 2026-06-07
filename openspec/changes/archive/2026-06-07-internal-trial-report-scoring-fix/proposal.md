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
