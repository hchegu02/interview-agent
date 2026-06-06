# AI Coding 开发规则

## 总原则

- 当前项目重点是 Go 后端实现和优化。
- 前端只做输入、状态展示和后端能力呈现。
- 修改前必须先读真实代码，不凭空设计。
- 新能力必须先明确接口、状态字段、错误处理和验证命令。
- 不重复维护架构文档，架构以 SDD 为准。

## 需求处理

复杂需求必须先沉淀为 OpenSpec change：

```text
openspec/changes/<change>/
  proposal.md
  design.md
  tasks.md
  specs/
```

必须明确：

- 背景和目标。
- 非目标。
- API 是否变化。
- Session JSON 是否变化。
- 是否影响 PG / Redis / 前端类型。
- 测试和回滚方式。

## 阶段收口

每个阶段必须先收口再进入下一阶段。推荐顺序：

```text
写 OpenSpec
  -> 写测试或验证点
  -> 实现
  -> 相关测试
  -> 全量测试
  -> OpenSpec strict 校验
  -> archive
  -> 主 spec strict 校验
  -> 精确暂存
  -> commit
  -> 必要时 push
```

不要在一个未提交的大改动上继续叠新功能。`main` 长期 ahead 时，应优先提交和 push，再开下一阶段。

## OpenSpec 规则

- `proposal.md` 写为什么做、做什么、不做什么。
- `design.md` 写接口、状态、数据流、失败语义和取舍。
- `tasks.md` 必须和真实完成状态一致，不能为了归档提前打勾。
- delta spec 的 requirement 必须和真实代码行为一致；如果代码会二次调整，例如动态难度再叠加 `GapStrategy`，spec 不能写成固定最终值。
- archive 后必须校验主 spec。

## Go 修改规则

- Handler 只负责参数解析、调用 service、返回响应。
- 业务流程放在 service、Graph、node 或领域层。
- 所有外部调用必须传 `context.Context`。
- LLM、RAG、parser、tool 调用要有超时、错误和降级。
- 新增 JSON 字段优先 `omitempty`，考虑旧 Session 兼容。
- Go 文件修改后运行 `gofmt`。

## Graph / Session 规则

- `Session` 是单次面试事实源。
- `WorkingMemory` 只保存当前 session 内策略状态。
- 后续 `UserMemory` 必须独立模块化，不塞进 `Session`。
- 当前运行时是业务 Agent Graph，不是 LangGraph，也不是 sub-agent runtime。
- 后续可吸收 LangGraph 的 interrupt、checkpoint、state update 思路，但保持 Go 自研实现。

## RAG / LLM 规则

- RAG 用于决定“问什么”，LLM 用于理解、评分、追问和总结。
- RAG 结果必须保留可解释 trace。
- rerank 失败要降级到 RRF 或 fallback，不直接中断面试。
- 不让 LLM 直接绕过后端 schema 和权限边界。

## 前端规则

- 前端类型必须和后端 JSON 对齐。
- 老 Session 没有新字段时，前端必须能容忍 `undefined`。
- 前端不实现 Agent Graph、RAG、LLM 或动态难度策略。
- 前端 build 会更新 `internal/httpapi/web/dist`，提交时要确认是否需要纳入。

## Git 规则

- 禁止默认 `git add .`。
- 提交前必须查看 `git diff --cached --name-status`。
- 不提交 `.vscode`、密钥、私有配置。
- 不默认提交 `docs/code-changes`、`docs/superpowers`、`docs/项目讲解`。
- `.vscode/` 属于本地 IDE 配置，默认放入 `.gitignore`。
- 提交前先看 `git status --short --branch`，确认没有误纳入未跟踪文件。
- push 前先确认本地提交数量和远端目标，不在大改未验证时 push。

## Sub-agent 规则

- sub-agent 用于提效，不代表项目运行时支持 sub-agent。
- 优先并发做代码审查、规格审查、下一阶段调研。
- 实现型 sub-agent 必须有不重叠的文件范围。
- sub-agent 结论必须由主 agent 复核，并通过真实测试或构建验证。
- 不能用 sub-agent 的“PASS”替代 `go test`、OpenSpec 校验或前端 build。

## 验证规则

按改动范围运行：

- 后端代码：`go test ./...`
- 前端代码：`npm --prefix web run test` 和 `npm --prefix web run build`
- RAG：`go run ./cmd/rag-eval ...`
- Agent 输出：`go run ./cmd/agent-verify -session testdata/agent_verify/pass_session.json`

未运行验证时，必须在最终说明中写明原因。
