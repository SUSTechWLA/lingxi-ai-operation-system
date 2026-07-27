package eventbus

import (
	"context"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/observability"
)

func TestConsumerHandlerReceivesReconstructedCorrelationContext(t *testing.T) {
	event := Event{
		TraceID:      "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:       "00f067aa0ba902b7",
		ParentSpanID: "b7ad6b7169203331",
	}
	var got observability.Correlation
	handler := &consumerGroupHandler{
		handlerFn: func(ctx context.Context, _ Event) error {
			got = observability.CorrelationFromContext(ctx)
			return nil
		},
	}

	if err := handler.handleEvent(event); err != nil {
		t.Fatal(err)
	}
	if got.TraceID != event.TraceID || got.SpanID != event.SpanID || got.ParentSpanID != event.ParentSpanID {
		t.Fatalf("handler correlation = %#v", got)
	}
}
