## Context

现有链路：

```text
POST /api/agent/message
  -> agent.Service.HandleMessage
  -> RuleRouter
  -> skills.Registry.Run("project_polish")
  -> projectPolishSkill.Run
```

阶段 4.1 已有：

```text
agentkit.ToolRegistry
  -> RegisterDefaultMCPTools
  -> github.project_analyze
  -> MockMCPClient
```

本阶段只把这两段链路接起来。

## Goals / Non-Goals

**Goals:**

- 给 `skills.Registry` 增加可选 tool registry 依赖。
- `project_polish` 能从 `SkillInput.Context["github_url"]` 或消息文本中提取 GitHub URL。
- 有 URL 且工具可用时调用 `github.project_analyze`。
- 工具失败时降级回旧模板，不中断用户请求。
- 服务启动时注入 mock tool registry。

**Non-Goals:**

- 不做真实网络调用。
- 不新增 HTTP 字段。
- 不改 router intent 规则。
- 不让其他 skill 调工具。

## Decisions

### 1. ToolRegistry 注入到 skills.Registry

`projectPolishSkill` 属于 skill 层，不应该直接在 HTTP 层或 agent service 内拼工具调用。把可选 `ToolRegistry` 注入 `skills.Registry`，可以保持 skill 自己决定是否用工具。

### 2. URL 识别只支持 GitHub

第一版只识别 `github.com/.../...`。非 GitHub URL 不调用工具，避免 `web.fetch` 也被不受控地纳入 project polish。

### 3. 工具失败降级

工具只是增强能力，不应破坏旧的项目亮点提炼。调用失败时返回旧模板，并可在内容中不暴露内部错误。

### 4. HTTP 响应结构不变

继续返回 `skills.SkillResult`，只改变 `Content` 文案和 `Actions` 内容，不新增 JSON 字段。

## Data Flow

```text
AgentMessage.Context / Message
  -> SkillInput
  -> projectPolishSkill.extractGitHubURL
  -> ToolRegistry.Call(github.project_analyze, PermissionReadOnly)
  -> ToolResult.Output
  -> project polish content
```

## Error Handling

- 无 GitHub URL：使用旧模板。
- ToolRegistry 未配置：使用旧模板。
- 工具不存在、权限拒绝、timeout 或 mock client 错误：使用旧模板。

## Tests

- 无工具时 `project_polish` 保持旧模板行为。
- 有 GitHub URL 和 mock tool registry 时，内容包含 mock 项目摘要或亮点。
- 工具失败时降级，不返回错误。
- `agent.NewDefaultService` 或 server 装配能创建带 mock tools 的默认服务。

## Risks / Trade-offs

- [Risk] 用户误以为已经真实分析 GitHub。Mitigation：输出和 SDD 明确 mock / local analysis foundation。
- [Risk] 正则误识别 URL。Mitigation：只处理 `github.com/owner/repo` 形式。
- [Risk] skill 层依赖 agentkit。Mitigation：依赖方向是业务 skill 使用工具边界，仍在后端内部。
