package observability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"go.uber.org/zap"
)

// EventEmitter is the narrow lifecycle-observability boundary used by domain
// services. *Emitter implements it; tests may substitute a bounded recorder.
type EventEmitter interface {
	Emit(context.Context, Event) error
}

type DurableEventEmitter interface {
	EventEmitter
	EmitAndWait(context.Context, Event) error
}

// PersistentPreparedEventEmitter owns one authenticated persistence domain.
// Only this emitter can freeze, restore, and replay its prepared capabilities.
type PersistentPreparedEventEmitter interface {
	DurableEventEmitter
	ValidatePersistentConfiguration() error
	FreezeAndSeal(context.Context, Event) (SealedPreparedEvent, error)
	MigrateClaimedLegacyPreparedEvent(context.Context, []byte, Event) (SealedPreparedEvent, error)
	MigrateClaimedPreparedEventSource(context.Context, SealedPreparedEvent, Event) (SealedPreparedEvent, bool, error)
	RestorePreparedEvent(SealedPreparedEvent) (PreparedEvent, error)
	RestorePreparedEventFor(context.Context, SealedPreparedEvent, Event) (PreparedEvent, error)
	ReplayPreparedAndWait(context.Context, PreparedEvent) error
}

// EnsureCorrelation supplies product-independent trace/span identifiers when a
// lifecycle operation starts outside HTTP or Kafka middleware.
func EnsureCorrelation(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	correlation := CorrelationFromContext(ctx)
	if correlation.TraceID == "" {
		correlation.TraceID = randomNonZeroHex(16)
	}
	if correlation.SpanID == "" {
		correlation.SpanID = randomNonZeroHex(8)
	}
	return WithCorrelation(ctx, correlation)
}

// EmitSafely keeps observability failures from failing a user operation while
// still surfacing bounded, structured diagnostics. It never logs event
// evidence, correlation values, or the producer error's source input.
func EmitSafely(ctx context.Context, emitter EventEmitter, component string, event Event) {
	if emitter == nil {
		return
	}
	// ERROR is reserved for failures represented by the stable registry. Never
	// silently relabel a producer's semantics.
	if event.Severity == SeverityError && event.Error == nil {
		zap.L().Warn("observability lifecycle event rejected",
			zap.String("component", component), zap.String("eventType", string(event.EventType)),
			zap.String("errorClass", "missing_stable_error"), zap.String("diagnostic", "observability.emit.failed"))
		return
	}
	if err := emitter.Emit(ctx, event); err != nil {
		zap.L().Warn("observability lifecycle event rejected",
			zap.String("component", component),
			zap.String("eventType", string(event.EventType)),
			zap.String("errorClass", emitterErrorClass(err)),
			zap.String("diagnostic", "observability.emit.failed"),
		)
	}
}

func emitterErrorClass(err error) string {
	switch {
	case errors.Is(err, ErrQueueFull):
		return "queue_full"
	case errors.Is(err, ErrEmitterClosed):
		return "closed"
	default:
		return "rejected"
	}
}

func HashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func ByteSize(value string) *int64 {
	size := int64(len([]byte(value)))
	return &size
}
