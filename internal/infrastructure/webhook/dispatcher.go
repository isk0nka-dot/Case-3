// Package webhook implements reliable webhook delivery with HMAC-SHA256
// payload signing and exponential backoff retry.
//
// Architecture follows the same pattern as the outbox relay
// (internal/infrastructure/outbox/relay.go): a background goroutine polls
// the webhook_deliveries table for pending entries and delivers them via HTTP.
//
// Retry schedule: 1s, 2s, 4s, 8s, 16s (5 attempts, exponential backoff).
// After max failures, the delivery is marked as 'failed' and the endpoint's
// consecutive_failures counter is incremented.
//
// HMAC-SHA256 signing allows partners to verify webhook authenticity:
//
//	X-Argus-Signature: sha256=<hex_digest>
//	X-Argus-Timestamp: <unix_seconds>
//	X-Argus-Event: <event_type>
package webhook

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
	adminHTTP "github.com/argus-ai/event-collector/internal/transport/http"
	"go.uber.org/zap"
)

// Config controls the webhook dispatcher behaviour.
type Config struct {
	// Enabled toggles the dispatcher. Default: true.
	Enabled bool `yaml:"enabled"`

	// PollInterval is how often the dispatcher checks for pending deliveries.
	// Default: 1 second.
	PollInterval time.Duration `yaml:"poll_interval"`

	// BatchSize is the maximum number of deliveries to process per poll.
	// Default: 50.
	BatchSize int `yaml:"batch_size"`

	// MaxRetries is the maximum delivery attempts before marking as failed.
	// Default: 5.
	MaxRetries int `yaml:"max_retries"`

	// TimeoutSec is the HTTP request timeout for each delivery attempt.
	// Default: 30 seconds.
	TimeoutSec int `yaml:"timeout_sec"`

	// MaxConcurrent limits parallel delivery goroutines.
	// Default: 10.
	MaxConcurrent int `yaml:"max_concurrent"`
}

// DefaultConfig returns sensible defaults for production.
func DefaultConfig() Config {
	return Config{
		Enabled:       true,
		PollInterval:  1 * time.Second,
		BatchSize:     50,
		MaxRetries:    5,
		TimeoutSec:    30,
		MaxConcurrent: 10,
	}
}

// Dispatcher polls the webhook_deliveries table and delivers payloads
// to registered endpoints with HMAC-SHA256 signatures.
type Dispatcher struct {
	repo       *postgres.Repository
	httpClient *http.Client
	cfg        Config
	logger     *zap.Logger
	sem        chan struct{} // Concurrency limiter.

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewDispatcher creates a new webhook dispatcher. Call Start() to begin polling.
func NewDispatcher(repo *postgres.Repository, cfg Config, logger *zap.Logger) *Dispatcher {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 1 * time.Second
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 50
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 5
	}
	if cfg.TimeoutSec == 0 {
		cfg.TimeoutSec = 30
	}
	if cfg.MaxConcurrent == 0 {
		cfg.MaxConcurrent = 10
	}

	return &Dispatcher{
		repo: repo,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.TimeoutSec) * time.Second,
		},
		cfg:    cfg,
		logger: logger.Named("webhook_dispatcher"),
		sem:    make(chan struct{}, cfg.MaxConcurrent),
	}
}

// Start begins the background polling loop.
func (d *Dispatcher) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.pollLoop(ctx)
	}()

	d.logger.Info("webhook dispatcher started",
		zap.Duration("poll_interval", d.cfg.PollInterval),
		zap.Int("batch_size", d.cfg.BatchSize),
		zap.Int("max_concurrent", d.cfg.MaxConcurrent),
	)
}

// Stop gracefully shuts down the dispatcher, waiting for in-flight deliveries.
func (d *Dispatcher) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
	d.wg.Wait()
	d.logger.Info("webhook dispatcher stopped")
}

func (d *Dispatcher) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.processBatch(ctx)
		}
	}
}

