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
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/argus-ai/event-collector/internal/application/usecase"
	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/domain/valueobject"
	"github.com/argus-ai/event-collector/internal/infrastructure/forensic"
	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
	"github.com/argus-ai/event-collector/internal/infrastructure/worker"
	"github.com/argus-ai/event-collector/pkg/auth"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
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

// ExternalEgressController auto-starts video recording when a student joins.
type ExternalEgressController interface {
	StartParticipantRecording(ctx context.Context, roomName, identity, sessionID, userID string) (string, error)
}

// ExternalHandler serves the REST API for external/SaaS partner integrations.
type ExternalHandler struct {
	repo             *postgres.Repository
	logger           *zap.Logger
	jwtSigningKey    []byte
	ingest           *usecase.IngestUseCase
	chConn           driver.Conn
	livekitAPIKey    string
	livekitAPISecret string
	livekitPublicURL string
	sdkURL           string
	egress           ExternalEgressController // nil until SetEgress is called
	asynqClient      *asynq.Client            // nil if worker not configured
}

// SetEgress wires in the LiveKit Egress service for auto-recording.
func (h *ExternalHandler) SetEgress(e ExternalEgressController) { h.egress = e }

// SetAnalyticsConn wires ClickHouse for external JSON reports.
func (h *ExternalHandler) SetAnalyticsConn(conn driver.Conn) { h.chConn = conn }

// SetAsynqClient wires in the asynq client for background enrollment tasks.
func (h *ExternalHandler) SetAsynqClient(c *asynq.Client) { h.asynqClient = c }

// NewExternalHandler creates a new external API handler.
func NewExternalHandler(repo *postgres.Repository, logger *zap.Logger, jwtSigningKey []byte, ingest *usecase.IngestUseCase) *ExternalHandler {
	lkAPIKey := os.Getenv("LIVEKIT_API_KEY")
	if lkAPIKey == "" {
		lkAPIKey = "argus-dev-api-key"
	}
	lkAPISecret := os.Getenv("LIVEKIT_API_SECRET")
	if lkAPISecret == "" {
		lkAPISecret = "argus-dev-api-secret-must-be-at-least-32-characters-long"
	}
	lkPublicURL := os.Getenv("LIVEKIT_PUBLIC_WS_URL")
	if lkPublicURL == "" {
		lkPublicURL = os.Getenv("LIVEKIT_WS_URL")
		if lkPublicURL == "" {
			lkPublicURL = "ws://localhost:7880"
		}
	}
	sdkURL := os.Getenv("ARGUS_SDK_URL")
	if sdkURL == "" {
		sdkURL = os.Getenv("ARGUS_PUBLIC_SDK_URL")
	}
	if sdkURL == "" {
		sdkURL = "https://cdn.argusai.kz/sdk/v1/argus-sdk.umd.js"
	}
	return &ExternalHandler{
		repo:             repo,
		logger:           logger.Named("external_api"),
		jwtSigningKey:    jwtSigningKey,
		ingest:           ingest,
		livekitAPIKey:    lkAPIKey,
		livekitAPISecret: lkAPISecret,
		livekitPublicURL: lkPublicURL,
		sdkURL:           sdkURL,
	}
}

