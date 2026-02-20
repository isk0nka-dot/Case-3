// =============================================================================
// Argus AI — Consent REST API Handler
// =============================================================================
//
// Provides HTTP endpoints for recording and querying student consent to
// proctoring terms. Required for GDPR/privacy compliance.
//
// Endpoints:
//   POST /api/v1/consent              — Record student consent
//   GET  /api/v1/consent/{sessionId}  — Get consent status for a session
// =============================================================================
package http

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/domain/entity"
)

// ConsentHandler serves the consent REST API.
type ConsentHandler struct {
	db     *sql.DB
	logger *zap.Logger

	jwtSigningKey []byte
	repo          adminRepo
}

// NewConsentHandler creates a new consent API handler.
func NewConsentHandler(
	db *sql.DB,
	repo adminRepo,
	logger *zap.Logger,
	jwtSigningKey []byte,
) *ConsentHandler {
	return &ConsentHandler{
		db:            db,
		logger:        logger.Named("consent_api"),
		jwtSigningKey: jwtSigningKey,
		repo:          repo,
	}
}

// RegisterRoutes registers consent API endpoints on the given mux.
func (h *ConsentHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/consent", h.requireAuth(h.handleRecordConsent))
	mux.HandleFunc("GET /api/v1/consent/{sessionId}", h.requireAuth(h.handleGetConsent))
}

// ==========================================================================
// Request/Response Types
// ==========================================================================

type recordConsentRequest struct {
	SessionID      string `json:"sessionId"`
	StudentID      string `json:"studentId"`
	OrgID          string `json:"orgId"`
	ExamID         string `json:"examId"`
	ConsentVersion string `json:"consentVersion"`
	ConsentText    string `json:"consentText"`
	Accepted       bool   `json:"accepted"`
}

type consentResponse struct {
	ID             string    `json:"id"`
	SessionID      string    `json:"sessionId"`
	StudentID      string    `json:"studentId"`
	OrgID          string    `json:"orgId"`
	ExamID         string    `json:"examId"`
	ConsentVersion string    `json:"consentVersion"`
	Accepted       bool      `json:"accepted"`
	CreatedAt      time.Time `json:"createdAt"`
}

// ==========================================================================
// Handlers
// ==========================================================================

func (h *ConsentHandler) handleRecordConsent(w http.ResponseWriter, r *http.Request) {
	var req recordConsentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.consentJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" || req.StudentID == "" || req.OrgID == "" || req.ExamID == "" {
		h.consentJSONError(w, "sessionId, studentId, orgId, and examId are required", http.StatusBadRequest)
		return
	}

	id := generateConsentID()
	ipAddress := r.Header.Get("X-Forwarded-For")
	if ipAddress == "" {
		ipAddress = r.RemoteAddr
	}
	userAgent := r.Header.Get("User-Agent")

	_, err := h.db.ExecContext(r.Context(), `
		INSERT INTO consent_records (id, session_id, student_id, org_id, exam_id,
			consent_version, consent_text, accepted, ip_address, user_agent, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())`,
		id, req.SessionID, req.StudentID, req.OrgID, req.ExamID,
		req.ConsentVersion, req.ConsentText, req.Accepted,
		ipAddress, userAgent,
	)
	if err != nil {
		h.logger.Error("consent: failed to record", zap.Error(err))
		h.consentJSONError(w, "Failed to record consent", http.StatusInternalServerError)
		return
	}

	h.logger.Info("consent recorded",
		zap.String("id", id),
		zap.String("session_id", req.SessionID),
		zap.String("student_id", req.StudentID),
		zap.Bool("accepted", req.Accepted),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(consentResponse{
		ID:             id,
		SessionID:      req.SessionID,
		StudentID:      req.StudentID,
		OrgID:          req.OrgID,
		ExamID:         req.ExamID,
		ConsentVersion: req.ConsentVersion,
		Accepted:       req.Accepted,
		CreatedAt:      time.Now(),
	})
}

func (h *ConsentHandler) handleGetConsent(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.consentJSONError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	var resp consentResponse
	err := h.db.QueryRowContext(r.Context(), `
		SELECT id, session_id, student_id, org_id, exam_id, consent_version, accepted, created_at
		FROM consent_records
		WHERE session_id = $1
		ORDER BY created_at DESC
		LIMIT 1`, sessionID,
	).Scan(
		&resp.ID, &resp.SessionID, &resp.StudentID, &resp.OrgID, &resp.ExamID,
		&resp.ConsentVersion, &resp.Accepted, &resp.CreatedAt,
	)
	if err == sql.ErrNoRows {
		h.consentJSONError(w, "No consent record found for this session", http.StatusNotFound)
		return
	}
	if err != nil {
		h.logger.Error("consent: failed to query", zap.Error(err))
		h.consentJSONError(w, "Failed to query consent", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ==========================================================================
// Auth + Helpers
// ==========================================================================

func (h *ConsentHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}

		if token == "" {
			h.consentJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		user, err := h.verifyToken(r.Context(), token)
		if err != nil {
			h.consentJSONError(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func (h *ConsentHandler) verifyToken(ctx context.Context, tokenStr string) (*entity.User, error) {
	parts := splitToken(tokenStr)
	if parts == nil {
		return nil, errInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig := hmacSHA256([]byte(signingInput), h.jwtSigningKey)
	actualSig, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, errInvalidToken
	}
	if !hmacEqual(expectedSig, actualSig) {
		return nil, errInvalidToken
	}

	payloadBytes, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, errInvalidToken
	}

	var claims struct {
		Sub string `json:"sub"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, errInvalidToken
	}
	if time.Now().Unix() > claims.Exp {
		return nil, errInvalidToken
	}

	user, err := h.repo.GetUserByID(ctx, claims.Sub)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (h *ConsentHandler) consentJSONError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func generateConsentID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "cns-" + hex.EncodeToString(b)
}
