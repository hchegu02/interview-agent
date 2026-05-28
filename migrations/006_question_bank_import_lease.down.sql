DROP INDEX IF EXISTS idx_qb_import_jobs_lease;

ALTER TABLE question_bank_import_jobs
    DROP COLUMN IF EXISTS lease_until,
    DROP COLUMN IF EXISTS owner_id;
