// =============================================================================
// Argus AI — Evidence REST API Handler
// =============================================================================
//
// Provides HTTP endpoints for evidence fragment retrieval, presigned URL
// generation, and integrity verification. Powers the evidence viewer in the
// archive dashboard.
//
// Endpoints:
//   GET  /api/v1/evidence/sessions/{sessionId}           — List fragments for session
//   GET  /api/v1/evidence/fragments/{fragmentId}/url     — Presigned download URL (5min)
//   GET  /api/v1/evidence/fragments/{fragmentId}/verify  — Re-hash + compare SHA-256
// =============================================================================
package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/clickhouse"
	minioStore "github.com/argus-ai/event-collector/internal/infrastructure/minio"
	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
	"go.uber.org/zap"
)

var errEvidenceNotFound = errors.New("evidence fragment not found")

// EvidenceHandler serves the evidence REST API for evidence fragment management.
type EvidenceHandler struct {
	chConn driver.Conn
	chMeta *clickhouse.Writer
	store  *minioStore.Store
	pgRepo *postgres.Repository
	logger *zap.Logger

	jwtSigningKey []byte
	repo          adminRepo
}

// NewEvidenceHandler creates a new evidence API handler.
func NewEvidenceHandler(
	chWriter *clickhouse.Writer,
	store *minioStore.Store,
	pgRepo *postgres.Repository,
	logger *zap.Logger,
	jwtSigningKey []byte,
) *EvidenceHandler {
	return &EvidenceHandler{
		chConn:        chWriter.Conn(),
		chMeta:        chWriter,
		store:         store,
		pgRepo:        pgRepo,
		logger:        logger.Named("evidence_api"),
		jwtSigningKey: jwtSigningKey,
		repo:          pgRepo,
	}
}

// RegisterRoutes registers evidence API endpoints on the given mux.
func (h *EvidenceHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/evidence/sessions/{sessionId}", h.requireAuth(h.handleListFragments))
	mux.HandleFunc("GET /api/v1/evidence/fragments/{fragmentId}/url", h.requireAuth(h.handlePresignURL))
	mux.HandleFunc("GET /api/v1/evidence/fragments/{fragmentId}/verify", h.requireAuth(h.handleVerifyIntegrity))
}

// ==========================================================================
// Response Types
// ==========================================================================

// EvidenceFragmentResponse represents an evidence fragment in API responses.
type EvidenceFragmentResponse struct {
	FragmentID  string  `json:"fragmentId"`
	SessionID   string  `json:"sessionId"`
	EventID     string  `json:"eventId"`
	OrgID       string  `json:"orgId"`
	ExamID      string  `json:"examId"`
	StudentID   string  `json:"studentId"`
	SHA256Hash  string  `json:"sha256Hash"`
	URI         string  `json:"uri"`
	SizeBytes   int64   `json:"sizeBytes"`
	ContentType string  `json:"contentType"`
	DurationSec float64 `json:"durationSec"`
	StartTime   string  `json:"startTime"`
	EndTime     string  `json:"endTime"`
	UploadedAt  string  `json:"uploadedAt"`
}

// EvidenceListResponse wraps the list of evidence fragments for a session.
type EvidenceListResponse struct {
	SessionID  string                     `json:"sessionId"`
	Fragments  []EvidenceFragmentResponse `json:"fragments"`
	Total      int                        `json:"total"`
	GeneratedAt string                    `json:"generatedAt"`
}

// PresignedURLResponse contains a time-limited download URL for evidence.
type PresignedURLResponse struct {
	FragmentID string `json:"fragmentId"`
	URL        string `json:"url"`
	ExpiresAt  string `json:"expiresAt"`
	TTLSeconds int    `json:"ttlSeconds"`
}

// VerifyIntegrityResponse contains the result of a SHA-256 integrity check.
type VerifyIntegrityResponse struct {
	FragmentID   string `json:"fragmentId"`
	Valid        bool   `json:"valid"`
	ExpectedHash string `json:"expectedHash"`
	Message      string `json:"message"`
}

// ==========================================================================
// Handlers
// ==========================================================================

