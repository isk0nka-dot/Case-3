// Package worker defines asynq job types and payloads for the Argus AI
// background processing system. Heavy CPU-bound tasks (video export, forensic
// PDF generation) are dispatched via Redis-backed asynq queues so they run
// independently of the latency-critical API server.
//
// Job types:
//
//	job:video_export    — TAR.GZ archive of session evidence
//	job:forensic_report — PDF forensic integrity report
package worker

// ---------------------------------------------------------------------------
// Task type constants
// ---------------------------------------------------------------------------

const (
	// TypeVideoExport is the asynq task type for bulk evidence export jobs.
	// The worker downloads evidence fragments from MinIO, queries violation
	// events from ClickHouse, and produces a TAR.GZ archive with a forensic
	// manifest.
	TypeVideoExport = "job:video_export"

	// TypeForensicReport is the asynq task type for forensic PDF generation.
	// The worker computes an integrity score, generates the PDF, uploads it
	// to MinIO, and notifies the admin via Telegram.
	TypeForensicReport = "job:forensic_report"

	// TypeAIAnalysis is the asynq task type for backend AI deep scan.
	// The worker downloads session evidence from MinIO, streams it to the
	// inference gateway for GPU-accelerated analysis, writes detected anomalies
	// back to ClickHouse, and fires Telegram alerts for critical fraud.
	TypeAIAnalysis = "job:ai_analysis"
)

// ---------------------------------------------------------------------------
// Queue names
// ---------------------------------------------------------------------------

const (
	// QueueCritical is the high-priority queue for export and forensic jobs.
	// These are lightweight, CPU-bound tasks (PDF, TAR.GZ) that must not be
	// starved by GPU-heavy inference workloads.
	QueueCritical = "critical"

	// QueueDefault is the standard-priority queue.
	QueueDefault = "default"

	// QueueInference is the isolated queue for GPU-bound AI analysis tasks.
	// Separated from QueueCritical to prevent inference from starving
	// lightweight export/forensic jobs. Concurrency is independently
	// controlled via config.Redis.InferenceConcurrency.
	QueueInference = "inference"

	// QueueLow is the low-priority queue for deferred tasks.
	QueueLow = "low"
)

// ---------------------------------------------------------------------------
// Payloads
// ---------------------------------------------------------------------------

// VideoExportPayload is the JSON payload for TypeVideoExport tasks.
// It mirrors the data needed by the export worker to build a TAR.GZ archive.
type VideoExportPayload struct {
	// ExportID is the unique identifier for the export job (matches PostgreSQL row).
	ExportID string `json:"export_id"`

	// OrgID is the organization that owns this export.
	OrgID string `json:"org_id"`

	// SessionIDs is the list of proctoring sessions to include in the archive.
	SessionIDs []string `json:"session_ids"`

	// PresignTTLSec is the time-to-live in seconds for the presigned download URL.
	PresignTTLSec int `json:"presign_ttl_sec"`
}

// ForensicReportPayload is the JSON payload for TypeForensicReport tasks.
type ForensicReportPayload struct {
	// SessionID is the proctoring session to generate the report for.
	SessionID string `json:"session_id"`

	// RequestedBy is the user ID who requested the report.
	RequestedBy string `json:"requested_by"`

	// OrgID is the organization that owns this session.
	OrgID string `json:"org_id"`
}

// AIAnalysisPayload is the JSON payload for TypeAIAnalysis tasks.
// Triggers a full-session deep scan via the backend inference gateway.
type AIAnalysisPayload struct {
	// SessionID is the proctoring session to re-analyze.
	SessionID string `json:"session_id"`

	// OrgID is the organization that owns this session.
	OrgID string `json:"org_id"`

	// ExamID is the exam this session belongs to.
	ExamID string `json:"exam_id"`

	// StudentID is the student who took the exam.
	StudentID string `json:"student_id"`

	// AnalysisType controls the depth of analysis.
	// Options: "full_scan" (all detectors), "face_verify" (face only), "object_sweep" (objects only).
	AnalysisType string `json:"analysis_type"`

	// ReferenceEmbedding is the optional enrolled ArcFace vector used by the
	// inference sidecar to compute face similarity during identity checks.
	ReferenceEmbedding []float32 `json:"reference_embedding,omitempty"`
}
