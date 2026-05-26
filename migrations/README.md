# Database Migrations

## 文件命名约定

`NNN_description.up.sql` 应用变更；`NNN_description.down.sql` 回滚。
版本号严格递增，**永远不修改已上线的 migration**，新需求加新文件。

## 当前 migrations

| 文件 | 作用 |
|---|---|
| `001_init.up.sql` | 创建 sessions / question_bank / idempotency_keys / events 表 + pgvector + HNSW 索引 |
| `002_question_bank_expected_points.up.sql` | 给 question_bank 增加 expected_points，用于模拟模式要点反馈 |
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
```

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
