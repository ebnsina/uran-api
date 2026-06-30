-- 0006_scaling_health.sql — per-service scaling and health-check settings.
-- min_replicas/max_replicas both 0 means autoscaling is disabled and the fixed
-- `replicas` count is used. health_path empty means a TCP check on the port.

ALTER TABLE services ADD COLUMN IF NOT EXISTS replicas      INTEGER NOT NULL DEFAULT 1;
ALTER TABLE services ADD COLUMN IF NOT EXISTS instance_size TEXT    NOT NULL DEFAULT 'small';
ALTER TABLE services ADD COLUMN IF NOT EXISTS health_path   TEXT    NOT NULL DEFAULT '';
ALTER TABLE services ADD COLUMN IF NOT EXISTS min_replicas  INTEGER NOT NULL DEFAULT 0;
ALTER TABLE services ADD COLUMN IF NOT EXISTS max_replicas  INTEGER NOT NULL DEFAULT 0;
