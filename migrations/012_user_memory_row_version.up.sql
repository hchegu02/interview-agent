ALTER TABLE user_memory
    ADD COLUMN IF NOT EXISTS row_version bigint NOT NULL DEFAULT 1;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'user_memory_row_version_positive'
    ) THEN
        ALTER TABLE user_memory
            ADD CONSTRAINT user_memory_row_version_positive CHECK (row_version > 0);
    END IF;
END $$;
