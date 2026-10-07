// =============================================================================
// Argus AI — Forensic Reporting REST API Handler
// =============================================================================
//
// Provides HTTP endpoints for generating, retrieving, and verifying forensic
// integrity reports. Integrates the forensic scorer, heatmap generator, and
// PDF engine.
//
// Endpoints:
//   GET  /api/v1/forensic/session/{sessionId}/report      — Full JSON forensic report
//   GET  /api/v1/forensic/session/{sessionId}/score       — Integrity score only
//   GET  /api/v1/forensic/session/{sessionId}/pdf         — PDF forensic report download (sync)
//   POST /api/v1/forensic/session/{sessionId}/pdf/async   — Enqueue async PDF generation via asynq
//   GET  /api/v1/forensic/session/{sessionId}/heatmap     — SVG gaze heatmap
//   GET  /api/v1/forensic/session/{sessionId}/voice       — Voice biometric analysis
//   POST /api/v1/forensic/verify                          — Verify a PDF report hash
// =============================================================================
package http

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/application/port"
	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/forensic"
	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
	"github.com/argus-ai/event-collector/internal/infrastructure/worker"
	"github.com/argus-ai/event-collector/pkg/randutil"
)

// ForensicHandler serves the forensic reporting REST API.
type ForensicHandler struct {
	scorer   *forensic.Scorer
	verifier port.IntegrityVerifier
	pgRepo   *postgres.Repository
	logger   *zap.Logger

	jwtSigningKey []byte
	repo          adminRepo

	// asynqClient dispatches forensic PDF jobs to the background worker service.
	// When nil, the synchronous GET .../pdf endpoint is the only option.
	asynqClient *asynq.Client
}

// NewForensicHandler creates a new forensic reporting API handler.
// If asynqClient is non-nil, the async PDF endpoint dispatches jobs via Redis/asynq.
func NewForensicHandler(
	chConn driver.Conn,
	verifier port.IntegrityVerifier,
	pgRepo *postgres.Repository,
	logger *zap.Logger,
	jwtSigningKey []byte,
	asynqClient *asynq.Client,
) *ForensicHandler {
	return &ForensicHandler{
		scorer:        forensic.NewScorer(chConn, logger),
		verifier:      verifier,
		pgRepo:        pgRepo,
		logger:        logger.Named("forensic_api"),
		jwtSigningKey: jwtSigningKey,
		repo:          pgRepo,
		asynqClient:   asynqClient,
	}
}

// RegisterRoutes registers forensic API endpoints on the given mux.
func (h *ForensicHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/forensic/session/{sessionId}/report", h.requireAuth(h.handleReport))
	mux.HandleFunc("GET /api/v1/forensic/session/{sessionId}/score", h.requireAuth(h.handleScore))
	mux.HandleFunc("GET /api/v1/forensic/session/{sessionId}/pdf", h.requireAuth(h.handlePDF))
	mux.HandleFunc("POST /api/v1/forensic/session/{sessionId}/pdf/async", h.requireAuth(h.handleAsyncPDF))
	mux.HandleFunc("GET /api/v1/forensic/session/{sessionId}/heatmap", h.requireAuth(h.handleHeatmap))
	mux.HandleFunc("GET /api/v1/forensic/session/{sessionId}/voice", h.requireAuth(h.handleVoice))
	mux.HandleFunc("POST /api/v1/forensic/verify", h.requireAuth(h.handleVerify))
}

// ==========================================================================
// Handlers
// ==========================================================================

