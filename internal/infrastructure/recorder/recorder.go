// Package recorder orchestrates the evidence capture pipeline:
// ring buffer → snapshot → concatenate → SHA-256 → MinIO upload → chain of custody.
//
// When a critical violation event is detected, the recorder:
//  1. Snapshots the session's ring buffer (30s before + 10s after).
//  2. Concatenates the video segments into a single WebM stream.
//  3. Uploads to MinIO via the EvidenceStore port (SHA-256 computed during upload).
//  4. Records the chain of custody entry via EvidenceChainWriter.
//
// Design principles:
//   - Async & non-blocking: evidence capture never blocks event ingestion.
//   - Bounded concurrency: semaphore limits concurrent uploads (default: 10).
//   - Fail-soft: upload failures are logged but never cause event rejection.
//   - Memory-efficient: ring buffers use ~3MB per active session at 480p.
package recorder

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/application/port"
	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/ringbuffer"
)

// Config holds configuration for the evidence recorder.
type Config struct {
	// WindowSec is the total ring buffer window size in seconds.
	// Default: 60 (maintains 60 seconds of rolling video).
	WindowSec int

	// PreCaptureSec is the number of seconds before the violation event
	// to include in the evidence clip. Default: 30.
	PreCaptureSec float64

	// PostCaptureSec is the number of seconds after the violation event
	// to include in the evidence clip. Default: 10.
	PostCaptureSec float64

	// MaxConcurrentUploads limits the number of concurrent evidence uploads.
	// Default: 10.
	MaxConcurrentUploads int
}

// DefaultConfig returns production-ready default configuration.
func DefaultConfig() Config {
	return Config{
		WindowSec:            60,
		PreCaptureSec:        30,
		PostCaptureSec:       10,
		MaxConcurrentUploads: 10,
	}
}

// Recorder manages per-session ring buffers and orchestrates evidence capture.
// It is safe for concurrent use by multiple goroutines.
type Recorder struct {
	mu       sync.RWMutex
	buffers  map[string]*ringbuffer.Buffer // sessionID → ring buffer
	store    port.EvidenceStore
	chain    port.EvidenceChainWriter
	config   Config
	logger   *zap.Logger
	uploadCh chan struct{} // semaphore for bounded concurrency
}

// New creates a new evidence recorder with the given dependencies.
// Pass nil for store or chain to disable evidence capture (noop mode).
func New(store port.EvidenceStore, chain port.EvidenceChainWriter, cfg Config, logger *zap.Logger) *Recorder {
	if cfg.WindowSec == 0 {
		cfg = DefaultConfig()
	}
	if cfg.MaxConcurrentUploads < 1 {
		cfg.MaxConcurrentUploads = 10
	}

	return &Recorder{
		buffers:  make(map[string]*ringbuffer.Buffer),
		store:    store,
		chain:    chain,
		config:   cfg,
		logger:   logger.Named("recorder"),
		uploadCh: make(chan struct{}, cfg.MaxConcurrentUploads),
	}
}

// StartSession allocates a ring buffer for the given session.
// Call this when a new proctoring session connects.
func (r *Recorder) StartSession(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.buffers[sessionID]; exists {
		return // Already started.
	}

	r.buffers[sessionID] = ringbuffer.New(r.config.WindowSec)
	r.logger.Debug("ring buffer allocated",
		zap.String("session_id", sessionID),
		zap.Int("capacity", r.config.WindowSec),
	)
}

// StopSession releases the ring buffer for the given session.
// Call this when a proctoring session disconnects.
func (r *Recorder) StopSession(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if buf, exists := r.buffers[sessionID]; exists {
		buf.Reset()
		delete(r.buffers, sessionID)
		r.logger.Debug("ring buffer released",
			zap.String("session_id", sessionID),
		)
	}
}

// PushSegment feeds a video segment into the session's ring buffer.
// This is called by the WebRTC media pipeline for each ~1s video chunk.
func (r *Recorder) PushSegment(sessionID string, data []byte, timestamp time.Time) {
	r.mu.RLock()
	buf, exists := r.buffers[sessionID]
	r.mu.RUnlock()

	if !exists {
		// Auto-start: create buffer on first segment push.
		r.StartSession(sessionID)
		r.mu.RLock()
		buf = r.buffers[sessionID]
		r.mu.RUnlock()
	}

	buf.Push(data, timestamp, 1*time.Second)
}

