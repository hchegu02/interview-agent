## Why

当前系统核心入口是固定面试 Graph：开始面试、提交回答、生成报告。后续要扩展为“面试训练 Agent”，需要在正式面试之外提供结构化消息入口，把用户请求路由到专项能力，例如知识讲解、专项测验、项目亮点润色。

第一版不使用 LLM intent classifier，先做规则 Router + Skill Registry。这样稳定、低成本、可测试，也不会把 LLM Router 直接放到核心控制面。

## What Changes

- 新增 `internal/agent`：消息请求、意图结果、规则 Router、AgentService。
- 新增 `internal/skills`：Skill 接口、Registry、规则实现的 `quiz`、`explain`、`project_polish`。
- 新增 HTTP 接口 `POST /api/agent/message`。
- 接口返回结构化 `intent`、`skill`、`confidence`、`reason` 和 `result`。
- 更新 SDD，明确这是规则路由第一版，不是 runtime sub-agent，也不是 LLM router。

## Capabilities

### New Capabilities

- `agent-router-skills`: 系统支持通过统一 Agent 消息入口，把用户请求路由到可复用 Skill。

## Impact

- `internal/agent`: 新增规则路由与服务。
- `internal/skills`: 新增 skill 注册和执行。
- `internal/httpapi`: 新增 handler、server 注入和路由。
- 测试：覆盖 router、skill registry、HTTP 接口。
- 不改数据库 schema。
- 不改现有 interview API。
