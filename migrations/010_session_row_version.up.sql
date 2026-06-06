-- sessions.row_version 是 PG Session Store 的乐观锁版本。
-- 它是数据库表列的权威值，不依赖 state_json 里的时间戳。
ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS row_version bigint NOT NULL DEFAULT 1;

DO $$
DECLARE
    col record;
BEGIN
    SELECT data_type, is_nullable, column_default
    INTO col
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'sessions'
      AND column_name = 'row_version';

    IF NOT FOUND THEN
        RAISE EXCEPTION 'sessions.row_version column was not created';
    END IF;

    IF col.data_type <> 'bigint'
       OR col.is_nullable <> 'NO'
       OR col.column_default NOT LIKE '%1%' THEN
        RAISE EXCEPTION 'sessions.row_version has unexpected shape: type=%, nullable=%, default=%',
            col.data_type, col.is_nullable, col.column_default;
    END IF;
END $$;
