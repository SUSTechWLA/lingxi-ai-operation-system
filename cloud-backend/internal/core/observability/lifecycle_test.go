package observability

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type rejectingLifecycleEmitter struct{ err error }

func (e rejectingLifecycleEmitter) Emit(context.Context, Event) error { return e.err }

type capturingLifecycleEmitter struct{ event Event }

func (e *capturingLifecycleEmitter) Emit(_ context.Context, event Event) error {
	e.event = event
	return nil
}

func TestEmitSafelyReservesErrorSeverityForStableNormalizedErrors(t *testing.T) {
	emitter := &capturingLifecycleEmitter{}
	EmitSafely(context.Background(), emitter, "test-component", Event{
		EventType: EventTypeLocalJobFailed,
		Severity:  SeverityError,
	})
	if emitter.event.Severity != SeverityWarn {
		t.Fatalf("unclassified failure severity=%s, want WARN", emitter.event.Severity)
	}

	stable := NormalizeError("TOOL.ARGUMENT.SCHEMA_INVALID", errors.New("private"), "tool", "")
	EmitSafely(context.Background(), emitter, "test-component", Event{
		EventType: EventTypeToolCallFailed,
		Severity:  SeverityError,
		Error:     stable,
	})
	if emitter.event.Severity != SeverityError || emitter.event.Error == nil ||
		emitter.event.Error.Code != "TOOL.ARGUMENT.SCHEMA_INVALID" {
		t.Fatalf("stable error event changed: %+v", emitter.event)
	}
}

func TestEmitSafelyLogsOnlyBoundedDiagnosticFields(t *testing.T) {
	const privateText = "PRIVATE_EMITTER_EVENT_PAYLOAD_SENTINEL"
	core, logs := observer.New(zap.WarnLevel)
	previous := zap.L()
	zap.ReplaceGlobals(zap.New(core))
	t.Cleanup(func() { zap.ReplaceGlobals(previous) })

	EmitSafely(context.Background(), rejectingLifecycleEmitter{err: errors.New(privateText)}, "test-component", Event{
		EventType: EventTypeAgentRunFailed,
		Evidence:  Evidence{Attributes: map[string]any{"payload": privateText}},
	})

	if logs.Len() != 1 {
		t.Fatalf("diagnostic log count=%d, want 1", logs.Len())
	}
	entry := logs.All()[0]
	fields := entry.ContextMap()
	if fields["eventType"] != string(EventTypeAgentRunFailed) || fields["errorClass"] != "rejected" {
		t.Fatalf("diagnostic fields=%v", fields)
	}
	if strings.Contains(entry.Message, privateText) {
		t.Fatalf("private emitter failure leaked in message: %q", entry.Message)
	}
	for key, value := range fields {
		if strings.Contains(key, "payload") || strings.Contains(value.(string), privateText) {
			t.Fatalf("private event material leaked in diagnostic field %s=%v", key, value)
		}
	}
}
