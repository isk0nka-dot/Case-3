// Package dlq provides a persistent dead-letter queue backed by BadgerDB for
// event pipeline resilience. When primary writers (Kafka) are unreachable,
// events are persisted locally and automatically reclaimed when the pipeline
// recovers.
//
// Design decisions:
//
//   - BadgerDB v4 (pure Go, no CGO) for embedded key-value persistence.
//     Crash-safe with SyncWrites=true. Suitable for millions of small events.
//
//   - FIFO ordering via lexicographically sortable keys:
//     "dlq/<20-digit-nanosecond-timestamp>/<event_id>"
//     BadgerDB iterates in key-sorted order, guaranteeing chronological drain.
//
//   - JSON serialization for events — matches the Kafka wire format, handles
//     []float32 face embeddings cleanly, and averages 2-4 KB per event.
//
//   - Background value log GC goroutine prevents unbounded disk growth.
//
//   - If a BadgerDB write fails, a CRITICAL Telegram alert fires immediately.
//     This is the last-resort integrity check — if even the DLQ is broken,
//     the operator must know within seconds.
package dlq

import (
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/alerting"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// StoreConfig holds BadgerDB DLQ configuration.
type StoreConfig struct {
	// DataDir is the filesystem path for BadgerDB data files.
	// Default: "/tmp/argus-dlq".
	DataDir string

	// GCInterval is the BadgerDB value log garbage collection cycle.
	// Default: 5 minutes.
	GCInterval time.Duration

	// GCDiscardRatio is the ratio of discardable data to trigger compaction.
	// Default: 0.5. Lower values compact more aggressively.
	GCDiscardRatio float64

	// MaxEntries is a safety cap on DLQ size to prevent unbounded disk usage.
	// Save() returns an error (without alerting) when this limit is reached.
	// Default: 1,000,000.
	MaxEntries int64

	// SyncWrites enables fsync on every write for crash safety.
	// Default: true. Set to false only for testing.
	SyncWrites bool
}

// applyStoreDefaults fills zero-valued fields with production-safe defaults.
func applyStoreDefaults(cfg *StoreConfig) {
	if cfg.DataDir == "" {
		cfg.DataDir = "/tmp/argus-dlq"
	}
	if cfg.GCInterval == 0 {
		cfg.GCInterval = 5 * time.Minute
	}
	if cfg.GCDiscardRatio == 0 {
		cfg.GCDiscardRatio = 0.5
	}
	if cfg.MaxEntries == 0 {
		cfg.MaxEntries = 1_000_000
	}
}

// ---------------------------------------------------------------------------
// DLQ Key prefix
// ---------------------------------------------------------------------------

const dlqKeyPrefix = "dlq/"

// makeKey creates a FIFO-ordered key: "dlq/<20-digit-nanosec>/<event_id>".
func makeKey(eventID string) []byte {
	return []byte(fmt.Sprintf("%s%020d/%s", dlqKeyPrefix, time.Now().UnixNano(), eventID))
}

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

// StoreMetrics holds atomic counter snapshots for monitoring.
type StoreMetrics struct {
	TotalSaved     int64
	TotalReclaimed int64
	TotalFailed    int64
	CurrentSize    int64
}

// Store wraps BadgerDB for DLQ persistence. It is safe for concurrent use.
type Store struct {
	db      *badger.DB
	logger  *zap.Logger
	alerter alerting.Provider
	cfg     StoreConfig

	done chan struct{}

	// Lock-free metrics.
	totalSaved     atomic.Int64
	totalReclaimed atomic.Int64
	totalFailed    atomic.Int64
	currentSize    atomic.Int64
}

// NewStore creates a new BadgerDB-backed DLQ store.
// The data directory is created if it does not exist.
// A background value log GC goroutine is started automatically.
func NewStore(cfg StoreConfig, logger *zap.Logger) (*Store, error) {
	applyStoreDefaults(&cfg)

	// Ensure data directory exists.
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return nil, fmt.Errorf("dlq: failed to create data dir %s: %w", cfg.DataDir, err)
	}

	opts := badger.DefaultOptions(cfg.DataDir).
		WithSyncWrites(cfg.SyncWrites).
		WithLogger(nil) // Suppress BadgerDB's built-in logger; we use zap.

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("dlq: failed to open badger db at %s: %w", cfg.DataDir, err)
	}

	s := &Store{
		db:     db,
		logger: logger.Named("dlq_store"),
		cfg:    cfg,
		done:   make(chan struct{}),
	}

	// Count existing entries (recovery after crash).
	// If recount fails, the DLQ would appear empty and silently drop
	// previously queued events. Fail startup loudly instead.
	if err := s.recount(); err != nil {
		db.Close()
		return nil, fmt.Errorf("dlq: startup recount failed (possible corruption): %w", err)
	}

	// Start background value log GC.
	go s.gcLoop()

	s.logger.Info("dlq store opened",
		zap.String("data_dir", cfg.DataDir),
		zap.Int64("existing_entries", s.currentSize.Load()),
		zap.Bool("sync_writes", cfg.SyncWrites),
	)

	return s, nil
}

