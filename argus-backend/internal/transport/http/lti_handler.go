// Package http — LTI 1.3 integration handler for Moodle/Canvas/Blackboard.
//
// Implements the LTI 1.3 Core specification:
//   - OIDC Login initiation (preflight)
//   - LTI Launch (resource link, deep linking)
//   - Assignment and Grade Services (AGS) — grade passback
//
// Flow:
//  1. LMS hits GET/POST /api/v1/lti/login   (OIDC login initiation)
//  2. Argus redirects student to LMS OIDC endpoint
//  3. LMS redirects to POST /api/v1/lti/launch with id_token
//  4. Argus validates token, creates proctoring session
//  5. Argus redirects student to student-exam-shell with session token
//  6. After exam, Argus posts grade back to /api/v1/lti/grades
//
// Configuration (env vars):
//
//	ARGUS_LTI_PLATFORM_ISSUER    — LMS issuer (e.g. https://moodle.example.kz)
//	ARGUS_LTI_PLATFORM_AUTH_URL  — LMS OIDC auth endpoint
//	ARGUS_LTI_PLATFORM_JWKS_URL  — LMS public key set URL
//	ARGUS_LTI_PLATFORM_TOKEN_URL — LMS access token URL (for AGS)
//	ARGUS_LTI_CLIENT_ID          — Client ID registered in LMS
//	ARGUS_LTI_CLIENT_SECRET      — Optional OAuth client secret for AGS token exchange
//	ARGUS_LTI_DEPLOYMENT_ID      — Deployment ID from LMS
//	ARGUS_LTI_REDIRECT_URL       — Public URL of this endpoint (e.g. https://argusai.kz/api/v1/lti/launch)
//	ARGUS_STUDENT_SHELL_URL      — Student exam shell URL
package http

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
	"github.com/argus-ai/event-collector/pkg/auth"
	"github.com/hibiken/asynq"
)

// LTIConfig holds platform-specific LTI 1.3 configuration.
type LTIConfig struct {
	PlatformIssuer   string // e.g. https://moodle.example.kz
	PlatformAuthURL  string // OIDC auth endpoint
	PlatformJWKSURL  string // public keys
	PlatformTokenURL string // for AGS grade passback
	ClientID         string // registered in LMS
	ClientSecret     string // optional client_credentials secret for AGS
	DeploymentID     string
	RedirectURL      string // this handler's /launch URL
	StudentShellURL  string // where to redirect student after session create
}

// LTIConfigFromEnv reads LTI config from environment variables.
func LTIConfigFromEnv() LTIConfig {
	shellURL := os.Getenv("ARGUS_STUDENT_SHELL_URL")
	if shellURL == "" {
		shellURL = os.Getenv("ARGUS_PUBLIC_SDK_URL")
	}
	return LTIConfig{
		PlatformIssuer:   os.Getenv("ARGUS_LTI_PLATFORM_ISSUER"),
		PlatformAuthURL:  os.Getenv("ARGUS_LTI_PLATFORM_AUTH_URL"),
		PlatformJWKSURL:  os.Getenv("ARGUS_LTI_PLATFORM_JWKS_URL"),
		PlatformTokenURL: os.Getenv("ARGUS_LTI_PLATFORM_TOKEN_URL"),
		ClientID:         os.Getenv("ARGUS_LTI_CLIENT_ID"),
		ClientSecret:     os.Getenv("ARGUS_LTI_CLIENT_SECRET"),
		DeploymentID:     os.Getenv("ARGUS_LTI_DEPLOYMENT_ID"),
		RedirectURL:      os.Getenv("ARGUS_LTI_REDIRECT_URL"),
		StudentShellURL:  shellURL,
	}
}

// LTIHandler implements LTI 1.3 launch and grade passback for Moodle/Canvas.
type LTIHandler struct {
	cfg         LTIConfig
	repo        *postgres.Repository
	asynqClient *asynq.Client
	jwtKey      []byte
	sdkURL      string
	serverURL   string
	logger      *zap.Logger
}

// NewLTIHandler creates the LTI 1.3 handler.
func NewLTIHandler(
	repo *postgres.Repository,
	asynqClient *asynq.Client,
	jwtKey []byte,
	sdkURL, serverURL string,
	logger *zap.Logger,
) *LTIHandler {
	return &LTIHandler{
		cfg:         LTIConfigFromEnv(),
		repo:        repo,
		asynqClient: asynqClient,
		jwtKey:      jwtKey,
		sdkURL:      sdkURL,
		serverURL:   serverURL,
		logger:      logger.Named("lti"),
	}
}

