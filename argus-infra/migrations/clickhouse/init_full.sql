-- =============================================================================
--  Argus AI — ClickHouse Full Init Schema (Fresh Install)
--  Database: argus_analytics
--
--  This is the CONSOLIDATED init script for fresh Docker deployments.
--  It represents the final schema state after all migrations (001–005):
--    001 — Initial schema (base tables + materialized views)
--    002 — Forensic ledger columns on evidence_fragments (absorbed into 001)
--    003 — AI Vision + Audio columns on proctoring_events
--    004 — ReplacingMergeTree engine upgrade + ORDER BY includes event_id
--    005 — causal_sequence HLC column on proctoring_events
--
--  All statements use CREATE TABLE IF NOT EXISTS — fully idempotent.
--  Run via docker-entrypoint-initdb.d on first container startup.
-- =============================================================================


-- =============================================================================
--  1. proctoring_events (final state: 004 engine + 003 AI columns + 005 HLC)
--
--  Engine: ReplacingMergeTree(server_timestamp)
--    Deduplicates events with the same ORDER BY key on background merge.
--    Required for DLQ replay safety — guarantees at-most-once delivery
--    when the same event arrives twice (Kafka retry + DLQ re-emit).
--
--  ORDER BY includes event_id (vs. server_timestamp in migration 001):
--    Enables point deduplication by event_id, not just session+time.
-- =============================================================================

CREATE TABLE IF NOT EXISTS proctoring_events
(
    -- Identity
    event_id            String                                    COMMENT 'UUIDv7 — unique event identifier',
    session_id          String                                    COMMENT 'Proctoring session identifier',
    student_id          String                                    COMMENT 'Student IIN',
    exam_id             String                                    COMMENT 'Exam identifier',
    org_id              String                                    COMMENT 'Tenant isolation boundary',

    -- Classification
    event_type          LowCardinality(String)                    COMMENT 'Event type (40 types across 8 categories)',
    severity            LowCardinality(String)                    COMMENT 'unspecified | info | warning | critical',
    source              LowCardinality(String)                    COMMENT 'webcam | side_camera | system | browser | kernel_agent | network_probe',

    -- Timestamps
    server_timestamp    DateTime64(3, 'UTC')                      COMMENT 'Server receipt time — authoritative, used for ORDER BY and TTL',
    client_timestamp    DateTime64(3, 'UTC')                      COMMENT 'Client-side time (may drift)',
    video_timestamp_sec Float64             DEFAULT 0             COMMENT 'Seconds from session start for video replay',

    -- Detection
    label               String              DEFAULT ''            COMMENT 'Human-readable description',
    confidence          Float32             DEFAULT 0             COMMENT 'AI model confidence [0.0, 1.0]',
    payload             String              DEFAULT ''            COMMENT 'JSON type-specific payload',
    payload_type        LowCardinality(String) DEFAULT ''         COMMENT 'gaze | face | object | audio | browser | system | psychometry | network | kernel',

    -- Client metadata
    user_agent          String              DEFAULT ''            COMMENT 'Browser User-Agent',
    sdk_version         LowCardinality(String) DEFAULT ''         COMMENT 'Client SDK version',
    resolution          LowCardinality(String) DEFAULT ''         COMMENT 'Screen resolution (e.g. 1920x1080)',
    timezone_offset_min Int32               DEFAULT 0             COMMENT 'UTC offset in minutes',
    ip_address          String              DEFAULT ''            COMMENT 'Client IP (set server-side)',
    region              LowCardinality(String) DEFAULT ''         COMMENT 'Geographic region from IP',

    -- AI Vision (migration 003)
    head_yaw            Float32             DEFAULT 0             COMMENT 'Head yaw in degrees (-90..+90)',
    head_pitch          Float32             DEFAULT 0             COMMENT 'Head pitch in degrees (-90..+90)',
    head_roll           Float32             DEFAULT 0             COMMENT 'Head roll in degrees (-180..+180)',
    face_bbox           String              DEFAULT ''            COMMENT 'Face bbox JSON: {"x":0.1,"y":0.2,"w":0.3,"h":0.4}',
    liveness_score      Float32             DEFAULT -1            COMMENT 'Liveness probability (0=spoof, 1=real, -1=not computed)',
    face_embedding      Array(Float32)      DEFAULT []            COMMENT '512-dim ArcFace embedding',
    face_similarity     Float32             DEFAULT -1            COMMENT 'Cosine similarity to enrolled reference (-1=not computed)',

    -- AI Audio (migration 003)
    audio_rms_db        Float32             DEFAULT -100          COMMENT 'RMS dB A-weighted (-100=silence)',
    vad_active          UInt8               DEFAULT 0             COMMENT 'Voice Activity Detection (0=no, 1=yes)',
    audio_classification LowCardinality(String) DEFAULT ''        COMMENT 'silence | speech | whisper | music | keyboard | ambient',
    speaker_count       UInt8               DEFAULT 0             COMMENT 'Number of distinct speakers detected',
    speaker_match       UInt8               DEFAULT 0             COMMENT 'Voice matches enrolled voiceprint (0=no, 1=yes)',

    -- Causal ordering (migration 005)
    causal_sequence     UInt64              DEFAULT 0             COMMENT 'HLC value for causal ordering (physical_ms << 16 | logical)'

    -- Data skipping indices
    , INDEX idx_event_id        event_id          TYPE bloom_filter(0.01) GRANULARITY 1
    , INDEX idx_session_id      session_id        TYPE bloom_filter(0.01) GRANULARITY 1
    , INDEX idx_student_id      student_id        TYPE bloom_filter(0.01) GRANULARITY 1
    , INDEX idx_ip_address      ip_address        TYPE bloom_filter(0.01) GRANULARITY 4

    , INDEX idx_event_type      event_type        TYPE set(100)           GRANULARITY 4
    , INDEX idx_severity        severity          TYPE set(10)            GRANULARITY 4
    , INDEX idx_source          source            TYPE set(20)            GRANULARITY 4
    , INDEX idx_payload_type    payload_type      TYPE set(20)            GRANULARITY 4

    , INDEX idx_client_ts       client_timestamp  TYPE minmax             GRANULARITY 4
    , INDEX idx_confidence      confidence        TYPE minmax             GRANULARITY 4

    , INDEX idx_liveness        liveness_score    TYPE minmax             GRANULARITY 4
    , INDEX idx_face_similarity face_similarity   TYPE minmax             GRANULARITY 4
    , INDEX idx_audio_class     audio_classification TYPE set(20)         GRANULARITY 4
    , INDEX idx_vad             vad_active        TYPE set(2)             GRANULARITY 4
    , INDEX idx_head_yaw        head_yaw          TYPE minmax             GRANULARITY 4
)
ENGINE = ReplacingMergeTree(server_timestamp)
PARTITION BY toYYYYMM(server_timestamp)
ORDER BY (org_id, exam_id, session_id, event_id)
TTL server_timestamp + INTERVAL 90 DAY DELETE
SETTINGS
    index_granularity        = 8192,
    min_bytes_for_wide_part  = 10485760,
    parts_to_delay_insert    = 300,
    parts_to_throw_insert    = 600;


