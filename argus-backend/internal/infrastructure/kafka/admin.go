// Package kafka — Kafka Topic Administration
//
// EnsureTopics creates the required Kafka topics if they don't already exist.
// This is called once at startup (idempotent) and replaces the implicit
// auto-creation that Kafka brokers provide by default.
//
// Explicit topic creation gives the operator control over:
//   - Partition count (affects consumer parallelism and throughput)
//   - Replication factor (affects durability and HA)
//   - Retention policy (affects disk usage and replay window)
//
// Design:
//   - Uses sarama.ClusterAdmin (short-lived admin connection, closed after use).
//   - Topics that already exist are silently skipped (ErrTopicAlreadyExists).
//   - Non-fatal: if topic creation fails (broker unreachable, permission denied),
//     the service logs a warning and continues — topics may be managed externally
//     by Terraform, Strimzi, or a Kafka operator.
package kafka

import (
	"fmt"

	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Topic Configuration
// ---------------------------------------------------------------------------

// TopicConfig describes a Kafka topic to be created.
type TopicConfig struct {
	// Name is the fully-qualified topic name (e.g., "argus.events.critical").
	Name string

	// NumPartitions controls consumer parallelism. Each partition can be consumed
	// by exactly one consumer in a consumer group. More partitions = more parallelism.
	// Rule of thumb: partitions >= max expected consumer instances × 2.
	NumPartitions int32

	// ReplicationFactor controls durability. RF=3 tolerates 2 broker failures.
	// Must not exceed the number of brokers in the cluster.
	ReplicationFactor int16

	// RetentionMs is the message retention period in milliseconds.
	// After this period, messages are eligible for deletion.
	// 0 = use broker default.
	RetentionMs int64

	// CleanupPolicy is "delete" (time-based) or "compact" (key-based dedup).
	// Default: "delete".
	CleanupPolicy string
}

// ---------------------------------------------------------------------------
// Default Topic Definitions
// ---------------------------------------------------------------------------

// DefaultTopics returns the standard Argus AI Kafka topics with production-grade
// defaults. Partition and replication values can be overridden via config.
func DefaultTopics(partitions int32, replication int16) []TopicConfig {
	if partitions <= 0 {
		partitions = 6
	}
	if replication <= 0 {
		replication = 1 // Safe default for single-broker dev; override to 3 in prod
	}

	return []TopicConfig{
		{
			Name:              "argus.events.critical",
			NumPartitions:     partitions,
			ReplicationFactor: replication,
			RetentionMs:       7 * 24 * 60 * 60 * 1000, // 7 days
			CleanupPolicy:     "delete",
		},
		{
			Name:              "argus.events.standard",
			NumPartitions:     partitions * 2, // Higher throughput
			ReplicationFactor: replication,
			RetentionMs:       3 * 24 * 60 * 60 * 1000, // 3 days
			CleanupPolicy:     "delete",
		},
		{
			Name:              "argus.events.telemetry",
			NumPartitions:     partitions * 2, // Highest throughput
			ReplicationFactor: max(1, replication-1), // Lower RF acceptable for telemetry
			RetentionMs:       1 * 24 * 60 * 60 * 1000, // 1 day
			CleanupPolicy:     "delete",
		},
	}
}

// ---------------------------------------------------------------------------
// Topic Administration
// ---------------------------------------------------------------------------

// EnsureTopics creates Kafka topics if they don't already exist.
//
// This function is idempotent — safe to call on every startup. Topics that
// already exist are silently skipped. The admin connection is short-lived
// and closed before returning.
//
// Returns an error only for connection-level failures (broker unreachable).
// Individual topic creation failures (e.g., already exists) are logged as
// warnings but do not cause a return error.
func EnsureTopics(brokers []string, topics []TopicConfig, logger *zap.Logger) error {
	if len(brokers) == 0 {
		return fmt.Errorf("kafka admin: no brokers configured")
	}
	if len(topics) == 0 {
		logger.Debug("kafka admin: no topics to create")
		return nil
	}

	// Create a minimal sarama config for the admin client.
	saramaCfg := sarama.NewConfig()
	saramaCfg.Version = sarama.V3_5_0_0

	admin, err := sarama.NewClusterAdmin(brokers, saramaCfg)
	if err != nil {
		return fmt.Errorf("kafka admin: failed to connect to brokers %v: %w", brokers, err)
	}
	defer func() {
		if closeErr := admin.Close(); closeErr != nil {
			logger.Warn("kafka admin: close error", zap.Error(closeErr))
		}
	}()

	// Fetch existing topics to avoid redundant creation attempts.
	existingTopics, err := admin.ListTopics()
	if err != nil {
		return fmt.Errorf("kafka admin: failed to list existing topics: %w", err)
	}

	created := 0
	skipped := 0

	for _, tc := range topics {
		// Skip if topic already exists.
		if _, exists := existingTopics[tc.Name]; exists {
			logger.Debug("kafka admin: topic already exists, skipping",
				zap.String("topic", tc.Name),
			)
			skipped++
			continue
		}

		// Build topic detail.
		detail := &sarama.TopicDetail{
			NumPartitions:     tc.NumPartitions,
			ReplicationFactor: tc.ReplicationFactor,
			ConfigEntries:     make(map[string]*string),
		}

		// Set retention if specified.
		if tc.RetentionMs > 0 {
			retention := fmt.Sprintf("%d", tc.RetentionMs)
			detail.ConfigEntries["retention.ms"] = &retention
		}

		// Set cleanup policy if specified.
		if tc.CleanupPolicy != "" {
			policy := tc.CleanupPolicy
			detail.ConfigEntries["cleanup.policy"] = &policy
		}

		if err := admin.CreateTopic(tc.Name, detail, false); err != nil {
			// ErrTopicAlreadyExists is not a real error — race condition with
			// another instance creating the same topic concurrently.
			logger.Warn("kafka admin: failed to create topic (may already exist)",
				zap.String("topic", tc.Name),
				zap.Error(err),
			)
			continue
		}

		logger.Info("kafka admin: topic created",
			zap.String("topic", tc.Name),
			zap.Int32("partitions", tc.NumPartitions),
			zap.Int16("replication", tc.ReplicationFactor),
			zap.Int64("retention_ms", tc.RetentionMs),
		)
		created++
	}

	logger.Info("kafka admin: topic provisioning complete",
		zap.Int("created", created),
		zap.Int("skipped", skipped),
		zap.Int("total", len(topics)),
	)

	return nil
}
