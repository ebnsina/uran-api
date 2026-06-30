-- 0007_disks.sql — optional persistent disk attached to a service.
-- disk_size empty means no disk. A disk pins the service to a single replica.

ALTER TABLE services ADD COLUMN IF NOT EXISTS disk_size TEXT NOT NULL DEFAULT '';
ALTER TABLE services ADD COLUMN IF NOT EXISTS disk_path TEXT NOT NULL DEFAULT '';
