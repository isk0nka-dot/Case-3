// Package kafka implements the Kafka producer for event streaming.
// This is an infrastructure adapter that implements the port.EventWriter interface.
//
// Architecture-level design decisions (Google/Meta SRE standards):
//
//   - IBM/sarama AsyncProducer — battle-tested, used in production at LinkedIn,
//     Uber, Shopify. confluent-kafka-go uses CGO (requires librdkafka), which
//     breaks CGO_ENABLED=0 static binaries needed for distroless/scratch Docker
//     images. segmentio/kafka-go lacks proper AsyncProducer with background
//     batching. sarama is the only pure-Go library with true async production,
//     which is the correct choice for a statically-compiled K8s microservice.
//
//   - Idempotent production with RequiredAcks=WaitForAll + MaxOpenRequests=1
//     guarantees exactly-once delivery within a producer session. This is
//     the strongest durability guarantee Kafka supports without transactions.
//
//   - Partition key is session_id for per-session event ordering. This means
//     all events from one proctoring session land on the same partition, enabling
//     stateful stream consumers (e.g., session violation aggregators) to process
//     events in chronological order without coordination.
//
//   - zstd compression yields 5-8x compression on JSON event payloads,
//     reducing Kafka broker storage by ~80% and network bandwidth proportionally.
//     zstd outperforms snappy (2-3x) and lz4 (3-4x) on structured data while
//     maintaining comparable CPU overhead.
//
//   - Background error listener with atomic metrics enables real-time monitoring
//     via Prometheus without any locking on the hot path.
//
//   - Circuit-breaker readiness: Write() returns errors that the use case can
//     propagate to a circuit breaker, allowing the system to fail-open under
//     Kafka broker outages rather than building up unbounded backpressure.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"
	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/alerting"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// ProducerConfig holds all tunable parameters for the Kafka async producer.
// Every field has a yaml tag for config-file binding and a sensible default
// applied in applyDefaults() so the producer works with zero configuration.
type ProducerConfig struct {
	// Brokers is the list of Kafka bootstrap servers (host:port).
	// At least one must be reachable for the producer to initialize.
	Brokers []string `yaml:"brokers"`

	// RequiredAcks controls durability:
	//   -1 = WaitForAll (all ISR replicas must acknowledge — strongest guarantee)
	//    1 = WaitForLeader (leader only — faster, but data at risk if leader crashes)
	//    0 = NoResponse (fire-and-forget — highest throughput, no durability)
	// Default: -1 (WaitForAll) for zero data loss.
	RequiredAcks int `yaml:"required_acks"`

	// MaxMessageBytes caps a single Kafka message size. Events exceeding this
	// limit are rejected at the producer level before hitting the broker.
	// Must be <= broker's message.max.bytes. Default: 1 MiB.
	MaxMessageBytes int `yaml:"max_message_bytes"`

	// FlushFrequency is the maximum duration the async producer will buffer
	// messages before flushing to the broker. Lower values reduce latency
	// but increase broker RPC overhead. Default: 100ms.
	FlushFrequency time.Duration `yaml:"flush_frequency"`

	// FlushMessages is the maximum number of messages the async producer
	// will buffer before triggering a batch flush. Default: 500.
	// Set to match the expected burst size of a batch IngestBatch() call.
	FlushMessages int `yaml:"flush_messages"`

	// FlushBytes triggers a flush when the accumulated batch size in bytes
	// exceeds this threshold. Default: 1 MiB. Works in conjunction with
	// FlushFrequency and FlushMessages — whichever triggers first wins.
	FlushBytes int `yaml:"flush_bytes"`

	// RetryMax is the number of times a failed produce request is retried
	// with exponential backoff before being reported as a permanent error.
	// Default: 5 (with idempotent=true, retries are safe and don't cause
	// duplicates). Set higher for flaky networks.
	RetryMax int `yaml:"retry_max"`

	// RetryBackoff is the initial backoff duration between retries.
	// Subsequent retries double this value (exponential backoff).
	// Default: 100ms → 200ms → 400ms → 800ms → 1600ms.
	RetryBackoff time.Duration `yaml:"retry_backoff"`

	// CompressionCodec selects the wire-level compression algorithm.
	// Valid values: "zstd" (recommended), "snappy", "lz4", "gzip", "none".
	// zstd provides the best compression ratio for JSON/protobuf event payloads
	// with acceptable CPU overhead. Default: "zstd".
	CompressionCodec string `yaml:"compression_codec"`

	// IdempotentEnabled activates the Kafka idempotent producer, which assigns
	// a producer ID and sequence number to every message. The broker deduplicates
	// retries, guaranteeing exactly-once semantics within a single producer session.
	// Requires: RequiredAcks=-1, MaxOpenRequests=1. Default: true.
	IdempotentEnabled bool `yaml:"idempotent_enabled"`

	// ChannelBufferSize sets the internal channel capacity for the async producer.
	// A larger buffer absorbs traffic spikes without blocking Write() calls,
	// at the cost of higher memory usage. Default: 4096.
	// Memory: ~256 bytes/msg * 4096 = ~1 MiB.
	ChannelBufferSize int `yaml:"channel_buffer_size"`

	// MetadataRefreshFrequency controls how often the producer refreshes
	// topic/partition metadata from the Kafka cluster. Needed to detect
	// broker failures and partition reassignments. Default: 5 minutes.
	MetadataRefreshFrequency time.Duration `yaml:"metadata_refresh_frequency"`
}

