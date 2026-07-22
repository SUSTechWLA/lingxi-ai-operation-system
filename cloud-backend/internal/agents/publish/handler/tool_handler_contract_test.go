package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestRegisterToolReturnsBadRequestForInvalidCanonicalSchema(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := tool.NewToolRegistry()
	service := tool.NewToolManifestService(nil, nil, registry)
	handler := NewToolHandler(registry, service)
	router := gin.New()
	handler.RegisterRoutes(router)

	body := `{
		"name":"invalid_contract",
		"description":"invalid",
		"type":"external",
		"endpoint":"https://tool.example.invalid/call",
		"inputSchema":{"type":"object","properties":{"value":{"$ref":"https://schemas.example.invalid/value.json"}}}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/tools/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", response.Code, response.Body.String())
	}
	if registry.GetExternalManifest("invalid_contract") != nil {
		t.Fatal("invalid HTTP registration mutated registry")
	}
}
