package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/observability"
)

type translatorRoundTripper func(*http.Request) (*http.Response, error)

func (f translatorRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type translatorEventSink struct {
	mu     sync.Mutex
	events []observability.Event
}

func (s *translatorEventSink) Write(_ context.Context, event observability.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (*translatorEventSink) Close(context.Context) error { return nil }

func (s *translatorEventSink) snapshot() []observability.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]observability.Event(nil), s.events...)
}

func TestTranslateToDagEmitsPairedCancellationWithoutPrompt(t *testing.T) {
	const privatePrompt = "PRIVATE_TRANSLATOR_PROMPT_SENTINEL"
	sink := &translatorEventSink{}
	emitter := observability.NewEmitter(
		observability.Source{Service: "cloud-backend", Component: "translator", Environment: "test"},
		observability.Runtime{},
		sink,
		32,
	)
	service := NewNlToDagService(config.OpenAIConfig{}, "http://orchestrator.test", nil).
		WithObservability(emitter)
	service.httpClient = &http.Client{Transport: translatorRoundTripper(func(req *http.Request) (*http.Response, error) {
		body := `{"taskId":"task-translate"}`
		if req.Method == http.MethodGet {
			body = `{"nodes":[{"id":"` + strings.TrimPrefix(req.URL.Path, "/api/task/") + `","status":"FAILED"}]}`
			// The implementation searches its generated node ID; a task-level
			// failure with no matching node times out, so return malformed JSON
			// to exercise the immediate submit-result terminal path instead.
			body = `{`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}
	ctx, cancel := context.WithCancel(observability.WithCorrelation(context.Background(), observability.Correlation{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:  "00f067aa0ba902b7",
	}))
	cancel()

	if _, err := service.TranslateToDag(ctx, privatePrompt); err == nil {
		t.Fatal("TranslateToDag returned nil error")
	}
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	events := sink.snapshot()
	var started, cancelled int
	for _, event := range events {
		if err := event.Validate(); err != nil {
			t.Fatalf("invalid translator event %s: %v", event.EventType, err)
		}
		switch event.EventType {
		case observability.EventTypeLLMCallStarted:
			started++
		case observability.EventType("llm.call.cancelled"):
			cancelled++
			if event.MessageKey != "llm.call.cancelled" || event.Execution.Status != observability.ExecutionStatusCancelled ||
				event.Severity != observability.SeverityWarn || event.Error != nil {
				t.Fatalf("LLM cancellation event=%+v", event)
			}
		}
	}
	if started != 1 || cancelled != 1 {
		t.Fatalf("LLM event pairing started=%d cancelled=%d events=%+v", started, cancelled, events)
	}
	wire, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), privatePrompt) {
		t.Fatalf("private translator prompt leaked into events: %s", wire)
	}
}

func TestTranslateToDagNormalizesActualFailureModesThroughEmitter(t *testing.T) {
	tests := []struct {
		name      string
		ctx       func() context.Context
		transport translatorRoundTripper
		wantCode  string
	}{
		{
			name: "transport", ctx: context.Background,
			transport: func(*http.Request) (*http.Response, error) { return nil, errors.New("connection unavailable") },
			wantCode:  "LLM.TRANSPORT.UNAVAILABLE",
		},
		{
			name: "schema", ctx: context.Background,
			transport: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{`)), Header: make(http.Header)}, nil
			},
			wantCode: "LLM.RESPONSE.SCHEMA_INVALID",
		},
		{
			name: "timeout", ctx: func() context.Context {
				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				defer cancel()
				return ctx
			},
			transport: func(req *http.Request) (*http.Response, error) { return nil, req.Context().Err() },
			wantCode:  "LLM.CALL.TIMEOUT",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := &translatorEventSink{}
			emitter := observability.NewEmitter(observability.Source{Service: "cloud", Component: "translator", Environment: "test"}, observability.Runtime{}, sink, 16)
			service := NewNlToDagService(config.OpenAIConfig{}, "http://orchestrator.test", nil).WithObservability(emitter)
			service.httpClient = &http.Client{Transport: tt.transport}
			if _, err := service.TranslateToDag(tt.ctx(), "private prompt"); err == nil {
				t.Fatal("TranslateToDag returned nil error")
			}
			if err := emitter.Close(context.Background()); err != nil {
				t.Fatal(err)
			}
			var terminal int
			for _, event := range sink.snapshot() {
				if err := event.Validate(); err != nil {
					t.Fatalf("invalid event %s: %v", event.EventType, err)
				}
				if event.EventType == observability.EventTypeLLMCallFailed {
					terminal++
					if event.Error == nil || event.Error.Code != tt.wantCode {
						t.Fatalf("terminal error=%+v, want %s", event.Error, tt.wantCode)
					}
				}
			}
			if terminal != 1 {
				t.Fatalf("LLM terminal count=%d events=%+v", terminal, sink.snapshot())
			}
		})
	}
}
