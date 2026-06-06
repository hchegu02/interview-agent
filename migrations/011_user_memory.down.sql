-- 注意：回滚会删除所有长期用户记忆。
-- 生产环境执行前应确认这些画像数据可以丢弃或已经备份。
DROP TABLE IF EXISTS user_memory;