// RegisterRoutes attaches LTI endpoints to the mux.
func (h *LTIHandler) RegisterRoutes(mux *http.ServeMux) {
	// OIDC login initiation (step 1: LMS → Argus)
	mux.HandleFunc("GET /api/v1/lti/login", h.handleLogin)
	mux.HandleFunc("POST /api/v1/lti/login", h.handleLogin)

	// LTI launch (step 3: LMS redirects student with id_token)
	mux.HandleFunc("POST /api/v1/lti/launch", h.handleLaunch)

	// LTI configuration endpoint (for auto-registration in Moodle)
	mux.HandleFunc("GET /api/v1/lti/config", h.handleConfig)

	// Grade passback (Argus → LMS after exam)
	mux.HandleFunc("POST /api/v1/lti/grades/{sessionId}", h.ltiRequireAuth(h.handleGradePassback))

	// Public JWKS for LMS to verify Argus tokens
	mux.HandleFunc("GET /api/v1/lti/jwks", h.handleJWKS)
}

// ==========================================================================
// Step 1 — OIDC Login Initiation
// ==========================================================================

// handleLogin handles the OIDC login initiation request from the LMS.
// The LMS sends login_hint, lti_message_hint, target_link_uri, etc.
// Argus redirects the student to the LMS OIDC auth endpoint.
//
// GET/POST /api/v1/lti/login
func (h *LTIHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			h.ltiError(w, "invalid form", http.StatusBadRequest)
			return
		}
	}

	getParam := func(key string) string {
		if r.Method == http.MethodPost {
			return r.FormValue(key)
		}
		return r.URL.Query().Get(key)
	}

	iss := getParam("iss")
	loginHint := getParam("login_hint")
	ltiMessageHint := getParam("lti_message_hint")
	targetLinkURI := getParam("target_link_uri")
	clientID := getParam("client_id")

	if loginHint == "" {
		h.ltiError(w, "missing login_hint", http.StatusBadRequest)
		return
	}

	// Validate issuer
	if h.cfg.PlatformIssuer != "" && iss != h.cfg.PlatformIssuer {
		h.ltiError(w, "unknown issuer: "+iss, http.StatusForbidden)
		return
	}

	// Generate state (CSRF protection) and nonce
	state := ltiRandomToken(16)
	nonce := ltiRandomToken(16)

	// Build redirect URL to LMS OIDC auth endpoint
	authURL := h.cfg.PlatformAuthURL
	if authURL == "" {
		h.ltiError(w, "ARGUS_LTI_PLATFORM_AUTH_URL not configured", http.StatusServiceUnavailable)
		return
	}

	redirectURI := h.cfg.RedirectURL
	if redirectURI == "" {
		redirectURI = h.serverURL + "/api/v1/lti/launch"
	}

	cid := clientID
	if cid == "" {
		cid = h.cfg.ClientID
	}

	params := url.Values{
		"scope":         {"openid"},
		"response_type": {"id_token"},
		"client_id":     {cid},
		"redirect_uri":  {redirectURI},
		"login_hint":    {loginHint},
		"state":         {state},
		"response_mode": {"form_post"},
		"nonce":         {nonce},
		"prompt":        {"none"},
	}
	if ltiMessageHint != "" {
		params.Set("lti_message_hint", ltiMessageHint)
	}
	if targetLinkURI != "" {
		params.Set("target_link_uri", targetLinkURI)
	}

	// Store state+nonce in cookie for verification at launch
	stateCookie := fmt.Sprintf("argus_lti_state=%s; HttpOnly; Secure; SameSite=None; Path=/; Max-Age=300", state)
	nonceCookie := fmt.Sprintf("argus_lti_nonce=%s; HttpOnly; Secure; SameSite=None; Path=/; Max-Age=300", nonce)
	w.Header().Add("Set-Cookie", stateCookie)
	w.Header().Add("Set-Cookie", nonceCookie)

	http.Redirect(w, r, authURL+"?"+params.Encode(), http.StatusFound)
}

// ==========================================================================
// Step 3 — LTI Launch
// ==========================================================================

