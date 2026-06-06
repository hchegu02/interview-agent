# 06-07 内部业务试用稳定版

## 变更概述

本次变更把内部业务试用稳定版从纯文档判断推进到本地可复跑 smoke 门禁。新增业务反馈 fixture 校验，`cmd/internal-trial-smoke` 默认读取该 fixture 并输出 `business_trial: feedback evidence verified`；如果 fixture 缺失、字段无效、评分越界或阻断项仍建议扩大试用，smoke 会失败。

影响范围是内部试用 smoke、AgentKit verify 原语、内部试用文档和测试数据。不涉及 HTTP API、数据库 schema、真实鉴权、生产租户、真实 MCP runtime 或外部网络调用。

## 变更文件

- `internal/agentkit/verify/business_trial.go`：新增业务试用反馈结构和校验器。
- `internal/agentkit/verify/business_trial_test.go`：新增业务反馈校验单测。
- `testdata/internal_trial/business_feedback_pass.json`：新增非敏感业务反馈通过 fixture。
- `cmd/internal-trial-smoke/main.go`：新增 `-business-feedback` 参数、业务反馈 fixture 加载和 smoke marker。
- `cmd/internal-trial-smoke/main_test.go`：新增 smoke marker、缺 fixture 和 flag fallback 测试。
- `docs/ai/internal-trial-launch-checklist.md`：把业务反馈证据纳入内部试用启动门禁。
- `docs/ai/internal-trial/business-trial-runbook.md`：新增扩大内部业务试用条件。
- `docs/ai/internal-trial/trial-go-no-go.md`：新增业务反馈证据的 Go/No-Go 和 pause 条件。

## 函数级说明

### `BusinessTrialFeedback`

位置：`internal/agentkit/verify/business_trial.go`

作用：表示一条最小业务试用反馈证据。输入来自 JSON fixture，字段包括试用角色、场景、固定脚本完成状态、三类评分、扩大建议、阻断标记和简短摘要。

行为变化：`HasBlocker` 使用 `*bool`，用于区分 JSON 缺失字段和显式 `false`。

### `BusinessTrialFeedbackVerifier.Verify`

位置：`internal/agentkit/verify/business_trial.go`

输入：`BusinessTrialFeedback`。

输出：`[]Failure`。没有失败时返回空切片。

主要逻辑：

- 校验 `trial_role` 和 `scenario` 非空。
- 校验 `completed_fixed_script` 为 `true`。
- 校验 `interview_flow_score`、`report_usefulness_score`、`project_polish_score` 在 1-5。
- 对 `expand_recommendation` 做 `TrimSpace` 和小写归一化，只允许 `yes`、`no`、`unsure`。
- 校验 `has_blocker` 必须存在。
- 如果 `has_blocker=true` 且 `expand_recommendation=yes`，返回冲突失败。

副作用：无。只做内存校验，不读写文件、不访问网络。

### `BusinessTrialFeedbackVerifier.VerifyFeedback`

位置：`internal/agentkit/verify/business_trial.go`

作用：兼容包装器，内部转调 `Verify`。保留它是为了不破坏可能使用 `VerifyFeedback` 命名的调用方；新的 smoke 路径使用 `Verify`。

### `businessTrialScoreInRange`

位置：`internal/agentkit/verify/business_trial.go`

作用：校验评分是否在 1-5。

### `registerFlags`

位置：`cmd/internal-trial-smoke/main.go`

作用：集中注册 `-session`、`-business-feedback`、`-real-github` 参数，并允许测试使用独立 `flag.FlagSet` 检查默认值。

行为变化：`-business-feedback` 默认值为空字符串，让 `businessFeedbackFixturePath` 保留默认路径和 `../../` fallback。

### `businessFeedbackFixturePath`

位置：`cmd/internal-trial-smoke/main.go`

输入：`smokeOptions`。

输出：业务反馈 fixture 路径。

主要逻辑：

- 如果用户显式传入 `BusinessFeedbackPath`，直接使用。
- 否则依次检查 `testdata/internal_trial/business_feedback_pass.json` 和 `../../testdata/internal_trial/business_feedback_pass.json`。
- 都不存在时返回默认路径，让后续加载函数产生明确错误。

