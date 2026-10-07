// Package http provides REST API handlers for the admin/SaaS management layer.
//
// These endpoints are served alongside the gRPC-Web proxy on the HTTP server
// (port 8080). They handle organization, user, and API key management.
//
// All admin endpoints require authentication via JWT or session cookie.
// Role-based access control is enforced at the handler level:
//   - super_admin: full access to all endpoints
//   - org_admin:   access limited to their own organization
//   - proctor/viewer: read-only access to their own organization
//
// URL structure:
//
//	POST   /api/v1/auth/login                     — Authenticate user
//	GET    /api/v1/auth/me                         — Get current user info
//	GET    /api/v1/admin/organizations              — List organizations (super_admin only)
//	POST   /api/v1/admin/organizations              — Create organization (super_admin only)
//	GET    /api/v1/admin/organizations/:orgId       — Get organization details
//	PUT    /api/v1/admin/organizations/:orgId       — Update organization
//	DELETE /api/v1/admin/organizations/:orgId       — Soft-delete organization
//	POST   /api/v1/admin/organizations/:orgId/retention/apply — Apply retention policy
//	GET    /api/v1/admin/organizations/:orgId/users — List users for org
//	POST   /api/v1/admin/organizations/:orgId/users — Create user
//	GET    /api/v1/admin/organizations/:orgId/keys     — List API keys
//	POST   /api/v1/admin/organizations/:orgId/keys     — Create API key
//	DELETE /api/v1/admin/keys/:keyId                   — Revoke API key
//	GET    /api/v1/admin/organizations/:orgId/webhooks — List webhook endpoints
//	POST   /api/v1/admin/organizations/:orgId/webhooks — Create webhook endpoint
//	DELETE /api/v1/admin/webhooks/:webhookId           — Delete webhook endpoint
//	GET    /api/v1/admin/stats                         — Cross-org statistics
package http

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/argus-ai/event-collector/internal/application/port"
	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// AdminHandler serves the REST API for the admin SaaS layer.
type AdminHandler struct {
	repo   *postgres.Repository
	logger *zap.Logger

	// JWT signing key for generating session tokens.
	jwtSigningKey []byte
}

// NewAdminHandler creates a new admin API handler.
func NewAdminHandler(repo *postgres.Repository, logger *zap.Logger, jwtSigningKey []byte) *AdminHandler {
	return &AdminHandler{
		repo:          repo,
		logger:        logger.Named("admin_api"),
		jwtSigningKey: jwtSigningKey,
	}
}

// RegisterRoutes registers admin API endpoints on the given mux.
func (h *AdminHandler) RegisterRoutes(mux *http.ServeMux) {
	// Auth endpoints (no auth required).
	mux.HandleFunc("POST /api/v1/auth/login", h.handleLogin)

	// Auth-required endpoints.
	mux.HandleFunc("GET /api/v1/auth/me", h.requireAuth(h.handleMe))
	mux.HandleFunc("POST /api/v1/auth/refresh", h.requireAuth(h.handleRefreshToken))

	// Organization management.
	mux.HandleFunc("GET /api/v1/admin/organizations", h.requireAuth(h.requireRole(entity.RoleSuperAdmin, h.handleListOrgs)))
	mux.HandleFunc("POST /api/v1/admin/organizations", h.requireAuth(h.requireRole(entity.RoleSuperAdmin, h.handleCreateOrg)))
	mux.HandleFunc("POST /api/v1/admin/organizations-with-admin", h.requireAuth(h.requireRole(entity.RoleSuperAdmin, h.handleCreateOrgWithAdmin)))
	mux.HandleFunc("GET /api/v1/admin/organizations/{orgId}", h.requireAuth(h.handleGetOrg))
	mux.HandleFunc("PUT /api/v1/admin/organizations/{orgId}", h.requireAuth(h.requireRole(entity.RoleSuperAdmin, h.handleUpdateOrg)))
	mux.HandleFunc("DELETE /api/v1/admin/organizations/{orgId}", h.requireAuth(h.requireRole(entity.RoleSuperAdmin, h.handleDeleteOrg)))
	mux.HandleFunc("POST /api/v1/admin/organizations/{orgId}/retention/apply", h.requireAuth(h.requireOrgAdmin(h.handleApplyRetention)))

	// User management.
	mux.HandleFunc("GET /api/v1/admin/organizations/{orgId}/users", h.requireAuth(h.handleListUsers))
	mux.HandleFunc("POST /api/v1/admin/organizations/{orgId}/users", h.requireAuth(h.requireOrgAdmin(h.handleCreateUser)))
	mux.HandleFunc("PUT /api/v1/admin/organizations/{orgId}/users/{userId}", h.requireAuth(h.requireOrgAdmin(h.handleUpdateUser)))
	mux.HandleFunc("DELETE /api/v1/admin/organizations/{orgId}/users/{userId}", h.requireAuth(h.requireOrgAdmin(h.handleDeactivateUser)))

	// API key management.
	mux.HandleFunc("GET /api/v1/admin/organizations/{orgId}/keys", h.requireAuth(h.handleListAPIKeys))
	mux.HandleFunc("POST /api/v1/admin/organizations/{orgId}/keys", h.requireAuth(h.requireOrgAdmin(h.handleCreateAPIKey)))
	mux.HandleFunc("DELETE /api/v1/admin/keys/{keyId}", h.requireAuth(h.requireOrgAdmin(h.handleRevokeAPIKey)))

	// Webhook endpoint management.
	mux.HandleFunc("GET /api/v1/admin/organizations/{orgId}/webhooks", h.requireAuth(h.handleListWebhooks))
	mux.HandleFunc("POST /api/v1/admin/organizations/{orgId}/webhooks", h.requireAuth(h.requireOrgAdmin(h.handleCreateWebhook)))
	mux.HandleFunc("DELETE /api/v1/admin/webhooks/{webhookId}", h.requireAuth(h.requireOrgAdmin(h.handleDeleteWebhook)))

	// Cross-org statistics (super_admin only).
	mux.HandleFunc("GET /api/v1/admin/stats", h.requireAuth(h.requireRole(entity.RoleSuperAdmin, h.handleStats)))

	// Audit logs (super_admin only).
	mux.HandleFunc("GET /api/v1/admin/audit-logs", h.requireAuth(h.requireRole(entity.RoleSuperAdmin, h.handleListAuditLogs)))
}

