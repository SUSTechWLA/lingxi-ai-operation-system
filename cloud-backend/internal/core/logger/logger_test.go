package logger

import (
	"context"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/observability"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestWithCorrelationAddsStructuredFieldsToExistingLogger(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	base := zap.New(core)
	correlation := observability.Correlation{
		TraceID:      "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:       "00f067aa0ba902b7",
		ParentSpanID: "b7ad6b7169203331",
	}

	WithCorrelation(base, correlation).Info("handled")

	if logs.Len() != 1 {
		t.Fatalf("log entries = %d", logs.Len())
	}
	fields := logs.All()[0].ContextMap()
	if fields["traceId"] != correlation.TraceID ||
		fields["spanId"] != correlation.SpanID ||
		fields["parentSpanId"] != correlation.ParentSpanID {
		t.Fatalf("correlation fields = %#v", fields)
	}
}

func TestEventSinkWritesCanonicalEventToExistingLogger(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	sink := NewEventSink(zap.New(core))
	event := observability.Event{
		EventID:    "evt_request",
		EventType:  observability.EventTypeRequestAccepted,
		MessageKey: "request.accepted",
		Severity:   observability.SeverityInfo,
		Correlation: observability.Correlation{
			TraceID: "trc_4bf92f3577b34da6a3ce929d0e0e4736",
			SpanID:  "spn_00f067aa0ba902b7",
		},
	}

	if err := sink.Write(context.Background(), event); err != nil {
		t.Fatal(err)
	}

	if logs.Len() != 1 {
		t.Fatalf("log entries = %d", logs.Len())
	}
	entry := logs.All()[0]
	if entry.Message != "observability.event" {
		t.Fatalf("message = %q", entry.Message)
	}
	fields := entry.ContextMap()
	if fields["eventId"] != event.EventID ||
		fields["eventType"] != string(event.EventType) ||
		fields["traceId"] != event.Correlation.TraceID {
		t.Fatalf("event fields = %#v", fields)
	}
}
