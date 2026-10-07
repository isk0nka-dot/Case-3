// Package entity defines the core domain entities for the Argus AI platform.
//
// EvidenceFragment represents an immutable video evidence clip captured during
// a proctoring session. Each fragment is linked to a specific violation event
// and contains a SHA-256 hash for tamper-evidence verification.
package entity

import (
	"fmt"
	"time"
)

// EvidenceFragment is an immutable record of a video evidence clip captured
// from a proctoring session's ring buffer when a violation event is detected.
//
// Lifecycle:
//  1. Violation event triggers ring buffer snapshot (30s before + 10s after).
//  2. Segments are concatenated into a single WebM stream.
//  3. SHA-256 hash is computed during upload (single-pass via TeeReader).
//  4. Fragment is uploaded to MinIO with GOVERNANCE retention (WORM).
//  5. Chain of custody record is written to ClickHouse + Kafka.
//
// Once created, evidence fragments are never modified or deleted (WORM policy).
type EvidenceFragment struct {
	// FragmentID is a unique identifier for this evidence clip (UUIDv7).
	FragmentID string `json:"fragmentId"`

	// SessionID links this evidence to the proctoring session.
	SessionID string `json:"sessionId"`

	// EventID links this evidence to the violation event that triggered capture.
	EventID string `json:"eventId"`

	// OrgID is the organization (tenant) that owns this evidence.
	OrgID string `json:"orgId"`

	// ExamID links this evidence to the specific exam.
	ExamID string `json:"examId"`

	// StudentID links this evidence to the student being proctored.
	StudentID string `json:"studentId"`

	// SHA256Hash is the cryptographic hash of the evidence file contents.
	// Computed during upload via TeeReader for single-pass integrity.
	// Format: lowercase hex string (64 characters).
	SHA256Hash string `json:"sha256Hash"`

	// URI is the S3 storage location of the evidence file.
	// Format: "s3://argus-evidence/{org_id}/{session_id}/{fragment_id}.webm"
	URI string `json:"uri"`

	// SizeBytes is the file size of the evidence fragment.
	SizeBytes int64 `json:"sizeBytes"`

	// ContentType is the MIME type of the evidence file.
	// Typically "video/webm" or "video/mp4".
	ContentType string `json:"contentType"`

	// DurationSec is the duration of the evidence clip in seconds.
	// Typically 30-40 seconds (pre-capture + post-capture window).
	DurationSec float64 `json:"durationSec"`

	// StartTime is the timestamp of the earliest video frame in the clip.
	StartTime time.Time `json:"startTime"`

	// EndTime is the timestamp of the latest video frame in the clip.
	EndTime time.Time `json:"endTime"`

	// CreatedAt is when this evidence record was created (server-side).
	CreatedAt time.Time `json:"createdAt"`
}

// Validate checks that all required fields are populated and internally
// consistent. Returns an error describing the first validation failure.
func (f *EvidenceFragment) Validate() error {
	if f.FragmentID == "" {
		return fmt.Errorf("evidence: fragment_id is required")
	}
	if f.SessionID == "" {
		return fmt.Errorf("evidence: session_id is required")
	}
	if f.EventID == "" {
		return fmt.Errorf("evidence: event_id is required")
	}
	if f.OrgID == "" {
		return fmt.Errorf("evidence: org_id is required")
	}
	if f.ExamID == "" {
		return fmt.Errorf("evidence: exam_id is required")
	}
	if f.StudentID == "" {
		return fmt.Errorf("evidence: student_id is required")
	}
	if f.ContentType == "" {
		return fmt.Errorf("evidence: content_type is required")
	}
	if f.StartTime.IsZero() {
		return fmt.Errorf("evidence: start_time is required")
	}
	if f.EndTime.IsZero() {
		return fmt.Errorf("evidence: end_time is required")
	}
	if !f.EndTime.After(f.StartTime) {
		return fmt.Errorf("evidence: end_time must be after start_time")
	}
	return nil
}
