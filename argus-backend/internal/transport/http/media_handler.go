// =============================================================================
// Argus AI — WebRTC Media REST API Handler (LiveKit Integration)
// =============================================================================
//
// Provides HTTP endpoints for WebRTC video streaming via LiveKit. Generates
// participant tokens for students (publishers) and proctors (subscribers),
// and lists active media rooms.
//
// Endpoints:
//   POST /api/v1/media/token — Generate LiveKit participant token
//   GET  /api/v1/media/rooms — LiveKit connection info
// =============================================================================
package http

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
	"go.uber.org/zap"
)

// MediaHandler serves the WebRTC media REST API for LiveKit integration.
type MediaHandler struct {
	pgRepo *postgres.Repository
	logger *zap.Logger

	jwtSigningKey []byte
	repo          adminRepo // for JWT verification (GetUserByID)

	livekitAPIKey    string
	livekitAPISecret string
	livekitWSURL     string
	livekitPublicURL string // public URL returned to browser (may differ from internal WS URL)
}

// NewMediaHandler creates a new media API handler with LiveKit configuration.
func NewMediaHandler(
	pgRepo *postgres.Repository,
	logger *zap.Logger,
	jwtSigningKey []byte,
) *MediaHandler {
	apiKey := os.Getenv("LIVEKIT_API_KEY")
	if apiKey == "" {
		apiKey = "argus-dev-api-key"
	}
	apiSecret := os.Getenv("LIVEKIT_API_SECRET")
	if apiSecret == "" {
		apiSecret = "argus-dev-api-secret-must-be-at-least-32-characters-long"
	}
	wsURL := os.Getenv("LIVEKIT_WS_URL")
	if wsURL == "" {
		wsURL = "ws://localhost:7880"
	}
	// LIVEKIT_PUBLIC_WS_URL is the URL browsers use to connect to LiveKit.
	// Defaults to LIVEKIT_WS_URL. Override when running in Docker where the
	// internal hostname (e.g. "livekit") is not resolvable by the browser.
	publicURL := os.Getenv("LIVEKIT_PUBLIC_WS_URL")
	if publicURL == "" {
		publicURL = wsURL
	}

	return &MediaHandler{
		pgRepo:           pgRepo,
		logger:           logger.Named("media_api"),
		jwtSigningKey:    jwtSigningKey,
		repo:             pgRepo,
		livekitAPIKey:    apiKey,
		livekitAPISecret: apiSecret,
		livekitWSURL:     wsURL,
		livekitPublicURL: publicURL,
	}
}

// RegisterRoutes registers media API endpoints on the given mux.
func (h *MediaHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/media/token", h.requireAuth(h.handleGenerateToken))
	mux.HandleFunc("GET /api/v1/media/rooms", h.requireAuth(h.handleListRooms))
}

// ==========================================================================
// Request / Response Types
// ==========================================================================

// MediaTokenRequest is the request body for generating a LiveKit token.
type MediaTokenRequest struct {
	SessionID string `json:"sessionId"`
	StudentID string `json:"studentId"`
	ExamID    string `json:"examId"`
	// Role: "student" (can publish) or "proctor" (can subscribe only)
	Role string `json:"role"`
}

// MediaTokenResponse is the response containing the LiveKit access token.
type MediaTokenResponse struct {
	Token string `json:"token"`
	WsURL string `json:"wsUrl"`
	Room  string `json:"room"`
}

// ==========================================================================
// LiveKit JWT Token Generation (Pure Go — no external SDK dependency)
// ==========================================================================

// livekitVideoGrant defines the video grant permissions for a LiveKit token.
type livekitVideoGrant struct {
	RoomJoin       bool   `json:"roomJoin"`
	Room           string `json:"room"`
	CanPublish     *bool  `json:"canPublish,omitempty"`
	CanSubscribe   *bool  `json:"canSubscribe,omitempty"`
	CanPublishData *bool  `json:"canPublishData,omitempty"`
}

// livekitTokenClaims is the JWT claims structure for LiveKit access tokens.
type livekitTokenClaims struct {
	Exp      int64             `json:"exp"`
	Iss      string            `json:"iss"`
	Nbf      int64             `json:"nbf"`
	Sub      string            `json:"sub"`
	Name     string            `json:"name,omitempty"`
	Video    livekitVideoGrant `json:"video"`
	Metadata string            `json:"metadata,omitempty"`
}

