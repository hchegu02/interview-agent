# 2026-06-07 内部业务试用执行单

本文档用于执行本轮 Interview Agent 内部业务试用。它不是生产发布说明，不允许对外开放或承诺 SLA。

## 1. 试用版本

- 分支：`main`
- 远端：`origin/main`
- 版本提交：`58206cc chore: archive business trial stabilization`
- 试用范围：受控内部业务试用

## 2. 启动前门禁

本轮推送前已通过以下命令：

```powershell
go test ./... -count=1
npm --prefix web run test
npm --prefix web run build
go run ./cmd/agent-verify -session testdata/agent_verify/pass_session.json -tool-events testdata/agent_verify/pass_tool_events.json -memory-observations testdata/agent_verify/pass_memory_observations.json
go run ./cmd/internal-trial-smoke
openspec validate --all --strict
```

`go run ./cmd/internal-trial-smoke` 必须输出：

```text
business_trial: feedback evidence verified
```

## 3. 试用对象

本轮建议至少覆盖 3 类角色：

- HR：验证流程是否顺畅、报告是否便于流转。
- 面试官：验证题目、追问、评分和建议是否符合真实面试判断。
- 候选人视角使用者：验证输入、答题、等待、报告阅读是否清楚。

每类角色至少完成 1 次固定脚本。不要输入真实候选人敏感信息、私有仓库、token、密钥、完整简历或完整面试回答。

## 4. 固定脚本

每位试用者按 `docs/ai/internal-trial/business-trial-runbook.md` 完成：

1. 开始一次面试。
2. 回答主问题。
3. 如果出现追问，回答一次追问；如果没有追问，记录“本轮无追问”。
4. 查看报告。
5. 在项目润色入口提交一次包含公开 GitHub URL 的请求。
6. 填写产品反馈。
7. 如果遇到阻塞，额外填写问题记录。

本轮报告评分修复后的复测优先使用本地 JD 样例：

- `testdata/internal_trial/yunzhi_backend_jd.md`

该样例只保留岗位描述和岗位要求，不包含申请表字段、候选人个人信息或投递记录。复测时重点检查逐题评分、原答案、追问评分证据、模拟/考试展示差异。

## 5. 反馈收集

反馈使用 `docs/ai/internal-trial/trial-feedback-template.md`。

必须记录：

- 试用者角色
- 使用场景
- 题目质量评分
- 追问质量评分
- 报告可信度评分
- 项目润色质量评分
- 整体可用性评分
- 最大阻塞
- 是否建议进入下一轮试用

问题记录使用 `docs/ai/internal-trial/trial-issue-template.md`。

## 6. 分级规则

- P0：阻断固定脚本，必须暂停扩大试用并修复。
- P1：明显影响 HR 或面试官判断，下一轮必须修。
- P2：体验问题或内容质量瑕疵，可排期。
- Idea：新增想法，只记录，不进入本轮修复范围。

## 7. Go/No-Go

本轮结束后按 `docs/ai/internal-trial/trial-go-no-go.md` 判断。

允许继续扩大内部试用的最低条件：

- 核心门禁仍能复跑通过。
- 没有 P0。
- P1 有明确复现信息和修复计划。
- `tool_trace` 与用户可见结果一致。
- 身份边界没有接受意外 dev fallback。
- 业务反馈中不存在“有阻断项但仍建议扩大”的矛盾。

必须暂停的条件：

- 任一核心门禁失败。
- 真实工具失败被描述成成功。
- 关键失败不可复现。
- 业务反馈证据缺失或字段不完整。
- 试用范围开始引入生产用户、生产数据、生产 SLA 或外部承诺。

## 8. 下一步

试用完成后，不直接新增功能。先汇总反馈，按 P0/P1/P2/Idea 分级，再开下一个 Comet change 修复真实阻断问题。
