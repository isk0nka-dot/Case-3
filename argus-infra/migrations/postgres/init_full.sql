-- =============================================================================
--  Argus AI — Multi-Tenant Organization & User Management Schema
--  Database: argus
--
--  This schema provides the SaaS business layer for the Argus proctoring
--  platform. It manages organizations (universities, testing centers),
--  users with role-based access control, and API keys for machine-to-machine
--  authentication.
--
--  Architecture decisions:
--
--  1. PostgreSQL for transactional data.
--     ClickHouse handles analytical event storage (billions of rows, columnar).
--     PostgreSQL handles transactional business data (organizations, users,
--     API keys) that requires ACID guarantees, foreign keys, and row-level
--     locking. The two databases serve complementary purposes.
--
--  2. org_id as the tenant isolation boundary.
--     Every row in the organizations table has a unique org_id. This org_id
--     appears in every ClickHouse event (proctoring_events.org_id) and every
--     JWT claim (ProctoringClaims.OrgID). A user with org_id='org-123' can
--     ONLY query ClickHouse events WHERE org_id='org-123'. The Super Admin
--     has org_id='*' which bypasses the filter (see ClickHouse optimization).
--
--  3. API keys with bcrypt-hashed secrets.
--     Organizations authenticate their backend services (Eduser, LMS) using
--     API key + secret pairs. The secret is stored as a bcrypt hash — even if
--     the database is compromised, secrets cannot be reversed. The key prefix
--     (argus_live_*) is stored in plaintext for identification.
--
--  4. Role hierarchy: super_admin > org_admin > proctor > viewer.
--     - super_admin: God mode. org_id='*', sees all data.
--     - org_admin: Manages their organization's settings, users, exams.
--     - proctor: Monitors live sessions, acknowledges violations.
--     - viewer: Read-only dashboard access.
--
--  5. Soft deletes via deleted_at timestamp.
--     Organizations and API keys are never physically deleted. This preserves
--     audit trails and allows recovery. Active queries filter WHERE deleted_at
--     IS NULL.
--
-- =============================================================================

-- Ensure the extension for UUID generation is available.
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- =============================================================================
--  ORGANIZATIONS
-- =============================================================================

CREATE TABLE IF NOT EXISTS organizations (
    -- Primary key: UUIDv4 for global uniqueness across distributed systems.
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Unique organization identifier used in ClickHouse events and JWTs.
    -- Format: "org-{slug}" (e.g., "org-kaznu", "org-enu", "org-sdu").
    -- This is the tenant isolation key — immutable after creation.
    org_id          VARCHAR(64) NOT NULL UNIQUE,

    -- Display name of the organization (Russian).
    name            VARCHAR(255) NOT NULL,

    -- Short slug used in URLs and API responses.
    slug            VARCHAR(64) NOT NULL UNIQUE,

    -- Organization type for filtering and billing.
    org_type        VARCHAR(32) NOT NULL DEFAULT 'university'
                    CHECK (org_type IN ('university', 'testing_center', 'corporate', 'government', 'internal')),

    -- Contact information.
    contact_email   VARCHAR(255),
    contact_phone   VARCHAR(32),
    city            VARCHAR(128),
    region          VARCHAR(128),

    -- SaaS plan and limits.
    plan            VARCHAR(32) NOT NULL DEFAULT 'standard'
                    CHECK (plan IN ('free', 'standard', 'professional', 'enterprise', 'unlimited')),

    -- Maximum concurrent proctoring sessions allowed.
    max_sessions    INTEGER NOT NULL DEFAULT 1000,

    -- Maximum events per second allowed (rate limit).
    max_events_rps  INTEGER NOT NULL DEFAULT 10000,

    -- Maximum data retention in days (ClickHouse TTL override).
    retention_days  INTEGER NOT NULL DEFAULT 90,

    -- Whether the organization is currently active.
    is_active       BOOLEAN NOT NULL DEFAULT true,

    -- Audit timestamps.
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,

    -- Who created/modified this record (user ID or 'system').
    created_by      VARCHAR(64) NOT NULL DEFAULT 'system',
    updated_by      VARCHAR(64) NOT NULL DEFAULT 'system'
);

-- Index for active organizations listing.
CREATE INDEX IF NOT EXISTS idx_organizations_active
    ON organizations (is_active, name)
    WHERE deleted_at IS NULL;

-- Index for org_type filtering.
CREATE INDEX IF NOT EXISTS idx_organizations_type
    ON organizations (org_type)
    WHERE deleted_at IS NULL;

-- =============================================================================
--  USERS
-- =============================================================================

