DROP INDEX IF EXISTS idx_question_bank_role_tags;
DROP INDEX IF EXISTS idx_question_bank_scenario;
DROP INDEX IF EXISTS idx_question_bank_status;

ALTER TABLE question_bank
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS locale,
    DROP COLUMN IF EXISTS follow_up_hints,
    DROP COLUMN IF EXISTS sample_answer,
    DROP COLUMN IF EXISTS rubric,
    DROP COLUMN IF EXISTS role_tags,
    DROP COLUMN IF EXISTS scenario;