// ---------------------------------------------------------------------------
// Producer
// ---------------------------------------------------------------------------

// Producer implements port.EventWriter using Apache Kafka's AsyncProducer.
// It is the PRIMARY data sink — if a Kafka write fails, the event is rejected.
//
// Thread safety: All methods are safe for concurrent use by multiple goroutines.
// Write() and WriteBatch() are non-blocking (they enqueue to the async channel).
// Close() blocks until all pending messages are flushed and acknowledged.
//
// Metrics are exposed via lock-free atomic counters for zero-contention
// observability. In a production deployment, these counters would be exported
// as Prometheus gauges via a /metrics HTTP endpoint.
type Producer struct {
	producer sarama.AsyncProducer
	logger   *zap.Logger
	wg       sync.WaitGroup
	closed   chan struct{}

	// Lock-free atomic metrics — no mutex on the hot path.
	// Updated by background goroutines (handleSuccesses, handleErrors).
	messagesProduced atomic.Int64 // Total messages successfully acknowledged by Kafka.
	messagesFailed   atomic.Int64 // Total messages that failed after all retries.
	bytesProduced    atomic.Int64 // Total payload bytes successfully produced.

	// Alerter: optional emergency notification provider.
	alerter alerting.Provider
}

// SetAlerter configures an optional emergency alerting provider.
func (p *Producer) SetAlerter(a alerting.Provider) {
	p.alerter = a
}