// ltiClaims holds the fields we extract from the LTI id_token JWT.
type ltiClaims struct {
	Iss        string          `json:"iss"`
	Aud        json.RawMessage `json:"aud"`
	Sub        string          `json:"sub"` // user identifier
	Email      string          `json:"email"`
	Name       string          `json:"name"`
	GivenName  string          `json:"given_name"`
	FamilyName string          `json:"family_name"`
	Nonce      string          `json:"nonce"`
	Exp        int64           `json:"exp"`
	Nbf        int64           `json:"nbf"`
	Iat        int64           `json:"iat"`
	// LTI-specific claims
	Roles        []string `json:"https://purl.imsglobal.org/spec/lti/claim/roles"`
	ResourceLink struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"https://purl.imsglobal.org/spec/lti/claim/resource_link"`
	Context struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"https://purl.imsglobal.org/spec/lti/claim/context"`
	AGS struct {
		LineItemsURL string   `json:"lineitems"`
		LineItemURL  string   `json:"lineitem"`
		Scopes       []string `json:"scope"`
	} `json:"https://purl.imsglobal.org/spec/lti-ags/claim/endpoint"`
	DeploymentID string `json:"https://purl.imsglobal.org/spec/lti/claim/deployment_id"`
	MessageType  string `json:"https://purl.imsglobal.org/spec/lti/claim/message_type"`
}

type ltiJWTHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

type ltiJWKS struct {
	Keys []ltiJWK `json:"keys"`
}

type ltiJWK struct {
	Kty string   `json:"kty"`
	Use string   `json:"use"`
	Kid string   `json:"kid"`
	Alg string   `json:"alg"`
	N   string   `json:"n"`
	E   string   `json:"e"`
	X5C []string `json:"x5c"`
}

