package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ---------------------------------------------------------------------------
// HTTP Metrics Middleware
//
// Records Prometheus histograms and counters for all HTTP requests.
// Metrics are registered via promauto (auto-register with default registry).
//
// Exposed metrics:
//   argus_http_request_duration_seconds — Histogram of request latency
//   argus_http_requests_total           — Counter of total requests
// ---------------------------------------------------------------------------

var (
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "argus_http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path", "status"},
	)

	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "argus_http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)
)

// MetricsHTTPMiddleware records request duration and count for all HTTP requests.
// The "path" label is normalized to the route pattern to avoid high-cardinality
// label explosion from dynamic path parameters.
func MetricsHTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap the response writer to capture the status code.
		wrapped := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start).Seconds()
		statusStr := strconv.Itoa(wrapped.statusCode)

		// Use the route pattern if available (Go 1.22+ ServeMux).
		// Falls back to a generic path to avoid cardinality explosion.
		path := normalizePath(r)

		httpRequestDuration.WithLabelValues(r.Method, path, statusStr).Observe(duration)
		httpRequestsTotal.WithLabelValues(r.Method, path, statusStr).Inc()
	})
}

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// normalizePath returns a normalized path label for Prometheus metrics.
// Uses the route pattern from Go 1.22+ ServeMux when available, otherwise
// falls back to a truncated/generic path to prevent label cardinality explosion.
func normalizePath(r *http.Request) string {
	// Go 1.22+ sets r.Pattern on matched routes.
	if r.Pattern != "" {
		return r.Pattern
	}

	// Fallback: use the raw path but cap length to prevent unbounded labels.
	path := r.URL.Path
	if len(path) > 64 {
		path = path[:64]
	}
	return path
}
