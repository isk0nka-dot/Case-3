// Package port defines the interfaces (ports) that the application layer
// requires from infrastructure. This file defines the ChunkAssembler port
// for managing chunked evidence uploads from the browser client.
package port

import (
	"context"
	"io"
)

// ChunkMeta describes a single chunk in a multi-part evidence upload.
// The browser client splits large binary evidence (JPEG snapshots, audio)
// into 256KB chunks and uploads them sequentially.
type ChunkMeta struct {
	// SessionID is the proctoring session this evidence belongs to.
	SessionID string
	// FragmentID is a unique identifier for the assembled evidence file.
	FragmentID string
	// ChunkIndex is the zero-based index of this chunk (0..TotalChunks-1).
	ChunkIndex int
	// TotalChunks is the total number of chunks for this fragment.
	TotalChunks int
	// SHA256Chunk is the hex-encoded SHA-256 hash of this chunk's data.
	SHA256Chunk string
	// SHA256Total is the hex-encoded SHA-256 hash of the complete file.
	SHA256Total string
	// ContentType is the MIME type of the evidence (e.g., "image/jpeg").
	ContentType string
	// OrgID is the organization identifier for scoping.
	OrgID string
	// ExamID is the exam identifier.
	ExamID string
	// StudentID is the student identifier.
	StudentID string
}

// ChunkStatus represents the current state of a chunked upload.
type ChunkStatus struct {
	// FragmentID is the unique identifier for this fragment.
	FragmentID string `json:"fragmentId"`
	// TotalChunks is the total expected number of chunks.
	TotalChunks int `json:"totalChunks"`
	// ReceivedChunks is the number of chunks received so far.
	ReceivedChunks int `json:"receivedChunks"`
	// Complete is true when all chunks have been received and assembled.
	Complete bool `json:"complete"`
	// URI is the S3 URI of the assembled file (set when complete).
	URI string `json:"uri,omitempty"`
	// SHA256 is the verified hash of the assembled file (set when complete).
	SHA256 string `json:"sha256,omitempty"`
	// SizeBytes is the total size of the assembled file (set when complete).
	SizeBytes int64 `json:"sizeBytes,omitempty"`
}

// ChunkAssembler manages the lifecycle of chunked evidence uploads.
//
// Upload flow:
//  1. Client sends N chunks via ReceiveChunk(), each with metadata.
//  2. Each chunk is SHA-256 verified and buffered.
//  3. When the last chunk arrives (complete=true), Assemble() is called.
//  4. Assembly concatenates chunks, verifies the total SHA-256, and
//     uploads the result to MinIO via EvidenceStore.
//  5. Incomplete fragments are cleaned up after a TTL.
//
// Design principles:
//   - Idempotent: re-uploading a chunk with the same (fragmentID, chunkIndex)
//     overwrites the previous data safely.
//   - Bounded memory: rejects fragments that exceed MaxFragmentSize.
//   - TTL cleanup: background goroutine removes stale fragments.
type ChunkAssembler interface {
	// ReceiveChunk stores a single chunk and verifies its SHA-256 hash.
	// Returns true if all chunks for this fragment have been received
	// (ready for assembly).
	ReceiveChunk(ctx context.Context, meta ChunkMeta, data io.Reader) (complete bool, err error)

	// Assemble concatenates all received chunks for a fragment,
	// verifies the total SHA-256 hash, and uploads to MinIO.
	// Returns the S3 URI, verified hash, and total size.
	// Should only be called after ReceiveChunk returns complete=true.
	Assemble(ctx context.Context, fragmentID string) (uri string, sha256 string, sizeBytes int64, err error)

	// GetStatus returns the current upload status for a fragment.
	GetStatus(ctx context.Context, fragmentID string) (*ChunkStatus, error)

	// Cleanup removes buffered chunks for a fragment.
	// Called on timeout, error, or after successful assembly.
	Cleanup(ctx context.Context, fragmentID string) error

	// Close stops the background cleanup goroutine and releases resources.
	Close() error
}