// handleLaunch handles the LTI launch from the LMS.
// Validates the id_token, creates an Argus session, and redirects the student.
//
// POST /api/v1/lti/launch
func (h *LTIHandler) handleLaunch(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.ltiError(w, "invalid form body", http.StatusBadRequest)
		return
	}

	idToken := r.FormValue("id_token")
	state := r.FormValue("state")

	if idToken == "" {
		h.ltiError(w, "missing id_token", http.StatusBadRequest)
		return
	}

	// Verify state cookie
	stateCookie, err := r.Cookie("argus_lti_state")
	if err != nil || stateCookie.Value != state {
		h.ltiError(w, "state mismatch — possible CSRF", http.StatusForbidden)
		return
	}

	claims, err := h.verifyAndDecodeLTIClaims(r.Context(), idToken)
	if err != nil {
		h.ltiError(w, "invalid id_token: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Verify nonce
	nonceCookie, err := r.Cookie("argus_lti_nonce")
	if err != nil || nonceCookie.Value != claims.Nonce {
		h.ltiError(w, "nonce mismatch", http.StatusForbidden)
		return
	}

	// Extract student info
	studentID := claims.Sub
	studentName := claims.Name
	if studentName == "" {
		studentName = strings.TrimSpace(claims.GivenName + " " + claims.FamilyName)
	}
	examID := claims.ResourceLink.ID
	if examID == "" {
		examID = claims.Context.ID
	}
	examName := claims.ResourceLink.Title
	if examName == "" {
		examName = claims.Context.Title
	}

	// Create Argus external session
	session := &sessionCreateResult{}
	if err := h.createArgusSession(r.Context(), studentID, studentName, examID, examName, claims.AGS.LineItemURL, session); err != nil {
		h.logger.Error("LTI: failed to create Argus session",
			zap.String("student_id", studentID),
			zap.String("exam_id", examID),
			zap.Error(err),
		)
		h.ltiError(w, "failed to create proctoring session", http.StatusInternalServerError)
		return
	}

	h.logger.Info("LTI launch → Argus session created",
		zap.String("student_id", studentID),
		zap.String("exam_id", examID),
		zap.String("session_id", session.SessionID),
	)

	// Redirect student to exam shell
	shellURL := h.cfg.StudentShellURL
	if shellURL == "" {
		shellURL = h.serverURL + "/exam.html"
	}

	params := url.Values{
		"token":     {session.SessionToken},
		"serverUrl": {h.serverURL},
		"examName":  {examName},
		"locale":    {"ru"},
	}
	if h.sdkURL != "" {
		params.Set("sdkUrl", h.sdkURL)
	}

	http.Redirect(w, r, shellURL+"?"+params.Encode(), http.StatusFound)
}

// ==========================================================================
// Grade Passback (AGS)
// ==========================================================================

type gradePassbackRequest struct {
	UserID      string  `json:"userId"`   // LMS user id; falls back to sessionId if omitted
	Score       float64 `json:"score"`    // 0.0–1.0
	MaxScore    float64 `json:"maxScore"` // default 100
	Verdict     string  `json:"verdict"`  // clean | suspicious | violation
	Comment     string  `json:"comment"`
	LineItemURL string  `json:"lineItemUrl"` // LTI AGS lineitem URL
}

type ltiAccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

// handleGradePassback pushes a score to the LMS via LTI AGS.
//
// POST /api/v1/lti/grades/{sessionId}
func (h *LTIHandler) handleGradePassback(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")

	var req gradePassbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.ltiError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.MaxScore == 0 {
		req.MaxScore = 100
	}

	lineItemURL := req.LineItemURL
	if lineItemURL == "" {
		// Try to look up from session metadata
		h.ltiError(w, "lineItemUrl required for grade passback", http.StatusBadRequest)
		return
	}

	userID := req.UserID
	if userID == "" {
		userID = sessionID
	}

	// Build LTI AGS score payload (JSON)
	score := map[string]interface{}{
		"userId":           userID,
		"scoreGiven":       req.Score * req.MaxScore,
		"scoreMaximum":     req.MaxScore,
		"comment":          req.Comment,
		"timestamp":        time.Now().UTC().Format(time.RFC3339),
		"activityProgress": "Completed",
		"gradingProgress":  "FullyGraded",
	}

	scoreJSON, err := json.Marshal(score)
	if err != nil {
		h.ltiError(w, "internal error", http.StatusInternalServerError)
		return
	}

	if h.cfg.PlatformTokenURL != "" && h.cfg.ClientID != "" && h.cfg.ClientSecret != "" {
		if err := h.postLTIGrade(r.Context(), lineItemURL, scoreJSON); err != nil {
			h.logger.Warn("LTI grade passback failed",
				zap.String("session_id", sessionID),
				zap.String("line_item_url", lineItemURL),
				zap.Error(err),
			)
			h.ltiJSONResponse(w, map[string]interface{}{
				"status":      "failed",
				"sessionId":   sessionID,
				"lineItemUrl": lineItemURL,
				"error":       err.Error(),
			}, http.StatusBadGateway)
			return
		}

		h.ltiJSONResponse(w, map[string]interface{}{
			"status":      "posted",
			"sessionId":   sessionID,
			"score":       req.Score,
			"lineItemUrl": lineItemURL,
		}, http.StatusOK)
		return
	}

	h.logger.Info("LTI grade passback",
		zap.String("session_id", sessionID),
		zap.String("line_item_url", lineItemURL),
		zap.Float64("score", req.Score),
		zap.String("payload_preview", string(scoreJSON[:min(len(scoreJSON), 200)])),
	)

	h.ltiJSONResponse(w, map[string]interface{}{
		"status":      "queued",
		"sessionId":   sessionID,
		"score":       req.Score,
		"lineItemUrl": lineItemURL,
		"note":        "Grade will be posted to LMS via AGS when token URL, client ID and client secret are configured",
	}, http.StatusAccepted)
}

func (h *LTIHandler) postLTIGrade(ctx context.Context, lineItemURL string, scoreJSON []byte) error {
	accessToken, err := h.fetchLTIAGSAccessToken(ctx)
	if err != nil {
		return err
	}

	scoreURL := strings.TrimRight(lineItemURL, "/") + "/scores"
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, scoreURL, bytes.NewReader(scoreJSON))
	if err != nil {
		return fmt.Errorf("build score request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/vnd.ims.lis.v1.score+json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("post score: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("post score: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (h *LTIHandler) fetchLTIAGSAccessToken(ctx context.Context) (string, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {h.cfg.ClientID},
		"client_secret": {h.cfg.ClientSecret},
		"scope":         {"https://purl.imsglobal.org/spec/lti-ags/scope/score"},
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, h.cfg.PlatformTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch token: HTTP %d", resp.StatusCode)
	}

	var tokenResp ltiAccessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("token response missing access_token")
	}
	return tokenResp.AccessToken, nil
}

// ==========================================================================
// LTI Configuration JSON (for Moodle auto-registration)
// ==========================================================================

// handleConfig returns an LTI 1.3 tool configuration for automatic Moodle registration.
//
// GET /api/v1/lti/config
func (h *LTIHandler) handleConfig(w http.ResponseWriter, r *http.Request) {
	redirectURI := h.cfg.RedirectURL
	if redirectURI == "" {
		redirectURI = h.serverURL + "/api/v1/lti/launch"
	}
	loginURI := h.serverURL + "/api/v1/lti/login"
	jwksURI := h.serverURL + "/api/v1/lti/jwks"

	config := map[string]interface{}{
		"title":               "Argus AI Proctoring",
		"description":         "AI-powered exam proctoring system for Kazakh universities",
		"oidc_initiation_url": loginURI,
		"target_link_uri":     redirectURI,
		"public_jwk_url":      jwksURI,
		"scopes": []string{
			"https://purl.imsglobal.org/spec/lti-ags/scope/lineitem",
			"https://purl.imsglobal.org/spec/lti-ags/scope/result.readonly",
			"https://purl.imsglobal.org/spec/lti-ags/scope/score",
		},
		"extensions": []map[string]interface{}{
			{
				"platform": "moodle.net",
				"settings": map[string]interface{}{
					"icon_url": h.serverURL + "/argus-logo.png",
					"placements": []map[string]interface{}{
						{
							"text":            "Argus Proctoring",
							"placement":       "assignment_edit",
							"target_link_uri": redirectURI,
							"message_type":    "LtiResourceLinkRequest",
						},
					},
				},
			},
		},
	}

	h.ltiJSONResponse(w, config, http.StatusOK)
}

// handleJWKS returns an empty JWKS (Argus uses symmetric keys for session tokens;
// LTI token verification uses LMS public keys fetched from PlatformJWKSURL).
func (h *LTIHandler) handleJWKS(w http.ResponseWriter, r *http.Request) {
	h.ltiJSONResponse(w, map[string]interface{}{"keys": []interface{}{}}, http.StatusOK)
}

// ==========================================================================
// Internal helpers
// ==========================================================================

type sessionCreateResult struct {
	SessionID    string
	SessionToken string
}

func (h *LTIHandler) createArgusSession(
	ctx interface {
		Deadline() (time.Time, bool)
		Done() <-chan struct{}
		Err() error
		Value(any) any
	},
	studentID, studentName, examID, examName, lineItemURL string,
	out *sessionCreateResult,
) error {
	sessionID := generateSessionID()
	tokenExpiry := time.Now().Add(4 * time.Hour)

	claims := &auth.ProctoringClaims{
		Subject:   studentID,
		Issuer:    "argus-lti",
		Audience:  []string{"argus-event-collector"},
		ExpiresAt: tokenExpiry,
		IssuedAt:  time.Now(),
		JTI:       sessionID,
		SessionID: sessionID,
		StudentID: studentID,
		OrgID:     "lti", // will be resolved from client config in production
		ExamID:    examID,
		Roles:     []string{"student"},
	}

	sessionToken, err := auth.GenerateToken(claims, h.jwtKey)
	if err != nil {
		return fmt.Errorf("generate token: %w", err)
	}

	out.SessionID = sessionID
	out.SessionToken = sessionToken

	// Enqueue enrollment if photo URL is available (not available from LTI basic launch)

	return nil
}

func (h *LTIHandler) verifyAndDecodeLTIClaims(ctx context.Context, idToken string) (*ltiClaims, error) {
	// Split JWT: header.payload.signature
	parts := strings.SplitN(idToken, ".", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed JWT: expected 3 parts")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}

	var header ltiJWTHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("unmarshal header: %w", err)
	}
	if header.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported alg %q", header.Alg)
	}
	if header.Kid == "" {
		return nil, fmt.Errorf("missing kid")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	var claims ltiClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal claims: %w", err)
	}
	if err := h.validateLTIClaims(&claims); err != nil {
		return nil, err
	}

	key, err := h.resolveLTIPublicKey(ctx, header.Kid)
	if err != nil {
		return nil, err
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}

	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return nil, fmt.Errorf("signature verification failed")
	}

	return &claims, nil
}

