package tool

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestManifestHandler_RegisterExternalTool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := NewToolRegistry()
	service := NewToolManifestService(&fakeManifestRepo{}, nil, registry)
	router := gin.New()
	NewManifestHandler(service).RegisterRoutes(router)

	body := `{
		"name": "parse_bid_files",
		"description": "Parse tender documents",
		"type": "http",
		"endpoint": "http://127.0.0.1:9001/tools/parse_bid_files",
		"capabilities": ["bid_writing", "bid_parsing", "document_parsing"],
		"parameters": {
			"file_path": {"type": "string", "required": true}
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/tools/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if registry.GetExternalManifest("parse_bid_files") == nil {
		t.Fatal("registered external tool should be visible in registry")
	}
}
