-- =============================================================================
-- Migration 006: Transactional Outbox — Job Durability Layer
--
-- Implements the Transactional Outbox pattern for asynq/Redis job dispatch.
-- Every job is first persisted in PostgreSQL within the same transaction as
-- the business operation, then asynchronously relayed to Redis. This prevents
-- job loss during Redis OOM or network partition.
--
-- Flow:
--   1. Business handler INSERTs into outbox_jobs within its DB transaction
--   2. Background relay goroutine polls for status='pending' rows
--   3. Relay enqueues to Redis/asynq, then UPDATEs status='dispatched'
--   4. On Redis failure: row stays 'pending', retried on next poll
--
-- Guarantees: At-least-once delivery. Asynq deduplication (TaskID) handles
-- the at-most-once side via idempotent processing.
-- =============================================================================

CREATE TABLE IF NOT EXISTS outbox_jobs (
    id              BIGSERIAL PRIMARY KEY,
    task_type       TEXT        NOT NULL,
    task_id         TEXT        NOT NULL UNIQUE,
    payload         JSONB       NOT NULL,
    queue           TEXT        NOT NULL DEFAULT 'default',
    max_retry       INT         NOT NULL DEFAULT 3,
    timeout_sec     INT         NOT NULL DEFAULT 300,
    status          TEXT        NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'dispatched', 'failed')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dispatched_at   TIMESTAMPTZ,
    error_message   TEXT,
    retry_count     INT         NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_outbox_jobs_pending
    ON outbox_jobs (status, created_at)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_outbox_jobs_task_id
    ON outbox_jobs (task_id);

COMMENT ON TABLE outbox_jobs IS 'Transactional outbox for asynq job dispatch — prevents task loss during Redis OOM';
