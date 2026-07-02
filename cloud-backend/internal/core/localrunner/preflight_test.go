package localrunner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakePreflightService struct {
	hasRunner bool
	commands  map[string]bool
}

func (f fakePreflightService) HasOnlineRunner(context.Context, string) (bool, error) {
	return f.hasRunner, nil
}

func (f fakePreflightService) SupportsCommand(_ context.Context, _ string, command string) (bool, error) {
	return f.commands[command], nil
}

func TestHandleVideoPreflightGuidedProfileRequiresRenderTools(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/video/preflight", withUser("user-1", HandleVideoPreflight(fakePreflightService{
		hasRunner: true,
		commands: map[string]bool{
			"HYPERFRAMES_PROJECT_GENERATE": true,
			"HYPERFRAMES_SNAPSHOT":         true,
			"HYPERFRAMES_RENDER":           true,
			"FFMPEG_PROBE":                 true,
			"ARTIFACT_PACKAGE":             true,
		},
	})))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/video/preflight?pipeline=wf-guided-image-text-video", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body PreflightResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Pipeline != "wf-guided-image-text-video" {
		t.Fatalf("expected guided pipeline, got %q", body.Pipeline)
	}
	if !body.CanStart || body.Status != "passed" {
		t.Fatalf("expected passed preflight, got status=%s canStart=%v blockers=%v", body.Status, body.CanStart, body.Blockers)
	}
	if len(body.CapabilityMenu.LocalTools) != 5 {
		t.Fatalf("expected 5 guided render tools, got %d", len(body.CapabilityMenu.LocalTools))
	}
}

func TestHandleVideoPreflightAIGCShotProfileRequiresExternalImport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/video/preflight", withUser("user-1", HandleVideoPreflight(fakePreflightService{
		hasRunner: true,
		commands: map[string]bool{
			"LOCAL_FILE_IMPORT": true,
			"FFMPEG_PROBE":      true,
			"ARTIFACT_PACKAGE":  true,
		},
	})))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/video/preflight?pipeline=wf-aigc-shot-video", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body PreflightResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Pipeline != "wf-aigc-shot-video" {
		t.Fatalf("expected aigc shot pipeline, got %q", body.Pipeline)
	}
	if !body.CanStart || body.Status != "passed" {
		t.Fatalf("expected passed preflight, got status=%s canStart=%v blockers=%v", body.Status, body.CanStart, body.Blockers)
	}
	if !hasTool(body.CapabilityMenu.LocalTools, "LOCAL_FILE_IMPORT") {
		t.Fatalf("expected LOCAL_FILE_IMPORT in capability menu: %+v", body.CapabilityMenu.LocalTools)
	}
	if !hasToolCommand(body.CapabilityMenu.LocalTools, "LOCAL_MCP_TOOL_CALL") {
		t.Fatalf("expected optional LOCAL_MCP_TOOL_CALL in capability menu: %+v", body.CapabilityMenu.LocalTools)
	}
}

func TestHandleVideoPreflightBlocksWhenAIGCShotImportMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/video/preflight", withUser("user-1", HandleVideoPreflight(fakePreflightService{
		hasRunner: true,
		commands: map[string]bool{
			"FFMPEG_PROBE":     true,
			"ARTIFACT_PACKAGE": true,
		},
	})))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/video/preflight?pipeline=wf-aigc-shot-video", nil)
	router.ServeHTTP(rec, req)

	var body PreflightResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.CanStart || body.Status != "blocked" {
		t.Fatalf("expected blocked preflight, got status=%s canStart=%v", body.Status, body.CanStart)
	}
	if !hasBlocker(body.Blockers, "LOCAL_FILE_IMPORT_NOT_AVAILABLE") {
		t.Fatalf("expected LOCAL_FILE_IMPORT blocker, got %+v", body.Blockers)
	}
}

func withUser(userID string, next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("userID", userID)
		next(c)
	}
}

func hasTool(tools []LocalToolStatus, command string) bool {
	for _, tool := range tools {
		if tool.Command == command && tool.Available {
			return true
		}
	}
	return false
}

func hasToolCommand(tools []LocalToolStatus, command string) bool {
	for _, tool := range tools {
		if tool.Command == command {
			return true
		}
	}
	return false
}

func hasBlocker(blockers []PreflightBlocker, code string) bool {
	for _, blocker := range blockers {
		if blocker.Code == code {
			return true
		}
	}
	return false
}
