// Package entity defines the core domain aggregates for the Argus AI proctoring
// platform. Each entity represents a business concept with identity and lifecycle.
//
// This file adds the multi-tenancy domain model:
//   - Organization: The tenant boundary (university, testing center)
//   - User:         A person with role-based access within an organization
//   - APIKey:       Machine-to-machine authentication credential
//
// Design decision: Flat structs vs nested domain objects
//
//   For the SaaS management layer, we use flat structs with simple validation
//   methods. Unlike ProctoringEvent (which has 20+ fields and complex oneof
//   payloads), Organization/User/APIKey are CRUD entities with straightforward
//   invariants. Rich DDD patterns (value objects, domain events) would add
//   ceremony without proportional benefit.
package entity

import (
	"errors"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Role Constants
// ---------------------------------------------------------------------------

// Role defines the access level of a user within the system.
type Role string

const (
	// RoleSuperAdmin has global access across ALL organizations.
	// org_id = "*" — ClickHouse queries omit the org_id filter.
	RoleSuperAdmin Role = "super_admin"

	// RoleOrgAdmin manages their organization's settings, users, and exams.
	RoleOrgAdmin Role = "org_admin"

	// RoleProctor monitors live proctoring sessions and acknowledges violations.
	RoleProctor Role = "proctor"

	// RoleViewer has read-only access to dashboards and reports.
	RoleViewer Role = "viewer"
)

// SuperAdminOrgID is the special org_id that grants access to all organizations.
const SuperAdminOrgID = "*"

// ValidRoles is the set of valid role values.
var ValidRoles = map[Role]bool{
	RoleSuperAdmin: true,
	RoleOrgAdmin:   true,
	RoleProctor:    true,
	RoleViewer:     true,
}

// IsValid returns true if the role is recognized.
func (r Role) IsValid() bool {
	return ValidRoles[r]
}

// IsSuperAdmin returns true if this role has global access.
func (r Role) IsSuperAdmin() bool {
	return r == RoleSuperAdmin
}

// CanManageOrg returns true if the role can manage organization settings.
func (r Role) CanManageOrg() bool {
	return r == RoleSuperAdmin || r == RoleOrgAdmin
}

// CanManageUsers returns true if the role can create/edit users.
func (r Role) CanManageUsers() bool {
	return r == RoleSuperAdmin || r == RoleOrgAdmin
}

// CanViewDashboard returns true if the role has dashboard access.
func (r Role) CanViewDashboard() bool {
	return true // All roles can view.
}

// ---------------------------------------------------------------------------
// Plan Constants
// ---------------------------------------------------------------------------

// Plan defines the SaaS subscription tier of an organization.
type Plan string

const (
	PlanFree         Plan = "free"
	PlanStandard     Plan = "standard"
	PlanProfessional Plan = "professional"
	PlanEnterprise   Plan = "enterprise"
	PlanUnlimited    Plan = "unlimited"
)

// ---------------------------------------------------------------------------
// Organization Entity
// ---------------------------------------------------------------------------

// Organization represents a tenant in the multi-tenant Argus platform.
// Each organization (university, testing center, corporation) is isolated:
// their users can only see events with matching org_id in ClickHouse.
type Organization struct {
	ID        string `json:"id"`
	OrgID     string `json:"orgId"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	OrgType   string `json:"orgType"`

	// Contact.
	ContactEmail string `json:"contactEmail,omitempty"`
	ContactPhone string `json:"contactPhone,omitempty"`
	City         string `json:"city,omitempty"`
	Region       string `json:"region,omitempty"`

	// Plan & Limits.
	Plan         Plan   `json:"plan"`
	MaxSessions  int    `json:"maxSessions"`
	MaxEventsRPS int    `json:"maxEventsRps"`
	RetentionDays int   `json:"retentionDays"`

	// Status.
	IsActive bool `json:"isActive"`

	// Audit.
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
	CreatedBy string     `json:"createdBy"`
	UpdatedBy string     `json:"updatedBy"`
}

// Validate checks organization invariants.
func (o *Organization) Validate() error {
	if strings.TrimSpace(o.OrgID) == "" {
		return errors.New("org_id is required")
	}
	if strings.TrimSpace(o.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(o.Slug) == "" {
		return errors.New("slug is required")
	}
	if o.MaxSessions <= 0 {
		return errors.New("max_sessions must be positive")
	}
	if o.MaxEventsRPS <= 0 {
		return errors.New("max_events_rps must be positive")
	}
	return nil
}

// ---------------------------------------------------------------------------
// User Entity
// ---------------------------------------------------------------------------

// User represents a person with access to the Argus platform.
// Users are scoped to an organization via org_id. The Super Admin
// has org_id = "*" which grants cross-organization access.
type User struct {
	ID       string `json:"id"`
	OrgID    string `json:"orgId"`
	Phone    string `json:"phone"`
	FullName string `json:"fullName"`
	Email    string `json:"email,omitempty"`
	Role     Role   `json:"role"`
	IsActive bool   `json:"isActive"`

	// Password hash (bcrypt). Never exposed in JSON responses.
	PasswordHash string `json:"-"`

	// Timestamps.
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	DeletedAt   *time.Time `json:"deletedAt,omitempty"`
	CreatedBy   string     `json:"createdBy"`
	UpdatedBy   string     `json:"updatedBy"`
}

// IsSuperAdmin returns true if this user has global access.
func (u *User) IsSuperAdmin() bool {
	return u.Role == RoleSuperAdmin && u.OrgID == SuperAdminOrgID
}

// CanAccessOrg returns true if the user can access data for the given org_id.
func (u *User) CanAccessOrg(targetOrgID string) bool {
	if u.IsSuperAdmin() {
		return true // Super Admin sees everything.
	}
	return u.OrgID == targetOrgID
}

// Validate checks user invariants.
func (u *User) Validate() error {
	if strings.TrimSpace(u.OrgID) == "" {
		return errors.New("org_id is required")
	}
	if strings.TrimSpace(u.Phone) == "" {
		return errors.New("phone is required")
	}
	if strings.TrimSpace(u.FullName) == "" {
		return errors.New("full_name is required")
	}
	if !u.Role.IsValid() {
		return errors.New("invalid role")
	}
	// Super Admin validation.
	if u.Role == RoleSuperAdmin && u.OrgID != SuperAdminOrgID {
		return errors.New("super_admin must have org_id='*'")
	}
	return nil
}

// ---------------------------------------------------------------------------
// API Key Entity
// ---------------------------------------------------------------------------

// APIKey represents a machine-to-machine authentication credential.
// Organizations use API keys to authenticate their backend services
// (Eduser, LMS) with the Argus Event Collector.
type APIKey struct {
	ID           string   `json:"id"`
	OrgID        string   `json:"orgId"`
	Name         string   `json:"name"`
	KeyID        string   `json:"keyId"`
	SecretHash   string   `json:"-"` // Never exposed.
	SecretPrefix string   `json:"secretPrefix"`
	Permissions  []string `json:"permissions"`
	RateLimitRPS int      `json:"rateLimitRps"`
	IsActive     bool     `json:"isActive"`

	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	DeletedAt  *time.Time `json:"deletedAt,omitempty"`
	CreatedBy  string     `json:"createdBy"`
	UpdatedBy  string     `json:"updatedBy"`
}

// IsExpired returns true if the API key has passed its expiration time.
func (k *APIKey) IsExpired() bool {
	if k.ExpiresAt == nil {
		return false // Never expires.
	}
	return time.Now().After(*k.ExpiresAt)
}

// Validate checks API key invariants.
func (k *APIKey) Validate() error {
	if strings.TrimSpace(k.OrgID) == "" {
		return errors.New("org_id is required")
	}
	if strings.TrimSpace(k.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(k.KeyID) == "" {
		return errors.New("key_id is required")
	}
	return nil
}
