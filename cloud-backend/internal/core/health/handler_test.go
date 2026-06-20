package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestReadinessReturnsOKWhenDependenciesPass(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler([]DependencyCheck{
		{Name: "postgres", Check: func(context.Context) error { return nil }},
		{Name: "redis", Check: func(context.Context) error { return nil }},
		{Name: "kafka", Check: func(context.Context) error { return nil }},
	}).RegisterRoutes(router)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health/ready", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body["status"] != "UP" {
		t.Fatalf("status = %v, want UP", body["status"])
	}
}

func TestReadinessReturnsUnavailableWhenDependencyFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler([]DependencyCheck{
		{Name: "postgres", Check: func(context.Context) error { return nil }},
		{Name: "kafka", Check: func(context.Context) error { return errors.New("broker unavailable") }},
	}).RegisterRoutes(router)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health/ready", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Status       string `json:"status"`
		Dependencies map[string]struct {
			Status string `json:"status"`
			Error  string `json:"error,omitempty"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if body.Status != "DOWN" {
		t.Fatalf("status = %s, want DOWN", body.Status)
	}
	if body.Dependencies["kafka"].Status != "DOWN" || body.Dependencies["kafka"].Error == "" {
		t.Fatalf("kafka failure should be reported: %+v", body.Dependencies)
	}
}
