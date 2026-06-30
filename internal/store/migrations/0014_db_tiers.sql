-- 0014_db_tiers.sql — database tier: "standard" (fixed) or "autoscale" (the
-- controller scales Postgres instances between min/max on CPU load).

ALTER TABLE databases ADD COLUMN IF NOT EXISTS tier          TEXT    NOT NULL DEFAULT 'standard';
ALTER TABLE databases ADD COLUMN IF NOT EXISTS min_instances INTEGER NOT NULL DEFAULT 1;
ALTER TABLE databases ADD COLUMN IF NOT EXISTS max_instances INTEGER NOT NULL DEFAULT 1;
