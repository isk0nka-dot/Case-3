package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
	"github.com/argus-ai/event-collector/internal/infrastructure/worker"
)

// AIAnalysisHandler provides a REST endpoint to trigger backend AI deep scans.
type AIAnalysisHandler struct {
	pgRepo        *postgres.Repository
	asynqClient   *asynq.Client
	logger        *zap.Logger
	jwtSigningKey []byte
}

// NewAIAnalysisHandler creates a new AI analysis REST handler.
func NewAIAnalysisHandler(
	pgRepo *postgres.Repository,
	asynqClient *asynq.Client,
	logger *zap.Logger,
	jwtSigningKey []byte,
) *AIAnalysisHandler {
	return &AIAnalysisHandler{
		pgRepo:        pgRepo,
		asynqClient:   asynqClient,
		logger:        logger.Named("ai_analysis_api"),
		jwtSigningKey: jwtSigningKey,
	}
}

// RegisterRoutes registers the AI analysis endpoints on the given mux.
func (h *AIAnalysisHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/sessions/{sessionId}/analyze", h.requireAuth(h.handleTriggerAnalysis))
}

// handleTriggerAnalysis enqueues a backend AI deep scan for the specified session.
//
//	POST /api/v1/sessions/{sessionId}/analyze
//	Body: { "analysis_type": "full_scan" }  (optional, defaults to "full_scan")
//	Response: 202 Accepted { "job_id": "ai:...", "status": "queued" }
func (h *AIAnalysisHandler) handleTriggerAnalysis(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.aiJSONError(w, "session_id is required", http.StatusBadRequest)
		return
	}

	if h.asynqClient == nil {
		h.aiJSONError(w, "job queue not configured (Redis not available)", http.StatusServiceUnavailable)
		return
	}

	// Parse optional request body.
	var req struct {
		AnalysisType string `json:"analysis_type"`
	}
	if r.Body != nil {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if req.AnalysisType == "" {
		req.AnalysisType = "full_scan"
	}

	// Look up session metadata from ClickHouse (via forensic scorer pattern).
	orgID, examID, studentID := h.resolveSessionMeta(r.Context(), sessionID)

	payload := worker.AIAnalysisPayload{
		SessionID:    sessionID,
		OrgID:        orgID,
		ExamID:       examID,
		StudentID:    studentID,
		AnalysisType: req.AnalysisType,
	}

	// Load per-exam proctoring settings and inject threshold overrides so the
	// worker respects exam-specific sensitivity levels instead of global defaults.
	if orgID != "" && examID != "" {
		if settings, err := h.pgRepo.GetExamProctoringSettings(r.Context(), orgID, examID); err == nil && settings != nil {
			payload.CleanThreshold = settings.CleanThreshold
			payload.WarningThreshold = settings.WarningThreshold
		}
	}

	task, err := worker.NewAIAnalysisTask(payload)
	if err != nil {
		h.logger.Error("failed to create AI analysis task",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		h.aiJSONError(w, "failed to create task", http.StatusInternalServerError)
		return
	}

	info, err := h.asynqClient.Enqueue(task)
	if err != nil {
		// Duplicate task ID is not an error — the job is already queued.
		if strings.Contains(err.Error(), "already exists") {
			h.aiJSONResponse(w, map[string]interface{}{
				"job_id":  fmt.Sprintf("ai:%s", sessionID),
				"status":  "already_queued",
				"message": "An AI analysis job for this session is already in the queue",
			}, http.StatusConflict)
			return
		}

		h.logger.Error("failed to enqueue AI analysis task",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		h.aiJSONError(w, "failed to enqueue task", http.StatusInternalServerError)
		return
	}

	h.logger.Info("AI analysis job enqueued",
		zap.String("session_id", sessionID),
		zap.String("task_id", info.ID),
		zap.String("queue", info.Queue),
		zap.String("analysis_type", req.AnalysisType),
	)

	h.aiJSONResponse(w, map[string]interface{}{
		"job_id":        info.ID,
		"queue":         info.Queue,
		"status":        "queued",
		"session_id":    sessionID,
		"analysis_type": req.AnalysisType,
	}, http.StatusAccepted)
}

// ---------------------------------------------------------------------------
// Session metadata resolution
// ---------------------------------------------------------------------------

// resolveSessionMeta attempts to look up org/exam/student IDs for a session.
// Returns empty strings if the lookup fails (non-fatal — the worker will proceed).
func (h *AIAnalysisHandler) resolveSessionMeta(_ context.Context, _ string) (orgID, examID, studentID string) {
	// The worker handler resolves session metadata from ClickHouse directly.
	// This endpoint is a thin trigger — metadata is optional for enqueue.
	return "", "", ""
}

// ---------------------------------------------------------------------------
// Auth middleware
// ---------------------------------------------------------------------------

func (h *AIAnalysisHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}

		if token == "" {
			h.aiJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		_, err := h.verifyToken(r.Context(), token)
		if err != nil {
			h.aiJSONError(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

func (h *AIAnalysisHandler) verifyToken(_ context.Context, tokenStr string) (string, error) {
	parts := splitToken(tokenStr)
	if parts == nil {
		return "", errInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig := hmacSHA256([]byte(signingInput), h.jwtSigningKey)
	actualSig, err := base64URLDecode(parts[2])
	if err != nil {
		return "", errInvalidToken
	}
	if !hmacEqual(expectedSig, actualSig) {
		return "", errInvalidToken
	}

	payloadBytes, err := base64URLDecode(parts[1])
	if err != nil {
		return "", errInvalidToken
	}

	var claims struct {
		Sub string `json:"sub"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return "", errInvalidToken
	}

	return claims.Sub, nil
}

// ---------------------------------------------------------------------------
// JSON response helpers
// ---------------------------------------------------------------------------

func (h *AIAnalysisHandler) aiJSONResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode JSON response", zap.Error(err))
	}
}

func (h *AIAnalysisHandler) aiJSONError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
