DROP INDEX IF EXISTS idx_qb_import_items_job_review;

ALTER TABLE question_bank_import_items
    DROP COLUMN IF EXISTS review_status;
