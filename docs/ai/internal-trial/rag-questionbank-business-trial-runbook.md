# RAG 题库业务试用 Runbook

本文档用于 Go 后端岗位题库的内部业务试用。它不是生产发布手册。

## 1. 试用范围

- 从 Go 后端相关原文、面经、JD 或内部技术材料构建题目草稿。
- 使用 Agent 生成和补齐题目字段。
- 查看 `auto_approved`、`needs_human_review`、`rejected` 等 Agent 质量建议。
- 只提交通过发布策略的题目。
- 用 Go 后端 golden queries 验证 RAG 召回。
- 在内部模拟面试中检查题目、追问、报告和 trace 是否可解释。

## 2. 不在本轮试用范围

- 不允许任意公网爬取。
- 不允许 skill 或 MCP 直接写正式题库。
- 不允许 `rejected` 题目进入正式题库。
- 不默认启用 HyDE live ranking；HyDE 第一版以 shadow 诊断为主。

## 3. 操作流程

1. 准备 Go 后端来源材料，例如 runtime、channel、context、Redis、PostgreSQL、MySQL、缓存一致性、P99 排查等主题。
2. 通过题库导入流程导入本地题库文件或源文档。
3. 检查暂存项的来源信息、字段来源和 Agent review 状态。
4. 对 `needs_human_review` 项做人工确认；`rejected` 项不得提交。
5. 执行 commit，让通过策略的题写入正式题库并生成 embedding。
6. 运行题库 lint 和 RAG eval。
7. 使用 Go 后端题库过滤条件启动内部模拟面试。
8. 记录题目是否贴合岗位、难度是否合理、trace 是否解释清楚 Query Rewriting 和 HyDE shadow 状态。

## 4. 必跑门禁

```powershell
go test ./internal/questionbank ./internal/nodes ./internal/retriever -count=1
go run ./cmd/questionbank-lint -seed seeds/question_bank.json -min-expected-points 3 -min-scenario-ratio 0.8
go run ./cmd/rag-eval -cases testdata/rag/golden_queries_go_backend.jsonl -config config/config.yaml.example -out tmp/eval/rag-go-backend
openspec validate rag-questionbank-business-trial --strict
```

## 5. Go/No-Go

可以继续内部试用的条件：

- 生成题目有来源快照或来源引用。
- Agent review 状态可见，且 `rejected` 项无法提交。
- Query Rewriting 失败时回退原 query。
- HyDE shadow 失败时只记录 fallback，不影响候选题。
- Go 后端 RAG eval 不出现关键主题整体漏召回。

必须暂停扩大的条件：

- 题目来源不可追溯。
- rejected 题进入正式题库。
- RAG trace 无法解释检索 query 或 fallback。
- Go 后端 golden queries 大面积 miss。
