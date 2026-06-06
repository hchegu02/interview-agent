---
archived-with: 2026-06-07-internal-trial-rollout-loop
status: final
---
# Internal Trial Rollout Loop Implementation Plan

---
change: internal-trial-rollout-loop
design-doc: docs/superpowers/specs/2026-06-07-internal-trial-rollout-loop-design.md
base-ref: 48e5df3e501bf874c19c9ef98d2e44f7f192e75d
---

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a documented internal trial rollout loop that lets technical users validate and diagnose the system before business users run controlled product trials.

**Architecture:** This is a documentation-first rollout package. `docs/ai/internal-trial-launch-checklist.md` remains the single entry point and links to focused Runbooks/templates under `docs/ai/internal-trial/`. OpenSpec artifacts remain the canonical requirements source; the implementation only creates or updates Markdown rollout documents.

**Tech Stack:** Markdown documentation, OpenSpec validation, existing Go/Node verification commands where explicitly touched.

---

## File Structure

- Modify: `docs/ai/internal-trial-launch-checklist.md`
  - Responsibility: single entry point for internal trial execution and links to the rollout package.
- Create: `docs/ai/internal-trial/technical-trial-runbook.md`
  - Responsibility: technical team startup, validation, diagnosis, and rollback procedure.
- Create: `docs/ai/internal-trial/business-trial-runbook.md`
  - Responsibility: business/HR/interviewer trial script and non-production framing.
- Create: `docs/ai/internal-trial/trial-issue-template.md`
  - Responsibility: structured reproduction template for technical failures and business quality issues.
- Create: `docs/ai/internal-trial/trial-feedback-template.md`
  - Responsibility: lightweight product feedback scoring template.
- Create: `docs/ai/internal-trial/trial-go-no-go.md`
  - Responsibility: continue, pause, rollback, and expansion decision criteria.
- Modify: `openspec/changes/internal-trial-rollout-loop/tasks.md`
  - Responsibility: track task completion.

## Task 1: Link Rollout Package From Launch Checklist

**Files:**
- Modify: `docs/ai/internal-trial-launch-checklist.md`

- [ ] **Step 1: Add rollout package links**

  Add a short section after the current trial scope that links to:

  ```markdown
  ## 1.1 试用闭环文档

  - [技术试用 Runbook](internal-trial/technical-trial-runbook.md)
  - [业务试用 Runbook](internal-trial/business-trial-runbook.md)
  - [问题记录模板](internal-trial/trial-issue-template.md)
  - [产品反馈模板](internal-trial/trial-feedback-template.md)
  - [Go/No-Go 标准](internal-trial/trial-go-no-go.md)
  ```

- [ ] **Step 2: Verify links are relative and local**

  Run:

  ```powershell
  rg -n "internal-trial/" "docs/ai/internal-trial-launch-checklist.md"
  ```

  Expected: five links under `internal-trial/`.

- [ ] **Step 3: Commit**

  ```powershell
  git add -- "docs/ai/internal-trial-launch-checklist.md"
  git commit -m "docs: link internal trial rollout package"
  ```

## Task 2: Add Technical Trial Runbook

**Files:**
- Create: `docs/ai/internal-trial/technical-trial-runbook.md`

- [ ] **Step 1: Create the technical Runbook**

  Include sections:

  ```markdown
  # 技术试用 Runbook

  ## 1. 前提
  ## 2. 启动前验证
  ## 3. 默认 mock/offline 工具检查
  ## 4. Agent project_polish 与 tool_trace 检查
  ## 5. 长期记忆 owner 和观测检查
  ## 6. 失败记录要求
  ## 7. 回滚
  ```

  Required commands:

  ```powershell
  go test ./... -count=1
  npm --prefix web run test
  npm --prefix web run build
  go run ./cmd/agent-verify -session testdata/agent_verify/pass_session.json -tool-events testdata/agent_verify/pass_tool_events.json -memory-observations testdata/agent_verify/pass_memory_observations.json
  go run ./cmd/internal-trial-smoke
  openspec validate --all --strict
  ```

- [ ] **Step 2: Check non-production wording**

  Run:

  ```powershell
  rg -n "生产|JWT|OIDC|mock|tool_trace|memory|config_missing" "docs/ai/internal-trial/technical-trial-runbook.md"
  ```

  Expected: wording explicitly says this is not production authentication or external release.

- [ ] **Step 3: Commit**

  ```powershell
  git add -- "docs/ai/internal-trial/technical-trial-runbook.md"
  git commit -m "docs: add technical internal trial runbook"
  ```

## Task 3: Add Business Trial Runbook

**Files:**
- Create: `docs/ai/internal-trial/business-trial-runbook.md`

