-- 0001_init.sql — core control-plane schema for Uran.

CREATE TABLE IF NOT EXISTS users (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sessions (
    token      TEXT PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions(user_id);

CREATE TABLE IF NOT EXISTS orgs (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS org_members (
    org_id  BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role    TEXT NOT NULL DEFAULT 'owner',
    PRIMARY KEY (org_id, user_id)
);

CREATE TABLE IF NOT EXISTS projects (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id     BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, slug)
);

CREATE TABLE IF NOT EXISTS services (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id  BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL,
    type        TEXT NOT NULL DEFAULT 'web',          -- web | static | worker | cron
    repo_url    TEXT NOT NULL DEFAULT '',
    branch      TEXT NOT NULL DEFAULT 'main',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, slug)
);

CREATE TABLE IF NOT EXISTS env_vars (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    service_id BIGINT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    secret     BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (service_id, key)
);

CREATE TABLE IF NOT EXISTS deploys (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    service_id BIGINT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    status     TEXT NOT NULL DEFAULT 'queued',        -- queued|building|deploying|live|failed
    commit_sha TEXT NOT NULL DEFAULT '',
    image      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS deploys_service_id_idx ON deploys(service_id);

CREATE TABLE IF NOT EXISTS builds (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    deploy_id  BIGINT NOT NULL REFERENCES deploys(id) ON DELETE CASCADE,
    status     TEXT NOT NULL DEFAULT 'queued',        -- queued|running|succeeded|failed
    logs       TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ,
    ended_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS builds_deploy_id_idx ON builds(deploy_id);
