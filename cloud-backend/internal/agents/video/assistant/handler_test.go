package assistant

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAssistantMessageIsScopedToVideoProject(t *testing.T) {
	router := testRouter()

	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/vp-1/assistant/message", strings.NewReader(`{
		"message":"当前做到哪一步？",
		"stage":"script",
		"runId":"run-1"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeEnvelope(t, rec.Body.String())
	data := body["data"].(map[string]interface{})
	if data["scope"] != "video_project" {
		t.Fatalf("scope = %v, want video_project", data["scope"])
	}
	if data["projectId"] != "vp-1" {
		t.Fatalf("projectId = %v, want vp-1", data["projectId"])
	}
	if strings.TrimSpace(data["answer"].(string)) == "" {
		t.Fatal("answer should not be empty")
	}
}

func TestAssistantReviseDelegatesToArtifactRevision(t *testing.T) {
	router := testRouter()

	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/vp-1/assistant/revise", strings.NewReader(`{
		"artifactId":"art-1",
		"message":"把开头改得更直接",
		"reviewId":"review-1"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeEnvelope(t, rec.Body.String())
	data := body["data"].(map[string]interface{})
	if data["bypassesArtifact"] != false {
		t.Fatalf("bypassesArtifact = %v, want false", data["bypassesArtifact"])
	}
	action := data["artifactAction"].(map[string]interface{})
	if action["method"] != http.MethodPost {
		t.Fatalf("artifactAction.method = %v, want POST", action["method"])
	}
	if action["path"] != "/api/artifacts/art-1/revise" {
		t.Fatalf("artifactAction.path = %v", action["path"])
	}
}

func TestAssistantExplainStageReturnsBetaGuidance(t *testing.T) {
	router := testRouter()

	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/vp-1/assistant/explain-stage", strings.NewReader(`{
		"stage":"preview"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeEnvelope(t, rec.Body.String())
	data := body["data"].(map[string]interface{})
	if data["stage"] != "preview" {
		t.Fatalf("stage = %v, want preview", data["stage"])
	}
	if !strings.Contains(data["explanation"].(string), "预览") {
		t.Fatalf("explanation should mention preview, got %q", data["explanation"])
	}
}

func testRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler().RegisterRoutes(router)
	return router
}

func decodeEnvelope(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("invalid json: %v body=%s", err, raw)
	}
	return body
}