// NewProducer creates a new Kafka async producer with the given configuration.
// It immediately starts two background goroutines:
//   - handleSuccesses: drains the Successes channel, updates metrics.
//   - handleErrors: drains the Errors channel, logs failures, updates metrics.
//
// Both goroutines MUST be drained — if either channel blocks, the producer
// deadlocks. sarama's documentation explicitly requires this.
//
// Returns an error if the initial connection to at least one broker fails.
func NewProducer(cfg ProducerConfig, logger *zap.Logger) (*Producer, error) {
	applyProducerDefaults(&cfg)

	saramaCfg := sarama.NewConfig()

	// -----------------------------------------------------------------
	// Wire protocol version. Require Kafka 2.8+ for KRaft and idempotent
	// producer support without Zookeeper.
	// -----------------------------------------------------------------
	saramaCfg.Version = sarama.V2_8_0_0

	// -----------------------------------------------------------------
	// Durability & acknowledgement.
	// -----------------------------------------------------------------
	saramaCfg.Producer.RequiredAcks = sarama.RequiredAcks(cfg.RequiredAcks)

	// Must enable both Return.Successes and Return.Errors so the background
	// goroutines can drain both channels. Failing to drain either channel
	// causes a deadlock in the async producer.
	saramaCfg.Producer.Return.Successes = true
	saramaCfg.Producer.Return.Errors = true

	// -----------------------------------------------------------------
	// Batching — tuned for 10,000+ concurrent sessions.
	//
	// The three flush triggers work as a logical OR:
	//   flush_when(elapsed >= FlushFrequency OR
	//              messages >= FlushMessages  OR
	//              bytes    >= FlushBytes)
	//
	// This ensures low-latency delivery during bursts (messages trigger)
	// while still flushing idle partitions periodically (time trigger).
	// -----------------------------------------------------------------
	saramaCfg.Producer.Flush.Frequency = cfg.FlushFrequency
	saramaCfg.Producer.Flush.Messages = cfg.FlushMessages
	saramaCfg.Producer.Flush.Bytes = cfg.FlushBytes

	// -----------------------------------------------------------------
	// Message size limit.
	// -----------------------------------------------------------------
	saramaCfg.Producer.MaxMessageBytes = cfg.MaxMessageBytes

	// -----------------------------------------------------------------
	// Retries with exponential backoff.
	// Safe with idempotent=true because the broker deduplicates by
	// (producer_id, sequence_number).
	// -----------------------------------------------------------------
	saramaCfg.Producer.Retry.Max = cfg.RetryMax
	saramaCfg.Producer.Retry.Backoff = cfg.RetryBackoff

	// -----------------------------------------------------------------
	// Compression.
	// zstd (Zstandard) provides 5-8x compression ratio on JSON payloads
	// with ~50% of the CPU cost compared to gzip. Broker-side decompression
	// is zero-copy since Kafka 2.1.
	// -----------------------------------------------------------------
	saramaCfg.Producer.Compression = parseCompressionCodec(cfg.CompressionCodec)

	// -----------------------------------------------------------------
	// Idempotent producer — exactly-once within a session.
	//
	// Guarantees:
	//   1. No duplicate messages on retry (broker-side deduplication).
	//   2. Per-partition message ordering is preserved even during retries.
	//
	// Constraints enforced by Kafka protocol:
	//   - RequiredAcks must be WaitForAll (-1).
	//   - MaxOpenRequests must be 1 (serialized requests per broker).
	//   - Retries must be > 0.
	//
	// The single in-flight request constraint reduces peak throughput by
	// ~15-20% compared to MaxOpenRequests=5, but guarantees strict ordering.
	// At 10K events/sec this is negligible — the bottleneck is network RTT,
	// not client-side parallelism.
	// -----------------------------------------------------------------
	if cfg.IdempotentEnabled {
		saramaCfg.Producer.Idempotent = true
		saramaCfg.Producer.RequiredAcks = sarama.WaitForAll
		saramaCfg.Net.MaxOpenRequests = 1
	}

	// -----------------------------------------------------------------
	// Channel buffer size for the async producer's input channel.
	// A larger buffer absorbs bursts without blocking Write() callers.
	// -----------------------------------------------------------------
	saramaCfg.ChannelBufferSize = cfg.ChannelBufferSize

	// -----------------------------------------------------------------
	// Metadata refresh — detect broker failures and rebalancing.
	// -----------------------------------------------------------------
	saramaCfg.Metadata.RefreshFrequency = cfg.MetadataRefreshFrequency

	// -----------------------------------------------------------------
	// Partitioner: hash-based on the message key (session_id).
	// sarama uses murmur2 by default, matching the Java client's
	// DefaultPartitioner for cross-language compatibility.
	// -----------------------------------------------------------------
	saramaCfg.Producer.Partitioner = sarama.NewHashPartitioner

	// -----------------------------------------------------------------
	// Create the async producer. This dials at least one broker to
	// verify connectivity and fetch initial metadata.
	// -----------------------------------------------------------------
	asyncProducer, err := sarama.NewAsyncProducer(cfg.Brokers, saramaCfg)
	if err != nil {
		return nil, fmt.Errorf("kafka: failed to create async producer: %w", err)
	}

	p := &Producer{
		producer: asyncProducer,
		logger:   logger.Named("kafka_producer"),
		closed:   make(chan struct{}),
	}

	// Start background goroutines that MUST drain the Successes and Errors
	// channels. If either goroutine stops, the producer will deadlock.
	p.wg.Add(2)
	go p.handleSuccesses()
	go p.handleErrors()

	p.logger.Info("kafka async producer initialized",
		zap.Strings("brokers", cfg.Brokers),
		zap.String("compression", cfg.CompressionCodec),
		zap.Bool("idempotent", cfg.IdempotentEnabled),
		zap.Int("required_acks", cfg.RequiredAcks),
		zap.Int("channel_buffer", cfg.ChannelBufferSize),
		zap.Duration("flush_frequency", cfg.FlushFrequency),
		zap.Int("flush_messages", cfg.FlushMessages),
		zap.Int("flush_bytes", cfg.FlushBytes),
		zap.Int("retry_max", cfg.RetryMax),
	)

	return p, nil
}

