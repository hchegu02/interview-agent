## 1. Agent Router

- [x] 1.1 新增 `internal/agent` 请求、响应和路由类型。
- [x] 1.2 实现规则 Router。
- [x] 1.3 实现 AgentService。

## 2. Skills

- [x] 2.1 新增 `internal/skills` 接口和 Registry。
- [x] 2.2 实现 `quiz`、`explain`、`project_polish` 三个规则 skill。
- [x] 2.3 处理未知 skill 和空输入。

## 3. HTTP API

- [x] 3.1 Server 增加 AgentService 注入。
- [x] 3.2 新增 `POST /api/agent/message` handler。
- [x] 3.3 Router 注册新接口。

## 4. 测试

- [x] 4.1 覆盖 Router intent。
- [x] 4.2 覆盖 Skill Registry。
- [x] 4.3 覆盖 AgentService。
- [x] 4.4 覆盖 HTTP handler。

## 5. 文档与验证

- [x] 5.1 更新 `docs/SDD-Backend.md`。
- [x] 5.2 更新 `docs/code-changes/06-06-agent-router-skills.md`。
- [x] 5.3 运行 Go 测试和 OpenSpec strict 校验。
