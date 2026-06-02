package entity

import (
	"errors"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// External Session Entity
// ---------------------------------------------------------------------------

// ExternalSession represents a proctoring session created by an external
// partner via the External API. The partner receives an argus_session_token
// (JWT) that the SDK uses to authenticate with the gRPC event pipeline.
type ExternalSession struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	OrgID     string `json:"orgId"`
	ExamID    string `json:"examId"`
	StudentID string `json:"studentId"`

	// Optional metadata from partner.
	StudentName string `json:"studentName,omitempty"`
	ExamName    string `json:"examName,omitempty"`
	CallbackURL string `json:"callbackUrl,omitempty"`
	Metadata    []byte `json:"metadata,omitempty"` // JSONB

	// Optional partner-provided idempotency key for safe create-session retries.
	IdempotencyKey string `json:"idempotencyKey,omitempty"`

	// Session token (JWT) — issued at creation.
	SessionToken   string    `json:"-"` // Never exposed after creation.
	TokenExpiresAt time.Time `json:"tokenExpiresAt"`

	// Lifecycle.
	Status string `json:"status"` // created, preflight, active, completed, expired, cancelled

	// Verdict (populated after completion).
	Verdict        *string  `json:"verdict,omitempty"` // clean, suspicious, violation
	VerdictDetails []byte   `json:"verdictDetails,omitempty"`
	IntegrityScore *float64 `json:"integrityScore,omitempty"`
	ViolationCount int      `json:"violationCount"`

	// Timestamps.
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// ValidSessionStatuses is the set of valid lifecycle states.
var ValidSessionStatuses = map[string]bool{
	"created":   true,
	"preflight": true,
	"active":    true,
	"completed": true,
	"expired":   true,
	"cancelled": true,
}

// ValidVerdicts is the set of valid verdict values.
var ValidVerdicts = map[string]bool{
	"clean":      true,
	"suspicious": true,
	"violation":  true,
}

// Validate checks external session invariants.
func (s *ExternalSession) Validate() error {
	if strings.TrimSpace(s.OrgID) == "" {
		return errors.New("org_id is required")
	}
	if strings.TrimSpace(s.ExamID) == "" {
		return errors.New("exam_id is required")
	}
	if strings.TrimSpace(s.StudentID) == "" {
		return errors.New("student_id is required")
	}
	if !ValidSessionStatuses[s.Status] {
		return errors.New("invalid session status")
	}
	if s.Verdict != nil && !ValidVerdicts[*s.Verdict] {
		return errors.New("invalid verdict")
	}
	return nil
}

// IsTerminal returns true if the session is in a final state.
func (s *ExternalSession) IsTerminal() bool {
	return s.Status == "completed" || s.Status == "expired" || s.Status == "cancelled"
}

// ---------------------------------------------------------------------------
// Webhook Endpoint Entity
// ---------------------------------------------------------------------------

// WebhookEndpoint represents a partner-registered callback URL for receiving
// real-time proctoring event notifications with HMAC-SHA256 signatures.
type WebhookEndpoint struct {
	ID     string `json:"id"`
	OrgID  string `json:"orgId"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Secret string `json:"-"` // HMAC signing secret — never exposed after creation.

	// Event type filtering.
	Events []string `json:"events"`

	IsActive bool `json:"isActive"`

	// Health tracking.
	LastDeliveryAt      *time.Time `json:"lastDeliveryAt,omitempty"`
	LastFailureAt       *time.Time `json:"lastFailureAt,omitempty"`
	ConsecutiveFailures int        `json:"consecutiveFailures"`

	// Audit.
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

// ValidWebhookEvents is the set of supported event types.
var ValidWebhookEvents = map[string]bool{
	"session.started":    true,
	"session.completed":  true,
	"violation.detected": true,
	"verdict.ready":      true,
}

// Validate checks webhook endpoint invariants.
func (w *WebhookEndpoint) Validate() error {
	if strings.TrimSpace(w.OrgID) == "" {
		return errors.New("org_id is required")
	}
	if strings.TrimSpace(w.URL) == "" {
		return errors.New("url is required")
	}
	if !strings.HasPrefix(w.URL, "https://") && !strings.HasPrefix(w.URL, "http://") {
		return errors.New("webhook url must use HTTP or HTTPS")
	}
	if len(w.Events) == 0 {
		return errors.New("at least one event type is required")
	}
	for _, e := range w.Events {
		if !ValidWebhookEvents[e] {
			return errors.New("invalid event type: " + e)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Webhook Delivery Entity
// ---------------------------------------------------------------------------

// WebhookDelivery tracks an individual delivery attempt for a webhook event.
// Failed deliveries are retried with exponential backoff.
type WebhookDelivery struct {
	ID         int64  `json:"id"`
	EndpointID string `json:"endpointId"`
	OrgID      string `json:"orgId"`
	EventType  string `json:"eventType"`
	Payload    []byte `json:"payload"` // JSONB

	// Delivery status.
	Status       string `json:"status"` // pending, delivered, failed
	HTTPStatus   int    `json:"httpStatus,omitempty"`
	ResponseBody string `json:"responseBody,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`

	// Retry tracking.
	Attempt     int       `json:"attempt"`
	MaxAttempts int       `json:"maxAttempts"`
	NextRetryAt time.Time `json:"nextRetryAt"`

	// Timestamps.
	CreatedAt   time.Time  `json:"createdAt"`
	DeliveredAt *time.Time `json:"deliveredAt,omitempty"`
}