-- =============================================================================
--  2. session_event_counts — pre-aggregated per-session violation counts
-- =============================================================================

CREATE TABLE IF NOT EXISTS session_event_counts
(
    session_id      String,
    exam_id         String,
    org_id          String,
    event_type      LowCardinality(String),
    severity        LowCardinality(String),
    count           UInt64,
    last_seen       DateTime64(3, 'UTC'),
    min_confidence  Float32 DEFAULT 1.0,
    max_confidence  Float32 DEFAULT 0.0,

    INDEX idx_sc_session session_id TYPE bloom_filter(0.01) GRANULARITY 1
)
ENGINE = SummingMergeTree(count)
PARTITION BY toYYYYMM(last_seen)
ORDER BY (org_id, exam_id, session_id, event_type, severity)
TTL last_seen + INTERVAL 90 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS session_event_counts_mv
TO session_event_counts
AS
SELECT
    session_id,
    exam_id,
    org_id,
    event_type,
    severity,
    toUInt64(1)      AS count,
    server_timestamp AS last_seen,
    confidence       AS min_confidence,
    confidence       AS max_confidence
FROM proctoring_events;


-- =============================================================================
--  3. hourly_event_stats — hourly aggregation for trend dashboards
-- =============================================================================

CREATE TABLE IF NOT EXISTS hourly_event_stats
(
    hour             DateTime,
    org_id           String,
    exam_id          String,
    event_type       LowCardinality(String),
    severity         LowCardinality(String),
    event_count      UInt64,
    unique_sessions  UInt64,
    avg_confidence   Float64,
    confidence_count UInt64,

    INDEX idx_hs_hour hour TYPE minmax GRANULARITY 1
)
ENGINE = SummingMergeTree((event_count, unique_sessions, avg_confidence, confidence_count))
PARTITION BY toYYYYMM(hour)
ORDER BY (org_id, exam_id, event_type, severity, hour)
TTL hour + INTERVAL 180 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS hourly_event_stats_mv
TO hourly_event_stats
AS
SELECT
    toStartOfHour(server_timestamp) AS hour,
    org_id,
    exam_id,
    event_type,
    severity,
    toUInt64(1)                     AS event_count,
    toUInt64(1)                     AS unique_sessions,
    toFloat64(confidence)           AS avg_confidence,
    toUInt64(1)                     AS confidence_count
