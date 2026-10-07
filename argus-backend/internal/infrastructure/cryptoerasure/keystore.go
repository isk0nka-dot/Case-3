package cryptoerasure

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// KeyStore manages per-student encryption keys in PostgreSQL.
// Each student has a DEK (Data Encryption Key) wrapped with a system-wide KEK.
//
// Table: crypto_key_store
//
//	student_id  TEXT PRIMARY KEY
//	wrapped_dek BYTEA NOT NULL      -- AES-256-GCM(DEK, KEK)
//	created_at  TIMESTAMPTZ NOT NULL
//	rotated_at  TIMESTAMPTZ
type KeyStore struct {
	db     *sql.DB
	kek    []byte // system-wide Key Encryption Key (from config/env)
	logger *zap.Logger

	// In-memory cache: student_id → unwrapped DEK.
	// Cache is invalidated on rotation or erasure.
	mu    sync.RWMutex
	cache map[string][]byte
}

// KeyStoreConfig holds configuration for the crypto key store.
type KeyStoreConfig struct {
	// KEK is the system-wide Key Encryption Key (hex-encoded, 64 chars = 32 bytes).
	// CRITICAL: Store in a secrets manager (Vault, AWS KMS). Never commit to code.
	KEKHex string
}

// NewKeyStore creates a new KeyStore. It ensures the crypto_key_store table
// exists (idempotent CREATE IF NOT EXISTS).
func NewKeyStore(db *sql.DB, cfg KeyStoreConfig, logger *zap.Logger) (*KeyStore, error) {
	if len(cfg.KEKHex) == 0 {
		return nil, fmt.Errorf("cryptoerasure: KEK must not be empty")
	}

	kek, err := hex.DecodeString(cfg.KEKHex)
	if err != nil {
		return nil, fmt.Errorf("cryptoerasure: invalid KEK hex: %w", err)
	}
	if len(kek) != 32 {
		return nil, fmt.Errorf("cryptoerasure: KEK must be 32 bytes (AES-256), got %d", len(kek))
	}

	ks := &KeyStore{
		db:     db,
		kek:    kek,
		logger: logger.Named("crypto_keystore"),
		cache:  make(map[string][]byte),
	}

	// Ensure table exists.
	if err := ks.ensureTable(context.Background()); err != nil {
		return nil, err
	}

	logger.Info("crypto key store initialised")
	return ks, nil
}

