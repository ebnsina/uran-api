-- 0010_build_args.sql — env vars can additionally be exposed at build time.
-- A build_time var is passed to the build (Docker --build-arg / Nixpacks env)
-- and is still injected at runtime like any other env var.

ALTER TABLE env_vars ADD COLUMN IF NOT EXISTS build_time BOOLEAN NOT NULL DEFAULT false;
