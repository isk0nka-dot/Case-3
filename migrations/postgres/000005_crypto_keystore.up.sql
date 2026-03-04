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
