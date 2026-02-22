package cryptoerasure

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/domain/entity"
)

// EvidenceEncryptor wraps evidence data with per-student envelope encryption
// before it reaches the underlying store. It reads the full evidence payload
// into memory, encrypts it with the student's DEK, and passes the ciphertext
// to the delegate store's Upload method.
//
// This is a decorator — the underlying store remains unchanged.
type EvidenceEncryptor struct {
	keyStore *KeyStore
	logger   *zap.Logger
}

// NewEvidenceEncryptor creates a new evidence encryption decorator.
func NewEvidenceEncryptor(ks *KeyStore, logger *zap.Logger) *EvidenceEncryptor {
	return &EvidenceEncryptor{
		keyStore: ks,
		logger:   logger.Named("evidence_encryptor"),
	}
}

// EncryptReader reads all data from reader, encrypts it with the student's
// DEK, and returns a new reader over the ciphertext along with the
// ciphertext size. The caller can pass this to the underlying store.
//
// Returns:
//   - encryptedReader: io.Reader over AES-256-GCM ciphertext
//   - size: byte length of the ciphertext
//   - error: if encryption fails or student key cannot be obtained
func (e *EvidenceEncryptor) EncryptReader(ctx context.Context, studentID string, reader io.Reader) (io.Reader, int64, error) {
	// Read all plaintext into memory. Evidence fragments are typically
	// <50MB which is within acceptable memory bounds.
	plaintext, err := io.ReadAll(reader)
	if err != nil {
		return nil, 0, fmt.Errorf("evidence_encryptor: failed to read evidence: %w", err)
	}

	// Get (or create) the student's DEK.
	dek, err := e.keyStore.GetOrCreateDEK(ctx, studentID)
	if err != nil {
		return nil, 0, fmt.Errorf("evidence_encryptor: failed to get DEK: %w", err)
	}

	// Encrypt with AES-256-GCM.
	ciphertext, err := Encrypt(plaintext, dek)
	if err != nil {
		return nil, 0, fmt.Errorf("evidence_encryptor: encryption failed: %w", err)
	}

	e.logger.Debug("evidence encrypted",
		zap.String("student_id", studentID),
		zap.Int("plaintext_bytes", len(plaintext)),
		zap.Int("ciphertext_bytes", len(ciphertext)),
	)

	return bytes.NewReader(ciphertext), int64(len(ciphertext)), nil
}

// DecryptEvidence decrypts ciphertext using the student's DEK.
// Returns ErrKeyDestroyed if the student's key has been erased (GDPR).
func (e *EvidenceEncryptor) DecryptEvidence(ctx context.Context, studentID string, ciphertext []byte) ([]byte, error) {
	// Check if key exists (may have been erased).
	exists, err := e.keyStore.StudentKeyExists(ctx, studentID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrKeyDestroyed
	}

	dek, err := e.keyStore.GetOrCreateDEK(ctx, studentID)
	if err != nil {
		return nil, fmt.Errorf("evidence_encryptor: failed to get DEK: %w", err)
	}

	plaintext, err := Decrypt(ciphertext, dek)
	if err != nil {
		return nil, fmt.Errorf("evidence_encryptor: decryption failed: %w", err)
	}

	return plaintext, nil
}

// EncryptField encrypts a small field (e.g., a ClickHouse column value)
// using the student's DEK. Suitable for event payload fields, labels, etc.
func (e *EvidenceEncryptor) EncryptField(ctx context.Context, studentID string, plaintext []byte) ([]byte, error) {
	dek, err := e.keyStore.GetOrCreateDEK(ctx, studentID)
	if err != nil {
		return nil, err
	}
	return Encrypt(plaintext, dek)
}

// ErrKeyDestroyed indicates that the student's encryption key has been
// destroyed via GDPR crypto-erasure. The data is irrecoverable.
var ErrKeyDestroyed = fmt.Errorf("cryptoerasure: student key destroyed (GDPR erasure)")

// EncryptEvidenceFragment encrypts an evidence fragment in place before
// storage. This is the primary integration point — called from the
// evidence upload path.
func (e *EvidenceEncryptor) EncryptEvidenceFragment(
	ctx context.Context,
	fragment *entity.EvidenceFragment,
	reader io.Reader,
) (io.Reader, int64, error) {
	return e.EncryptReader(ctx, fragment.StudentID, reader)
}
