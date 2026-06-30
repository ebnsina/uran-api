-- 0015_usage.sql — periodic resource-usage samples per service (metering).

CREATE TABLE IF NOT EXISTS usage_samples (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    service_id     BIGINT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    cpu_millicores BIGINT NOT NULL,
    memory_bytes   BIGINT NOT NULL,
    sampled_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS usage_samples_service_idx ON usage_samples(service_id, id DESC);
