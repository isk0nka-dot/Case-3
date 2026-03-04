-- =============================================================================
--  Argus AI — Migration 003: Consent Records
--
--  Tracks student consent to proctoring terms before exam sessions.
--  Immutable append-only table — records are never modified or deleted.
--  Required for GDPR/privacy compliance and legal defensibility.
-- =============================================================================

CREATE TABLE IF NOT EXISTS consent_records (
    id                TEXT        PRIMARY KEY,
    session_id        TEXT        NOT NULL,
    student_id        TEXT        NOT NULL,
    org_id            TEXT        NOT NULL REFERENCES organizations(org_id),
    exam_id           TEXT        NOT NULL,
    consent_version   TEXT        NOT NULL DEFAULT '1.0',
    consent_text      TEXT        NOT NULL DEFAULT '',
    accepted          BOOLEAN     NOT NULL DEFAULT FALSE,
    ip_address        TEXT        DEFAULT '',
    user_agent        TEXT        DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for looking up consent by session (primary query pattern).
CREATE INDEX IF NOT EXISTS idx_consent_session ON consent_records(session_id);

-- Index for auditing consent by student.
CREATE INDEX IF NOT EXISTS idx_consent_student ON consent_records(student_id, created_at DESC);

-- Index for org-level consent reporting.
CREATE INDEX IF NOT EXISTS idx_consent_org ON consent_records(org_id, created_at DESC);
