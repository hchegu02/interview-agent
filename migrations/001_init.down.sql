-- 001_init.down.sql
-- 回滚 001_init.up.sql
-- 注意：down 不删除 extension（其他库可能在用）

DROP TRIGGER IF EXISTS sessions_set_updated_at ON sessions;
DROP FUNCTION IF EXISTS trg_set_updated_at();

DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS question_bank;
DROP TABLE IF EXISTS sessions;

-- pgvector / pg_trgm 不卸载，避免影响共享 PG 实例上的其他库
