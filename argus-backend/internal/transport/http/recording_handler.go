package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"go.uber.org/zap"
)

type EgressController interface {
	StartRecording(ctx context.Context, roomName, videoTrackID, audioTrackID, userID, sessionID string) (string, error)
	StopRecording(ctx context.Context, egressID string) error
}

type RecordingStore interface {
	CreateRecording(ctx context.Context, rec *entity.Recording) error
	UpdateRecordingStopped(ctx context.Context, egressID string) error
}

type RecordingHandler struct {
	egress        EgressController
	store         RecordingStore
	logger        *zap.Logger
	jwtSigningKey []byte
	repo          adminRepo
}

func NewRecordingHandler(egress EgressController, store RecordingStore, logger *zap.Logger, jwtSigningKey []byte, repo adminRepo) *RecordingHandler {
	return &RecordingHandler{
		egress:        egress,
		store:         store,
		logger:        logger.Named("recording_api"),
		jwtSigningKey: jwtSigningKey,
		repo:          repo,
	}
}

func (h *RecordingHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/media/recordings/start", h.requireAuth(h.handleStartRecording))
	mux.HandleFunc("POST /api/v1/media/recordings/stop", h.requireAuth(h.handleStopRecording))
}

type startRecordingRequest struct {
	SessionID    string `json:"sessionId"`
	StudentID    string `json:"studentId"`
	RoomName     string `json:"roomName"`
	VideoTrackID string `json:"videoTrackId"`
	AudioTrackID string `json:"audioTrackId"`
}

type stopRecordingRequest struct {
	EgressID string `json:"egressId"`
}

func (h *RecordingHandler) handleStartRecording(w http.ResponseWriter, r *http.Request) {
	var req startRecordingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.SessionID == "" || req.StudentID == "" || req.VideoTrackID == "" {
		h.jsonError(w, "sessionId, studentId and videoTrackId are required", http.StatusBadRequest)
		return
	}
	if req.RoomName == "" {
		req.RoomName = fmt.Sprintf("argus-session-%s", req.SessionID)
	}

	egressID, err := h.egress.StartRecording(r.Context(), req.RoomName, req.VideoTrackID, req.AudioTrackID, req.StudentID, req.SessionID)
	if err != nil {
		h.logger.Error("failed to start livekit recording", zap.Error(err))
		h.jsonError(w, "failed to start recording", http.StatusBadGateway)
		return
	}

	rec := &entity.Recording{
		EgressID:     egressID,
		SessionID:    req.SessionID,
		UserID:       req.StudentID,
		RoomName:     req.RoomName,
		VideoTrackID: req.VideoTrackID,
		AudioTrackID: req.AudioTrackID,
		Status:       "EGRESS_ACTIVE",
	}
	if err := h.store.CreateRecording(r.Context(), rec); err != nil {
		h.logger.Error("failed to persist livekit recording", zap.String("egress_id", egressID), zap.Error(err))
		if stopErr := h.egress.StopRecording(context.Background(), egressID); stopErr != nil {
			h.logger.Error("failed to stop orphaned livekit recording", zap.String("egress_id", egressID), zap.Error(stopErr))
		}
		h.jsonError(w, "failed to persist recording", http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, http.StatusAccepted, map[string]string{
		"egressId": egressID,
		"status":   rec.Status,
	})
}

func (h *RecordingHandler) handleStopRecording(w http.ResponseWriter, r *http.Request) {
	var req stopRecordingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.EgressID == "" {
		h.jsonError(w, "egressId is required", http.StatusBadRequest)
		return
	}
	if err := h.egress.StopRecording(r.Context(), req.EgressID); err != nil {
		h.logger.Error("failed to stop livekit recording", zap.String("egress_id", req.EgressID), zap.Error(err))
		h.jsonError(w, "failed to stop recording", http.StatusBadGateway)
		return
	}
	if err := h.store.UpdateRecordingStopped(r.Context(), req.EgressID); err != nil {
		h.logger.Error("failed to mark recording stopped", zap.String("egress_id", req.EgressID), zap.Error(err))
		h.jsonError(w, "failed to update recording", http.StatusInternalServerError)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]string{"egressId": req.EgressID, "status": "EGRESS_ENDING"})
}

func (h *RecordingHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
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

func (h *RecordingHandler) verifyToken(ctx context.Context, tokenStr string) (*entity.User, error) {
	if len(h.jwtSigningKey) == 0 || h.repo == nil {
		return nil, errInvalidToken
	}
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

	payloadJSON, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, errInvalidToken
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, errInvalidToken
	}

	exp, ok := claims["exp"].(float64)
	if !ok {
		return nil, errInvalidToken
	}
	if time.Unix(int64(exp), 0).Before(time.Now()) {
		return nil, errTokenExpired
	}
	userID, _ := claims["sub"].(string)
	if userID == "" {
		return nil, errInvalidToken
	}

	user, err := h.repo.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return nil, errUserNotFound
	}
	return user, nil
}

func (h *RecordingHandler) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *RecordingHandler) jsonError(w http.ResponseWriter, msg string, status int) {
	h.writeJSON(w, status, map[string]string{"error": msg})
}
