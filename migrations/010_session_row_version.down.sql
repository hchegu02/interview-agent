-- 注意：回滚会丢失所有 Session 的乐观锁版本历史。
-- 生产环境执行前应停写或确认没有并发 Session mutation。
ALTER TABLE sessions
    DROP COLUMN IF EXISTS row_version;
