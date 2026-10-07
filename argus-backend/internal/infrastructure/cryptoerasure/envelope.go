// Package cryptoerasure implements GDPR-compliant cryptographic erasure
// via envelope encryption. Each student gets a unique Data Encryption Key
// (DEK) that is itself wrapped with a Key Encryption Key (KEK). To exercise
// the "right to be forgotten", the KEK is destroyed — making all of the
// student's data irrecoverable while keeping the hash chain and forensic
// ledger intact (hashes of ciphertext, not plaintext).
//
// Envelope encryption flow:
//
//	┌─────────┐     wrap(DEK, KEK)     ┌──────────────┐
//	│ DEK     │ ──────────────────────▶ │ wrapped_dek  │ → stored in DB
//	│ (AES)   │                         └──────────────┘
//	└─────────┘
//	     │
//	     ▼  encrypt(plaintext, DEK)
//	┌──────────────┐
//	│  ciphertext   │ → stored in MinIO / ClickHouse
//	└──────────────┘
//
// Crypto-erasure:
//
//	DELETE FROM key_store WHERE student_id = ?
//	→ wrapped_dek destroyed → DEK unrecoverable → ciphertext is gibberish
package cryptoerasure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// Encrypt encrypts plaintext using AES-256-GCM with the given key.
// Returns nonce || ciphertext (nonce is prepended to the ciphertext).
func Encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cryptoerasure: aes.NewCipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cryptoerasure: cipher.NewGCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("cryptoerasure: nonce generation: %w", err)
	}

	// Seal appends ciphertext+tag to nonce, producing: nonce || ciphertext || tag
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts data produced by Encrypt (nonce || ciphertext || tag).
func Decrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cryptoerasure: aes.NewCipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cryptoerasure: cipher.NewGCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("cryptoerasure: ciphertext too short (len=%d, nonceSize=%d)", len(ciphertext), nonceSize)
	}

	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("cryptoerasure: decryption failed (key destroyed or data corrupted): %w", err)
	}

	return plaintext, nil
}

// GenerateKey generates a cryptographically random AES-256 key (32 bytes).
func GenerateKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("cryptoerasure: key generation: %w", err)
	}
	return key, nil
}

// WrapKey encrypts a DEK with a KEK using AES-256-GCM (envelope wrap).
func WrapKey(dek, kek []byte) ([]byte, error) {
	return Encrypt(dek, kek)
}

// UnwrapKey decrypts a wrapped DEK using the KEK.
func UnwrapKey(wrappedDEK, kek []byte) ([]byte, error) {
	return Decrypt(wrappedDEK, kek)
}
