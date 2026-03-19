// =============================================================================
// Argus AI — Appeals REST API Handler
// =============================================================================
//
// Provides HTTP endpoints for the student appeals workflow. Students can
// appeal proctoring violation decisions, and admins can review them.
//
// State Machine:
//   submitted -> under_review -> upheld | overturned | withdrawn
//   submitted -> withdrawn
//
// Endpoints:
//   POST /api/v1/appeals                    — Submit a new appeal
//   GET  /api/v1/appeals                    — List appeals for org
//   GET  /api/v1/appeals/{appealId}         — Get appeal details
//   PUT  /api/v1/appeals/{appealId}/review  — Update appeal status (admin)
// =============================================================================
package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/pkg/randutil"

	"github.com/argus-ai/event-collector/internal/domain/entity"
)

// AppealsHandler serves the appeals REST API.
type AppealsHandler struct {
	db     *sql.DB
	logger *zap.Logger

	jwtSigningKey []byte
	repo          adminRepo
}

// NewAppealsHandler creates a new appeals API handler.
func NewAppealsHandler(
	db *sql.DB,
	repo adminRepo,
	logger *zap.Logger,
	jwtSigningKey []byte,
) *AppealsHandler {
	return &AppealsHandler{
		db:            db,
		logger:        logger.Named("appeals_api"),
		jwtSigningKey: jwtSigningKey,
		repo:          repo,
	}
}

// RegisterRoutes registers appeals API endpoints on the given mux.
func (h *AppealsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/appeals", h.requireAuth(h.handleSubmitAppeal))
	mux.HandleFunc("GET /api/v1/appeals", h.requireAuth(h.handleListAppeals))
	mux.HandleFunc("GET /api/v1/appeals/{appealId}", h.requireAuth(h.handleGetAppeal))
	mux.HandleFunc("PUT /api/v1/appeals/{appealId}/review", h.requireAuth(h.handleReviewAppeal))
}

// ==========================================================================
// Request/Response Types
// ==========================================================================

type submitAppealRequest struct {
	SessionID string `json:"sessionId"`
	StudentID string `json:"studentId"`
	OrgID     string `json:"orgId"`
	ExamID    string `json:"examId"`
	Reason    string `json:"reason"`
}

type reviewAppealRequest struct {
	Status      string `json:"status"` // upheld, overturned, withdrawn
	ReviewNotes string `json:"reviewNotes"`
}