FROM proctoring_events;


-- =============================================================================
--  4. critical_events_recent — real-time critical-violation feed (7-day TTL)
-- =============================================================================

CREATE TABLE IF NOT EXISTS critical_events_recent
(
    event_id         String,
    session_id       String,
    student_id       String,
    exam_id          String,
    org_id           String,
    event_type       LowCardinality(String),
    source           LowCardinality(String),
    server_timestamp DateTime64(3, 'UTC'),
    label            String          DEFAULT '',
    confidence       Float32         DEFAULT 0,
    payload_type     LowCardinality(String) DEFAULT '',

    INDEX idx_cr_session  session_id TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_cr_student  student_id TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_cr_type     event_type TYPE set(100)           GRANULARITY 2
)
ENGINE = MergeTree
PARTITION BY toYYYYMMDD(server_timestamp)
ORDER BY (org_id, exam_id, server_timestamp)
TTL server_timestamp + INTERVAL 7 DAY
SETTINGS index_granularity = 4096;

CREATE MATERIALIZED VIEW IF NOT EXISTS critical_events_recent_mv
TO critical_events_recent
AS
SELECT
    event_id,
    session_id,
    student_id,
    exam_id,
    org_id,
    event_type,
    source,
    server_timestamp,
    label,
    confidence,
    payload_type
FROM proctoring_events
WHERE severity = 'critical';


-- =============================================================================
--  5. student_session_summary — per-student violation counters
-- =============================================================================

CREATE TABLE IF NOT EXISTS student_session_summary
(
    student_id          String,
    session_id          String,
    exam_id             String,
    org_id              String,
    total_events        UInt64,
    critical_count      UInt64,
    warning_count       UInt64,
    info_count          UInt64,
    first_event_time    DateTime64(3, 'UTC'),
    last_event_time     DateTime64(3, 'UTC'),

    INDEX idx_ss_student student_id TYPE bloom_filter(0.01) GRANULARITY 1
)
ENGINE = SummingMergeTree((total_events, critical_count, warning_count, info_count))
PARTITION BY toYYYYMM(last_event_time)
ORDER BY (org_id, exam_id, student_id, session_id)
TTL last_event_time + INTERVAL 90 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS student_session_summary_mv
TO student_session_summary
AS
SELECT
    student_id,
    session_id,
    exam_id,
    org_id,
    toUInt64(1)                              AS total_events,
    toUInt64(if(severity = 'critical', 1, 0)) AS critical_count,
    toUInt64(if(severity = 'warning',  1, 0)) AS warning_count,
    toUInt64(if(severity = 'info',     1, 0)) AS info_count,
    server_timestamp                          AS first_event_time,
    server_timestamp                          AS last_event_time
FROM proctoring_events;


-- =============================================================================
--  6. global_org_stats — cross-org daily aggregation for Super Admin dashboard
-- =============================================================================

CREATE TABLE IF NOT EXISTS global_org_stats
(
    day              Date,
    org_id           String,
    event_count      UInt64,
    critical_count   UInt64,
    warning_count    UInt64,
    session_count    UInt64,
    exam_count       UInt64,

    INDEX idx_gos_org org_id TYPE set(1000) GRANULARITY 1
)
ENGINE = SummingMergeTree((event_count, critical_count, warning_count, session_count, exam_count))
PARTITION BY toYYYYMM(day)
ORDER BY (day, org_id)
TTL day + INTERVAL 365 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS global_org_stats_mv
TO global_org_stats
AS
SELECT
    toDate(server_timestamp)                           AS day,
    org_id,
    toUInt64(1)                                        AS event_count,
    toUInt64(if(severity = 'critical', 1, 0))          AS critical_count,
    toUInt64(if(severity = 'warning',  1, 0))          AS warning_count,
    toUInt64(1)                                        AS session_count,
    toUInt64(1)                                        AS exam_count
FROM proctoring_events;


-- =============================================================================
--  7. schema_version — migration history tracking
-- =============================================================================

CREATE TABLE IF NOT EXISTS schema_version
(
    version     String,
    description String,
    applied_at  DateTime64(3, 'UTC') DEFAULT now64(3),
    checksum    String               DEFAULT ''
)
ENGINE = MergeTree
ORDER BY (version, applied_at);


-- =============================================================================
--  8. evidence_fragments — video evidence metadata with forensic hash chain
--
--  Includes forensic ledger columns from migration 002.
--  TTL: 730 days (2 years) for legal compliance.
-- =============================================================================

