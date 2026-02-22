// =============================================================================
// Argus AI — Proctoring Settings REST API Handler
// =============================================================================
//
// Provides CRUD endpoints for per-exam proctoring rule configuration.
// These settings control which penalty rules the forensic scorer applies
// and with what weights, dynamically linking the Admin UI toggles to
// backend enforcement.
//
// Endpoints:
//   GET    /api/v1/proctoring/settings/{orgId}/{examId}  — Get exam settings
//   PUT    /api/v1/proctoring/settings/{orgId}/{examId}  — Save/update settings
//   DELETE /api/v1/proctoring/settings/{orgId}/{examId}  — Delete settings
//   GET    /api/v1/proctoring/settings/{orgId}           — List org settings
//   GET    /api/v1/proctoring/rule-registry               — Default rules + toggle map
// =============================================================================
package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/forensic"
	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
	"go.uber.org/zap"
)

// ProctoringSettingsHandler serves the proctoring settings REST API.
type ProctoringSettingsHandler struct {
	pgRepo *postgres.Repository
	logger *zap.Logger

	jwtSigningKey []byte
	repo          adminRepo
}

// NewProctoringSettingsHandler creates a new proctoring settings API handler.
func NewProctoringSettingsHandler(
	pgRepo *postgres.Repository,
	logger *zap.Logger,
	jwtSigningKey []byte,
) *ProctoringSettingsHandler {
	return &ProctoringSettingsHandler{
		pgRepo:        pgRepo,
		logger:        logger.Named("proctoring_settings_api"),
		jwtSigningKey: jwtSigningKey,
		repo:          pgRepo,
	}
}

// RegisterRoutes registers proctoring settings API endpoints on the given mux.
func (h *ProctoringSettingsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/proctoring/settings/{orgId}/{examId}", h.requireAuth(h.handleGetSettings))
	mux.HandleFunc("PUT /api/v1/proctoring/settings/{orgId}/{examId}", h.requireAuth(h.handleSaveSettings))
	mux.HandleFunc("DELETE /api/v1/proctoring/settings/{orgId}/{examId}", h.requireAuth(h.handleDeleteSettings))
	mux.HandleFunc("GET /api/v1/proctoring/settings/{orgId}", h.requireAuth(h.handleListSettings))
	mux.HandleFunc("GET /api/v1/proctoring/rule-registry", h.requireAuth(h.handleRuleRegistry))
}

// ==========================================================================
// Handlers
// ==========================================================================

func (h *ProctoringSettingsHandler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	examID := r.PathValue("examId")
	if orgID == "" || examID == "" {
		h.jsonError(w, "orgId and examId are required", http.StatusBadRequest)
		return
	}

	settings, err := h.pgRepo.GetExamProctoringSettings(r.Context(), orgID, examID)
	if err != nil {
		h.logger.Error("proctoring settings: get failed",
			zap.String("org_id", orgID),
			zap.String("exam_id", examID),
			zap.Error(err),
		)
		h.jsonError(w, "Failed to retrieve settings", http.StatusInternalServerError)
		return
	}

	if settings == nil {
		// Return defaults if no custom settings exist
		defaults := entity.DefaultExamProctoringSettings()
		defaults.OrgID = orgID
		defaults.ExamID = examID
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Argus-Settings-Source", "defaults")
		json.NewEncoder(w).Encode(defaults)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Argus-Settings-Source", "database")
	json.NewEncoder(w).Encode(settings)
}

func (h *ProctoringSettingsHandler) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	examID := r.PathValue("examId")
	if orgID == "" || examID == "" {
		h.jsonError(w, "orgId and examId are required", http.StatusBadRequest)
		return
	}

	var settings entity.ExamProctoringSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Enforce path parameters over body values
	settings.OrgID = orgID
	settings.ExamID = examID
	settings.UpdatedAt = time.Now().UTC()

	// Extract updatedBy from JWT user context
	if user, ok := r.Context().Value(userContextKey).(*entity.User); ok {
		settings.UpdatedBy = user.ID
	}

	if err := h.pgRepo.SaveExamProctoringSettings(r.Context(), &settings); err != nil {
		h.logger.Error("proctoring settings: save failed",
			zap.String("org_id", orgID),
			zap.String("exam_id", examID),
			zap.Error(err),
		)
		h.jsonError(w, "Failed to save settings", http.StatusInternalServerError)
		return
	}

	h.logger.Info("proctoring settings saved",
		zap.String("org_id", orgID),
		zap.String("exam_id", examID),
		zap.String("updated_by", settings.UpdatedBy),
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "saved",
		"orgId":   orgID,
		"examId":  examID,
		"savedAt": settings.UpdatedAt,
	})
}