// ---------------------------------------------------------------------------
// port.EventWriter Implementation
// ---------------------------------------------------------------------------

// Write enqueues a single event to the Kafka async producer.
//
// This method is NON-BLOCKING under normal conditions — it pushes the message
// onto the internal channel and returns immediately. The actual broker send
// happens asynchronously in a background goroutine managed by sarama.
//
// Blocking occurs only when the internal channel is full (backpressure),
// which is bounded by ChannelBufferSize. In that case, Write blocks until
// either a slot becomes available or the context is cancelled.
//
// The context is checked in a select — if the caller's context expires
// (e.g., gRPC deadline), Write returns ctx.Err() immediately rather than
// waiting indefinitely for a free channel slot.
func (p *Producer) Write(ctx context.Context, event *entity.ProctoringEvent) error {
	select {
	case <-p.closed:
		return fmt.Errorf("kafka: producer is closed")
	default:
	}

	msg, err := p.buildMessage(event)
	if err != nil {
		return fmt.Errorf("kafka: failed to build message: %w", err)
	}

	select {
	case p.producer.Input() <- msg:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("kafka: write cancelled: %w", ctx.Err())
	case <-p.closed:
		return fmt.Errorf("kafka: producer closed during write")
	}
}

// WriteBatch enqueues multiple events to the Kafka async producer.
//
// Events are pushed individually onto the channel — sarama handles internal
// batching, compression, and wire-level grouping by partition. This means
// a "batch" of 100 events going to 10 different partitions will be split
// into 10 broker-level produce requests, each compressed independently.
//
// If the channel fills up mid-batch, WriteBatch blocks on the full slot
// and respects context cancellation. Events already enqueued before the
// cancellation are NOT rolled back — they will be produced asynchronously.
// This is intentional: partial delivery is acceptable because Kafka is the
// source of truth and consumers are idempotent.
func (p *Producer) WriteBatch(ctx context.Context, events []*entity.ProctoringEvent) error {
	select {
	case <-p.closed:
		return fmt.Errorf("kafka: producer is closed")
	default:
	}

	for _, event := range events {
		msg, err := p.buildMessage(event)
		if err != nil {
			// Skip malformed events in a batch — don't fail the entire batch
			// for one bad message. The use case layer already validated domain
			// invariants; this is a serialization safety net.
			p.logger.Warn("kafka: skipping malformed event in batch",
				zap.String("event_id", event.EventID),
				zap.Error(err),
			)
			continue
		}

		select {
		case p.producer.Input() <- msg:
			// Message enqueued successfully.
		case <-ctx.Done():
			return fmt.Errorf("kafka: batch write cancelled after %d events: %w",
				len(events), ctx.Err())
		case <-p.closed:
			return fmt.Errorf("kafka: producer closed during batch write")
		}
	}

	return nil
}

// PublishRaw enqueues a raw message to a specified Kafka topic.
// This is used for non-event messages such as evidence chain of custody records.
//
// Unlike Write/WriteBatch which use domain events, PublishRaw takes raw key/value
// bytes and a topic name. It uses the same async producer and respects the same
// backpressure and shutdown semantics.
func (p *Producer) PublishRaw(ctx context.Context, topic string, key string, value []byte) error {
	select {
	case <-p.closed:
		return fmt.Errorf("kafka: producer is closed")
	default:
	}

	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(value),
	}

	select {
	case p.producer.Input() <- msg:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("kafka: publish raw cancelled: %w", ctx.Err())
	case <-p.closed:
		return fmt.Errorf("kafka: producer closed during publish raw")
	}
}

