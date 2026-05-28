-- Question bank import staging.
-- Imported or LLM-generated questions must be validated in staging before they
-- are committed into question_bank.

CREATE TABLE IF NOT EXISTS question_bank_import_jobs (
    id              text        PRIMARY KEY,
    source_type     text        NOT NULL CHECK (source_type IN ('question_bank','document')),
    filename        text        NOT NULL DEFAULT '',
    status          text        NOT NULL CHECK (status IN ('queued','created','parsing','generating','validating','ready','committing','committed','failed')),
    total_chunks    integer     NOT NULL DEFAULT 0,
    total_items     integer     NOT NULL DEFAULT 0,
    valid_items     integer     NOT NULL DEFAULT 0,
    invalid_items   integer     NOT NULL DEFAULT 0,
    imported_items  integer     NOT NULL DEFAULT 0,
    error           text        NOT NULL DEFAULT '',
    metadata        jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS question_bank_import_chunks (
    id              text        PRIMARY KEY,
    job_id          text        NOT NULL REFERENCES question_bank_import_jobs(id) ON DELETE CASCADE,
    chunk_index     integer     NOT NULL,
    content         text        NOT NULL,
    metadata        jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (job_id, chunk_index)
);

CREATE TABLE IF NOT EXISTS question_bank_import_items (
    id              text        PRIMARY KEY,
    job_id          text        NOT NULL REFERENCES question_bank_import_jobs(id) ON DELETE CASCADE,
    chunk_id        text        REFERENCES question_bank_import_chunks(id) ON DELETE SET NULL,
    question_id     text        NOT NULL,
    status          text        NOT NULL CHECK (status IN ('valid','invalid','rejected','imported')),
    item_json       jsonb       NOT NULL,
    errors          text[]      NOT NULL DEFAULT '{}',
    raw_json        jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_qb_import_jobs_status
    ON question_bank_import_jobs (status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_qb_import_chunks_job
    ON question_bank_import_chunks (job_id, chunk_index);

CREATE INDEX IF NOT EXISTS idx_qb_import_items_job_status
    ON question_bank_import_items (job_id, status);

DROP TRIGGER IF EXISTS qb_import_jobs_set_updated_at ON question_bank_import_jobs;
CREATE TRIGGER qb_import_jobs_set_updated_at
    BEFORE UPDATE ON question_bank_import_jobs
    FOR EACH ROW EXECUTE FUNCTION trg_set_updated_at();
