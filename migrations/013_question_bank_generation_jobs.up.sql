CREATE TABLE IF NOT EXISTS question_bank_generation_jobs (
    id              text        PRIMARY KEY,
    status          text        NOT NULL,
    source_job_id   text        NOT NULL,
    request_json    jsonb       NOT NULL DEFAULT '{}'::jsonb,
    job_json        jsonb       NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_qb_generation_jobs_status
    ON question_bank_generation_jobs (status, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_qb_generation_jobs_source
    ON question_bank_generation_jobs (source_job_id, created_at DESC);