// RegisterRoutes registers external API endpoints on the given mux.
func (h *ExternalHandler) RegisterRoutes(mux *http.ServeMux) {
	// Session management (API key auth).
	mux.HandleFunc("POST /api/v1/external/sessions", h.requireAPIKey("sessions:write", h.handleCreateSession))
	mux.HandleFunc("GET /api/v1/external/sessions/{sessionId}", h.requireAPIKey("sessions:read", h.handleGetSession))
	mux.HandleFunc("GET /api/v1/external/sessions/{sessionId}/report", h.requireAPIKey("sessions:read", h.handleSessionReport))
	mux.HandleFunc("GET /api/v1/external/sessions/{sessionId}/report.pdf", h.requireAPIKey("sessions:read", h.handleSessionReportPDF))
	mux.HandleFunc("POST /api/v1/external/sessions/{sessionId}/complete", h.requireAPIKey("sessions:write", h.handleCompleteSession))
	mux.HandleFunc("POST /api/v1/external/sessions/{sessionId}/cancel", h.requireAPIKey("sessions:write", h.handleCancelSession))

	// Webhook management (API key auth).
	mux.HandleFunc("GET /api/v1/external/webhooks", h.requireAPIKey("webhooks:read", h.handleListWebhooks))
	mux.HandleFunc("POST /api/v1/external/webhooks", h.requireAPIKey("webhooks:write", h.handleCreateWebhook))
	mux.HandleFunc("DELETE /api/v1/external/webhooks/{webhookId}", h.requireAPIKey("webhooks:write", h.handleDeleteWebhook))

	// Sessions by result — query completed sessions for a given exam+student pair.
	mux.HandleFunc("GET /api/v1/external/sessions/by-result", h.requireAPIKey("sessions:read", h.handleListSessionsByResult))

	// Event ingestion (proctoring session JWT auth) — for SDK / test clients.
	mux.HandleFunc("POST /api/v1/external/events", h.requireSessionToken(h.handleIngestEvents))

	// Session heartbeat — keeps session alive, marks session as active on first call.
	mux.HandleFunc("POST /api/v1/external/heartbeat", h.requireSessionToken(h.handleHeartbeat))

	// LiveKit student token (proctoring session JWT auth) — SDK calls this to publish camera.
	mux.HandleFunc("POST /api/v1/external/sessions/{sessionId}/student-token", h.requireSessionToken(h.handleStudentMediaToken))
	// Recording-ready signal — SDK calls this after student successfully joins LiveKit room.
	mux.HandleFunc("POST /api/v1/external/sessions/{sessionId}/recording-ready", h.requireSessionToken(h.handleRecordingReady))
	// Near-real-time AI frame snapshots — SDK samples webcam frames for backend inference.
	mux.HandleFunc("POST /api/v1/external/sessions/{sessionId}/ai-frame", h.requireSessionToken(h.handleAIFrame))
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
	ExamID            string          `json:"examId"`
	StudentID         string          `json:"studentId"`
	StudentName       string          `json:"studentName,omitempty"`
	ExamName          string          `json:"examName,omitempty"`
	CallbackURL       string          `json:"callbackUrl,omitempty"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
	DurationMin       int             `json:"durationMin,omitempty"`       // Session duration in minutes (default: 240).
	ReferencePhotoURL string          `json:"referencePhotoUrl,omitempty"` // LMS-provided student photo for face enrollment.
}

type createSessionResponse struct {
	SessionID          string `json:"sessionId"`
	ArgusSessionToken  string `json:"argusSessionToken"`
	SDKUrl             string `json:"sdkUrl"`
	ExpiresAt          string `json:"expiresAt"`
	EnrollmentRequired bool   `json:"enrollmentRequired"`
	EnrollmentStatus   string `json:"enrollmentStatus"`
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

	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		idempotencyKey = strings.TrimSpace(r.Header.Get("X-Idempotency-Key"))
	}
	if len(idempotencyKey) > 128 {
		h.jsonError(w, "Idempotency-Key is too long (max 128 characters)", http.StatusBadRequest)
		return
	}
	if idempotencyKey != "" {
		existing, err := h.repo.GetExternalSessionByIdempotencyKey(r.Context(), apiCtx.OrgID, idempotencyKey)
		if err != nil {
			h.logger.Error("Failed to look up idempotent external session",
				zap.Error(err),
				zap.String("org_id", apiCtx.OrgID),
				zap.String("idempotency_key", idempotencyKey),
			)
			h.jsonError(w, "Failed to create session", http.StatusInternalServerError)
			return
		}
		if existing != nil {
			enrollmentStatus := h.resolveEnrollmentStatus(r.Context(), apiCtx.OrgID, existing.StudentID, "")
			h.logger.Info("External session idempotency replay",
				zap.String("session_id", existing.SessionID),
				zap.String("org_id", apiCtx.OrgID),
				zap.String("idempotency_key", idempotencyKey),
			)
			h.jsonResponse(w, createSessionResponse{
				SessionID:          existing.SessionID,
				ArgusSessionToken:  existing.SessionToken,
				SDKUrl:             h.sdkURL,
				ExpiresAt:          existing.TokenExpiresAt.UTC().Format(time.RFC3339),
				EnrollmentRequired: enrollmentStatus != "ready",
				EnrollmentStatus:   enrollmentStatus,
			}, http.StatusOK)
			return
		}
	}

	// Generate unique session ID.
	sessionID := generateSessionID()

	// Token duration: default 24 hours, or custom if provided (max 48h).
	durationMin := 1440
	if req.DurationMin > 0 && req.DurationMin <= 2880 {
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
		IdempotencyKey: idempotencyKey,
		SessionToken:   sessionToken,
		TokenExpiresAt: tokenExpiry,
		Status:         "created",
	}

	if err := h.repo.CreateExternalSession(r.Context(), session); err != nil {
		if idempotencyKey != "" {
			existing, lookupErr := h.repo.GetExternalSessionByIdempotencyKey(r.Context(), apiCtx.OrgID, idempotencyKey)
			if lookupErr == nil && existing != nil {
				enrollmentStatus := h.resolveEnrollmentStatus(r.Context(), apiCtx.OrgID, existing.StudentID, "")
				h.logger.Info("External session idempotency replay after create conflict",
					zap.String("session_id", existing.SessionID),
					zap.String("org_id", apiCtx.OrgID),
					zap.String("idempotency_key", idempotencyKey),
				)
				h.jsonResponse(w, createSessionResponse{
					SessionID:          existing.SessionID,
					ArgusSessionToken:  existing.SessionToken,
					SDKUrl:             h.sdkURL,
					ExpiresAt:          existing.TokenExpiresAt.UTC().Format(time.RFC3339),
					EnrollmentRequired: enrollmentStatus != "ready",
					EnrollmentStatus:   enrollmentStatus,
				}, http.StatusOK)
				return
			}
			if lookupErr != nil {
				h.logger.Warn("Failed to look up idempotent external session after create error",
					zap.Error(lookupErr),
					zap.String("org_id", apiCtx.OrgID),
					zap.String("idempotency_key", idempotencyKey),
				)
			}
		}
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
		zap.String("idempotency_key", idempotencyKey),
	)

	// If a reference photo was provided, kick off background enrollment so the
	// AI deep scan can perform face identity verification.
	if req.ReferencePhotoURL != "" && h.asynqClient != nil {
		enrollTask, err := worker.NewEnrollStudentTask(worker.EnrollStudentPayload{
			StudentID:  req.StudentID,
			OrgID:      apiCtx.OrgID,
			PhotoURL:   req.ReferencePhotoURL,
			EnrolledBy: "lms",
		})
		if err == nil {
			if _, err := h.asynqClient.Enqueue(enrollTask); err != nil {
				h.logger.Warn("failed to enqueue enrollment task",
					zap.String("student_id", req.StudentID),
					zap.Error(err),
				)
			}
		}
	}

	enrollmentStatus := h.resolveEnrollmentStatus(r.Context(), apiCtx.OrgID, req.StudentID, req.ReferencePhotoURL)
	h.jsonResponse(w, createSessionResponse{
		SessionID:          sessionID,
		ArgusSessionToken:  sessionToken,
		SDKUrl:             h.sdkURL,
		ExpiresAt:          tokenExpiry.UTC().Format(time.RFC3339),
		EnrollmentRequired: enrollmentStatus != "ready",
		EnrollmentStatus:   enrollmentStatus,
	}, http.StatusCreated)
}

func (h *ExternalHandler) resolveEnrollmentStatus(ctx context.Context, orgID, studentID, referencePhotoURL string) string {
	if h.repo == nil || strings.TrimSpace(studentID) == "" || strings.TrimSpace(orgID) == "" {
		if strings.TrimSpace(referencePhotoURL) != "" {
			return "queued"
		}
		return "unknown"
	}
	enrollment, err := h.repo.GetEnrollment(ctx, studentID, orgID)
	if err == nil && enrollment != nil && len(enrollment.Embedding) > 0 {
		return "ready"
	}
	if strings.TrimSpace(referencePhotoURL) != "" {
		if h.asynqClient != nil {
			return "queued"
		}
		return "photo_provided_queue_unavailable"
	}
	return "missing"
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

type externalReportResponse struct {
	SessionID         string                    `json:"sessionId"`
	ExamID            string                    `json:"examId"`
	StudentID         string                    `json:"studentId"`
	StudentName       string                    `json:"studentName,omitempty"`
	ExamName          string                    `json:"examName,omitempty"`
	OrgID             string                    `json:"orgId"`
	Status            string                    `json:"status"`
	Verdict           *string                   `json:"verdict,omitempty"`
	IntegrityScore    *float64                  `json:"integrityScore,omitempty"`
	RiskScore         *float64                  `json:"riskScore,omitempty"`
	ViolationCount    int                       `json:"violationCount"`
	ReviewStatus      string                    `json:"reviewStatus"`
	Timeline          []externalReportEvent     `json:"timeline"`
	Recordings        []externalReportRecording `json:"recordings"`
	AIDetections      *aiDetectionsSummary      `json:"aiDetections,omitempty"`
	EvidenceIntegrity externalEvidenceIntegrity `json:"evidenceIntegrity"`
	ReportURL         string                    `json:"reportUrl"`
	GeneratedAt       string                    `json:"generatedAt"`
}

// aiDetectionsSummary holds the backend AI deep scan summary for a session.
type aiDetectionsSummary struct {
	Scanned               bool                `json:"scanned"`
	IdentityVerified      *bool               `json:"identityVerified,omitempty"`
	AvgFaceSimilarity     *float64            `json:"avgFaceSimilarity,omitempty"`
	FaceMismatchCount     int                 `json:"faceMismatchCount"`
	LivenessFailCount     int                 `json:"livenessFailCount"`
	DeepfakeCount         int                 `json:"deepfakeCount"`
	ScreenReflectionCount int                 `json:"screenReflectionCount"`
	VoiceSynthCount       int                 `json:"voiceSynthCount"`
	AvgLivenessScore      *float64            `json:"avgLivenessScore,omitempty"`
	ObjectDetections      []aiObjectDetection `json:"objectDetections"`
	BackendEventCount     int                 `json:"backendEventCount"`
}

type aiObjectDetection struct {
	ObjectType string  `json:"objectType"`
	Count      int     `json:"count"`
	MaxConf    float64 `json:"maxConfidence"`
}

func upsertAIObjectDetection(items map[string]*aiObjectDetection, objectType string, count int, maxConf float64) {
	if existing, ok := items[objectType]; ok {
		existing.Count += count
		if maxConf > existing.MaxConf {
			existing.MaxConf = maxConf
		}
		return
	}
	items[objectType] = &aiObjectDetection{
		ObjectType: objectType,
		Count:      count,
		MaxConf:    maxConf,
	}
}

type externalReportEvent struct {
	EventID        string  `json:"eventId"`
	EventType      string  `json:"eventType"`
	Severity       string  `json:"severity"`
	Source         string  `json:"source"`
	Label          string  `json:"label"`
	Confidence     float64 `json:"confidence"`
	Timestamp      string  `json:"timestamp"`
	VideoTimestamp int64   `json:"videoTimestamp"`
}

type externalReportRecording struct {
	RecordingID  string  `json:"recordingId"`
	EgressID     string  `json:"egressId"`
	SessionID    string  `json:"sessionId"`
	Status       string  `json:"status"`
	RoomName     string  `json:"roomName"`
	FileURL      string  `json:"fileUrl,omitempty"`
	VideoURL     string  `json:"videoUrl,omitempty"`
	ContentType  string  `json:"contentType,omitempty"`
	DurationSec  int64   `json:"durationSec"`
	ErrorMessage string  `json:"errorMessage,omitempty"`
	StartedAt    string  `json:"startedAt"`
	EndedAt      *string `json:"endedAt,omitempty"`
}

type externalEvidenceIntegrity struct {
	Status     string `json:"status"`
	ChainValid *bool  `json:"chainValid,omitempty"`
	Message    string `json:"message,omitempty"`
}

func (h *ExternalHandler) handleSessionReport(w http.ResponseWriter, r *http.Request) {
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

	timeline, err := h.queryExternalReportTimeline(r.Context(), sessionID)
	if err != nil {
		h.logger.Warn("external report timeline unavailable",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		timeline = []externalReportEvent{}
	}

	recordings, err := h.queryExternalReportRecordings(r.Context(), sessionID)
	if err != nil {
		h.logger.Warn("external report recordings unavailable",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		recordings = []externalReportRecording{}
	}

	aiDetections, err := h.queryAIDetections(r.Context(), sessionID)
	if err != nil {
		h.logger.Warn("ai detections unavailable",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
	}

	integrityScore := session.IntegrityScore
	var riskScore *float64
	if integrityScore != nil {
		score := 100 - *integrityScore
		if score < 0 {
			score = 0
		}
		if score > 100 {
			score = 100
		}
		riskScore = &score
	}

	reviewStatus := "pending"
	if session.Verdict != nil && *session.Verdict != "" {
		reviewStatus = "ready"
	}
	if session.Status == "cancelled" || session.Status == "expired" {
		reviewStatus = session.Status
	}

	resp := externalReportResponse{
		SessionID:      session.SessionID,
		ExamID:         session.ExamID,
		StudentID:      session.StudentID,
		StudentName:    session.StudentName,
		ExamName:       session.ExamName,
		OrgID:          session.OrgID,
		Status:         session.Status,
		Verdict:        session.Verdict,
		IntegrityScore: integrityScore,
		RiskScore:      riskScore,
		ViolationCount: session.ViolationCount,
		ReviewStatus:   reviewStatus,
		Timeline:       timeline,
		Recordings:     recordings,
		AIDetections:   aiDetections,
		EvidenceIntegrity: externalEvidenceIntegrity{
			Status:  "not_checked",
			Message: "Forensic integrity verification runs asynchronously after session completion",
		},
		ReportURL:   fmt.Sprintf("/api/v1/external/sessions/%s/report", session.SessionID),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	h.jsonResponse(w, resp, http.StatusOK)
}

// handleSessionReportPDF generates a bilingual (RU/KZ) PDF report for a session.
//
//	GET /api/v1/external/sessions/{sessionId}/report.pdf
func (h *ExternalHandler) handleSessionReportPDF(w http.ResponseWriter, r *http.Request) {
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

	// Collect timeline violations from ClickHouse.
	timeline, _ := h.queryExternalReportTimeline(r.Context(), sessionID)
	violations := make([]forensic.ExternalViolation, 0, len(timeline))
	for _, e := range timeline {
		if e.Severity == "critical" || e.Severity == "warning" {
			violations = append(violations, forensic.ExternalViolation{
				EventType:  e.EventType,
				Severity:   e.Severity,
				Label:      e.Label,
				Confidence: e.Confidence,
				Timestamp:  e.Timestamp,
			})
		}
	}

	// Collect AI detections summary.
	aiDetections, _ := h.queryAIDetections(r.Context(), sessionID)

	var (
		aiScanned         bool
		identityVerified  *bool
		faceMismatchCount int
		livenessFailCount int
		objDetections     []forensic.ExternalObjectDetection
	)
	if aiDetections != nil {
		aiScanned = aiDetections.Scanned
		identityVerified = aiDetections.IdentityVerified
		faceMismatchCount = aiDetections.FaceMismatchCount
		livenessFailCount = aiDetections.LivenessFailCount
		for _, obj := range aiDetections.ObjectDetections {
			objDetections = append(objDetections, forensic.ExternalObjectDetection{
				ObjectType: obj.ObjectType,
				Count:      obj.Count,
				MaxConf:    obj.MaxConf,
			})
		}
	}

	// Collect recordings.
	recordings, _ := h.queryExternalReportRecordings(r.Context(), sessionID)
	recs := make([]forensic.ExternalRecordingRef, 0, len(recordings))
	for _, rec := range recordings {
		recs = append(recs, forensic.ExternalRecordingRef{
			EgressID:  rec.EgressID,
			Status:    rec.Status,
			StartedAt: rec.StartedAt,
		})
	}

	verdict := ""
	if session.Verdict != nil {
		verdict = *session.Verdict
	}
	integrityScore := 0.0
	if session.IntegrityScore != nil {
		integrityScore = *session.IntegrityScore
	}

	data := &forensic.ExternalReportData{
		SessionID:         sessionID,
		ExamID:            session.ExamID,
		ExamName:          session.ExamName,
		StudentID:         session.StudentID,
		StudentName:       session.StudentName,
		OrgID:             session.OrgID,
		Status:            session.Status,
		Verdict:           verdict,
		IntegrityScore:    integrityScore,
		ViolationCount:    session.ViolationCount,
		ReviewStatus:      "pending",
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
		Violations:        violations,
		AIScanned:         aiScanned,
		IdentityVerified:  identityVerified,
		FaceMismatchCount: faceMismatchCount,
		LivenessFailCount: livenessFailCount,
		ObjectDetections:  objDetections,
		Recordings:        recs,
	}

	pdfBytes, hash, err := forensic.GenerateExternalPDF(data)
	if err != nil {
		h.logger.Error("failed to generate external PDF report",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		h.jsonError(w, "Failed to generate PDF report", http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("argus-report-%s.pdf", sessionID)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("X-Report-Hash", hash)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pdfBytes)))
	w.WriteHeader(http.StatusOK)
	w.Write(pdfBytes) //nolint:errcheck
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
	webhookPayload := map[string]interface{}{
		"event":          "verdict.ready",
		"sessionId":      sessionID,
		"verdict":        req.Verdict,
		"integrityScore": req.IntegrityScore,
		"violationCount": req.ViolationCount,
		"reportUrl":      fmt.Sprintf("/api/v1/external/sessions/%s/report", sessionID),
	}
	h.enqueueWebhook(r.Context(), apiCtx.OrgID, "verdict.ready", webhookPayload)
	h.deliverSessionCallback(r.Context(), session, "verdict.ready", webhookPayload)

	// Auto-trigger AI deep scan so backend inference runs asynchronously after
	// every session completion. Load per-exam settings for threshold overrides.
	if h.asynqClient != nil {
		aiPayload := worker.AIAnalysisPayload{
			SessionID:    sessionID,
			OrgID:        session.OrgID,
			ExamID:       session.ExamID,
			StudentID:    session.StudentID,
			AnalysisType: "full_scan",
		}
		if settings, err := h.repo.GetExamProctoringSettings(r.Context(), session.OrgID, session.ExamID); err == nil && settings != nil {
			aiPayload.CleanThreshold = settings.CleanThreshold
			aiPayload.WarningThreshold = settings.WarningThreshold
		}
		if aiTask, err := worker.NewAIAnalysisTask(aiPayload); err == nil {
			if _, err := h.asynqClient.Enqueue(aiTask); err != nil && !isErrAlreadyQueued(err) {
				h.logger.Warn("failed to auto-enqueue AI analysis",
					zap.String("session_id", sessionID),
					zap.Error(err),
				)
			}
		}
	}

	h.jsonResponse(w, map[string]string{"status": "completed", "verdict": req.Verdict}, http.StatusOK)
}

func isErrAlreadyQueued(err error) bool {
	return err != nil && len(err.Error()) > 0 &&
		(contains(err.Error(), "already exists") || contains(err.Error(), "duplicate"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
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

// deliverSessionCallback sends the verdict to a per-session callbackUrl when a
// partner supplied one on session creation. Registered webhook endpoints still
// use the durable retry dispatcher; callbackUrl is a direct partner callback so
// Ustaz can receive a result even before webhook endpoint setup is completed.
func (h *ExternalHandler) deliverSessionCallback(ctx context.Context, session *entity.ExternalSession, eventType string, payload interface{}) {
	if session == nil || strings.TrimSpace(session.CallbackURL) == "" {
		return
	}

	callbackURL := strings.TrimSpace(session.CallbackURL)
	if !strings.HasPrefix(callbackURL, "https://") && !strings.HasPrefix(callbackURL, "http://") {
		h.logger.Warn("skipping invalid session callback URL",
			zap.String("session_id", session.SessionID),
			zap.String("callback_url", callbackURL),
		)
		return
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		h.logger.Warn("failed to marshal session callback payload",
			zap.String("session_id", session.SessionID),
			zap.Error(err),
		)
		return
	}

	go func() {
		reqCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		timestamp := time.Now().Unix()
		signingSecret := hex.EncodeToString(h.jwtSigningKey)
		if signingSecret == "" {
			signingSecret = "argus-session-callback"
		}
		signature := SignWebhookPayload(signingSecret, timestamp, payloadJSON)

		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, callbackURL, bytes.NewReader(payloadJSON))
		if err != nil {
			h.logger.Warn("failed to build session callback request",
				zap.String("session_id", session.SessionID),
				zap.Error(err),
			)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "ArgusAI-SessionCallback/1.0")
		req.Header.Set("X-Argus-Event", eventType)
		req.Header.Set("X-Argus-Timestamp", fmt.Sprintf("%d", timestamp))
		req.Header.Set("X-Argus-Signature", fmt.Sprintf("sha256=%s", signature))
		req.Header.Set("X-Argus-Session-Id", session.SessionID)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			h.logger.Warn("session callback delivery failed",
				zap.String("session_id", session.SessionID),
				zap.String("callback_url", callbackURL),
				zap.Error(err),
			)
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			h.logger.Warn("session callback returned non-2xx",
				zap.String("session_id", session.SessionID),
				zap.String("callback_url", callbackURL),
				zap.Int("status", resp.StatusCode),
				zap.String("body", string(body)),
			)
			return
		}
		h.logger.Info("session callback delivered",
			zap.String("session_id", session.SessionID),
			zap.String("callback_url", callbackURL),
			zap.Int("status", resp.StatusCode),
		)
	}()
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

// ==========================================================================
// Session Token Auth Middleware
// ==========================================================================

// requireSessionToken validates the Authorization: Bearer <argusSessionToken>
// header and injects the proctoring claims into the request context.
func (h *ExternalHandler) requireSessionToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			h.jsonError(w, "missing or invalid Authorization header", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		verifier, err := auth.NewVerifier(auth.VerifierConfig{
			Algorithm:  "HS256",
			SigningKey: h.jwtSigningKey,
			Issuer:     "argus-external-api",
			Audience:   "argus-event-collector",
			ClockSkew:  30 * time.Second,
		})
		if err != nil {
			h.jsonError(w, "internal auth configuration error", http.StatusInternalServerError)
			return
		}

		claims, err := verifier.VerifyToken(tokenStr)
		if err != nil {
			h.jsonError(w, "invalid or expired session token", http.StatusUnauthorized)
			return
		}

		ctx := auth.ContextWithClaims(r.Context(), claims)
		next(w, r.WithContext(ctx))
	}
}

// ==========================================================================
// Event Ingestion Handler
// ==========================================================================

// ingestEventRequest is the JSON body for a single event.
type ingestEventRequest struct {
	EventType  int32   `json:"event_type"`
	Severity   int32   `json:"severity"`
	Source     int32   `json:"source"`
	Confidence float32 `json:"confidence"`
	Label      string  `json:"label"`
}

// handleIngestEvents accepts a JSON batch of proctoring events authenticated
// with the argusSessionToken returned by createSession.
func (h *ExternalHandler) handleIngestEvents(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		h.jsonError(w, "missing session claims", http.StatusUnauthorized)
		return
	}

	var body struct {
		Events  []ingestEventRequest `json:"events"`
		BatchID string               `json:"batch_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if len(body.Events) == 0 {
		h.jsonError(w, "at least one event is required", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	domainEvents := make([]*entity.ProctoringEvent, 0, len(body.Events))
	for _, ev := range body.Events {
		domainEvents = append(domainEvents, &entity.ProctoringEvent{
			EventID:         uuid.New().String(),
			SessionID:       claims.SessionID,
			StudentID:       claims.StudentID,
			ExamID:          claims.ExamID,
			OrgID:           claims.OrgID,
			EventType:       valueobject.EventType(ev.EventType),
			Severity:        valueobject.Severity(ev.Severity),
			Source:          valueobject.EventSource(ev.Source),
			ClientTimestamp: now,
			Confidence:      ev.Confidence,
			Label:           ev.Label,
		})
	}

	result, err := h.ingest.IngestBatch(r.Context(), domainEvents)
	if err != nil {
		h.logger.Error("rest event ingestion failed", zap.Error(err))
		h.jsonError(w, "event ingestion failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"accepted_count": result.AcceptedCount,
		"rejected_count": result.RejectedCount,
		"batch_id":       body.BatchID,
	})
}

// ==========================================================================
// Student LiveKit Media Token
// ==========================================================================

// handleStudentMediaToken generates a LiveKit student token so the SDK can
// publish the student's camera to the proctoring room. Authenticated with the
// argusSessionToken (student JWT), NOT admin credentials.
//
//	POST /api/v1/external/sessions/{sessionId}/student-token
func (h *ExternalHandler) handleStudentMediaToken(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		h.jsonError(w, "missing session claims", http.StatusUnauthorized)
		return
	}

	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		sessionID = claims.SessionID
	}

	// Verify the session exists in DB.
	session, err := h.repo.GetExternalSessionByID(r.Context(), sessionID)
	if err != nil || session == nil {
		h.jsonError(w, "session not found", http.StatusNotFound)
		return
	}

	// Room name matches the convention used by the proctor token generator.
	roomName := "argus-session-" + sessionID
	identity := fmt.Sprintf("student-%s", claims.StudentID)

	token, err := h.generateStudentLiveKitToken(identity, session.StudentName, roomName)
	if err != nil {
		h.logger.Error("failed to generate student livekit token", zap.Error(err))
		h.jsonError(w, "failed to generate livekit token", http.StatusInternalServerError)
		return
	}

	// Auto-start recording after student joins (non-blocking, retries until room exists).
	if h.egress != nil {
		capturedRoom := roomName
		capturedIdentity := identity
		capturedStudentID := claims.StudentID
		capturedSessionID := sessionID
		go func() {
			// Student needs a few seconds to connect to LiveKit after receiving the token.
			// Retry every 5s for up to 60s until the room exists.
			for attempt := 1; attempt <= 12; attempt++ {
				time.Sleep(5 * time.Second)
				if egressID, _, ok := h.findExistingRecording(context.Background(), capturedSessionID); ok {
					h.logger.Info("auto-recording skipped: recording already exists",
						zap.String("egress_id", egressID),
						zap.String("room", capturedRoom),
					)
					return
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				egressID, err := h.egress.StartParticipantRecording(ctx, capturedRoom, capturedIdentity, capturedSessionID, capturedStudentID)
				cancel()
				if err == nil {
					h.persistStartedRecording(context.Background(), egressID, capturedSessionID, capturedStudentID, capturedRoom)
					h.logger.Info("auto-recording started",
						zap.String("egress_id", egressID),
						zap.String("room", capturedRoom),
						zap.Int("attempt", attempt),
					)
					return
				}
				h.logger.Debug("recording not started yet, retrying",
					zap.String("room", capturedRoom),
					zap.Int("attempt", attempt),
					zap.Error(err),
				)
			}
			h.logger.Warn("auto-recording gave up: student did not join room in time",
				zap.String("room", capturedRoom),
			)
		}()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"livekitToken": token,
		"livekitUrl":   h.livekitPublicURL,
		"room":         roomName,
	})
}

