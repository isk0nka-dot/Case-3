// Package entity defines the core domain entities for the Argus AI platform.
//
// AuditEntry represents an immutable record of an administrative action.
// Every mutation performed through the admin API is recorded in the audit log
// for compliance, debugging, and accountability purposes.
package entity

import "time"

// AuditEntry is an immutable record of an administrative action.
// Once written, audit entries are never updated or deleted.
//
// The audit_log table in PostgreSQL stores these records with indices
// on (org_id, created_at) and (user_id, created_at) for efficient querying.
type AuditEntry struct {
	// Unique identifier (UUIDv4, assigned by PostgreSQL).
	ID string `json:"id"`

	// The user who performed the action.
	UserID    string `json:"userId"`
	UserPhone string `json:"userPhone"`
	UserRole  string `json:"userRole"`

	// The organization affected by the action.
	// For cross-org actions (super_admin), this is the target org.
	OrgID string `json:"orgId"`

	// Action classification.
	// Examples: "login", "create_org", "update_org", "delete_org",
	//           "create_user", "create_api_key", "revoke_api_key".
	Action string `json:"action"`

	// The type of resource affected.
	// Examples: "organization", "user", "api_key", "session".
	ResourceType string `json:"resourceType"`

	// The identifier of the specific resource affected.
	// For organizations: org_id. For users: user UUID. For API keys: key_id.
	ResourceID string `json:"resourceId"`

	// Structured details about the action (stored as JSONB in PostgreSQL).
	// For mutations, this typically contains "before" and "after" snapshots.
	// For logins, this contains the login method and result.
	Details interface{} `json:"details"`

	// Client metadata for forensic analysis.
	IPAddress string `json:"ipAddress"`
	UserAgent string `json:"userAgent"`

	// When the action occurred (set server-side, never trusted from client).
	CreatedAt time.Time `json:"createdAt"`
}
