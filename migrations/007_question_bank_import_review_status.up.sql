-- Item-level import review state.
-- Validation status stays separate from human review status: invalid items remain
-- invalid, while valid items can be accepted or rejected before commit.

ALTER TABLE question_bank_import_items
    ADD COLUMN IF NOT EXISTS review_status text NOT NULL DEFAULT 'accepted'
    CHECK (review_status IN ('accepted','rejected'));

CREATE INDEX IF NOT EXISTS idx_qb_import_items_job_review
    ON question_bank_import_items (job_id, review_status);
