-- 0005_databases.sql — managed databases provisioned for a project.

CREATE TABLE IF NOT EXISTS databases (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id     BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    slug           TEXT NOT NULL,
    engine         TEXT NOT NULL DEFAULT 'postgres',  -- postgres
    status         TEXT NOT NULL DEFAULT 'creating',  -- creating|ready|failed
    connection_uri TEXT NOT NULL DEFAULT '',          -- populated when ready
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, slug)
);
CREATE INDEX IF NOT EXISTS databases_project_id_idx ON databases(project_id);
