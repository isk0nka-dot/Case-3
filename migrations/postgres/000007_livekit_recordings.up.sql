-- =============================================================================
-- Migration 007: LiveKit Egress Recording Metadata
--
-- Stores the durable mapping between a LiveKit Egress ID and an Argus
-- proctoring session. Webhooks update rows by egress_id instead of parsing
-- session identifiers from object-storage paths.
-- =============================================================================

CREATE TABLE IF NOT EXISTS livekit_recordings (
    egress_id      TEXT PRIMARY KEY,
    session_id     TEXT        NOT NULL,
    user_id        TEXT        NOT NULL,
    room_name      TEXT        NOT NULL,
    video_track_id TEXT        NOT NULL DEFAULT '',
    audio_track_id TEXT        NOT NULL DEFAULT '',
    status         TEXT        NOT NULL,
    file_url       TEXT        NOT NULL DEFAULT '',
    error_message  TEXT        NOT NULL DEFAULT '',
    started_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at       TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_livekit_recordings_session
    ON livekit_recordings (session_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_livekit_recordings_user
    ON livekit_recordings (user_id, created_at DESC);

COMMENT ON TABLE livekit_recordings IS 'LiveKit Egress recording metadata keyed by egress_id for webhook-safe updates';
