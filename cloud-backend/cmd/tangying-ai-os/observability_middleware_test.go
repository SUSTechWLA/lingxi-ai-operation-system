package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tangying-ai/aios-core/internal/core/observability"
)

type serverEventSink struct {
	mu     sync.Mutex
	events []observability.Event
}

func (s *serverEventSink) Write(_ context.Context, event observability.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (*serverEventSink) Close(context.Context) error { return nil }

func TestNewHTTPRouterInstallsRecoveryCORSAndObservability(t *testing.T) {
	sink := &serverEventSink{}
	emitter := observability.NewEmitter(
		observability.Source{Service: "cloud-backend", Component: "http-server", Environment: "test"},
		observability.Runtime{},
		sink,
		4,
	)
	router := newHTTPRouter([]string{"https://creator.example"}, emitter)
	router.GET("/panic", func(_ *gin.Context) {
		panic("test panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic?token=must-not-leak", nil)
	req.Header.Set("origin", "https://creator.example")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if got := rec.Header().Get("access-control-allow-origin"); got != "https://creator.example" {
		t.Fatalf("allow origin = %q", got)
	}
	if got := rec.Header().Get("traceparent"); got == "" {
		t.Fatal("traceparent response header is missing")
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.events) != 1 {
		t.Fatalf("observability events = %d, want 1", len(sink.events))
	}
}
