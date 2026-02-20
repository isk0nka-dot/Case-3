// =============================================================================
// Argus AI — Secondary Camera (Mobile) Orchestration REST API Handler
// =============================================================================
//
// Provides HTTP endpoints for the full secondary camera lifecycle:
//   - QR-based pairing initiation & completion
//   - Spatial calibration verification (Golden Angle 45-60°)
//   - Session gating (mandatory/optional/disabled)
//   - Device telemetry processing (battery, thermal, displacement)
//   - Stream health heartbeat
//   - Hands-on-desk anomaly reporting
//   - Pairing status query & cleanup
//
// Endpoints:
//   POST /api/v1/sidecam/pair/initiate           — Generate QR pairing payload
//   POST /api/v1/sidecam/pair/complete            — Mobile device completes pairing
//   POST /api/v1/sidecam/validate-start           — Gate exam start (mandatory mode)
//   POST /api/v1/sidecam/calibrate                — Process calibration frame
//   POST /api/v1/sidecam/telemetry                — Device health telemetry
//   POST /api/v1/sidecam/heartbeat                — Stream health heartbeat
//   POST /api/v1/sidecam/hands                    — Hands-on-desk detection
//   GET  /api/v1/sidecam/session/{sessionId}      — Get pairing status
//   DELETE /api/v1/sidecam/session/{sessionId}     — Cleanup pairing session
// =============================================================================
package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/sidecam"
)

// SidecamHandler serves the secondary camera orchestration REST API.
type SidecamHandler struct {
	orchestrator *sidecam.Orchestrator
	logger       *zap.Logger

	jwtSigningKey []byte
	repo          adminRepo
}

// NewSidecamHandler creates a new secondary camera API handler.
func NewSidecamHandler(
	orchestrator *sidecam.Orchestrator,
	repo adminRepo,
	logger *zap.Logger,
	jwtSigningKey []byte,
) *SidecamHandler {
	return &SidecamHandler{
		orchestrator:  orchestrator,
		logger:        logger.Named("sidecam_api"),
		jwtSigningKey: jwtSigningKey,
		repo:          repo,
	}
}

// RegisterRoutes registers secondary camera API endpoints on the given mux.
func (h *SidecamHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/sidecam/pair/initiate", h.requireAuth(h.handleInitiatePairing))
	mux.HandleFunc("POST /api/v1/sidecam/pair/complete", h.handleCompletePairing) // No auth — mobile device uses pairing token
	mux.HandleFunc("POST /api/v1/sidecam/validate-start", h.requireAuth(h.handleValidateStart))
	mux.HandleFunc("POST /api/v1/sidecam/calibrate", h.handleCalibrate) // Token-based auth via pairing
	mux.HandleFunc("POST /api/v1/sidecam/telemetry", h.handleTelemetry)
	mux.HandleFunc("POST /api/v1/sidecam/heartbeat", h.handleHeartbeat)
	mux.HandleFunc("POST /api/v1/sidecam/hands", h.handleHandsDetection)
	mux.HandleFunc("GET /api/v1/sidecam/session/{sessionId}", h.requireAuth(h.handleGetSession))
	mux.HandleFunc("DELETE /api/v1/sidecam/session/{sessionId}", h.requireAuth(h.handleCleanupSession))
}

// ==========================================================================
// Request/Response Types
// ==========================================================================

type initiatePairingRequest struct {
	SessionID string              `json:"sessionId"`
	StudentID string              `json:"studentId"`
	ExamID    string              `json:"examId"`
	OrgID     string              `json:"orgId"`
	Policy    sidecam.CameraPolicy `json:"policy"`
}

type initiatePairingResponse struct {
	Session  *sidecam.PairingSession     `json:"session"`
	QRData   string                      `json:"qrData"`
	Payload  *sidecam.PairingCodePayload `json:"payload"`
}

type completePairingRequest struct {
	SessionID    string                   `json:"sessionId"`
	PairingToken string                   `json:"pairingToken"`
	Device       *sidecam.MobileDeviceInfo `json:"device"`
}

type validateStartRequest struct {
	SessionID string              `json:"sessionId"`
	Policy    sidecam.CameraPolicy `json:"policy"`
}

type calibrateRequest struct {
	SessionID string                   `json:"sessionId"`
	Frame     *sidecam.CalibrationFrame `json:"frame"`
}

type telemetryRequest struct {
	SessionID string                   `json:"sessionId"`
	Device    *sidecam.MobileDeviceInfo `json:"device"`
}

type heartbeatRequest struct {
	SessionID     string `json:"sessionId"`
	ClientTs      string `json:"clientTs"`
	FrameRate     int    `json:"frameRate"`
	DroppedFrames int    `json:"droppedFrames"`
}

type handsDetectionRequest struct {
	SessionID    string `json:"sessionId"`
	HandsVisible bool   `json:"handsVisible"`
}