// ==========================================================================
// Auth Endpoints
// ==========================================================================

type loginRequest struct {
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string       `json:"token"`
	User  *entity.User `json:"user"`
}

func (h *AdminHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Normalize phone.
	phone := req.Phone
	if !strings.HasPrefix(phone, "+7") {
		phone = "+7" + phone
	}

	// Look up user.
	user, err := h.repo.GetUserByPhone(r.Context(), phone)
	if err != nil {
		h.logger.Error("Login: DB error", zap.Error(err))
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if user == nil {
		h.jsonError(w, "Неверный номер телефона или пароль", http.StatusUnauthorized)
		return
	}

	// Verify password.
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		h.jsonError(w, "Неверный номер телефона или пароль", http.StatusUnauthorized)
		return
	}

	if !user.IsActive {
		h.jsonError(w, "Аккаунт деактивирован", http.StatusForbidden)
		return
	}

	// Generate JWT token.
	token, err := h.generateToken(user)
	if err != nil {
		h.logger.Error("Login: token generation failed", zap.Error(err))
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Update last login.
	_ = h.repo.UpdateUserLastLogin(r.Context(), user.ID)

	h.logger.Info("User logged in",
		zap.String("user_id", user.ID),
		zap.String("phone", user.Phone),
		zap.String("role", string(user.Role)),
		zap.String("org_id", user.OrgID),
	)

	// Audit: record successful login.
	// Login is special — the user isn't in context yet, so we construct the entry directly.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.repo.CreateAuditEntry(ctx, &entity.AuditEntry{
			UserID:       user.ID,
			UserPhone:    user.Phone,
			UserRole:     string(user.Role),
			OrgID:        user.OrgID,
			Action:       "login",
			ResourceType: "session",
			ResourceID:   user.ID,
			Details:      map[string]string{"phone": user.Phone},
			IPAddress:    extractClientIP(r),
			UserAgent:    r.UserAgent(),
		})
	}()

	h.jsonResponse(w, loginResponse{Token: token, User: user}, http.StatusOK)
}

func (h *AdminHandler) handleMe(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	h.jsonResponse(w, user, http.StatusOK)
}

// handleRefreshToken issues a fresh JWT for an authenticated user.
// Called by the frontend before heartbeat when the current token is near expiry.
// This keeps long-running exam sessions (2-4 hours) alive without forcing re-login.
func (h *AdminHandler) handleRefreshToken(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Re-fetch user from DB to ensure they haven't been deactivated since last login
	fresh, err := h.repo.GetUserByID(r.Context(), user.ID)
	if err != nil || fresh == nil || !fresh.IsActive {
		h.jsonError(w, "User is inactive or not found", http.StatusForbidden)
		return
	}

	token, err := h.generateToken(fresh)
	if err != nil {
		h.logger.Error("Token refresh failed", zap.Error(err), zap.String("user_id", user.ID))
		h.jsonError(w, "Failed to refresh token", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]string{"token": token}, http.StatusOK)
}

// ==========================================================================
// Organization Endpoints
// ==========================================================================

