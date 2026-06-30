-- 0003_custom_domains.sql — user-supplied custom domains for a service.
-- A service is always reachable at its default <slug>.<base-domain> host; custom
-- domains are additional hostnames routed to the same workload.

CREATE TABLE IF NOT EXISTS custom_domains (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    service_id BIGINT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    domain     TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS custom_domains_service_id_idx ON custom_domains(service_id);
