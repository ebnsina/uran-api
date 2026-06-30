-- 0002_previews.sql — support per-PR preview environments.
-- A deploy is either a "production" deploy (the service's configured branch) or
-- a "preview" deploy tied to a pull request number.

ALTER TABLE deploys ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'production';
ALTER TABLE deploys ADD COLUMN IF NOT EXISTS pr_number INTEGER;

CREATE INDEX IF NOT EXISTS deploys_service_pr_idx ON deploys(service_id, pr_number);
