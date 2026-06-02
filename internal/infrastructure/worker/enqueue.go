package worker

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// ---------------------------------------------------------------------------
// Task constructors — called from HTTP handlers to enqueue jobs
// ---------------------------------------------------------------------------

// NewVideoExportTask creates an asynq task for bulk evidence export.
// The task is enqueued on the "critical" queue with a 30-minute timeout
// (large archives can take significant time) and up to 3 retries.
func NewVideoExportTask(payload VideoExportPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal VideoExportPayload: %w", err)
	}
	return asynq.NewTask(
		TypeVideoExport,
		data,
		asynq.MaxRetry(3),
		asynq.Timeout(30*time.Minute),
		asynq.Queue(QueueCritical),
		asynq.TaskID(fmt.Sprintf("export:%s", payload.ExportID)),
	), nil
}

// NewForensicReportTask creates an asynq task for PDF forensic report generation.
// The task is enqueued on the "critical" queue with a 5-minute timeout
// and up to 3 retries.
func NewForensicReportTask(payload ForensicReportPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal ForensicReportPayload: %w", err)
	}
	return asynq.NewTask(
		TypeForensicReport,
		data,
		asynq.MaxRetry(3),
		asynq.Timeout(5*time.Minute),
		asynq.Queue(QueueCritical),
		asynq.TaskID(fmt.Sprintf("forensic:%s", payload.SessionID)),
	), nil
}

// NewAIAnalysisTask creates an asynq task for backend AI deep scan.
// The task is enqueued on the isolated "inference" queue to prevent
// GPU-heavy analysis from starving lightweight export/forensic jobs.
// 15-minute timeout and up to 2 retries.
func NewAIAnalysisTask(payload AIAnalysisPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal AIAnalysisPayload: %w", err)
	}
	return asynq.NewTask(
		TypeAIAnalysis,
		data,
		asynq.MaxRetry(2),
		asynq.Timeout(15*time.Minute),
		asynq.Queue(QueueInference),
		asynq.TaskID(fmt.Sprintf("ai:%s", payload.SessionID)),
	), nil
}

// NewAIFrameAnalysisTask creates an asynq task for one browser snapshot.
// Frame jobs are intentionally not deduplicated by session: each sampled frame
// can produce a distinct timestamped anomaly for live monitoring.
func NewAIFrameAnalysisTask(payload AIFrameAnalysisPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal AIFrameAnalysisPayload: %w", err)
	}
	return asynq.NewTask(
		TypeAIFrameAnalysis,
		data,
		asynq.MaxRetry(1),
		asynq.Timeout(45*time.Second),
		asynq.Queue(QueueInference),
	), nil
}

// NewEnrollStudentTask creates an asynq task for student biometric enrollment.
// Fetches reference photo, extracts ArcFace embedding via inference gateway,
// and stores it in student_enrollments. Enqueued on the default queue.
func NewEnrollStudentTask(payload EnrollStudentPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal EnrollStudentPayload: %w", err)
	}
	return asynq.NewTask(
		TypeEnrollStudent,
		data,
		asynq.MaxRetry(3),
		asynq.Timeout(2*time.Minute),
		asynq.Queue(QueueDefault),
		asynq.TaskID(fmt.Sprintf("enroll:%s:%s", payload.OrgID, payload.StudentID)),
	), nil
}
