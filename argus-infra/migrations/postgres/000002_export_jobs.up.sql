-- =============================================================================
--  Argus AI — Migration 002: Export Jobs
--
--  Tracks bulk evidence export requests. Organizations can download archives
--  of session evidence for legal proceedings and long-term archival.
--
--  Lifecycle: pending -> processing -> completed|failed -> expired
--  Archives stored in MinIO 'argus-exports' bucket with Object Lock.
-- =============================================================================

CREATE TABLE IF NOT EXISTS export_jobs (
    id                TEXT        PRIMARY KEY,
    org_id            TEXT        NOT NULL REFERENCES organizations(org_id),
    requested_by      UUID        NOT NULL REFERENCES users(id),
    session_ids       TEXT[]      NOT NULL DEFAULT '{}',
    status            TEXT        NOT NULL DEFAULT 'pending'
                                  CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'expired')),
    archive_uri       TEXT        DEFAULT '',
    manifest_uri      TEXT        DEFAULT '',
    sha256_archive    TEXT        DEFAULT '',
    presign_ttl_sec   INTEGER     NOT NULL DEFAULT 86400,
    download_url      TEXT        DEFAULT '',
    error_message     TEXT        DEFAULT '',
    total_size_bytes  BIGINT      DEFAULT 0,
    fragment_count    INTEGER     DEFAULT 0,
    expires_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at      TIMESTAMPTZ
);

-- Index for polling pending jobs (worker query pattern).
CREATE INDEX IF NOT EXISTS idx_export_jobs_status ON export_jobs(status) WHERE status = 'pending';

-- Index for listing exports by org.
CREATE INDEX IF NOT EXISTS idx_export_jobs_org ON export_jobs(org_id, created_at DESC);
