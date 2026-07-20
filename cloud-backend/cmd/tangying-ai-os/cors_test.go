package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSMiddlewareAllowsCaseInsensitiveIdempotencyKeyPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(corsMiddleware([]string{"https://creator.example"}))
	router.POST("/api/video-projects/:id/steps/:stepId/revisions", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodOptions, "/api/video-projects/vp-1/steps/script/revisions", nil)
	req.Header.Set("origin", "https://creator.example")
	req.Header.Set("access-control-request-method", "POST")
	req.Header.Set("access-control-request-headers", "content-type, idempotency-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("access-control-allow-origin"); got != "https://creator.example" {
		t.Fatalf("allow origin = %q", got)
	}
	if !containsHeaderName(rec.Header().Get("access-control-allow-headers"), "IDEMPOTENCY-KEY") {
		t.Fatalf("allow headers = %q, want case-insensitive Idempotency-Key", rec.Header().Get("access-control-allow-headers"))
	}
}

func containsHeaderName(value, want string) bool {
	for _, candidate := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(candidate), want) {
			return true
		}
	}
	return false
}
