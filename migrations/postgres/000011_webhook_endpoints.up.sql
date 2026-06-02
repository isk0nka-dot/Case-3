-- =============================================================================
-- Migration 011: Partner Webhook Endpoints and Deliveries
--
-- Durable callback delivery for external LMS partners. The dispatcher polls
-- webhook_deliveries and posts signed payloads to active webhook_endpoints.
-- =============================================================================

CREATE TABLE IF NOT EXISTS webhook_endpoints (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id               TEXT        NOT NULL,
    name                 TEXT        NOT NULL DEFAULT 'Default Webhook',
    url                  TEXT        NOT NULL,
    secret               TEXT        NOT NULL,
    events               TEXT[]      NOT NULL DEFAULT ARRAY['session.started', 'session.completed', 'violation.detected', 'verdict.ready'],
    is_active            BOOLEAN     NOT NULL DEFAULT TRUE,
    last_delivery_at     TIMESTAMPTZ,
    last_failure_at      TIMESTAMPTZ,
    consecutive_failures INT         NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at           TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_org
    ON webhook_endpoints (org_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_events
    ON webhook_endpoints USING GIN (events)
    WHERE deleted_at IS NULL AND is_active = TRUE;

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id             BIGSERIAL PRIMARY KEY,
    endpoint_id    UUID        NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
    org_id         TEXT        NOT NULL,
    event_type     TEXT        NOT NULL,
    payload        JSONB       NOT NULL,
    status         TEXT        NOT NULL DEFAULT 'pending',
    http_status    INT         NOT NULL DEFAULT 0,
    response_body  TEXT        NOT NULL DEFAULT '',
    error_message  TEXT        NOT NULL DEFAULT '',
    attempt        INT         NOT NULL DEFAULT 0,
    max_attempts   INT         NOT NULL DEFAULT 5,
    next_retry_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending
    ON webhook_deliveries (next_retry_at, id)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_endpoint
    ON webhook_deliveries (endpoint_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_org
    ON webhook_deliveries (org_id, created_at DESC);

COMMENT ON TABLE webhook_endpoints IS 'Partner callback endpoints for signed Argus webhook events';
COMMENT ON TABLE webhook_deliveries IS 'Durable webhook delivery queue with retry metadata';
