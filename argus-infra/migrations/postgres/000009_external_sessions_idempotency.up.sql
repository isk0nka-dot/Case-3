ALTER TABLE external_sessions
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_external_sessions_org_idempotency
    ON external_sessions(org_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

