-- =============================================================================
--  Argus AI — Migration 004: Appeals Workflow
--
--  Students can appeal proctoring violation decisions. Implements a simple
--  state machine: submitted -> under_review -> upheld|overturned|withdrawn.
-- =============================================================================

CREATE TABLE IF NOT EXISTS appeals (
    id              TEXT        PRIMARY KEY,
    session_id      TEXT        NOT NULL,
    student_id      TEXT        NOT NULL,
    org_id          TEXT        NOT NULL REFERENCES organizations(org_id),
    exam_id         TEXT        NOT NULL,
    reason          TEXT        NOT NULL DEFAULT '',
    status          TEXT        NOT NULL DEFAULT 'submitted'
                                CHECK (status IN ('submitted', 'under_review', 'upheld', 'overturned', 'withdrawn')),
    reviewed_by     TEXT        DEFAULT '',
    review_notes    TEXT        DEFAULT '',
    reviewed_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for listing appeals by org (admin dashboard).
CREATE INDEX IF NOT EXISTS idx_appeals_org ON appeals(org_id, created_at DESC);

-- Index for looking up appeals by session.
CREATE INDEX IF NOT EXISTS idx_appeals_session ON appeals(session_id);

-- Index for filtering pending appeals (admin review queue).
CREATE INDEX IF NOT EXISTS idx_appeals_status ON appeals(status) WHERE status IN ('submitted', 'under_review');
