package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/domain/valueobject"
	"github.com/argus-ai/event-collector/internal/infrastructure/clickhouse"
	"github.com/argus-ai/event-collector/internal/infrastructure/kafka"
	"go.uber.org/zap"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "pipeline_test: fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	logger, err := zap.NewDevelopment()
	if err != nil {
		return fmt.Errorf("failed to initialise logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	// ── Environment-based configuration (no recompilation needed) ──────
	kafkaBrokers := os.Getenv("PIPELINE_TEST_KAFKA_BROKERS")
	if kafkaBrokers == "" {
		kafkaBrokers = "localhost:9095,localhost:9096"
	}
	chAddr := os.Getenv("PIPELINE_TEST_CH_ADDR")
	if chAddr == "" {
		chAddr = "localhost:9001"
	}
	chDatabase := os.Getenv("PIPELINE_TEST_CH_DATABASE")
	if chDatabase == "" {
		chDatabase = "argus_analytics"
	}

	// 1. Init Kafka Producer
	kafkaCfg := kafka.ProducerConfig{
		Brokers:           strings.Split(kafkaBrokers, ","),
		RequiredAcks:      -1,
		IdempotentEnabled: true,
		CompressionCodec:  "zstd",
	}
	producer, err := kafka.NewProducer(kafkaCfg, logger)
	if err != nil {
		return fmt.Errorf("kafka init failed: %w", err)
	}
	defer func() {
		if err := producer.Close(); err != nil {
			logger.Error("kafka producer close error", zap.Error(err))
		} else {
			logger.Info("kafka producer closed (pending messages flushed)")
		}
	}()

	// 2. Init ClickHouse Writer
	chCfg := clickhouse.WriterConfig{
		Addrs:    []string{chAddr},
		Database: chDatabase,
		Username: "default",
	}
	chWriter, err := clickhouse.NewWriter(chCfg, logger)
	if err != nil {
		return fmt.Errorf("clickhouse init failed: %w", err)
	}
	defer func() {
		// Close() flushes the internal buffer and blocks until all pending
		// batches are written. This replaces the fragile time.Sleep(6s) that
		// previously assumed the flush would complete within a fixed window.
		if err := chWriter.Close(); err != nil {
			logger.Error("clickhouse writer close error", zap.Error(err))
		} else {
			logger.Info("clickhouse writer closed (buffer flushed)")
		}
	}()

	// 3. Create mock telemetry event with a 30s timeout for all I/O.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	event := &entity.ProctoringEvent{
		EventID:         "evt-test-123456",
		SessionID:       "sess-test-auto",
		StudentID:       "stud-1",
		ExamID:          "exam-1",
		OrgID:           "org-test-1",
		EventType:       valueobject.GazeTelemetry,
		Severity:        valueobject.SeverityInfo,
		Source:          valueobject.SourceSideCamera,
		ServerTimestamp: time.Now(),
		ClientTimestamp: time.Now(),
		VideoTimestamp:  10.5,
		Confidence:      0.95,
	}

	// 4. Send to Kafka (with timeout context — no hanging on partition)
	if err := producer.Write(ctx, event); err != nil {
		return fmt.Errorf("kafka write failed: %w", err)
	}
	fmt.Println("Successfully pushed event to Kafka.")

	// 5. Send to ClickHouse (with timeout context)
	if err := chWriter.WriteBatch(ctx, []*entity.ProctoringEvent{event}); err != nil {
		return fmt.Errorf("clickhouse write failed: %w", err)
	}
	fmt.Println("Successfully pushed event to ClickHouse buffer.")

	// 6. Deterministic flush: chWriter.Close() in the defer above blocks
	//    until the internal buffer is flushed. No time.Sleep needed.
	fmt.Println("Closing writers (blocking flush)...")

	return nil
}
