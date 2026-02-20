// Package entity defines the core domain entities for the Event Collector.
// These are pure domain objects with no infrastructure dependencies.
// They enforce business invariants and encapsulate domain logic.
package entity

import (
	"fmt"
	"time"

	"github.com/argus-ai/event-collector/internal/domain/valueobject"
)

// ProctoringEvent is the core aggregate root of the Event Collector domain.
// It represents a single proctoring signal captured from a browser session.
//
// Invariants:
//   - EventID must be a valid UUIDv7.
//   - SessionID, StudentID, ExamID must be non-empty.
//   - ServerTimestamp is always set server-side upon ingestion.
//   - Confidence must be in range [0.0, 1.0].
type ProctoringEvent struct {
	EventID         string
	SessionID       string
	StudentID       string
	ExamID          string
	OrgID           string
	EventType       valueobject.EventType
	Severity        valueobject.Severity
	Source          valueobject.EventSource
	ServerTimestamp time.Time
	ClientTimestamp time.Time
	VideoTimestamp  float64 // seconds from session start
	Label          string
	Confidence     float32
	Payload        []byte  // JSON-encoded type-specific payload
	PayloadType    string  // discriminator for payload deserialization
	ClientMeta     ClientMeta
}

// ClientMeta holds browser/client metadata attached to every event.
// IP and region are always set server-side for security.
type ClientMeta struct {
	UserAgent         string
	SDKVersion        string
	Resolution        string
	TimezoneOffsetMin int32
	IPAddress         string // set server-side
	Region            string // derived server-side from IP
}

// Validate enforces domain invariants on the event.
// Returns a domain error if any invariant is violated.
func (e *ProctoringEvent) Validate() error {
	if e.EventID == "" {
		return fmt.Errorf("event_id is required")
	}
	if e.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if e.StudentID == "" {
		return fmt.Errorf("student_id is required")
	}
	if e.ExamID == "" {
		return fmt.Errorf("exam_id is required")
	}
	if e.EventType == valueobject.EventTypeUnspecified {
		return fmt.Errorf("event_type must be specified")
	}
	if e.Severity == valueobject.SeverityUnspecified {
		return fmt.Errorf("severity must be specified")
	}
	if e.Confidence < 0 || e.Confidence > 1 {
		return fmt.Errorf("confidence must be in range [0.0, 1.0], got %f", e.Confidence)
	}
	if e.ClientTimestamp.IsZero() {
		return fmt.Errorf("client_timestamp is required")
	}
	return nil
}

// SetServerTimestamp stamps the event with the current server time.
// This must be called exactly once during ingestion.
func (e *ProctoringEvent) SetServerTimestamp() {
	e.ServerTimestamp = time.Now().UTC()
}

// IsCritical returns true if this event requires immediate attention.
func (e *ProctoringEvent) IsCritical() bool {
	return e.Severity == valueobject.SeverityCritical
}

// IsTelemetry returns true if this is a high-frequency telemetry event
// that should be routed to the analytics pipeline rather than alerts.
func (e *ProctoringEvent) IsTelemetry() bool {
	return e.EventType.IsTelemetry()
}

// KafkaTopic returns the appropriate Kafka topic based on event classification.
// Critical events go to a dedicated high-priority topic.
// Telemetry goes to a high-throughput compacted topic.
// Everything else goes to the standard events topic.
func (e *ProctoringEvent) KafkaTopic() string {
	if e.IsCritical() {
		return "argus.events.critical"
	}
	if e.IsTelemetry() {
		return "argus.events.telemetry"
	}
	return "argus.events.standard"
}

// PartitionKey returns the Kafka partition key.
// We partition by session_id to ensure all events from a single
// session land on the same partition, preserving per-session ordering.
func (e *ProctoringEvent) PartitionKey() string {
	return e.SessionID
}
