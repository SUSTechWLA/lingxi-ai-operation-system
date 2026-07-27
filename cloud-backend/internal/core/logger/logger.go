package logger

import (
	"context"
	"errors"

	"github.com/tangying-ai/aios-core/internal/core/observability"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func Init(mode string) {
	var cfg zap.Config
	if mode == "production" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, err := cfg.Build()
	if err != nil {
		panic("failed to init logger: " + err.Error())
	}

	zap.ReplaceGlobals(logger)
}

// WithCorrelation derives a structured child from the supplied logger. It
// deliberately does not replace the process-global logger.
func WithCorrelation(base *zap.Logger, correlation observability.Correlation) *zap.Logger {
	if base == nil {
		base = zap.L()
	}
	fields := []zap.Field{
		zap.String("traceId", correlation.TraceID),
		zap.String("spanId", correlation.SpanID),
	}
	if correlation.ParentSpanID != "" {
		fields = append(fields, zap.String("parentSpanId", correlation.ParentSpanID))
	}
	return base.With(fields...)
}

type EventSink struct {
	logger *zap.Logger
}

func NewEventSink(base *zap.Logger) *EventSink {
	if base == nil {
		base = zap.L()
	}
	return &EventSink{logger: base}
}

func (s *EventSink) Write(_ context.Context, event observability.Event) error {
	event = observability.Redact(event)
	if observability.ContainsSecret(event) {
		return errors.New("refusing to log observability event containing secret material")
	}
	WithCorrelation(s.logger, event.Correlation).Info("observability.event",
		zap.String("eventId", event.EventID),
		zap.String("eventType", string(event.EventType)),
		zap.String("severity", string(event.Severity)),
		zap.String("messageKey", event.MessageKey),
		zap.Any("event", event),
	)
	return nil
}

func (s *EventSink) Close(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return s.logger.Sync()
	}
}
