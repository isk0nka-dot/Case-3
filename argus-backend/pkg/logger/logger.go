// Package logger provides a production-grade structured logger factory built on
// go.uber.org/zap. It offers pre-configured logger constructors for development
// and production environments with sensible defaults.
//
// In the event-collector service, structured logging is critical for:
//   - Correlating events across the ingestion pipeline (gRPC -> Kafka -> ClickHouse).
//   - Diagnosing performance issues under high event throughput (10k+ events/sec).
//   - Alerting on anomalies via log-based monitoring (e.g., Grafana Loki, Datadog).
//
// Production mode features:
//   - JSON encoding for machine-parseable log aggregation.
//   - Sampling to prevent log flooding under sustained high load. The first 100
//     messages at each log level are emitted, then every 100th message thereafter.
//     This bounds log volume to O(1) per level under steady state while preserving
//     the initial burst for debugging ramp-up issues.
//   - Millisecond-precision timestamps in ISO 8601 format.
//   - Caller information (file:line) for fast source location in production logs.
//
// Development mode features:
//   - Console encoding with colorized output for human readability.
//   - DPanic level causes panics (useful for catching programmer errors in tests).
//   - Stack traces on Warn and above for immediate debugging.
//   - No sampling — every message is emitted.
//
// Design decisions:
//   - Factory pattern over global logger for testability and dependency injection.
//   - Config struct with YAML tags for unified configuration file support.
//   - All constructors return a *zap.Logger (not *zap.SugaredLogger) for zero-
//     allocation structured logging on the hot path.
package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config holds the configuration for creating a structured logger.
// All fields have sensible defaults; only override what you need.
type Config struct {
	// Level sets the minimum enabled logging level. Valid values:
	// "debug", "info", "warn", "error", "dpanic", "panic", "fatal".
	// Default: "info" for production, "debug" for development.
	Level string `yaml:"level"`

	// Encoding sets the log output format. Valid values:
	//   "json"    — Machine-parseable JSON, one object per line. Best for
	//               production log aggregation (Loki, ELK, Datadog).
	//   "console" — Human-readable colored output. Best for local development.
	// Default: "json".
	Encoding string `yaml:"encoding"`

	// Development enables development-mode behavior:
	//   - DPanicLevel logs cause panics instead of just logging.
	//   - Stack traces are captured at WarnLevel and above.
	// Default: false.
	Development bool `yaml:"development"`

	// OutputPaths is a list of destinations for log output. Each entry can be:
	//   - "stdout" — Standard output.
	//   - "stderr" — Standard error.
	//   - A file path (e.g., "/var/log/event-collector/app.log").
	// Multiple destinations are written to simultaneously (tee behavior).
	// Default: ["stdout"].
	OutputPaths []string `yaml:"output_paths"`
}

// samplingConfig defines the production log sampling parameters.
// These values are chosen to balance operational visibility with log volume control.
const (
	// samplingInitial is the number of messages at each log level that are emitted
	// without any sampling. This ensures that the first burst of messages at any
	// level is fully captured — critical for debugging startup issues, initial
	// connection failures, and deployment rollout problems.
	samplingInitial = 100

	// samplingThereafter controls the sampling rate after the initial burst.
	// Every Nth message at each level is emitted. At 100, this means roughly
	// 1% of messages are logged under sustained high-frequency logging,
	// bounding output to a manageable volume.
	samplingThereafter = 100
)

