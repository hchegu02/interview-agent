-- Add trigram index for content-based candidate retrieval.
-- pg_trgm extension is created in 001_init.up.sql.
CREATE INDEX IF NOT EXISTS idx_question_bank_content_trgm
    ON question_bank USING gin (content gin_trgm_ops);