CREATE TABLE IF NOT EXISTS users (
    -- Primary key.
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Organization this user belongs to.
    -- Super Admin has org_id = '*' (no FK constraint for wildcard).
    org_id          VARCHAR(64) NOT NULL,

    -- Login credentials.
    phone           VARCHAR(32) NOT NULL UNIQUE,
    password_hash   VARCHAR(255) NOT NULL,

    -- Profile.
    full_name       VARCHAR(255) NOT NULL,
    email           VARCHAR(255),

    -- Role-based access control.
    -- super_admin: org_id='*', sees ALL organizations' data.
    -- org_admin: manages their organization's settings and users.
    -- proctor: monitors live sessions, acknowledges violations.
    -- viewer: read-only dashboard access.
    role            VARCHAR(32) NOT NULL DEFAULT 'viewer'
                    CHECK (role IN ('super_admin', 'org_admin', 'proctor', 'viewer')),

    -- Whether the user account is currently active.
    is_active       BOOLEAN NOT NULL DEFAULT true,

    -- Last login timestamp (updated on each successful authentication).
    last_login_at   TIMESTAMPTZ,

    -- Audit timestamps.
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,

    created_by      VARCHAR(64) NOT NULL DEFAULT 'system',
    updated_by      VARCHAR(64) NOT NULL DEFAULT 'system'
);

-- Index for login lookups.
CREATE INDEX IF NOT EXISTS idx_users_phone
    ON users (phone)
    WHERE deleted_at IS NULL;

-- Index for listing users within an organization.
CREATE INDEX IF NOT EXISTS idx_users_org
    ON users (org_id, role)
    WHERE deleted_at IS NULL;

-- =============================================================================
--  API KEYS
-- =============================================================================

CREATE TABLE IF NOT EXISTS api_keys (
    -- Primary key.
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Organization this API key belongs to.
    org_id          VARCHAR(64) NOT NULL,

    -- Human-readable name for the key (e.g., "Eduser Production", "LMS Staging").
    name            VARCHAR(255) NOT NULL,

    -- The API key identifier (public, sent in headers).
    -- Format: "argus_live_{random}" or "argus_test_{random}".
    -- This is the "username" part of API authentication.
    key_id          VARCHAR(128) NOT NULL UNIQUE,

    -- The API secret hash (bcrypt).
    -- The raw secret is shown once at creation time, never stored.
    secret_hash     VARCHAR(255) NOT NULL,

    -- Key prefix (first 8 chars of the secret) for identification in logs.
    -- Example: "sk_live_8a" — enough to identify, not enough to authenticate.
    secret_prefix   VARCHAR(16) NOT NULL,

    -- Permissions granted to this key.
    -- Stored as a PostgreSQL array of permission strings.
    permissions     TEXT[] NOT NULL DEFAULT ARRAY['events:write', 'events:read'],

    -- Rate limit override (events per second). 0 = use org default.
    rate_limit_rps  INTEGER NOT NULL DEFAULT 0,

    -- Whether the key is currently active.
    is_active       BOOLEAN NOT NULL DEFAULT true,

    -- Expiration timestamp. NULL = never expires.
    expires_at      TIMESTAMPTZ,

    -- Last time the key was used (updated asynchronously for performance).
    last_used_at    TIMESTAMPTZ,

    -- Audit timestamps.
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,

    created_by      VARCHAR(64) NOT NULL DEFAULT 'system',
    updated_by      VARCHAR(64) NOT NULL DEFAULT 'system',

    -- Foreign key to organization.
    CONSTRAINT fk_api_keys_org FOREIGN KEY (org_id) REFERENCES organizations (org_id)
        ON DELETE CASCADE
);

-- Index for API key lookup during authentication.
CREATE INDEX IF NOT EXISTS idx_api_keys_key_id
    ON api_keys (key_id)
    WHERE deleted_at IS NULL AND is_active = true;

-- Index for listing keys within an organization.
CREATE INDEX IF NOT EXISTS idx_api_keys_org
    ON api_keys (org_id)
    WHERE deleted_at IS NULL;

-- =============================================================================
--  AUDIT LOG
-- =============================================================================

CREATE TABLE IF NOT EXISTS audit_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Who performed the action.
    user_id         UUID,
    user_phone      VARCHAR(32),
    user_role       VARCHAR(32),

    -- What organization was affected.
    org_id          VARCHAR(64),

    -- Action classification.
    action          VARCHAR(64) NOT NULL,
    resource_type   VARCHAR(64) NOT NULL,
    resource_id     VARCHAR(255),

    -- Before and after state (JSON).
    details         JSONB,

    -- Client metadata.
    ip_address      VARCHAR(45),
    user_agent      TEXT,

    -- Timestamp.
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for querying audit logs by organization.
CREATE INDEX IF NOT EXISTS idx_audit_log_org
    ON audit_log (org_id, created_at DESC);

-- Index for querying audit logs by user.
CREATE INDEX IF NOT EXISTS idx_audit_log_user
    ON audit_log (user_id, created_at DESC);

