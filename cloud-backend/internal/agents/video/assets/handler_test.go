package assets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/artifact"
)

func TestRegisterExternalGenerationResultCreatesLocalMediaArtifact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := &fakeRegisterArtifactSink{}
	router := gin.New()
	NewHandler(sink).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/project-1/external-generation-results", strings.NewReader(`{
		"kind":"video",
		"storageType":"local",
		"storageRef":"local://projects/project-1/artifacts/extgen-video-1/hash/shot-1.mp4",
		"mimeType":"video/mp4",
		"sizeBytes":42,
		"contentHash":"sha256:abc123",
		"promptHash":"prompt-sha",
		"relatedShotId":"shot-1",
		"generationRequestId":"extgen_123",
		"externalPlatform":"seedance-web",
		"referenceAssetIds":["char-a","scene-bookstore"]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("register status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if sink.lastReq == nil {
		t.Fatalf("artifact sink was not called")
	}
	if sink.lastReq.ProjectID != "project-1" || sink.lastReq.Kind != artifact.KindVideo {
		t.Fatalf("unexpected artifact request: %+v", sink.lastReq)
	}
	if sink.lastReq.StorageType != artifact.StorageLocal || len(sink.lastReq.Data) != 0 {
		t.Fatalf("registered media must stay local: storage=%s data=%d", sink.lastReq.StorageType, len(sink.lastReq.Data))
	}
	if sink.lastReq.ContentHash != "sha256:abc123" || sink.lastReq.StorageRef == "" {
		t.Fatalf("missing local index fields: %+v", sink.lastReq)
	}

	var body struct {
		Data struct {
			Manifest ManualAssetManifest `json:"manifest"`
			Artifact artifact.Artifact   `json:"artifact"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if body.Data.Manifest.GenerationRequestID != "extgen_123" {
		t.Fatalf("manifest should preserve generation request id: %+v", body.Data.Manifest)
	}
	if body.Data.Artifact.Kind != artifact.KindVideo {
		t.Fatalf("response should include created artifact: %+v", body.Data.Artifact)
	}
}

func TestRegisterExternalGenerationResultRejectsMissingStorageRef(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(&fakeRegisterArtifactSink{}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/project-1/external-generation-results", strings.NewReader(`{"kind":"image"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing storageRef should return 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

type fakeRegisterArtifactSink struct {
	lastReq *artifact.CreateArtifactRequest
}

func (s *fakeRegisterArtifactSink) CreateArtifact(ctx context.Context, req *artifact.CreateArtifactRequest) (*artifact.Artifact, error) {
	s.lastReq = req
	return &artifact.Artifact{
		ID:          "artifact-1",
		ProjectID:   req.ProjectID,
		StageName:   req.StageName,
		UnitID:      req.UnitID,
		Kind:        req.Kind,
		Name:        req.Name,
		StorageType: req.StorageType,
		StorageRef:  req.StorageRef,
		ContentHash: req.ContentHash,
		PromptHash:  req.PromptHash,
		Provider:    req.Provider,
		Metadata:    req.Metadata,
	}, nil
}