// SetAlerter wires the Telegram emergency alerter for CRITICAL DLQ failures.
func (s *Store) SetAlerter(a alerting.Provider) {
	s.alerter = a
}

// Save persists a single event to BadgerDB.
// If the DLQ size exceeds MaxEntries, the event is rejected (not alerted).
// If BadgerDB itself fails, a CRITICAL Telegram alert fires.
func (s *Store) Save(event *entity.ProctoringEvent) error {
	// Safety cap — prevent unbounded disk growth.
	if s.currentSize.Load() >= s.cfg.MaxEntries {
		s.logger.Warn("dlq: max entries reached, rejecting event",
			zap.String("event_id", event.EventID),
			zap.Int64("current_size", s.currentSize.Load()),
			zap.Int64("max_entries", s.cfg.MaxEntries),
		)
		return fmt.Errorf("dlq: max entries reached (%d)", s.cfg.MaxEntries)
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("dlq: marshal event %s: %w", event.EventID, err)
	}

	key := makeKey(event.EventID)

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		s.totalFailed.Add(1)
		s.fireCriticalAlert(fmt.Sprintf("BadgerDB write failed: %v", err))
		return fmt.Errorf("dlq: badger write failed: %w", err)
	}

	s.totalSaved.Add(1)
	s.currentSize.Add(1)
	return nil
}

// SaveBatch persists multiple events in a single BadgerDB transaction.
func (s *Store) SaveBatch(events []*entity.ProctoringEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Safety cap check.
	if s.currentSize.Load()+int64(len(events)) > s.cfg.MaxEntries {
		s.logger.Warn("dlq: batch would exceed max entries, rejecting",
			zap.Int("batch_size", len(events)),
			zap.Int64("current_size", s.currentSize.Load()),
		)
		return fmt.Errorf("dlq: batch would exceed max entries (%d)", s.cfg.MaxEntries)
	}

	wb := s.db.NewWriteBatch()
	defer wb.Cancel()

	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("dlq: marshal event %s: %w", event.EventID, err)
		}
		key := makeKey(event.EventID)
		if err := wb.Set(key, data); err != nil {
			s.totalFailed.Add(1)
			s.fireCriticalAlert(fmt.Sprintf("BadgerDB batch set failed: %v", err))
			return fmt.Errorf("dlq: badger batch set failed: %w", err)
		}
	}

	if err := wb.Flush(); err != nil {
		s.totalFailed.Add(int64(len(events)))
		s.fireCriticalAlert(fmt.Sprintf("BadgerDB batch flush failed: %v", err))
		return fmt.Errorf("dlq: badger batch flush failed: %w", err)
	}

	s.totalSaved.Add(int64(len(events)))
	s.currentSize.Add(int64(len(events)))
	return nil
}

// DrainBatch reads up to `limit` events from BadgerDB in FIFO order.
// Returns the deserialized events and their raw keys. The caller must call
// DeleteKeys() after successfully writing the events to the primary pipeline.
func (s *Store) DrainBatch(limit int) ([]*entity.ProctoringEvent, [][]byte, error) {
	events := make([]*entity.ProctoringEvent, 0, limit)
	keys := make([][]byte, 0, limit)

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = limit
		opts.Prefix = []byte(dlqKeyPrefix)

		it := txn.NewIterator(opts)
		defer it.Close()

		count := 0
		for it.Seek([]byte(dlqKeyPrefix)); it.Valid() && count < limit; it.Next() {
			item := it.Item()

			// Copy key (iterator reuses buffer).
			keyCopy := make([]byte, len(item.Key()))
			copy(keyCopy, item.Key())

			err := item.Value(func(val []byte) error {
				var event entity.ProctoringEvent
				if err := json.Unmarshal(val, &event); err != nil {
					// Log with raw bytes for manual recovery — do NOT delete
					// the entry. It stays in the DLQ and will alert on every
					// drain cycle until an operator inspects and clears it.
					s.logger.Error("dlq: corrupt entry, leaving in queue for manual inspection",
						zap.String("key", string(keyCopy)),
						zap.ByteString("raw_bytes", val),
						zap.Error(err),
					)
					return nil
				}
				events = append(events, &event)
				keys = append(keys, keyCopy)
				return nil
			})
			if err != nil {
				return err
			}
			count++
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("dlq: drain batch failed: %w", err)
	}

	return events, keys, nil
}

