// Package shutdown provides a graceful shutdown orchestrator for the event-collector
// service. It listens for OS termination signals (SIGINT, SIGTERM) and coordinates
// the orderly teardown of all service components.
//
// In a microservice that bridges real-time event ingestion (gRPC), stream processing
// (Kafka), and analytical storage (ClickHouse), shutdown order matters critically:
//
//  1. Stop accepting new requests (gRPC server).
//  2. Drain in-flight requests and flush buffers.
//  3. Close downstream connections (Kafka producer, ClickHouse writer).
//  4. Release infrastructure resources (health checks, metrics).
//
// The orchestrator uses a priority-based hook system where lower priority numbers
// execute first. Hooks at the same priority level execute sequentially in registration
// order. Each hook is given a context derived from the global shutdown timeout — if a
// hook exceeds its budget, the context is cancelled and the orchestrator moves on.
//
// Design decisions:
//   - Priority-based ordering over simple LIFO/FIFO for explicit control over
//     shutdown sequencing across independently registered components.
//   - Single global timeout (not per-hook) to bound total shutdown duration.
//     Kubernetes sends SIGKILL after terminationGracePeriodSeconds, so the total
//     shutdown must complete well within that window.
//   - Hooks run sequentially, not concurrently, to avoid race conditions during
//     teardown (e.g., flushing Kafka before closing the connection).
//   - Errors from individual hooks are logged but do not abort the remaining hooks.
//     Best-effort cleanup is preferred over all-or-nothing.
package shutdown

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// ShutdownHook represents a single cleanup action to perform during shutdown.
type ShutdownHook struct {
	// Name is a human-readable identifier for this hook, used in log messages.
	// Should describe the resource being cleaned up (e.g., "kafka-producer",
	// "clickhouse-writer", "grpc-server").
	Name string

	// Priority controls execution order during shutdown. Hooks with lower
	// priority values execute first. Typical priority scheme:
	//   0-9:   Stop accepting new work (gRPC server, HTTP listener).
	//   10-19: Drain in-flight work and flush buffers.
	//   20-29: Close downstream connections (Kafka, ClickHouse).
	//   30-39: Close infrastructure (metrics, health checks).
	//   40+:   Final cleanup (temp files, PID files).
	Priority int

	// Fn is the cleanup function to execute. It receives a context that will be
	// cancelled if the global shutdown timeout is reached. Implementations should
	// respect context cancellation and return promptly when ctx.Done() fires.
	//
	// Returning an error does not abort remaining hooks — it is logged and the
	// orchestrator proceeds to the next hook.
	Fn func(ctx context.Context) error
}

// GracefulShutdown orchestrates the orderly teardown of service components.
// Components register cleanup hooks via Register(), and the orchestrator executes
// them in priority order when a termination signal is received.
type GracefulShutdown struct {
	logger  *zap.Logger
	timeout time.Duration
	hooks   []ShutdownHook
	mu      sync.Mutex
}

// New creates a new GracefulShutdown orchestrator.
//
// The timeout parameter bounds the total time allowed for all hooks to complete.
// This should be set to a value less than the Kubernetes terminationGracePeriodSeconds
// (typically 30s) to leave a buffer for the OS to perform its own cleanup. A typical
// value is 25 seconds for a 30-second termination grace period.
func New(timeout time.Duration, logger *zap.Logger) *GracefulShutdown {
	return &GracefulShutdown{
		logger:  logger.Named("graceful_shutdown"),
		timeout: timeout,
		hooks:   make([]ShutdownHook, 0, 8),
	}
}

// Register adds a shutdown hook to the orchestrator. Hooks are executed in
// ascending priority order during shutdown. Multiple hooks with the same
// priority are executed in the order they were registered.
//
// Register is safe to call from multiple goroutines, but should typically be
// called during service initialization before Wait() is invoked.
//
// Example:
//
//	gs.Register("grpc-server", 0, func(ctx context.Context) error {
//	    server.GracefulStop()
//	    return nil
//	})
//	gs.Register("kafka-producer", 20, func(ctx context.Context) error {
//	    return producer.Close()
//	})
func (gs *GracefulShutdown) Register(name string, priority int, fn func(ctx context.Context) error) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	gs.hooks = append(gs.hooks, ShutdownHook{
		Name:     name,
		Priority: priority,
		Fn:       fn,
	})

	gs.logger.Debug("shutdown hook registered",
		zap.String("hook", name),
		zap.Int("priority", priority),
	)
}

