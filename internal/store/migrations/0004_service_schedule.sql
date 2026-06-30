-- 0004_service_schedule.sql — cron services need a schedule (cron expression).
-- Empty for non-cron service types.

ALTER TABLE services ADD COLUMN IF NOT EXISTS schedule TEXT NOT NULL DEFAULT '';
