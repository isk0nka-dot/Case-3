// =============================================================================
// Argus AI — Bulk Export REST API Handler
// =============================================================================
//
// Provides HTTP endpoints for creating and managing bulk evidence export jobs.
// Export jobs run asynchronously via a background worker.
//
// Endpoints:
//   POST   /api/v1/export/bulk          — Create a new export job
//   GET    /api/v1/export/{exportId}     — Get export job status + download URL
//   GET    /api/v1/export               — List export jobs for the org
//   DELETE /api/v1/export/{exportId}     — Cancel a pending export job
// =============================================================================
package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/hibiken/asynq"
	"github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/config"
	"github.com/argus-ai/event-collector/internal/infrastructure/worker"
	"github.com/argus-ai/event-collector/pkg/randutil"
)

// ExportHandler serves the bulk export REST API.
type ExportHandler struct {
	db     *sql.DB
	cfg    config.ExportConfig
	logger *zap.Logger

	jwtSigningKey []byte
	repo          adminRepo

	// asynqClient dispatches export jobs to the background worker service.
	// When nil, the legacy PostgreSQL polling worker picks up jobs instead.
	asynqClient *asynq.Client
}

// NewExportHandler creates a new export API handler.
// If asynqClient is non-nil, export jobs are dispatched via Redis/asynq.
// If nil, the legacy PostgreSQL polling worker picks up jobs.
func NewExportHandler(
	db *sql.DB,
	cfg config.ExportConfig,
	repo adminRepo,
	logger *zap.Logger,
	jwtSigningKey []byte,
	asynqClient *asynq.Client,
) *ExportHandler {
	return &ExportHandler{
		db:            db,
		cfg:           cfg,
		logger:        logger.Named("export_api"),
		jwtSigningKey: jwtSigningKey,
		repo:          repo,
		asynqClient:   asynqClient,
	}
}

// RegisterRoutes registers export API endpoints on the given mux.
func (h *ExportHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/export/bulk", h.requireAuth(h.handleCreateExport))
	mux.HandleFunc("GET /api/v1/export/{exportId}", h.requireAuth(h.handleGetExport))
	mux.HandleFunc("GET /api/v1/export", h.requireAuth(h.handleListExports))
	mux.HandleFunc("DELETE /api/v1/export/{exportId}", h.requireAuth(h.handleCancelExport))
}

// ==========================================================================
// Request/Response Types
// ==========================================================================

type createExportRequest struct {
	SessionIDs    []string `json:"sessionIds"`
	PresignTTLSec int      `json:"presignTtlSec,omitempty"`
}

