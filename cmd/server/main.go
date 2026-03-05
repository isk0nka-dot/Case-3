// Package main is the entry point for the Event-Collector service.
//
// The Event-Collector is a high-throughput gRPC service that ingests real-time
// proctoring events from browser SDKs, validates them, and fans out to:
//
//   - Apache Kafka (primary, source of truth for real-time streaming)
//   - ClickHouse (secondary, analytical storage for dashboards)
//
// This file wires all dependencies together using constructor injection —
// there are no globals, no init() functions, no service locators. Every
// dependency is created explicitly and passed to the component that needs it.
//
// The service exposes two network listeners:
//
//   - gRPC (port 50051): Native gRPC for internal service-to-service calls.
//   - HTTP (port 8080):  Unified listener for gRPC-Web (browser clients),
//     health checks (/healthz, /readyz), and future Prometheus metrics.
//
// Browser clients (Nuxt 3 SPA) connect via gRPC-Web over HTTP/2 with CORS.
// Authentication uses stateless JWT tokens issued by the Eduser (Java) system.
// Session validation uses an in-memory cache backed by the Eduser API.
//
// Lifecycle (18 steps):
//
//	 1. Parse CLI flags
//	 2. Load configuration (YAML + env overrides)
//	 3. Initialise logger
//	 4. Initialise Kafka producer
//	 5. Initialise ClickHouse writer
//	 6. Initialise IngestUseCase
//	 7. Initialise Worker Pool
//	 8. Initialise JWT Verifier (optional, based on config)
//	 9. Initialise Session Validator (optional, based on config)
//	10. Initialise gRPC interceptor chain
//	11. Create gRPC server with keepalive + interceptors
//	12. Register EventCollectorService
//	13. Create HTTP server (gRPC-Web proxy + health checks + CORS)
//	14. Set up OS signal handler
//	15. Start gRPC server
//	16. Start HTTP server
//	17. Block on signal or fatal error
//	18. Graceful shutdown (gRPC → WorkerPool → HTTP → Session → UseCase)
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	pprofHTTP "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	pb "github.com/argus-ai/event-collector/api/proto/v1"
	"github.com/argus-ai/event-collector/internal/application/port"
	"github.com/argus-ai/event-collector/internal/application/usecase"
	"github.com/argus-ai/event-collector/internal/infrastructure/alerting"
	"github.com/argus-ai/event-collector/internal/infrastructure/chunk"
	"github.com/argus-ai/event-collector/internal/infrastructure/clickhouse"
	"github.com/argus-ai/event-collector/internal/infrastructure/config"
	"github.com/argus-ai/event-collector/internal/infrastructure/evidence"
	exportInfra "github.com/argus-ai/event-collector/internal/infrastructure/export"
	"github.com/argus-ai/event-collector/internal/infrastructure/forensic"
	"github.com/argus-ai/event-collector/internal/infrastructure/health"
	integrityInfra "github.com/argus-ai/event-collector/internal/infrastructure/integrity"
	"github.com/argus-ai/event-collector/internal/infrastructure/dlq"
	"github.com/argus-ai/event-collector/internal/infrastructure/kafka"
	metricsInfra "github.com/argus-ai/event-collector/internal/infrastructure/metrics"
	minioStore "github.com/argus-ai/event-collector/internal/infrastructure/minio"
	"github.com/argus-ai/event-collector/internal/infrastructure/recorder"
	"github.com/argus-ai/event-collector/internal/infrastructure/session"
	"github.com/argus-ai/event-collector/internal/infrastructure/sidecam"
	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/domain/valueobject"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	grpcTransport "github.com/argus-ai/event-collector/internal/transport/grpc"
	"github.com/argus-ai/event-collector/internal/transport/grpcweb"
	adminHTTP "github.com/argus-ai/event-collector/internal/transport/http"
	"github.com/argus-ai/event-collector/pkg/auth"
	"github.com/argus-ai/event-collector/pkg/circuitbreaker"
	"github.com/argus-ai/event-collector/pkg/cors"
	"github.com/argus-ai/event-collector/pkg/randutil"
	"github.com/argus-ai/event-collector/pkg/ratelimiter"
	httpMiddleware "github.com/argus-ai/event-collector/pkg/middleware"
	"github.com/argus-ai/event-collector/pkg/securityheaders"
)

// Build-time variables injected via ldflags:
//
//	go build -ldflags "-X main.version=1.0.0 -X main.buildTime=..."
var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "event-collector: fatal: %v\n", err)
		os.Exit(1)
	}
}

