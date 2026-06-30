-- 0009_audit_log.sql — records authenticated mutating actions.

CREATE TABLE IF NOT EXISTS audit_logs (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    actor_email   TEXT NOT NULL,
    method        TEXT NOT NULL,
    path          TEXT NOT NULL,
    status        INTEGER NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS audit_logs_actor_idx ON audit_logs(actor_user_id, id DESC);
