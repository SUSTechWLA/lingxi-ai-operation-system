package observability

import (
	"context"
	"testing"
)

func TestWithCorrelationRoundTripsCorrelation(t *testing.T) {
	want := Correlation{
		TraceID:      "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:       "00f067aa0ba902b7",
		ParentSpanID: "b7ad6b7169203331",
		TaskID:       "tsk_123",
	}

	got := CorrelationFromContext(WithCorrelation(context.Background(), want))
	if got != want {
		t.Fatalf("CorrelationFromContext() = %#v, want %#v", got, want)
	}
}

func TestCorrelationFromContextHandlesNilAndMissingContext(t *testing.T) {
	if got := CorrelationFromContext(nil); got != (Correlation{}) {
		t.Fatalf("nil context correlation = %#v", got)
	}
	if got := CorrelationFromContext(context.Background()); got != (Correlation{}) {
		t.Fatalf("missing context correlation = %#v", got)
	}
}
