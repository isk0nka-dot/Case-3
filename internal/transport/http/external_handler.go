// Package http provides the External API handler for third-party SaaS integrations.
//
// These endpoints allow external partners to create proctoring sessions,
// retrieve session verdicts, and manage webhook subscriptions. Authentication
// uses API keys (X-CLIENT-ID + X-API-KEY headers) instead of JWT tokens.
//
// URL structure:
//
//	POST   /api/v1/external/sessions                      — Create proctoring session
//	GET    /api/v1/external/sessions/{sessionId}           — Get session status/verdict
//	POST   /api/v1/external/sessions/{sessionId}/complete  — Complete session with verdict
//	POST   /api/v1/external/sessions/{sessionId}/cancel    — Cancel session
//	GET    /api/v1/external/webhooks                       — List webhook endpoints
//	POST   /api/v1/external/webhooks                       — Register webhook endpoint
//	DELETE /api/v1/external/webhooks/{webhookId}           — Delete webhook endpoint
package http

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
	"github.com/argus-ai/event-collector/pkg/auth"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// ==========================================================================
// API Key Context
// ==========================================================================

type apiKeyContextKey struct{}

// APIKeyContext holds the validated API key information injected into the
// request context by requireAPIKey middleware.
type APIKeyContext struct {
	OrgID        string
	KeyID        string
	Permissions  []string
	RateLimitRPS int
}

func getAPIKeyFromContext(ctx context.Context) *APIKeyContext {
	v, _ := ctx.Value(apiKeyContextKey{}).(*APIKeyContext)
	return v
}

// ==========================================================================
// External Handler
// ==========================================================================

// ExternalHandler serves the REST API for external/SaaS partner integrations.
type ExternalHandler struct {
	repo          *postgres.Repository
	logger        *zap.Logger
	jwtSigningKey []byte
}

// NewExternalHandler creates a new external API handler.
func NewExternalHandler(repo *postgres.Repository, logger *zap.Logger, jwtSigningKey []byte) *ExternalHandler {
	return &ExternalHandler{
		repo:          repo,
		logger:        logger.Named("external_api"),
		jwtSigningKey: jwtSigningKey,
	}
}

// RegisterRoutes registers external API endpoints on the given mux.
func (h *ExternalHandler) RegisterRoutes(mux *http.ServeMux) {
	// Session management (API key auth).
	mux.HandleFunc("POST /api/v1/external/sessions", h.requireAPIKey("sessions:write", h.handleCreateSession))
	mux.HandleFunc("GET /api/v1/external/sessions/{sessionId}", h.requireAPIKey("sessions:read", h.handleGetSession))
	mux.HandleFunc("POST /api/v1/external/sessions/{sessionId}/complete", h.requireAPIKey("sessions:write", h.handleCompleteSession))
	mux.HandleFunc("POST /api/v1/external/sessions/{sessionId}/cancel", h.requireAPIKey("sessions:write", h.handleCancelSession))

	// Webhook management (API key auth).
	mux.HandleFunc("GET /api/v1/external/webhooks", h.requireAPIKey("webhooks:read", h.handleListWebhooks))
	mux.HandleFunc("POST /api/v1/external/webhooks", h.requireAPIKey("webhooks:write", h.handleCreateWebhook))
	mux.HandleFunc("DELETE /api/v1/external/webhooks/{webhookId}", h.requireAPIKey("webhooks:write", h.handleDeleteWebhook))
}

// ==========================================================================
// API Key Authentication Middleware
// ==========================================================================

// requireAPIKey validates X-CLIENT-ID and X-API-KEY headers against the
// api_keys table. The API key must be active, not expired, and have the
// required permission.
func (h *ExternalHandler) requireAPIKey(requiredPerm string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientID := r.Header.Get("X-CLIENT-ID")
		apiSecret := r.Header.Get("X-API-KEY")

		if clientID == "" || apiSecret == "" {
			h.jsonError(w, "Missing X-CLIENT-ID or X-API-KEY header", http.StatusUnauthorized)
			return
		}

		// Look up API key by key_id (the public identifier).
		apiKey, err := h.repo.GetAPIKeyByKeyID(r.Context(), clientID)
		if err != nil || apiKey == nil {
			h.jsonError(w, "Invalid API credentials", http.StatusUnauthorized)
			return
		}

		// Check if active and not deleted.
		if !apiKey.IsActive {
			h.jsonError(w, "API key is deactivated", http.StatusUnauthorized)
			return
		}

		// Check expiration.
		if apiKey.IsExpired() {
			h.jsonError(w, "API key has expired", http.StatusUnauthorized)
			return
		}

		// Verify secret via bcrypt.
		if err := bcrypt.CompareHashAndPassword([]byte(apiKey.SecretHash), []byte(apiSecret)); err != nil {
			h.jsonError(w, "Invalid API credentials", http.StatusUnauthorized)
			return
		}

		// Check required permission.
		if !hasPermission(apiKey.Permissions, requiredPerm) {
			h.jsonError(w, "Insufficient permissions", http.StatusForbidden)
			return
		}

		// Update last_used_at asynchronously (fire-and-forget).
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := h.repo.UpdateAPIKeyLastUsed(ctx, apiKey.KeyID); err != nil {
				h.logger.Warn("Failed to update API key last_used_at",
					zap.Error(err), zap.String("key_id", apiKey.KeyID))
			}
		}()

		// Inject API key context for downstream handlers.
		ctx := context.WithValue(r.Context(), apiKeyContextKey{}, &APIKeyContext{
			OrgID:        apiKey.OrgID,
			KeyID:        apiKey.KeyID,
			Permissions:  apiKey.Permissions,
			RateLimitRPS: apiKey.RateLimitRPS,
		})

		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// hasPermission checks if the permissions list contains the required permission.