func (d *Dispatcher) processBatch(ctx context.Context) {
	deliveries, err := d.repo.GetPendingWebhookDeliveries(ctx, d.cfg.BatchSize)
	if err != nil {
		d.logger.Error("Failed to fetch pending deliveries", zap.Error(err))
		return
	}

	if len(deliveries) == 0 {
		return
	}

	d.logger.Debug("Processing webhook deliveries", zap.Int("count", len(deliveries)))

	for _, delivery := range deliveries {
		delivery := delivery // Capture for goroutine.

		// Acquire concurrency slot.
		select {
		case d.sem <- struct{}{}:
		case <-ctx.Done():
			return
		}

		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			defer func() { <-d.sem }() // Release slot.

			d.deliver(ctx, delivery)
		}()
	}
}

func (d *Dispatcher) deliver(ctx context.Context, delivery *entity.WebhookDelivery) {
	// The repository query stores URL and secret in ResponseBody/ErrorMessage
	// fields for pending deliveries (reusing empty fields to avoid extra struct).
	endpointURL := delivery.ResponseBody
	endpointSecret := delivery.ErrorMessage

	timestamp := time.Now().Unix()
	signature := adminHTTP.SignWebhookPayload(endpointSecret, timestamp, delivery.Payload)

	req, err := http.NewRequestWithContext(ctx, "POST", endpointURL, bytes.NewReader(delivery.Payload))
	if err != nil {
		d.handleFailure(ctx, delivery, 0, fmt.Sprintf("invalid URL: %v", err))
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ArgusAI-Webhook/1.0")
	req.Header.Set("X-Argus-Signature", fmt.Sprintf("sha256=%s", signature))
	req.Header.Set("X-Argus-Timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("X-Argus-Event", delivery.EventType)
	req.Header.Set("X-Argus-Delivery-Id", fmt.Sprintf("%d", delivery.ID))

	resp, err := d.httpClient.Do(req)
	if err != nil {
		d.handleFailure(ctx, delivery, 0, fmt.Sprintf("http error: %v", err))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096)) // Read up to 4KB of response.

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Success.
		if err := d.repo.MarkWebhookDelivered(ctx, delivery.ID, resp.StatusCode, string(body)); err != nil {
			d.logger.Error("Failed to mark delivery as delivered",
				zap.Error(err), zap.Int64("delivery_id", delivery.ID))
		}
		d.logger.Debug("Webhook delivered",
			zap.Int64("delivery_id", delivery.ID),
			zap.String("event_type", delivery.EventType),
			zap.Int("status", resp.StatusCode),
		)
	} else {
		d.handleFailure(ctx, delivery, resp.StatusCode, string(body))
	}
}

func (d *Dispatcher) handleFailure(ctx context.Context, delivery *entity.WebhookDelivery, httpStatus int, errorMsg string) {
	nextAttempt := delivery.Attempt + 1

	if nextAttempt >= delivery.MaxAttempts {
		// Max retries exceeded — mark as permanently failed.
		if err := d.repo.MarkWebhookFailed(ctx, delivery.ID, httpStatus, errorMsg, nil); err != nil {
			d.logger.Error("Failed to mark delivery as failed",
				zap.Error(err), zap.Int64("delivery_id", delivery.ID))
		}
		d.logger.Warn("Webhook delivery permanently failed",
			zap.Int64("delivery_id", delivery.ID),
			zap.String("event_type", delivery.EventType),
			zap.Int("attempts", nextAttempt),
			zap.String("error", truncate(errorMsg, 200)),
		)
		return
	}

	// Exponential backoff: 1s, 2s, 4s, 8s, 16s.
	backoff := time.Duration(math.Pow(2, float64(nextAttempt-1))) * time.Second
	nextRetry := time.Now().Add(backoff)

	if err := d.repo.MarkWebhookFailed(ctx, delivery.ID, httpStatus, errorMsg, &nextRetry); err != nil {
		d.logger.Error("Failed to schedule retry",
			zap.Error(err), zap.Int64("delivery_id", delivery.ID))
	}

	d.logger.Debug("Webhook delivery scheduled for retry",
		zap.Int64("delivery_id", delivery.ID),
		zap.Int("attempt", nextAttempt),
		zap.Duration("backoff", backoff),
	)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