### `loadBusinessFeedbackFixture`

位置：`cmd/internal-trial-smoke/main.go`

输入：fixture 路径。

输出：`verify.BusinessTrialFeedback` 或错误。

主要逻辑：读取本地 JSON 文件并反序列化。错误会被 `run` 聚合为 smoke failure。

### `run`

位置：`cmd/internal-trial-smoke/main.go`

行为变化：在既有 session/report、retrieval trace、graph、memory 和 project polish smoke 基础上，新增业务反馈 fixture 校验。成功时输出 `business_trial: feedback evidence verified`。加载或校验失败时返回非 0。

### 测试函数

位置：`internal/agentkit/verify/business_trial_test.go`

- `TestBusinessTrialFeedbackVerifierValidPass`：验证通过 fixture 可通过。
- `TestBusinessTrialFeedbackVerifierRejectsIncompleteScript`：脚本未完成失败。
- `TestBusinessTrialFeedbackVerifierRejectsScoreOutOfRange`：评分越界失败。
- `TestBusinessTrialFeedbackVerifierRejectsBlockerExpansionConflict`：阻断项仍建议扩大失败。
- `TestBusinessTrialFeedbackVerifierRejectsMissingRecommendation`：扩大建议缺失失败。
- `TestBusinessTrialFeedbackVerifierRejectsMissingHasBlocker`：`has_blocker` 缺失失败。

位置：`cmd/internal-trial-smoke/main_test.go`

- `TestRunPassesOfflineInternalTrialSmoke`：确认默认 smoke 通过并输出完整业务试用 marker。
- `TestRunFailsWhenBusinessFeedbackFixtureIsMissing`：确认业务反馈 fixture 缺失时 smoke 失败。
- `TestBusinessFeedbackFlagDefaultUsesFallback`：确认 `-business-feedback` 默认值为空，不绕过 fallback。

## 调用链

### 默认内部试用 smoke

```text
go run ./cmd/internal-trial-smoke
  -> main
  -> registerFlags
  -> run
  -> businessFeedbackFixturePath
  -> loadBusinessFeedbackFixture
  -> BusinessTrialFeedbackVerifier.Verify
  -> stdout: business_trial: feedback evidence verified
```

### 缺失业务反馈 fixture

```text
go run ./cmd/internal-trial-smoke -business-feedback <missing>
  -> run
  -> businessFeedbackFixturePath
  -> loadBusinessFeedbackFixture
  -> append failure: load business feedback fixture
  -> stderr + exit code 1
```

## 数据流

`testdata/internal_trial/business_feedback_pass.json` 保存非敏感业务反馈样例。`loadBusinessFeedbackFixture` 读取 JSON 到 `BusinessTrialFeedback`，`BusinessTrialFeedbackVerifier.Verify` 校验字段完整性和扩大试用结论一致性，`run` 根据结果决定 smoke 成功或失败。

该数据不会写入数据库，不通过 HTTP 返回，不进入长期记忆，也不会访问公网。

## 依赖与副作用

- 新增依赖：无第三方依赖，只使用标准库 `encoding/json`、`flag`、`os`、`filepath`。
- 外部 API：无。
- 数据库：无。
- 文件系统：只读 fixture。
- 环境变量：无新增。
- 安全影响：fixture 不包含 token、私有仓库、完整简历、完整回答或完整报告。

## 测试

已执行：

```powershell
go test ./internal/agentkit/verify -count=1
go test ./cmd/internal-trial-smoke -count=1
go run ./cmd/internal-trial-smoke
```

结果：均通过。`go run ./cmd/internal-trial-smoke` 输出包含：

```text
business_trial: feedback evidence verified
```

## 风险

- 该门禁只验证业务反馈证据结构和最低一致性，不能自动判断报告内容质量是否真实优秀。
- `business_trial` marker 只代表可扩大受控内部业务试用，不是生产上线批准。
- 后续如果收集真实业务反馈，应另开 change 设计存储、权限、脱敏、删除和审计策略。