func (h *AdminHandler) handleListOrgs(w http.ResponseWriter, r *http.Request) {
	filter := port.OrgFilter{
		Search:  r.URL.Query().Get("search"),
		OrgType: r.URL.Query().Get("type"),
		Plan:    r.URL.Query().Get("plan"),
	}

	orgs, err := h.repo.ListOrgs(r.Context(), filter)
	if err != nil {
		h.logger.Error("List orgs failed", zap.Error(err))
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	h.jsonResponse(w, orgs, http.StatusOK)
}

type createOrgRequest struct {
	OrgID         string `json:"orgId"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	OrgType       string `json:"orgType"`
	ContactEmail  string `json:"contactEmail"`
	ContactPhone  string `json:"contactPhone"`
	City          string `json:"city"`
	Region        string `json:"region"`
	Plan          string `json:"plan"`
	MaxSessions   int    `json:"maxSessions"`
	MaxEventsRPS  int    `json:"maxEventsRps"`
	RetentionDays int    `json:"retentionDays"`

	// Feature Toggles & Quotas.
	AllowedFeatures map[string]bool `json:"allowedFeatures,omitempty"`
	SessionLimit    int             `json:"sessionLimit"`
	TrialEndsAt     *time.Time      `json:"trialEndsAt,omitempty"`
}

type retentionApplyRequest struct {
	DryRun             *bool `json:"dryRun"`
	VideoRetentionDays int   `json:"videoRetentionDays"`
	AuditRetentionDays int   `json:"auditRetentionDays"`
}

func (h *AdminHandler) handleCreateOrg(w http.ResponseWriter, r *http.Request) {
	var req createOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Create org: JSON decode failed",
			zap.Error(err),
			zap.String("content_type", r.Header.Get("Content-Type")),
		)
		h.jsonError(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	caller := getUserFromContext(r.Context())
	if caller == nil {
		h.jsonError(w, "Unauthorized: no user in context", http.StatusUnauthorized)
		return
	}

	h.logger.Info("Create org: received request",
		zap.String("org_id", req.OrgID),
		zap.String("name", req.Name),
		zap.String("slug", req.Slug),
		zap.String("org_type", req.OrgType),
		zap.String("plan", req.Plan),
		zap.String("caller", caller.Phone),
	)

	// Default plan if not provided.
	plan := req.Plan
	if plan == "" {
		plan = "standard"
	}

	// Default org type if not provided.
	orgType := req.OrgType
	if orgType == "" {
		orgType = "university"
	}

	org := &entity.Organization{
		OrgID:           req.OrgID,
		Name:            req.Name,
		Slug:            req.Slug,
		OrgType:         orgType,
		ContactEmail:    req.ContactEmail,
		ContactPhone:    req.ContactPhone,
		City:            req.City,
		Region:          req.Region,
		Plan:            entity.Plan(plan),
		MaxSessions:     req.MaxSessions,
		MaxEventsRPS:    req.MaxEventsRPS,
		RetentionDays:   90,
		AllowedFeatures: req.AllowedFeatures,
		SessionLimit:    req.SessionLimit,
		TrialEndsAt:     req.TrialEndsAt,
		IsActive:        true,
		CreatedBy:       caller.ID,
		UpdatedBy:       caller.ID,
	}

	if req.MaxSessions == 0 {
		org.MaxSessions = 1000
	}
	if req.MaxEventsRPS == 0 {
		org.MaxEventsRPS = 10000
	}

	if err := org.Validate(); err != nil {
		h.logger.Warn("Create org: validation failed",
			zap.Error(err),
			zap.String("org_id", req.OrgID),
		)
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.repo.CreateOrg(r.Context(), org); err != nil {
		h.logger.Error("Create org: database insert failed",
			zap.Error(err),
			zap.String("org_id", org.OrgID),
			zap.String("slug", org.Slug),
		)
		h.jsonError(w, "Failed to create organization: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logger.Info("Organization created successfully",
		zap.String("id", org.ID),
		zap.String("org_id", org.OrgID),
		zap.String("name", org.Name),
		zap.String("created_by", caller.Phone),
	)

	h.audit(r, "create_org", "organization", org.OrgID, map[string]interface{}{
		"name": org.Name, "slug": org.Slug, "plan": string(org.Plan), "orgType": org.OrgType,
	})

	h.jsonResponse(w, org, http.StatusCreated)
}

// ==========================================================================
// Create Organization + Primary Admin (Transactional)
// ==========================================================================

type createOrgWithAdminRequest struct {
	// Organization fields.
	OrgID        string `json:"orgId"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	OrgType      string `json:"orgType"`
	ContactEmail string `json:"contactEmail"`
	ContactPhone string `json:"contactPhone"`
	City         string `json:"city"`
	Region       string `json:"region"`
	Plan         string `json:"plan"`
	MaxSessions  int    `json:"maxSessions"`
	MaxEventsRPS int    `json:"maxEventsRps"`
	// Feature Toggles & Quotas.
	AllowedFeatures map[string]bool `json:"allowedFeatures,omitempty"`
	SessionLimit    int             `json:"sessionLimit"`
	TrialEndsAt     *time.Time      `json:"trialEndsAt,omitempty"`
	// Primary admin fields.
	AdminFullName string `json:"adminFullName"`
	AdminPhone    string `json:"adminPhone"`
	AdminPassword string `json:"adminPassword"`
}

type createOrgWithAdminResponse struct {
	Organization *entity.Organization `json:"organization"`
	Admin        *entity.User         `json:"admin"`
}

func (h *AdminHandler) handleCreateOrgWithAdmin(w http.ResponseWriter, r *http.Request) {
	var req createOrgWithAdminRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	caller := getUserFromContext(r.Context())
	if caller == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Validate required admin fields.
	if req.AdminFullName == "" || req.AdminPhone == "" || req.AdminPassword == "" {
		h.jsonError(w, "Admin full name, phone, and password are required", http.StatusBadRequest)
		return
	}
	if len(req.AdminPassword) < 10 {
		h.jsonError(w, "Admin password must be at least 10 characters", http.StatusBadRequest)
		return
	}

	// Normalize admin phone.
	adminPhone := req.AdminPhone
	if !strings.HasPrefix(adminPhone, "+7") {
		adminPhone = "+7" + adminPhone
	}

	// Build the organization entity.
	plan := req.Plan
	if plan == "" {
		plan = "standard"
	}
	orgType := req.OrgType
	if orgType == "" {
		orgType = "university"
	}

	org := &entity.Organization{
		OrgID:           req.OrgID,
		Name:            req.Name,
		Slug:            req.Slug,
		OrgType:         orgType,
		ContactEmail:    req.ContactEmail,
		ContactPhone:    req.ContactPhone,
		City:            req.City,
		Region:          req.Region,
		Plan:            entity.Plan(plan),
		MaxSessions:     req.MaxSessions,
		MaxEventsRPS:    req.MaxEventsRPS,
		RetentionDays:   90,
		AllowedFeatures: req.AllowedFeatures,
		SessionLimit:    req.SessionLimit,
		TrialEndsAt:     req.TrialEndsAt,
		IsActive:        true,
		CreatedBy:       caller.ID,
		UpdatedBy:       caller.ID,
	}
	if org.MaxSessions == 0 {
		org.MaxSessions = 1000
	}
	if org.MaxEventsRPS == 0 {
		org.MaxEventsRPS = 10000
	}
	if err := org.Validate(); err != nil {
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Hash the admin password.
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.AdminPassword), 12)
	if err != nil {
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	adminUser := &entity.User{
		OrgID:        req.OrgID,
		Phone:        adminPhone,
		PasswordHash: string(passwordHash),
		FullName:     req.AdminFullName,
		Email:        req.ContactEmail, // Reuse org contact email.
		Role:         entity.RoleOrgAdmin,
		IsActive:     true,
		CreatedBy:    caller.ID,
		UpdatedBy:    caller.ID,
	}
	if err := adminUser.Validate(); err != nil {
		h.jsonError(w, "Admin user validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	// ---- BEGIN TRANSACTION ----
	tx, err := h.repo.DB().BeginTx(r.Context(), nil)
	if err != nil {
		h.logger.Error("Failed to begin transaction", zap.Error(err))
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Insert organization inside the transaction.
	orgQuery := `
		INSERT INTO organizations (
			org_id, name, slug, org_type, contact_email, contact_phone,
			city, region, plan, max_sessions, max_events_rps, retention_days, is_active, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING id, created_at, updated_at`
	err = tx.QueryRowContext(r.Context(), orgQuery,
		org.OrgID, org.Name, org.Slug, org.OrgType,
		org.ContactEmail, org.ContactPhone, org.City, org.Region,
		string(org.Plan), org.MaxSessions, org.MaxEventsRPS, org.RetentionDays,
		org.IsActive, org.CreatedBy, org.UpdatedBy,
	).Scan(&org.ID, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		h.logger.Error("TX: create org failed", zap.Error(err))
		h.jsonError(w, "Failed to create organization: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Insert admin user inside the same transaction.
	userQuery := `
		INSERT INTO users (org_id, phone, password_hash, full_name, email, role, is_active, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, created_at, updated_at`
	err = tx.QueryRowContext(r.Context(), userQuery,
		adminUser.OrgID, adminUser.Phone, adminUser.PasswordHash,
		adminUser.FullName, adminUser.Email, string(adminUser.Role),
		adminUser.IsActive, adminUser.CreatedBy, adminUser.UpdatedBy,
	).Scan(&adminUser.ID, &adminUser.CreatedAt, &adminUser.UpdatedAt)
	if err != nil {
		h.logger.Error("TX: create admin user failed", zap.Error(err))
		h.jsonError(w, "Не удалось создать администратора (возможно, такой номер телефона уже зарегистрирован)", http.StatusConflict)
		return
	}

	// ---- COMMIT ----
	if err := tx.Commit(); err != nil {
		h.logger.Error("TX: commit failed", zap.Error(err))
		h.jsonError(w, "Transaction commit failed", http.StatusInternalServerError)
		return
	}

	h.logger.Info("Organization + admin created atomically",
		zap.String("org_id", org.OrgID),
		zap.String("admin_phone", adminUser.Phone),
		zap.String("admin_role", string(adminUser.Role)),
		zap.String("created_by", caller.Phone),
	)

	// Audit both actions.
	h.audit(r, "create_org", "organization", org.OrgID, map[string]interface{}{
		"name": org.Name, "slug": org.Slug, "plan": string(org.Plan),
		"adminPhone": adminUser.Phone, "adminFullName": adminUser.FullName,
	})
	h.audit(r, "create_user", "user", adminUser.ID, map[string]interface{}{
		"phone": adminUser.Phone, "fullName": adminUser.FullName,
		"role": string(adminUser.Role), "orgId": org.OrgID,
	})

	// Clear the password hash before returning — never expose it.
	adminUser.PasswordHash = ""

	h.jsonResponse(w, createOrgWithAdminResponse{
		Organization: org,
		Admin:        adminUser,
	}, http.StatusCreated)
}

func (h *AdminHandler) handleGetOrg(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	caller := getUserFromContext(r.Context())

	// Non-super-admin can only view their own org.
	if !caller.IsSuperAdmin() && caller.OrgID != orgID {
		h.jsonError(w, "Access denied", http.StatusForbidden)
		return
	}

	org, err := h.repo.GetOrgByOrgID(r.Context(), orgID)
	if err != nil {
		h.logger.Error("Get org failed", zap.Error(err))
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if org == nil {
		h.jsonError(w, "Organization not found", http.StatusNotFound)
		return
	}

	h.jsonResponse(w, org, http.StatusOK)
}

func (h *AdminHandler) handleUpdateOrg(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")

	var req createOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	caller := getUserFromContext(r.Context())

	org, err := h.repo.GetOrgByOrgID(r.Context(), orgID)
	if err != nil || org == nil {
		h.jsonError(w, "Organization not found", http.StatusNotFound)
		return
	}

	// Capture before state for audit trail.
	beforeState := map[string]interface{}{
		"name": org.Name, "plan": string(org.Plan),
		"maxSessions": org.MaxSessions, "maxEventsRps": org.MaxEventsRPS,
		"retentionDays":   org.RetentionDays,
		"allowedFeatures": org.AllowedFeatures, "sessionLimit": org.SessionLimit,
	}

	// Apply updates.
	if req.Name != "" {
		org.Name = req.Name
	}
	if req.ContactEmail != "" {
		org.ContactEmail = req.ContactEmail
	}
	if req.ContactPhone != "" {
		org.ContactPhone = req.ContactPhone
	}
	if req.City != "" {
		org.City = req.City
	}
	if req.Region != "" {
		org.Region = req.Region
	}
	if req.Plan != "" {
		org.Plan = entity.Plan(req.Plan)
	}
	if req.MaxSessions > 0 {
		org.MaxSessions = req.MaxSessions
	}
	if req.MaxEventsRPS > 0 {
		org.MaxEventsRPS = req.MaxEventsRPS
	}
	if req.RetentionDays > 0 {
		org.RetentionDays = req.RetentionDays
	}

	// Apply feature toggles & quotas.
	// When AllowedFeatures is non-nil, the features tab is being saved —
	// apply features + sessionLimit atomically (solves zero-value ambiguity).
	if req.AllowedFeatures != nil {
		org.AllowedFeatures = req.AllowedFeatures
		org.SessionLimit = req.SessionLimit
	}
	if req.TrialEndsAt != nil {
		org.TrialEndsAt = req.TrialEndsAt
	}

	org.UpdatedBy = caller.ID

	if err := h.repo.UpdateOrg(r.Context(), org); err != nil {
		h.logger.Error("Update org failed", zap.Error(err))
		h.jsonError(w, "Failed to update organization", http.StatusInternalServerError)
		return
	}

	afterState := map[string]interface{}{
		"name": org.Name, "plan": string(org.Plan),
		"maxSessions": org.MaxSessions, "maxEventsRps": org.MaxEventsRPS,
		"retentionDays":   org.RetentionDays,
		"allowedFeatures": org.AllowedFeatures, "sessionLimit": org.SessionLimit,
	}
	h.audit(r, "update_org", "organization", orgID, map[string]interface{}{
		"before": beforeState, "after": afterState,
	})

	h.jsonResponse(w, org, http.StatusOK)
}

func (h *AdminHandler) handleDeleteOrg(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")

	if orgID == entity.SuperAdminOrgID {
		h.jsonError(w, "Cannot delete the global admin organization", http.StatusForbidden)
		return
	}

	if err := h.repo.SoftDeleteOrg(r.Context(), orgID); err != nil {
		h.logger.Error("Delete org failed", zap.Error(err))
		h.jsonError(w, "Failed to delete organization", http.StatusInternalServerError)
		return
	}

	h.audit(r, "delete_org", "organization", orgID, map[string]string{"orgId": orgID})

	h.jsonResponse(w, map[string]string{"status": "deleted"}, http.StatusOK)
}

func (h *AdminHandler) handleApplyRetention(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	caller := getUserFromContext(r.Context())
	if caller == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if !caller.IsSuperAdmin() && caller.OrgID != orgID {
		h.jsonError(w, "Access denied", http.StatusForbidden)
		return
	}

	var req retentionApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	dryRun := true
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}

	org, err := h.repo.GetOrgByOrgID(r.Context(), orgID)
	if err != nil || org == nil {
		h.jsonError(w, "Organization not found", http.StatusNotFound)
		return
	}

	result, err := h.repo.ApplyRetentionCleanup(r.Context(), postgres.RetentionCleanupParams{
		OrgID:              orgID,
		VideoRetentionDays: req.VideoRetentionDays,
		AuditRetentionDays: req.AuditRetentionDays,
		DryRun:             dryRun,
	})
	if err != nil {
		h.logger.Error("Retention cleanup failed",
			zap.String("org_id", orgID),
			zap.Bool("dry_run", dryRun),
			zap.Error(err),
		)
		h.jsonError(w, "Failed to apply retention policy", http.StatusInternalServerError)
		return
	}

	h.audit(r, "apply_retention", "organization", orgID, result)
	h.jsonResponse(w, result, http.StatusOK)
}

// ==========================================================================
// User Endpoints
// ==========================================================================

func (h *AdminHandler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	caller := getUserFromContext(r.Context())

	if !caller.IsSuperAdmin() && caller.OrgID != orgID {
		h.jsonError(w, "Access denied", http.StatusForbidden)
		return
	}

	users, err := h.repo.ListUsersByOrg(r.Context(), orgID)
	if err != nil {
		h.logger.Error("List users failed", zap.Error(err))
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	h.jsonResponse(w, users, http.StatusOK)
}

type createUserRequest struct {
	Phone    string `json:"phone"`
	Password string `json:"password"`
	FullName string `json:"fullName"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

func (h *AdminHandler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	caller := getUserFromContext(r.Context())

	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Normalize phone.
	phone := req.Phone
	if !strings.HasPrefix(phone, "+7") {
		phone = "+7" + phone
	}

	// Validate password length.
	if len(req.Password) < 6 {
		h.jsonError(w, "Password must be at least 6 characters", http.StatusBadRequest)
		return
	}

	// Hash password.
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Only super_admin can create super_admin or users in other orgs.
	role := entity.Role(req.Role)
	if role == entity.RoleSuperAdmin && !caller.IsSuperAdmin() {
		h.jsonError(w, "Only Super Admin can create super_admin users", http.StatusForbidden)
		return
	}

	user := &entity.User{
		OrgID:        orgID,
		Phone:        phone,
		PasswordHash: string(hash),
		FullName:     req.FullName,
		Email:        req.Email,
		Role:         role,
		IsActive:     true,
		CreatedBy:    caller.ID,
		UpdatedBy:    caller.ID,
	}

	if err := user.Validate(); err != nil {
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.repo.CreateUser(r.Context(), user); err != nil {
		h.logger.Error("Create user failed", zap.Error(err))
		h.jsonError(w, "Failed to create user (phone may already exist)", http.StatusConflict)
		return
	}

	h.logger.Info("User created",
		zap.String("user_id", user.ID),
		zap.String("phone", user.Phone),
		zap.String("role", string(user.Role)),
		zap.String("org_id", orgID),
	)

	h.audit(r, "create_user", "user", user.ID, map[string]interface{}{
		"phone": user.Phone, "fullName": user.FullName,
		"role": string(user.Role), "orgId": orgID, "email": user.Email,
	})

	h.jsonResponse(w, user, http.StatusCreated)
}

// --------------------------------------------------------------------------
// Update User
// --------------------------------------------------------------------------

type updateUserRequest struct {
	FullName string `json:"fullName"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	IsActive *bool  `json:"isActive"` // pointer so we can distinguish "not provided" from "false"
}

func (h *AdminHandler) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	userID := r.PathValue("userId")
	caller := getUserFromContext(r.Context())

	if caller == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Fetch target user.
	target, err := h.repo.GetUserByID(r.Context(), userID)
	if err != nil || target == nil {
		h.jsonError(w, "User not found", http.StatusNotFound)
		return
	}

	// Org isolation: non-super_admin can only manage users within their own org.
	if !caller.IsSuperAdmin() && target.OrgID != orgID {
		h.jsonError(w, "User not found", http.StatusNotFound) // anti-enumeration: 404 not 403
		return
	}

	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Only super_admin can promote to super_admin.
	if req.Role != "" {
		newRole := entity.Role(req.Role)
		if newRole == entity.RoleSuperAdmin && !caller.IsSuperAdmin() {
			h.jsonError(w, "Only Super Admin can assign super_admin role", http.StatusForbidden)
			return
		}
		target.Role = newRole
	}

	// Apply optional updates.
	if req.FullName != "" {
		target.FullName = req.FullName
	}
	if req.Email != "" {
		target.Email = req.Email
	}
	if req.IsActive != nil {
		target.IsActive = *req.IsActive
	}
	target.UpdatedBy = caller.ID

	if err := h.repo.UpdateUser(r.Context(), target); err != nil {
		h.logger.Error("Update user failed", zap.Error(err), zap.String("user_id", userID))
		h.jsonError(w, "Failed to update user", http.StatusInternalServerError)
		return
	}

	h.logger.Info("User updated",
		zap.String("user_id", userID),
		zap.String("updated_by", caller.ID),
		zap.String("org_id", orgID),
	)

	h.audit(r, "update_user", "user", userID, map[string]interface{}{
		"fullName": target.FullName, "email": target.Email,
		"role": string(target.Role), "isActive": target.IsActive,
		"orgId": orgID,
	})

	h.jsonResponse(w, target, http.StatusOK)
}

// --------------------------------------------------------------------------
// Deactivate (Soft-Delete) User
// --------------------------------------------------------------------------

func (h *AdminHandler) handleDeactivateUser(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	userID := r.PathValue("userId")
	caller := getUserFromContext(r.Context())

	if caller == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Self-deactivation guard.
	if caller.ID == userID {
		h.jsonError(w, "Cannot deactivate your own account", http.StatusBadRequest)
		return
	}

	// Fetch target user.
	target, err := h.repo.GetUserByID(r.Context(), userID)
	if err != nil || target == nil {
		h.jsonError(w, "User not found", http.StatusNotFound)
		return
	}

	// Org isolation.
	if !caller.IsSuperAdmin() && target.OrgID != orgID {
		h.jsonError(w, "User not found", http.StatusNotFound) // anti-enumeration
		return
	}

	// Non-super_admin cannot deactivate super_admin.
	if target.IsSuperAdmin() && !caller.IsSuperAdmin() {
		h.jsonError(w, "Insufficient permissions", http.StatusForbidden)
		return
	}

	// Soft delete — sets deleted_at, user no longer appears in queries.
	if err := h.repo.SoftDeleteUser(r.Context(), userID); err != nil {
		h.logger.Error("Deactivate user failed", zap.Error(err), zap.String("user_id", userID))
		h.jsonError(w, "Failed to deactivate user", http.StatusInternalServerError)
		return
	}

	h.logger.Info("User deactivated",
		zap.String("user_id", userID),
		zap.String("deactivated_by", caller.ID),
		zap.String("org_id", orgID),
	)

	h.audit(r, "deactivate_user", "user", userID, map[string]interface{}{
		"phone": target.Phone, "fullName": target.FullName,
		"role": string(target.Role), "orgId": orgID,
		"deactivatedBy": caller.ID,
	})

	h.jsonResponse(w, map[string]string{"status": "deactivated", "userId": userID}, http.StatusOK)
}

// ==========================================================================
// API Key Endpoints
// ==========================================================================

func (h *AdminHandler) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	caller := getUserFromContext(r.Context())

	if !caller.IsSuperAdmin() && caller.OrgID != orgID {
		h.jsonError(w, "Access denied", http.StatusForbidden)
		return
	}

	keys, err := h.repo.ListAPIKeysByOrg(r.Context(), orgID)
	if err != nil {
		h.logger.Error("List API keys failed", zap.Error(err))
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	h.jsonResponse(w, keys, http.StatusOK)
}

type createAPIKeyRequest struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
	Environment string   `json:"environment"` // "live" or "test"
}

type createAPIKeyResponse struct {
	Key    *entity.APIKey `json:"key"`
	Secret string         `json:"secret"` // Shown ONCE, never stored.
}

func (h *AdminHandler) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	caller := getUserFromContext(r.Context())

	var req createAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		h.jsonError(w, "Name is required", http.StatusBadRequest)
		return
	}

	env := "live"
	if req.Environment == "test" {
		env = "test"
	}

	// Generate key pair.
	keyID, rawSecret, err := postgres.GenerateAPIKeyPair(env)
	if err != nil {
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Hash the secret.
	secretHash, err := bcrypt.GenerateFromPassword([]byte(rawSecret), 12)
	if err != nil {
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	permissions := req.Permissions
	if len(permissions) == 0 {
		permissions = []string{"events:write", "events:read"}
	}

	apiKey := &entity.APIKey{
		OrgID:        orgID,
		Name:         req.Name,
		KeyID:        keyID,
		SecretHash:   string(secretHash),
		SecretPrefix: rawSecret[:12], // First 12 chars for identification.
		Permissions:  permissions,
		IsActive:     true,
		CreatedBy:    caller.ID,
		UpdatedBy:    caller.ID,
	}

	if err := h.repo.CreateAPIKey(r.Context(), apiKey); err != nil {
		h.logger.Error("Create API key failed", zap.Error(err))
		h.jsonError(w, "Failed to create API key", http.StatusInternalServerError)
		return
	}

	h.logger.Info("API key created",
		zap.String("key_id", keyID),
		zap.String("org_id", orgID),
		zap.String("name", req.Name),
	)

	h.audit(r, "create_api_key", "api_key", keyID, map[string]interface{}{
		"keyId": keyID, "orgId": orgID, "name": req.Name,
		"environment": env, "permissions": permissions,
	})

	// Return the raw secret — this is the ONLY time it's available.
	h.jsonResponse(w, createAPIKeyResponse{
		Key:    apiKey,
		Secret: rawSecret,
	}, http.StatusCreated)
}

func (h *AdminHandler) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	keyID := r.PathValue("keyId")

	caller := getUserFromContext(r.Context())
	if caller == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// ── Org isolation: verify the API key belongs to the caller's org ──────
	// Fetch the key first to check org ownership before revoking.
	apiKey, err := h.repo.GetAPIKeyByKeyID(r.Context(), keyID)
	if err != nil || apiKey == nil {
		h.jsonError(w, "API key not found", http.StatusNotFound)
		return
	}

	if !caller.IsSuperAdmin() && caller.OrgID != apiKey.OrgID {
		h.jsonError(w, "Access denied", http.StatusForbidden)
		return
	}

	if err := h.repo.RevokeAPIKey(r.Context(), keyID); err != nil {
		h.logger.Error("Revoke API key failed", zap.Error(err))
		h.jsonError(w, "Failed to revoke API key", http.StatusInternalServerError)
		return
	}

	h.audit(r, "revoke_api_key", "api_key", keyID, map[string]string{
		"keyId": keyID,
		"orgId": apiKey.OrgID,
	})

	h.jsonResponse(w, map[string]string{"status": "revoked"}, http.StatusOK)
}

// ==========================================================================
// Webhook Endpoints (Admin — JWT-authenticated)
// ==========================================================================

func (h *AdminHandler) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	caller := getUserFromContext(r.Context())

	// Org isolation: non-super-admins can only see their own org.
	if !caller.IsSuperAdmin() && caller.OrgID != orgID {
		h.jsonError(w, "Access denied", http.StatusForbidden)
		return
	}

	endpoints, err := h.repo.GetWebhookEndpointsByOrg(r.Context(), orgID)
	if err != nil {
		h.logger.Error("List webhooks failed", zap.Error(err), zap.String("org_id", orgID))
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	h.jsonResponse(w, endpoints, http.StatusOK)
}

type createAdminWebhookRequest struct {
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Events []string `json:"events,omitempty"` // Default: all events.
}

type createAdminWebhookResponse struct {
	Endpoint *entity.WebhookEndpoint `json:"endpoint"`
	Secret   string                  `json:"secret"` // Shown ONCE, never stored.
}

func (h *AdminHandler) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	caller := getUserFromContext(r.Context())

	var req createAdminWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.URL) == "" {
		h.jsonError(w, "URL is required", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(req.URL, "https://") && !strings.HasPrefix(req.URL, "http://") {
		h.jsonError(w, "Webhook URL must use HTTP or HTTPS", http.StatusBadRequest)
		return
	}

	name := req.Name
	if name == "" {
		name = "Default Webhook"
	}

	// Default to all events if not specified.
	events := req.Events
	if len(events) == 0 {
		events = []string{"session.started", "session.completed", "violation.detected", "verdict.ready"}
	}
	// Validate event types.
	for _, e := range events {
		if !entity.ValidWebhookEvents[e] {
			h.jsonError(w, "Invalid event type: "+e, http.StatusBadRequest)
			return
		}
	}

	// Generate HMAC signing secret (reuse function from external_handler.go).
	secret, err := generateWebhookSecret()
	if err != nil {
		h.logger.Error("Failed to generate webhook secret", zap.Error(err))
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	endpoint := &entity.WebhookEndpoint{
		OrgID:    orgID,
		Name:     name,
		URL:      req.URL,
		Secret:   secret,
		Events:   events,
		IsActive: true,
	}

	if err := h.repo.CreateWebhookEndpoint(r.Context(), endpoint); err != nil {
		h.logger.Error("Create webhook failed", zap.Error(err), zap.String("org_id", orgID))
		h.jsonError(w, "Failed to create webhook", http.StatusInternalServerError)
		return
	}

	h.logger.Info("Webhook endpoint created via admin API",
		zap.String("org_id", orgID),
		zap.String("endpoint_id", endpoint.ID),
		zap.String("url", req.URL),
		zap.String("created_by", caller.ID),
	)

	h.audit(r, "create_webhook", "webhook_endpoint", endpoint.ID, map[string]interface{}{
		"orgId": orgID, "url": req.URL, "name": name, "events": events,
	})

	// Return the secret — this is the ONLY time it's available.
	h.jsonResponse(w, createAdminWebhookResponse{
		Endpoint: endpoint,
		Secret:   secret,
	}, http.StatusCreated)
}

func (h *AdminHandler) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	webhookID := r.PathValue("webhookId")
	caller := getUserFromContext(r.Context())
	if caller == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Fetch the endpoint to verify org ownership (anti-enumeration: 404 not 403).
	endpoint, err := h.repo.GetWebhookEndpointByID(r.Context(), webhookID)
	if err != nil || endpoint == nil {
		h.jsonError(w, "Webhook not found", http.StatusNotFound)
		return
	}

	if !caller.IsSuperAdmin() && caller.OrgID != endpoint.OrgID {
		// Anti-enumeration: don't reveal that the webhook exists for another org.
		h.jsonError(w, "Webhook not found", http.StatusNotFound)
		return
	}

	if err := h.repo.DeleteWebhookEndpoint(r.Context(), webhookID); err != nil {
		h.logger.Error("Delete webhook failed", zap.Error(err), zap.String("webhook_id", webhookID))
		h.jsonError(w, "Failed to delete webhook", http.StatusInternalServerError)
		return
	}

	h.audit(r, "delete_webhook", "webhook_endpoint", webhookID, map[string]string{
		"webhookId": webhookID,
		"orgId":     endpoint.OrgID,
	})

	h.jsonResponse(w, map[string]string{"status": "deleted"}, http.StatusOK)
}

// ==========================================================================
// Stats Endpoint (Super Admin Only)
// ==========================================================================

type adminStats struct {
	TotalOrganizations int `json:"totalOrganizations"`
	TotalUsers         int `json:"totalUsers"`
	TotalAPIKeys       int `json:"totalApiKeys"`
}

func (h *AdminHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	orgCount, err := h.repo.CountOrgs(r.Context())
	if err != nil {
		h.logger.Error("Failed to count organizations", zap.Error(err))
		h.jsonError(w, "Failed to load statistics", http.StatusInternalServerError)
		return
	}

	allUsers, err := h.repo.ListAllUsers(r.Context(), port.UserFilter{})
	if err != nil {
		h.logger.Error("Failed to list users", zap.Error(err))
		h.jsonError(w, "Failed to load statistics", http.StatusInternalServerError)
		return
	}

	allKeys, err := h.repo.ListAllAPIKeys(r.Context())
	if err != nil {
		h.logger.Error("Failed to list API keys", zap.Error(err))
		h.jsonError(w, "Failed to load statistics", http.StatusInternalServerError)
		return
	}

	stats := adminStats{
		TotalOrganizations: orgCount,
		TotalUsers:         len(allUsers),
		TotalAPIKeys:       len(allKeys),
	}

	h.jsonResponse(w, stats, http.StatusOK)
}

// ==========================================================================
// Auth Middleware
// ==========================================================================

type contextKey string

const userContextKey contextKey = "user"

func getUserFromContext(ctx context.Context) *entity.User {
	user, _ := ctx.Value(userContextKey).(*entity.User)
	return user
}

// requireAuth middleware validates the JWT token and injects the user into context.
func (h *AdminHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract token from Authorization header or cookie.
		token := ""
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}

		if token == "" {
			h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Verify token and get user.
		user, err := h.verifyToken(r.Context(), token)
		if err != nil {
			h.jsonError(w, "Токен недействителен или устарел", http.StatusUnauthorized)
			return
		}

		// Inject user into context.
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// requireRole middleware checks if the user has the required role.
func (h *AdminHandler) requireRole(role entity.Role, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := getUserFromContext(r.Context())
		if user == nil || (user.Role != role && !user.IsSuperAdmin()) {
			h.jsonError(w, "Insufficient permissions", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}
}

// requireOrgAdmin middleware checks if the user is at least an org_admin.
func (h *AdminHandler) requireOrgAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := getUserFromContext(r.Context())
		if user == nil || !user.Role.CanManageUsers() {
			h.jsonError(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		// Org admins can only manage their own org.
		orgID := r.PathValue("orgId")
		if orgID != "" && !user.IsSuperAdmin() && user.OrgID != orgID {
			h.jsonError(w, "Access denied to this organization", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	}
}

// ==========================================================================
// JWT Token Generation & Verification (Simplified for Admin API)
// ==========================================================================

// generateToken creates a simple JWT for the admin API.
// Uses HMAC-SHA256 with the same signing key as the event collector.
func (h *AdminHandler) generateToken(user *entity.User) (string, error) {
	// Use the existing auth package for token generation.
	// For simplicity, we create a base64-encoded JSON payload with HMAC signature.
	payload := map[string]interface{}{
		"sub":    user.ID,
		"phone":  user.Phone,
		"org_id": user.OrgID,
		"role":   string(user.Role),
		"name":   user.FullName,
		"iat":    time.Now().Unix(),
		"exp":    time.Now().Add(24 * time.Hour).Unix(),
		"iss":    "argus-admin",
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	// Create JWT manually: header.payload.signature
	header := `{"alg":"HS256","typ":"JWT"}`
	headerB64 := base64URLEncode([]byte(header))
	payloadB64 := base64URLEncode(payloadJSON)

	signingInput := headerB64 + "." + payloadB64
	signature := hmacSHA256([]byte(signingInput), h.jwtSigningKey)
	signatureB64 := base64URLEncode(signature)

	return signingInput + "." + signatureB64, nil
}

// verifyToken validates a JWT and returns the associated user.
func (h *AdminHandler) verifyToken(ctx context.Context, tokenStr string) (*entity.User, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Verify signature.
	signingInput := parts[0] + "." + parts[1]
	expectedSig := hmacSHA256([]byte(signingInput), h.jwtSigningKey)
	actualSig, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding")
	}

	if !hmacEqual(expectedSig, actualSig) {
		return nil, fmt.Errorf("invalid signature")
	}

	// Decode payload.
	payloadJSON, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid payload encoding")
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, fmt.Errorf("invalid payload JSON")
	}

	// Check expiration.
	exp, ok := claims["exp"].(float64)
	if !ok || time.Unix(int64(exp), 0).Before(time.Now()) {
		return nil, fmt.Errorf("token expired")
	}

	// Look up user from database to get fresh data.
	userID, _ := claims["sub"].(string)
	if userID == "" {
		return nil, fmt.Errorf("missing subject claim")
	}

	user, err := h.repo.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return nil, fmt.Errorf("user not found")
	}

	return user, nil
}

// ==========================================================================
// Crypto Helpers
// ==========================================================================

func hmacSHA256(message, key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	return mac.Sum(nil)
}

func hmacEqual(a, b []byte) bool {
	return hmac.Equal(a, b)
}

func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// ==========================================================================
// JSON Response Helpers
// ==========================================================================

func (h *AdminHandler) jsonResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("Failed to encode JSON response", zap.Error(err))
	}
}

func (h *AdminHandler) jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		h.logger.Error("Failed to encode JSON error response", zap.Error(err))
	}
}

// ==========================================================================
// Audit Logging
// ==========================================================================

// audit records an administrative action asynchronously.
// It extracts the caller from context and client metadata from the request,
// then inserts an audit entry in a background goroutine so it never slows
// down the HTTP response.
func (h *AdminHandler) audit(r *http.Request, action, resourceType, resourceID string, details interface{}) {
	caller := getUserFromContext(r.Context())
	if caller == nil {
		return
	}

	entry := &entity.AuditEntry{
		UserID:       caller.ID,
		UserPhone:    caller.Phone,
		UserRole:     string(caller.Role),
		OrgID:        caller.OrgID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      details,
		IPAddress:    extractClientIP(r),
		UserAgent:    r.UserAgent(),
	}

	// Fire-and-forget: audit logging must never block the response.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := h.repo.CreateAuditEntry(ctx, entry); err != nil {
			h.logger.Error("audit log insert failed",
				zap.String("action", action),
				zap.String("resource_type", resourceType),
				zap.String("resource_id", resourceID),
				zap.Error(err),
			)
		}
	}()
}

// extractClientIP returns the client's real IP address, considering proxies.
func extractClientIP(r *http.Request) string {
	// Check X-Forwarded-For (set by reverse proxies/load balancers).
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP (original client).
		if parts := strings.SplitN(xff, ",", 2); len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	// Check X-Real-IP (set by Nginx).
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to RemoteAddr (strip port).
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ==========================================================================
// Audit Log Query Endpoint
// ==========================================================================

func (h *AdminHandler) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	filter := postgres.AuditFilter{
		OrgID:        r.URL.Query().Get("org_id"),
		UserID:       r.URL.Query().Get("user_id"),
		Action:       r.URL.Query().Get("action"),
		ResourceType: r.URL.Query().Get("resource_type"),
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			filter.Limit = parsed
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			filter.Offset = parsed
		}
	}

	entries, err := h.repo.ListAuditLogs(r.Context(), filter)
	if err != nil {
		h.logger.Error("List audit logs failed", zap.Error(err))
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, entries, http.StatusOK)
}