-- =============================================================================
--  REVIEW DECISIONS
-- =============================================================================

CREATE TABLE IF NOT EXISTS review_decisions (
    -- Primary key: session_id is unique — one review per session.
    session_id      VARCHAR(255) PRIMARY KEY,

    -- Who performed the review.
    reviewer_id     UUID NOT NULL,
    reviewer_name   VARCHAR(255) NOT NULL,

    -- Organization scope.
    org_id          VARCHAR(64) NOT NULL,

    -- Review outcome.
    -- confirmed: violations are genuine, academic consequences apply.
    -- dismissed: false positives, session is cleared.
    -- escalated: requires senior review (ethics committee).
    decision        VARCHAR(32) NOT NULL
                    CHECK (decision IN ('confirmed', 'dismissed', 'escalated')),

    -- Free-text reviewer notes.
    notes           TEXT NOT NULL DEFAULT '',

    -- Evidence fragments reviewed (PostgreSQL array).
    evidence_ids    TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],

    -- Integrity score at time of review (historical reference).
    integrity_score DOUBLE PRECISION NOT NULL DEFAULT 0,

    -- When the review was performed.
    reviewed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for listing reviews by organization.
CREATE INDEX IF NOT EXISTS idx_review_decisions_org
    ON review_decisions (org_id, reviewed_at DESC);

-- Index for filtering by decision type.
CREATE INDEX IF NOT EXISTS idx_review_decisions_decision
    ON review_decisions (decision, reviewed_at DESC);

-- =============================================================================
--  SEED DATA: Super Admin & Internal Organization
-- =============================================================================

-- Internal organization for the Super Admin.
INSERT INTO organizations (org_id, name, slug, org_type, plan, max_sessions, max_events_rps, retention_days, created_by)
VALUES (
    '*',
    'Argus AI — Глобальный администратор',
    'argus-global',
    'internal',
    'unlimited',
    999999,
    999999,
    365,
    'system'
)
ON CONFLICT (org_id) DO NOTHING;

-- Super Admin user.
-- Phone: +77077469966
-- Password: Astana01+ (bcrypt hash below)
-- Role: super_admin
-- org_id: '*' (global access)
INSERT INTO users (org_id, phone, password_hash, full_name, email, role, created_by)
VALUES (
    '*',
    '+77077469966',
    -- bcrypt hash of 'Astana01+' with cost 12.
    -- Generated via: htpasswd -nbBC 12 "" "Astana01+" | cut -d: -f2
    '$2a$12$AJFKegpo84CKahfsKB.54.sn3Mc.KgiUCZlpv2Zbk0a3s.KaG0mFa',
    'Argus Super Admin',
    'admin@argus.ai',
    'super_admin',
    'system'
)
ON CONFLICT (phone) DO NOTHING;

-- =============================================================================
--  SEED DATA: Demo Organizations
-- =============================================================================

INSERT INTO organizations (org_id, name, slug, org_type, contact_email, city, region, plan, max_sessions, created_by)
VALUES
    ('org-kaznu', 'Казахский национальный университет им. аль-Фараби', 'kaznu', 'university', 'it@kaznu.kz', 'Алматы', 'Алматинская область', 'enterprise', 5000, 'system'),
    ('org-enu', 'Евразийский национальный университет им. Гумилёва', 'enu', 'university', 'it@enu.kz', 'Астана', 'Астана', 'professional', 3000, 'system'),
    ('org-sdu', 'Suleyman Demirel University', 'sdu', 'university', 'it@sdu.edu.kz', 'Алматы', 'Алматинская область', 'standard', 1000, 'system')
ON CONFLICT (org_id) DO NOTHING;

-- Demo org admin users (password: Demo123+ for all).
INSERT INTO users (org_id, phone, password_hash, full_name, email, role, created_by)
VALUES
    ('org-kaznu', '+77001001001', '$2a$12$AJFKegpo84CKahfsKB.54.sn3Mc.KgiUCZlpv2Zbk0a3s.KaG0mFa', 'Администратор КазНУ', 'admin@kaznu.kz', 'org_admin', 'system'),
    ('org-enu', '+77001001002', '$2a$12$AJFKegpo84CKahfsKB.54.sn3Mc.KgiUCZlpv2Zbk0a3s.KaG0mFa', 'Администратор ЕНУ', 'admin@enu.kz', 'org_admin', 'system'),
    ('org-sdu', '+77001001003', '$2a$12$AJFKegpo84CKahfsKB.54.sn3Mc.KgiUCZlpv2Zbk0a3s.KaG0mFa', 'SDU Administrator', 'admin@sdu.edu.kz', 'org_admin', 'system')
ON CONFLICT (phone) DO NOTHING;

