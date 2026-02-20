// =============================================================================
// Argus AI — Integrity Verification REST API Handler
// =============================================================================
//
// Provides HTTP endpoints for verifying the integrity of stored evidence
// against the forensic ledger. Cross-references S3 object hashes against
// ClickHouse records and validates cryptographic hash chains.
//
// Endpoints:
//   POST /api/v1/integrity/verify-session     — Verify all fragments + hash chain for a session
//   POST /api/v1/integrity/verify-fragment    — Verify a single fragment (S3 vs ClickHouse)
//   GET  /api/v1/integrity/session/{sessionId}/report — Get integrity report for a session
// =============================================================================
package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/application/port"
	"github.com/argus-ai/event-collector/internal/domain/entity"
)

// IntegrityHandler serves the integrity verification REST API.
type IntegrityHandler struct {
	verifier port.IntegrityVerifier
	logger   *zap.Logger

	jwtSigningKey []byte
	repo          adminRepo
}

// NewIntegrityHandler creates a new integrity verification API handler.
func NewIntegrityHandler(
	verifier port.IntegrityVerifier,
	repo adminRepo,
	logger *zap.Logger,
	jwtSigningKey []byte,
) *IntegrityHandler {
	return &IntegrityHandler{
		verifier:      verifier,
		logger:        logger.Named("integrity_api"),
		jwtSigningKey: jwtSigningKey,
		repo:          repo,
	}
}

// RegisterRoutes registers integrity API endpoints on the given mux.
func (h *IntegrityHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/integrity/verify-session", h.requireAuth(h.handleVerifySession))
	mux.HandleFunc("POST /api/v1/integrity/verify-fragment", h.requireAuth(h.handleVerifyFragment))
	mux.HandleFunc("GET /api/v1/integrity/session/{sessionId}/report", h.requireAuth(h.handleSessionReport))
}

// ==========================================================================
// Request/Response Types
// ==========================================================================

type verifySessionRequest struct {
	SessionID string `json:"sessionId"`
}

type verifyFragmentRequest struct {
	FragmentID string `json:"fragmentId"`
}

// ==========================================================================
// Handlers
// ==========================================================================

// handleVerifySession verifies all evidence fragments and hash chain for a session.
func (h *IntegrityHandler) handleVerifySession(w http.ResponseWriter, r *http.Request) {
	var req verifySessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.integrityJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" {
		h.integrityJSONError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	report, err := h.verifier.VerifySession(r.Context(), req.SessionID)
	if err != nil {
		h.logger.Error("integrity: session verification failed",
			zap.String("session_id", req.SessionID),
			zap.Error(err),
		)
		h.integrityJSONError(w, "Verification failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(report)
}

// handleVerifyFragment verifies a single evidence fragment against S3.
func (h *IntegrityHandler) handleVerifyFragment(w http.ResponseWriter, r *http.Request) {
	var req verifyFragmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.integrityJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.FragmentID == "" {
		h.integrityJSONError(w, "fragmentId is required", http.StatusBadRequest)
		return
	}

	result, err := h.verifier.VerifyFragment(r.Context(), req.FragmentID)
	if err != nil {
		h.logger.Error("integrity: fragment verification failed",
			zap.String("fragment_id", req.FragmentID),
			zap.Error(err),
		)
		h.integrityJSONError(w, "Verification failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

// handleSessionReport returns the integrity report for a session.
// This is a GET endpoint for easy browser-based access.
func (h *IntegrityHandler) handleSessionReport(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.integrityJSONError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	report, err := h.verifier.VerifySession(r.Context(), sessionID)
	if err != nil {
		h.logger.Error("integrity: session report generation failed",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		h.integrityJSONError(w, "Report generation failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(report)
}

// ==========================================================================
// Auth + Helpers (follows existing handler pattern)
// ==========================================================================

func (h *IntegrityHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}

		if token == "" {
			h.integrityJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		user, err := h.verifyToken(r.Context(), token)
		if err != nil {
			h.integrityJSONError(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func (h *IntegrityHandler) verifyToken(ctx context.Context, tokenStr string) (*entity.User, error) {
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

func (h *IntegrityHandler) integrityJSONError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
