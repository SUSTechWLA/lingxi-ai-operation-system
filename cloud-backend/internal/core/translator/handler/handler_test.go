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

func TestTranslatorHandler_Translate_BadRequest(t *testing.T) {
	r := gin.New()
	h := &TranslatorHandler{}
	h.RegisterRoutes(r)

	req := httptest.NewRequest("POST", "/api/translate", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for empty body, got %d", w.Code)
	}
}

func TestTranslatorHandler_TranslateAndSubmit_BadRequest(t *testing.T) {
	r := gin.New()
	h := &TranslatorHandler{}
	h.RegisterRoutes(r)

	req := httptest.NewRequest("POST", "/api/translate/submit", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for empty body, got %d", w.Code)
	}
}

func TestTranslatorHandler_RouteRegistration(t *testing.T) {
	r := gin.New()
	h := &TranslatorHandler{}
	h.RegisterRoutes(r)

	routes := r.Routes()
	expectedPaths := map[string]bool{
		"POST-/api/translate":        false,
		"POST-/api/translate/submit": false,
		"GET-/api/task/:taskId/status": false,
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
