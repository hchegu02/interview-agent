DROP INDEX IF EXISTS idx_question_bank_embedding_status;

ALTER TABLE question_bank
    DROP COLUMN IF EXISTS embedding_error,
    DROP COLUMN IF EXISTS embedded_at,
    DROP COLUMN IF EXISTS embedding_model,
    DROP COLUMN IF EXISTS embedding_status;
