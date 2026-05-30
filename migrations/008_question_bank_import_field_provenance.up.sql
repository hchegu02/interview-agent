-- Persist field-level source metadata for import diff preview.
-- This keeps staging metadata separate from uploaded raw_json and staged item_json.

ALTER TABLE question_bank_import_items
    ADD COLUMN IF NOT EXISTS field_provenance jsonb NOT NULL DEFAULT '{}'::jsonb;
