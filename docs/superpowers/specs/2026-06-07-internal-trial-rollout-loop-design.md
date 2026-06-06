---
comet_change: internal-trial-rollout-loop
role: technical-design
canonical_spec: openspec
---

# Internal Trial Rollout Loop Design

## Context

`internal-trial-readiness` 已经把内部试用需要的工程边界补齐：内部试用配置、可信 owner header、默认 mock/offline 工具、显式真实 GitHub 工具、长期记忆观测、顶层 `tool_trace` 和离线 `cmd/internal-trial-smoke`。

当前问题不是缺一个新的运行时能力，而是缺一个可执行的试用闭环。技术团队需要知道怎么启动、验证、诊断和回滚；业务、HR 或面试官需要按固定脚本试用，并用统一维度反馈题目、追问、报告和项目润色质量。

本设计遵循 OpenSpec change `internal-trial-rollout-loop`，不重定义需求，只说明实现方式。

## Technical Approach

采用文档化试用包，不做 dashboard、不新增 collector、不改数据库 schema、不改 HTTP API。

入口仍是 `docs/ai/internal-trial-launch-checklist.md`。该入口链接 5 个具体交付件：

- `docs/ai/internal-trial/technical-trial-runbook.md`
- `docs/ai/internal-trial/business-trial-runbook.md`
- `docs/ai/internal-trial/trial-issue-template.md`
- `docs/ai/internal-trial/trial-feedback-template.md`
- `docs/ai/internal-trial/trial-go-no-go.md`

这些文档共同形成闭环：

```text
技术试用
  -> 启动/验证/smoke
  -> trace 与 memory 诊断
  -> Go/No-Go
  -> 业务试用
  -> 业务反馈与问题记录
  -> 下一轮修复或暂停扩大范围
```

事实源保持在现有系统边界内：

- Session id 用于定位单次面试。
- `tool_trace.status` / `tool_trace.error_class` 用于定位工具状态。
- memory observation 状态用于定位长期记忆写入结果。
- smoke 和 verify 命令输出用于判断技术试用是否可继续。

## Components

### Launch Checklist

`internal-trial-launch-checklist.md` 是唯一入口。它不复制所有细节，只链接各 Runbook 和模板，并列出最短启动前门禁。

### Technical Trial Runbook

面向开发或技术试用者。内容包括：

- 环境和配置前提。
- 必跑验证命令。
- 默认 mock/offline 工具路径确认。
- `project_polish` + GitHub URL 的 `tool_trace` 检查。
- 长期记忆读取和 owner 边界检查。
- 失败时如何记录 session、命令、错误类别和复现步骤。
- 何时回滚 `github_tool_mode` 到 `mock`。

### Business Trial Runbook

面向业务、HR 或面试官。内容不暴露内部 runtime 细节，只要求按固定脚本完成：

- 开始一次完整面试。
- 提交至少一轮主问题回答和追问回答。
- 查看报告。
- 使用包含 GitHub 仓库 URL 的项目润色请求。
- 填写产品反馈模板。

该 Runbook 必须明确当前是内部试用，不是生产发布。

### Issue Template

问题记录模板必须把复现信息结构化。必填字段包括：

- 问题类型。
- session id 或命令。
- 页面/API/命令入口。
- 复现步骤。
- 期望结果。
- 实际结果。
- `tool_trace.status` 和 `tool_trace.error_class`。
- memory observation 状态。
- 是否阻断继续试用。

模板不得要求记录 token、密钥、完整回答正文、完整报告正文或私有配置。

### Feedback Template

反馈模板只做轻量评分，不做复杂调研系统。评分维度：

- 题目质量。
- 追问质量。
- 报告可信度。
- 项目润色质量。
- 整体可用性。

每项保留短文本说明，避免只有数字没有上下文。

### Go/No-Go Standard

Go/No-Go 文档定义四类结论：

- Continue technical trial。
- Enter business trial。
- Pause rollout。
- Roll back to mock / do not expand scope。

真实 GitHub 工具失败、身份边界混乱、核心验证失败、关键问题不可复现，都必须阻止扩大试用范围。

## Data Flow

```text
试用操作
  -> 页面/API/命令
  -> session/tool_trace/memory observation
  -> issue template 或 feedback template
  -> Go/No-Go 判断
  -> 下一轮修复、暂停或扩大试用
```

本 change 不新增持久化数据流。所有记录先以 Markdown 模板或外部团队表格承载，避免在没有真实反馈前固化数据库模型。

## Error Handling

- 技术验证失败：记录命令、退出状态、关键输出和复现环境，不进入业务试用。
- `tool_trace` 缺失或状态不稳定：记录为技术失败，不让业务试用者判断工具状态。
- 真实工具失败：优先回滚到 `github_tool_mode=mock`，再判断是否继续默认试用。
- 长期记忆失败：确认面试完成不被阻断，并记录 memory observation 状态。
- 身份边界异常：暂停扩大试用范围，确认可信 owner header 和 dev fallback 配置。

## Testing Strategy

文档和规格验证为主：

```powershell
openspec validate internal-trial-rollout-loop --strict
```

实现阶段还应检查：

- `docs/ai/internal-trial-launch-checklist.md` 链接到所有新增 Runbook 和模板。
- 新增 Markdown 文件没有未完成占位文本。
- 文档没有要求提交 token、密钥或私有配置。
- 如果只改文档，不需要运行 Go 或前端全量测试。
- 如果触碰配置或脚本，运行对应最小测试。

## Risks And Trade-offs

- 人工模板可能漏填字段。缓解方式：把复现步骤、session id、期望/实际、trace 状态设为必填。
- 业务团队可能误解为生产发布。缓解方式：每个入口都声明内部试用、mock/offline 和非生产边界。
- 文档过多可能分散。缓解方式：只保留一个入口，细节由入口链接到具体文档。
- 第一轮反馈量小，不适合复杂指标。缓解方式：评分模板保持轻量，平台化等第二轮再设计。

## Spec Patches

当前 OpenSpec delta spec 已覆盖分阶段试用、可复现问题记录、轻量产品反馈和 Go/No-Go 标准，不需要额外 spec patch。
