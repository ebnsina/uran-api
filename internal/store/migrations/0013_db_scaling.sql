-- 0013_db_scaling.sql — managed database HA (replicas) and sizing.
-- instances: number of Postgres nodes (1 = single, >1 = primary + standbys with
-- a load-balanced read endpoint). read_uri is the read-only endpoint URI.

ALTER TABLE databases ADD COLUMN IF NOT EXISTS instances INTEGER NOT NULL DEFAULT 1;
ALTER TABLE databases ADD COLUMN IF NOT EXISTS size      TEXT    NOT NULL DEFAULT 'small';
ALTER TABLE databases ADD COLUMN IF NOT EXISTS storage   TEXT    NOT NULL DEFAULT '1Gi';
ALTER TABLE databases ADD COLUMN IF NOT EXISTS read_uri  TEXT    NOT NULL DEFAULT '';
