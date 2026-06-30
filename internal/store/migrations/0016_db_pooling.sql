-- 0016_db_pooling.sql — optional PgBouncer connection pooler for Postgres.

ALTER TABLE databases ADD COLUMN IF NOT EXISTS pooling    BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE databases ADD COLUMN IF NOT EXISTS pooled_uri TEXT    NOT NULL DEFAULT '';
