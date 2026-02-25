// Package clickhouse — Backfiller: Kafka consumer group that replays events
// from Kafka into ClickHouse.
//
// ADR-007: Edge Persistence & Replay Policy
//
// The backfiller is a Kafka consumer group (`argus-clickhouse-backfiller`)
// that reads from all three proctoring event topics and inserts batches
// directly into the ClickHouse `proctoring_events` table.
//
// It is idempotent: ClickHouse's ReplacingMergeTree engine deduplicates
// events on (org_id, exam_id, session_id, event_id), making at-least-once
// delivery safe. Events that already exist in ClickHouse are silently
// merged away during background part merges.
//
// Usage:
//
//	bf, err := clickhouse.NewBackfiller(chWriter, cfg, logger)
//	bf.Start()
//	defer bf.Close()
package clickhouse

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/domain/valueobject"
	"github.com/argus-ai/event-collector/internal/infrastructure/alerting"
	"github.com/argus-ai/event-collector/pkg/circuitbreaker"
)

// Prometheus gauge for consumer lag per topic/partition.
// Updated on every message consumption — measures how far behind
// the consumer is from the high-water mark.
var backfillerConsumerLag = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "argus_backfiller_consumer_lag",
		Help: "Consumer lag (high-water mark minus current offset) per topic/partition.",
	},
	[]string{"topic", "partition"},
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// BackfillerConfig holds Kafka-to-ClickHouse backfiller configuration.
type BackfillerConfig struct {
	// Brokers is the Kafka bootstrap server list.
	Brokers []string `yaml:"brokers"`

	// Topics to consume from. Default: all three proctoring event topics.
	Topics []string `yaml:"topics"`

	// GroupID is the Kafka consumer group name. Default: "argus-clickhouse-backfiller".
	GroupID string `yaml:"group_id"`

	// BatchSize is the number of events to accumulate before flushing to ClickHouse.
	// Default: 500.
	BatchSize int `yaml:"batch_size"`

	// FlushInterval is the maximum time between batch flushes. Default: 5s.
	FlushInterval time.Duration `yaml:"flush_interval"`

	// PauseOnError is the sleep duration after a ClickHouse insert failure.
	// Offsets are NOT committed — Kafka will redeliver on next fetch.
	// Default: 10s.
	PauseOnError time.Duration `yaml:"pause_on_error"`
}

// applyBackfillerDefaults fills zero-valued fields with production defaults.
func applyBackfillerDefaults(cfg *BackfillerConfig) {
	if len(cfg.Topics) == 0 {
		cfg.Topics = []string{
			"argus.events.critical",
			"argus.events.standard",
			"argus.events.telemetry",
		}
	}
	if cfg.GroupID == "" {
		cfg.GroupID = "argus-clickhouse-backfiller"
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 500
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 5 * time.Second
	}
	if cfg.PauseOnError <= 0 {
		cfg.PauseOnError = 10 * time.Second
	}
}

// ---------------------------------------------------------------------------
// Backfiller
// ---------------------------------------------------------------------------

// Backfiller is a Kafka consumer group that replays proctoring events into
// ClickHouse. It implements sarama.ConsumerGroupHandler for the consume loop
// and port.HealthChecker for Kubernetes readiness probes.
type Backfiller struct {
	writer  *Writer
	breaker *circuitbreaker.Breaker
	alerter alerting.Provider
	logger  *zap.Logger
	cfg     BackfillerConfig
	group   sarama.ConsumerGroup

	done chan struct{}
	wg   sync.WaitGroup

	// Metrics (lock-free).
	totalConsumed atomic.Int64
	totalInserted atomic.Int64
	totalErrors   atomic.Int64
	isConsuming   atomic.Bool
}

// NewBackfiller creates a Kafka consumer group that replays events into
// ClickHouse via the given writer.
func NewBackfiller(writer *Writer, cfg BackfillerConfig, alerter alerting.Provider, logger *zap.Logger) (*Backfiller, error) {
	applyBackfillerDefaults(&cfg)

	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("backfiller: at least one Kafka broker is required")
	}

	// Configure sarama consumer group.
	saramaCfg := sarama.NewConfig()
	saramaCfg.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRoundRobin(),
	}
	saramaCfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	saramaCfg.Consumer.Offsets.AutoCommit.Enable = true
	saramaCfg.Consumer.Offsets.AutoCommit.Interval = 5 * time.Second
	saramaCfg.Version = sarama.V3_5_0_0

	group, err := sarama.NewConsumerGroup(cfg.Brokers, cfg.GroupID, saramaCfg)
	if err != nil {
		return nil, fmt.Errorf("backfiller: failed to create consumer group: %w", err)
	}

	// Circuit breaker protects ClickHouse from sustained insert storms.
	cb := circuitbreaker.New(circuitbreaker.Config{
		Name:             "clickhouse-backfiller",
		FailureThreshold: 5,
		Timeout:          30 * time.Second,
		MaxRequests:      1,
		Interval:         60 * time.Second,
		OnOpenCallback: func(name string) {
			if alerter != nil {
				msg := alerting.MsgCircuitBreakerOpen(
					name,
					"5 consecutive ClickHouse insert failures",
					"Auto-probe in 30s",
					time.Now().Format("2006-01-02 15:04:05 MST"),
				)
				_ = alerter.SendDirect(msg)
			}
		},
	}, logger)

	return &Backfiller{
		writer:  writer,
		breaker: cb,
		alerter: alerter,
		logger:  logger.Named("ch_backfiller"),
		cfg:     cfg,
		group:   group,
		done:    make(chan struct{}),
	}, nil
}