// Wait blocks the calling goroutine until either SIGINT or SIGTERM is received,
// then executes all registered hooks in priority order within the configured timeout.
//
// This should be called on the main goroutine after all service components have
// been initialized and their shutdown hooks registered.
//
// Wait installs its own signal handler and restores default signal behavior after
// the first signal is received. A second signal during shutdown will trigger an
// immediate, ungraceful exit.
func (gs *GracefulShutdown) Wait() {
	// Create a buffered channel so the signal is not missed if we are not yet
	// ready to receive when it arrives.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Block until a termination signal is received.
	sig := <-sigChan

	gs.logger.Info("received termination signal, initiating graceful shutdown",
		zap.String("signal", sig.String()),
		zap.Duration("timeout", gs.timeout),
		zap.Int("hook_count", len(gs.hooks)),
	)

	// Stop catching signals — a second signal during shutdown will trigger
	// the default behavior (immediate termination).
	signal.Stop(sigChan)

	// Execute shutdown hooks with a bounded timeout.
	ctx, cancel := context.WithTimeout(context.Background(), gs.timeout)
	defer cancel()

	if err := gs.executeHooks(ctx); err != nil {
		gs.logger.Error("graceful shutdown completed with errors", zap.Error(err))
	} else {
		gs.logger.Info("graceful shutdown completed successfully")
	}
}

// Shutdown manually triggers the shutdown sequence. This is primarily intended
// for testing, but can also be used for programmatic shutdown (e.g., in response
// to a fatal health check failure).
//
// The provided context controls the overall timeout. If ctx has no deadline,
// the configured timeout is used as a fallback.
func (gs *GracefulShutdown) Shutdown(ctx context.Context) error {
	gs.logger.Info("manual shutdown initiated",
		zap.Int("hook_count", len(gs.hooks)),
	)

	// If the caller's context has no deadline, apply our own timeout.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, gs.timeout)
		defer cancel()
	}

	return gs.executeHooks(ctx)
}

// executeHooks runs all registered hooks in priority order (ascending).
// Hooks at the same priority level run in registration order.
//
// Each hook receives the same parent context. If the context deadline is reached
// during execution, remaining hooks still get a chance to run with an already-
// cancelled context — well-behaved hooks should check ctx.Done() and exit quickly.
//
// Errors from individual hooks are accumulated and returned as a combined error,
// but do NOT prevent subsequent hooks from executing.
func (gs *GracefulShutdown) executeHooks(ctx context.Context) error {
	gs.mu.Lock()
	// Copy and sort hooks to avoid holding the lock during execution.
	hooks := make([]ShutdownHook, len(gs.hooks))
	copy(hooks, gs.hooks)
	gs.mu.Unlock()

	// Sort by priority (ascending). Stable sort preserves registration order
	// for hooks with equal priority.
	sort.SliceStable(hooks, func(i, j int) bool {
		return hooks[i].Priority < hooks[j].Priority
	})

	var errs []error

	for _, hook := range hooks {
		// Check if the overall deadline has already been exceeded.
		select {
		case <-ctx.Done():
			gs.logger.Warn("shutdown timeout exceeded, remaining hooks will run with cancelled context",
				zap.String("hook", hook.Name),
				zap.Int("priority", hook.Priority),
			)
		default:
		}

		gs.logger.Info("executing shutdown hook",
			zap.String("hook", hook.Name),
			zap.Int("priority", hook.Priority),
		)

		start := time.Now()
		err := hook.Fn(ctx)
		elapsed := time.Since(start)

		if err != nil {
			gs.logger.Error("shutdown hook failed",
				zap.String("hook", hook.Name),
				zap.Int("priority", hook.Priority),
				zap.Duration("elapsed", elapsed),
				zap.Error(err),
			)
			errs = append(errs, fmt.Errorf("hook %q (priority %d): %w", hook.Name, hook.Priority, err))
		} else {
			gs.logger.Info("shutdown hook completed",
				zap.String("hook", hook.Name),
				zap.Int("priority", hook.Priority),
				zap.Duration("elapsed", elapsed),
			)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown completed with %d error(s): %v", len(errs), errs)
	}

	return nil
}

// HookCount returns the number of registered shutdown hooks.
// This is useful for testing and health check endpoints.
func (gs *GracefulShutdown) HookCount() int {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	return len(gs.hooks)
}
