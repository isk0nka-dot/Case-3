// Package main is the entry point for the Argus AI background Worker service.
//
// The Worker service processes CPU-bound tasks (video export, forensic PDF
// generation) that are dispatched via Redis-backed asynq queues. It scales
// independently from the API server, ensuring that heavy archive creation
// and PDF rendering never impact API latency.
//
// Supported job types:
//
//   - job:video_export    — TAR.GZ archive of session evidence
//   - job:forensic_report — PDF forensic integrity report
//
// The Worker shares the same config file as the API server and connects
// to the same PostgreSQL, ClickHouse, MinIO, and Telegram instances.
//
// Usage:
//
//	go run cmd/worker/main.go -config deployments/config.yaml
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/argus-ai/event-collector/internal/infrastructure/alerting"
	"github.com/argus-ai/event-collector/internal/infrastructure/clickhouse"
	"github.com/argus-ai/event-collector/internal/infrastructure/config"
	integrityInfra "github.com/argus-ai/event-collector/internal/infrastructure/integrity"
	minioStore "github.com/argus-ai/event-collector/internal/infrastructure/minio"
	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
	"github.com/argus-ai/event-collector/internal/infrastructure/worker"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "argus-worker: fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// =================================================================
	// STEP 1: Parse CLI flags.
	// =================================================================
	configPath := flag.String("config", "deployments/config.yaml", "path to the YAML configuration file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("argus-worker version=%s build_time=%s\n", version, buildTime)
		return nil
	}

	// =================================================================
	// STEP 2: Load configuration (shared with API server).
	// =================================================================
	cfg, err := config.LoadWithEnv(*configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// =================================================================
	// STEP 3: Initialise logger.
	// =================================================================
	logger, err := buildLogger(cfg.Logger)
	if err != nil {
		return fmt.Errorf("failed to initialise logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	logger.Info("starting argus-worker",
		zap.String("version", version),
		zap.String("build_time", buildTime),
		zap.String("redis_addr", cfg.Redis.Addr),
		zap.Int("concurrency", cfg.Redis.WorkerConcurrency),
	)

	// =================================================================
	// STEP 4: Initialise ClickHouse (read-only, for queries).
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
		return fmt.Errorf("failed to initialise clickhouse: %w", err)
	}
	logger.Info("clickhouse connected",
		zap.Strings("addrs", cfg.ClickHouse.Addrs),
		zap.String("database", cfg.ClickHouse.Database),
	)

	// =================================================================
	// STEP 5: Initialise PostgreSQL.
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
		return fmt.Errorf("failed to initialise postgres: %w", err)
	}
	logger.Info("postgresql connected",
		zap.String("host", cfg.Postgres.Host),
		zap.Int("port", cfg.Postgres.Port),
		zap.String("database", cfg.Postgres.Database),
	)

	// =================================================================
	// STEP 6: Initialise MinIO (S3 evidence + export storage).
	// =================================================================
	evidenceStore, err := minioStore.NewStore(minioStore.StoreConfig{
		Endpoint:      cfg.MinIO.Endpoint,
		AccessKey:     cfg.MinIO.AccessKey,
		SecretKey:     cfg.MinIO.SecretKey,
		Bucket:        cfg.MinIO.Bucket,
		Region:        cfg.MinIO.Region,
		UseSSL:        cfg.MinIO.UseSSL,
		PresignTTL:    cfg.MinIO.PresignTTL,
		UploadTimeout: cfg.MinIO.UploadTimeout,
	}, logger)
	if err != nil {
		return fmt.Errorf("failed to initialise minio: %w", err)
	}

	minioClient := minioStore.Client(evidenceStore)
	if minioClient == nil {
		return fmt.Errorf("failed to extract minio client from evidence store")
	}
	logger.Info("minio connected",
		zap.String("endpoint", cfg.MinIO.Endpoint),
		zap.String("bucket", cfg.MinIO.Bucket),
	)

	// =================================================================
	// STEP 7: Initialise Telegram alerter (optional).
	// =================================================================
	telegramAlerter := alerting.NewTelegramProvider(alerting.TelegramConfig{
		BotToken: cfg.Telegram.BotToken,
		ChatID:   cfg.Telegram.ChatID,
	}, logger)

	// =================================================================
	// STEP 8: Initialise integrity verifier (for forensic reports).
	// =================================================================
	integrityVerifier := integrityInfra.NewVerifier(chWriter.Conn(), evidenceStore, logger)

	// =================================================================
	// STEP 9: Create asynq task handlers.
	// =================================================================
	videoExportHandler := worker.NewVideoExportHandler(
		pgRepo.DB(), chWriter.Conn(), minioClient,
		cfg.Export, cfg.MinIO.Bucket, logger, telegramAlerter,
	)

	forensicReportHandler := worker.NewForensicReportHandler(
		chWriter.Conn(), integrityVerifier, pgRepo,
		minioClient, logger, telegramAlerter,
	)

	aiAnalysisHandler := worker.NewAIAnalysisHandler(
		cfg.Inference.GRPCAddr(),
		chWriter.Conn(), chWriter, pgRepo,
		minioClient, cfg.MinIO.Bucket,
		logger, telegramAlerter,
	)

	// =================================================================
	// STEP 10: Create and configure asynq server.
	// =================================================================
	srv := asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		},
		asynq.Config{
			Concurrency: cfg.Redis.WorkerConcurrency,
			Queues: map[string]int{
				worker.QueueCritical: 6,
				worker.QueueDefault:  3,
				worker.QueueLow:      1,
			},
			RetryDelayFunc: func(n int, err error, t *asynq.Task) time.Duration {
				// Exponential backoff: 10s, 40s, 90s, ...
				return time.Duration(n*n) * 10 * time.Second
			},
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
				retried, _ := asynq.GetRetryCount(ctx)
				maxRetry, _ := asynq.GetMaxRetry(ctx)
				logger.Error("asynq task failed",
					zap.String("type", task.Type()),
					zap.Int("retry", retried),
					zap.Int("max_retry", maxRetry),
					zap.Error(err),
				)

				// Only send Telegram alert on final failure (all retries exhausted).
				if retried >= maxRetry {
					worker.NotifyJobFailed(telegramAlerter, task.Type(), task.ResultWriter().TaskID(), err)
				}
			}),
			Logger: &asynqZapLogger{logger: logger.Named("asynq")},
		},
	)

	// Register handlers.
	mux := asynq.NewServeMux()
	mux.Handle(worker.TypeVideoExport, videoExportHandler)
	mux.Handle(worker.TypeForensicReport, forensicReportHandler)
	mux.Handle(worker.TypeAIAnalysis, aiAnalysisHandler)

	logger.Info("asynq handlers registered",
		zap.String(worker.TypeVideoExport, "VideoExportHandler"),
		zap.String(worker.TypeForensicReport, "ForensicReportHandler"),
		zap.String(worker.TypeAIAnalysis, "AIAnalysisHandler"),
	)

	// =================================================================
	// STEP 11: Send startup notification.
	// =================================================================
	if telegramAlerter != nil {
		_ = telegramAlerter.SendDirect(alerting.MsgWorkerStartup(
			version, cfg.Redis.Addr, cfg.Redis.WorkerConcurrency,
			time.Now().Format("2006-01-02 15:04:05 MST"),
			[]string{"job:video_export", "job:forensic_report", "job:ai_analysis"},
		))
	}

	// =================================================================
	// STEP 12: Start server (blocks until signal).
	// =================================================================

	// Capture SIGINT/SIGTERM for graceful shutdown logging.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("received shutdown signal", zap.String("signal", sig.String()))
		srv.Shutdown()
	}()

	logger.Info("argus-worker running — waiting for tasks...")

	if err := srv.Run(mux); err != nil {
		return fmt.Errorf("asynq server error: %w", err)
	}

	// =================================================================
	// STEP 13: Graceful shutdown.
	// =================================================================
	logger.Info("argus-worker stopped gracefully")

	if telegramAlerter != nil {
		_ = telegramAlerter.SendDirect(alerting.MsgWorkerShutdown(
			time.Now().Format("2006-01-02 15:04:05 MST"),
		))
		telegramAlerter.Close()
	}

	return nil
}

