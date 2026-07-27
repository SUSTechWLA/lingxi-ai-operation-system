package eventbus

import (
	"context"
	"regexp"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/observability"
	"github.com/tangying-ai/aios-core/internal/core/trustedcontext"
)

func TestEventCorrelationCarriesTrustedOwnerAcrossKafkaBoundary(t *testing.T) {
	ctx := trustedcontext.WithUserID(context.Background(), "user-1")
	event := eventWithCorrelation(ctx, Event{TaskID: "task-1"})
	if event.OwnerUserID != "user-1" {
		t.Fatalf("owner=%q", event.OwnerUserID)
	}
	var got string
	h := consumerGroupHandler{handlerFn: func(ctx context.Context, _ Event) error { got, _ = trustedcontext.UserID(ctx); return nil }}
	if err := h.handleEvent(event); err != nil {
		t.Fatal(err)
	}
	if got != "user-1" {
		t.Fatalf("consumer owner=%q", got)
	}
}

type recordingLegacyPublisher struct {
	events []Event
}

func (p *recordingLegacyPublisher) Publish(_ string, _ string, event Event) error {
	p.events = append(p.events, event)
	return nil
}

func TestPublishWithContextPopulatesMissingCorrelationWithoutStaleValues(t *testing.T) {
	publisher := &recordingLegacyPublisher{}
	contexts := []observability.Correlation{
		{TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", SpanID: "00f067aa0ba902b7", ParentSpanID: "b7ad6b7169203331"},
		{TraceID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SpanID: "1111111111111111", ParentSpanID: "2222222222222222"},
	}
	for _, correlation := range contexts {
		ctx := observability.WithCorrelation(context.Background(), correlation)
		if err := PublishWithContext(ctx, publisher, TopicProgress, "key", Event{}); err != nil {
			t.Fatal(err)
		}
	}
	if len(publisher.events) != 2 {
		t.Fatalf("published events = %d", len(publisher.events))
	}
	for i, want := range contexts {
		got := publisher.events[i]
		if got.TraceID != want.TraceID || got.SpanID != want.SpanID || got.ParentSpanID != want.ParentSpanID {
			t.Errorf("event %d correlation = %#v, want %#v", i, got, want)
		}
	}
}

func TestConsumerHandlerReceivesReconstructedCorrelationContext(t *testing.T) {
	event := Event{
		TraceID:      "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:       "00f067aa0ba902b7",
		ParentSpanID: "b7ad6b7169203331",
	}
	var got []observability.Correlation
	handler := &consumerGroupHandler{
		handlerFn: func(ctx context.Context, _ Event) error {
			got = append(got, observability.CorrelationFromContext(ctx))
			return nil
		},
	}

	if err := handler.handleEvent(event); err != nil {
		t.Fatal(err)
	}
	second := event
	second.TraceID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	second.SpanID = "1111111111111111"
	if err := handler.handleEvent(second); err != nil {
		t.Fatal(err)
	}
	spanPattern := regexp.MustCompile(`^[0-9a-f]{16}$`)
	if len(got) != 2 {
		t.Fatalf("handler correlations = %d", len(got))
	}
	for i, want := range []Event{event, second} {
		if got[i].TraceID != want.TraceID || got[i].ParentSpanID != want.SpanID || !spanPattern.MatchString(got[i].SpanID) || got[i].SpanID == want.SpanID {
			t.Errorf("handler correlation %d = %#v, producer event = %#v", i, got[i], want)
		}
	}
	if got[0].SpanID == got[1].SpanID {
		t.Fatalf("consumer reused child span across messages: %q", got[0].SpanID)
	}
}
