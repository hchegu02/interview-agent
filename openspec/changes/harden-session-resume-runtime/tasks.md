## 1. OpenSpec And Design

- [x] 1.1 创建 `harden-session-resume-runtime` proposal/design/tasks。
- [x] 1.2 增加或更新 delta spec，覆盖结构化恢复、mutation lease、SSE runtime state、错误 trace。
- [x] 1.3 运行 `openspec validate harden-session-resume-runtime --strict`。

## 2. Suspension-First Answer Recovery

- [x] 2.1 调整 `fillPendingAnswer`：优先读取 `Session.Suspension.Awaiting/Node`。
- [x] 2.2 保留旧 Session `CurrentNode` 回退。
- [x] 2.3 增加测试：`Suspension.Node != CurrentNode` 时按 `Suspension.Node` 写入答案。
- [x] 2.4 增加测试：`Suspension.Awaiting != answer` 时拒绝普通答案提交。

## 3. Mutation Lease Scope

- [x] 3.1 调整 `Start` 正常暂停保存后释放 lease。
- [x] 3.2 调整 `Answer` 正常暂停保存后释放 lease。
- [x] 3.3 保持 completed 和 failure 路径释放 lease。
- [x] 3.4 增加测试：暂停保存成功后会释放 mutation lease。

## 4. SSE Runtime State

- [x] 4.1 在 `InterviewEvent` 增加 `suspension,omitempty`、`trace_id,omitempty`、`replay_gap,omitempty`。
- [x] 4.2 在事件构造时 clone suspension，避免外部修改 Session。
- [x] 4.3 在 SSE snapshot / session updated / graph node event 中带 trace id。
- [x] 4.4 增加测试：SSE snapshot 或事件包含 suspension，不泄露可变引用。

## 5. Structured Errors And Trace

- [x] 5.1 增加 trace id helper，统一从 request context 获取。
- [x] 5.2 `writeInterviewError` 保留 `error` 字段，同时增加稳定 `code` 和 `trace_id`。
- [x] 5.3 区分 `session_not_found`、`invalid_state`、`lease_conflict`、`invalid_config`。
- [x] 5.4 增加 HTTP 错误响应测试。

## 6. Docs And Verification

- [x] 6.1 更新 `docs/SDD-Backend.md`，说明本阶段完成的运行时可靠性边界。
- [x] 6.2 创建 `docs/code-changes/MM-DD-session-resume-runtime.md`。
- [x] 6.3 运行 `go test ./internal/httpapi -count=1`。
- [x] 6.4 运行 `go test ./... -count=1`。
- [x] 6.5 运行 `openspec validate harden-session-resume-runtime --strict`。
