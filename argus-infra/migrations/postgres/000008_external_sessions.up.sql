CREATE TABLE IF NOT EXISTS external_sessions (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id       TEXT        NOT NULL UNIQUE,
    org_id           TEXT        NOT NULL,
    exam_id          TEXT        NOT NULL,
    student_id       TEXT        NOT NULL,
    student_name     TEXT,
    exam_name        TEXT,
    callback_url     TEXT,
    metadata         JSONB,
    session_token    TEXT        NOT NULL,
    token_expires_at TIMESTAMPTZ NOT NULL,
    status           TEXT        NOT NULL DEFAULT 'created',
    verdict          TEXT,
    verdict_details  JSONB,
    integrity_score  FLOAT8,
    violation_count  INT         NOT NULL DEFAULT 0,
    started_at       TIMESTAMPTZ,
    completed_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_external_sessions_org_id     ON external_sessions(org_id);
CREATE INDEX IF NOT EXISTS idx_external_sessions_exam_student ON external_sessions(exam_id, student_id);
CREATE INDEX IF NOT EXISTS idx_external_sessions_status      ON external_sessions(status);
