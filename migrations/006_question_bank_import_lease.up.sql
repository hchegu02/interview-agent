ALTER TABLE question_bank_import_jobs
    ADD COLUMN IF NOT EXISTS owner_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS lease_until timestamptz;

CREATE INDEX IF NOT EXISTS idx_qb_import_jobs_lease
    ON question_bank_import_jobs (status, lease_until);
