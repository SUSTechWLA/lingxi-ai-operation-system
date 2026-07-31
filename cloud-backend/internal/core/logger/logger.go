package logger

import (
	"context"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// contextKey avoids collisions with other packages.
type contextKey string

const requestIDKey contextKey = "logger-request-id"

// Init configures the global zap logger. In production mode, logs are written to
// both stdout and a file. In development mode, only stdout is used with color output.
func Init(mode string) {
	var cfg zap.Config
	filePath := os.Getenv("AIOS_LOG_FILE")

	switch mode {
	case "production":
		cfg = zap.NewProductionConfig()
		cfg.EncoderConfig.TimeKey = "timestamp"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		cfg.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
		cfg.Sampling = &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		}
		// Set level from env, default to info
		if lvl := os.Getenv("AIOS_LOG_LEVEL"); lvl != "" {
			var level zapcore.Level
			if err := level.UnmarshalText([]byte(lvl)); err == nil {
				cfg.Level = zap.NewAtomicLevelAt(level)
			}
		}
	default:
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	var cores []zapcore.Core

	// Always output to stdout
	stdoutCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(cfg.EncoderConfig),
		zapcore.AddSync(os.Stdout),
		cfg.Level,
	)
	cores = append(cores, stdoutCore)

	// In production, also write to a file
	if filePath != "" || mode == "production" {
		if filePath == "" {
			filePath = filepath.Join(os.TempDir(), "tangying-cloud-backend.jsonl")
		}
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err == nil {
			fileCore := zapcore.NewCore(
				zapcore.NewJSONEncoder(cfg.EncoderConfig),
				zapcore.AddSync(zapcore.Lock(zapcore.AddSync(&rotateWriter{path: filePath}))),
				cfg.Level,
			)
			cores = append(cores, fileCore)
		}
	}

	// Add caller skip for wrapper functions
	logger := zap.New(zapcore.NewTee(cores...), zap.AddCaller())

	zap.ReplaceGlobals(logger)
}

// WithRequestID stores a request-scoped identifier in the context for
// correlation across the entire call chain (HTTP → agent → MCP → local runner).
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestIDFrom extracts the request ID from a context, or returns "".
func RequestIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// RequestIDField returns a zap field for the request ID in the given context.
func RequestIDField(ctx context.Context) zap.Field {
	if id := RequestIDFrom(ctx); id != "" {
		return zap.String("requestId", id)
	}
	return zap.Skip()
}

// rotateWriter is a simple line-buffered file writer. In production, consider
// using a proper log rotation library like lumberjack.
type rotateWriter struct {
	path string
}

func (w *rotateWriter) Write(p []byte) (int, error) {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return f.Write(p)
}
