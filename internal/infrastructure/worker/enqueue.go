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
// The task is enqueued on the "default" queue (not critical — post-session)
// with a 15-minute timeout and up to 2 retries.
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
		asynq.Queue(QueueDefault),
		asynq.TaskID(fmt.Sprintf("ai:%s", payload.SessionID)),
	), nil
}
