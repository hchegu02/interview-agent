# Database Migrations

## 文件命名约定

`NNN_description.up.sql` 应用变更；`NNN_description.down.sql` 回滚。
版本号严格递增，**永远不修改已上线的 migration**，新需求加新文件。

## 当前 migrations

| 文件 | 作用 |
|---|---|
| `001_init.up.sql` | 创建 sessions / question_bank / idempotency_keys / events 表 + pgvector + HNSW 索引 |
| `002_question_bank_expected_points.up.sql` | 给 question_bank 增加 expected_points，用于模拟模式要点反馈 |
| `003_question_bank_metadata.up.sql` | 给题库增加场景、角色标签、rubric、参考答案、追问提示、locale、status 和 updated_at |
| `004_question_bank_imports.up.sql` | 创建题库导入 staging 表：jobs / chunks / items |
| `005_question_bank_embedding_status.up.sql` | 增加 embedding_status / embedding_model / embedded_at / embedding_error，RAG 只检索 embedded 题 |
| `006_question_bank_import_lease.up.sql` | 给导入 job 增加 owner_id / lease_until，支持异步任务恢复 |
| `007_question_bank_import_review_status.up.sql` | 给导入 item 增加人工 accept / reject 状态 |
| `008_question_bank_import_field_provenance.up.sql` | 给导入 item 增加 field_provenance，追踪字段来源 |
| `009_question_bank_content_trgm.up.sql` | 给题库内容增加 trigram GIN 索引，提升关键词检索 |
| `010_session_row_version.up.sql` | 给 sessions 增加 row_version，支持 PG 乐观锁写入 |
| `011_user_memory.up.sql` | 创建长期用户记忆表 user_memory，按 user_id 主键保存 memory_json |
| `012_user_memory_row_version.up.sql` | 给 user_memory 增加 row_version，支持长期记忆 CAS 写入 |
| `seed_question_bank.sql` | 演示种子数据（15 道题），不走版本控制，可重复执行 |

## 本地开发：用 docker-compose 跑

```bash
make docker-up           # 启动 pg + redis
make migrate-up          # 应用 001_init.up.sql + 后续增量迁移
psql -h localhost -U interview -d interview -f migrations/seed_question_bank.sql
```

## 手动应用

```bash
psql "postgres://interview:interview@localhost:5432/interview" \
    -v ON_ERROR_STOP=1 \
    -f migrations/001_init.up.sql
psql "postgres://interview:interview@localhost:5432/interview" \
    -v ON_ERROR_STOP=1 \
    -f migrations/002_question_bank_expected_points.up.sql
psql "postgres://interview:interview@localhost:5432/interview" \
    -v ON_ERROR_STOP=1 \
    -f migrations/003_question_bank_metadata.up.sql
psql "postgres://interview:interview@localhost:5432/interview" \
    -v ON_ERROR_STOP=1 \
    -f migrations/004_question_bank_imports.up.sql
psql "postgres://interview:interview@localhost:5432/interview" \
    -v ON_ERROR_STOP=1 \
    -f migrations/005_question_bank_embedding_status.up.sql
psql "postgres://interview:interview@localhost:5432/interview" \
    -v ON_ERROR_STOP=1 \
    -f migrations/006_question_bank_import_lease.up.sql
psql "postgres://interview:interview@localhost:5432/interview" \
    -v ON_ERROR_STOP=1 \
    -f migrations/007_question_bank_import_review_status.up.sql
psql "postgres://interview:interview@localhost:5432/interview" \
    -v ON_ERROR_STOP=1 \
    -f migrations/008_question_bank_import_field_provenance.up.sql
psql "postgres://interview:interview@localhost:5432/interview" \
    -v ON_ERROR_STOP=1 \
    -f migrations/009_question_bank_content_trgm.up.sql
psql "postgres://interview:interview@localhost:5432/interview" \
    -v ON_ERROR_STOP=1 \
    -f migrations/010_session_row_version.up.sql
psql "postgres://interview:interview@localhost:5432/interview" \
    -v ON_ERROR_STOP=1 \
    -f migrations/011_user_memory.up.sql
psql "postgres://interview:interview@localhost:5432/interview" \
    -v ON_ERROR_STOP=1 \
    -f migrations/012_user_memory_row_version.up.sql
```

