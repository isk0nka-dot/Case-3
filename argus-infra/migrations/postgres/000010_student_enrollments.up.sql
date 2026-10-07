-- 000010: Student biometric enrollment for face identity verification.
--
-- Stores a reference face embedding (ArcFace 512-dim float32 vector) per
-- student per organisation. Used by the AI analysis worker to compute
-- face similarity (identity check) during backend deep scan.
--
-- One row per (student_id, org_id) — upsert on re-enroll.

CREATE TABLE IF NOT EXISTS student_enrollments (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id      TEXT        NOT NULL,
    org_id          TEXT        NOT NULL,
    -- ArcFace 512-dim vector stored as JSONB float array.
    -- Size: 512 × 4 bytes ≈ 2 KB per row, well within TOAST threshold.
    embedding       JSONB       NOT NULL,
    photo_url       TEXT,
    enrolled_by     TEXT        NOT NULL DEFAULT 'lms',  -- 'lms' | 'manual' | 'preflight'
    model_version   TEXT        NOT NULL DEFAULT 'arcface_r50_w600k',
    enrolled_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_enrollment UNIQUE (student_id, org_id)
);

CREATE INDEX IF NOT EXISTS idx_enrollments_student_org
    ON student_enrollments (student_id, org_id);

CREATE INDEX IF NOT EXISTS idx_enrollments_org
    ON student_enrollments (org_id);
