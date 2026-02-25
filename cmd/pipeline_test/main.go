package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/domain/valueobject"
	"github.com/argus-ai/event-collector/internal/infrastructure/clickhouse"
	"github.com/argus-ai/event-collector/internal/infrastructure/kafka"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()

	// 1. Init Kafka Producer
	kafkaCfg := kafka.ProducerConfig{
		Brokers:           []string{"localhost:9095", "localhost:9096"}, // Using healthy mapped ports
		RequiredAcks:      -1,
		IdempotentEnabled: true,
		CompressionCodec:  "zstd",
	}
	producer, err := kafka.NewProducer(kafkaCfg, logger)
	if err != nil {
		log.Fatalf("Kafka error: %v", err)
	}
	defer producer.Close()

	// 2. Init ClickHouse Writer
	chCfg := clickhouse.WriterConfig{
		Addrs:    []string{"localhost:9001"}, // Native protocol port
		Database: "argus_analytics",
		Username: "default",
	}
	chWriter, err := clickhouse.NewWriter(chCfg, logger)
	if err != nil {
		log.Fatalf("ClickHouse error: %v", err)
	}
	defer chWriter.Close()

	// 3. Create mock telemetry event
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

	// 4. Send to Kafka
	err = producer.Write(context.Background(), event)
	if err != nil {
		log.Fatalf("Failed to write to Kafka: %v", err)
	}
	fmt.Println("Successfully pushed event to Kafka.")

	// 5. Send to ClickHouse
	err = chWriter.WriteBatch(context.Background(), []*entity.ProctoringEvent{event})
	if err != nil {
		log.Fatalf("Failed to write to ClickHouse: %v", err)
	}
	fmt.Println("Successfully pushed event to ClickHouse buffer (flushing).")
	time.Sleep(6 * time.Second) // wait for background flusher (5s default)
}
