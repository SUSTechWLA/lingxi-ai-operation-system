package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

type stubManifestWriter struct {
	registered *tool.ToolManifest
}

func (s *stubManifestWriter) RegisterExternal(_ context.Context, manifest *tool.ToolManifest) error {
	s.registered = manifest
	return nil
}

func (s *stubManifestWriter) DeregisterExternal(context.Context, string) error { return nil }

func TestRegisterToolRequiresInternalAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := tool.NewToolRegistry()
	writer := &stubManifestWriter{}
	handler := NewToolHandler(registry, writer).WithRegistrationInternalToken("internal-registration-secret")
	router := gin.New()
	handler.RegisterRoutes(router)
	body := `{"name":"safe_tool","description":"safe","type":"external","endpoint":"https://tool.example.invalid/call"}`

	request := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/tools/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("X-Internal-Tool-Token", token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}

	if response := request(""); response.Code != http.StatusForbidden || writer.registered != nil {
		t.Fatalf("ordinary authenticated route access must not mutate global tools: status=%d manifest=%#v", response.Code, writer.registered)
	}
	if response := request("wrong"); response.Code != http.StatusForbidden || writer.registered != nil {
		t.Fatalf("wrong internal token must be rejected: status=%d manifest=%#v", response.Code, writer.registered)
	}
	if response := request("internal-registration-secret"); response.Code != http.StatusOK || writer.registered == nil {
		t.Fatalf("internal registration failed: status=%d body=%s manifest=%#v", response.Code, response.Body.String(), writer.registered)
	}
}

func TestRegisterToolReturnsBadRequestForInvalidCanonicalSchema(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := tool.NewToolRegistry()
	service := tool.NewToolManifestService(nil, nil, registry)
	handler := NewToolHandler(registry, service).WithRegistrationInternalToken("test-internal-token")
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
	req.Header.Set("X-Internal-Tool-Token", "test-internal-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", response.Code, response.Body.String())
	}
	if registry.GetExternalManifest("invalid_contract") != nil {
		t.Fatal("invalid HTTP registration mutated registry")
	}
}
