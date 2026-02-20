// Package entity defines core domain entities for the Argus AI platform.
//
// ExportJob represents a bulk evidence export request. Organizations can
// request TAR.GZ archives of session evidence for physical media delivery,
// legal proceedings, or long-term archival.
//
// Lifecycle:
//  1. Admin creates export job via POST /api/v1/export/bulk.
//  2. Background worker picks up 'pending' jobs (polls every 5s).
//  3. Worker downloads evidence from MinIO, queries violations from ClickHouse.
//  4. Worker builds forensic manifest.json + TAR.GZ archive.
//  5. Archive uploaded to MinIO 'argus-exports' bucket.
//  6. Presigned URL generated with configurable TTL (default 24h).
//  7. Job status updated to 'completed' with download URL.
package entity

import "time"

// ExportStatus represents the lifecycle state of a bulk export job.
type ExportStatus string

const (
	ExportStatusPending    ExportStatus = "pending"
	ExportStatusProcessing ExportStatus = "processing"
	ExportStatusCompleted  ExportStatus = "completed"
	ExportStatusFailed     ExportStatus = "failed"
	ExportStatusExpired    ExportStatus = "expired"
)

// ExportJob is an immutable record of a bulk evidence export request.
// Once completed, the archive is stored in MinIO with Object Lock.
type ExportJob struct {
	// ID is a unique identifier for this export job (UUIDv7).
	ID string `json:"id"`

	// OrgID is the organization that owns this export.
	OrgID string `json:"orgId"`

	// RequestedBy is the user ID who initiated the export.
	RequestedBy string `json:"requestedBy"`

	// SessionIDs is the list of proctoring sessions to include.
	SessionIDs []string `json:"sessionIds"`

	// Status tracks the export job lifecycle.
	Status ExportStatus `json:"status"`

	// ArchiveURI is the S3 location of the completed TAR.GZ archive.
	// Format: "s3://argus-exports/{org_id}/{export_id}.tar.gz"
	ArchiveURI string `json:"archiveUri,omitempty"`

	// ManifestURI is the S3 location of the forensic manifest.
	// Format: "s3://argus-exports/{org_id}/{export_id}_manifest.json"
	ManifestURI string `json:"manifestUri,omitempty"`

	// SHA256Archive is the SHA-256 hash of the completed archive file.
	SHA256Archive string `json:"sha256Archive,omitempty"`

	// PresignTTLSec is the time-to-live in seconds for the presigned download URL.
	// Default: 86400 (24 hours).
	PresignTTLSec int `json:"presignTtlSec"`

	// DownloadURL is the presigned URL for downloading the archive.
	// Only populated when status is 'completed' and not expired.
	DownloadURL string `json:"downloadUrl,omitempty"`

	// ErrorMessage contains the failure reason if status is 'failed'.
	ErrorMessage string `json:"errorMessage,omitempty"`

	// TotalSizeBytes is the total size of the archive in bytes.
	TotalSizeBytes int64 `json:"totalSizeBytes,omitempty"`

	// FragmentCount is the number of evidence fragments in the archive.
	FragmentCount int `json:"fragmentCount,omitempty"`

	// ExpiresAt is when the download URL expires.
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`

	// CreatedAt is when the export job was created.
	CreatedAt time.Time `json:"createdAt"`

	// CompletedAt is when the export job was completed.
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}
