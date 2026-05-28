-- Track whether a question is actually ready for vector RAG.
-- A row in question_bank without embedding is only list/filter searchable.

ALTER TABLE question_bank
    ADD COLUMN IF NOT EXISTS embedding_status text NOT NULL DEFAULT 'pending'
        CHECK (embedding_status IN ('pending','embedded','failed')),
    ADD COLUMN IF NOT EXISTS embedding_model text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS embedded_at timestamptz,
    ADD COLUMN IF NOT EXISTS embedding_error text NOT NULL DEFAULT '';

UPDATE question_bank
SET embedding_status = 'embedded',
    embedded_at = COALESCE(embedded_at, updated_at, created_at)
WHERE embedding IS NOT NULL
  AND embedding_status <> 'embedded';

CREATE INDEX IF NOT EXISTS idx_question_bank_embedding_status
    ON question_bank (embedding_status, updated_at DESC);
