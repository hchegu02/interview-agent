## Why

当前后端已经开始接入真实 PostgreSQL、pgvector 和内部试用 RAG 题库，但数据库相关问题缺少足够的运行证据：

- Postgres pool 配置需要在所有使用项目配置的后端入口中统一生效，不能只有 HTTP server 生效。
- 题库列表搜索存在 `OFFSET` 深翻页、`ILIKE OR` 和 `unnest(tags)` 查询计划风险，需要先通过真实 `EXPLAIN` 观察，而不是盲改 SQL。
- PGVector 检索已有 vector/tag/text 三路候选和 Go 端 fusion，但离线评估之前无法逐 case 看到三路候选贡献。

这些问题会直接影响内部团队试用时的稳定性、性能诊断和 RAG 质量判断。

## What Changes

- 将 Postgres pool config 构建下沉到 `internal/config`，供 server 和后端 CLI 复用。
- 对 Postgres pool 参数增加配置校验，非法值在启动或 CLI 加载配置时提前失败。
- 新增题库列表查询 EXPLAIN 诊断 CLI，用真实列表查询形状输出 `EXPLAIN (ANALYZE, BUFFERS)`。
- 让 PGVector retriever 暴露可选 `Search` 诊断结果，记录 vector/tag/text/fusion 阶段候选。
- 增强 RAG eval per-case 输出，写入 `stage_candidates`，用于分析每个 golden case 的候选来源。

## Scope

包含：

- 后端配置、数据库连接池、题库 PG store、PGVector retriever、RAG eval CLI。
- 对应 Go 测试和代码变更文档。

不包含：

- 前端布局或候选人工作台调整。
- 数据库 schema 迁移。
- 题库搜索 SQL 重写或 keyset pagination。
- Session JSON 拆表。
- events 分区、TTL 或审计表治理。
- 真实生产数据库运行结果提交。

## Impact

- 正常 HTTP API 响应和 Session JSON 结构不变。
- `cmd/server`、`cmd/rag-eval`、`cmd/demo`、`cmd/questionbank-explain` 在配置了 Postgres DSN 时统一使用项目 pool 设置。
- `cmd/rag-eval` 的 `summary.json` 增加可选字段 `stage_candidates`。
- 新增 `cmd/questionbank-explain` 只读诊断命令。