// ---------------------------------------------------------------------------
// Logger
// ---------------------------------------------------------------------------

func buildLogger(cfg config.LoggerConfig) (*zap.Logger, error) {
	var level zapcore.Level
	switch cfg.Level {
	case "debug":
		level = zap.DebugLevel
	case "info":
		level = zap.InfoLevel
	case "warn":
		level = zap.WarnLevel
	case "error":
		level = zap.ErrorLevel
	default:
		level = zap.InfoLevel
	}

	zapCfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(level),
		Development:      cfg.Development,
		Encoding:         cfg.Encoding,
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	if cfg.Encoding == "console" {
		zapCfg.EncoderConfig = zap.NewDevelopmentEncoderConfig()
	}

	return zapCfg.Build()
}

// ---------------------------------------------------------------------------
// asynq ↔ zap logger bridge
// ---------------------------------------------------------------------------

// asynqZapLogger adapts zap.Logger to the asynq.Logger interface.
type asynqZapLogger struct {
	logger *zap.Logger
}

func (l *asynqZapLogger) Debug(args ...interface{}) {
	l.logger.Debug(fmt.Sprint(args...))
}

func (l *asynqZapLogger) Info(args ...interface{}) {
	l.logger.Info(fmt.Sprint(args...))
}

func (l *asynqZapLogger) Warn(args ...interface{}) {
	l.logger.Warn(fmt.Sprint(args...))
}

func (l *asynqZapLogger) Error(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args...))
}

func (l *asynqZapLogger) Fatal(args ...interface{}) {
	l.logger.Fatal(fmt.Sprint(args...))
}