// Supports wildcard: ["*"] grants all permissions.
func hasPermission(perms []string, required string) bool {
	for _, p := range perms {
		if p == "*" || p == required {
			return true
		}
		// Support category wildcard: "sessions:*" matches "sessions:write".
		if strings.HasSuffix(p, ":*") {
			prefix := strings.TrimSuffix(p, "*")
			if strings.HasPrefix(required, prefix) {
				return true
			}
		}
	}
	return false
}

// ==========================================================================
// Session Endpoints
// ==========================================================================

type createSessionRequest struct {
	ExamID      string          `json:"examId"`
	StudentID   string          `json:"studentId"`
	StudentName string          `json:"studentName,omitempty"`
	ExamName    string          `json:"examName,omitempty"`
	CallbackURL string          `json:"callbackUrl,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	DurationMin int             `json:"durationMin,omitempty"` // Session duration in minutes (default: 240).
}

type createSessionResponse struct {
	SessionID         string `json:"sessionId"`
	ArgusSessionToken string `json:"argusSessionToken"`
	SDKUrl            string `json:"sdkUrl"`
	ExpiresAt         string `json:"expiresAt"`
}

func (h *ExternalHandler) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	apiCtx := getAPIKeyFromContext(r.Context())
	if apiCtx == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields.
	if strings.TrimSpace(req.ExamID) == "" {
		h.jsonError(w, "examId is required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.StudentID) == "" {
		h.jsonError(w, "studentId is required", http.StatusBadRequest)
		return
	}

	// Generate unique session ID.
	sessionID := generateSessionID()

	// Token duration: default 4 hours, or custom if provided.
	durationMin := 240
	if req.DurationMin > 0 && req.DurationMin <= 480 {
		durationMin = req.DurationMin
	}
	tokenExpiry := time.Now().Add(time.Duration(durationMin) * time.Minute)

	// Generate scoped JWT using the existing auth package.
	claims := &auth.ProctoringClaims{
		Subject:   req.StudentID,
		Issuer:    "argus-external-api",
		Audience:  []string{"argus-event-collector"},
		ExpiresAt: tokenExpiry,
		IssuedAt:  time.Now(),
		JTI:       sessionID,
		SessionID: sessionID,
		StudentID: req.StudentID,
		OrgID:     apiCtx.OrgID,
		ExamID:    req.ExamID,
		Roles:     []string{"student"},
	}

	sessionToken, err := auth.GenerateToken(claims, h.jwtSigningKey)
	if err != nil {
		h.logger.Error("Failed to generate session token", zap.Error(err))
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Build the session entity.
	session := &entity.ExternalSession{
		SessionID:      sessionID,
		OrgID:          apiCtx.OrgID,
		ExamID:         req.ExamID,
		StudentID:      req.StudentID,
		StudentName:    req.StudentName,
		ExamName:       req.ExamName,
		CallbackURL:    req.CallbackURL,
		Metadata:       req.Metadata,
		SessionToken:   sessionToken,
		TokenExpiresAt: tokenExpiry,
		Status:         "created",
	}

	if err := h.repo.CreateExternalSession(r.Context(), session); err != nil {
		h.logger.Error("Failed to create external session",
			zap.Error(err),
			zap.String("org_id", apiCtx.OrgID),
			zap.String("exam_id", req.ExamID),
		)
		h.jsonError(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	h.logger.Info("External session created",
		zap.String("session_id", sessionID),
		zap.String("org_id", apiCtx.OrgID),
		zap.String("exam_id", req.ExamID),
		zap.String("student_id", req.StudentID),
		zap.String("api_key", apiCtx.KeyID),
	)

	h.jsonResponse(w, createSessionResponse{
		SessionID:         sessionID,
		ArgusSessionToken: sessionToken,
		SDKUrl:            "https://cdn.argusai.kz/sdk/v1/argus-sdk.umd.js",
		ExpiresAt:         tokenExpiry.UTC().Format(time.RFC3339),
	}, http.StatusCreated)
}

func (h *ExternalHandler) handleGetSession(w http.ResponseWriter, r *http.Request) {
	apiCtx := getAPIKeyFromContext(r.Context())
	if apiCtx == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID := r.PathValue("sessionId")

	session, err := h.repo.GetExternalSessionByID(r.Context(), sessionID)
	if err != nil || session == nil {
		// Anti-enumeration: return 404 for both "not found" and "wrong org".
		h.jsonError(w, "Session not found", http.StatusNotFound)
		return
	}

	// Org isolation: verify the session belongs to the caller's org.
	if session.OrgID != apiCtx.OrgID {
		h.jsonError(w, "Session not found", http.StatusNotFound)
		return
	}

	h.jsonResponse(w, session, http.StatusOK)
}

type completeSessionRequest struct {
	Verdict        string          `json:"verdict"`        // clean, suspicious, violation
	IntegrityScore float64         `json:"integrityScore"` // 0.0–100.0
	ViolationCount int             `json:"violationCount"`
	VerdictDetails json.RawMessage `json:"verdictDetails,omitempty"`
}

func (h *ExternalHandler) handleCompleteSession(w http.ResponseWriter, r *http.Request) {
	apiCtx := getAPIKeyFromContext(r.Context())
	if apiCtx == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID := r.PathValue("sessionId")

	session, err := h.repo.GetExternalSessionByID(r.Context(), sessionID)
	if err != nil || session == nil || session.OrgID != apiCtx.OrgID {
		h.jsonError(w, "Session not found", http.StatusNotFound)
		return
	}

	if session.IsTerminal() {
		h.jsonError(w, "Session is already in terminal state: "+session.Status, http.StatusConflict)
		return
	}

	var req completeSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if !entity.ValidVerdicts[req.Verdict] {
		h.jsonError(w, "Invalid verdict (must be: clean, suspicious, violation)", http.StatusBadRequest)
		return
	}

	if err := h.repo.CompleteExternalSession(r.Context(), sessionID, req.Verdict, req.VerdictDetails, req.IntegrityScore, req.ViolationCount); err != nil {
		h.logger.Error("Failed to complete session", zap.Error(err), zap.String("session_id", sessionID))
		h.jsonError(w, "Failed to complete session", http.StatusInternalServerError)
		return
	}

	h.logger.Info("External session completed",
		zap.String("session_id", sessionID),
		zap.String("verdict", req.Verdict),
		zap.Float64("integrity_score", req.IntegrityScore),
	)

	// Enqueue webhook: verdict.ready
	h.enqueueWebhook(r.Context(), apiCtx.OrgID, "verdict.ready", map[string]interface{}{
		"sessionId":      sessionID,
		"verdict":        req.Verdict,
		"integrityScore": req.IntegrityScore,
		"violationCount": req.ViolationCount,
	})

	h.jsonResponse(w, map[string]string{"status": "completed", "verdict": req.Verdict}, http.StatusOK)
}

func (h *ExternalHandler) handleCancelSession(w http.ResponseWriter, r *http.Request) {
	apiCtx := getAPIKeyFromContext(r.Context())
	if apiCtx == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID := r.PathValue("sessionId")

	session, err := h.repo.GetExternalSessionByID(r.Context(), sessionID)
	if err != nil || session == nil || session.OrgID != apiCtx.OrgID {
		h.jsonError(w, "Session not found", http.StatusNotFound)
		return
	}

	if session.IsTerminal() {
		h.jsonError(w, "Session is already in terminal state: "+session.Status, http.StatusConflict)
		return
	}

	if err := h.repo.UpdateExternalSessionStatus(r.Context(), sessionID, "cancelled"); err != nil {
		h.logger.Error("Failed to cancel session", zap.Error(err))
		h.jsonError(w, "Failed to cancel session", http.StatusInternalServerError)
		return
	}

	h.logger.Info("External session cancelled",
		zap.String("session_id", sessionID),
		zap.String("org_id", apiCtx.OrgID),
	)

	h.jsonResponse(w, map[string]string{"status": "cancelled"}, http.StatusOK)
}

// ==========================================================================
// Webhook Endpoints
// ==========================================================================

type createWebhookRequest struct {
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Events []string `json:"events,omitempty"` // Default: all events.
}

type createWebhookResponse struct {
	Endpoint *entity.WebhookEndpoint `json:"endpoint"`
	Secret   string                  `json:"secret"` // Shown ONCE.
}

func (h *ExternalHandler) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	apiCtx := getAPIKeyFromContext(r.Context())
	if apiCtx == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	endpoints, err := h.repo.GetWebhookEndpointsByOrg(r.Context(), apiCtx.OrgID)
	if err != nil {
		h.logger.Error("Failed to list webhooks", zap.Error(err))
		h.jsonError(w, "Failed to list webhooks", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]interface{}{"endpoints": endpoints}, http.StatusOK)
}

func (h *ExternalHandler) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	apiCtx := getAPIKeyFromContext(r.Context())
	if apiCtx == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req createWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.URL) == "" {
		h.jsonError(w, "url is required", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(req.URL, "https://") {
		h.jsonError(w, "Webhook URL must use HTTPS", http.StatusBadRequest)
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

	// Generate HMAC signing secret.
	secret, err := generateWebhookSecret()
	if err != nil {
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	endpoint := &entity.WebhookEndpoint{
		OrgID:    apiCtx.OrgID,
		Name:     name,
		URL:      req.URL,
		Secret:   secret,
		Events:   events,
		IsActive: true,
	}

	if err := h.repo.CreateWebhookEndpoint(r.Context(), endpoint); err != nil {
		h.logger.Error("Failed to create webhook", zap.Error(err))
		h.jsonError(w, "Failed to create webhook", http.StatusInternalServerError)
		return
	}

	h.logger.Info("Webhook endpoint created",
		zap.String("org_id", apiCtx.OrgID),
		zap.String("endpoint_id", endpoint.ID),
		zap.String("url", req.URL),
	)

	// Return the secret — this is the ONLY time it's available.
	h.jsonResponse(w, createWebhookResponse{
		Endpoint: endpoint,
		Secret:   secret,
	}, http.StatusCreated)
}

func (h *ExternalHandler) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	apiCtx := getAPIKeyFromContext(r.Context())
	if apiCtx == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	webhookID := r.PathValue("webhookId")

	// Verify org ownership before deleting (anti-enumeration).
	endpoint, err := h.repo.GetWebhookEndpointByID(r.Context(), webhookID)
	if err != nil || endpoint == nil || endpoint.OrgID != apiCtx.OrgID {
		h.jsonError(w, "Webhook not found", http.StatusNotFound)
		return
	}

	if err := h.repo.DeleteWebhookEndpoint(r.Context(), webhookID); err != nil {
		h.logger.Error("Failed to delete webhook", zap.Error(err))
		h.jsonError(w, "Failed to delete webhook", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]string{"status": "deleted"}, http.StatusOK)
}

// ==========================================================================
// Webhook Dispatch (Internal)
// ==========================================================================

// enqueueWebhook creates webhook delivery entries for all endpoints registered
// for the given event type. Delivery is handled by the background dispatcher.
func (h *ExternalHandler) enqueueWebhook(ctx context.Context, orgID, eventType string, payload interface{}) {
	endpoints, err := h.repo.GetWebhookEndpointsByOrgAndEvent(ctx, orgID, eventType)
	if err != nil {
		h.logger.Error("Failed to get webhook endpoints",
			zap.Error(err), zap.String("org_id", orgID), zap.String("event_type", eventType))
		return
	}
	if len(endpoints) == 0 {
		return
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error("Failed to marshal webhook payload", zap.Error(err))
		return
	}

	for _, ep := range endpoints {
		delivery := &entity.WebhookDelivery{
			EndpointID:  ep.ID,
			OrgID:       orgID,
			EventType:   eventType,
			Payload:     payloadJSON,
			Status:      "pending",
			MaxAttempts: 5,
			NextRetryAt: time.Now(),
		}
		if err := h.repo.CreateWebhookDelivery(ctx, delivery); err != nil {
			h.logger.Error("Failed to enqueue webhook delivery",
				zap.Error(err),
				zap.String("endpoint_id", ep.ID),
				zap.String("event_type", eventType),
			)
		}
	}
}

// ==========================================================================
// Helpers
// ==========================================================================

func (h *ExternalHandler) jsonResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("Failed to encode JSON response", zap.Error(err))
	}
}

func (h *ExternalHandler) jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		h.logger.Error("Failed to encode JSON error response", zap.Error(err))
	}
}

// generateSessionID creates a unique session identifier with argus_ prefix.
func generateSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID (extremely unlikely).
		return fmt.Sprintf("argus_ses_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("argus_ses_%s", hex.EncodeToString(b))
}

// generateWebhookSecret creates a cryptographically random HMAC signing secret.
func generateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("whsec_%s", hex.EncodeToString(b)), nil
}

// SignWebhookPayload creates an HMAC-SHA256 signature for webhook delivery.
// Partners verify this signature to ensure the webhook came from Argus.
//
// Signature format: HMAC-SHA256(secret, timestamp + "." + payload)
// Header: X-Argus-Signature: sha256=<hex_digest>
func SignWebhookPayload(secret string, timestamp int64, payload []byte) string {
	message := fmt.Sprintf("%d.%s", timestamp, string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}