// DeleteKeys removes successfully reclaimed entries from BadgerDB.
// This should be called after the events have been confirmed written to Kafka.
func (s *Store) DeleteKeys(keys [][]byte) error {
	if len(keys) == 0 {
		return nil
	}

	wb := s.db.NewWriteBatch()
	defer wb.Cancel()

	for _, key := range keys {
		if err := wb.Delete(key); err != nil {
			return fmt.Errorf("dlq: delete key failed: %w", err)
		}
	}

	if err := wb.Flush(); err != nil {
		return fmt.Errorf("dlq: delete batch flush failed: %w", err)
	}

	deleted := int64(len(keys))
	s.totalReclaimed.Add(deleted)
	s.currentSize.Add(-deleted)
	return nil
}

// Size returns the current number of entries in the DLQ.
func (s *Store) Size() int64 {
	return s.currentSize.Load()
}

// Metrics returns a snapshot of DLQ store statistics.
func (s *Store) Metrics() StoreMetrics {
	return StoreMetrics{
		TotalSaved:     s.totalSaved.Load(),
		TotalReclaimed: s.totalReclaimed.Load(),
		TotalFailed:    s.totalFailed.Load(),
		CurrentSize:    s.currentSize.Load(),
	}
}

// Clear removes ALL entries from the DLQ. This is a destructive operation
// intended for admin panic-button recovery when the DLQ contains stuck or
// corrupt entries that prevent normal reclamation.
//
// This is idempotent — calling Clear() on an empty DLQ is a no-op.
func (s *Store) Clear() error {
	entriesCleared := s.currentSize.Load()

	s.logger.Warn("DLQ cleared by admin",
		zap.Int64("entries_cleared", entriesCleared),
	)

	err := s.db.DropAll()
	if err != nil {
		return fmt.Errorf("dlq: clear failed: %w", err)
	}

	s.currentSize.Store(0)
	return nil
}

// Close stops the GC goroutine and closes BadgerDB.
func (s *Store) Close() error {
	close(s.done)

	// Run a final GC pass before closing.
	_ = s.db.RunValueLogGC(s.cfg.GCDiscardRatio)

	if err := s.db.Close(); err != nil {
		return fmt.Errorf("dlq: close badger db: %w", err)
	}

	s.logger.Info("dlq store closed",
		zap.Int64("remaining_entries", s.currentSize.Load()),
		zap.Int64("total_saved", s.totalSaved.Load()),
		zap.Int64("total_reclaimed", s.totalReclaimed.Load()),
	)
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// recount scans BadgerDB to count existing entries on startup (crash recovery).
// Returns an error if the scan fails — callers must decide whether to fail
// startup or proceed with a potentially stale count.
func (s *Store) recount() error {
	var count int64
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Keys only — fast.
		opts.Prefix = []byte(dlqKeyPrefix)

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek([]byte(dlqKeyPrefix)); it.Valid(); it.Next() {
			count++
		}
		return nil
	})
	if err != nil {
		s.logger.Error("dlq: recount scan failed — DLQ size may be inaccurate",
			zap.Error(err),
		)
		return fmt.Errorf("dlq: recount failed: %w", err)
	}
	s.currentSize.Store(count)
	return nil
}

// gcLoop periodically runs BadgerDB value log garbage collection.
func (s *Store) gcLoop() {
	ticker := time.NewTicker(s.cfg.GCInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// RunValueLogGC returns ErrNoRewrite when nothing to collect.
			for {
				err := s.db.RunValueLogGC(s.cfg.GCDiscardRatio)
				if err != nil {
					break // No more GC work to do.
				}
			}
		case <-s.done:
			return
		}
	}
}

// fireCriticalAlert sends a CRITICAL Telegram alert for DLQ write failures.
func (s *Store) fireCriticalAlert(detail string) {
	s.logger.Error("CRITICAL: dlq write failure", zap.String("detail", detail))

	if s.alerter == nil {
		return
	}

	msg := alerting.MsgDLQEmergency(
		time.Now().Format("2006-01-02 15:04:05 MST"),
		detail,
	)

	if err := s.alerter.SendDirect(msg); err != nil {
		s.logger.Error("dlq: failed to send critical alert", zap.Error(err))
	}
}
