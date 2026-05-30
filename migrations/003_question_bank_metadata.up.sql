ALTER TABLE question_bank
    ADD COLUMN IF NOT EXISTS scenario text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS role_tags text[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS rubric jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS sample_answer text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS follow_up_hints text[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS locale text NOT NULL DEFAULT 'zh-CN',
    ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS idx_question_bank_status
    ON question_bank (status);

CREATE INDEX IF NOT EXISTS idx_question_bank_scenario
    ON question_bank (scenario);

CREATE INDEX IF NOT EXISTS idx_question_bank_role_tags
    ON question_bank USING gin (role_tags);
