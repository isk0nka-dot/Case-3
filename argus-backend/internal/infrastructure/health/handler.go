// Package health provides HTTP handlers for Kubernetes liveness and readiness
// probes. It implements the standard health-check pattern where:
//
//   - Liveness returns 200 as long as the process is alive (not deadlocked).
//     Kubernetes uses this to decide whether to restart the pod.
//   - Readiness returns 200 only when ALL infrastructure dependencies (Kafka,
//     ClickHouse, etc.) are reachable. Kubernetes uses this to decide whether
//     to route traffic to the pod.
//
// Both endpoints return a JSON body describing the status of each dependency,
// which is invaluable for debugging during rollouts and incident response.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/application/port"
)

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

// Status represents the overall health state of the service.
type Status string

const (
	// StatusOK means all dependency checks passed.
	StatusOK Status = "ok"

	// StatusDegraded means at least one non-critical check failed.
	// The service can still serve traffic but with reduced functionality.
	StatusDegraded Status = "degraded"

	// StatusUnhealthy means at least one critical check failed.
	// The service should be taken out of the load-balancer rotation.
	StatusUnhealthy Status = "unhealthy"
)

// Response is the JSON body returned by both liveness and readiness endpoints.
type Response struct {
	Status    Status        `json:"status"`
	Checks   []CheckResult `json:"checks,omitempty"`
	Pipeline *PipelineInfo `json:"pipeline,omitempty"`
}

// PipelineInfo exposes the Kafka/DLQ pipeline state for observability.
// Included in readiness responses when a DLQ status provider is configured.
type PipelineInfo struct {
	BreakerState string `json:"breaker_state"`
	DLQSize      int64  `json:"dlq_size"`
	IsDegraded   bool   `json:"is_degraded"`
	DegradedFor  string `json:"degraded_for,omitempty"`
}

// DLQStatusProvider is an optional interface for exposing DLQ/pipeline metrics
// through the health endpoint. The ResilientWriter implements this.
type DLQStatusProvider interface {
	BreakerState() string
	DLQSize() int64
	IsDegraded() bool
	DegradedDuration() time.Duration
}

// CheckResult is the outcome of a single dependency health check.
type CheckResult struct {
	Name     string `json:"name"`
	Status   Status `json:"status"`
	Critical bool   `json:"critical"`
	Error    string `json:"error,omitempty"`
}

// ---------------------------------------------------------------------------
// Handler
// ---------------------------------------------------------------------------

// checkTimeout is the maximum duration allowed for each individual health
// check. If a dependency does not respond within this window, the check is
// considered failed. This prevents a slow dependency from blocking the entire
// readiness probe and triggering Kubernetes timeouts.
const checkTimeout = 5 * time.Second

// TaggedChecker wraps a port.HealthChecker with a criticality flag.
// Critical checkers (e.g. Kafka) cause StatusUnhealthy when they fail.
// Non-critical checkers (e.g. ClickHouse) cause StatusDegraded — the service
// can still serve traffic but with reduced functionality.
type TaggedChecker struct {
	Checker  port.HealthChecker
	Critical bool
}

// Critical wraps a checker as critical (failure → unhealthy/503).
func Critical(c port.HealthChecker) TaggedChecker {
	return TaggedChecker{Checker: c, Critical: true}
}

// NonCritical wraps a checker as non-critical (failure → degraded/200).
func NonCritical(c port.HealthChecker) TaggedChecker {
	return TaggedChecker{Checker: c, Critical: false}
}

// Handler exposes HTTP endpoints for health checking. It holds references to
// all TaggedChecker implementations (Kafka, ClickHouse, etc.) and runs
// them concurrently during readiness probes.
type Handler struct {
	checkers    []TaggedChecker
	dlqProvider DLQStatusProvider
	logger      *zap.Logger
}

// NewHandler creates a Handler with the given logger and tagged health checkers.
// Each checker is tagged as critical or non-critical:
//   - Critical failure → StatusUnhealthy (503)
//   - Non-critical failure only → StatusDegraded (200)
//   - All pass → StatusOK (200)
func NewHandler(logger *zap.Logger, checkers ...TaggedChecker) *Handler {
	return &Handler{
		checkers: checkers,
		logger:   logger.Named("health"),
	}
}