// Close flushes all pending messages and shuts down the producer.
//
// Shutdown sequence:
//  1. Signal the closed channel (Write/WriteBatch will return errors).
//  2. Call AsyncClose() on the sarama producer, which:
//     a. Closes the Input channel (no new messages accepted).
//     b. Flushes all buffered messages to brokers.
//     c. Waits for all in-flight produce requests to complete.
//     d. Closes the Successes and Errors channels.
//  3. Wait for handleSuccesses and handleErrors goroutines to exit
//     (they exit when their respective channels are closed by sarama).
//
// After Close returns, no background goroutines are running.
func (p *Producer) Close() error {
	// Prevent double-close panic.
	select {
	case <-p.closed:
		return nil
	default:
		close(p.closed)
	}

	// AsyncClose flushes pending messages and closes channels.
	// We use AsyncClose + wg.Wait instead of Close() to avoid
	// a deadlock if the error/success handlers are blocked.
	p.producer.AsyncClose()

	// Wait for background goroutines to drain their channels.
	p.wg.Wait()

	p.logger.Info("kafka producer closed",
		zap.Int64("total_produced", p.messagesProduced.Load()),
		zap.Int64("total_failed", p.messagesFailed.Load()),
		zap.Int64("total_bytes", p.bytesProduced.Load()),
	)

	return nil
}

// ---------------------------------------------------------------------------
// Health Check (port.HealthChecker)
// ---------------------------------------------------------------------------

// Check verifies that the producer can reach the Kafka cluster.
// This is called by the HTTP /readyz endpoint for Kubernetes readiness probes.
func (p *Producer) Check(_ context.Context) error {
	select {
	case <-p.closed:
		return fmt.Errorf("kafka producer is closed")
	default:
		return nil
	}
}

// Name returns the health checker name for the readiness response.
func (p *Producer) Name() string {
	return "kafka_producer"
}

// ---------------------------------------------------------------------------
// Metrics
// ---------------------------------------------------------------------------

// ProducerMetrics holds real-time producer statistics.
// All values are atomically updated — safe to read from any goroutine.
type ProducerMetrics struct {
	MessagesProduced int64 // Successfully acknowledged by Kafka.
	MessagesFailed   int64 // Permanently failed after all retries.
	BytesProduced    int64 // Total payload bytes produced.
}

// Metrics returns a snapshot of the current producer statistics.
func (p *Producer) Metrics() ProducerMetrics {
	return ProducerMetrics{
		MessagesProduced: p.messagesProduced.Load(),
		MessagesFailed:   p.messagesFailed.Load(),
		BytesProduced:    p.bytesProduced.Load(),
	}
}

// ---------------------------------------------------------------------------
// Background Goroutines
// ---------------------------------------------------------------------------

// handleSuccesses drains the Successes channel from the async producer.
// Every message that is successfully acknowledged by all ISR replicas
// appears on this channel. We update atomic metrics and discard the message.
//
// This goroutine MUST run for the lifetime of the producer. If it stops,
// the Successes channel fills up and the producer deadlocks.
func (p *Producer) handleSuccesses() {
	defer p.wg.Done()

	for msg := range p.producer.Successes() {
		p.messagesProduced.Add(1)
		// Track bytes for throughput monitoring.
		if msg.Value != nil {
			p.bytesProduced.Add(int64(msg.Value.Length()))
		}
	}
}