CREATE TABLE IF NOT EXISTS evidence_fragments
(
    fragment_id     String,
    session_id      String,
    event_id        String,
    org_id          String,
    exam_id         String,
    student_id      String,

    sha256_hash     String,
    uri             String,
    size_bytes      Int64,
    content_type    LowCardinality(String),

    duration_sec    Float64,
    start_time      DateTime64(3, 'UTC'),
    end_time        DateTime64(3, 'UTC'),
    uploaded_at     DateTime64(3, 'UTC') DEFAULT now64(3),

    -- Forensic ledger (migration 002)
    sequence_num    UInt64  DEFAULT 0   COMMENT 'Monotonic sequence per session (0=legacy)',
    previous_hash   String  DEFAULT ''  COMMENT 'SHA-256 of previous record (GENESIS for first)',
    record_hash     String  DEFAULT ''  COMMENT 'SHA-256 chain hash: H(seq|prev_hash|fragment_id|sha256|uploaded_at)'

    , INDEX idx_ef_fragment_id  fragment_id TYPE bloom_filter(0.01) GRANULARITY 1
    , INDEX idx_ef_session_id   session_id  TYPE bloom_filter(0.01) GRANULARITY 1
    , INDEX idx_ef_event_id     event_id    TYPE bloom_filter(0.01) GRANULARITY 1
    , INDEX idx_ef_sha256       sha256_hash TYPE bloom_filter(0.01) GRANULARITY 1
    , INDEX idx_ef_student_id   student_id  TYPE bloom_filter(0.01) GRANULARITY 4
    , INDEX idx_ef_record_hash  record_hash TYPE bloom_filter(0.01) GRANULARITY 1
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(uploaded_at)
ORDER BY (org_id, session_id, uploaded_at)
TTL uploaded_at + INTERVAL 730 DAY
SETTINGS index_granularity = 8192;


-- =============================================================================
--  9. session_ai_summary — per-session AI inference aggregates (migration 003)
-- =============================================================================

CREATE TABLE IF NOT EXISTS session_ai_summary
(
    org_id               String,
    exam_id              String,
    session_id           String,
    student_id           String,
    avg_liveness_score   Float64,
    liveness_count       UInt64,
    min_face_similarity  Float32,
    avg_face_similarity  Float64,
    face_check_count     UInt64,
    head_pose_anomalies  UInt64,
    face_occluded_count  UInt64,
    speech_segments      UInt64,
    whisper_detections   UInt64,
    second_speaker_count UInt64,
    avg_audio_rms_db     Float64,
    audio_sample_count   UInt64,
    first_event_time     SimpleAggregateFunction(min, DateTime64(3, 'UTC')),
    last_event_time      SimpleAggregateFunction(max, DateTime64(3, 'UTC'))
)
ENGINE = AggregatingMergeTree()
ORDER BY (org_id, exam_id, session_id)
TTL last_event_time + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

CREATE MATERIALIZED VIEW IF NOT EXISTS session_ai_summary_mv
TO session_ai_summary
AS SELECT
    org_id,
    exam_id,
    session_id,
    student_id,
    avgIf(liveness_score,  liveness_score  >= 0)  AS avg_liveness_score,
    countIf(liveness_score >= 0)                  AS liveness_count,
    minIf(face_similarity,  face_similarity >= 0) AS min_face_similarity,
    avgIf(face_similarity,  face_similarity >= 0) AS avg_face_similarity,
    countIf(face_similarity >= 0)                 AS face_check_count,
    countIf(event_type = 'head_pose_anomaly')     AS head_pose_anomalies,
    countIf(event_type = 'face_occluded')         AS face_occluded_count,
    countIf(vad_active = 1)                       AS speech_segments,
    countIf(event_type = 'whisper_detected')      AS whisper_detections,
    countIf(event_type = 'second_speaker_detected') AS second_speaker_count,
    avgIf(audio_rms_db, audio_rms_db > -100)      AS avg_audio_rms_db,
    countIf(audio_rms_db > -100)                  AS audio_sample_count,
    min(server_timestamp)                         AS first_event_time,
    max(server_timestamp)                         AS last_event_time
FROM proctoring_events
GROUP BY org_id, exam_id, session_id, student_id;


-- =============================================================================
--  Schema version entries
-- =============================================================================

INSERT INTO schema_version (version, description) VALUES
    ('1.0.0', 'Initial schema: proctoring_events + 5 materialized views + data skipping indices'),
    ('1.1.0', 'Evidence storage: evidence_fragments with 730-day TTL'),
    ('2.1.0', 'Forensic ledger: hash chaining columns on evidence_fragments'),
    ('3.0.0', 'AI inference columns: head pose, liveness, face embedding, audio analysis'),
    ('4.0.0', 'Reliability hardening: ReplacingMergeTree, event_id in ORDER BY, explicit TTL DELETE'),
    ('5.0.0', 'HLC causal_sequence column for deterministic event ordering');