- [ ] **Step 1: Create the business Runbook**

  Include sections:

  ```markdown
  # 业务试用 Runbook

  ## 1. 适用对象
  ## 2. 非生产边界
  ## 3. 固定试用脚本
  ## 4. 面试流程检查点
  ## 5. 项目润色检查点
  ## 6. 反馈提交
  ## 7. 遇到阻塞时怎么记录
  ```

  Fixed script must cover:

  1. Start one interview.
  2. Answer the main question.
  3. Answer one follow-up when present.
  4. Review the report.
  5. Submit one `project_polish` request with a GitHub URL.
  6. Fill the feedback template.

- [ ] **Step 2: Verify business wording hides runtime internals**

  Run:

  ```powershell
  rg -n "MCP|sub-agent|daemon|Sandbox|PostgreSQL|Redis" "docs/ai/internal-trial/business-trial-runbook.md"
  ```

  Expected: no matches except explicit non-production boundary if included.

- [ ] **Step 3: Commit**

  ```powershell
  git add -- "docs/ai/internal-trial/business-trial-runbook.md"
  git commit -m "docs: add business internal trial runbook"
  ```

## Task 4: Add Issue And Feedback Templates

**Files:**
- Create: `docs/ai/internal-trial/trial-issue-template.md`
- Create: `docs/ai/internal-trial/trial-feedback-template.md`

- [ ] **Step 1: Create issue template**

  Required fields:

  ```markdown
  # 内部试用问题记录模板

  - 问题类型：
  - 阶段：技术试用 / 业务试用
  - session id 或命令：
  - 页面 / API / 命令入口：
  - 复现步骤：
  - 期望结果：
  - 实际结果：
  - tool_trace.status：
  - tool_trace.error_class：
  - memory observation 状态：
  - 是否阻断继续试用：
  - 附加说明：
  ```

  Add a warning that token, keys, full answers, full reports, and private config must not be recorded.

- [ ] **Step 2: Create feedback template**

  Required fields:

  ```markdown
  # 内部试用产品反馈模板

  - 试用者角色：
  - 使用场景：
  - 题目质量（1-5）：
  - 追问质量（1-5）：
  - 报告可信度（1-5）：
  - 项目润色质量（1-5）：
  - 整体可用性（1-5）：
  - 最有价值的部分：
  - 最大阻塞：
  - 是否建议进入下一轮试用：
  ```

- [ ] **Step 3: Verify sensitive data warning**

  Run:

  ```powershell
  rg -n "token|密钥|完整回答|完整报告|私有配置" "docs/ai/internal-trial/trial-issue-template.md" "docs/ai/internal-trial/trial-feedback-template.md"
  ```

  Expected: warning appears in issue template.

- [ ] **Step 4: Commit**

  ```powershell
  git add -- "docs/ai/internal-trial/trial-issue-template.md" "docs/ai/internal-trial/trial-feedback-template.md"
  git commit -m "docs: add internal trial feedback templates"
  ```

## Task 5: Add Go/No-Go Standard And Closeout Checks

**Files:**
- Create: `docs/ai/internal-trial/trial-go-no-go.md`
- Modify: `openspec/changes/internal-trial-rollout-loop/tasks.md`

- [ ] **Step 1: Create Go/No-Go standard**

  Include:

  ```markdown
  # 内部试用 Go/No-Go 标准

  ## 1. Enter Technical Trial
  ## 2. Enter Business Trial
  ## 3. Pause Rollout
  ## 4. Roll Back To Mock
  ## 5. Do Not Expand Scope
  ```

  Hard pause conditions:

  - Core verification fails.
  - `tool_trace` is missing or contradictory for tool-driven requests.
  - Real tool failure is described as success.
  - Identity boundary accepts dev fallback unexpectedly.
  - Critical failures cannot be reproduced.

- [ ] **Step 2: Mark OpenSpec tasks complete as corresponding documents are done**

  Update `openspec/changes/internal-trial-rollout-loop/tasks.md` by changing completed items from `- [ ]` to `- [x]`.

- [ ] **Step 3: Run validation**

  ```powershell
  openspec validate internal-trial-rollout-loop --strict
  $pattern = ("TO"+"DO|T"+"BD|<"+"!--|PLACE"+"HOLDER|待"+"定")
  rg -n $pattern "docs/ai/internal-trial-launch-checklist.md" "docs/ai/internal-trial" "openspec/changes/internal-trial-rollout-loop"
  ```

  Expected: OpenSpec passes; placeholder scan returns no unfinished placeholders.

- [ ] **Step 4: Commit**

  ```powershell
  git add -- "docs/ai/internal-trial/trial-go-no-go.md" "openspec/changes/internal-trial-rollout-loop/tasks.md"
  git commit -m "docs: define internal trial go no-go standard"
  ```
