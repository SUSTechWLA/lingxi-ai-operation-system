package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMiddlewarePreservesIncomingTraceparent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := &memorySink{}
	emitter := NewEmitter(testSource(), Runtime{}, sink, 16)
	r := gin.New()
	r.Use(Middleware(emitter))
	r.GET("/x", func(c *gin.Context) {
		c.String(http.StatusOK, CorrelationFromContext(c.Request.Context()).TraceID)
	})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Body.String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace id = %q", rec.Body.String())
	}
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	events, _ := sink.snapshot()
	if len(events) != 1 {
		t.Fatalf("emitted events = %d, want 1", len(events))
	}
	if events[0].Correlation.TraceID != "trc_4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("wire trace ID = %q", events[0].Correlation.TraceID)
	}
	if events[0].Correlation.ParentSpanID != "spn_00f067aa0ba902b7" {
		t.Fatalf("wire parent span ID = %q", events[0].Correlation.ParentSpanID)
	}
}

func TestMiddlewareDoesNotEmitQueryOrHeaderSecrets(t *testing.T) {
	sink := &memorySink{}
	emitter := NewEmitter(testSource(), Runtime{}, sink, 4)
	r := gin.New()
	r.Use(Middleware(emitter))
	r.GET("/search", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/search?token=raw-query-secret", nil)
	req.Header.Set("authorization", "Bearer raw-header-secret")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	events, _ := sink.snapshot()
	if len(events) != 1 {
		t.Fatalf("emitted events = %d, want 1", len(events))
	}
	if ContainsSecret(events[0]) {
		t.Fatalf("emitted event contains a secret: %#v", events[0])
	}
	if strings.Contains(events[0].MessageKey, "secret") {
		t.Fatalf("message key leaked request data: %q", events[0].MessageKey)
	}
}
