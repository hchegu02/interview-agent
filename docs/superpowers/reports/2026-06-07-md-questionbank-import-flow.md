# 2026-06-07 MD 题库导入闭环验证

## 输入

- 文件：`D:\Documents\Mirrorfiles\Obsidian\CODEX\raw\project-experience\AI 模拟面试项目 Top100 高频追问回答.md`
- 后端：临时启动 `127.0.0.1:18080`
- Embedding：本地 BGE-M3 OpenAI-compatible endpoint，临时启动 `127.0.0.1:18000`
- PostgreSQL：`interview-agent-postgres-1`
- LLM：`INTERVIEW_LLM_MODE=mock`
- Spool：`D:\Documents\New project\interview-agent\tmp\md-import-flow\import-spool`

## 结果

最新成功 job：

```text
job_id=imp-03316aae96b3
chunks=18
total_items=18
valid_items=18
accepted_auto_approved=18
imported_items=18
```

PostgreSQL 精确校验：

```sql
SELECT q.embedding_status, q.embedding_model, count(*)
FROM question_bank_import_items i
JOIN question_bank q ON q.id = i.question_id
WHERE i.job_id = 'imp-03316aae96b3'
GROUP BY q.embedding_status, q.embedding_model;
```

结果：

```text
embedded,BAAI/bge-m3,18
```

API / 日志输出保留在：

```text
D:\Documents\New project\interview-agent\tmp\md-import-flow
```

关键文件：

- `upload.json`
- `job.latest.json`
- `review.json`
- `commit.json`
- `query-agent.json`
- `bge.stdout.log`
- `bge.stderr.log`
- `server.stdout.log`
- `server.stderr.log`

## 修复的后端 blocker

1. 文档生成题 ID 不可信。
   - mock LLM 对多个 chunk 反复返回 `generated-go-001`。
   - 原逻辑用 `jobID:itemID` 做 import item 主键，PG `ON CONFLICT(id)` 会覆盖前面的 chunk。
   - 修复：文档生成题统一由后端生成 `docq-*` 稳定 ID。

2. PG import item `errors` 字段 nil 写入。
   - `question_bank_import_items.errors` 是 `text[] NOT NULL DEFAULT '{}'`。
   - Go nil slice 作为参数会被 PG 当成 SQL NULL，不会触发表默认值。
   - 修复：PG store 写入前把 nil errors 规范为空数组。

3. 异步文档导入过早 ready。
   - 原逻辑每个 chunk staging 后都会把 job 标为 `ready`。
   - HTTP 轮询可能在后台只处理了部分 chunks 时提前审核/提交。
   - 修复：文档 chunk staging 期间保持 `generating`，`processDocument` 完成全部 chunks 后才置 `ready`。

## 验证命令

```powershell
go test ./internal/questionbank -count=1
go test ./... -count=1
openspec validate validate-md-questionbank-import-flow --strict
openspec validate --all --strict
```

结果：全部通过。

## 剩余风险

- 当前 mock LLM 每个 chunk 生成同一题干，验证的是导入链路，不代表题目质量已达生产标准。
- 查询 API 默认非 admin view 不返回 embedding 字段；embedding 状态用 PG join 精确确认。
- 临时验证写入了本地 PostgreSQL，不包含清库或删除操作。
