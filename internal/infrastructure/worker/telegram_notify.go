package worker

import (
	"time"

	"github.com/argus-ai/event-collector/internal/infrastructure/alerting"
)

// ---------------------------------------------------------------------------
// Telegram job lifecycle notifications
// ---------------------------------------------------------------------------

// NotifyJobCompleted sends a Telegram alert when a background job finishes
// successfully. Uses SendDirect (synchronous, bypasses rate limiter) because
// job events are low-volume and completion visibility is important.
func NotifyJobCompleted(provider alerting.Provider, jobType, jobID string, duration time.Duration) {
	if provider == nil {
		return
	}

	label := friendlyJobType(jobType)
	msg := alerting.MsgJobCompleted(
		label, jobID,
		duration.Round(time.Millisecond).String(),
		time.Now().Format("2006-01-02 15:04:05 MST"),
	)

	// Best-effort — don't block the worker on Telegram failures.
	_ = provider.SendDirect(msg)
}

// NotifyJobFailed sends a Telegram alert when a background job fails after
// all retries are exhausted. This is the final failure notification.
func NotifyJobFailed(provider alerting.Provider, jobType, jobID string, err error) {
	if provider == nil {
		return
	}

	label := friendlyJobType(jobType)

	errMsg := "unknown error"
	if err != nil {
		errMsg = err.Error()
	}

	msg := alerting.MsgJobFailed(
		label, jobID,
		time.Now().Format("2006-01-02 15:04:05 MST"),
		errMsg,
	)

	_ = provider.SendDirect(msg)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func friendlyJobType(taskType string) string {
	switch taskType {
	case TypeVideoExport:
		return "Video Export"
	case TypeForensicReport:
		return "Forensic Report"
	case TypeAIAnalysis:
		return "AI Deep Analysis"
	default:
		return taskType
	}
}