// New creates a configured zap logger from the given Config.
//
// This is the primary constructor for application use. It builds the logger
// from individual components (encoder, level, output) rather than using zap's
// preset configs, giving full control over every aspect of the logger.
//
// In production mode (Development == false), log sampling is enabled automatically
// to prevent log flooding. In development mode, sampling is disabled so every
// message is visible during debugging.
//
// Returns an error if the configuration is invalid (e.g., unrecognized level
// or encoding, invalid output path).
func New(cfg Config) (*zap.Logger, error) {
	// Parse the log level.
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", cfg.Level, err)
	}

	// Apply defaults.
	encoding := cfg.Encoding
	if encoding == "" {
		encoding = "json"
	}

	outputPaths := cfg.OutputPaths
	if len(outputPaths) == 0 {
		outputPaths = []string{"stdout"}
	}

	// Build the encoder config based on the environment.
	var encoderCfg zapcore.EncoderConfig
	if cfg.Development {
		encoderCfg = zap.NewDevelopmentEncoderConfig()
	} else {
		encoderCfg = zap.NewProductionEncoderConfig()
	}

	// Use ISO 8601 timestamps with millisecond precision for all environments.
	// This format is universally parseable by log aggregation systems and is
	// human-readable in UTC.
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	// Use lowercase level names for consistency in JSON output.
	encoderCfg.EncodeLevel = zapcore.LowercaseLevelEncoder

	// Use millisecond duration encoding for performance-related fields.
	encoderCfg.EncodeDuration = zapcore.MillisDurationEncoder

	// Include the short caller path (package/file:line) for source location.
	encoderCfg.EncodeCaller = zapcore.ShortCallerEncoder

	// Construct the zap.Config.
	zapCfg := zap.Config{
		Level:             zap.NewAtomicLevelAt(level),
		Development:       cfg.Development,
		Encoding:          encoding,
		EncoderConfig:     encoderCfg,
		OutputPaths:       outputPaths,
		ErrorOutputPaths:  []string{"stderr"},
		DisableStacktrace: !cfg.Development, // Stack traces only in dev mode.
	}

	// Enable sampling in production to prevent log flooding under sustained load.
	// Sampling is per-level and per-message: it tracks each unique message template
	// (the first argument to logger.Info, logger.Error, etc.) independently.
	if !cfg.Development {
		zapCfg.Sampling = &zap.SamplingConfig{
			Initial:    samplingInitial,
			Thereafter: samplingThereafter,
		}
	}

	logger, err := zapCfg.Build(
		// Add caller skip so the reported caller is the actual call site,
		// not the logger wrapper.
		zap.AddCaller(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}

	return logger, nil
}

// NewDevelopment creates a logger configured for local development.
//
// Features:
//   - Debug level (all messages visible).
//   - Console encoding with colorized output.
//   - DPanic causes panics (catches programmer errors early).
//   - Stack traces on Warn and above.
//   - No sampling (every message emitted).
//   - Output to stdout only.
//
// This is a convenience constructor equivalent to:
//
//	New(Config{
//	    Level:       "debug",
//	    Encoding:    "console",
//	    Development: true,
//	    OutputPaths: []string{"stdout"},
//	})
//
// Panics if the logger cannot be built (should never happen with valid defaults).
func NewDevelopment() *zap.Logger {
	logger, err := New(Config{
		Level:       "debug",
		Encoding:    "console",
		Development: true,
		OutputPaths: []string{"stdout"},
	})
	if err != nil {
		// This should never happen with hardcoded valid configuration.
		// If it does, there is a programmer error in the defaults.
		panic(fmt.Sprintf("failed to create development logger: %v", err))
	}
	return logger
}

// NewProduction creates a logger configured for production deployment.
//
// Features:
//   - Info level (debug messages suppressed).
//   - JSON encoding for machine-parseable log aggregation.
//   - Sampling enabled (first 100, then every 100th per level).
//   - No stack traces (reduces log volume and noise).
//   - Output to stdout for container log drivers (Docker, Kubernetes).
//
// This is a convenience constructor equivalent to:
//
//	New(Config{
//	    Level:       "info",
//	    Encoding:    "json",
//	    Development: false,
//	    OutputPaths: []string{"stdout"},
//	})
//
// Panics if the logger cannot be built (should never happen with valid defaults).
func NewProduction() *zap.Logger {
	logger, err := New(Config{
		Level:       "info",
		Encoding:    "json",
		Development: false,
		OutputPaths: []string{"stdout"},
	})
	if err != nil {
		panic(fmt.Sprintf("failed to create production logger: %v", err))
	}
	return logger
}

// parseLevel converts a string log level name to the corresponding zapcore.Level.
// Accepts both lowercase and mixed-case inputs (e.g., "INFO", "info", "Info").
// Returns an error for unrecognized level strings.
func parseLevel(s string) (zapcore.Level, error) {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return zapcore.InfoLevel, fmt.Errorf("unrecognized level: %s", s)
	}
	return level, nil
}