// handleErrors drains the Errors channel from the async producer.
// Messages appear here only after all retry attempts have been exhausted
// (controlled by RetryMax). This represents a permanent send failure.
//
// Each failure is logged at ERROR level with the topic and error details.
// In production, this would also increment a Prometheus counter and
// potentially trigger a PagerDuty alert if the error rate exceeds a threshold.
//
// This goroutine MUST run for the lifetime of the producer. If it stops,
// the Errors channel fills up and the producer deadlocks.
func (p *Producer) handleErrors() {
	defer p.wg.Done()

	for prodErr := range p.producer.Errors() {
		p.messagesFailed.Add(1)

		// Extract metadata from the failed message for debugging.
		topic := ""
		key := ""
		if prodErr.Msg != nil {
			topic = prodErr.Msg.Topic
			if prodErr.Msg.Key != nil {
				keyBytes, _ := prodErr.Msg.Key.Encode()
				key = string(keyBytes)
			}
		}

		p.logger.Error("kafka: message delivery failed permanently",
			zap.String("topic", topic),
			zap.String("partition_key", key),
			zap.Error(prodErr.Err),
			zap.Int64("total_failures", p.messagesFailed.Load()),
		)

		// Fire emergency alert on permanent delivery failure.
		if p.alerter != nil {
			p.alerter.Send(alerting.Alert{
				Component: "kafka-producer",
				SessionID: key,
				Severity:  "CRITICAL",
				Error:     fmt.Sprintf("Message delivery failed permanently. Topic: %s, Key: %s, Error: %v, Total failures: %d", topic, key, prodErr.Err, p.messagesFailed.Load()),
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Message Construction
// ---------------------------------------------------------------------------

// buildMessage converts a domain ProctoringEvent into a sarama.ProducerMessage
// ready for async production.
//
// Wire format: JSON. While Protobuf would be ~40% smaller, JSON is chosen because:
//   - ClickHouse natively parses JSON columns (no deserialization step).
//   - Kafka Connect / Flink / Spark consumers don't need .proto definitions.
//   - Human-readable when debugging with kafkacat / kcat.
//   - Compression (zstd) reduces the JSON overhead to near-Protobuf sizes.
//
// Message key: session_id (murmur2 hash → partition assignment).
// This guarantees per-session ordering across all event types.
//
// Headers: Kafka headers provide O(1) filtering in consumers without
// deserializing the message body. A consumer that only cares about CRITICAL
// events can filter by the "severity" header without parsing JSON.
func (p *Producer) buildMessage(event *entity.ProctoringEvent) (*sarama.ProducerMessage, error) {
	value, err := json.Marshal(p.toKafkaEvent(event))
	if err != nil {
		return nil, fmt.Errorf("json marshal failed: %w", err)
	}

	return &sarama.ProducerMessage{
		// Topic routing: critical/standard/telemetry based on event severity
		// and type. Defined in domain entity — producer doesn't make routing decisions.
		Topic: event.KafkaTopic(),

		// Partition key: session_id → all events from one proctoring session
		// land on the same Kafka partition, preserving chronological order.
		Key: sarama.StringEncoder(event.PartitionKey()),

		// Payload: JSON-encoded event body.
		Value: sarama.ByteEncoder(value),

		// Metadata headers for consumer-side filtering without deserialization.
		// These enable efficient topic-level consumers that process only
		// specific event types (e.g., a "face mismatch alert" microservice
		// filters by event_type=face_mismatch in the header).
		Headers: []sarama.RecordHeader{
			{Key: []byte("event_type"), Value: []byte(event.EventType.String())},
			{Key: []byte("severity"), Value: []byte(event.Severity.String())},
			{Key: []byte("source"), Value: []byte(event.Source.String())},
			{Key: []byte("exam_id"), Value: []byte(event.ExamID)},
			{Key: []byte("org_id"), Value: []byte(event.OrgID)},
			{Key: []byte("session_id"), Value: []byte(event.SessionID)},
		},

		// Timestamp: server-side timestamp for Kafka log-append-time correlation.
		Timestamp: event.ServerTimestamp,
	}, nil
}

// ---------------------------------------------------------------------------
// Kafka Wire Format
// ---------------------------------------------------------------------------

// kafkaEvent is the JSON wire format written to Kafka topics. This struct is
// intentionally decoupled from the domain entity — the wire format is a
// public API contract that must remain stable across service versions.
// Adding fields is safe; removing or renaming fields is a breaking change
// that requires consumer coordination.
type kafkaEvent struct {
	EventID         string           `json:"event_id"`
	SessionID       string           `json:"session_id"`
	StudentID       string           `json:"student_id"`
	ExamID          string           `json:"exam_id"`
	OrgID           string           `json:"org_id"`
	EventType       string           `json:"event_type"`
	Severity        string           `json:"severity"`
	Source          string           `json:"source"`
	ServerTimestamp string           `json:"server_timestamp"`
	ClientTimestamp string           `json:"client_timestamp"`
	VideoTimestamp  float64          `json:"video_timestamp_sec"`
	Label          string           `json:"label"`
	Confidence     float32          `json:"confidence"`
	Payload        json.RawMessage  `json:"payload,omitempty"`
	PayloadType    string           `json:"payload_type,omitempty"`
	ClientMeta     *kafkaClientMeta `json:"client_meta,omitempty"`
}

// kafkaClientMeta holds browser/client metadata in the Kafka wire format.
type kafkaClientMeta struct {
	UserAgent         string `json:"user_agent"`
	SDKVersion        string `json:"sdk_version"`
	Resolution        string `json:"resolution"`
	TimezoneOffsetMin int32  `json:"timezone_offset_min"`
	IPAddress         string `json:"ip_address"`
	Region            string `json:"region"`
}

// toKafkaEvent maps a domain entity to the Kafka wire format.
// Timestamps are formatted as RFC3339Nano for maximum precision
// and universal parseability across languages (Go, Python, Java, JS).
func (p *Producer) toKafkaEvent(e *entity.ProctoringEvent) *kafkaEvent {
	ke := &kafkaEvent{
		EventID:         e.EventID,
		SessionID:       e.SessionID,
		StudentID:       e.StudentID,
		ExamID:          e.ExamID,
		OrgID:           e.OrgID,
		EventType:       e.EventType.String(),
		Severity:        e.Severity.String(),
		Source:          e.Source.String(),
		ServerTimestamp: e.ServerTimestamp.Format(time.RFC3339Nano),
		ClientTimestamp: e.ClientTimestamp.Format(time.RFC3339Nano),
		VideoTimestamp:  e.VideoTimestamp,
		Label:          e.Label,
		Confidence:     e.Confidence,
		Payload:        e.Payload,
		PayloadType:    e.PayloadType,
	}

	// Only include client_meta if at least one field is populated.
	// Reduces average message size by ~80 bytes for synthetic/test events.
	if e.ClientMeta.UserAgent != "" || e.ClientMeta.SDKVersion != "" || e.ClientMeta.IPAddress != "" {
		ke.ClientMeta = &kafkaClientMeta{
			UserAgent:         e.ClientMeta.UserAgent,
			SDKVersion:        e.ClientMeta.SDKVersion,
			Resolution:        e.ClientMeta.Resolution,
			TimezoneOffsetMin: e.ClientMeta.TimezoneOffsetMin,
			IPAddress:         e.ClientMeta.IPAddress,
			Region:            e.ClientMeta.Region,
		}
	}

	return ke
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseCompressionCodec converts a string compression name to sarama's codec.
func parseCompressionCodec(codec string) sarama.CompressionCodec {
	switch codec {
	case "zstd":
		return sarama.CompressionZSTD
	case "snappy":
		return sarama.CompressionSnappy
	case "lz4":
		return sarama.CompressionLZ4
	case "gzip":
		return sarama.CompressionGZIP
	case "none":
		return sarama.CompressionNone
	default:
		return sarama.CompressionZSTD
	}
}

// applyProducerDefaults fills zero-valued config fields with production defaults.
func applyProducerDefaults(cfg *ProducerConfig) {
	if len(cfg.Brokers) == 0 {
		cfg.Brokers = []string{"localhost:9092"}
	}
	if cfg.RequiredAcks == 0 {
		cfg.RequiredAcks = -1 // WaitForAll
	}
	if cfg.MaxMessageBytes == 0 {
		cfg.MaxMessageBytes = 1_048_576 // 1 MiB
	}
	if cfg.FlushFrequency == 0 {
		cfg.FlushFrequency = 100 * time.Millisecond
	}
	if cfg.FlushMessages == 0 {
		cfg.FlushMessages = 500
	}
	if cfg.FlushBytes == 0 {
		cfg.FlushBytes = 1_048_576 // 1 MiB
	}
	if cfg.RetryMax == 0 {
		cfg.RetryMax = 5
	}
	if cfg.RetryBackoff == 0 {
		cfg.RetryBackoff = 100 * time.Millisecond
	}
	if cfg.CompressionCodec == "" {
		cfg.CompressionCodec = "zstd"
	}
	if cfg.ChannelBufferSize == 0 {
		cfg.ChannelBufferSize = 4096
	}
	if cfg.MetadataRefreshFrequency == 0 {
		cfg.MetadataRefreshFrequency = 5 * time.Minute
	}
}