// handleRecordingReady is called by the SDK after the student has successfully
// joined the LiveKit room. This is the reliable trigger point for starting
// ParticipantEgress (the room is guaranteed to exist at this point).
//
//	POST /api/v1/external/sessions/{sessionId}/recording-ready
func (h *ExternalHandler) handleRecordingReady(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		h.jsonError(w, "missing session claims", http.StatusUnauthorized)
		return
	}

	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		sessionID = claims.SessionID
	}

	if h.egress == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "no_egress"})
		return
	}

	roomName := "argus-session-" + sessionID
	identity := fmt.Sprintf("student-%s", claims.StudentID)

	if existingEgressID, existingStatus, ok := h.findExistingRecording(r.Context(), sessionID); ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":   "recording_already_started",
			"egressId": existingEgressID,
			"state":    existingStatus,
			"room":     roomName,
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	egressID, err := h.egress.StartParticipantRecording(ctx, roomName, identity, sessionID, claims.StudentID)
	if err != nil {
		h.logger.Warn("recording-ready: failed to start egress",
			zap.String("room", roomName),
			zap.Error(err),
		)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "recording_requested",
			"detail": "egress_start_failed",
		})
		return
	}

	h.logger.Info("recording-ready: egress started",
		zap.String("egress_id", egressID),
		zap.String("room", roomName),
	)
	h.persistStartedRecording(ctx, egressID, sessionID, claims.StudentID, roomName)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":   "recording_started",
		"egressId": egressID,
		"room":     roomName,
	})
}