// generateLiveKitToken creates a signed JWT access token for LiveKit.
// This implements LiveKit's token format without requiring the external SDK.
func (h *MediaHandler) generateLiveKitToken(identity, name, room string, canPublish, canSubscribe bool) (string, error) {
	now := time.Now()
	boolTrue := true
	boolFalse := false

	var pubPtr, subPtr *bool
	if canPublish {
		pubPtr = &boolTrue
	} else {
		pubPtr = &boolFalse
	}
	if canSubscribe {
		subPtr = &boolTrue
	} else {
		subPtr = &boolFalse
	}

	claims := livekitTokenClaims{
		Exp:  now.Add(24 * time.Hour).Unix(),
		Iss:  h.livekitAPIKey,
		Nbf:  now.Unix(),
		Sub:  identity,
		Name: name,
		Video: livekitVideoGrant{
			RoomJoin:       true,
			Room:           room,
			CanPublish:     pubPtr,
			CanSubscribe:   subPtr,
			CanPublishData: &boolTrue,
		},
	}

	// JWT header
	header := `{"alg":"HS256","typ":"JWT"}`
	headerB64 := lkBase64Encode([]byte(header))

	// JWT payload
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	payloadB64 := lkBase64Encode(payloadJSON)

	// HMAC-SHA256 signature
	signingInput := headerB64 + "." + payloadB64
	mac := hmac.New(sha256.New, []byte(h.livekitAPISecret))
	mac.Write([]byte(signingInput))
	signatureB64 := lkBase64Encode(mac.Sum(nil))

	return signingInput + "." + signatureB64, nil
}

// lkBase64Encode encodes bytes to base64url (no padding) for LiveKit JWT.
// Named differently to avoid conflict with the shared base64URLEncode in admin_handler.go.
func lkBase64Encode(data []byte) string {
	const encodeURL = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	buf := make([]byte, 0, (len(data)*4+2)/3)

	for i := 0; i < len(data); i += 3 {
		var b0, b1, b2 byte
		b0 = data[i]
		if i+1 < len(data) {
			b1 = data[i+1]
		}
		if i+2 < len(data) {
			b2 = data[i+2]
		}

		buf = append(buf, encodeURL[(b0>>2)&0x3F])
		buf = append(buf, encodeURL[((b0<<4)|(b1>>4))&0x3F])

		if i+1 < len(data) {
			buf = append(buf, encodeURL[((b1<<2)|(b2>>6))&0x3F])
		}
		if i+2 < len(data) {
			buf = append(buf, encodeURL[b2&0x3F])
		}
	}
	return string(buf)
}

// ==========================================================================
// Handlers
// ==========================================================================

// handleGenerateToken generates a LiveKit participant token.
//
//	POST /api/v1/media/token
//	Body: { "sessionId": "...", "studentId": "...", "examId": "...", "role": "student"|"proctor" }
func (h *MediaHandler) handleGenerateToken(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())

	var req MediaTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" {
		h.jsonError(w, "sessionId is required", http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = "proctor"
	}
	if req.Role != "student" && req.Role != "proctor" {
		h.jsonError(w, "role must be 'student' or 'proctor'", http.StatusBadRequest)
		return
	}

	// Room name = exam session
	roomName := fmt.Sprintf("argus-session-%s", req.SessionID)

	// Identity and permissions based on role
	var identity, displayName string
	var canPublish, canSubscribe bool

	switch req.Role {
	case "student":
		identity = fmt.Sprintf("student-%s", req.StudentID)
		displayName = req.StudentID
		canPublish = true
		canSubscribe = false
	case "proctor":
		userID := "admin"
		if user != nil {
			userID = user.ID
		}
		identity = fmt.Sprintf("proctor-%s", userID)
		displayName = "Проктор"
		canPublish = false
		canSubscribe = true
	}

	token, err := h.generateLiveKitToken(identity, displayName, roomName, canPublish, canSubscribe)
	if err != nil {
		h.logger.Error("failed to generate livekit token", zap.Error(err))
		h.jsonError(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	h.logger.Info("livekit token generated",
		zap.String("room", roomName),
		zap.String("identity", identity),
		zap.String("role", req.Role),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(MediaTokenResponse{
		Token: token,
		WsURL: h.livekitPublicURL,
		Room:  roomName,
	})
}

// handleListRooms returns LiveKit connection info.
//
//	GET /api/v1/media/rooms
func (h *MediaHandler) handleListRooms(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"livekitUrl":  h.livekitPublicURL,
		"apiKey":      h.livekitAPIKey,
		"status":      "available",
		"description": "Use POST /api/v1/media/token to generate participant tokens",
	})
}

// ==========================================================================
// Auth & Helpers (same pattern as MonitoringHandler)
// ==========================================================================

func (h *MediaHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
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

func (h *MediaHandler) verifyToken(ctx context.Context, tokenStr string) (*entity.User, error) {
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
	if !ok || time.Unix(int64(exp), 0).Before(time.Now()) {
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

func (h *MediaHandler) jsonError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