// SetDLQProvider wires the DLQ status provider for pipeline observability.
func (h *Handler) SetDLQProvider(p DLQStatusProvider) {
	h.dlqProvider = p
}

// LivenessHandler returns an http.HandlerFunc that always responds with
// HTTP 200 and a static JSON body. If this handler can execute at all, the
// process is alive.
//
// Route: GET /healthz
func (h *Handler) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := Response{
			Status: StatusOK,
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// ReadinessHandler returns an http.HandlerFunc that executes every registered
// health checker concurrently. The overall status is:
//
//   - "ok"        if every check passes.
//   - "unhealthy" if any check fails.
//
// HTTP status codes:
//   - 200 when all checks pass.
//   - 503 when any check fails (tells Kubernetes to stop routing traffic).
//
// Route: GET /readyz
func (h *Handler) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// No checkers registered — service is trivially ready.
		if len(h.checkers) == 0 {
			writeJSON(w, http.StatusOK, Response{Status: StatusOK})
			return
		}

		results := h.runChecks(r.Context())

		// Determine overall status using criticality tags:
		//   - Any critical failure   → StatusUnhealthy (503)
		//   - Non-critical failure   → StatusDegraded  (200)
		//   - All pass               → StatusOK        (200)
		overallStatus := StatusOK
		for _, result := range results {
			if result.Status != StatusOK {
				if result.Critical {
					overallStatus = StatusUnhealthy
					h.logger.Warn("readiness check failed (critical)",
						zap.String("check", result.Name),
						zap.String("error", result.Error),
					)
				} else if overallStatus != StatusUnhealthy {
					overallStatus = StatusDegraded
					h.logger.Warn("readiness check failed (non-critical, degraded)",
						zap.String("check", result.Name),
						zap.String("error", result.Error),
					)
				}
			}
		}

		resp := Response{
			Status: overallStatus,
			Checks: results,
		}

		// Include pipeline status if DLQ provider is configured.
		if h.dlqProvider != nil {
			pi := &PipelineInfo{
				BreakerState: h.dlqProvider.BreakerState(),
				DLQSize:      h.dlqProvider.DLQSize(),
				IsDegraded:   h.dlqProvider.IsDegraded(),
			}
			if pi.IsDegraded {
				pi.DegradedFor = h.dlqProvider.DegradedDuration().Round(time.Second).String()
				// Pipeline degradation is non-critical — we're still accepting events.
				if overallStatus == StatusOK {
					overallStatus = StatusDegraded
					resp.Status = overallStatus
				}
			}
			resp.Pipeline = pi
		}

		httpStatus := http.StatusOK
		if overallStatus == StatusUnhealthy {
			httpStatus = http.StatusServiceUnavailable
		}

		writeJSON(w, httpStatus, resp)
	}
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// runChecks executes all registered health checkers concurrently and collects
// results. Each check runs in its own goroutine with a per-check timeout so
// that a single slow dependency does not block the entire probe.
func (h *Handler) runChecks(parentCtx context.Context) []CheckResult {
	results := make([]CheckResult, len(h.checkers))
	var wg sync.WaitGroup

	for i, tc := range h.checkers {
		wg.Add(1)
		go func(idx int, tc TaggedChecker) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(parentCtx, checkTimeout)
			defer cancel()

			err := tc.Checker.Check(ctx)
			if err != nil {
				results[idx] = CheckResult{
					Name:     tc.Checker.Name(),
					Status:   StatusUnhealthy,
					Critical: tc.Critical,
					Error:    err.Error(),
				}
			} else {
				results[idx] = CheckResult{
					Name:     tc.Checker.Name(),
					Status:   StatusOK,
					Critical: tc.Critical,
				}
			}
		}(i, tc)
	}

	wg.Wait()
	return results
}

// writeJSON serialises v as JSON and writes it to the http.ResponseWriter
// with the given status code. If serialisation fails (should never happen
// with our simple structs), it falls back to a plain-text 500 response.
func writeJSON(w http.ResponseWriter, statusCode int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		// This is a last-resort fallback. If JSON encoding fails, write a
		// minimal plain-text error. The status code has already been sent,
		// so we cannot change it.
		http.Error(w, `{"status":"error","message":"failed to encode response"}`, http.StatusInternalServerError)
	}
}