type exportResponse struct {
	ID             string         `json:"id"`
	OrgID          string         `json:"orgId"`
	RequestedBy    string         `json:"requestedBy"`
	SessionIDs     []string       `json:"sessionIds"`
	Status         string         `json:"status"`
	ArchiveURI     string         `json:"archiveUri,omitempty"`
	SHA256Archive  string         `json:"sha256Archive,omitempty"`
	DownloadURL    string         `json:"downloadUrl,omitempty"`
	ErrorMessage   string         `json:"errorMessage,omitempty"`
	TotalSizeBytes int64          `json:"totalSizeBytes,omitempty"`
	FragmentCount  int            `json:"fragmentCount,omitempty"`
	ExpiresAt      *time.Time     `json:"expiresAt,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
	CompletedAt    *time.Time     `json:"completedAt,omitempty"`
}

// ==========================================================================
// Handlers
// ==========================================================================

func (h *ExportHandler) handleCreateExport(w http.ResponseWriter, r *http.Request) {
	caller := getUserFromContext(r.Context())
	if caller == nil {
		h.exportJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Role check: only org_admin or super_admin can create exports.
	if caller.Role != "org_admin" && caller.Role != "super_admin" {
		h.exportJSONError(w, "Forbidden: org_admin or super_admin required", http.StatusForbidden)
		return
	}

	var req createExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.exportJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.SessionIDs) == 0 {
		h.exportJSONError(w, "sessionIds must not be empty", http.StatusBadRequest)
		return
	}

	if len(req.SessionIDs) > h.cfg.MaxSessionsPerExport {
		h.exportJSONError(w, "Too many sessions in a single export", http.StatusBadRequest)
		return
	}

	// Generate UUIDv7-like ID.
	id := generateExportID()

	presignTTL := req.PresignTTLSec
	if presignTTL <= 0 {
		presignTTL = int(h.cfg.PresignTTL.Seconds())
	}

	orgID := caller.OrgID
	if orgID == "*" {
		// Super admin may specify org_id; if omitted, defaults to "*".
		orgIDParam := r.URL.Query().Get("org_id")
		if orgIDParam != "" {
			orgID = orgIDParam
		}
	}

	_, err := h.db.ExecContext(r.Context(), `
		INSERT INTO export_jobs (id, org_id, requested_by, session_ids, presign_ttl_sec, status, created_at)
		VALUES ($1, $2, $3, $4, $5, 'pending', NOW())`,
		id, orgID, caller.ID, pq.Array(req.SessionIDs), presignTTL,
	)
	if err != nil {
		h.logger.Error("export: failed to create job", zap.Error(err))
		h.exportJSONError(w, "Failed to create export job", http.StatusInternalServerError)
		return
	}

	// Audit log.
	h.logger.Info("export job created",
		zap.String("export_id", id),
		zap.String("org_id", orgID),
		zap.String("requested_by", caller.ID),
		zap.Int("sessions", len(req.SessionIDs)),
	)

	// Dispatch to asynq worker if Redis queue is configured.
	// Otherwise, the legacy PostgreSQL polling worker will pick it up.
	dispatchMode := "polling"
	if h.asynqClient != nil {
		task, taskErr := worker.NewVideoExportTask(worker.VideoExportPayload{
			ExportID:      id,
			OrgID:         orgID,
			SessionIDs:    req.SessionIDs,
			PresignTTLSec: presignTTL,
		})
		if taskErr != nil {
			h.logger.Error("export: failed to create asynq task, falling back to polling",
				zap.Error(taskErr))
		} else {
			info, enqErr := h.asynqClient.Enqueue(task)
			if enqErr != nil {
				h.logger.Error("export: failed to enqueue asynq task, falling back to polling",
					zap.Error(enqErr))
			} else {
				dispatchMode = "asynq"
				h.logger.Info("export job dispatched to asynq",
					zap.String("export_id", id),
					zap.String("queue", info.Queue),
					zap.String("task_id", info.ID),
				)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if dispatchMode == "asynq" {
		w.WriteHeader(http.StatusAccepted)
	} else {
		w.WriteHeader(http.StatusCreated)
	}
	json.NewEncoder(w).Encode(map[string]string{
		"id":     id,
		"status": "pending",
	})
}

func (h *ExportHandler) handleGetExport(w http.ResponseWriter, r *http.Request) {
	exportID := r.PathValue("exportId")
	if exportID == "" {
		h.exportJSONError(w, "exportId is required", http.StatusBadRequest)
		return
	}

	caller := getUserFromContext(r.Context())
	if caller == nil {
		h.exportJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var resp exportResponse

	err := h.db.QueryRowContext(r.Context(), `
		SELECT id, org_id, requested_by, session_ids, status, archive_uri,
		       sha256_archive, download_url, error_message, total_size_bytes,
		       fragment_count, expires_at, created_at, completed_at
		FROM export_jobs
		WHERE id = $1`, exportID,
	).Scan(
		&resp.ID, &resp.OrgID, &resp.RequestedBy, pq.Array(&resp.SessionIDs), &resp.Status,
		&resp.ArchiveURI, &resp.SHA256Archive, &resp.DownloadURL, &resp.ErrorMessage,
		&resp.TotalSizeBytes, &resp.FragmentCount, &resp.ExpiresAt,
		&resp.CreatedAt, &resp.CompletedAt,
	)
	if err == sql.ErrNoRows {
		h.exportJSONError(w, "Export job not found", http.StatusNotFound)
		return
	}
	if err != nil {
		h.logger.Error("export: failed to query job", zap.Error(err))
		h.exportJSONError(w, "Failed to query export job", http.StatusInternalServerError)
		return
	}

	// Org access check.
	if caller.OrgID != "*" && resp.OrgID != caller.OrgID {
		h.exportJSONError(w, "Forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *ExportHandler) handleListExports(w http.ResponseWriter, r *http.Request) {
	caller := getUserFromContext(r.Context())
	if caller == nil {
		h.exportJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	orgID := caller.OrgID
	if orgID == "*" {
		if q := r.URL.Query().Get("org_id"); q != "" {
			orgID = q
		}
	}

	var rows *sql.Rows
	var err error

	if orgID == "*" {
		rows, err = h.db.QueryContext(r.Context(), `
			SELECT id, org_id, requested_by, session_ids, status, total_size_bytes, fragment_count, created_at, completed_at
			FROM export_jobs
			ORDER BY created_at DESC
			LIMIT 100`)
	} else {
		rows, err = h.db.QueryContext(r.Context(), `
			SELECT id, org_id, requested_by, session_ids, status, total_size_bytes, fragment_count, created_at, completed_at
			FROM export_jobs
			WHERE org_id = $1
			ORDER BY created_at DESC
			LIMIT 100`, orgID)
	}

	if err != nil {
		h.logger.Error("export: failed to list jobs", zap.Error(err))
		h.exportJSONError(w, "Failed to list export jobs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var exports []exportResponse
	for rows.Next() {
		var resp exportResponse
		if err := rows.Scan(
			&resp.ID, &resp.OrgID, &resp.RequestedBy, pq.Array(&resp.SessionIDs), &resp.Status,
			&resp.TotalSizeBytes, &resp.FragmentCount, &resp.CreatedAt, &resp.CompletedAt,
		); err != nil {
			continue
		}
		exports = append(exports, resp)
	}

	if exports == nil {
		exports = []exportResponse{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(exports)
}

func (h *ExportHandler) handleCancelExport(w http.ResponseWriter, r *http.Request) {
	exportID := r.PathValue("exportId")
	if exportID == "" {
		h.exportJSONError(w, "exportId is required", http.StatusBadRequest)
		return
	}

	caller := getUserFromContext(r.Context())
	if caller == nil {
		h.exportJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Org-scoped cancel: super_admin can cancel any, org_admin only their org.
	var result sql.Result
	var err error
	if caller.OrgID == "*" {
		result, err = h.db.ExecContext(r.Context(), `
			UPDATE export_jobs SET status = 'failed', error_message = 'Cancelled by user'
			WHERE id = $1 AND status = 'pending'`, exportID)
	} else {
		result, err = h.db.ExecContext(r.Context(), `
			UPDATE export_jobs SET status = 'failed', error_message = 'Cancelled by user'
			WHERE id = $1 AND status = 'pending' AND org_id = $2`, exportID, caller.OrgID)
	}
	if err != nil {
		h.logger.Error("export: failed to cancel job", zap.Error(err))
		h.exportJSONError(w, "Failed to cancel export", http.StatusInternalServerError)
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		h.exportJSONError(w, "Export job not found or not cancellable", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
}

// ==========================================================================
// Auth + Helpers
// ==========================================================================

func (h *ExportHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}

		if token == "" {
			h.exportJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		user, err := h.verifyToken(r.Context(), token)
		if err != nil {
			h.exportJSONError(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func (h *ExportHandler) verifyToken(ctx context.Context, tokenStr string) (*entity.User, error) {
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

func (h *ExportHandler) exportJSONError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// generateExportID creates a random export ID.
func generateExportID() string {
	return randutil.PrefixedID("exp-", 16)
}
