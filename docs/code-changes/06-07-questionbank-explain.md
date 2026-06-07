# 06-07 题库查询计划诊断

## 1. 变更概述

本次新增一个只读诊断入口，用于对 PostgreSQL 题库列表查询输出 `EXPLAIN (ANALYZE, BUFFERS)`。目的不是立即重写分页或搜索 SQL，而是先拿到真实查询计划证据，判断 `OFFSET`、`ILIKE OR`、`unnest(tags)` 和现有索引在实际数据量下的表现。

影响范围限定在题库 PG 查询构建和新增 CLI；不修改 HTTP API、数据库 schema、题库分页语义、搜索语义或前端。

## 2. 变更文件

- `internal/questionbank/pg_store.go`
  - 抽出题库列表 SQL 构建逻辑。
  - 新增 EXPLAIN 查询构建和执行入口。
- `internal/questionbank/pg_store_test.go`
  - 覆盖 EXPLAIN 查询复用列表查询形状，以及非法 cursor 错误。
- `cmd/questionbank-explain/main.go`
  - 新增只读 CLI，读取配置中的 PostgreSQL DSN，输出题库列表查询计划。
- `cmd/questionbank-explain/main_test.go`
  - 覆盖 CLI 参数到 `questionbank.Filter` 的映射，以及缺少 PG DSN 时失败。

## 3. 函数级说明

### `PGStore.List`

位置：`internal/questionbank/pg_store.go`

作用：列出题库条目。行为保持不变，但 SQL 构建改为调用 `buildListQuery`。

输入：`context.Context` 和 `questionbank.Filter`。

输出：`ListResult`，包含当前页 items 和 offset cursor。

副作用：只读查询 PostgreSQL。

错误处理：保留原有 cursor 校验、查询错误和扫描错误包装。

主要逻辑变化：列表 SQL、参数、limit 和 offset 由 `buildListQuery` 统一返回，避免后续诊断 SQL 与真实列表 SQL 分叉。

### `PGStore.ExplainList`

位置：`internal/questionbank/pg_store.go`

作用：执行题库列表查询的 `EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)`，返回 PostgreSQL 查询计划文本行。

输入：`context.Context` 和 `questionbank.Filter`。

输出：`[]string`，每个元素是一行查询计划。

副作用：连接 PostgreSQL 执行只读 SELECT 的查询计划；`ANALYZE` 会实际执行该 SELECT，但不写业务数据。

错误处理：无 PG pool 时返回 nil；cursor、查询和扫描错误会返回带上下文的错误。

主要逻辑：调用 `BuildListExplainQuery` 得到 SQL 和参数，再扫描单列文本计划。

### `BuildListExplainQuery`

位置：`internal/questionbank/pg_store.go`

作用：构建题库列表查询对应的 EXPLAIN SQL 和参数。

输入：`questionbank.Filter`。

输出：SQL 字符串、参数列表、错误。

副作用：无。

错误处理：非法 cursor 直接返回错误。

主要逻辑：复用 `buildListQuery`，只在前面加 `EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)`。

### `buildListQuery`

位置：`internal/questionbank/pg_store.go`

作用：构建题库列表查询 SQL、参数、limit 和 offset。

输入：`questionbank.Filter`。

输出：SQL 字符串、参数列表、规范化 limit、offset、错误。

副作用：无。

错误处理：非法 cursor 返回错误。

主要逻辑：复用原有 `buildWhere`，保留 `ORDER BY skill_category, difficulty, id LIMIT ... OFFSET ...` 结构。

### `main`

位置：`cmd/questionbank-explain/main.go`

作用：解析 CLI 参数并退出。

输入：命令行参数。

输出：进程退出码。

副作用：读取命令行参数并调用 `run`。

错误处理：由 `run` 决定退出码。

### `run`

位置：`cmd/questionbank-explain/main.go`

作用：加载配置、连接 PostgreSQL、执行题库列表查询计划诊断并输出结果。

输入：`options`。

输出：退出码，`0` 表示成功，`2` 表示配置、连接或查询错误。

副作用：读取配置文件，建立 PostgreSQL 连接，向 stdout/stderr 写入文本。

错误处理：配置读取失败、缺少 `postgres_dsn`、连接失败和 EXPLAIN 失败都会输出 `ERROR:` 并返回 `2`。

### `buildFilter`

位置：`cmd/questionbank-explain/main.go`

作用：把 CLI 参数转换为 `questionbank.Filter`。

输入：`options`。

输出：`questionbank.Filter`。

副作用：无。

错误处理：无；cursor 合法性由 `questionbank` 层处理。

### `splitCSV`

位置：`cmd/questionbank-explain/main.go`

作用：解析逗号分隔 tags，去掉空白和空项。

输入：CSV 字符串。

输出：tag 列表。

副作用：无。

错误处理：无。

## 4. 调用链

### 题库管理页查询

调用链部分确认：

`GET /api/question-bank` -> HTTP handler -> `questionbank.Store.List` -> `PGStore.List` -> `buildListQuery` -> PostgreSQL `SELECT`

本次只把 `PGStore.List` 的 SQL 构建抽出，外部调用链和响应结构不变。

### 诊断 CLI

`go run ./cmd/questionbank-explain ...` -> `main` -> `run` -> `config.Load` -> `pgxpool.New` -> `questionbank.NewPGStore` -> `PGStore.ExplainList` -> `BuildListExplainQuery` -> `buildListQuery` -> PostgreSQL `EXPLAIN`

## 5. 数据流

CLI 参数来源于命令行，转换为 `questionbank.Filter`：

- `-query` -> `Filter.Query`
- `-skill` -> `Filter.SkillCategory`
- `-scenario` -> `Filter.Scenario`
- `-difficulty` -> `Filter.Difficulty`
- `-tags` -> `Filter.Tags`
- `-status` -> `Filter.Status`
- `-limit` -> `Filter.Limit`
- `-cursor` -> `Filter.Cursor`

`Filter` 进入 `buildListQuery` 后生成参数化 SQL，不拼接用户输入到 SQL 文本。`BuildListExplainQuery` 只添加 EXPLAIN 前缀，参数列表保持不变。

## 6. 依赖与副作用

- 新增 CLI 依赖 `github.com/jackc/pgx/v5/pgxpool`，项目已有该依赖。
- 读取 `config.Load` 支持的配置文件。
- 需要配置 `postgres_dsn` 才能运行。
- 不新增数据库表、索引或迁移。
- 不写业务数据；但 `EXPLAIN ANALYZE` 会实际执行 SELECT，因此在大数据量环境中会产生一次真实查询成本。
- 不输出 DSN、密钥、原始连接串或用户回答。

## 7. 测试

已执行：

```powershell
go test ./internal/questionbank -run BuildListExplainQuery -count=1
go test ./cmd/questionbank-explain -count=1
go test ./internal/questionbank -count=1
```

结果：全部通过。

## 8. 风险

- 兼容性：`PGStore.List` 外部行为不变；新增导出函数仅用于诊断。
- 性能：正常业务列表查询不增加额外查询；只有显式运行 CLI 才执行 EXPLAIN。
- 安全：CLI 只读，不打印 DSN；仍需避免在生产高峰直接运行深 offset 查询。
- 已知限制：当前只提供查询计划文本，不自动解析 Seq Scan、BitmapOr、trigram 命中等结论；后续应基于真实输出再决定是否改 SQL 或索引。
