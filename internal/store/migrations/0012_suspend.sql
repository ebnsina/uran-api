-- 0012_suspend.sql — a suspended service runs zero replicas (scaled to zero).

ALTER TABLE services ADD COLUMN IF NOT EXISTS suspended BOOLEAN NOT NULL DEFAULT false;
