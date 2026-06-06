CREATE TABLE IF NOT EXISTS user_memory (
    user_id text PRIMARY KEY,
    memory_json jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_memory_user_id_not_blank CHECK (btrim(user_id) <> '')
);
