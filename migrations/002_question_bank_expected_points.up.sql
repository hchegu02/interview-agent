ALTER TABLE question_bank
    ADD COLUMN IF NOT EXISTS expected_points text[] NOT NULL DEFAULT '{}';
