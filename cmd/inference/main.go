// Package main is the entry point for the Argus AI Inference Gateway.
//
// The Inference Gateway is a standalone gRPC service that accepts raw image
// frames or video segments and returns structured AI inference results (face
// detection, object detection, liveness verification). It runs independently
// from the API server and worker, scaling with GPU resources.
//
// The service uses a pluggable Engine interface — the default StubEngine
// returns synthetic results for development. Swap in ONNXEngine or
// PythonBridge for production GPU-accelerated inference.
//
// Usage:
//
//	go run cmd/inference/main.go -config deployments/config.yaml
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	inferencepb "github.com/argus-ai/event-collector/api/proto/v1/inferencepb"
	"github.com/argus-ai/event-collector/internal/infrastructure/alerting"
	"github.com/argus-ai/event-collector/internal/infrastructure/config"
	"github.com/argus-ai/event-collector/internal/infrastructure/inference"
	grpcTransport "github.com/argus-ai/event-collector/internal/transport/grpc"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "argus-inference: fatal: %v\n", err)
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
		fmt.Printf("argus-inference version=%s build_time=%s\n", version, buildTime)
		return nil
	}

	// =================================================================
	// STEP 2: Load configuration (shared with API server + worker).
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

	logger.Info("starting argus-inference",
		zap.String("version", version),
		zap.String("build_time", buildTime),
		zap.Int("grpc_port", cfg.Inference.GRPCPort),
		zap.String("engine", cfg.Inference.EngineType),
		zap.Int("concurrency", cfg.Inference.Concurrency),
	)

	// =================================================================
	// STEP 4: Initialise AI inference engine.
	// =================================================================
	var engine inference.Engine
	switch cfg.Inference.EngineType {
	case "stub":
		engine = inference.NewStubEngine()
	// Future: case "onnx": engine = inference.NewONNXEngine(cfg.Inference.ModelDir)
	// Future: case "python_bridge": engine = inference.NewPythonBridge(...)
	default:
		return fmt.Errorf("unknown inference engine type: %q (supported: stub)", cfg.Inference.EngineType)
	}
	defer engine.Close()

	logger.Info("inference engine initialised",
		zap.String("engine", engine.Name()),
	)

	// =================================================================
	// STEP 5: Initialise Telegram alerter (optional).
	// =================================================================
	var telegramAlerter alerting.Provider
	if cfg.Telegram.BotToken != "" && cfg.Telegram.ChatID != "" {
		telegramAlerter = alerting.NewTelegramProvider(alerting.TelegramConfig{
			BotToken: cfg.Telegram.BotToken,
			ChatID:   cfg.Telegram.ChatID,
		}, logger)
	}

	// =================================================================
	// STEP 6: Create gRPC server.
	// =================================================================
	kaParams := keepalive.ServerParameters{
		MaxConnectionIdle:     5 * time.Minute,
		MaxConnectionAge:      30 * time.Minute,
		MaxConnectionAgeGrace: 10 * time.Second,
		Time:                  1 * time.Minute,
		Timeout:               20 * time.Second,
	}

	grpcServer := grpc.NewServer(
		grpc.KeepaliveParams(kaParams),
		grpc.MaxRecvMsgSize(cfg.Inference.MaxFrameBytes+1024), // frame + metadata
		grpc.MaxSendMsgSize(4*1024*1024),
	)

	// =================================================================
	// STEP 7: Register InferenceService.
	// =================================================================
	inferenceServer := grpcTransport.NewInferenceServer(engine, logger, cfg.Inference.MaxFrameBytes)
	inferencepb.RegisterInferenceServiceServer(grpcServer, inferenceServer)

	logger.Info("inference service registered",
		zap.Int("max_frame_bytes", cfg.Inference.MaxFrameBytes),
	)

	// =================================================================
	// STEP 8: Send startup notification.
	// =================================================================
	if telegramAlerter != nil {
		_ = telegramAlerter.SendDirect(alerting.MsgInferenceStartup(
			version, engine.Name(),
			cfg.Inference.GRPCPort, cfg.Inference.Concurrency,
			time.Now().Format("2006-01-02 15:04:05 MST"),
		))
	}

	// =================================================================
	// STEP 9: Start gRPC listener.
	// =================================================================
	addr := fmt.Sprintf(":%d", cfg.Inference.GRPCPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	// Signal handler for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("argus-inference gRPC listening", zap.String("addr", addr))
		if err := grpcServer.Serve(lis); err != nil {
			errCh <- fmt.Errorf("gRPC serve error: %w", err)
		}
	}()

	// =================================================================
	// STEP 10: Block until signal or fatal error.
	// =================================================================
	select {
	case sig := <-sigCh:
		logger.Info("received shutdown signal", zap.String("signal", sig.String()))
	case err := <-errCh:
		return err
	}

	// =================================================================
	// STEP 11: Graceful shutdown.
	// =================================================================
	logger.Info("argus-inference shutting down...")
	grpcServer.GracefulStop()

	if telegramAlerter != nil {
		_ = telegramAlerter.SendDirect(alerting.MsgInferenceShutdown(
			time.Now().Format("2006-01-02 15:04:05 MST"),
		))
		telegramAlerter.Close()
	}

	logger.Info("argus-inference stopped gracefully")
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