// Start launches the background consumer goroutine. It continuously calls
// group.Consume() in a loop to handle rebalances.
func (b *Backfiller) Start() {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.logger.Info("backfiller started",
			zap.Strings("topics", b.cfg.Topics),
			zap.String("group_id", b.cfg.GroupID),
			zap.Int("batch_size", b.cfg.BatchSize),
		)

		for {
			select {
			case <-b.done:
				return
			default:
			}

			// Consume blocks until the session ends (rebalance, close, or error).
			if err := b.group.Consume(context.Background(), b.cfg.Topics, b); err != nil {
				b.logger.Error("backfiller: consume error", zap.Error(err))
				b.totalErrors.Add(1)

				// Brief pause before retry to avoid hot-loop on persistent errors.
				select {
				case <-time.After(5 * time.Second):
				case <-b.done:
					return
				}
			}
		}
	}()
}

// Close signals the consumer to stop and waits for shutdown.
func (b *Backfiller) Close() error {
	close(b.done)
	b.wg.Wait()
	return b.group.Close()
}

// ---------------------------------------------------------------------------
// port.HealthChecker implementation
// ---------------------------------------------------------------------------

// Check verifies that the backfiller is actively consuming.
func (b *Backfiller) Check(_ context.Context) error {
	if !b.isConsuming.Load() {
		return fmt.Errorf("backfiller is not consuming")
	}
	if b.breaker.State() == "open" {
		return fmt.Errorf("backfiller circuit breaker is open")
	}
	return nil
}

// Name returns the health checker name.
func (b *Backfiller) Name() string {
	return "clickhouse_backfiller"
}

// ---------------------------------------------------------------------------
// BackfillerMetrics
// ---------------------------------------------------------------------------

// BackfillerMetrics holds real-time backfiller statistics.
type BackfillerMetrics struct {
	TotalConsumed int64
	TotalInserted int64
	TotalErrors   int64
	IsConsuming   bool
}

// Metrics returns a snapshot of the backfiller statistics.
func (b *Backfiller) Metrics() BackfillerMetrics {
	return BackfillerMetrics{
		TotalConsumed: b.totalConsumed.Load(),
		TotalInserted: b.totalInserted.Load(),
		TotalErrors:   b.totalErrors.Load(),
		IsConsuming:   b.isConsuming.Load(),
	}
}

// ---------------------------------------------------------------------------
// sarama.ConsumerGroupHandler implementation
// ---------------------------------------------------------------------------

// Setup is called at the beginning of a new consumer group session.
func (b *Backfiller) Setup(session sarama.ConsumerGroupSession) error {
	b.isConsuming.Store(true)
	b.logger.Info("backfiller session started",
		zap.Int32("generation_id", session.GenerationID()),
	)
	return nil
}

// Cleanup is called at the end of a consumer group session.
func (b *Backfiller) Cleanup(session sarama.ConsumerGroupSession) error {
	b.isConsuming.Store(false)
	b.logger.Info("backfiller session ended",
		zap.Int32("generation_id", session.GenerationID()),
	)
	return nil
}