func (h *ExternalHandler) findExistingRecording(ctx context.Context, sessionID string) (string, string, bool) {
	queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var egressID, status string
	err := h.repo.DB().QueryRowContext(queryCtx, `
		SELECT egress_id, status
		FROM livekit_recordings
		WHERE session_id = $1
			AND status IN ('EGRESS_STARTING', 'EGRESS_ACTIVE', 'EGRESS_ENDING', 'EGRESS_COMPLETE', 'EGRESS_COMPLETED')
		ORDER BY created_at DESC
		LIMIT 1`, sessionID).Scan(&egressID, &status)
	if err != nil {
		return "", "", false
	}
	return egressID, status, true
}

func (h *ExternalHandler) persistStartedRecording(ctx context.Context, egressID, sessionID, studentID, roomName string) {
	if strings.TrimSpace(egressID) == "" {
		return
	}
	persistCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rec := &entity.Recording{
		EgressID:  egressID,
		SessionID: sessionID,
		UserID:    studentID,
		RoomName:  roomName,
		Status:    "EGRESS_ACTIVE",
		FileURL:   fmt.Sprintf("content/recordings/%s/%s-%s.mp4", roomName, sessionID, studentID),
	}
	if err := h.repo.CreateRecording(persistCtx, rec); err != nil {
		h.logger.Warn("failed to persist egress metadata",
			zap.String("egress_id", egressID),
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
	}
}

const maxAIFrameUploadBytes = 768 * 1024

// handleAIFrame accepts a sampled webcam frame from the browser SDK and queues
// it for backend inference. It intentionally does not block the exam flow:
// if the inference queue is disabled, the SDK receives a clear non-fatal status.
//
//	POST /api/v1/external/sessions/{sessionId}/ai-frame
func (h *ExternalHandler) handleAIFrame(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		h.jsonError(w, "missing session claims", http.StatusUnauthorized)
		return
	}

	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		sessionID = claims.SessionID
	}
	if sessionID != claims.SessionID {
		h.jsonError(w, "session mismatch", http.StatusForbidden)
		return
	}

	if h.asynqClient == nil {
		h.jsonResponse(w, map[string]string{"status": "inference_disabled"}, http.StatusAccepted)
		return
	}

	session, err := h.repo.GetExternalSessionByID(r.Context(), sessionID)
	if err != nil || session == nil {
		h.jsonError(w, "session not found", http.StatusNotFound)
		return
	}
	if session.IsTerminal() {
		h.jsonResponse(w, map[string]string{"status": "session_terminal"}, http.StatusAccepted)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAIFrameUploadBytes+64*1024)
	if err := r.ParseMultipartForm(maxAIFrameUploadBytes + 64*1024); err != nil {
		h.jsonError(w, "invalid multipart frame upload", http.StatusBadRequest)
		return
	}

	contentType := strings.TrimSpace(r.FormValue("contentType"))
	if contentType == "" {
		contentType = "image/jpeg"
	}
	if !isAllowedAIFrameContentType(contentType) {
		h.jsonError(w, "unsupported frame content type", http.StatusUnsupportedMediaType)
		return
	}

	videoTimestampSec := 0.0
	if raw := strings.TrimSpace(r.FormValue("videoTimestampSec")); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil || parsed < 0 {
			h.jsonError(w, "invalid videoTimestampSec", http.StatusBadRequest)
			return
		}
		videoTimestampSec = parsed
	}

	file, _, err := r.FormFile("frame")
	if err != nil {
		h.jsonError(w, "missing frame file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	frameData, err := io.ReadAll(io.LimitReader(file, maxAIFrameUploadBytes+1))
	if err != nil {
		h.jsonError(w, "failed to read frame", http.StatusBadRequest)
		return
	}
	if len(frameData) == 0 {
		h.jsonError(w, "empty frame", http.StatusBadRequest)
		return
	}
	if len(frameData) > maxAIFrameUploadBytes {
		h.jsonError(w, "frame is too large", http.StatusRequestEntityTooLarge)
		return
	}

	payload := worker.AIFrameAnalysisPayload{
		SessionID:         sessionID,
		OrgID:             claims.OrgID,
		ExamID:            claims.ExamID,
		StudentID:         claims.StudentID,
		ContentType:       contentType,
		FrameData:         frameData,
		VideoTimestampSec: videoTimestampSec,
	}

	task, err := worker.NewAIFrameAnalysisTask(payload)
	if err != nil {
		h.logger.Warn("failed to build AI frame task",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		h.jsonError(w, "failed to queue frame", http.StatusInternalServerError)
		return
	}
	if _, err := h.asynqClient.Enqueue(task); err != nil {
		h.logger.Warn("failed to enqueue AI frame task",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		h.jsonError(w, "failed to queue frame", http.StatusBadGateway)
		return
	}

	h.jsonResponse(w, map[string]interface{}{
		"status":            "queued",
		"videoTimestampSec": videoTimestampSec,
		"sizeBytes":         len(frameData),
	}, http.StatusAccepted)
}

func isAllowedAIFrameContentType(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch mediaType {
	case "image/jpeg", "image/jpg", "image/png", "image/webp":
		return true
	default:
		return false
	}
}

// generateStudentLiveKitToken creates a LiveKit JWT for a publishing student.
func (h *ExternalHandler) generateStudentLiveKitToken(identity, name, room string) (string, error) {
	now := time.Now()
	boolTrue := true
	boolFalse := false

	claims := livekitTokenClaims{
		Exp:  now.Add(24 * time.Hour).Unix(),
		Iss:  h.livekitAPIKey,
		Nbf:  now.Unix(),
		Sub:  identity,
		Name: name,
		Video: livekitVideoGrant{
			RoomJoin:       true,
			Room:           room,
			CanPublish:     &boolTrue,
			CanSubscribe:   &boolFalse,
			CanPublishData: &boolTrue,
		},
	}

	header := `{"alg":"HS256","typ":"JWT"}`
	headerB64 := lkBase64Encode([]byte(header))

	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	payloadB64 := lkBase64Encode(payloadJSON)

	signingInput := headerB64 + "." + payloadB64
	mac := hmac.New(sha256.New, []byte(h.livekitAPISecret))
	mac.Write([]byte(signingInput))
	signatureB64 := lkBase64Encode(mac.Sum(nil))

	return headerB64 + "." + payloadB64 + "." + signatureB64, nil
}

// ==========================================================================
// Sessions By Result
// ==========================================================================

// handleListSessionsByResult returns proctoring sessions for a given exam+student pair.
// Used by LMS partners to fetch the verdict after a test is submitted.
//
//	GET /api/v1/external/sessions/by-result?testId=X&profileId=Y
func (h *ExternalHandler) handleListSessionsByResult(w http.ResponseWriter, r *http.Request) {
	examID := r.URL.Query().Get("testId")
	studentID := r.URL.Query().Get("profileId")
	if examID == "" || studentID == "" {
		h.jsonError(w, "testId and profileId query params are required", http.StatusBadRequest)
		return
	}

	sessions, err := h.repo.ListExternalSessionsByResult(r.Context(), examID, studentID)
	if err != nil {
		h.logger.Error("list sessions by result failed", zap.Error(err))
		h.jsonError(w, "failed to fetch sessions", http.StatusInternalServerError)
		return
	}
	if sessions == nil {
		sessions = []*entity.ExternalSession{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(sessions)
}

func (h *ExternalHandler) queryExternalReportTimeline(ctx context.Context, sessionID string) ([]externalReportEvent, error) {
	if h.chConn == nil {
		return []externalReportEvent{}, nil
	}

	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rows, err := h.chConn.Query(qctx, `
		SELECT
			event_id,
			event_type,
			severity,
			source,
			label,
			confidence,
			server_timestamp,
			video_timestamp_sec
		FROM proctoring_events
		WHERE session_id = ?
		ORDER BY server_timestamp ASC
		LIMIT 1000`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]externalReportEvent, 0)
	for rows.Next() {
		var event externalReportEvent
		var ts time.Time
		var confidence float32
		var videoTs float64
		if err := rows.Scan(
			&event.EventID,
			&event.EventType,
			&event.Severity,
			&event.Source,
			&event.Label,
			&confidence,
			&ts,
			&videoTs,
		); err != nil {
			return nil, err
		}
		event.Confidence = float64(confidence)
		event.Timestamp = ts.UTC().Format(time.RFC3339)
		event.VideoTimestamp = int64(videoTs)
		events = append(events, event)
	}
	return events, rows.Err()
}

// queryAIDetections fetches the backend AI deep-scan summary for a session.
// Returns nil (not an error) if ClickHouse is unavailable or the session was
// never scanned — the caller omits the field from the JSON response.
func (h *ExternalHandler) queryAIDetections(ctx context.Context, sessionID string) (*aiDetectionsSummary, error) {
	if h.chConn == nil {
		return nil, nil
	}

	qctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	// Count current BACKEND_AI event types and aggregate only populated AI scores.
	type aiRow struct {
		eventType     string
		severity      string
		payloadType   string
		objectType    string
		count         uint64
		avgSimilarity float64
		simCount      uint64
		avgLiveness   float64
		liveCount     uint64
		maxConf       float64
	}

	rows, err := h.chConn.Query(qctx, `
		SELECT
			event_type,
			severity,
			payload_type,
			if(payload_type = 'object_detection', JSONExtractString(payload, 'object_type'), '') AS object_type,
			count()                                           AS cnt,
			avgIf(face_similarity, face_similarity >= 0)      AS avg_sim,
			countIf(face_similarity >= 0)                     AS sim_count,
			avgIf(liveness_score, liveness_score >= 0)        AS avg_live,
			countIf(liveness_score >= 0)                      AS live_count,
			max(confidence)                                   AS max_conf
		FROM proctoring_events
		WHERE session_id = ? AND source = 'BACKEND_AI'
		GROUP BY event_type, severity, payload_type, object_type
		ORDER BY cnt DESC
		LIMIT 50`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summary := &aiDetectionsSummary{
		ObjectDetections: []aiObjectDetection{},
	}

	var totalBackend int
	var simSum, simCount, liveSum, liveCount float64
	objectMap := map[string]*aiObjectDetection{}

	for rows.Next() {
		var r aiRow
		if err := rows.Scan(
			&r.eventType,
			&r.severity,
			&r.payloadType,
			&r.objectType,
			&r.count,
			&r.avgSimilarity,
			&r.simCount,
			&r.avgLiveness,
			&r.liveCount,
			&r.maxConf,
		); err != nil {
			continue
		}
		totalBackend += int(r.count)

		switch r.eventType {
		case "BACKEND_AI_FACE_MISMATCH":
			if r.payloadType == "liveness" {
				summary.LivenessFailCount += int(r.count)
			} else {
				summary.FaceMismatchCount += int(r.count)
			}
		case "BACKEND_AI_DEEPFAKE_DETECTED":
			summary.DeepfakeCount += int(r.count)
			summary.LivenessFailCount += int(r.count)
		case "BACKEND_AI_HIDDEN_OBJECT", "BACKEND_AI_SCREEN_REFLECTION":
			if r.eventType == "BACKEND_AI_SCREEN_REFLECTION" {
				summary.ScreenReflectionCount += int(r.count)
			}
			objectType := r.objectType
			if objectType == "" {
				objectType = r.eventType
			}
			upsertAIObjectDetection(objectMap, objectType, int(r.count), r.maxConf)
		case "BACKEND_AI_VOICE_SYNTH":
			summary.VoiceSynthCount += int(r.count)
		}

		if r.simCount > 0 {
			simSum += r.avgSimilarity * float64(r.simCount)
			simCount += float64(r.simCount)
		}
		if r.liveCount > 0 {
			liveSum += r.avgLiveness * float64(r.liveCount)
			liveCount += float64(r.liveCount)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	summary.BackendEventCount = totalBackend
	summary.Scanned = totalBackend > 0

	if simCount > 0 {
		avg := simSum / simCount
		summary.AvgFaceSimilarity = &avg
		verified := avg >= 0.4 && summary.FaceMismatchCount == 0
		summary.IdentityVerified = &verified
	}
	if liveCount > 0 {
		avg := liveSum / liveCount
		summary.AvgLivenessScore = &avg
	}
	for _, obj := range objectMap {
		summary.ObjectDetections = append(summary.ObjectDetections, *obj)
	}

	if !summary.Scanned {
		return nil, nil
	}
	return summary, nil
}

func (h *ExternalHandler) queryExternalReportRecordings(ctx context.Context, sessionID string) ([]externalReportRecording, error) {
	rows, err := h.repo.DB().QueryContext(ctx, `
		SELECT egress_id, session_id, status, room_name, file_url, error_message, started_at, ended_at
		FROM livekit_recordings
		WHERE session_id = $1
		ORDER BY created_at DESC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	recordings := make([]externalReportRecording, 0)
	for rows.Next() {
		var rec externalReportRecording
		var startedAt time.Time
		var endedAt *time.Time
		if err := rows.Scan(
			&rec.EgressID,
			&rec.SessionID,
			&rec.Status,
			&rec.RoomName,
			&rec.FileURL,
			&rec.ErrorMessage,
			&startedAt,
			&endedAt,
		); err != nil {
			return nil, err
		}
		rec.RecordingID = rec.EgressID
		rec.ContentType = "video/mp4"
		rec.StartedAt = startedAt.UTC().Format(time.RFC3339)
		if endedAt != nil {
			formatted := endedAt.UTC().Format(time.RFC3339)
			rec.EndedAt = &formatted
			rec.DurationSec = int64(endedAt.Sub(startedAt).Seconds())
		} else if rec.Status == "EGRESS_ACTIVE" || rec.Status == "EGRESS_STARTING" {
			rec.DurationSec = int64(time.Since(startedAt).Seconds())
		}
		if rec.DurationSec < 0 {
			rec.DurationSec = 0
		}
		if strings.TrimSpace(rec.FileURL) != "" {
			rec.VideoURL = fmt.Sprintf("/api/v1/archive/sessions/%s/video", sessionID)
		}
		recordings = append(recordings, rec)
	}
	return recordings, rows.Err()
}

// ==========================================================================
// Session Heartbeat
// ==========================================================================

// handleHeartbeat keeps an external proctoring session alive and transitions
// it from "created" → "active" on the first call (student has opened the exam page).
func (h *ExternalHandler) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		h.jsonError(w, "missing session claims", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()

	// Transition created → active on first heartbeat.
	_ = h.repo.ActivateExternalSession(ctx, claims.SessionID)

	// Update updated_at so monitoring can detect live sessions.
	_ = h.repo.TouchExternalSession(ctx, claims.SessionID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":    "ok",
		"sessionId": claims.SessionID,
	})
}
