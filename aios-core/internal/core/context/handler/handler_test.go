package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestContextHandler_RecordContext_BadRequest(t *testing.T) {
	r := gin.New()
	h := &ContextHandler{}
	h.RegisterRoutes(r)

	req := httptest.NewRequest("POST", "/api/context/record", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for empty body, got %d", w.Code)
	}
}

func TestContextHandler_RouteRegistration(t *testing.T) {
	r := gin.New()
	h := &ContextHandler{}
	h.RegisterRoutes(r)

	routes := r.Routes()
	expectedPaths := map[string]bool{
		"GET-/api/context/:taskId":                          false,
		"GET-/api/context/:taskId/node/:nodeId/snapshot/latest": false,
		"POST-/api/context/:taskId/node/:nodeId/restore":    false,
		"POST-/api/context/record":                          false,
	}

	for _, route := range routes {
		key := route.Method + "-" + route.Path
		if _, ok := expectedPaths[key]; ok {
			expectedPaths[key] = true
		}
	}

	for path, found := range expectedPaths {
		if !found {
			t.Errorf("Expected route %s to be registered", path)
		}
	}
}
