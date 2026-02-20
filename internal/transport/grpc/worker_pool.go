package grpc

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/application/usecase"
	"github.com/argus-ai/event-collector/internal/domain/entity"
)

// ---------------------------------------------------------------------------
//  Worker Pool
//
//  The Event Collector must sustain 10,000+ concurrent gRPC streams, each
//  producing events at up to 30 Hz (gaze telemetry). A naive 1-goroutine-
//  per-stream model would create unbounded concurrency on the ingestion
//  pipeline. The worker pool bounds concurrency to a fixed number of workers
//  that pull jobs from a shared buffered channel.
//
//  Architecture:
//
//    Stream 1 --+                          +-- Worker 1 --> IngestUseCase
//    Stream 2 --+--> [jobs channel] -------+-- Worker 2 --> IngestUseCase
//    Stream N --+    (buffered, bounded)   +-- Worker M --> IngestUseCase
//
//  Back-pressure: If the jobs channel is full, Submit blocks until either
//  a worker becomes available or the caller's context is cancelled. This
//  naturally applies back-pressure to streaming clients without dropping events.
// ---------------------------------------------------------------------------

// WorkerPool manages a fixed set of goroutines that process ingestion jobs.
// It decouples gRPC stream handling from the domain use case, preventing
// unbounded goroutine growth under high connection counts.
type WorkerPool struct {
	jobs   chan Job
	wg     sync.WaitGroup
	logger *zap.Logger
	size   int

	// done is closed when Close() is called to signal workers to drain and exit.
	done chan struct{}
}

// Job represents a single unit of work submitted to the pool.
// Each job carries one domain event and a channel for returning the result
// to the submitting goroutine (typically a gRPC stream handler).
type Job struct {
	// Event is the domain entity to be ingested.
	Event *entity.ProctoringEvent

	// ResultCh receives the ingestion result. The pool worker sends exactly
	// one value and never closes the channel — the caller is responsible
	// for reading the result and closing the channel if needed.
	ResultCh chan<- *usecase.IngestResult
}

// DefaultWorkerPoolSize is the default number of worker goroutines.
// Tuned for a machine with 8-16 cores handling 10K+ concurrent streams.
// Each worker performs CPU-light I/O (Kafka produce + ClickHouse insert),
// so over-subscribing relative to core count is safe.
const DefaultWorkerPoolSize = 256

// DefaultJobChannelBuffer is the default capacity of the jobs channel.
// A larger buffer absorbs traffic bursts without blocking stream handlers.
// Memory cost: ~256 bytes per Job struct * 8192 = ~2 MB.
const DefaultJobChannelBuffer = 8192

// NewWorkerPool creates a pool with the given number of workers and starts
// them immediately. Each worker reads from the shared jobs channel and
// processes events through the IngestUseCase.
//
// Parameters:
//   - size: Number of worker goroutines. Use DefaultWorkerPoolSize if unsure.
//   - ingest: The application-layer use case that owns ingestion logic.
//   - logger: Structured logger for operational visibility.
func NewWorkerPool(size int, ingest *usecase.IngestUseCase, logger *zap.Logger) *WorkerPool {
	if size <= 0 {
		size = DefaultWorkerPoolSize
	}

	wp := &WorkerPool{
		jobs:   make(chan Job, DefaultJobChannelBuffer),
		logger: logger.Named("worker_pool"),
		size:   size,
		done:   make(chan struct{}),
	}

	wp.wg.Add(size)
	for i := 0; i < size; i++ {
		go wp.worker(i, ingest)
	}

	wp.logger.Info("worker pool started",
		zap.Int("workers", size),
		zap.Int("job_buffer", DefaultJobChannelBuffer),
	)

	return wp
}

// worker is the main loop for a single pool worker. It reads jobs from the
// shared channel, processes them through the use case, and sends results
// back to the caller via the job's result channel.
//
// Workers exit when the jobs channel is closed (during graceful shutdown).
func (wp *WorkerPool) worker(id int, ingest *usecase.IngestUseCase) {
	defer wp.wg.Done()

	for job := range wp.jobs {
		// Create a background context for the ingestion call. The original
		// stream context may have been cancelled if the client disconnected,
		// but we still want to complete the write to Kafka/ClickHouse to
		// avoid data loss. If the caller has already gone away, the result
		// send below is a no-op (the channel may be unbuffered or closed).
		result := ingest.Ingest(context.Background(), job.Event)

		// Send result back to the stream handler. Use a non-blocking send
		// to avoid hanging if the stream handler has already exited.
		select {
		case job.ResultCh <- result:
		default:
			// The caller is gone. Log at debug level — this is expected
			// when clients disconnect mid-stream.
			wp.logger.Debug("result channel full or closed, discarding result",
				zap.String("event_id", result.EventID),
				zap.Int("worker_id", id),
			)
		}
	}

	wp.logger.Debug("worker exiting", zap.Int("worker_id", id))
}

// Submit enqueues a job for processing. It blocks until either:
//   - The job is accepted by a worker (via the channel).
//   - The context is cancelled (client disconnect, timeout, or shutdown).
//   - The pool is shutting down.
//
// Returns nil on successful submission. Returns the context error if the
// context expires before a worker picks up the job.
func (wp *WorkerPool) Submit(ctx context.Context, job Job) error {
	select {
	case wp.jobs <- job:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-wp.done:
		return context.Canceled
	}
}

// Pending returns the current number of jobs waiting in the queue.
// Useful for metrics and health checks.
func (wp *WorkerPool) Pending() int {
	return len(wp.jobs)
}

// Size returns the number of worker goroutines in the pool.
func (wp *WorkerPool) Size() int {
	return wp.size
}

// Close performs a graceful shutdown of the worker pool:
//  1. Signals that no new jobs should be accepted.
//  2. Closes the jobs channel so workers drain remaining work.
//  3. Waits for all workers to finish processing.
//
// After Close returns, Submit will return an error.
// Close is safe to call multiple times (subsequent calls are no-ops).
func (wp *WorkerPool) Close() {
	// Signal shutdown. The select prevents panic on double-close.
	select {
	case <-wp.done:
		return // Already closed.
	default:
		close(wp.done)
	}

	// Close the jobs channel so workers exit their range loop after
	// draining all remaining jobs.
	close(wp.jobs)

	// Wait for all in-flight jobs to complete.
	wp.wg.Wait()

	wp.logger.Info("worker pool stopped",
		zap.Int("workers", wp.size),
	)
}
