package http

import (
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// AgentHandler receives heartbeats from the Argus Desktop Agent.
//
// Endpoints:
//
//	POST /api/v1/agent/heartbeat — desktop agent status report (session token auth)
//	GET  /api/v1/agent/config   — fetch forbidden process policy (session token auth)
type AgentHandler struct {
	logger        *zap.Logger
	jwtSigningKey []byte
}

// NewAgentHandler creates the agent REST handler.
func NewAgentHandler(logger *zap.Logger, jwtSigningKey []byte) *AgentHandler {
	return &AgentHandler{
		logger:        logger.Named("agent_api"),
		jwtSigningKey: jwtSigningKey,
	}
}

// RegisterRoutes attaches agent endpoints to the mux.
func (h *AgentHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/agent/heartbeat", h.requireSessionJWT(h.handleHeartbeat))
	mux.HandleFunc("GET /api/v1/agent/config", h.requireSessionJWT(h.handleGetConfig))
}

// ---------------------------------------------------------------------------
// Heartbeat
// ---------------------------------------------------------------------------

type agentHeartbeatRequest struct {
	SessionToken   string   `json:"sessionToken"`
	AgentVersion   string   `json:"agentVersion"`
	OS             string   `json:"os"`
	Hostname       string   `json:"hostname"`
	MachineID      string   `json:"machineId"`
	MonitorCount   int      `json:"monitorCount"`
	IsVM           bool     `json:"isVm"`
	VMProduct      string   `json:"vmProduct,omitempty"`
	ProcessCount   int      `json:"processCount"`
	ForbiddenProcs []string `json:"forbiddenProcs,omitempty"`
	Timestamp      string   `json:"timestamp"`
}

func (h *AgentHandler) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req agentHeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	h.logger.Info("agent heartbeat",
		zap.String("os", req.OS),
		zap.String("host", req.Hostname),
		zap.String("machine_id", req.MachineID),
		zap.Int("monitors", req.MonitorCount),
		zap.Bool("is_vm", req.IsVM),
		zap.String("vm_product", req.VMProduct),
		zap.Int("processes", req.ProcessCount),
		zap.Strings("forbidden", req.ForbiddenProcs),
	)

	h.jsonResponse(w, map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		// Future: return updated config, policy overrides, or session status
	}, http.StatusOK)
}

// ---------------------------------------------------------------------------
// Config (policy for what processes to scan)
// ---------------------------------------------------------------------------

func (h *AgentHandler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	// For now, return the default forbidden process list.
	// Future: load per-exam policy from proctoring_settings.
	h.jsonResponse(w, map[string]interface{}{
		"forbiddenProcesses": defaultForbiddenProcessNames(),
		"scanIntervalSec":    30,
		"reportMultiMonitor": true,
		"reportVM":           true,
	}, http.StatusOK)
}

func defaultForbiddenProcessNames() []string {
	return []string{
		"anydesk", "teamviewer", "radmin", "vnc", "rustdesk", "parsec",
		"obs", "bandicam", "fraps", "xsplit", "shadowplay",
		"discord", "zoom", "teams", "skype", "webex",
		"manycam", "splitcam",
	}
}

// ---------------------------------------------------------------------------
// Auth middleware — validates Authorization: Bearer <argusSessionToken>
// Uses the same JWT verification pattern as ExternalHandler.requireSessionToken
// ---------------------------------------------------------------------------

func (h *AgentHandler) requireSessionJWT(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if auth := r.Header.Get("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
			token = auth[7:]
		}
		if token == "" {
			h.jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Validate the JWT signature using the same key as the session tokens.
		parts := splitToken(token)
		if parts == nil {
			h.jsonError(w, "invalid token", http.StatusUnauthorized)
			return
		}
		sig := hmacSHA256([]byte(parts[0]+"."+parts[1]), h.jwtSigningKey)
		actual, err := base64URLDecode(parts[2])
		if err != nil || !hmacEqual(sig, actual) {
			h.jsonError(w, "invalid token", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

func (h *AgentHandler) jsonResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data) //nolint:errcheck
}

func (h *AgentHandler) jsonError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck
}