// ConsumeClaim processes messages from a single Kafka partition.
// It accumulates events into a batch and flushes on BatchSize or FlushInterval.
func (b *Backfiller) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	batch := make([]*entity.ProctoringEvent, 0, b.cfg.BatchSize)
	ticker := time.NewTicker(b.cfg.FlushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}

		events := batch
		batch = make([]*entity.ProctoringEvent, 0, b.cfg.BatchSize)

		// Insert through circuit breaker.
		_, err := b.breaker.Execute(func() (interface{}, error) {
			return nil, b.writer.insertBatch(events)
		})

		if err != nil {
			b.totalErrors.Add(1)
			b.logger.Error("backfiller: ClickHouse insert failed, sleeping before retry",
				zap.Int("event_count", len(events)),
				zap.Error(err),
			)

			// Do NOT commit offsets — Kafka will redeliver these events.
			// Sleep to avoid hammering a sick ClickHouse.
			select {
			case <-time.After(b.cfg.PauseOnError):
			case <-b.done:
			}
			return
		}

		b.totalInserted.Add(int64(len(events)))
		b.logger.Debug("backfiller: batch inserted",
			zap.Int("event_count", len(events)),
		)
	}

	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				// Channel closed — flush remaining.
				flush()
				return nil
			}

			b.totalConsumed.Add(1)

			// Update consumer lag metric for monitoring.
			hwm := claim.HighWaterMarkOffset()
			lag := hwm - msg.Offset - 1
			if lag < 0 {
				lag = 0
			}
			backfillerConsumerLag.WithLabelValues(
				msg.Topic, fmt.Sprintf("%d", msg.Partition),
			).Set(float64(lag))

			// Deserialize the Kafka wire format.
			event, err := deserializeKafkaEvent(msg.Value)
			if err != nil {
				b.logger.Warn("backfiller: failed to deserialize event, skipping",
					zap.Int64("offset", msg.Offset),
					zap.String("topic", msg.Topic),
					zap.Error(err),
				)
				session.MarkMessage(msg, "")
				continue
			}

			batch = append(batch, event)
			session.MarkMessage(msg, "")

			if len(batch) >= b.cfg.BatchSize {
				flush()
			}

		case <-ticker.C:
			flush()

		case <-b.done:
			flush()
			return nil
		}
	}
}

// ---------------------------------------------------------------------------
// Kafka Event Deserialization
// ---------------------------------------------------------------------------

// backfillerKafkaEvent mirrors the kafkaEvent wire format in producer.go.
// Duplicated here to avoid exporting the producer's internal type.
type backfillerKafkaEvent struct {
	EventID         string                    `json:"event_id"`
	SessionID       string                    `json:"session_id"`
	StudentID       string                    `json:"student_id"`
	ExamID          string                    `json:"exam_id"`
	OrgID           string                    `json:"org_id"`
	EventType       string                    `json:"event_type"`
	Severity        string                    `json:"severity"`
	Source          string                    `json:"source"`
	ServerTimestamp string                    `json:"server_timestamp"`
	ClientTimestamp string                    `json:"client_timestamp"`
	VideoTimestamp  float64                   `json:"video_timestamp_sec"`
	Label           string                    `json:"label"`
	Confidence      float32                   `json:"confidence"`
	Payload         json.RawMessage           `json:"payload,omitempty"`
	PayloadType     string                    `json:"payload_type,omitempty"`
	ClientMeta      *backfillerKafkaClientMeta `json:"client_meta,omitempty"`
}

type backfillerKafkaClientMeta struct {
	UserAgent         string `json:"user_agent"`
	SDKVersion        string `json:"sdk_version"`
	Resolution        string `json:"resolution"`
	TimezoneOffsetMin int32  `json:"timezone_offset_min"`
	IPAddress         string `json:"ip_address"`
	Region            string `json:"region"`
}

// deserializeKafkaEvent converts a Kafka message value (JSON) into a
// domain ProctoringEvent. This is the inverse of Producer.toKafkaEvent().
func deserializeKafkaEvent(data []byte) (*entity.ProctoringEvent, error) {
	var ke backfillerKafkaEvent
	if err := json.Unmarshal(data, &ke); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}

	serverTS, _ := time.Parse(time.RFC3339Nano, ke.ServerTimestamp)
	clientTS, _ := time.Parse(time.RFC3339Nano, ke.ClientTimestamp)

	event := &entity.ProctoringEvent{
		EventID:         ke.EventID,
		SessionID:       ke.SessionID,
		StudentID:       ke.StudentID,
		ExamID:          ke.ExamID,
		OrgID:           ke.OrgID,
		EventType:       valueobject.ParseEventType(ke.EventType),
		Severity:        valueobject.ParseSeverity(ke.Severity),
		Source:          valueobject.ParseEventSource(ke.Source),
		ServerTimestamp: serverTS,
		ClientTimestamp: clientTS,
		VideoTimestamp:  ke.VideoTimestamp,
		Label:           ke.Label,
		Confidence:      ke.Confidence,
		Payload:         ke.Payload,
		PayloadType:     ke.PayloadType,
	}

	if ke.ClientMeta != nil {
		event.ClientMeta = entity.ClientMeta{
			UserAgent:         ke.ClientMeta.UserAgent,
			SDKVersion:        ke.ClientMeta.SDKVersion,
			Resolution:        ke.ClientMeta.Resolution,
			TimezoneOffsetMin: ke.ClientMeta.TimezoneOffsetMin,
			IPAddress:         ke.ClientMeta.IPAddress,
			Region:            ke.ClientMeta.Region,
		}
	}

	return event, nil
}
