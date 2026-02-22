// Package outbox implements the Transactional Outbox pattern for reliable
// job dispatch to Redis/asynq. Jobs are first persisted in PostgreSQL within
// the same database transaction as the business operation, then asynchronously
// relayed to Redis by a background goroutine.
//
// This prevents job loss during Redis OOM, network partitions, or process crashes.
// At-least-once delivery is guaranteed; asynq TaskID deduplication ensures
// idempotent processing on the consumer side.
package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

// Job represents a single outbox entry to be relayed to Redis/asynq.
type Job struct {
	ID          int64           `json:"id"`
	TaskType    string          `json:"task_type"`
	TaskID      string          `json:"task_id"`
	Payload     json.RawMessage `json:"payload"`
	Queue       string          `json:"queue"`
	MaxRetry    int             `json:"max_retry"`
	TimeoutSec  int             `json:"timeout_sec"`
	Status      string          `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	RetryCount  int             `json:"retry_count"`
}

// RelayConfig controls the outbox relay behaviour.
type RelayConfig struct {
	// PollInterval is how often the relay checks for pending jobs.
	// Default: 1 second.
	PollInterval time.Duration

	// BatchSize is the maximum number of jobs to relay per poll.
	// Default: 100.
	BatchSize int

	// MaxRetries is the maximum number of relay attempts before marking
	// a job as 'failed'. Default: 10.
	MaxRetries int
}

// DefaultRelayConfig returns sensible defaults for production.
func DefaultRelayConfig() RelayConfig {
	return RelayConfig{
		PollInterval: 1 * time.Second,
		BatchSize:    100,
		MaxRetries:   10,
	}
}

// Relay polls the outbox_jobs table for pending entries and dispatches
// them to Redis via asynq. It is safe for concurrent use but should
// typically run as a single instance per database to avoid duplicate
// dispatch (asynq TaskID dedup handles the rare race).
type Relay struct {
	db     *sql.DB
	client *asynq.Client
	cfg    RelayConfig
	logger *zap.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewRelay creates a new outbox relay. Call Start() to begin polling.
func NewRelay(db *sql.DB, client *asynq.Client, cfg RelayConfig, logger *zap.Logger) *Relay {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 1 * time.Second
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 100
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 10
	}

	return &Relay{
		db:     db,
		client: client,
		cfg:    cfg,
		logger: logger,
	}
}

// Enqueue persists a job to the outbox table within the given transaction.
// The job will be relayed to Redis asynchronously by the background relay.
//
// Usage:
//
//	tx, _ := db.BeginTx(ctx, nil)
//	outbox.Enqueue(ctx, tx, "job:video_export", "export:123", payload, "critical", 3, 1800)
//	tx.Commit()  // Job is now durable in PostgreSQL
func Enqueue(ctx context.Context, tx *sql.Tx, taskType, taskID string, payload json.RawMessage, queue string, maxRetry, timeoutSec int) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO outbox_jobs (task_type, task_id, payload, queue, max_retry, timeout_sec)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (task_id) DO NOTHING`,
		taskType, taskID, payload, queue, maxRetry, timeoutSec,
	)
	if err != nil {
		return fmt.Errorf("outbox: enqueue failed: %w", err)
	}
	return nil
}

// Start begins the background relay loop. It polls the outbox_jobs table
// for pending entries and dispatches them to Redis/asynq.
func (r *Relay) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.loop(ctx)
	}()

	r.logger.Info("outbox relay started",
		zap.Duration("poll_interval", r.cfg.PollInterval),
		zap.Int("batch_size", r.cfg.BatchSize),
	)
}

// Stop gracefully shuts down the relay loop.
func (r *Relay) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
	r.logger.Info("outbox relay stopped")
}

func (r *Relay) loop(ctx context.Context) {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.relayBatch(ctx); err != nil {
				r.logger.Error("outbox relay batch failed", zap.Error(err))
			}
		}
	}
}

func (r *Relay) relayBatch(ctx context.Context) error {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, task_type, task_id, payload, queue, max_retry, timeout_sec, retry_count
		 FROM outbox_jobs
		 WHERE status = 'pending'
		 ORDER BY created_at ASC
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`,
		r.cfg.BatchSize,
	)
	if err != nil {
		return fmt.Errorf("outbox: query pending jobs: %w", err)
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.TaskType, &j.TaskID, &j.Payload, &j.Queue, &j.MaxRetry, &j.TimeoutSec, &j.RetryCount); err != nil {
			return fmt.Errorf("outbox: scan job: %w", err)
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("outbox: rows iteration: %w", err)
	}

	for _, job := range jobs {
		if err := r.dispatchJob(ctx, job); err != nil {
			r.logger.Warn("outbox: dispatch failed, will retry",
				zap.String("task_id", job.TaskID),
				zap.Error(err),
			)
			r.markRetry(ctx, job)
			continue
		}
		r.markDispatched(ctx, job)
	}

	return nil
}

func (r *Relay) dispatchJob(ctx context.Context, job Job) error {
	task := asynq.NewTask(
		job.TaskType,
		job.Payload,
		asynq.MaxRetry(job.MaxRetry),
		asynq.Timeout(time.Duration(job.TimeoutSec)*time.Second),
		asynq.Queue(job.Queue),
		asynq.TaskID(job.TaskID),
	)

	_, err := r.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("asynq enqueue: %w", err)
	}
	return nil
}

func (r *Relay) markDispatched(ctx context.Context, job Job) {
	_, err := r.db.ExecContext(ctx,
		`UPDATE outbox_jobs SET status = 'dispatched', dispatched_at = NOW() WHERE id = $1`,
		job.ID,
	)
	if err != nil {
		r.logger.Error("outbox: mark dispatched failed",
			zap.Int64("id", job.ID),
			zap.Error(err),
		)
	}
}

func (r *Relay) markRetry(ctx context.Context, job Job) {
	newRetryCount := job.RetryCount + 1
	status := "pending"
	var errMsg *string

	if newRetryCount >= r.cfg.MaxRetries {
		status = "failed"
		msg := fmt.Sprintf("exceeded max relay retries (%d)", r.cfg.MaxRetries)
		errMsg = &msg
	}

	_, err := r.db.ExecContext(ctx,
		`UPDATE outbox_jobs SET status = $1, retry_count = $2, error_message = $3 WHERE id = $4`,
		status, newRetryCount, errMsg, job.ID,
	)
	if err != nil {
		r.logger.Error("outbox: mark retry failed",
			zap.Int64("id", job.ID),
			zap.Error(err),
		)
	}
}
