## Design

### Router

第一版使用规则 Router：

- 包含“出题/测验/quiz/练习”等关键词 → `skill.quiz`
- 包含“解释/讲解/原理/为什么”等关键词 → `skill.explain`
- 包含“项目/简历/亮点/润色”等关键词 → `skill.project_polish`
- 包含“面试/开始/模拟”等关键词 → `interview.start`
- 其它 → `chat`

Router 输出：

```go
type RouteResult struct {
    Intent     string
    Skill      string
    Confidence float64
    Reason     string
}
```

### Skill Registry

`internal/skills` 提供：

```go
type Skill interface {
    Name() string
    Run(ctx context.Context, input SkillInput) (SkillResult, error)
}
```

Registry 根据 skill name 查找实现。第一版 Skill 都是规则实现，不调用 LLM，不访问数据库。

### AgentService

AgentService 接收用户 message 和可选 context，调用 Router，再按 `Skill` 执行。对于 `interview.start` 和 `chat`，第一版返回结构化提示，不自动创建面试 session，避免绕过现有 `/api/interview/start` 的 Graph 和存储流程。

### HTTP API

新增：

```text
POST /api/agent/message
```

请求：

```json
{
  "user_id": "u1",
  "message": "帮我讲一下 Redis 缓存击穿",
  "context": {"skill": "redis"}
}
```

响应：

```json
{
  "intent": "skill.explain",
  "skill": "explain",
  "confidence": 0.86,
  "reason": "matched explain keywords",
  "result": {
    "title": "知识讲解",
    "content": "...",
    "actions": []
  }
}
```

### Boundaries

- 不实现 LLM Router。
- 不实现运行时 sub-agent。
- 不改现有 interview API。
- 不把 skill 结果写数据库。
- 不把用户消息作为工具调用权限来源。

## Verification

- Router 单测覆盖关键 intent。
- Skill registry 单测覆盖查找和缺失。
- HTTP handler 测试覆盖成功、空消息、服务未配置。
- `go test ./... -count=1`
- `openspec validate add-agent-router-skills --strict`
