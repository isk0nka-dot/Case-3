// Package metrics provides Prometheus metric definitions and registration
// for the Argus AI Event Collector service.
//
// Metrics are organized by subsystem:
//   - grpc:     gRPC request counters and latency histograms
//   - http:     HTTP request counters and latency histograms
//   - kafka:    Kafka publish counters and latency histograms
//   - evidence: Evidence upload counters and latency histograms
//   - export:   Bulk export job counters
//   - session:  Active session gauges
//
// Usage:
//
//	metrics := metrics.NewCollector()
//	metrics.Register()  // register with prometheus.DefaultRegisterer
//	// In gRPC interceptor:
//	metrics.GRPCRequestsTotal.WithLabelValues("IngestEvent", "ok").Inc()
//	metrics.GRPCRequestDuration.WithLabelValues("IngestEvent").Observe(elapsed)
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Collector holds all Prometheus metric instruments for the service.
type Collector struct {
	// ── gRPC ──────────────────────────────────────────────────────────────

	// GRPCRequestsTotal counts gRPC requests by method and status.
	GRPCRequestsTotal *prometheus.CounterVec

	// GRPCRequestDuration is a histogram of gRPC request durations.
	GRPCRequestDuration *prometheus.HistogramVec

	// ── HTTP ──────────────────────────────────────────────────────────────

	// HTTPRequestsTotal counts HTTP requests by path and status.
	HTTPRequestsTotal *prometheus.CounterVec

	// HTTPRequestDuration is a histogram of HTTP request durations.
	HTTPRequestDuration *prometheus.HistogramVec

	// ── Kafka ─────────────────────────────────────────────────────────────

	// KafkaPublishTotal counts Kafka messages published by topic and status.
	KafkaPublishTotal *prometheus.CounterVec

	// KafkaPublishDuration is a histogram of Kafka publish durations.
	KafkaPublishDuration *prometheus.HistogramVec

	// ── Evidence ──────────────────────────────────────────────────────────

	// EvidenceUploadTotal counts evidence uploads by status.
	EvidenceUploadTotal *prometheus.CounterVec

	// EvidenceUploadDuration is a histogram of evidence upload durations.
	EvidenceUploadDuration *prometheus.HistogramVec

	// EvidenceUploadBytes is a histogram of evidence upload sizes.
	EvidenceUploadBytes *prometheus.HistogramVec

	// ── Events ────────────────────────────────────────────────────────────

	// EventsIngestedTotal counts events ingested by type and severity.
	EventsIngestedTotal *prometheus.CounterVec

	// ── Export ─────────────────────────────────────────────────────────────

	// ExportJobsTotal counts export jobs by status.
	ExportJobsTotal *prometheus.CounterVec

	// ── Sessions ──────────────────────────────────────────────────────────

	// ActiveSessionsCount is a gauge of currently active proctoring sessions.
	ActiveSessionsCount prometheus.Gauge

	// ── Workers ───────────────────────────────────────────────────────────

	// WorkerPoolUtilization is a gauge of worker pool usage (0-1).
	WorkerPoolUtilization prometheus.Gauge

	// ── Chunk Assembly ────────────────────────────────────────────────────

	// ChunkAssemblerActiveFragments is a gauge of in-progress fragment assemblies.
	ChunkAssemblerActiveFragments prometheus.Gauge
}

// NewCollector creates a new metric collector with all instruments initialized.
func NewCollector() *Collector {
	return &Collector{
		// gRPC
		GRPCRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "argus",
				Subsystem: "grpc",
				Name:      "requests_total",
				Help:      "Total gRPC requests by method and status.",
			},
			[]string{"method", "status"},
		),
		GRPCRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "argus",
				Subsystem: "grpc",
				Name:      "request_duration_seconds",
				Help:      "gRPC request duration in seconds.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method"},
		),

		// HTTP
		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "argus",
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total HTTP requests by path and status code.",
			},
			[]string{"path", "status"},
		),
		HTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "argus",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "HTTP request duration in seconds.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"path"},
		),

		// Kafka
		KafkaPublishTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "argus",
				Subsystem: "kafka",
				Name:      "publish_total",
				Help:      "Total Kafka messages published by topic and status.",
			},
			[]string{"topic", "status"},
		),
		KafkaPublishDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "argus",
				Subsystem: "kafka",
				Name:      "publish_duration_seconds",
				Help:      "Kafka publish duration in seconds.",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
			},
			[]string{"topic"},
		),

		// Evidence
		EvidenceUploadTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "argus",
				Subsystem: "evidence",
				Name:      "upload_total",
				Help:      "Total evidence uploads by status.",
			},
			[]string{"status"},
		),
		EvidenceUploadDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "argus",
				Subsystem: "evidence",
				Name:      "upload_duration_seconds",
				Help:      "Evidence upload duration in seconds.",
				Buckets:   []float64{0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0},
			},
			[]string{},
		),
		EvidenceUploadBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "argus",
				Subsystem: "evidence",
				Name:      "upload_bytes",
				Help:      "Evidence upload size in bytes.",
				Buckets:   []float64{1024, 10240, 102400, 1048576, 10485760, 52428800},
			},
			[]string{},
		),

		// Events
		EventsIngestedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "argus",
				Subsystem: "events",
				Name:      "ingested_total",
				Help:      "Total events ingested by type and severity.",
			},
			[]string{"event_type", "severity"},
		),

		// Export
		ExportJobsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "argus",
				Subsystem: "export",
				Name:      "jobs_total",
				Help:      "Total export jobs by status.",
			},
			[]string{"status"},
		),

		// Sessions
		ActiveSessionsCount: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "argus",
				Subsystem: "sessions",
				Name:      "active_count",
				Help:      "Number of currently active proctoring sessions.",
			},
		),

		// Workers
		WorkerPoolUtilization: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "argus",
				Subsystem: "workers",
				Name:      "pool_utilization",
				Help:      "Worker pool utilization ratio (0-1).",
			},
		),

		// Chunk Assembly
		ChunkAssemblerActiveFragments: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "argus",
				Subsystem: "chunk_assembler",
				Name:      "active_fragments",
				Help:      "Number of in-progress fragment assemblies.",
			},
		),
	}
}

// Register registers all metrics with the default Prometheus registerer.
// Safe to call multiple times — will not panic on duplicate registration.
func (c *Collector) Register() {
	registerers := []prometheus.Collector{
		c.GRPCRequestsTotal,
		c.GRPCRequestDuration,
		c.HTTPRequestsTotal,
		c.HTTPRequestDuration,
		c.KafkaPublishTotal,
		c.KafkaPublishDuration,
		c.EvidenceUploadTotal,
		c.EvidenceUploadDuration,
		c.EvidenceUploadBytes,
		c.EventsIngestedTotal,
		c.ExportJobsTotal,
		c.ActiveSessionsCount,
		c.WorkerPoolUtilization,
		c.ChunkAssemblerActiveFragments,
	}

	for _, r := range registerers {
		// Use MustRegister in production. For safety during tests,
		// we silently ignore duplicate registration errors.
		_ = prometheus.DefaultRegisterer.Register(r)
	}
}
