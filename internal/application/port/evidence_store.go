// Package port defines the interfaces (ports) that the application layer
// requires from infrastructure. This file defines the EvidenceStore port
// for S3-compatible object storage of video evidence fragments.
package port

import (
	"context"
	"io"
	"time"

	"github.com/argus-ai/event-collector/internal/domain/entity"
)

// EvidenceStore is the outbound port for persisting video evidence fragments
// to S3-compatible object storage (MinIO). Implementations must guarantee
// immutability via S3 Object Lock (WORM) for legal compliance.
//
// Contract:
//   - Upload must compute SHA-256 during the upload (single-pass via TeeReader).
//   - Upload must set GOVERNANCE retention on the object after successful upload.
//   - PresignURL must return a time-limited GET URL for secure evidence retrieval.
//   - Implementations must handle retries internally.
type EvidenceStore interface {
	// Upload stores an evidence fragment in object storage.
	// The reader provides the raw video data (WebM/MP4).
	// Returns the computed SHA-256 hash, S3 URI, and size in bytes.
	// The S3 key format is: {org_id}/{session_id}/{fragment_id}.webm
	Upload(ctx context.Context, fragment *entity.EvidenceFragment, reader io.Reader) (sha256Hash string, uri string, sizeBytes int64, err error)

	// PresignURL generates a time-limited presigned GET URL for downloading
	// an evidence fragment. The URL expires after the given TTL.
	// This prevents unauthorized permanent access to evidence files.
	PresignURL(ctx context.Context, uri string, ttl time.Duration) (string, error)

	// VerifyIntegrity re-downloads the object and computes SHA-256 to verify
	// against the stored hash. Returns true if the hashes match.
	VerifyIntegrity(ctx context.Context, uri string, expectedHash string) (bool, error)

	// Close releases resources held by the store.
	Close() error
}
