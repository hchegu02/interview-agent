## 1. 身份与访问边界

- [x] 1.1 梳理 `internal/httpapi/user_memory.go` 当前 owner resolver / authorizer 调用链，确认生产身份注入点。
- [x] 1.2 增加或调整身份 resolver 装配，使业务 handler 只依赖当前用户结果，不直接解析认证细节。
- [x] 1.3 补充用户资源访问测试，覆盖本人访问、跨用户拒绝和缺身份拒绝。

## 2. 长期记忆写入可观测性

- [x] 2.1 梳理 `persistLongTermMemory` 成功、跳过、失败和 CAS 冲突重试路径。
- [x] 2.2 增加结构化观测结果或 recorder，记录 `user_id`、`session_id`、状态、错误类别、重试次数和耗时。
- [x] 2.3 补充测试，覆盖成功、跳过、非冲突失败和 CAS 冲突重试耗尽。
- [x] 2.4 确认观测信号不包含完整回答正文、完整报告正文、token 或私有配置。

## 3. 真实 MCP / Tool MVP

- [x] 3.1 梳理 `internal/agentkit` ToolRegistry、MCP mock client、hook 和权限测试。
- [x] 3.2 选择第一版低风险真实工具，并明确配置、超时、错误分类和 mock 回退策略。
- [x] 3.3 在 ToolRegistry 边界内实现真实工具适配，保留 deterministic mock 用例。
- [x] 3.4 补充工具调用测试，覆盖成功、权限拒绝、超时或配置缺失错误、before/after hook 成对。

## 4. Agent / Tool Trace 展示

- [x] 4.1 设计后端 `tool_trace` DTO，字段只包含工具名、权限、状态、错误类别、耗时和必要摘要。
- [x] 4.2 让 Agent Skill / AgentService 收集工具 trace，并通过 `/api/agent/message` 以 `omitempty` 字段返回。
- [ ] 4.3 前端补充 `tool_trace` TypeScript 类型和只读展示，缺字段时保持现有 Agent 页面行为。
- [ ] 4.4 补充 HTTP 和前端测试，覆盖包含 trace、不包含 trace 和工具失败 trace。

## 5. 验证门禁增强

- [ ] 5.1 扩展 `cmd/agent-verify` 或 fixture，验证真实工具事件 before/after 成对、权限、状态和错误类别。
- [ ] 5.2 增加长期记忆观测相关测试或 fixture，确认失败不阻断面试完成。
- [ ] 5.3 更新本地验证说明，明确需要运行 Go 测试、前端测试/build、agent-verify tool events。

## 6. 文档和收口

- [ ] 6.1 更新 `docs/SDD-Backend.md`，同步真实身份边界、长期记忆观测、真实工具 MVP、tool trace 和验证门禁。
- [ ] 6.2 更新 `docs/SDD-Frontend.md`，同步前端只读展示 `tool_trace` 且不直接调用 MCP 服务。
- [ ] 6.3 若本 change 修改代码，按项目规则新增或更新 `docs/code-changes/MM-DD-简短变更名.md`。
- [ ] 6.4 运行最小必要验证：`go test ./... -count=1`、`npm --prefix web run test`、`npm --prefix web run build`、`go run ./cmd/agent-verify -session testdata/agent_verify/pass_session.json -tool-events testdata/agent_verify/pass_tool_events.json`。
- [ ] 6.5 运行 `openspec validate production-agent-boundaries --strict` 并确认通过。
