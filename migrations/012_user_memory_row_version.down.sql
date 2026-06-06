ALTER TABLE user_memory
    DROP CONSTRAINT IF EXISTS user_memory_row_version_positive,
    DROP COLUMN IF EXISTS row_version;
