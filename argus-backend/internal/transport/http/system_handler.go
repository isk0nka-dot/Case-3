// Package http — SystemHandler: Admin Panic Button & System Health Audit
//
// Provides admin-only endpoints for auditing system health and performing
// emergency recovery actions ("panic button"). These endpoints are the
// last line of defense when the system enters a zombie state (breakers
// stuck open, DLQ full, overflow queue growing unbounded).
//
// Endpoints:
//   GET  /api/v1/admin/system-health  — Full system health audit
//   POST /api/v1/admin/system-reset   — Emergency recovery actions
//
// Both endpoints require super_admin authentication.
package http

import (
	"encoding/json"
	"net/http"
	"runtime"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/clickhouse"
	"github.com/argus-ai/event-collector/internal/infrastructure/dlq"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// SystemHealthResponse is the full system health audit payload.
type SystemHealthResponse struct {
	// Kafka pipeline health.
	KafkaBreakerState  string `json:"kafka_breaker_state"`
	KafkaDLQSize       int64  `json:"kafka_dlq_size"`
	KafkaIsDegraded    bool   `json:"kafka_is_degraded"`
	KafkaDegradedFor   string `json:"kafka_degraded_for"`

	// ClickHouse pipeline health.
	CHBreakerState     string `json:"ch_breaker_state"`
	CHOverflowSize     int    `json:"ch_overflow_size"`
	CHBackpressure     bool   `json:"ch_backpressure_active"`

	// Runtime metrics.
	GoroutineCount     int    `json:"goroutine_count"`
	MemAllocMB         uint64 `json:"mem_alloc_mb"`
	MemSysMB           uint64 `json:"mem_sys_mb"`
	NumGC              uint32 `json:"num_gc"`
}

// SystemResetRequest specifies which recovery actions to perform.
type SystemResetRequest struct {
	Actions []string `json:"actions"`
}

// SystemResetResponse reports the result of each recovery action.
type SystemResetResponse struct {
	Results []ActionResult `json:"results"`
}

// ActionResult is the outcome of a single recovery action.
type ActionResult struct {
	Action  string `json:"action"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// Handler
// ---------------------------------------------------------------------------

// SystemHandler provides system health audit and panic-button reset endpoints.
type SystemHandler struct {
	dlqWriter  *dlq.ResilientWriter
	chWriter   *clickhouse.Writer
	logger     *zap.Logger

	// Embeds AdminHandler for auth middleware reuse.
	adminAuth  *AdminHandler
}

// NewSystemHandler creates a new system handler.
// dlqWriter may be nil if DLQ is disabled.
// chWriter may be nil if ClickHouse is unavailable.
func NewSystemHandler(
	dlqWriter *dlq.ResilientWriter,
	chWriter *clickhouse.Writer,
	logger *zap.Logger,
	jwtSigningKey []byte,
	authHandler *AdminHandler,
) *SystemHandler {
	return &SystemHandler{
		dlqWriter:  dlqWriter,
		chWriter:   chWriter,
		logger:     logger.Named("system_api"),
		adminAuth:  authHandler,
	}
}

// RegisterRoutes registers system health and reset endpoints.
func (h *SystemHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/system-health",
		h.adminAuth.requireAuth(h.adminAuth.requireRole(entity.RoleSuperAdmin, h.handleSystemHealth)))
	mux.HandleFunc("POST /api/v1/admin/system-reset",
		h.adminAuth.requireAuth(h.adminAuth.requireRole(entity.RoleSuperAdmin, h.handleSystemReset)))
}

// ---------------------------------------------------------------------------
// GET /api/v1/admin/system-health
// ---------------------------------------------------------------------------

func (h *SystemHandler) handleSystemHealth(w http.ResponseWriter, r *http.Request) {
	resp := SystemHealthResponse{
		GoroutineCount: runtime.NumGoroutine(),
	}

	// Memory stats.
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	resp.MemAllocMB = memStats.Alloc / (1024 * 1024)
	resp.MemSysMB = memStats.Sys / (1024 * 1024)
	resp.NumGC = memStats.NumGC

	// Kafka / DLQ health.
	if h.dlqWriter != nil {
		resp.KafkaBreakerState = h.dlqWriter.BreakerState()
		resp.KafkaDLQSize = h.dlqWriter.DLQSize()
		resp.KafkaIsDegraded = h.dlqWriter.IsDegraded()
		resp.KafkaDegradedFor = h.dlqWriter.DegradedDuration().String()
	} else {
		resp.KafkaBreakerState = "n/a"
		resp.KafkaDegradedFor = "0s"
	}

	// ClickHouse health.
	if h.chWriter != nil {
		resp.CHBreakerState = h.chWriter.BreakerState()
		resp.CHOverflowSize = h.chWriter.OverflowSize()
		resp.CHBackpressure = h.chWriter.BackpressureActive()
	} else {
		resp.CHBreakerState = "n/a"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)

	h.logger.Info("system health audit requested",
		zap.String("kafka_breaker", resp.KafkaBreakerState),
		zap.String("ch_breaker", resp.CHBreakerState),
		zap.Int64("dlq_size", resp.KafkaDLQSize),
		zap.Int("overflow_size", resp.CHOverflowSize),
		zap.Int("goroutines", resp.GoroutineCount),
	)
}

// ---------------------------------------------------------------------------
// POST /api/v1/admin/system-reset
// ---------------------------------------------------------------------------

func (h *SystemHandler) handleSystemReset(w http.ResponseWriter, r *http.Request) {
	var req SystemResetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if len(req.Actions) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "at least one action is required"})
		return
	}

	resp := SystemResetResponse{
		Results: make([]ActionResult, 0, len(req.Actions)),
	}

	for _, action := range req.Actions {
		result := h.executeAction(action)
		resp.Results = append(resp.Results, result)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)

	h.logger.Warn("system reset executed by admin",
		zap.Strings("actions", req.Actions),
		zap.Int("results", len(resp.Results)),
	)
}

// executeAction performs a single recovery action and returns the result.
// Supported actions:
//   - "reset_kafka_breaker"  — Reset the Kafka circuit breaker to Closed.
//   - "reset_ch_breaker"     — Reset the ClickHouse circuit breaker to Closed.
//   - "clear_dlq"            — Remove all entries from the BadgerDB dead-letter queue.
//   - "flush_overflow"       — Trigger immediate drain of the ClickHouse overflow queue.
func (h *SystemHandler) executeAction(action string) ActionResult {
	switch action {
	case "reset_kafka_breaker":
		if h.dlqWriter == nil {
			return ActionResult{Action: action, Success: false, Message: "DLQ/Kafka writer not available"}
		}
		h.dlqWriter.ResetBreaker()
		h.logger.Warn("admin: Kafka circuit breaker manually reset")
		return ActionResult{Action: action, Success: true, Message: "Kafka circuit breaker reset to closed"}

	case "reset_ch_breaker":
		if h.chWriter == nil {
			return ActionResult{Action: action, Success: false, Message: "ClickHouse writer not available"}
		}
		h.chWriter.ResetBreaker()
		h.logger.Warn("admin: ClickHouse circuit breaker manually reset")
		return ActionResult{Action: action, Success: true, Message: "ClickHouse circuit breaker reset to closed"}

	case "clear_dlq":
		if h.dlqWriter == nil {
			return ActionResult{Action: action, Success: false, Message: "DLQ writer not available"}
		}
		if err := h.dlqWriter.ClearDLQ(); err != nil {
			h.logger.Error("admin: DLQ clear failed", zap.Error(err))
			return ActionResult{Action: action, Success: false, Message: "DLQ clear failed: " + err.Error()}
		}
		h.logger.Warn("admin: DLQ cleared by admin")
		return ActionResult{Action: action, Success: true, Message: "DLQ cleared successfully"}

	case "flush_overflow":
		if h.chWriter == nil {
			return ActionResult{Action: action, Success: false, Message: "ClickHouse writer not available"}
		}
		h.chWriter.DrainOverflow()
		h.logger.Warn("admin: ClickHouse overflow drain triggered")
		return ActionResult{Action: action, Success: true, Message: "ClickHouse overflow drain triggered"}

	default:
		return ActionResult{Action: action, Success: false, Message: "unknown action: " + action}
	}
}
