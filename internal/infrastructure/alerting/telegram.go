// Package alerting provides non-blocking emergency notification delivery
// for critical infrastructure failures (ClickHouse data loss, Kafka outages).
//
// The TelegramProvider sends Markdown-formatted alerts via the Telegram Bot API.
// Alerts are rate-limited (1 per component per minute) to prevent notification storms
// during sustained outages. Delivery is fully asynchronous — callers never block.
package alerting

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Provider interface
// ---------------------------------------------------------------------------

// Provider is a pluggable alerting backend. Implementations must be non-blocking
// and safe for concurrent use.
type Provider interface {
	// Send delivers a critical alert asynchronously. Must not block the caller.
	Send(alert Alert)

	// SendDirect sends a raw Markdown message synchronously, bypassing the alert
	// queue and rate limiter. Used for system lifecycle messages (startup, shutdown,
	// diagnostics). Errors are returned to the caller — never panics.
	SendDirect(text string) error

	// Close drains pending alerts and shuts down the provider.
	Close()
}

// Alert represents a structured emergency notification.
type Alert struct {
	Component string // e.g. "clickhouse-writer", "kafka-producer"
	SessionID string // Session context (empty if not applicable)
	Error     string // Error message or stack
	Severity  string // "CRITICAL", "WARNING"
	Timestamp time.Time
}

// ---------------------------------------------------------------------------
// Telegram Provider
// ---------------------------------------------------------------------------

// TelegramConfig holds Telegram Bot API credentials.
type TelegramConfig struct {
	BotToken string // From TELEGRAM_BOT_TOKEN env var
	ChatID   string // From TELEGRAM_CHAT_ID env var
}

// TelegramProvider delivers alerts via the Telegram Bot API.
// Alerts are queued in a buffered channel and sent by a background goroutine.
// Rate limiting prevents flooding during sustained outages.
type TelegramProvider struct {
	cfg    TelegramConfig
	logger *zap.Logger
	client *http.Client

	alertCh chan Alert
	done    chan struct{}
	wg      sync.WaitGroup

	// Rate limiting: track last alert time per component.
	rateMu   sync.Mutex
	lastSent map[string]time.Time
}

const (
	alertQueueSize   = 100
	rateLimitPerComp = 60 * time.Second // Max 1 alert per component per minute
	sendTimeout      = 10 * time.Second

	// defaultBotToken is intentionally empty — TELEGRAM_BOT_TOKEN env var is required.
	defaultBotToken = ""
)

// NewTelegramProvider creates a Telegram alerting provider.
// Returns nil if BotToken or ChatID is empty (alerting disabled).
// Both TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID environment variables are required.
func NewTelegramProvider(cfg TelegramConfig, logger *zap.Logger) *TelegramProvider {
	if cfg.BotToken == "" {
		cfg.BotToken = defaultBotToken
	}
	if cfg.BotToken == "" || cfg.ChatID == "" {
		logger.Info("telegram alerting disabled (missing TELEGRAM_BOT_TOKEN or TELEGRAM_CHAT_ID)")
		return nil
	}

	tp := &TelegramProvider{
		cfg:      cfg,
		logger:   logger.Named("telegram_alerter"),
		client:   &http.Client{Timeout: sendTimeout},
		alertCh:  make(chan Alert, alertQueueSize),
		done:     make(chan struct{}),
		lastSent: make(map[string]time.Time),
	}

	tp.wg.Add(1)
	go tp.drainLoop()

	logger.Info("telegram alerting enabled",
		zap.String("chat_id", cfg.ChatID),
	)

	return tp
}

// Send enqueues an alert for asynchronous delivery. Non-blocking: if the queue
// is full, the alert is dropped with a warning log.
func (tp *TelegramProvider) Send(alert Alert) {
	if alert.Timestamp.IsZero() {
		alert.Timestamp = time.Now()
	}

	// Rate limiting: suppress duplicate alerts for the same component.
	tp.rateMu.Lock()
	last, ok := tp.lastSent[alert.Component]
	if ok && time.Since(last) < rateLimitPerComp {
		tp.rateMu.Unlock()
		return // Suppressed (rate limited)
	}
	tp.lastSent[alert.Component] = time.Now()
	tp.rateMu.Unlock()

	select {
	case tp.alertCh <- alert:
	default:
		tp.logger.Warn("telegram: alert queue full, dropping alert",
			zap.String("component", alert.Component),
		)
	}
}

// Close drains pending alerts and stops the background sender.
func (tp *TelegramProvider) Close() {
	close(tp.done)
	tp.wg.Wait()
}

// SendDirect sends a raw Markdown message synchronously, bypassing the alert
// queue and rate limiter. Used for system lifecycle messages (startup hello,
// shutdown, diagnostics). Errors are returned — never panics.
func (tp *TelegramProvider) SendDirect(text string) error {
	payload := map[string]interface{}{
		"chat_id":    tp.cfg.ChatID,
		"text":       text,
		"parse_mode": "HTML",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram SendDirect: marshal: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", tp.cfg.BotToken)

	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram SendDirect: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := tp.client.Do(req)
	if err != nil {
		tp.logger.Warn("telegram SendDirect: send failed", zap.Error(err))
		return fmt.Errorf("telegram SendDirect: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		tp.logger.Warn("telegram SendDirect: non-200 response",
			zap.Int("status", resp.StatusCode),
		)
		return fmt.Errorf("telegram SendDirect: HTTP %d", resp.StatusCode)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Internal
// ---------------------------------------------------------------------------

func (tp *TelegramProvider) drainLoop() {
	defer tp.wg.Done()

	for {
		select {
		case alert := <-tp.alertCh:
			tp.deliver(alert)
		case <-tp.done:
			// Drain remaining alerts before exit.
			for {
				select {
				case alert := <-tp.alertCh:
					tp.deliver(alert)
				default:
					return
				}
			}
		}
	}
}

func (tp *TelegramProvider) deliver(alert Alert) {
	msg := formatTelegramMessage(alert)

	payload := map[string]interface{}{
		"chat_id":    tp.cfg.ChatID,
		"text":       msg,
		"parse_mode": "HTML",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		tp.logger.Error("telegram: failed to marshal payload", zap.Error(err))
		return
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", tp.cfg.BotToken)

	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		tp.logger.Error("telegram: failed to create request", zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := tp.client.Do(req)
	if err != nil {
		tp.logger.Warn("telegram: send failed", zap.Error(err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tp.logger.Warn("telegram: non-200 response",
			zap.Int("status", resp.StatusCode),
			zap.String("component", alert.Component),
		)
	}
}

func formatTelegramMessage(a Alert) string {
	return FormatAlertBilingual(a)
}

// shortID generates a short hex incident identifier (8 chars).
func shortID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%08x", time.Now().UnixNano()&0xFFFFFFFF)
	}
	return hex.EncodeToString(b)
}

// htmlEscape escapes HTML special characters for Telegram HTML parse mode.
func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