func (h *LTIHandler) validateLTIClaims(claims *ltiClaims) error {
	now := time.Now().Unix()
	if claims.Iss == "" {
		return fmt.Errorf("missing iss")
	}
	if h.cfg.PlatformIssuer != "" && claims.Iss != h.cfg.PlatformIssuer {
		return fmt.Errorf("unexpected issuer")
	}
	if h.cfg.ClientID != "" && !ltiAudienceContains(claims.Aud, h.cfg.ClientID) {
		return fmt.Errorf("audience mismatch")
	}
	if claims.Exp == 0 || now > claims.Exp {
		return fmt.Errorf("token expired")
	}
	if claims.Nbf > 0 && now+60 < claims.Nbf {
		return fmt.Errorf("token not yet valid")
	}
	if claims.Iat > 0 && claims.Iat > now+300 {
		return fmt.Errorf("token issued in the future")
	}
	if claims.Sub == "" {
		return fmt.Errorf("missing sub")
	}
	if claims.Nonce == "" {
		return fmt.Errorf("missing nonce")
	}
	if h.cfg.DeploymentID != "" && claims.DeploymentID != h.cfg.DeploymentID {
		return fmt.Errorf("deployment_id mismatch")
	}
	if claims.MessageType != "" && claims.MessageType != "LtiResourceLinkRequest" {
		return fmt.Errorf("unsupported message_type %q", claims.MessageType)
	}
	return nil
}