PowerShell 示例：

```powershell
$dsn = "postgres://interview:interview@localhost:5432/interview"
psql $dsn -v ON_ERROR_STOP=1 -f migrations/001_init.up.sql
psql $dsn -v ON_ERROR_STOP=1 -f migrations/002_question_bank_expected_points.up.sql
psql $dsn -v ON_ERROR_STOP=1 -f migrations/003_question_bank_metadata.up.sql
psql $dsn -v ON_ERROR_STOP=1 -f migrations/004_question_bank_imports.up.sql
psql $dsn -v ON_ERROR_STOP=1 -f migrations/005_question_bank_embedding_status.up.sql
psql $dsn -v ON_ERROR_STOP=1 -f migrations/006_question_bank_import_lease.up.sql
psql $dsn -v ON_ERROR_STOP=1 -f migrations/007_question_bank_import_review_status.up.sql
psql $dsn -v ON_ERROR_STOP=1 -f migrations/008_question_bank_import_field_provenance.up.sql
psql $dsn -v ON_ERROR_STOP=1 -f migrations/009_question_bank_content_trgm.up.sql
psql $dsn -v ON_ERROR_STOP=1 -f migrations/010_session_row_version.up.sql
psql $dsn -v ON_ERROR_STOP=1 -f migrations/011_user_memory.up.sql
psql $dsn -v ON_ERROR_STOP=1 -f migrations/012_user_memory_row_version.up.sql
```

`010_session_row_version.down.sql` 会删除 `sessions.row_version` 并丢失版本历史；生产回滚前应停写或确认没有并发 Session mutation。
`011_user_memory.down.sql` 会删除所有长期用户记忆；生产回滚前应先备份或确认数据可丢弃。
`012_user_memory_row_version.down.sql` 会删除 `user_memory.row_version` 并失去长期记忆 CAS 保护；生产回滚前应停写或确认不会多实例并发写长期记忆。

## CI / 生产

Stage 6 引入 [`golang-migrate/migrate`](https://github.com/golang-migrate/migrate) 做带版本表的自动迁移：

```bash
migrate -path migrations -database "$DATABASE_URL" up
```

`migrate` 工具会自动维护 `schema_migrations` 表记录已应用的版本，避免重复执行。
当前阶段还没引入，靠 `IF NOT EXISTS` 保证脚本可重复跑。

## 索引选型记录

| 索引 | 类型 | 理由 |
|---|---|---|
| `idx_question_bank_embedding` | HNSW | pgvector 0.5+ 推荐；查询快、内存友好；建索引慢（一次性成本） |
| `idx_question_bank_tags` | GIN | 数组 `&&` 包含查询走索引 |
| `idx_question_bank_filter` | B-tree (skill, difficulty) | 向量检索前的候选集预过滤 |
| `idx_sessions_orphan` | B-tree (partial) | 只索引 running/paused 的过期 lease，扫描成本极低 |
| `idx_sessions_expires` | B-tree (partial) | 同上，只索引 completed/failed 待清理的行 |

## pgvector 升级路径

| 当前 | 切到 text-embedding-v3 | 切到 OpenAI text-embedding-3-large |
|---|---|---|
| `vector(1024)` | `vector(1024)` | `vector(3072)` |
| HNSW m=16 | 同 | m=32（高维需要更大 m） |
| ef_construction=64 | 同 | 128 |

切换需要：(1) 加新列 `embedding_v2 vector(1536)` (2) reindex 工具回填 (3) 应用切代码 (4) drop 老列。
不要直接 ALTER 列类型，会锁表 + 丢索引。