// run contains the real entry-point logic, returning an error instead of
// calling os.Exit directly. This pattern makes the startup sequence testable
// and ensures deferred functions execute on every exit path.
func run() error {
	// =================================================================
	// STEP 1: Parse CLI flags.
	// =================================================================
	configPath := flag.String("config", "deployments/config.yaml", "path to the YAML configuration file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("event-collector version=%s build_time=%s\n", version, buildTime)
		return nil
	}

	// =================================================================
	// STEP 2: Load configuration (YAML + environment overrides).
	// =================================================================
	cfg, err := config.LoadWithEnv(*configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// =================================================================
	// STEP 3: Initialise the structured logger.
	// =================================================================
	logger, err := buildLogger(cfg.Logger)
	if err != nil {
		return fmt.Errorf("failed to initialise logger: %w", err)
	}
	defer func() {
		// Sync flushes any buffered log entries. Ignoring the error is
		// intentional — stdout/stderr sync failures are expected on some
		// platforms and should not prevent a clean shutdown.
		_ = logger.Sync()
	}()

	logger.Info("starting event-collector",
		zap.String("version", version),
		zap.String("build_time", buildTime),
		zap.Int("grpc_port", cfg.Server.GRPCPort),
		zap.Int("http_port", cfg.Server.HTTPPort),
		zap.Bool("auth_enabled", cfg.Auth.Enabled),
	)

	// =================================================================
	// STEP 4: Initialise infrastructure: Kafka producer.
	// =================================================================
	kafkaProducer, err := kafka.NewProducer(kafka.ProducerConfig{
		Brokers:           cfg.Kafka.Brokers,
		RequiredAcks:      cfg.Kafka.RequiredAcks,
		MaxMessageBytes:   cfg.Kafka.MaxMessageBytes,
		FlushFrequency:    cfg.Kafka.FlushFrequency,
		FlushMessages:     cfg.Kafka.FlushMessages,
		RetryMax:          cfg.Kafka.RetryMax,
		CompressionCodec:  cfg.Kafka.CompressionCodec,
		IdempotentEnabled: cfg.Kafka.IdempotentEnabled,
	}, logger)
	if err != nil {
		return fmt.Errorf("failed to initialise kafka producer: %w", err)
	}
	logger.Info("kafka producer initialised",
		zap.Strings("brokers", cfg.Kafka.Brokers),
		zap.Bool("idempotent", cfg.Kafka.IdempotentEnabled),
		zap.String("compression", cfg.Kafka.CompressionCodec),
	)

	// =================================================================
	// STEP 4a: Ensure Kafka topics exist (idempotent).
	//
	// Explicitly creates topics with configured partition count and
	// replication factor. This replaces implicit auto-creation and
	// gives operators control over consumer parallelism and durability.
	// =================================================================
	if cfg.Kafka.AutoCreateTopics {
		defaultTopics := kafka.DefaultTopics(
			int32(cfg.Kafka.TopicPartitions),
			int16(cfg.Kafka.TopicReplication),
		)
		if topicErr := kafka.EnsureTopics(cfg.Kafka.Brokers, defaultTopics, logger); topicErr != nil {
			logger.Warn("kafka topic provisioning failed — topics may be managed externally",
				zap.Error(topicErr),
			)
			// Non-fatal: topics may already exist or be managed by Terraform/Strimzi
		}
	}

	// =================================================================
	// STEP 5: Initialise infrastructure: ClickHouse writer.
	// =================================================================
	chWriter, err := clickhouse.NewWriter(clickhouse.WriterConfig{
		Addrs:         cfg.ClickHouse.Addrs,
		Database:      cfg.ClickHouse.Database,
		Username:      cfg.ClickHouse.Username,
		Password:      cfg.ClickHouse.Password,
		BatchSize:     cfg.ClickHouse.BatchSize,
		FlushInterval: cfg.ClickHouse.FlushInterval,
		MaxRetries:    cfg.ClickHouse.MaxRetries,
	}, logger)
	if err != nil {
		return fmt.Errorf("failed to initialise clickhouse writer: %w", err)
	}
	logger.Info("clickhouse writer initialised",
		zap.Strings("addrs", cfg.ClickHouse.Addrs),
		zap.String("database", cfg.ClickHouse.Database),
		zap.Int("batch_size", cfg.ClickHouse.BatchSize),
	)

	// =================================================================
	// STEP 5a: Initialise ClickHouse Backfiller (ADR-007).
	//
	// Kafka consumer group that replays events from all three proctoring
	// topics into ClickHouse. ClickHouse ReplacingMergeTree deduplicates
	// replayed events — at-least-once delivery is safe.
	// =================================================================
	var chBackfiller *clickhouse.Backfiller
	if cfg.Backfiller.Enabled {
		chBackfiller, err = clickhouse.NewBackfiller(chWriter, clickhouse.BackfillerConfig{
			Brokers:       cfg.Kafka.Brokers,
			BatchSize:     cfg.Backfiller.BatchSize,
			FlushInterval: cfg.Backfiller.FlushInterval,
			PauseOnError:  cfg.Backfiller.PauseOnError,
		}, nil, logger) // alerter wired after Step 5c below
		if err != nil {
			logger.Warn("backfiller init failed — continuing without backfill",
				zap.Error(err),
			)
			chBackfiller = nil
		} else {
			logger.Info("clickhouse backfiller initialised (ADR-007)",
				zap.Int("batch_size", cfg.Backfiller.BatchSize),
				zap.Duration("flush_interval", cfg.Backfiller.FlushInterval),
			)
		}
	} else {
		logger.Info("clickhouse backfiller disabled (set EVENT_COLLECTOR_BACKFILLER_ENABLED=true to enable)")
	}

	// =================================================================
	// STEP 5c: Initialise Telegram emergency alerter (optional).
	// =================================================================
	telegramAlerter := alerting.NewTelegramProvider(alerting.TelegramConfig{
		BotToken: cfg.Telegram.BotToken,
		ChatID:   cfg.Telegram.ChatID,
	}, logger)

	// Wire alerter into data stores for critical failure notifications.
	if telegramAlerter != nil {
		chWriter.SetAlerter(telegramAlerter)
		kafkaProducer.SetAlerter(telegramAlerter)
	}

	// Start backfiller if initialised (must happen after alerter setup).
	if chBackfiller != nil {
		chBackfiller.Start()
	}

	// =================================================================
	// STEP 5d: Initialise DLQ (BadgerDB dead-letter queue).
	//
	// Wraps the Kafka producer with a ResilientWriter that catches write
	// failures and persists events to BadgerDB. A background goroutine
	// automatically drains events back to Kafka when it recovers.
	// =================================================================
	var kafkaEventWriter port.EventWriter = kafkaProducer
	var dlqStatusProvider health.DLQStatusProvider
	var dlqResilientWriter *dlq.ResilientWriter

	if cfg.DLQ.Enabled {
		dlqStore, dlqErr := dlq.NewStore(dlq.StoreConfig{
			DataDir:        cfg.DLQ.DataDir,
			GCInterval:     cfg.DLQ.GCInterval,
			GCDiscardRatio: cfg.DLQ.GCDiscardRatio,
			MaxEntries:     cfg.DLQ.MaxEntries,
			SyncWrites:     cfg.DLQ.SyncWrites,
		}, logger)
		if dlqErr != nil {
			return fmt.Errorf("failed to initialise dlq store: %w", dlqErr)
		}

		resilientWriter := dlq.NewResilientWriter(
			kafkaProducer,
			dlqStore,
			dlq.ResilientWriterConfig{
				ReclamationInterval:  cfg.DLQ.ReclamationInterval,
				ReclamationBatchSize: cfg.DLQ.ReclamationBatchSize,
				HeartbeatInterval:    cfg.DLQ.HeartbeatInterval,
				CircuitBreaker: circuitbreaker.Config{
					Name:             "kafka-dlq",
					FailureThreshold: cfg.DLQ.CircuitBreakerFailureThreshold,
					Timeout:          cfg.DLQ.CircuitBreakerTimeout,
					MaxRequests:      2,
					Interval:         60 * time.Second,
					OnOpenCallback: func(name string) {
						if telegramAlerter != nil {
							msg := alerting.MsgCircuitBreakerOpen(
								name,
								fmt.Sprintf("%d consecutive Kafka write failures", cfg.DLQ.CircuitBreakerFailureThreshold),
								fmt.Sprintf("Auto-probe in %s", cfg.DLQ.CircuitBreakerTimeout),
								time.Now().Format("2006-01-02 15:04:05 MST"),
							)
							_ = telegramAlerter.SendDirect(msg)
						}
					},
				},
			},
			logger,
		)

		if telegramAlerter != nil {
			resilientWriter.SetAlerter(telegramAlerter)
		}

		resilientWriter.Start()
		kafkaEventWriter = resilientWriter
		dlqStatusProvider = resilientWriter
		dlqResilientWriter = resilientWriter

		logger.Info("dlq enabled (BadgerDB dead-letter queue)",
			zap.String("data_dir", cfg.DLQ.DataDir),
			zap.Duration("reclamation_interval", cfg.DLQ.ReclamationInterval),
			zap.Int("reclamation_batch_size", cfg.DLQ.ReclamationBatchSize),
		)
	} else {
		logger.Warn("DLQ disabled — Kafka failures will reject events")
	}

	// =================================================================
	// STEP 5b: Initialise infrastructure: PostgreSQL (SaaS admin layer).
	// =================================================================
	pgRepo, err := postgres.NewRepository(postgres.Config{
		Host:            cfg.Postgres.Host,
		Port:            cfg.Postgres.Port,
		Database:        cfg.Postgres.Database,
		Username:        cfg.Postgres.Username,
		Password:        cfg.Postgres.Password,
		SSLMode:         cfg.Postgres.SSLMode,
		MaxOpenConns:    cfg.Postgres.MaxOpenConns,
		MaxIdleConns:    cfg.Postgres.MaxIdleConns,
		ConnMaxLifetime: cfg.Postgres.ConnMaxLifetime,
	}, logger)
	if err != nil {
		logger.Warn("PostgreSQL not available — admin API will be disabled",
			zap.Error(err),
			zap.String("host", cfg.Postgres.Host),
			zap.Int("port", cfg.Postgres.Port),
		)
		pgRepo = nil
	} else {
		logger.Info("postgresql repository initialised",
			zap.String("host", cfg.Postgres.Host),
			zap.Int("port", cfg.Postgres.Port),
			zap.String("database", cfg.Postgres.Database),
		)
	}

	// =================================================================
	// STEP 5c: Initialise infrastructure: MinIO (evidence storage).
	//
	// MinIO provides S3-compatible object storage for video evidence
	// fragments with Object Lock (WORM) for legal compliance.
	// Fail-soft: if MinIO is unavailable, evidence capture is disabled
	// but event ingestion continues normally.
	// =================================================================
	var evidenceStore *minioStore.Store
	minioStoreCfg := minioStore.StoreConfig{
		Endpoint:      cfg.MinIO.Endpoint,
		AccessKey:     cfg.MinIO.AccessKey,
		SecretKey:     cfg.MinIO.SecretKey,
		Bucket:        cfg.MinIO.Bucket,
		Region:        cfg.MinIO.Region,
		UseSSL:        cfg.MinIO.UseSSL,
		PresignTTL:    cfg.MinIO.PresignTTL,
		UploadTimeout: cfg.MinIO.UploadTimeout,
	}
	evidenceStore, err = minioStore.NewStore(minioStoreCfg, logger)
	if err != nil {
		logger.Warn("MinIO not available — evidence capture will be disabled",
			zap.Error(err),
			zap.String("endpoint", cfg.MinIO.Endpoint),
			zap.String("bucket", cfg.MinIO.Bucket),
		)
		evidenceStore = nil
	} else {
		logger.Info("minio evidence store initialised",
			zap.String("endpoint", cfg.MinIO.Endpoint),
			zap.String("bucket", cfg.MinIO.Bucket),
			zap.Duration("presign_ttl", cfg.MinIO.PresignTTL),
		)
	}

	// =================================================================
	// STEP 5d: Initialise Evidence Chain Writer & Recorder.
	//
	// The chain writer records evidence metadata to ClickHouse + Kafka.
	// The recorder manages per-session ring buffers and orchestrates
	// evidence capture on violation events.
	// Only enabled if MinIO is available.
	// =================================================================
	var evidenceRecorder *recorder.Recorder
	var chunkAssembler *chunk.Assembler
	if evidenceStore != nil && chWriter != nil {
		chainWriter := evidence.NewChainWriter(chWriter.Conn(), kafkaProducer, logger)

		evidenceRecorder = recorder.New(evidenceStore, chainWriter, recorder.DefaultConfig(), logger)

		logger.Info("evidence recorder initialised",
			zap.Int("window_sec", recorder.DefaultConfig().WindowSec),
			zap.Float64("pre_capture_sec", recorder.DefaultConfig().PreCaptureSec),
			zap.Float64("post_capture_sec", recorder.DefaultConfig().PostCaptureSec),
			zap.Int("max_concurrent_uploads", recorder.DefaultConfig().MaxConcurrentUploads),
		)
	} else {
		logger.Warn("evidence recorder DISABLED — MinIO or ClickHouse not available")
	}

	// =================================================================
	// STEP 6: Initialise application layer: IngestUseCase.
	// =================================================================
	var ingestOpts []usecase.IngestOption
	if evidenceRecorder != nil {
		ingestOpts = append(ingestOpts, usecase.WithRecorder(evidenceRecorder))
	}
	ingestUC := usecase.NewIngestUseCase(kafkaEventWriter, chWriter, logger, ingestOpts...)
	logger.Info("ingest use-case initialised",
		zap.Bool("evidence_capture_enabled", evidenceRecorder != nil),
	)

	// =================================================================
	// STEP 7: Initialise Worker Pool for bounded-concurrency stream processing.
	// =================================================================
	workerPool := grpcTransport.NewWorkerPool(cfg.WorkerPool.Size, ingestUC, logger)
	logger.Info("worker pool initialised",
		zap.Int("workers", cfg.WorkerPool.Size),
		zap.Int("queue_size", cfg.WorkerPool.QueueSize),
	)

	// =================================================================
	// STEP 8: Initialise JWT Verifier (conditional on auth.enabled).
	//
	// When auth is disabled (local development), the interceptor chain
	// skips JWT verification entirely. In production, auth MUST be enabled
	// via config or the EVENT_COLLECTOR_JWT_SIGNING_KEY env variable.
	// =================================================================
	var jwtVerifier *auth.Verifier
	if cfg.Auth.Enabled {
		// Determine algorithm — default to RS256 (asymmetric, recommended).
		algorithm := cfg.Auth.Algorithm
		if algorithm == "" {
			algorithm = auth.AlgorithmRS256
		}

		verifierCfg := auth.VerifierConfig{
			Algorithm:            algorithm,
			PublicKeyPath:        cfg.Auth.PublicKeyPath,
			PrivateKeyPath:       cfg.Auth.PrivateKeyPath,
			SigningKeyBase64:     cfg.Auth.SigningKey, // Legacy HS256 fallback.
			Issuer:               cfg.Auth.Issuer,
			Audience:             cfg.Auth.Audience,
			ClockSkew:            cfg.Auth.ClockSkew,
			RequireSessionSecret: cfg.Auth.RequireSessionSecret,
		}

		jwtVerifier, err = auth.NewVerifier(verifierCfg)
		if err != nil {
			return fmt.Errorf("failed to initialise jwt verifier: %w", err)
		}
		logger.Info("jwt verifier initialised",
			zap.String("algorithm", jwtVerifier.Algorithm()),
			zap.Bool("can_sign", jwtVerifier.CanSign()),
			zap.String("issuer", cfg.Auth.Issuer),
			zap.String("audience", cfg.Auth.Audience),
			zap.Duration("clock_skew", cfg.Auth.ClockSkew),
			zap.Bool("require_session_secret", cfg.Auth.RequireSessionSecret),
		)
	} else {
		logger.Warn("JWT authentication is DISABLED — all requests are accepted without verification",
			zap.String("note", "Set auth.enabled=true and provide RSA key paths for production"),
		)
	}

	// =================================================================
	// STEP 9: Initialise Session Validator (Eduser integration).
	//
	// The session validator caches Eduser session lookups in memory
	// with TTL-based expiry. In development mode (no Eduser endpoint),
	// it uses a noop validator that accepts all sessions.
	// =================================================================
	var sessionValidator *session.CachedValidator
	if cfg.Session.EduserEndpoint != "" {
		// Production mode: validate sessions against Eduser API.
		// TODO: Replace this stub with a real gRPC client to Eduser.
		// The eduserClient function signature matches the CachedValidator's
		// constructor parameter for easy swapping.
		eduserClient := func(ctx context.Context, sessionID string, sessionSecret string) error {
			// Stub implementation — replace with real Eduser gRPC call:
			//   resp, err := eduserGRPCClient.ValidateSession(ctx, &eduser.ValidateSessionRequest{
			//       SessionId:     sessionID,
			//       SessionSecret: sessionSecret,
			//   })
			//   if err != nil || !resp.Valid {
			//       return fmt.Errorf("session %s is not valid", sessionID)
			//   }
			//   return nil
			_ = ctx
			_ = sessionID
			_ = sessionSecret
			return nil // Accept all sessions until Eduser client is integrated.
		}
		sessionValidator = session.NewCachedValidator(session.ValidatorConfig{
			CacheTTL:         cfg.Session.CacheTTL,
			NegativeCacheTTL: cfg.Session.NegativeCacheTTL,
			CleanupInterval:  cfg.Session.CleanupInterval,
			EduserEndpoint:   cfg.Session.EduserEndpoint,
			EduserTimeout:    cfg.Session.EduserTimeout,
		}, logger, eduserClient)
		logger.Info("session validator initialised (production mode)",
			zap.String("eduser_endpoint", cfg.Session.EduserEndpoint),
			zap.Duration("cache_ttl", cfg.Session.CacheTTL),
		)
	} else {
		sessionValidator = session.NewNoopValidator(logger)
		logger.Warn("session validator in NOOP mode — all sessions accepted",
			zap.String("note", "Set session.eduser_endpoint for production"),
		)
	}

	// =================================================================
	// STEP 10: Initialise gRPC interceptor chain.
	//
	// Chain order: Recovery → RequestID → Metrics → Dedup → RateLimit → Auth → Logging
	//
	// When auth is disabled, the Auth interceptor is still in the chain
	// but passes through all requests (jwtVerifier is nil → skip auth).
	// =================================================================
	unaryLimiter := rate.NewLimiter(rate.Limit(cfg.RateLimit.GlobalRPS), cfg.RateLimit.GlobalBurst)
	streamLimiter := rate.NewLimiter(rate.Limit(cfg.RateLimit.GlobalRPS), cfg.RateLimit.GlobalBurst)

	// Idempotency cache — prevents duplicate event ingestion during
	// frontend transport retries. Keys expire after 5 minutes.
	idempCache := grpcTransport.NewIdempotencyCache(5*time.Minute, logger)

	unaryInterceptors, streamInterceptors := grpcTransport.NewInterceptorChain(
		logger, unaryLimiter, streamLimiter, jwtVerifier, sessionValidator, idempCache,
	)

	logger.Info("interceptor chain configured",
		zap.String("unary_rate_limit", grpcTransport.FormatRateLimit(unaryLimiter)),
		zap.String("stream_rate_limit", grpcTransport.FormatRateLimit(streamLimiter)),
		zap.Int("chain_length_unary", len(unaryInterceptors)),
		zap.Int("chain_length_stream", len(streamInterceptors)),
	)

	// =================================================================
	// STEP 11: Create gRPC server with keepalive and interceptors.
	//
	// Keepalive parameters are tuned for proctoring sessions that can
	// last 1-4 hours. MaxConnectionAge forces periodic reconnection to
	// distribute load evenly across horizontally-scaled pods after
	// a rolling deployment.
	// =================================================================
	kaParams := keepalive.ServerParameters{
		MaxConnectionIdle:     5 * time.Minute,    // Close idle connections after 5 min.
		MaxConnectionAge:      30 * time.Minute,   // Force reconnect after 30 min (load balancing).
		MaxConnectionAgeGrace: 10 * time.Second,   // Allow 10s for in-flight RPCs to complete.
		Time:                  1 * time.Minute,    // Send keepalive ping every 1 min.
		Timeout:               20 * time.Second,   // Wait 20s for keepalive ACK.
	}

	kaPolicy := keepalive.EnforcementPolicy{
		MinTime:             10 * time.Second,  // Minimum time between client pings.
		PermitWithoutStream: true,              // Allow pings even without active streams.
	}

	grpcServer := grpc.NewServer(
		// Keepalive.
		grpc.KeepaliveParams(kaParams),
		grpc.KeepaliveEnforcementPolicy(kaPolicy),

		// Message size limits (4 MiB).
		grpc.MaxRecvMsgSize(4*1024*1024),
		grpc.MaxSendMsgSize(4*1024*1024),

		// Max concurrent streams for 10,000+ proctoring sessions.
		grpc.MaxConcurrentStreams(10_000),

		// Interceptor chains: Recovery -> RateLimit -> Auth -> Logging.
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	)

	// =================================================================
	// STEP 12: Register the EventCollectorService implementation.
	// =================================================================
	transportServer := grpcTransport.NewServer(ingestUC, workerPool, logger)
	pb.RegisterEventCollectorServiceServer(grpcServer, transportServer)

	logger.Info("grpc server created",
		zap.Int("max_concurrent_streams", 10_000),
		zap.Int("max_recv_msg_size_bytes", 4*1024*1024),
	)

	// =================================================================
	// STEP 13: Create unified HTTP server.
	//
	// A single HTTP listener serves:
	//   - gRPC-Web requests (Content-Type: application/grpc-web)
	//   - Health checks (/healthz, /readyz)
	//   - Future: Prometheus metrics (/metrics)
	//
	// Request routing:
	//   gRPC-Web Content-Type? → gRPC server (via grpcweb proxy)
	//   /healthz, /readyz     → health handler
	//   Everything else       → 404
	//
	// CORS middleware wraps everything to allow cross-origin requests
	// from the Nuxt 3 frontend.
	// =================================================================

	// Health handler with tagged dependency checks:
	//   - Kafka is critical: failure → 503 (events cannot be ingested)
	//   - ClickHouse is non-critical: failure → degraded (analytics delayed, Kafka retains data)
	healthCheckers := []health.TaggedChecker{
		health.Critical(kafkaProducer),
		health.NonCritical(chWriter),
	}
	if chBackfiller != nil {
		healthCheckers = append(healthCheckers, health.NonCritical(chBackfiller))
	}
	healthHandler := health.NewHandler(logger, healthCheckers...)
	if dlqStatusProvider != nil {
		healthHandler.SetDLQProvider(dlqStatusProvider)
	}

	// HTTP mux for non-gRPC routes.
	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/healthz", healthHandler.LivenessHandler())
	httpMux.HandleFunc("/readyz", healthHandler.ReadinessHandler())

	// Admin API endpoints (REST) — only if PostgreSQL is available.
	var sidecamOrchestrator *sidecam.Orchestrator
	if pgRepo != nil {
		// JWT signing key for admin tokens — reuse the auth signing key,
		// or use a separate key if auth is disabled.
		adminJWTKey := []byte(cfg.Auth.SigningKey)
		if len(adminJWTKey) == 0 {
			adminJWTKey = []byte("argus-dev-admin-jwt-key-CHANGE-IN-PRODUCTION!")
		}

		// =============================================================
		// Asynq Client (Redis-backed job queue) — optional.
		// When cfg.Redis.Addr is non-empty, export and forensic PDF
		// jobs are dispatched to the standalone cmd/worker binary via
		// Redis. When empty, the legacy PostgreSQL polling worker and
		// synchronous PDF generation remain the default.
		// =============================================================
		var asynqClient *asynq.Client
		if cfg.Redis.Addr != "" {
			asynqClient = asynq.NewClient(asynq.RedisClientOpt{
				Addr:     cfg.Redis.Addr,
				Password: cfg.Redis.Password,
				DB:       cfg.Redis.DB,
			})
			defer asynqClient.Close()
			logger.Info("asynq client connected",
				zap.String("redis_addr", cfg.Redis.Addr),
				zap.Int("redis_db", cfg.Redis.DB),
			)
		}

		adminHandler := adminHTTP.NewAdminHandler(pgRepo, logger, adminJWTKey)
		adminHandler.RegisterRoutes(httpMux)

		// Analytics API endpoints (REST) — queries ClickHouse materialized views.
		// Powers the Executive Dashboard: Risk Distribution, System Health, etc.
		if chWriter != nil {
			analyticsHandler := adminHTTP.NewAnalyticsHandler(chWriter, pgRepo, logger, adminJWTKey)
			analyticsHandler.RegisterRoutes(httpMux)

			logger.Info("analytics api registered",
				zap.String("base_path", "/api/v1/analytics"),
				zap.Int("endpoints", 6),
			)

			// Monitoring API endpoints (REST) — live session monitoring with risk scores.
			// Powers the Monitoring dashboard: active sessions, warn, terminate.
			monitoringHandler := adminHTTP.NewMonitoringHandler(chWriter, pgRepo, logger, adminJWTKey)
			monitoringHandler.RegisterRoutes(httpMux)

			// Auto-create session_terminations table.
			if err := pgRepo.EnsureSessionTerminationsTable(context.Background()); err != nil {
				logger.Error("failed to create session_terminations table", zap.Error(err))
			} else {
				logger.Info("session_terminations table ensured")
			}

			// Auto-create review_decisions table.
			if err := pgRepo.EnsureReviewDecisionsTable(context.Background()); err != nil {
				logger.Error("failed to create review_decisions table", zap.Error(err))
			} else {
				logger.Info("review_decisions table ensured")
			}

			logger.Info("monitoring api registered",
				zap.String("base_path", "/api/v1/monitoring"),
				zap.Int("endpoints", 3),
			)

			// =============================================================
			// v4.0 — Exam Proctoring Settings API (Dynamic Config Registry)
			// Per-exam rule configuration, toggle-to-penalty mapping, and
			// configurable verdict thresholds.
			// =============================================================
			if err := pgRepo.EnsureExamProctoringSettingsTable(context.Background()); err != nil {
				logger.Error("failed to create exam_proctoring_settings table", zap.Error(err))
			} else {
				logger.Info("exam_proctoring_settings table ensured")
			}

			proctoringSettingsHandler := adminHTTP.NewProctoringSettingsHandler(pgRepo, logger, adminJWTKey)
			proctoringSettingsHandler.RegisterRoutes(httpMux)

			logger.Info("proctoring settings api registered",
				zap.String("base_path", "/api/v1/proctoring"),
				zap.Int("endpoints", 5),
			)

			// =============================================================
			// v4.1 — SSE Real-Time Session Stream
			// Push-based live monitoring with auto-terminate + Telegram alerting.
			// =============================================================
			sseScorer := forensic.NewScorer(chWriter.Conn(), logger)
			sseHandler := adminHTTP.NewSSEHandler(
				chWriter.Conn(), sseScorer, pgRepo, telegramAlerter, logger, adminJWTKey,
			)
			sseHandler.RegisterRoutes(httpMux)

			logger.Info("sse stream api registered",
				zap.String("base_path", "/api/v1/monitoring/sessions/*/stream"),
				zap.Int("endpoints", 1),
			)
		}

		// Media API endpoints (REST) — LiveKit WebRTC token generation.
		// Powers the Monitoring dashboard video streaming integration.
		mediaHandler := adminHTTP.NewMediaHandler(pgRepo, logger, adminJWTKey)
		mediaHandler.RegisterRoutes(httpMux)

		logger.Info("media api registered",
			zap.String("base_path", "/api/v1/media"),
			zap.Int("endpoints", 2),
		)

		// Archive API endpoints (REST) — Historical session review and export.
		// Powers the "Архив сессий" dashboard with ClickHouse queries.
		archiveHandler := adminHTTP.NewArchiveHandler(chWriter, pgRepo, logger, adminJWTKey)
		archiveHandler.RegisterRoutes(httpMux)

		logger.Info("archive api registered",
			zap.String("base_path", "/api/v1/archive"),
			zap.Int("endpoints", 4),
		)

		// Evidence API endpoints (REST) — Evidence fragment retrieval, presigned URLs.
		// Conditional: only if MinIO evidence store is available.
		if evidenceStore != nil {
			evidenceHandler := adminHTTP.NewEvidenceHandler(chWriter, evidenceStore, pgRepo, logger, adminJWTKey)
			evidenceHandler.RegisterRoutes(httpMux)

			logger.Info("evidence api registered",
				zap.String("base_path", "/api/v1/evidence"),
				zap.Int("endpoints", 3),
			)

			// Chunked Upload API endpoints (REST) — Receives chunked binary evidence
			// from browser clients in degraded network conditions (Tier B/C).
			// Assembles chunks → verifies SHA-256 → uploads to MinIO.
			chunkAssembler = chunk.NewAssembler(evidenceStore, chunk.Config{
				MaxChunkSize:            cfg.Chunk.MaxChunkSize,
				MaxFragmentSize:         cfg.Chunk.MaxFragmentSize,
				ChunkTTL:                cfg.Chunk.ChunkTTL,
				MaxConcurrentAssemblies: cfg.Chunk.MaxConcurrentAssemblies,
			}, logger)
			chunkAssembler.SetPublisher(kafkaProducer)

			chunkHandler := adminHTTP.NewChunkHandler(chunkAssembler, pgRepo, logger, adminJWTKey)
			chunkHandler.RegisterRoutes(httpMux)

			logger.Info("chunk upload api registered",
				zap.String("base_path", "/api/v1/ingest/chunk"),
				zap.Int("endpoints", 2),
				zap.Int("max_chunk_size", cfg.Chunk.MaxChunkSize),
				zap.Int64("max_fragment_size", cfg.Chunk.MaxFragmentSize),
			)
		}

		// =============================================================
		// v2.1 — Integrity Verification API
		// Cross-references S3 evidence against ClickHouse forensic ledger.
		// =============================================================
		if evidenceStore != nil && chWriter != nil {
			integrityVerifier := integrityInfra.NewVerifier(chWriter.Conn(), evidenceStore, logger)
			integrityHandler := adminHTTP.NewIntegrityHandler(integrityVerifier, pgRepo, logger, adminJWTKey)
			integrityHandler.RegisterRoutes(httpMux)

			logger.Info("integrity api registered",
				zap.String("base_path", "/api/v1/integrity"),
				zap.Int("endpoints", 3),
			)

			// =============================================================
			// v3.0 — Forensic Reporting API
			// AI-generated integrity scores, PDF reports, gaze heatmaps,
			// voice biometric analysis, and report hash verification.
			// =============================================================
			forensicHandler := adminHTTP.NewForensicHandler(
				chWriter.Conn(), integrityVerifier, pgRepo, logger, adminJWTKey, asynqClient,
			)
			forensicHandler.RegisterRoutes(httpMux)

			logger.Info("forensic api registered",
				zap.String("base_path", "/api/v1/forensic"),
				zap.Int("endpoints", 7),
			)
		}

		// =============================================================
		// v4.2 — AI Deep Analysis Trigger API
		// Allows admins to trigger backend GPU-accelerated analysis
		// for a specific session via POST /api/v1/sessions/:id/analyze.
		// =============================================================
		if asynqClient != nil {
			aiAnalysisHandler := adminHTTP.NewAIAnalysisHandler(pgRepo, asynqClient, logger, adminJWTKey)
			aiAnalysisHandler.RegisterRoutes(httpMux)

			logger.Info("ai analysis api registered",
				zap.String("base_path", "/api/v1/sessions/*/analyze"),
				zap.Int("endpoints", 1),
			)
		}

		// =============================================================
		// v3.1 — Secondary Camera (Mobile) Orchestration API
		// QR pairing, spatial calibration, device telemetry,
		// stream health, and session gating for mobile secondary cameras.
		// =============================================================

		// Fix 7: Event emitter closure — bridges sidecam anomalies into ClickHouse
		sidecamEventEmitter := func(sessionID, studentID, examID, orgID, eventType, severity, label string) {
			// Map string event type to valueobject.EventType constant
			etMap := map[string]valueobject.EventType{
				"SIDECAM_DEVICE_DISPLACED":    valueobject.SidecamDeviceDisplaced,
				"SIDECAM_HANDS_OFF_DESK":      valueobject.SidecamHandsOffDesk,
				"SIDECAM_BATTERY_CRITICAL":    valueobject.SidecamBatteryCritical,
				"SIDECAM_STREAM_DISCONNECTED": valueobject.SidecamStreamDisconnected,
				"SIDECAM_CALIBRATION_FAILED":  valueobject.SidecamCalibrationFailed,
				"SIDECAM_THERMAL_THROTTLE":    valueobject.SidecamThermalThrottle,
			}

			sevMap := map[string]valueobject.Severity{
				"critical": valueobject.SeverityCritical,
				"warning":  valueobject.SeverityWarning,
				"info":     valueobject.SeverityInfo,
			}

			et, ok := etMap[eventType]
			if !ok {
				logger.Warn("sidecam: unknown event type for emission", zap.String("event_type", eventType))
				return
			}
			sev := sevMap[severity]
			if sev == valueobject.SeverityUnspecified {
				sev = valueobject.SeverityWarning
			}

			// Generate a unique event ID
			eid := randutil.HexToken(16)

			now := time.Now().UTC()
			evt := &entity.ProctoringEvent{
				EventID:        eid,
				SessionID:      sessionID,
				StudentID:      studentID,
				ExamID:         examID,
				OrgID:          orgID,
				EventType:      et,
				Severity:       sev,
				Source:         valueobject.SourceSideCamera,
				ServerTimestamp: now,
				ClientTimestamp: now,
				Label:          label,
				Confidence:     1.0,
			}

			if chWriter != nil {
				if err := chWriter.Write(context.Background(), evt); err != nil {
					logger.Error("sidecam: failed to write event to clickhouse",
						zap.String("session_id", sessionID),
						zap.String("event_type", eventType),
						zap.Error(err),
					)
				}
			}
		}

		sidecamOrchestrator = sidecam.NewOrchestrator(logger, adminJWTKey, fmt.Sprintf(":%d", cfg.Server.HTTPPort), sidecamEventEmitter)
		sidecamHandler := adminHTTP.NewSidecamHandler(sidecamOrchestrator, pgRepo, logger, adminJWTKey)
		sidecamHandler.RegisterRoutes(httpMux)

		logger.Info("sidecam api registered",
			zap.String("base_path", "/api/v1/sidecam"),
			zap.Int("endpoints", 9),
		)

		// =============================================================
		// v2.1 — Bulk Export API + Background Worker
		// Async TAR.GZ archive creation with forensic manifest.
		// =============================================================
		exportHandler := adminHTTP.NewExportHandler(pgRepo.DB(), cfg.Export, pgRepo, logger, adminJWTKey, asynqClient)
		exportHandler.RegisterRoutes(httpMux)

		logger.Info("export api registered",
			zap.String("base_path", "/api/v1/export"),
			zap.Int("endpoints", 4),
		)

		// Start export background worker.
		if evidenceStore != nil && chWriter != nil {
			exportWorker := exportInfra.NewWorker(
				pgRepo.DB(), chWriter.Conn(), minioStore.Client(evidenceStore),
				cfg.Export, cfg.MinIO.Bucket, logger,
			)
			go exportWorker.Start(context.Background())
			defer exportWorker.Stop()

			logger.Info("export worker started",
				zap.Duration("poll_interval", cfg.Export.PollInterval),
			)
		}

		// =============================================================
		// v2.1 — Consent API
		// Student consent recording for GDPR/privacy compliance.
		// =============================================================
		consentHandler := adminHTTP.NewConsentHandler(pgRepo.DB(), pgRepo, logger, adminJWTKey)
		consentHandler.RegisterRoutes(httpMux)

		logger.Info("consent api registered",
			zap.String("base_path", "/api/v1/consent"),
			zap.Int("endpoints", 2),
		)

		// =============================================================
		// v2.1 — Appeals Workflow API
		// Student appeals with state machine transitions.
		// =============================================================
		appealsHandler := adminHTTP.NewAppealsHandler(pgRepo.DB(), pgRepo, logger, adminJWTKey)
		appealsHandler.RegisterRoutes(httpMux)

		logger.Info("appeals api registered",
			zap.String("base_path", "/api/v1/appeals"),
			zap.Int("endpoints", 4),
		)

		// =============================================================
		// v2.1 — Prometheus Metrics
		// =============================================================
		metricsCollector := metricsInfra.NewCollector()
		metricsCollector.Register()

		httpMux.Handle("GET /metrics", promhttp.Handler())

		logger.Info("prometheus metrics registered",
			zap.String("endpoint", "/metrics"),
		)

		// =============================================================
		// pprof debug endpoints (guarded by config flag)
		//
		// MUST be disabled in production. Only enable for debugging.
		// Exposes Go runtime profiling at /debug/pprof/*.
		// =============================================================
		if cfg.Server.EnablePprof {
			httpMux.HandleFunc("GET /debug/pprof/", pprofHTTP.Index)
			httpMux.HandleFunc("GET /debug/pprof/cmdline", pprofHTTP.Cmdline)
			httpMux.HandleFunc("GET /debug/pprof/profile", pprofHTTP.Profile)
			httpMux.HandleFunc("GET /debug/pprof/symbol", pprofHTTP.Symbol)
			httpMux.HandleFunc("GET /debug/pprof/trace", pprofHTTP.Trace)

			logger.Warn("pprof debug endpoints enabled — DISABLE IN PRODUCTION",
				zap.String("endpoint", "/debug/pprof/"),
			)
		}

		// =============================================================
		// Temporary diagnostic: test-alert endpoint
		// =============================================================
		httpMux.HandleFunc("POST /api/v1/internal/test-alert", func(w http.ResponseWriter, r *http.Request) {
			if telegramAlerter == nil {
				http.Error(w, `{"error":"telegram alerter not configured"}`, http.StatusServiceUnavailable)
				return
			}
			msg := alerting.MsgDiagnosticTest(time.Now().Format("2006-01-02 15:04:05 MST"))
			if err := telegramAlerter.SendDirect(msg); err != nil {
				logger.Error("test-alert: send failed", zap.Error(err))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				fmt.Fprintf(w, `{"error":"%s"}`, err.Error())
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"sent","message":"Test alert fired to Telegram"}`)
		})

		// =============================================================
		// System Health & Panic Button (Gap 5 — Indestructible)
		// Admin-only endpoints for system health audit and emergency
		// recovery: breaker reset, DLQ clear, overflow flush.
		// =============================================================
		systemHandler := adminHTTP.NewSystemHandler(dlqResilientWriter, chWriter, logger, adminJWTKey, adminHandler)
		systemHandler.RegisterRoutes(httpMux)

		logger.Info("system health api registered",
			zap.String("base_path", "/api/v1/admin/system-health"),
			zap.Int("endpoints", 2),
		)

		logger.Info("admin api registered",
			zap.String("base_path", "/api/v1"),
			zap.Int("endpoints", 34),
		)
	}

	// =================================================================
	// HTTP Rate Limiting for Admin API.
	//
	// This is separate from the gRPC rate limiter because the admin API
	// has different throughput requirements. The middleware enforces both
	// global (all clients) and per-IP limits using token-bucket algorithm.
	// =================================================================
	httpRateLimiter := ratelimiter.NewHTTPRateLimiter(
		cfg.RateLimit.HTTPGlobalRPS,
		cfg.RateLimit.HTTPGlobalBurst,
		cfg.RateLimit.HTTPPerIPRPS,
		cfg.RateLimit.HTTPPerIPBurst,
		logger,
	)
	rateLimitedMux := httpRateLimiter.Middleware(httpMux)

	logger.Info("http rate limiter configured",
		zap.Float64("global_rps", cfg.RateLimit.HTTPGlobalRPS),
		zap.Int("global_burst", cfg.RateLimit.HTTPGlobalBurst),
		zap.Float64("per_ip_rps", cfg.RateLimit.HTTPPerIPRPS),
		zap.Int("per_ip_burst", cfg.RateLimit.HTTPPerIPBurst),
	)

	// gRPC-Web proxy wrapping the gRPC server with rate-limited HTTP mux fallback.
	grpcWebProxy := grpcweb.NewHandler(grpcServer, rateLimitedMux, logger)

	// CORS middleware — wraps the gRPC-Web proxy.
	corsConfig := cors.Config{
		AllowedOrigins:   cfg.CORS.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{
			"Accept",
			"Accept-Language",
			"Content-Type",
			"Content-Language",
			"Authorization",
			"X-Auth-Token",
			"X-Grpc-Web",
			"X-User-Agent",
			"X-Requested-With",
			"Grpc-Timeout",
			"X-Argus-Session-Id",
			"X-Idempotency-Key",
		},
		ExposedHeaders: []string{
			"Grpc-Status",
			"Grpc-Message",
			"Grpc-Status-Details-Bin",
			"X-Argus-Request-Id",
			"Retry-After",
			"X-RateLimit-Retry-After-Ms",
		},
		MaxAge:           cfg.CORS.MaxAge,
		AllowCredentials: cfg.CORS.AllowCredentials,
	}
	// ---- Debug Middleware to log all requests ----
	loggingMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("INCOMING HTTP REQUEST", zap.String("method", r.Method), zap.String("url", r.URL.String()), zap.String("path", r.URL.Path))
		grpcWebProxy.ServeHTTP(w, r) // Pass the request to the next handler in the chain
	})

	corsHandler := cors.Middleware(corsConfig)(loggingMux)

	// Security headers middleware — wraps the full handler stack.
	// TLS is enabled when HTTPS is terminated at this server (not at Nginx).
	// In production, TLS termination is handled by Nginx, so TLS=false here.
	secHeadersHandler := securityheaders.Middleware(securityheaders.Config{
		TLS: cfg.Server.TLS,
	})(corsHandler)

	// Observability middleware chain (outermost → innermost):
	//   RequestID → Metrics → SecurityHeaders → CORS → gRPC-Web → Mux
	metricsHandler := httpMiddleware.MetricsHTTPMiddleware(secHeadersHandler)
	requestIDHandler := httpMiddleware.RequestIDHTTPMiddleware(metricsHandler)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.HTTPPort),
		Handler:           requestIDHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       0,               // Disabled for streaming (gRPC-Web streams can be long-lived).
		WriteTimeout:      0,               // Disabled for streaming.
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 16,         // 64 KiB (accommodates large JWT tokens).
	}

	logger.Info("http server created (gRPC-Web + CORS + health)",
		zap.Int("port", cfg.Server.HTTPPort),
		zap.Strings("cors_origins", cfg.CORS.AllowedOrigins),
		zap.Bool("cors_credentials", cfg.CORS.AllowCredentials),
	)

	// =================================================================
	// STEP 13b: Org Lifecycle Cleanup (background job).
	//
	// The cleanup use case periodically scans for orgs in "purged" state
	// and permanently removes their data from PostgreSQL and optionally
	// ClickHouse. Disabled by default — opt-in via config.
	// =================================================================
	var cleanupUC *usecase.CleanupUseCase
	if cfg.Cleanup.Enabled && pgRepo != nil {
		cleanupCfg := usecase.CleanupConfig{
			Enabled:         cfg.Cleanup.Enabled,
			RunInterval:     cfg.Cleanup.RunInterval,
			GracePeriodDays: cfg.Cleanup.GracePeriodDays,
			ClickHousePurge: cfg.Cleanup.ClickHousePurge,
		}

		// ClickHouse purger is optional — only set if writer is available.
		var chPurger usecase.ClickHousePurger
		if chWriter != nil && cfg.Cleanup.ClickHousePurge {
			chPurger = chWriter
		}

		cleanupUC = usecase.NewCleanupUseCase(pgRepo, chPurger, logger, cleanupCfg)
		go cleanupUC.Start(context.Background())

		logger.Info("org lifecycle cleanup job started",
			zap.Duration("interval", cfg.Cleanup.RunInterval),
			zap.Int("grace_period_days", cfg.Cleanup.GracePeriodDays),
			zap.Bool("clickhouse_purge", cfg.Cleanup.ClickHousePurge),
		)
	} else {
		logger.Info("org lifecycle cleanup job DISABLED (opt-in via cleanup.enabled)")
	}

	// =================================================================
	// STEP 14: Set up signal handler for graceful shutdown.
	// =================================================================
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start background cleanup of stale HTTP rate limiter IP entries (every 5 minutes).
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				httpRateLimiter.Cleanup(30 * time.Minute)
			}
		}
	}()

	// =================================================================
	// STEP 15: Start gRPC server in background goroutine.
	// =================================================================
	errCh := make(chan error, 2)

	go func() {
		grpcAddr := fmt.Sprintf(":%d", cfg.Server.GRPCPort)
		lis, err := net.Listen("tcp", grpcAddr)
		if err != nil {
			errCh <- fmt.Errorf("failed to listen on %s: %w", grpcAddr, err)
			return
		}
		logger.Info("grpc server listening",
			zap.String("addr", grpcAddr),
			zap.String("protocol", "native gRPC (H2C)"),
		)
		if err := grpcServer.Serve(lis); err != nil {
			errCh <- fmt.Errorf("grpc server error: %w", err)
		}
	}()

	// =================================================================
	// STEP 16: Start HTTP server in background goroutine.
	// =================================================================
	go func() {
		logger.Info("http server listening",
			zap.String("addr", httpServer.Addr),
			zap.String("serves", "gRPC-Web, /healthz, /readyz"),
		)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("http server error: %w", err)
		}
	}()

	// =================================================================
	// STEP 16b: Telegram startup hello (non-blocking).
	// =================================================================
	if telegramAlerter != nil {
		go func() {
			msg := alerting.MsgServerStartup(time.Now().Format("2006-01-02 15:04:05 MST"))
			if err := telegramAlerter.SendDirect(msg); err != nil {
				logger.Warn("startup hello failed", zap.Error(err))
			}
		}()
	}

	// =================================================================
	// STEP 17: Block until shutdown signal or fatal server error.
	// =================================================================
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining...")
	case err := <-errCh:
		logger.Error("server failed, initiating shutdown", zap.Error(err))
	}

	// =================================================================
	// STEP 18: Graceful shutdown in priority order.
	//
	// The shutdown sequence is carefully ordered to prevent data loss:
	//
	//   Phase 1 — Stop accepting new connections:
	//     (a) gRPC server GracefulStop() — drains in-flight RPCs.
	//     (b) Worker pool Close() — drains pending jobs in the queue.
	//
	//   Phase 2 — Stop HTTP:
	//     (c) HTTP server Shutdown() — drains in-flight HTTP/gRPC-Web.
	//
	//   Phase 3 — Stop background services:
	//     (d) Session validator Close() — stops cache cleanup goroutine.
	//
	//   Phase 4 — Flush data stores:
	//     (e) IngestUseCase Close() → Kafka AsyncClose + wg.Wait,
	//         then ClickHouse final flush + overflow drain.
	//
	// Each phase gets a fraction of the total shutdown budget.
	// The shutdown timeout is typically 30 seconds (matching Kubernetes
	// terminationGracePeriodSeconds).
	// =================================================================
	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(), cfg.Server.ShutdownTimeout,
	)
	defer shutdownCancel()

	// Phase 1a: Stop gRPC server.
	logger.Info("phase 1a: stopping grpc server...")
	grpcShutdownDone := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(grpcShutdownDone)
	}()

	select {
	case <-grpcShutdownDone:
		logger.Info("grpc server stopped gracefully")
	case <-shutdownCtx.Done():
		logger.Warn("grpc server graceful stop timed out, forcing stop")
		grpcServer.Stop()
	}

	// Phase 1b: Drain worker pool.
	logger.Info("phase 1b: draining worker pool...")
	workerPool.Close()
	logger.Info("worker pool drained")

	// Phase 2: Stop HTTP server.
	logger.Info("phase 2: stopping http server...")
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Warn("http server shutdown error", zap.Error(err))
	} else {
		logger.Info("http server stopped gracefully")
	}

	// Phase 3: Stop session validator.
	logger.Info("phase 3: stopping session validator...")
	sessionValidator.Close()
	logger.Info("session validator stopped")

	// Phase 3b: Close PostgreSQL connection pool.
	if pgRepo != nil {
		logger.Info("phase 3b: closing postgresql...")
		if err := pgRepo.Close(); err != nil {
			logger.Warn("postgresql close error", zap.Error(err))
		} else {
			logger.Info("postgresql closed")
		}
	}

	// Phase 3c: Close evidence recorder (releases ring buffers).
	if evidenceRecorder != nil {
		logger.Info("phase 3c: closing evidence recorder...")
		evidenceRecorder.Close()
		logger.Info("evidence recorder closed")
	}

	// Phase 3d: Close chunk assembler (stops cleanup goroutine).
	if chunkAssembler != nil {
		logger.Info("phase 3d: closing chunk assembler...")
		if err := chunkAssembler.Close(); err != nil {
			logger.Warn("chunk assembler close error", zap.Error(err))
		} else {
			logger.Info("chunk assembler closed")
		}
	}

	// Phase 3e: Close MinIO evidence store.
	if evidenceStore != nil {
		logger.Info("phase 3c: closing minio evidence store...")
		if err := evidenceStore.Close(); err != nil {
			logger.Warn("minio close error", zap.Error(err))
		} else {
			logger.Info("minio evidence store closed")
		}
	}

	// Phase 3f: Close sidecam orchestrator (stops health monitor goroutine).
	if sidecamOrchestrator != nil {
		logger.Info("phase 3f: closing sidecam orchestrator...")
		sidecamOrchestrator.Close()
	}

	// Phase 3g: Stop cleanup use case (stop before data stores close).
	if cleanupUC != nil {
		logger.Info("phase 3g: stopping cleanup use case...")
		cleanupUC.Stop()
		logger.Info("cleanup use case stopped")
	}

	// Phase 3h: Stop idempotency cache eviction goroutine.
	if idempCache != nil {
		logger.Info("phase 3h: stopping idempotency cache...")
		idempCache.Close()
		logger.Info("idempotency cache stopped")
	}

	// Phase 3i: Stop backfiller (must stop BEFORE ClickHouse writer closes).
	if chBackfiller != nil {
		logger.Info("phase 3g: stopping clickhouse backfiller...")
		if err := chBackfiller.Close(); err != nil {
			logger.Warn("backfiller close error", zap.Error(err))
		} else {
			logger.Info("clickhouse backfiller stopped")
		}
	}

	// Phase 4: Flush data stores (Kafka + ClickHouse).
	logger.Info("phase 4: flushing data stores...")
	if err := ingestUC.Close(); err != nil {
		logger.Error("ingest use-case close error", zap.Error(err))
	} else {
		logger.Info("ingest use-case closed (kafka flushed, clickhouse drained)")
	}

	// Phase 5: Close alerter (drain pending Telegram messages).
	if telegramAlerter != nil {
		logger.Info("phase 5: closing telegram alerter...")
		telegramAlerter.Close()
	}

	logger.Info("event-collector shut down successfully",
		zap.String("version", version),
	)
	return nil
}

// ---------------------------------------------------------------------------
// Logger construction
// ---------------------------------------------------------------------------

// buildLogger creates a production-grade zap.Logger from the given config.
// In development mode, it uses a human-readable console encoder with coloured
// output. In production, it uses a structured JSON encoder.
func buildLogger(cfg config.LoggerConfig) (*zap.Logger, error) {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", cfg.Level, err)
	}

	var zapCfg zap.Config
	if cfg.Development {
		zapCfg = zap.NewDevelopmentConfig()
	} else {
		zapCfg = zap.NewProductionConfig()
	}

	zapCfg.Level = zap.NewAtomicLevelAt(level)
	zapCfg.Encoding = cfg.Encoding

	// ISO-8601 timestamps are easier to parse in log aggregation systems
	// (Loki, Elasticsearch, CloudWatch) than epoch seconds.
	zapCfg.EncoderConfig.TimeKey = "ts"
	zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, err := zapCfg.Build(
		zap.AddCallerSkip(0),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}

	return logger, nil
}