// handleListFragments returns all evidence fragments for a given session.
// GET /api/v1/evidence/sessions/{sessionId}
func (h *EvidenceHandler) handleListFragments(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.jsonError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	// Resolve org scoping.
	orgID := h.resolveOrgID(r)
	if orgID == "" {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	fragments, err := h.queryFragments(ctx, sessionID, orgID)
	if err != nil {
		h.logger.Error("failed to query evidence fragments",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		h.jsonError(w, "Failed to retrieve evidence", http.StatusInternalServerError)
		return
	}

	// Audit log the access.
	h.logAccess(r, "evidence_list", sessionID)

	h.jsonResponse(w, EvidenceListResponse{
		SessionID:   sessionID,
		Fragments:   fragments,
		Total:       len(fragments),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}, http.StatusOK)
}

// handlePresignURL generates a time-limited presigned URL for downloading evidence.
// GET /api/v1/evidence/{fragmentId}/url
func (h *EvidenceHandler) handlePresignURL(w http.ResponseWriter, r *http.Request) {
	fragmentID := r.PathValue("fragmentId")
	if fragmentID == "" {
		h.jsonError(w, "fragmentId is required", http.StatusBadRequest)
		return
	}

	orgID := h.resolveOrgID(r)
	if orgID == "" {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Look up the fragment to get its URI and verify org access.
	fragment, err := h.queryFragmentByID(ctx, fragmentID, orgID)
	if err != nil {
		h.jsonError(w, "Evidence fragment not found", http.StatusNotFound)
		return
	}

	// Generate presigned URL.
	ttl := 5 * time.Minute
	url, err := h.store.PresignURL(ctx, fragment.URI, ttl)
	if err != nil {
		h.logger.Error("failed to generate presigned URL",
			zap.String("fragment_id", fragmentID),
			zap.Error(err),
		)
		h.jsonError(w, "Failed to generate download URL", http.StatusInternalServerError)
		return
	}

	h.logAccess(r, "evidence_download", fragmentID)

	h.jsonResponse(w, PresignedURLResponse{
		FragmentID: fragmentID,
		URL:        url,
		ExpiresAt:  time.Now().Add(ttl).UTC().Format(time.RFC3339),
		TTLSeconds: int(ttl.Seconds()),
	}, http.StatusOK)
}

// handleVerifyIntegrity re-downloads evidence and verifies SHA-256 hash.
// GET /api/v1/evidence/{fragmentId}/verify
func (h *EvidenceHandler) handleVerifyIntegrity(w http.ResponseWriter, r *http.Request) {
	fragmentID := r.PathValue("fragmentId")
	if fragmentID == "" {
		h.jsonError(w, "fragmentId is required", http.StatusBadRequest)
		return
	}

	orgID := h.resolveOrgID(r)
	if orgID == "" {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Look up the fragment.
	fragment, err := h.queryFragmentByID(ctx, fragmentID, orgID)
	if err != nil {
		h.jsonError(w, "Evidence fragment not found", http.StatusNotFound)
		return
	}

	// Verify integrity.
	valid, err := h.store.VerifyIntegrity(ctx, fragment.URI, fragment.SHA256Hash)
	if err != nil {
		h.logger.Error("integrity verification failed",
			zap.String("fragment_id", fragmentID),
			zap.Error(err),
		)
		h.jsonError(w, "Integrity verification failed", http.StatusInternalServerError)
		return
	}

	h.logAccess(r, "evidence_verify", fragmentID)

	msg := "Integrity verified — SHA-256 hash matches"
	if !valid {
		msg = "INTEGRITY VIOLATION — SHA-256 hash mismatch detected"
		h.logger.Error("EVIDENCE INTEGRITY VIOLATION",
			zap.String("fragment_id", fragmentID),
			zap.String("expected_hash", fragment.SHA256Hash),
		)
	}

	h.jsonResponse(w, VerifyIntegrityResponse{
		FragmentID:   fragmentID,
		Valid:        valid,
		ExpectedHash: fragment.SHA256Hash,
		Message:      msg,
	}, http.StatusOK)
}

// ==========================================================================
// ClickHouse Queries
// ==========================================================================

func (h *EvidenceHandler) queryFragments(ctx context.Context, sessionID string, orgID string) ([]EvidenceFragmentResponse, error) {
	query := `
		SELECT
			fragment_id, session_id, event_id, org_id, exam_id, student_id,
			sha256_hash, uri, size_bytes, content_type,
			duration_sec, start_time, end_time, uploaded_at
		FROM evidence_fragments
		WHERE session_id = ?`

	args := []interface{}{sessionID}

	if orgID != "*" {
		query += ` AND org_id = ?`
		args = append(args, orgID)
	}

	query += ` ORDER BY uploaded_at ASC`

	rows, err := h.chConn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fragments []EvidenceFragmentResponse
	for rows.Next() {
		var (
			fragmentID, sessionID2, eventID, orgID2, examID, studentID string
			sha256Hash, uri, contentType                               string
			sizeBytes                                                  int64
			durationSec                                                float64
			startTime, endTime, uploadedAt                             time.Time
		)

		if err := rows.Scan(
			&fragmentID, &sessionID2, &eventID, &orgID2, &examID, &studentID,
			&sha256Hash, &uri, &sizeBytes, &contentType,
			&durationSec, &startTime, &endTime, &uploadedAt,
		); err != nil {
			return nil, err
		}

		fragments = append(fragments, EvidenceFragmentResponse{
			FragmentID:  fragmentID,
			SessionID:   sessionID2,
			EventID:     eventID,
			OrgID:       orgID2,
			ExamID:      examID,
			StudentID:   studentID,
			SHA256Hash:  sha256Hash,
			URI:         uri,
			SizeBytes:   sizeBytes,
			ContentType: contentType,
			DurationSec: durationSec,
			StartTime:   startTime.Format(time.RFC3339),
			EndTime:     endTime.Format(time.RFC3339),
			UploadedAt:  uploadedAt.Format(time.RFC3339),
		})
	}

	if fragments == nil {
		fragments = []EvidenceFragmentResponse{}
	}

	return fragments, nil
}

func (h *EvidenceHandler) queryFragmentByID(ctx context.Context, fragmentID string, orgID string) (*EvidenceFragmentResponse, error) {
	query := `
		SELECT
			fragment_id, session_id, event_id, org_id, exam_id, student_id,
			sha256_hash, uri, size_bytes, content_type,
			duration_sec, start_time, end_time, uploaded_at
		FROM evidence_fragments
		WHERE fragment_id = ?`

	args := []interface{}{fragmentID}

	if orgID != "*" {
		query += ` AND org_id = ?`
		args = append(args, orgID)
	}

	query += ` LIMIT 1`

	rows, err := h.chConn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, errEvidenceNotFound
	}

	var (
		fID, sID, eID, oID, exID, stuID string
		sha256Hash, uri, contentType     string
		sizeBytes                        int64
		durationSec                      float64
		startTime, endTime, uploadedAt   time.Time
	)

	if err := rows.Scan(
		&fID, &sID, &eID, &oID, &exID, &stuID,
		&sha256Hash, &uri, &sizeBytes, &contentType,
		&durationSec, &startTime, &endTime, &uploadedAt,
	); err != nil {
		return nil, err
	}

	return &EvidenceFragmentResponse{
		FragmentID:  fID,
		SessionID:   sID,
		EventID:     eID,
		OrgID:       oID,
		ExamID:      exID,
		StudentID:   stuID,
		SHA256Hash:  sha256Hash,
		URI:         uri,
		SizeBytes:   sizeBytes,
		ContentType: contentType,
		DurationSec: durationSec,
		StartTime:   startTime.Format(time.RFC3339),
		EndTime:     endTime.Format(time.RFC3339),
		UploadedAt:  uploadedAt.Format(time.RFC3339),
	}, nil
}

// ==========================================================================
// Auth & Helpers (same pattern as ArchiveHandler)
// ==========================================================================

func (h *EvidenceHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}

		if token == "" {
			h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		user, err := h.verifyToken(r.Context(), token)
		if err != nil {
			h.jsonError(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func (h *EvidenceHandler) verifyToken(ctx context.Context, tokenStr string) (*entity.User, error) {
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

func (h *EvidenceHandler) resolveOrgID(r *http.Request) string {
	caller := getUserFromContext(r.Context())
	if caller == nil {
		return ""
	}

	if caller.OrgID == "*" {
		qOrg := r.URL.Query().Get("org_id")
		if qOrg != "" {
			return qOrg
		}
		return "*"
	}

	return caller.OrgID
}

func (h *EvidenceHandler) logAccess(r *http.Request, action string, resourceID string) {
	caller := getUserFromContext(r.Context())
	if caller == nil {
		return
	}

	h.logger.Info("evidence access",
		zap.String("action", action),
		zap.String("resource_id", resourceID),
		zap.String("user_id", caller.ID),
		zap.String("user_role", string(caller.Role)),
		zap.String("org_id", caller.OrgID),
		zap.String("ip", r.RemoteAddr),
	)

	// Write to audit log if PostgreSQL is available.
	if h.pgRepo != nil {
		_ = h.pgRepo.CreateAuditEntry(r.Context(), &entity.AuditEntry{
			UserID:       caller.ID,
			UserPhone:    caller.Phone,
			UserRole:     string(caller.Role),
			OrgID:        caller.OrgID,
			Action:       action,
			ResourceType: "evidence",
			ResourceID:   resourceID,
			IPAddress:    r.RemoteAddr,
			UserAgent:    r.Header.Get("User-Agent"),
		})
	}
}

func (h *EvidenceHandler) jsonResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *EvidenceHandler) jsonError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
