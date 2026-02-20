// Package clickhouse implements the ClickHouse batch writer for event analytics.
// This is an infrastructure adapter that implements the port.EventWriter interface.
//
// Architecture-level design decisions:
//
//   - ClickHouse is the SECONDARY store — Kafka is the source of truth.
//     If ClickHouse is down, events are still safely in Kafka and a separate
//     consumer backfills ClickHouse from the topic. This means ClickHouse
//     write failures are NEVER fatal to the ingestion pipeline.
//
//   - Buffered batch inserts via the ClickHouse Native Protocol.
//     The native protocol is 2-3x faster than HTTP for bulk inserts because
//     it avoids JSON parsing overhead on the server side and uses columnar
//     data encoding on the wire, matching ClickHouse's internal storage format.
//
//   - Two-stage buffer with swap technique. Write() appends to a "hot" buffer
//     under a brief mutex. When flush triggers, the hot buffer is atomically
//     swapped with an empty "cold" buffer. The flush goroutine then writes the
//     cold buffer to ClickHouse WITHOUT holding the mutex — new events continue
//     accumulating in the hot buffer concurrently. This minimizes lock contention
//     to a single pointer swap per flush cycle.
//
//   - Bounded overflow queue for backpressure resilience. If ClickHouse is slow
//     or down, failed batches are pushed onto a bounded overflow queue (default
//     capacity: 50 batches = ~50,000 events). When ClickHouse recovers, the
//     overflow is drained before new batches. If the overflow fills up, the
//     oldest batch is dropped and logged — this is acceptable because Kafka
//     retains the data for replay.
//
//   - Exponential backoff with jitter on retries to prevent thundering-herd
//     reconnections after a ClickHouse restart.
//
//   - Health check via ClickHouse PING for Kubernetes readiness probes.
package clickhouse

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/domain/entity"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// WriterConfig holds ClickHouse writer configuration. All fields have yaml
// tags for config-file binding and sensible defaults applied in NewWriter().
type WriterConfig struct {
	// Addrs is the list of ClickHouse native protocol addresses (host:port).
	// Multiple addresses enable client-side load balancing across ClickHouse
	// cluster nodes. Default: ["localhost:9000"].
	Addrs []string `yaml:"addrs"`

	// Database is the target ClickHouse database for all inserts.
	// Default: "argus_analytics".
	Database string `yaml:"database"`

	// Username for ClickHouse authentication. Default: "default".
	Username string `yaml:"username"`

	// Password for ClickHouse authentication. Default: "" (no password).
	Password string `yaml:"password"`

	// BatchSize is the number of events buffered before triggering an
	// immediate flush, even if FlushInterval has not elapsed.
	// Default: 1,000.
	//
	// ClickHouse documentation recommends batches of 1,000-100,000 rows
	// for optimal MergeTree insert performance. Batches smaller than ~100
	// create excessive part files that degrade read performance until merges
	// catch up.
	BatchSize int `yaml:"batch_size"`

	// FlushInterval is the maximum time between periodic flushes.
	// Guarantees bounded latency: even during low-traffic periods, events
	// reach ClickHouse within this interval. Default: 5 seconds.
	FlushInterval time.Duration `yaml:"flush_interval"`

	// MaxRetries is the number of times a failed batch insert is retried
	// with exponential backoff before the batch is moved to the overflow
	// queue. Default: 3.
	MaxRetries int `yaml:"max_retries"`

	// OverflowQueueSize is the maximum number of failed batches held in
	// memory for retry. When ClickHouse recovers, the overflow is drained
	// before new batches are flushed.
	// Default: 50 (= 50 * BatchSize = ~50,000 events in memory).
	//
	// If the overflow fills up, the OLDEST batch is dropped. This is
	// acceptable because Kafka retains the original events for replay.
	OverflowQueueSize int `yaml:"overflow_queue_size"`

	// MaxOpenConns limits the number of simultaneous ClickHouse connections.
	// Default: 10.
	MaxOpenConns int `yaml:"max_open_conns"`

	// MaxIdleConns is the number of connections kept in the pool.
	// Default: 5.
	MaxIdleConns int `yaml:"max_idle_conns"`

	// ConnMaxLifetime is the maximum duration a connection is reused before
	// being closed and replaced. Prevents stale connections after ClickHouse
	// failovers. Default: 10 minutes.
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`

	// DialTimeout is the maximum time to establish a connection.
	// Default: 5 seconds.
	DialTimeout time.Duration `yaml:"dial_timeout"`

	// InsertTimeout is the maximum time for a single batch INSERT to complete.
	// Default: 30 seconds.
	InsertTimeout time.Duration `yaml:"insert_timeout"`
}

// ---------------------------------------------------------------------------
// Writer
// ---------------------------------------------------------------------------

// Writer implements port.EventWriter for ClickHouse analytical storage.
// It buffers events in memory and periodically flushes them as batch INSERTs
// using the ClickHouse native protocol for maximum throughput.
//
// Thread safety: All exported methods are safe for concurrent use.
// Write() and WriteBatch() are effectively non-blocking (mutex is held only
// for a slice append, ~50ns).
//
// The writer manages three background activities:
//  1. Periodic flusher — triggers every FlushInterval.
//  2. Overflow drainer — retries failed batches when ClickHouse recovers.
//  3. Metrics tracking — lock-free atomic counters.
type Writer struct {
	conn   driver.Conn
	logger *zap.Logger
	cfg    WriterConfig

	// Hot buffer: current accumulation target.
	// Protected by mu. Lock is held only for append (O(1) amortized).
	buffer []*entity.ProctoringEvent
	mu     sync.Mutex

	// Overflow queue: bounded ring buffer for failed batches.
	// When ClickHouse is down, failed batches are queued here for retry.
	overflow     [][]*entity.ProctoringEvent
	overflowMu   sync.Mutex
	overflowFull atomic.Int64 // Counter: batches dropped due to full overflow.

	// Lifecycle management.
	done chan struct{}
	wg   sync.WaitGroup

	// Lock-free metrics.
	totalFlushed atomic.Int64 // Events successfully written to ClickHouse.
	totalDropped atomic.Int64 // Events lost (overflow full + all retries exhausted).
	totalRetries atomic.Int64 // Total retry attempts across all batches.
	flushCount   atomic.Int64 // Number of successful flush cycles.
	flushErrors  atomic.Int64 // Number of flush failures (before overflow).
}

// NewWriter creates a new ClickHouse batch writer and starts the background
// flusher goroutine.
//
// The constructor verifies connectivity with a PING before returning. If
// ClickHouse is unreachable, NewWriter returns an error and the service
// should refuse to start (fail-fast at startup, not at runtime).
func NewWriter(cfg WriterConfig, logger *zap.Logger) (*Writer, error) {
	applyWriterDefaults(&cfg)

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: cfg.Addrs,
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Settings: clickhouse.Settings{
			// Server-side query timeout. Prevents runaway INSERTs from
			// holding connections indefinitely.
			"max_execution_time": 60,
		},
		DialTimeout:     cfg.DialTimeout,
		MaxOpenConns:    cfg.MaxOpenConns,
		MaxIdleConns:    cfg.MaxIdleConns,
		ConnMaxLifetime: cfg.ConnMaxLifetime,
	})
	if err != nil {
		return nil, fmt.Errorf("clickhouse: failed to open connection: %w", err)
	}

	// Verify connectivity at startup — fail-fast rather than silently
	// buffering events that can never be flushed.
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("clickhouse: ping failed (is ClickHouse running?): %w", err)
	}

	w := &Writer{
		conn:     conn,
		logger:   logger.Named("clickhouse_writer"),
		cfg:      cfg,
		buffer:   make([]*entity.ProctoringEvent, 0, cfg.BatchSize),
		overflow: make([][]*entity.ProctoringEvent, 0, cfg.OverflowQueueSize),
		done:     make(chan struct{}),
	}

	// Start the background flusher.
	w.wg.Add(1)
	go w.backgroundFlusher()

	w.logger.Info("clickhouse writer initialized",
		zap.Strings("addrs", cfg.Addrs),
		zap.String("database", cfg.Database),
		zap.Int("batch_size", cfg.BatchSize),
		zap.Duration("flush_interval", cfg.FlushInterval),
		zap.Int("max_retries", cfg.MaxRetries),
		zap.Int("overflow_queue_size", cfg.OverflowQueueSize),
		zap.Int("max_open_conns", cfg.MaxOpenConns),
	)

	return w, nil
}

// ---------------------------------------------------------------------------
// port.EventWriter Implementation
// ---------------------------------------------------------------------------

// Write buffers a single event for batch insertion. This method is effectively
// non-blocking — the mutex is held only for a slice append operation (~50ns).
//
// The event is NOT written to ClickHouse immediately. It accumulates in the
// internal buffer and is flushed either:
//   - When the buffer reaches BatchSize (immediate flush).
//   - On the next FlushInterval tick (periodic flush).
//
// Returns nil always — ClickHouse write errors are handled asynchronously
// in the flush goroutine. Since ClickHouse is the secondary store, individual
// write failures do not propagate back to the caller.
func (w *Writer) Write(_ context.Context, event *entity.ProctoringEvent) error {
	w.mu.Lock()
	w.buffer = append(w.buffer, event)
	shouldFlush := len(w.buffer) >= w.cfg.BatchSize
	w.mu.Unlock()

	// If the buffer is full, trigger an immediate flush in a separate
	// goroutine to avoid blocking the caller (which is on the hot path
	// of the gRPC ingestion pipeline).
	if shouldFlush {
		go w.flush()
	}

	return nil
}

// WriteBatch buffers multiple events for batch insertion.
// Same semantics as Write() — events are appended to the buffer and
// flushed asynchronously.
func (w *Writer) WriteBatch(_ context.Context, events []*entity.ProctoringEvent) error {
	w.mu.Lock()
	w.buffer = append(w.buffer, events...)
	shouldFlush := len(w.buffer) >= w.cfg.BatchSize
	w.mu.Unlock()

	if shouldFlush {
		go w.flush()
	}

	return nil
}

// Close flushes all remaining events and shuts down the writer.
//
// Shutdown sequence:
//  1. Signal the background flusher to stop.
//  2. Wait for the flusher goroutine to exit.
//  3. Perform a final flush of any remaining buffer contents.
//  4. Drain the overflow queue (best-effort).
//  5. Close the ClickHouse connection pool.
func (w *Writer) Close() error {
	close(w.done)
	w.wg.Wait()

	// Final flush of remaining events in the buffer.
	w.flush()

	// Best-effort drain of the overflow queue.
	w.drainOverflow()

	stats := w.Metrics()
	w.logger.Info("clickhouse writer closed",
		zap.Int64("total_flushed", stats.TotalFlushed),
		zap.Int64("total_dropped", stats.TotalDropped),
		zap.Int64("flush_count", stats.FlushCount),
		zap.Int64("flush_errors", stats.FlushErrors),
	)

	return w.conn.Close()
}

// ---------------------------------------------------------------------------
// Health Check (port.HealthChecker)
// ---------------------------------------------------------------------------

// Check verifies that ClickHouse is reachable via PING.
// Called by the HTTP /readyz endpoint.
func (w *Writer) Check(ctx context.Context) error {
	return w.conn.Ping(ctx)
}

// Name returns the health checker name.
func (w *Writer) Name() string {
	return "clickhouse_writer"
}

// Conn returns the underlying ClickHouse driver connection for read queries.
// This is used by the analytics handler to query materialized views without
// creating a separate connection pool. The returned connection is thread-safe
// (the ClickHouse driver manages connection pooling internally).
func (w *Writer) Conn() driver.Conn {
	return w.conn
}

// ---------------------------------------------------------------------------
// Metrics
// ---------------------------------------------------------------------------

// WriterMetrics holds real-time writer statistics.
type WriterMetrics struct {
	TotalFlushed  int64 // Events successfully written to ClickHouse.
	TotalDropped  int64 // Events permanently lost (overflow full).
	TotalRetries  int64 // Total retry attempts.
	FlushCount    int64 // Successful flush cycles.
	FlushErrors   int64 // Failed flush attempts (before overflow).
	OverflowDrops int64 // Batches dropped because overflow was full.
	BufferSize    int   // Current number of events in the buffer.
	OverflowSize  int   // Current number of batches in the overflow queue.
}

// Metrics returns a snapshot of the current writer statistics.
func (w *Writer) Metrics() WriterMetrics {
	w.mu.Lock()
	bufSize := len(w.buffer)
	w.mu.Unlock()

	w.overflowMu.Lock()
	ovfSize := len(w.overflow)
	w.overflowMu.Unlock()

	return WriterMetrics{
		TotalFlushed:  w.totalFlushed.Load(),
		TotalDropped:  w.totalDropped.Load(),
		TotalRetries:  w.totalRetries.Load(),
		FlushCount:    w.flushCount.Load(),
		FlushErrors:   w.flushErrors.Load(),
		OverflowDrops: w.overflowFull.Load(),
		BufferSize:    bufSize,
		OverflowSize:  ovfSize,
	}
}

// ---------------------------------------------------------------------------
// Background Flusher
// ---------------------------------------------------------------------------

// backgroundFlusher runs in a dedicated goroutine for the lifetime of the writer.
// On every tick, it:
//  1. Drains the overflow queue (failed batches from previous cycles).
//  2. Flushes the current buffer.
//
// The overflow is drained FIRST so that events are written to ClickHouse in
// approximately chronological order, even after a recovery from downtime.
func (w *Writer) backgroundFlusher() {
	defer w.wg.Done()
	ticker := time.NewTicker(w.cfg.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Drain overflow first (old batches before new ones).
			w.drainOverflow()
			// Then flush the current buffer.
			w.flush()
		case <-w.done:
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Flush Logic
// ---------------------------------------------------------------------------

// flush atomically swaps the buffer, then writes the snapshot to ClickHouse.
//
// The swap technique ensures that:
//   - The mutex is held for only the duration of a pointer swap (~10ns).
//   - New events can continue accumulating while the flush is in progress.
//   - No memory is shared between the hot path (Write) and the cold path (flush).
func (w *Writer) flush() {
	// Atomically swap the buffer under the mutex.
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return
	}
	events := w.buffer
	w.buffer = make([]*entity.ProctoringEvent, 0, w.cfg.BatchSize)
	w.mu.Unlock()

	// Attempt to insert with retries.
	if err := w.insertWithRetry(events); err != nil {
		// All retries exhausted — push to the overflow queue.
		w.pushOverflow(events)
		w.flushErrors.Add(1)
		w.logger.Warn("clickhouse: batch failed, moved to overflow queue",
			zap.Int("event_count", len(events)),
			zap.Error(err),
		)
		return
	}

	// Success.
	w.totalFlushed.Add(int64(len(events)))
	w.flushCount.Add(1)
	w.logger.Debug("clickhouse: batch flushed",
		zap.Int("event_count", len(events)),
	)
}

// insertWithRetry attempts to insert a batch with exponential backoff + jitter.
//
// Jitter prevents thundering-herd reconnections when multiple service replicas
// retry simultaneously after a ClickHouse restart. The formula is:
//
//	sleep = backoff * (1 + rand[0, 0.5))
//
// Example with MaxRetries=3, base=200ms:
//
//	Attempt 1: 200ms * (1 + 0.23) = 246ms
//	Attempt 2: 400ms * (1 + 0.41) = 564ms
//	Attempt 3: 800ms * (1 + 0.12) = 896ms
func (w *Writer) insertWithRetry(events []*entity.ProctoringEvent) error {
	var lastErr error

	for attempt := 0; attempt <= w.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			w.totalRetries.Add(1)
			// Exponential backoff with jitter.
			baseBackoff := time.Duration(1<<uint(attempt)) * 100 * time.Millisecond
			jitter := time.Duration(float64(baseBackoff) * (0.5 * rand.Float64()))
			sleep := baseBackoff + jitter
			time.Sleep(sleep)
		}

		if err := w.insertBatch(events); err != nil {
			lastErr = err
			w.logger.Warn("clickhouse: insert attempt failed",
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", w.cfg.MaxRetries),
				zap.Int("event_count", len(events)),
				zap.Error(err),
			)
			continue
		}

		return nil // Success.
	}

	return lastErr
}

// insertBatch performs the actual batch INSERT into ClickHouse using the
// native protocol's PrepareBatch API.
//
// PrepareBatch creates a server-side prepared statement that accepts columnar
// data in ClickHouse's native binary format. This is significantly faster than
// HTTP-based JSON inserts because:
//   - No JSON parsing on the server (saves ~30% CPU per insert).
//   - Columnar wire encoding matches ClickHouse's internal storage format.
//   - Batch.Send() is a single network round-trip regardless of batch size.
//
// The INSERT statement maps exactly to the proctoring_events table schema
// defined in deployments/clickhouse/init.sql.
func (w *Writer) insertBatch(events []*entity.ProctoringEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), w.cfg.InsertTimeout)
	defer cancel()

	batch, err := w.conn.PrepareBatch(ctx, `
		INSERT INTO proctoring_events (
			event_id, session_id, student_id, exam_id, org_id,
			event_type, severity, source,
			server_timestamp, client_timestamp, video_timestamp_sec,
			label, confidence, payload, payload_type,
			user_agent, sdk_version, resolution,
			timezone_offset_min, ip_address, region,
			head_yaw, head_pitch, head_roll, face_bbox,
			liveness_score, face_embedding, face_similarity,
			audio_rms_db, vad_active, audio_classification,
			speaker_count, speaker_match
		)
	`)
	if err != nil {
		return fmt.Errorf("prepare batch failed: %w", err)
	}

	for _, event := range events {
		// Convert VAD bool to UInt8 for ClickHouse
		var vadActive uint8
		if event.VADActive {
			vadActive = 1
		}
		var speakerMatch uint8
		if event.SpeakerMatch {
			speakerMatch = 1
		}

		if err := batch.Append(
			event.EventID,
			event.SessionID,
			event.StudentID,
			event.ExamID,
			event.OrgID,
			event.EventType.String(),
			event.Severity.String(),
			event.Source.String(),
			event.ServerTimestamp,
			event.ClientTimestamp,
			event.VideoTimestamp,
			event.Label,
			event.Confidence,
			string(event.Payload),
			event.PayloadType,
			event.ClientMeta.UserAgent,
			event.ClientMeta.SDKVersion,
			event.ClientMeta.Resolution,
			event.ClientMeta.TimezoneOffsetMin,
			event.ClientMeta.IPAddress,
			event.ClientMeta.Region,
			event.HeadYaw,
			event.HeadPitch,
			event.HeadRoll,
			event.FaceBBox,
			event.LivenessScore,
			event.FaceEmbedding,
			event.FaceSimilarity,
			event.AudioRmsDb,
			vadActive,
			event.AudioClassification,
			event.SpeakerCount,
			speakerMatch,
		); err != nil {
			return fmt.Errorf("append to batch failed: %w", err)
		}
	}

	return batch.Send()
}

// ---------------------------------------------------------------------------
// Overflow Queue — Backpressure Resilience
// ---------------------------------------------------------------------------

// pushOverflow adds a failed batch to the overflow queue for later retry.
//
// If the queue is full, the OLDEST batch is evicted (FIFO) and its events
// are counted as permanently dropped. This is acceptable because:
//   - Kafka retains all events for the configured retention period (7 days).
//   - A separate Kafka-to-ClickHouse consumer can backfill missing data.
//   - Dropping the oldest batch preserves the freshest data, which is more
//     valuable for real-time dashboards.
func (w *Writer) pushOverflow(events []*entity.ProctoringEvent) {
	w.overflowMu.Lock()
	defer w.overflowMu.Unlock()

	if len(w.overflow) >= w.cfg.OverflowQueueSize {
		// Evict the oldest batch (index 0).
		dropped := w.overflow[0]
		w.overflow = w.overflow[1:]
		w.totalDropped.Add(int64(len(dropped)))
		w.overflowFull.Add(1)
		w.logger.Error("clickhouse: overflow queue full, dropping oldest batch",
			zap.Int("dropped_events", len(dropped)),
			zap.Int("queue_size", w.cfg.OverflowQueueSize),
			zap.Int64("total_dropped", w.totalDropped.Load()),
		)
	}

	w.overflow = append(w.overflow, events)
}

// drainOverflow attempts to flush all batches in the overflow queue.
// It stops at the first failure — remaining batches stay in the queue
// for the next drain cycle. This prevents hammering a sick ClickHouse
// with many concurrent retries.
func (w *Writer) drainOverflow() {
	for {
		// Pop the oldest batch from the queue.
		w.overflowMu.Lock()
		if len(w.overflow) == 0 {
			w.overflowMu.Unlock()
			return
		}
		batch := w.overflow[0]
		w.overflow = w.overflow[1:]
		w.overflowMu.Unlock()

		// Attempt to insert (single attempt, no retry — the next drain
		// cycle will pick it up again if it fails).
		if err := w.insertBatch(batch); err != nil {
			// Push it back to the front of the queue.
			w.overflowMu.Lock()
			w.overflow = append([][]*entity.ProctoringEvent{batch}, w.overflow...)
			w.overflowMu.Unlock()

			w.logger.Warn("clickhouse: overflow drain failed, will retry next cycle",
				zap.Int("remaining_overflow_batches", len(w.overflow)),
				zap.Error(err),
			)
			return // Stop draining — ClickHouse is still unhealthy.
		}

		w.totalFlushed.Add(int64(len(batch)))
		w.logger.Info("clickhouse: overflow batch recovered",
			zap.Int("event_count", len(batch)),
		)
	}
}

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

// applyWriterDefaults fills zero-valued config fields with production defaults.
func applyWriterDefaults(cfg *WriterConfig) {
	if len(cfg.Addrs) == 0 {
		cfg.Addrs = []string{"localhost:9000"}
	}
	if cfg.Database == "" {
		cfg.Database = "argus_analytics"
	}
	if cfg.Username == "" {
		cfg.Username = "default"
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 1000
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 5 * time.Second
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.OverflowQueueSize <= 0 {
		cfg.OverflowQueueSize = 50
	}
	if cfg.MaxOpenConns <= 0 {
		cfg.MaxOpenConns = 10
	}
	if cfg.MaxIdleConns <= 0 {
		cfg.MaxIdleConns = 5
	}
	if cfg.ConnMaxLifetime <= 0 {
		cfg.ConnMaxLifetime = 10 * time.Minute
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 5 * time.Second
	}
	if cfg.InsertTimeout <= 0 {
		cfg.InsertTimeout = 30 * time.Second
	}
}