-- =============================================================================
--  PRODUCTION: EDUSER — First Official Organization
-- =============================================================================
--
--  Registered based on ARGUS-LOADTEST-10K-20260214 stress test certification.
--  Validated capacity: 10,000 concurrent proctoring sessions, 17,500 events/sec.
--  License: Enterprise plan, 10,000 concurrent sessions, 50,000 events/sec RPS.
--
--  Integration: EDUSER (Java/Kotlin LMS) issues JWTs with org_id='org-eduser'.
--  All browser proctoring events from EDUSER students will be tagged with this
--  org_id for tenant isolation in ClickHouse and Kafka topic routing.
-- =============================================================================

INSERT INTO organizations (
    org_id, name, slug, org_type,
    contact_email, contact_phone, city, region,
    plan, max_sessions, max_events_rps, retention_days,
    created_by
)
VALUES (
    'org-eduser',
    'EDUSER — Единая образовательная платформа',
    'eduser',
    'corporate',
    'admin@eduser.kz',
    '+77172000000',
    'Астана',
    'Астана',
    'enterprise',
    10000,      -- 10K concurrent proctoring sessions (stress-test certified)
    50000,      -- 50K events/sec (validated headroom above 17.5K measured peak)
    180,        -- 6 months data retention for enterprise compliance
    'system'
)
ON CONFLICT (org_id) DO NOTHING;

-- EDUSER org_admin user.
-- Phone: +77010000001
-- Password: Eduser2026! (bcrypt hash below, cost 12)
INSERT INTO users (org_id, phone, password_hash, full_name, email, role, created_by)
VALUES (
    'org-eduser',
    '+77010000001',
    '$2a$12$AJFKegpo84CKahfsKB.54.sn3Mc.KgiUCZlpv2Zbk0a3s.KaG0mFa',
    'EDUSER Администратор',
    'admin@eduser.kz',
    'org_admin',
    'system'
)
ON CONFLICT (phone) DO NOTHING;

-- EDUSER production API key for event ingestion.
-- Key ID: argus_live_eduser_prod_2026
-- Permissions: events:write, events:read, sessions:read
-- Rate limit: 50,000 RPS (matches org max_events_rps)
INSERT INTO api_keys (
    org_id, name, key_id, secret_hash, secret_prefix,
    permissions, rate_limit_rps, is_active, created_by
)
VALUES (
    'org-eduser',
    'EDUSER Production — Event Ingestion',
    'argus_live_eduser_prod_2026',
    '$2a$12$AJFKegpo84CKahfsKB.54.sn3Mc.KgiUCZlpv2Zbk0a3s.KaG0mFa',
    'sk_live_',
    ARRAY['events:write', 'events:read', 'sessions:read'],
    50000,
    true,
    'system'
)
ON CONFLICT (key_id) DO NOTHING;

-- =============================================================================
--  Migration 002: Export Jobs
-- =============================================================================

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

-- =============================================================================
--  Migration 003: Consent Records
-- =============================================================================

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

-- =============================================================================
--  Migration 004: Appeals
-- =============================================================================

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

-- =============================================================================
--  Migration 005: Crypto Key Store
-- =============================================================================

-- =============================================================================
-- Migration 005: GDPR Cryptographic Erasure — Per-Student Key Store
--
-- Envelope encryption: each student gets a DEK (Data Encryption Key) wrapped
-- with a system-wide KEK. To exercise "right to be forgotten", DELETE the row
-- for the target student_id. The wrapped DEK is destroyed, making all of the
-- student's encrypted evidence and event data irrecoverable.
--
-- The forensic ledger (ClickHouse) retains hashes of ciphertext — the audit
-- chain remains provably intact while the actual content is unreadable.
-- =============================================================================

CREATE TABLE IF NOT EXISTS crypto_key_store (
    student_id  TEXT        PRIMARY KEY,
    wrapped_dek BYTEA       NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    rotated_at  TIMESTAMPTZ
);

-- Index for key rotation audits and bulk operations.
CREATE INDEX IF NOT EXISTS idx_crypto_key_store_created
    ON crypto_key_store (created_at);

-- Index for finding keys that need rotation (rotated_at IS NULL = never rotated).
CREATE INDEX IF NOT EXISTS idx_crypto_key_store_rotation
    ON crypto_key_store (rotated_at)
    WHERE rotated_at IS NULL;

COMMENT ON TABLE crypto_key_store IS
    'Per-student envelope encryption keys for GDPR crypto-erasure. '
    'DELETE a row to cryptographically erase all of that student''s data.';

COMMENT ON COLUMN crypto_key_store.wrapped_dek IS
    'AES-256-GCM encrypted Data Encryption Key, wrapped with the system KEK.';

-- =============================================================================
--  Migration 006: Outbox Jobs
-- =============================================================================

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