type appealResponse struct {
	ID          string     `json:"id"`
	SessionID   string     `json:"sessionId"`
	StudentID   string     `json:"studentId"`
	OrgID       string     `json:"orgId"`
	ExamID      string     `json:"examId"`
	Reason      string     `json:"reason"`
	Status      string     `json:"status"`
	ReviewedBy  string     `json:"reviewedBy,omitempty"`
	ReviewNotes string     `json:"reviewNotes,omitempty"`
	ReviewedAt  *time.Time `json:"reviewedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// ==========================================================================
// Handlers
// ==========================================================================

func (h *AppealsHandler) handleSubmitAppeal(w http.ResponseWriter, r *http.Request) {
	var req submitAppealRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.appealsJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" || req.Reason == "" {
		h.appealsJSONError(w, "sessionId and reason are required", http.StatusBadRequest)
		return
	}

	// ── Org isolation: NEVER trust client-supplied org_id ──────────────────
	// Force org_id to the caller's organization. Super admin may override
	// via the ?org_id= query parameter for cross-org operations.
	caller := getUserFromContext(r.Context())
	if caller == nil {
		h.appealsJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	orgID := caller.OrgID
	if orgID == "*" {
		if override := r.URL.Query().Get("org_id"); override != "" {
			orgID = override
		}
	}
	if orgID == "*" {
		h.appealsJSONError(w, "org_id query parameter is required for super_admin", http.StatusBadRequest)
		return
	}

	// Override the client-provided value with the verified org_id.
	req.OrgID = orgID

	id := generateAppealID()

	_, err := h.db.ExecContext(r.Context(), `
		INSERT INTO appeals (id, session_id, student_id, org_id, exam_id, reason, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'submitted', NOW(), NOW())`,
		id, req.SessionID, req.StudentID, req.OrgID, req.ExamID, req.Reason,
	)
	if err != nil {
		h.logger.Error("appeals: failed to submit", zap.Error(err))
		h.appealsJSONError(w, "Failed to submit appeal", http.StatusInternalServerError)
		return
	}

	h.logger.Info("appeal submitted",
		zap.String("id", id),
		zap.String("org_id", req.OrgID),
		zap.String("session_id", req.SessionID),
		zap.String("student_id", req.StudentID),
		zap.String("submitted_by", caller.ID),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(appealResponse{
		ID:        id,
		SessionID: req.SessionID,
		StudentID: req.StudentID,
		OrgID:     req.OrgID,
		ExamID:    req.ExamID,
		Reason:    req.Reason,
		Status:    "submitted",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
}

func (h *AppealsHandler) handleListAppeals(w http.ResponseWriter, r *http.Request) {
	caller := getUserFromContext(r.Context())
	if caller == nil {
		h.appealsJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	orgID := caller.OrgID
	if orgID == "*" {
		if q := r.URL.Query().Get("org_id"); q != "" {
			orgID = q
		}
	}

	statusFilter := r.URL.Query().Get("status")

	var rows *sql.Rows
	var err error

	if statusFilter != "" {
		if orgID == "*" {
			rows, err = h.db.QueryContext(r.Context(), `
				SELECT id, session_id, student_id, org_id, exam_id, reason, status,
				       reviewed_by, review_notes, reviewed_at, created_at, updated_at
				FROM appeals WHERE status = $1
				ORDER BY created_at DESC LIMIT 100`, statusFilter)
		} else {
			rows, err = h.db.QueryContext(r.Context(), `
				SELECT id, session_id, student_id, org_id, exam_id, reason, status,
				       reviewed_by, review_notes, reviewed_at, created_at, updated_at
				FROM appeals WHERE org_id = $1 AND status = $2
				ORDER BY created_at DESC LIMIT 100`, orgID, statusFilter)
		}
	} else {
		if orgID == "*" {
			rows, err = h.db.QueryContext(r.Context(), `
				SELECT id, session_id, student_id, org_id, exam_id, reason, status,
				       reviewed_by, review_notes, reviewed_at, created_at, updated_at
				FROM appeals ORDER BY created_at DESC LIMIT 100`)
		} else {
			rows, err = h.db.QueryContext(r.Context(), `
				SELECT id, session_id, student_id, org_id, exam_id, reason, status,
				       reviewed_by, review_notes, reviewed_at, created_at, updated_at
				FROM appeals WHERE org_id = $1
				ORDER BY created_at DESC LIMIT 100`, orgID)
		}
	}

	if err != nil {
		h.logger.Error("appeals: failed to list", zap.Error(err))
		h.appealsJSONError(w, "Failed to list appeals", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var appeals []appealResponse
	for rows.Next() {
		var a appealResponse
		if err := rows.Scan(
			&a.ID, &a.SessionID, &a.StudentID, &a.OrgID, &a.ExamID, &a.Reason, &a.Status,
			&a.ReviewedBy, &a.ReviewNotes, &a.ReviewedAt, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			continue
		}
		appeals = append(appeals, a)
	}

	if appeals == nil {
		appeals = []appealResponse{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(appeals)
}

func (h *AppealsHandler) handleGetAppeal(w http.ResponseWriter, r *http.Request) {
	appealID := r.PathValue("appealId")
	if appealID == "" {
		h.appealsJSONError(w, "appealId is required", http.StatusBadRequest)
		return
	}

	// ── Org isolation: verify caller has access to this appeal ─────────────
	caller := getUserFromContext(r.Context())
	if caller == nil {
		h.appealsJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var a appealResponse
	err := h.db.QueryRowContext(r.Context(), `
		SELECT id, session_id, student_id, org_id, exam_id, reason, status,
		       reviewed_by, review_notes, reviewed_at, created_at, updated_at
		FROM appeals WHERE id = $1`, appealID,
	).Scan(
		&a.ID, &a.SessionID, &a.StudentID, &a.OrgID, &a.ExamID, &a.Reason, &a.Status,
		&a.ReviewedBy, &a.ReviewNotes, &a.ReviewedAt, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		h.appealsJSONError(w, "Appeal not found", http.StatusNotFound)
		return
	}
	if err != nil {
		h.logger.Error("appeals: failed to get", zap.Error(err))
		h.appealsJSONError(w, "Failed to get appeal", http.StatusInternalServerError)
		return
	}

	// Org access check: non-super-admin can only see their own org's appeals.
	// Returns 404 (not 403) to prevent appeal ID enumeration across orgs.
	if caller.OrgID != "*" && a.OrgID != caller.OrgID {
		h.appealsJSONError(w, "Appeal not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a)
}

func (h *AppealsHandler) handleReviewAppeal(w http.ResponseWriter, r *http.Request) {
	appealID := r.PathValue("appealId")
	if appealID == "" {
		h.appealsJSONError(w, "appealId is required", http.StatusBadRequest)
		return
	}

	caller := getUserFromContext(r.Context())
	if caller == nil {
		h.appealsJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Only admins can review appeals.
	if caller.Role != "org_admin" && caller.Role != "super_admin" {
		h.appealsJSONError(w, "Forbidden: admin role required", http.StatusForbidden)
		return
	}

	var req reviewAppealRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.appealsJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate the requested status.
	validStatuses := map[string]bool{
		"under_review": true,
		"upheld":       true,
		"overturned":   true,
		"withdrawn":    true,
	}
	if !validStatuses[req.Status] {
		h.appealsJSONError(w, "Invalid status: must be under_review, upheld, overturned, or withdrawn", http.StatusBadRequest)
		return
	}

	// ── Org isolation: fetch appeal status AND org_id in a single query ────
	// Verify the caller's org matches the appeal's org before allowing update.
	var currentStatus string
	var appealOrgID string
	err := h.db.QueryRowContext(r.Context(),
		`SELECT status, org_id FROM appeals WHERE id = $1`, appealID,
	).Scan(&currentStatus, &appealOrgID)
	if err == sql.ErrNoRows {
		h.appealsJSONError(w, "Appeal not found", http.StatusNotFound)
		return
	}
	if err != nil {
		h.logger.Error("appeals: failed to query", zap.Error(err))
		h.appealsJSONError(w, "Failed to query appeal", http.StatusInternalServerError)
		return
	}

	// Org access check: org_admin can only review appeals within their org.
	if caller.OrgID != "*" && appealOrgID != caller.OrgID {
		h.appealsJSONError(w, "Appeal not found", http.StatusNotFound)
		return
	}

	// Validate state transition using domain entity.
	appeal := &entity.Appeal{Status: entity.AppealStatus(currentStatus)}
	if err := appeal.ValidateTransition(entity.AppealStatus(req.Status)); err != nil {
		h.appealsJSONError(w, err.Error(), http.StatusConflict)
		return
	}

	// Update appeal with org_id in WHERE clause as defense-in-depth.
	_, err = h.db.ExecContext(r.Context(), `
		UPDATE appeals SET status = $1, reviewed_by = $2, review_notes = $3,
		       reviewed_at = NOW(), updated_at = NOW()
		WHERE id = $4 AND org_id = $5`,
		req.Status, caller.ID, req.ReviewNotes, appealID, appealOrgID,
	)
	if err != nil {
		h.logger.Error("appeals: failed to update", zap.Error(err))
		h.appealsJSONError(w, "Failed to update appeal", http.StatusInternalServerError)
		return
	}

	h.logger.Info("appeal reviewed",
		zap.String("appeal_id", appealID),
		zap.String("new_status", req.Status),
		zap.String("reviewed_by", caller.ID),
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"id":     appealID,
		"status": req.Status,
	})
}

// ==========================================================================
// Auth + Helpers
// ==========================================================================

func (h *AppealsHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}

		if token == "" {
			h.appealsJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		user, err := h.verifyToken(r.Context(), token)
		if err != nil {
			h.appealsJSONError(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func (h *AppealsHandler) verifyToken(ctx context.Context, tokenStr string) (*entity.User, error) {
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

func (h *AppealsHandler) appealsJSONError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func generateAppealID() string {
	return randutil.PrefixedID("apl-", 16)
}
