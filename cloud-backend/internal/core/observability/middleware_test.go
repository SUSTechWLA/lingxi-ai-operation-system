package observability

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tangying-ai/aios-core/internal/core/trustedcontext"
)

func TestMiddlewareDefersPairedEventsUntilPostAuthOwnerExists(t *testing.T) {
	sink := &ownerCapturingSink{}
	emitter := NewEmitter(testSource(), Runtime{}, sink, 4)
	r := gin.New()
	r.Use(Middleware(emitter))
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(trustedcontext.WithUserID(c.Request.Context(), "user-1"))
		c.Next()
	})
	r.GET("/private", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/private", nil))
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sink.owners) != 2 || sink.owners[0] != "user-1" || sink.owners[1] != "user-1" {
		t.Fatalf("owners=%v", sink.owners)
	}
	if !sink.events[0].OccurredAt.Before(sink.events[1].OccurredAt) && !sink.events[0].OccurredAt.Equal(sink.events[1].OccurredAt) {
		t.Fatalf("event order=%v", sink.events)
	}
}

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
	if len(events) != 2 {
		t.Fatalf("emitted events = %d, want 2", len(events))
	}
	if events[0].EventType != EventTypeRequestAccepted || events[1].EventType != EventTypeRequestCompleted {
		t.Fatalf("request event types = %q, %q", events[0].EventType, events[1].EventType)
	}
	if events[1].Correlation.TraceID != "trc_4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("wire trace ID = %q", events[1].Correlation.TraceID)
	}
	if events[1].Correlation.ParentSpanID != "spn_00f067aa0ba902b7" {
		t.Fatalf("wire parent span ID = %q", events[1].Correlation.ParentSpanID)
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
	if len(events) != 2 {
		t.Fatalf("emitted events = %d, want 2", len(events))
	}
	for _, event := range events {
		if ContainsSecret(event) {
			t.Fatalf("emitted event contains a secret: %#v", event)
		}
		if strings.Contains(event.MessageKey, "secret") {
			t.Fatalf("message key leaked request data: %q", event.MessageKey)
		}
	}
}

func TestMiddlewareEmitsExactlyOneFailedTerminalEventOnPanic(t *testing.T) {
	sink := &memorySink{}
	emitter := NewEmitter(testSource(), Runtime{}, sink, 4)
	r := gin.New()
	recoveries := 0
	r.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		recoveries++
		c.AbortWithStatus(http.StatusInternalServerError)
	}))
	r.Use(Middleware(emitter))
	r.GET("/panic", func(*gin.Context) { panic("test panic") })
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	events, _ := sink.snapshot()
	if len(events) != 2 || events[0].EventType != EventTypeRequestAccepted || events[1].EventType != EventTypeRequestFailed {
		t.Fatalf("request events = %#v", events)
	}
	if events[1].Execution.Status != ExecutionStatusFailed {
		t.Fatalf("terminal status = %q", events[1].Execution.Status)
	}
	if recoveries != 1 || rec.Code != http.StatusInternalServerError {
		t.Fatalf("outer recoveries = %d, status = %d", recoveries, rec.Code)
	}
}

func TestMiddlewarePanicAfterCommittedSuccessStillEmitsOneFailedTerminalAndRepanics(t *testing.T) {
	sink := &memorySink{}
	emitter := NewEmitter(testSource(), Runtime{}, sink, 4)
	r := gin.New()
	recoveries := 0
	r.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		recoveries++
		c.AbortWithStatus(http.StatusInternalServerError)
	}))
	r.Use(Middleware(emitter))
	r.GET("/panic-after-write", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
		c.Writer.WriteHeaderNow()
		panic("test panic after committed response")
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic-after-write", nil))
	if err := emitter.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	events, _ := sink.snapshot()
	if len(events) != 2 || events[0].EventType != EventTypeRequestAccepted || events[1].EventType != EventTypeRequestFailed {
		t.Fatalf("request events = %#v", events)
	}
	if recoveries != 1 {
		t.Fatalf("outer recoveries = %d, want 1", recoveries)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("committed response status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestMiddlewareRejectsInvalidTraceparentAndPreservesSamplingFlag(t *testing.T) {
	tests := []struct {
		name   string
		header string
		valid  bool
		flag   string
	}{
		{"sampled", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", true, "01"},
		{"uppercase trace", "00-4BF92F3577B34DA6A3CE929D0E0E4736-00f067aa0ba902b7-01", false, "00"},
		{"uppercase span", "00-4bf92f3577b34da6a3ce929d0e0e4736-00F067AA0BA902B7-01", false, "00"},
		{"zero trace", "00-00000000000000000000000000000000-00f067aa0ba902b7-01", false, "00"},
		{"zero span", "00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01", false, "00"},
		{"malformed", "not-a-traceparent", false, "00"},
	}
	hex32 := regexp.MustCompile(`^[0-9a-f]{32}$`)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(Middleware(nil))
			r.GET("/x", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			req.Header.Set("traceparent", tt.header)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			parts := strings.Split(rec.Header().Get("traceparent"), "-")
			if len(parts) != 4 || !hex32.MatchString(parts[1]) || parts[3] != tt.flag {
				t.Fatalf("response traceparent = %q", rec.Header().Get("traceparent"))
			}
			if tt.valid && parts[1] != "4bf92f3577b34da6a3ce929d0e0e4736" {
				t.Fatalf("valid trace ID changed to %q", parts[1])
			}
			if !tt.valid && strings.Contains(tt.header, parts[1]) {
				t.Fatalf("invalid trace ID was preserved: %q", parts[1])
			}
		})
	}
}