// TriggerCapture initiates async evidence capture for a violation event.
// This method is non-blocking — it spawns a goroutine with bounded concurrency.
//
// The capture pipeline:
//  1. Acquire semaphore slot (bounded concurrency).
//  2. Snapshot ring buffer (pre-capture + post-capture window).
//  3. Concatenate segments into single byte stream.
//  4. Upload to MinIO (SHA-256 computed during upload).
//  5. Record chain of custody entry.
func (r *Recorder) TriggerCapture(ctx context.Context, event *entity.ProctoringEvent) {
	if r.store == nil || r.chain == nil {
		return // Evidence capture disabled.
	}

	go func() {
		// Acquire semaphore slot.
		select {
		case r.uploadCh <- struct{}{}:
			defer func() { <-r.uploadCh }()
		case <-ctx.Done():
			r.logger.Warn("evidence capture cancelled — context done",
				zap.String("event_id", event.EventID),
			)
			return
		default:
			r.logger.Warn("evidence capture skipped — upload concurrency limit reached",
				zap.String("event_id", event.EventID),
				zap.Int("max_concurrent", r.config.MaxConcurrentUploads),
			)
			return
		}

		if err := r.capture(ctx, event); err != nil {
			r.logger.Error("evidence capture failed",
				zap.String("event_id", event.EventID),
				zap.String("session_id", event.SessionID),
				zap.Error(err),
			)
		}
	}()
}

// capture performs the synchronous evidence capture pipeline.
func (r *Recorder) capture(ctx context.Context, event *entity.ProctoringEvent) error {
	// Step 1: Snapshot the ring buffer.
	r.mu.RLock()
	buf, exists := r.buffers[event.SessionID]
	r.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no ring buffer for session %s", event.SessionID)
	}

	segments := buf.Snapshot(
		event.ServerTimestamp,
		r.config.PreCaptureSec,
		r.config.PostCaptureSec,
	)

	if len(segments) == 0 {
		r.logger.Warn("no video segments in capture window",
			zap.String("event_id", event.EventID),
			zap.String("session_id", event.SessionID),
		)
		return nil // Not an error — buffer may not have data yet.
	}

	// Step 2: Concatenate segments into a single byte stream.
	var totalSize int
	for _, seg := range segments {
		totalSize += len(seg.Data)
	}

	combined := bytes.NewBuffer(make([]byte, 0, totalSize))
	for _, seg := range segments {
		combined.Write(seg.Data)
	}

	// Compute time window.
	startTime := segments[0].Timestamp
	endTime := segments[len(segments)-1].Timestamp
	durationSec := endTime.Sub(startTime).Seconds()

	// Step 3: Build evidence fragment entity.
	fragmentID := fmt.Sprintf("ev-%s-%d", event.EventID[:8], time.Now().UnixMilli())
	fragment := &entity.EvidenceFragment{
		FragmentID:  fragmentID,
		SessionID:   event.SessionID,
		EventID:     event.EventID,
		OrgID:       event.OrgID,
		ExamID:      event.ExamID,
		StudentID:   event.StudentID,
		ContentType: "video/webm",
		DurationSec: durationSec,
		StartTime:   startTime,
		EndTime:     endTime,
		CreatedAt:   time.Now(),
	}

	// Step 4: Upload to MinIO (SHA-256 computed during upload via TeeReader).
	sha256Hash, uri, sizeBytes, err := r.store.Upload(ctx, fragment, combined)
	if err != nil {
		return fmt.Errorf("evidence upload failed: %w", err)
	}

	// Fill in post-upload fields.
	fragment.SHA256Hash = sha256Hash
	fragment.URI = uri
	fragment.SizeBytes = sizeBytes

	// Step 5: Record chain of custody entry.
	if err := r.chain.RecordEvidence(ctx, fragment); err != nil {
		r.logger.Error("chain of custody write failed (evidence is uploaded but unlinked)",
			zap.String("fragment_id", fragmentID),
			zap.String("sha256", sha256Hash),
			zap.Error(err),
		)
		return fmt.Errorf("chain of custody write failed: %w", err)
	}

	r.logger.Info("evidence fragment captured",
		zap.String("fragment_id", fragmentID),
		zap.String("session_id", event.SessionID),
		zap.String("event_id", event.EventID),
		zap.String("sha256", sha256Hash),
		zap.Int64("size_bytes", sizeBytes),
		zap.Float64("duration_sec", durationSec),
		zap.Int("segments", len(segments)),
	)

	return nil
}

// ActiveSessions returns the number of sessions with active ring buffers.
func (r *Recorder) ActiveSessions() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.buffers)
}

// Close releases all ring buffers. Should be called during graceful shutdown.
func (r *Recorder) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, buf := range r.buffers {
		buf.Reset()
		delete(r.buffers, id)
	}

	r.logger.Info("recorder closed, all ring buffers released")
}