// ==========================================================================
// Handlers
// ==========================================================================

// handleInitiatePairing generates a QR pairing payload for the mobile device.
func (h *SidecamHandler) handleInitiatePairing(w http.ResponseWriter, r *http.Request) {
	var req initiatePairingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" || req.ExamID == "" {
		h.jsonError(w, "sessionId and examId are required", http.StatusBadRequest)
		return
	}

	if req.Policy == "" {
		req.Policy = sidecam.PolicyOptional
	}

	session, payload, err := h.orchestrator.InitiatePairing(
		req.SessionID, req.StudentID, req.ExamID, req.OrgID, req.Policy,
	)
	if err != nil {
		h.logger.Error("sidecam: initiate pairing failed",
			zap.String("session_id", req.SessionID),
			zap.Error(err),
		)
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	qrData, err := sidecam.MarshalQRPayload(payload)
	if err != nil {
		h.jsonError(w, "Failed to encode QR payload", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(initiatePairingResponse{
		Session: session,
		QRData:  qrData,
		Payload: payload,
	})
}

// handleCompletePairing validates the mobile device's pairing attempt.
// No JWT auth — the mobile device authenticates via the pairing token.
func (h *SidecamHandler) handleCompletePairing(w http.ResponseWriter, r *http.Request) {
	var req completePairingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" || req.PairingToken == "" {
		h.jsonError(w, "sessionId and pairingToken are required", http.StatusBadRequest)
		return
	}

	if err := h.orchestrator.CompletePairing(req.SessionID, req.PairingToken, req.Device); err != nil {
		h.logger.Error("sidecam: complete pairing failed",
			zap.String("session_id", req.SessionID),
			zap.Error(err),
		)
		status := http.StatusBadRequest
		if err.Error() == "sidecam: invalid pairing token" {
			status = http.StatusUnauthorized
		}
		h.jsonError(w, err.Error(), status)
		return
	}

	// Return updated session state
	session, _ := h.orchestrator.GetPairingSession(req.SessionID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "paired",
		"session": session,
	})
}

// handleValidateStart checks whether an exam session can start given the camera policy.
func (h *SidecamHandler) handleValidateStart(w http.ResponseWriter, r *http.Request) {
	var req validateStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" {
		h.jsonError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	if req.Policy == "" {
		req.Policy = sidecam.PolicyOptional
	}

	if err := h.orchestrator.ValidateSessionStart(req.SessionID, req.Policy); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"allowed": false,
			"reason":  err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"allowed": true,
	})
}

// handleCalibrate processes a spatial calibration frame from the mobile device.
func (h *SidecamHandler) handleCalibrate(w http.ResponseWriter, r *http.Request) {
	var req calibrateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" || req.Frame == nil {
		h.jsonError(w, "sessionId and frame are required", http.StatusBadRequest)
		return
	}

	status, err := h.orchestrator.ProcessCalibrationFrame(req.SessionID, req.Frame)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}

// handleTelemetry processes device health telemetry (battery, thermal, accel).
func (h *SidecamHandler) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	var req telemetryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" || req.Device == nil {
		h.jsonError(w, "sessionId and device are required", http.StatusBadRequest)
		return
	}

	directive, err := h.orchestrator.ProcessDeviceTelemetry(req.SessionID, req.Device)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(directive)
}

// handleHeartbeat processes a stream health heartbeat from the mobile device.
func (h *SidecamHandler) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req heartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" {
		h.jsonError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	clientTs, err := time.Parse(time.RFC3339, req.ClientTs)
	if err != nil {
		clientTs = time.Now() // Fallback to server time if client timestamp is invalid
	}

	health, err := h.orchestrator.ProcessHeartbeat(req.SessionID, clientTs, req.FrameRate, req.DroppedFrames)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(health)
}

// handleHandsDetection processes a hands-on-desk detection report.
func (h *SidecamHandler) handleHandsDetection(w http.ResponseWriter, r *http.Request) {
	var req handsDetectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" {
		h.jsonError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	h.orchestrator.ProcessHandsDetection(req.SessionID, req.HandsVisible)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleGetSession returns the current pairing state for a session.
func (h *SidecamHandler) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.jsonError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	session, err := h.orchestrator.GetPairingSession(sessionID)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(session)
}

// handleCleanupSession removes a pairing session.
func (h *SidecamHandler) handleCleanupSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.jsonError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	h.orchestrator.CleanupSession(sessionID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "cleaned"})
}

// ==========================================================================
// Auth + Helpers (follows existing handler pattern)
// ==========================================================================

func (h *SidecamHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
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

func (h *SidecamHandler) verifyToken(ctx context.Context, tokenStr string) (*entity.User, error) {
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

func (h *SidecamHandler) jsonError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
