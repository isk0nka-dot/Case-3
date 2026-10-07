// Package port defines the application-layer interfaces (ports) that the
// infrastructure layer must implement. This is the inversion-of-dependency
// boundary: the application layer depends on abstractions, not concretions.
//
// This file defines the repository ports for the multi-tenant SaaS layer:
//   - OrgRepository:    CRUD for organizations
//   - UserRepository:   CRUD for users + authentication
//   - APIKeyRepository: CRUD for API keys + validation
package port

import (
	"context"

	"github.com/argus-ai/event-collector/internal/domain/entity"
)

// ---------------------------------------------------------------------------
// Organization Repository
// ---------------------------------------------------------------------------

// OrgRepository defines the data access interface for organizations.
type OrgRepository interface {
	// Create inserts a new organization.
	Create(ctx context.Context, org *entity.Organization) error

	// GetByOrgID returns an organization by its org_id.
	GetByOrgID(ctx context.Context, orgID string) (*entity.Organization, error)

	// GetBySlug returns an organization by its URL slug.
	GetBySlug(ctx context.Context, slug string) (*entity.Organization, error)

	// List returns all active organizations, optionally filtered.
	List(ctx context.Context, filter OrgFilter) ([]*entity.Organization, error)

	// Update modifies an existing organization.
	Update(ctx context.Context, org *entity.Organization) error

	// SoftDelete marks an organization as deleted (sets deleted_at).
	SoftDelete(ctx context.Context, orgID string) error

	// Count returns the number of active organizations.
	Count(ctx context.Context) (int, error)
}

// OrgFilter defines optional filters for listing organizations.
type OrgFilter struct {
	OrgType  string // Filter by organization type.
	Plan     string // Filter by subscription plan.
	IsActive *bool  // Filter by active status.
	Search   string // Full-text search on name, slug, city.
	Limit    int    // Maximum results. 0 = no limit.
	Offset   int    // Pagination offset.
}

// ---------------------------------------------------------------------------
// User Repository
// ---------------------------------------------------------------------------

// UserRepository defines the data access interface for users.
type UserRepository interface {
	// Create inserts a new user with a hashed password.
	Create(ctx context.Context, user *entity.User) error

	// GetByID returns a user by their UUID.
	GetByID(ctx context.Context, userID string) (*entity.User, error)

	// GetByPhone returns a user by their phone number (for login).
	GetByPhone(ctx context.Context, phone string) (*entity.User, error)

	// ListByOrg returns all active users for an organization.
	ListByOrg(ctx context.Context, orgID string) ([]*entity.User, error)

	// ListAll returns all active users (Super Admin only).
	ListAll(ctx context.Context, filter UserFilter) ([]*entity.User, error)

	// Update modifies an existing user.
	Update(ctx context.Context, user *entity.User) error

	// UpdateLastLogin sets the last_login_at timestamp.
	UpdateLastLogin(ctx context.Context, userID string) error

	// SoftDelete marks a user as deleted.
	SoftDelete(ctx context.Context, userID string) error
}

// UserFilter defines optional filters for listing users.
type UserFilter struct {
	OrgID  string      // Filter by organization.
	Role   entity.Role // Filter by role.
	Search string      // Full-text search on name, phone, email.
	Limit  int
	Offset int
}

// ---------------------------------------------------------------------------
// API Key Repository
// ---------------------------------------------------------------------------

// APIKeyRepository defines the data access interface for API keys.
type APIKeyRepository interface {
	// Create inserts a new API key.
	Create(ctx context.Context, key *entity.APIKey) error

	// GetByKeyID returns an API key by its public key_id.
	GetByKeyID(ctx context.Context, keyID string) (*entity.APIKey, error)

	// ListByOrg returns all active API keys for an organization.
	ListByOrg(ctx context.Context, orgID string) ([]*entity.APIKey, error)

	// ListAll returns all API keys (Super Admin only).
	ListAll(ctx context.Context) ([]*entity.APIKey, error)

	// Update modifies an existing API key.
	Update(ctx context.Context, key *entity.APIKey) error

	// Revoke deactivates an API key (sets is_active = false).
	Revoke(ctx context.Context, keyID string) error

	// UpdateLastUsed sets the last_used_at timestamp.
	UpdateLastUsed(ctx context.Context, keyID string) error

	// SoftDelete marks an API key as deleted.
	SoftDelete(ctx context.Context, keyID string) error
}
