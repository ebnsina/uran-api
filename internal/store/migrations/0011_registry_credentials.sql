-- 0011_registry_credentials.sql — org-level credentials for private image
-- registries, used to build a Kubernetes image pull secret.

CREATE TABLE IF NOT EXISTS registry_credentials (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id     BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    registry   TEXT NOT NULL,
    username   TEXT NOT NULL,
    password   TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, registry)
);
CREATE INDEX IF NOT EXISTS registry_credentials_org_idx ON registry_credentials(org_id);