// ensureTable creates the crypto_key_store table if it does not exist.
func (ks *KeyStore) ensureTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS crypto_key_store (
			student_id  TEXT        PRIMARY KEY,
			wrapped_dek BYTEA       NOT NULL,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			rotated_at  TIMESTAMPTZ
		);
		CREATE INDEX IF NOT EXISTS idx_crypto_key_store_created
			ON crypto_key_store (created_at);
	`
	_, err := ks.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("cryptoerasure: failed to create key_store table: %w", err)
	}
	return nil
}

// GetOrCreateDEK returns the DEK for a student, creating one if it doesn't exist.
// Thread-safe: uses cache with fallback to DB.
func (ks *KeyStore) GetOrCreateDEK(ctx context.Context, studentID string) ([]byte, error) {
	// Fast path: cache hit.
	ks.mu.RLock()
	if dek, ok := ks.cache[studentID]; ok {
		ks.mu.RUnlock()
		return dek, nil
	}
	ks.mu.RUnlock()

	// Slow path: DB lookup.
	dek, err := ks.loadDEK(ctx, studentID)
	if err == nil {
		ks.mu.Lock()
		ks.cache[studentID] = dek
		ks.mu.Unlock()
		return dek, nil
	}

	// Not found — generate new DEK.
	dek, err = GenerateKey()
	if err != nil {
		return nil, err
	}

	wrappedDEK, err := WrapKey(dek, ks.kek)
	if err != nil {
		return nil, err
	}

	_, err = ks.db.ExecContext(ctx,
		`INSERT INTO crypto_key_store (student_id, wrapped_dek, created_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (student_id) DO NOTHING`,
		studentID, wrappedDEK, time.Now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("cryptoerasure: failed to store DEK for student %s: %w", studentID, err)
	}

	// Another goroutine might have raced — reload from DB to be safe.
	dek, err = ks.loadDEK(ctx, studentID)
	if err != nil {
		return nil, err
	}

	ks.mu.Lock()
	ks.cache[studentID] = dek
	ks.mu.Unlock()

	ks.logger.Debug("DEK created for student", zap.String("student_id", studentID))
	return dek, nil
}

// loadDEK reads and unwraps the DEK from PostgreSQL.
func (ks *KeyStore) loadDEK(ctx context.Context, studentID string) ([]byte, error) {
	var wrappedDEK []byte
	err := ks.db.QueryRowContext(ctx,
		`SELECT wrapped_dek FROM crypto_key_store WHERE student_id = $1`,
		studentID,
	).Scan(&wrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("cryptoerasure: DEK not found for student %s: %w", studentID, err)
	}

	dek, err := UnwrapKey(wrappedDEK, ks.kek)
	if err != nil {
		return nil, fmt.Errorf("cryptoerasure: failed to unwrap DEK for student %s: %w", studentID, err)
	}

	return dek, nil
}

// EraseStudent implements GDPR "right to be forgotten" by destroying the
// student's wrapped DEK. After this call, all data encrypted with the
// student's DEK is cryptographically irrecoverable.
//
// The actual ciphertext in MinIO and ClickHouse remains intact — the hash
// chain and forensic audit trail are preserved. Only the key needed to
// decrypt that data is destroyed.
func (ks *KeyStore) EraseStudent(ctx context.Context, studentID string) error {
	result, err := ks.db.ExecContext(ctx,
		`DELETE FROM crypto_key_store WHERE student_id = $1`,
		studentID,
	)
	if err != nil {
		return fmt.Errorf("cryptoerasure: failed to erase student %s: %w", studentID, err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		ks.logger.Warn("crypto-erasure: no key found for student (already erased?)",
			zap.String("student_id", studentID),
		)
	}

	// Invalidate cache.
	ks.mu.Lock()
	delete(ks.cache, studentID)
	ks.mu.Unlock()

	ks.logger.Info("GDPR crypto-erasure completed",
		zap.String("student_id", studentID),
		zap.Int64("keys_destroyed", rows),
	)

	return nil
}

// RotateStudentDEK generates a new DEK for a student and re-wraps it.
// NOTE: Existing ciphertext encrypted with the old DEK is NOT re-encrypted.
// New data will use the new DEK. For full re-encryption, a separate
// migration job is needed.
func (ks *KeyStore) RotateStudentDEK(ctx context.Context, studentID string) error {
	newDEK, err := GenerateKey()
	if err != nil {
		return err
	}

	wrappedDEK, err := WrapKey(newDEK, ks.kek)
	if err != nil {
		return err
	}

	result, err := ks.db.ExecContext(ctx,
		`UPDATE crypto_key_store
		 SET wrapped_dek = $1, rotated_at = $2
		 WHERE student_id = $3`,
		wrappedDEK, time.Now().UTC(), studentID,
	)
	if err != nil {
		return fmt.Errorf("cryptoerasure: failed to rotate DEK for student %s: %w", studentID, err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("cryptoerasure: student %s not found for rotation", studentID)
	}

	// Update cache.
	ks.mu.Lock()
	ks.cache[studentID] = newDEK
	ks.mu.Unlock()

	ks.logger.Info("DEK rotated for student", zap.String("student_id", studentID))
	return nil
}

// StudentKeyExists checks if a student has an active encryption key.
// Returns false if the key has been erased (GDPR erasure completed).
func (ks *KeyStore) StudentKeyExists(ctx context.Context, studentID string) (bool, error) {
	var count int
	err := ks.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM crypto_key_store WHERE student_id = $1`,
		studentID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("cryptoerasure: failed to check key existence: %w", err)
	}
	return count > 0, nil
}
