package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	return r
}

func TestPublishHandler_AIGenerateContent_MissingPrompt(t *testing.T) {
	r := setupRouter()
	h := NewPublishHandler(nil)
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]string{})
	req, _ := http.NewRequest("POST", "/api/ai/generate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] != float64(400) {
		t.Errorf("expected code 400, got %v", response["code"])
	}
}

func TestPublishHandler_AIPolishText_MissingText(t *testing.T) {
	r := setupRouter()
	h := NewPublishHandler(nil)
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]string{})
	req, _ := http.NewRequest("POST", "/api/ai/polish", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestPublishHandler_PublishContent_MissingFields(t *testing.T) {
	r := setupRouter()
	h := NewPublishHandler(nil)
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]string{})
	req, _ := http.NewRequest("POST", "/api/publish", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] != float64(400) {
		t.Errorf("expected code 400, got %v", response["code"])
	}
}

func TestPublishHandler_PublishContent_ValidRequest_NilService(t *testing.T) {
	r := setupRouter()
	h := NewPublishHandler(nil)
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]interface{}{
		"title":       "test title",
		"description": "test description",
		"platforms":   []string{"douyin"},
	})
	req, _ := http.NewRequest("POST", "/api/publish", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestPublishHandler_AIPolishText_DefaultType(t *testing.T) {
	r := setupRouter()
	h := NewPublishHandler(nil)
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]string{
		"text": "some text to polish",
	})
	req, _ := http.NewRequest("POST", "/api/ai/polish", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestPublishHandler_AIGenerateContent_NilService(t *testing.T) {
	r := setupRouter()
	h := NewPublishHandler(nil)
	h.RegisterRoutes(r)

	body, _ := json.Marshal(map[string]string{"prompt": "test"})
	req, _ := http.NewRequest("POST", "/api/ai/generate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d, body: %s", w.Code, w.Body.String())
	}
}
