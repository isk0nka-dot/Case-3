DROP INDEX IF EXISTS idx_external_sessions_org_idempotency;

ALTER TABLE external_sessions
    DROP COLUMN IF EXISTS idempotency_key;