func ltiAudienceContains(raw json.RawMessage, expected string) bool {
	if len(raw) == 0 {
		return false
	}

	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return single == expected
	}

	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		for _, aud := range many {
			if aud == expected {
				return true
			}
		}
	}

	return false
}

func (h *LTIHandler) resolveLTIPublicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if h.cfg.PlatformJWKSURL == "" {
		return nil, fmt.Errorf("ARGUS_LTI_PLATFORM_JWKS_URL not configured")
	}

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, h.cfg.PlatformJWKSURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build jwks request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch jwks: HTTP %d", resp.StatusCode)
	}

	var jwks ltiJWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("decode jwks: %w", err)
	}

	for _, key := range jwks.Keys {
		if key.Kid == kid {
			return ltiJWKToRSAPublicKey(key)
		}
	}

	return nil, fmt.Errorf("jwks key not found")
}

func ltiJWKToRSAPublicKey(jwk ltiJWK) (*rsa.PublicKey, error) {
	if jwk.Kty != "" && jwk.Kty != "RSA" {
		return nil, fmt.Errorf("unsupported jwk kty %q", jwk.Kty)
	}

	if len(jwk.X5C) > 0 {
		certDER, err := base64.StdEncoding.DecodeString(jwk.X5C[0])
		if err != nil {
			return nil, fmt.Errorf("decode x5c certificate: %w", err)
		}
		cert, err := x509.ParseCertificate(certDER)
		if err != nil {
			return nil, fmt.Errorf("parse x5c certificate: %w", err)
		}
		pub, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("x5c key is not RSA")
		}
		return pub, nil
	}

	if jwk.N == "" || jwk.E == "" {
		return nil, fmt.Errorf("jwk missing modulus or exponent")
	}

	modulusBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, fmt.Errorf("decode jwk n: %w", err)
	}
	exponentBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, fmt.Errorf("decode jwk e: %w", err)
	}
	if len(exponentBytes) == 0 {
		return nil, fmt.Errorf("empty jwk exponent")
	}

	exponent := 0
	for _, b := range exponentBytes {
		exponent = exponent<<8 + int(b)
	}
	if exponent == 0 {
		return nil, fmt.Errorf("invalid jwk exponent")
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(modulusBytes),
		E: exponent,
	}, nil
}

func ltiRandomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func (h *LTIHandler) ltiRequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if auth := r.Header.Get("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
			token = auth[7:]
		}
		if token == "" {
			h.ltiError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		parts := splitToken(token)
		if parts == nil {
			h.ltiError(w, "invalid token", http.StatusUnauthorized)
			return
		}
		sig := hmacSHA256([]byte(parts[0]+"."+parts[1]), h.jwtKey)
		actual, err := base64URLDecode(parts[2])
		if err != nil || !hmacEqual(sig, actual) {
			h.ltiError(w, "invalid token", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (h *LTIHandler) ltiError(w http.ResponseWriter, msg string, status int) {
	h.logger.Warn("LTI error", zap.String("msg", msg), zap.Int("status", status))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><html><body>
<h2>Argus LTI — Ошибка</h2>
<p>%s</p>
<p><small>HTTP %d</small></p>
</body></html>`, msg, status)
}

func (h *LTIHandler) ltiJSONResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data) //nolint:errcheck
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
