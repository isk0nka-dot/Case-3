package port

import (
	"context"

	"github.com/argus-ai/event-collector/internal/domain/entity"
)

// ---------------------------------------------------------------------------
// External Session Repository
// ---------------------------------------------------------------------------

// ExternalSessionFilter controls query filtering for session listings.
type ExternalSessionFilter struct {
	Status    string // Filter by lifecycle status (empty = all).
	StudentID string // Filter by student.
	ExamID    string // Filter by exam.
	Limit     int    // Max results (default: 50).
	Offset    int    // Pagination offset.
}

// ExternalSessionRepository defines the contract for external session
// persistence. These sessions are created by third-party partners via the
// External API and represent proctoring sessions managed by the SDK.
type ExternalSessionRepository interface {
	// Create inserts a new external session.
	Create(ctx context.Context, session *entity.ExternalSession) error

	// GetBySessionID retrieves a session by its unique session_id.
	GetBySessionID(ctx context.Context, sessionID string) (*entity.ExternalSession, error)

	// UpdateStatus transitions the session lifecycle state.
	UpdateStatus(ctx context.Context, sessionID, status string) error

	// UpdateVerdict records the final proctoring verdict after session completion.
	UpdateVerdict(ctx context.Context, sessionID, verdict string, details []byte, score float64, violations int) error

	// ListByOrg returns sessions for an organization with optional filtering.
	ListByOrg(ctx context.Context, orgID string, filter ExternalSessionFilter) ([]*entity.ExternalSession, int, error)
}

// ---------------------------------------------------------------------------
// Webhook Repository
// ---------------------------------------------------------------------------

// WebhookRepository defines the contract for webhook endpoint management
// and delivery tracking.
type WebhookRepository interface {
	// Endpoint management.
	CreateEndpoint(ctx context.Context, endpoint *entity.WebhookEndpoint) error
	GetEndpointByID(ctx context.Context, endpointID string) (*entity.WebhookEndpoint, error)
	GetEndpointsByOrg(ctx context.Context, orgID string) ([]*entity.WebhookEndpoint, error)
	GetEndpointsByOrgAndEvent(ctx context.Context, orgID, eventType string) ([]*entity.WebhookEndpoint, error)
	DeleteEndpoint(ctx context.Context, endpointID string) error

	// Delivery management.
	CreateDelivery(ctx context.Context, delivery *entity.WebhookDelivery) error
	GetPendingDeliveries(ctx context.Context, limit int) ([]*entity.WebhookDelivery, error)
	MarkDelivered(ctx context.Context, deliveryID int64, httpStatus int, responseBody string) error
	MarkFailed(ctx context.Context, deliveryID int64, attempt int, httpStatus int, errorMsg string, nextRetryAt *interface{}) error
	MarkDeliveryFailed(ctx context.Context, deliveryID int64, httpStatus int, errorMsg string) error

	// Endpoint health.
	IncrementFailures(ctx context.Context, endpointID string) error
	ResetFailures(ctx context.Context, endpointID string) error
}