// handleReport generates the full forensic report for a session.
func (h *ForensicHandler) handleReport(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.forensicJSONError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	report, err := h.buildForensicReport(r.Context(), sessionID)
	if err != nil {
		h.logger.Error("forensic: report generation failed",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		h.forensicJSONError(w, "Report generation failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

// handleScore returns only the integrity score for a session.
func (h *ForensicHandler) handleScore(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.forensicJSONError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	// Attempt config-aware scoring.
	var score *forensic.IntegrityScore
	var err error

	if h.pgRepo != nil {
		orgID, examID, _, metaErr := h.scorer.QuerySessionMeta(r.Context(), sessionID)
		if metaErr == nil && orgID != "" && examID != "" {
			settings, settingsErr := h.pgRepo.GetExamProctoringSettings(r.Context(), orgID, examID)
			if settingsErr == nil && settings != nil {
				score, err = h.scorer.ComputeScoreWithConfig(r.Context(), sessionID, settings)
			}
		}
	}

	if score == nil && err == nil {
		score, err = h.scorer.ComputeScore(r.Context(), sessionID)
	}

	if err != nil {
		h.logger.Error("forensic: score computation failed",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		h.forensicJSONError(w, "Score computation failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(score)
}

// handlePDF generates and serves a PDF forensic report.
func (h *ForensicHandler) handlePDF(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.forensicJSONError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	report, err := h.buildForensicReport(r.Context(), sessionID)
	if err != nil {
		h.logger.Error("forensic: pdf generation failed",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		h.forensicJSONError(w, "PDF generation failed", http.StatusInternalServerError)
		return
	}

	pdfBytes, pdfHash, err := forensic.GenerateForensicPDF(report)
	if err != nil {
		h.logger.Error("forensic: pdf rendering failed",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		h.forensicJSONError(w, "PDF rendering failed", http.StatusInternalServerError)
		return
	}

	h.logger.Info("forensic: pdf generated",
		zap.String("session_id", sessionID),
		zap.Int("size_bytes", len(pdfBytes)),
		zap.String("pdf_hash", pdfHash),
	)

	filename := fmt.Sprintf("argus-forensic-%s.pdf", sessionID[:min(12, len(sessionID))])
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pdfBytes)))
	w.Header().Set("X-Report-Hash", pdfHash)
	if _, err := w.Write(pdfBytes); err != nil {
		h.logger.Error("Failed to write PDF response", zap.Error(err))
	}
}

// handleAsyncPDF enqueues a forensic PDF generation job via asynq and returns
// 202 Accepted with a job ID. The worker service generates the PDF, uploads it
// to MinIO, and sends a Telegram notification when ready.
func (h *ForensicHandler) handleAsyncPDF(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.forensicJSONError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	if h.asynqClient == nil {
		h.forensicJSONError(w, "Async PDF generation is not available — Redis not configured", http.StatusServiceUnavailable)
		return
	}

	// Extract requesting user from auth context.
	user, _ := r.Context().Value(userContextKey).(*entity.User)
	requestedBy := ""
	if user != nil {
		requestedBy = user.ID
	}

	// Resolve orgID from session metadata via the scorer.
	orgID, _, _, _ := h.scorer.QuerySessionMeta(r.Context(), sessionID)

	jobID := "FR-" + randutil.HexToken(12)

	task, err := worker.NewForensicReportTask(worker.ForensicReportPayload{
		SessionID:   sessionID,
		RequestedBy: requestedBy,
		OrgID:       orgID,
	})
	if err != nil {
		h.logger.Error("forensic: failed to create asynq task",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		h.forensicJSONError(w, "Failed to enqueue PDF job", http.StatusInternalServerError)
		return
	}

	info, err := h.asynqClient.Enqueue(task)
	if err != nil {
		h.logger.Error("forensic: failed to enqueue PDF job",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		h.forensicJSONError(w, "Failed to enqueue PDF job", http.StatusInternalServerError)
		return
	}

	h.logger.Info("forensic: PDF job enqueued",
		zap.String("session_id", sessionID),
		zap.String("job_id", jobID),
		zap.String("asynq_id", info.ID),
		zap.String("queue", info.Queue),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"jobId":     jobID,
		"asynqId":   info.ID,
		"sessionId": sessionID,
		"status":    "queued",
	})
}

// handleHeatmap generates an SVG gaze heatmap for a session.
func (h *ForensicHandler) handleHeatmap(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.forensicJSONError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	gazeData, err := h.scorer.QueryGazeData(r.Context(), sessionID)
	if err != nil {
		h.logger.Error("forensic: gaze query failed",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		h.forensicJSONError(w, "Gaze data query failed", http.StatusInternalServerError)
		return
	}

	svg := forensic.GenerateGazeHeatmapSVG(gazeData, nil)

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if _, err := w.Write([]byte(svg)); err != nil {
		h.logger.Error("Failed to write SVG response", zap.Error(err))
	}
}

// handleVoice returns voice biometric analysis for a session.
func (h *ForensicHandler) handleVoice(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.forensicJSONError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	voice, err := h.scorer.QueryVoiceBiometric(r.Context(), sessionID)
	if err != nil {
		h.logger.Error("forensic: voice analysis failed",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		h.forensicJSONError(w, "Voice analysis failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(voice)
}

// handleVerify verifies a PDF report hash against the session's forensic data.
func (h *ForensicHandler) handleVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID  string `json:"sessionId"`
		ReportHash string `json:"reportHash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.forensicJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" || req.ReportHash == "" {
		h.forensicJSONError(w, "sessionId and reportHash are required", http.StatusBadRequest)
		return
	}

	// Regenerate the report and compare hashes
	report, err := h.buildForensicReport(r.Context(), req.SessionID)
	if err != nil {
		h.forensicJSONError(w, "Verification failed: cannot regenerate report", http.StatusInternalServerError)
		return
	}

	reportJSON, _ := json.Marshal(report)
	expectedHash := fmt.Sprintf("%x", sha256.Sum256(reportJSON))

	result := map[string]interface{}{
		"sessionId":    req.SessionID,
		"submitted":    req.ReportHash,
		"expected":     expectedHash,
		"match":        req.ReportHash == expectedHash,
		"verified":     req.ReportHash == expectedHash,
		"verifiedAt":   time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ==========================================================================
// Report Builder
// ==========================================================================

func (h *ForensicHandler) buildForensicReport(ctx context.Context, sessionID string) (*forensic.ForensicReport, error) {
	// Attempt config-aware scoring: look up per-exam settings from PostgreSQL.
	// If custom settings exist, use dynamic scoring. Otherwise, fall back to defaults.
	var score *forensic.IntegrityScore
	var err error

	if h.pgRepo != nil {
		orgID, examID, _, metaErr := h.scorer.QuerySessionMeta(ctx, sessionID)
		if metaErr == nil && orgID != "" && examID != "" {
			settings, settingsErr := h.pgRepo.GetExamProctoringSettings(ctx, orgID, examID)
			if settingsErr == nil && settings != nil {
				score, err = h.scorer.ComputeScoreWithConfig(ctx, sessionID, settings)
				if err != nil {
					return nil, fmt.Errorf("dynamic score computation: %w", err)
				}
			}
		}
	}

	// Fallback to hardcoded defaults if no custom settings or metadata unavailable.
	if score == nil {
		score, err = h.scorer.ComputeScore(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("score computation: %w", err)
		}
	}

	timeline, err := h.scorer.QueryTimeline(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("timeline query: %w", err)
	}

	deviceInfo, err := h.scorer.QueryDeviceInfo(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("device info query: %w", err)
	}

	gazeData, err := h.scorer.QueryGazeData(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("gaze data query: %w", err)
	}

	voice, err := h.scorer.QueryVoiceBiometric(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("voice biometric query: %w", err)
	}

	// Fix 9: Secondary camera summary
	sidecamSummary, err := h.scorer.QuerySidecamSummary(ctx, sessionID)
	if err != nil {
		h.logger.Warn("forensic: sidecam summary query failed, continuing",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		sidecamSummary = &forensic.SidecamSummary{}
	}

	// Integrity verification (hash chain + S3)
	var ledger forensic.LedgerSummary
	if h.verifier != nil {
		intReport, err := h.verifier.VerifySession(ctx, sessionID)
		if err != nil {
			h.logger.Warn("forensic: ledger verification failed, continuing",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		} else if intReport != nil {
			ledger = forensic.LedgerSummary{
				TotalFragments: intReport.TotalFragments,
				VerifiedOK:     intReport.VerifiedOK,
				ChainValid:     intReport.ChainValid,
				S3Verified:     intReport.S3Verified,
				S3Mismatches:   intReport.S3Mismatches,
			}
		}
	}

	// Build report
	reportID := fmt.Sprintf("FR-%s-%d", sessionID[:min(8, len(sessionID))], time.Now().UnixMilli())

	report := &forensic.ForensicReport{
		ReportID:      reportID,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		SessionID:     sessionID,
		StudentID:     score.StudentID,
		ExamID:        score.ExamID,
		OrgID:         score.OrgID,
		Integrity:     *score,
		Voice:         *voice,
		Timeline:      timeline,
		DeviceInfo:    *deviceInfo,
		GazeData:      gazeData,
		LedgerSummary: ledger,
		Sidecam:       *sidecamSummary,
	}

	// Compute report hash
	reportJSON, _ := json.Marshal(report)
	hash := sha256.Sum256(reportJSON)
	report.ReportHash = fmt.Sprintf("%x", hash[:])

	return report, nil
}

// ==========================================================================
// Auth + Helpers (follows existing handler pattern)
// ==========================================================================

func (h *ForensicHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}

		// Support token via query parameter for PDF/SVG downloads (browser direct links)
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if token == "" {
			h.forensicJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		user, err := h.verifyToken(r.Context(), token)
		if err != nil {
			h.forensicJSONError(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func (h *ForensicHandler) verifyToken(ctx context.Context, tokenStr string) (*entity.User, error) {
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

func (h *ForensicHandler) forensicJSONError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
