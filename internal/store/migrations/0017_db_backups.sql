-- 0017_db_backups.sql — opt-in continuous backups (WAL archiving + scheduled)
-- to object storage for managed Postgres, enabling point-in-time recovery.

ALTER TABLE databases ADD COLUMN IF NOT EXISTS backups BOOLEAN NOT NULL DEFAULT false;