func (h *ProctoringSettingsHandler) handleDeleteSettings(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	examID := r.PathValue("examId")
	if orgID == "" || examID == "" {
		h.jsonError(w, "orgId and examId are required", http.StatusBadRequest)
		return
	}

	if err := h.pgRepo.DeleteExamProctoringSettings(r.Context(), orgID, examID); err != nil {
		h.logger.Error("proctoring settings: delete failed",
			zap.String("org_id", orgID),
			zap.String("exam_id", examID),
			zap.Error(err),
		)
		h.jsonError(w, "Failed to delete settings", http.StatusInternalServerError)
		return
	}

	h.logger.Info("proctoring settings deleted",
		zap.String("org_id", orgID),
		zap.String("exam_id", examID),
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "deleted",
		"orgId":  orgID,
		"examId": examID,
	})
}

func (h *ProctoringSettingsHandler) handleListSettings(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	if orgID == "" {
		h.jsonError(w, "orgId is required", http.StatusBadRequest)
		return
	}

	settings, err := h.pgRepo.ListExamProctoringSettingsByOrg(r.Context(), orgID)
	if err != nil {
		h.logger.Error("proctoring settings: list failed",
			zap.String("org_id", orgID),
			zap.Error(err),
		)
		h.jsonError(w, "Failed to list settings", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"orgId":    orgID,
		"count":    len(settings),
		"settings": settings,
	})
}

// handleRuleRegistry returns the complete default penalty rule set with
// toggle-to-rule mappings. This allows the frontend to display which
// rules each toggle controls.
func (h *ProctoringSettingsHandler) handleRuleRegistry(w http.ResponseWriter, r *http.Request) {
	type RuleInfo struct {
		EventType   string   `json:"eventType"`
		PenaltyPer  float64  `json:"penaltyPer"`
		MaxPenalty  float64  `json:"maxPenalty"`
		Description string   `json:"description"`
		Toggles     []string `json:"toggles"` // which toggles control this rule
	}

	// Build reverse map: eventType → []toggleName
	eventToToggles := make(map[string][]string)
	for toggle, events := range forensic.ToggleToRuleMap {
		for _, et := range events {
			eventToToggles[et] = append(eventToToggles[et], toggle)
		}
	}

	// Access default rules via a public helper.
	rules := forensic.GetDefaultPenaltyRules()
	var ruleInfos []RuleInfo
	for _, rule := range rules {
		ruleInfos = append(ruleInfos, RuleInfo{
			EventType:   rule.EventType,
			PenaltyPer:  rule.PenaltyPer,
			MaxPenalty:   rule.MaxPenalty,
			Description: rule.Description,
			Toggles:     eventToToggles[rule.EventType],
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"rules":                ruleInfos,
		"toggleMap":            forensic.ToggleToRuleMap,
		"defaultCleanThreshold": 80,
		"defaultWarningThreshold": 50,
		"defaults":             entity.DefaultExamProctoringSettings(),
	})
}

// ==========================================================================
// Auth + Helpers (follows existing handler pattern)
// ==========================================================================

func (h *ProctoringSettingsHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
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

func (h *ProctoringSettingsHandler) verifyToken(ctx context.Context, tokenStr string) (*entity.User, error) {
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

func (h *ProctoringSettingsHandler) jsonError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
