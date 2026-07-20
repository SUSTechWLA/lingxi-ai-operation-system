package assets

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
	"github.com/tangying-ai/aios-core/internal/core/artifact"
	"github.com/tangying-ai/aios-core/internal/core/auth"
)

func TestRegisterExternalGenerationResultCreatesLocalMediaArtifact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := &fakeRegisterArtifactSink{}
	router := gin.New()
	NewHandler(sink, nil).RegisterRoutes(router)

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
	NewHandler(&fakeRegisterArtifactSink{}, nil).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/project-1/external-generation-results", strings.NewReader(`{"kind":"image"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing storageRef should return 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRegisterProjectMaterialRouteExists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(&fakeRegisterArtifactSink{}, nil).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/vp-1/materials", strings.NewReader(`{
		"name":"采访录音.wav",
		"kind":"audio",
		"storageRef":"local://vp-1/materials/interview-audio",
		"mimeType":"audio/wav",
		"sizeBytes":102400,
		"contentHash":"sha256:audio-1"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Fatalf("material registration route is not registered: %s", rec.Body.String())
	}
}

func TestRegisterProjectMaterialCreatesRequirementsArtifactWithoutPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := &fakeRegisterArtifactSink{}
	projects := &fakeProjectMaterialReader{project: &model.VideoProject{ID: "vp-1", UserID: "u-auth"}}
	router := authenticatedMaterialRouter(sink, projects)

	rec := postProjectMaterial(router, "vp-1", `{
		"name":"采访录音.wav",
		"kind":"audio",
		"storageRef":"local://vp-1/materials/interview-audio",
		"mimeType":"audio/wav",
		"sizeBytes":102400,
		"contentHash":"sha256:audio-1"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if projects.userID != "u-auth" || projects.projectID != "vp-1" {
		t.Fatalf("project ownership lookup = user=%q project=%q", projects.userID, projects.projectID)
	}
	if sink.lastReq == nil {
		t.Fatal("artifact sink was not called")
	}
	if sink.lastReq.ProjectID != "vp-1" || sink.lastReq.StageName != "requirements" || sink.lastReq.UnitID != "source-materials" {
		t.Fatalf("artifact scope = %+v", sink.lastReq)
	}
	if sink.lastReq.Kind != artifact.KindAudio || sink.lastReq.StorageType != artifact.StorageLocal || len(sink.lastReq.Data) != 0 {
		t.Fatalf("material storage = %+v", sink.lastReq)
	}
	for key, want := range map[string]interface{}{
		"relatedProjectId": "vp-1", "name": "采访录音.wav", "kind": "audio", "mimeType": "audio/wav",
		"sizeBytes": int64(102400), "storageRef": "local://vp-1/materials/interview-audio", "contentHash": "sha256:audio-1",
	} {
		if got := sink.lastReq.Metadata[key]; got != want {
			t.Fatalf("metadata[%q] = %#v, want %#v", key, got, want)
		}
	}
	if strings.Contains(rec.Body.String(), "cloudUrl") || strings.Contains(rec.Body.String(), "inlineJson") || strings.Contains(rec.Body.String(), "bytes") {
		t.Fatalf("response exposed a payload or cloud URL: %s", rec.Body.String())
	}
	var body struct {
		Data struct {
			Material ProjectMaterial   `json:"material"`
			Artifact artifact.Artifact `json:"artifact"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid response: %v", err)
	}
	if body.Data.Material.Name != "采访录音.wav" || body.Data.Material.StorageRef != "local://vp-1/materials/interview-audio" || body.Data.Artifact.ID == "" {
		t.Fatalf("response data = %+v", body.Data)
	}
}

func TestRegisterProjectMaterialAcceptsDocumentMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := &fakeRegisterArtifactSink{}
	router := authenticatedMaterialRouter(sink, &fakeProjectMaterialReader{project: &model.VideoProject{ID: "vp-1", UserID: "u-auth"}})
	rec := postProjectMaterial(router, "vp-1", `{
		"name":"brief.pdf","kind":"document","storageRef":"local://projects/vp-1/materials/brief",
		"mimeType":"application/pdf","sizeBytes":7,"contentHash":"sha256:document-1"
	}`)
	if rec.Code != http.StatusOK || sink.lastReq == nil || sink.lastReq.Kind != artifact.KindBundle {
		t.Fatalf("status=%d request=%+v body=%s", rec.Code, sink.lastReq, rec.Body.String())
	}
}

func TestRegisterProjectMaterialRejectsInvalidOrPayloadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	valid := `{"name":"a.wav","kind":"audio","storageRef":"local://vp-1/materials/a","mimeType":"audio/wav","sizeBytes":0,"contentHash":"sha256:audio-1"}`
	cases := []struct{ name, body string }{
		{"unsupported kind", strings.Replace(valid, `"audio"`, `"archive"`, 1)},
		{"blank name", strings.Replace(valid, `"a.wav"`, `" "`, 1)},
		{"blank MIME", strings.Replace(valid, `"audio/wav"`, `" "`, 1)},
		{"negative size", strings.Replace(valid, `"sizeBytes":0`, `"sizeBytes":-1`, 1)},
		{"malformed hash", strings.Replace(valid, `"sha256:audio-1"`, `"not a hash"`, 1)},
		{"cloud storage", strings.Replace(valid, `"local://vp-1/materials/a"`, `"https://bucket/a"`, 1)},
		{"different project", strings.Replace(valid, `"local://vp-1/materials/a"`, `"local://vp-2/materials/a"`, 1)},
		{"path traversal", strings.Replace(valid, `"local://vp-1/materials/a"`, `"local://vp-1/materials/../vp-2/a"`, 1)},
		{"inline payload", strings.TrimSuffix(valid, "}") + `,"data":"base64-not-allowed"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink := &fakeRegisterArtifactSink{}
			router := authenticatedMaterialRouter(sink, &fakeProjectMaterialReader{project: &model.VideoProject{ID: "vp-1", UserID: "u-auth"}})
			rec := postProjectMaterial(router, "vp-1", tc.body)
			if rec.Code != http.StatusBadRequest || sink.lastReq != nil {
				t.Fatalf("status=%d sink=%+v body=%s", rec.Code, sink.lastReq, rec.Body.String())
			}
		})
	}
}

func TestRegisterProjectMaterialRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := &fakeRegisterArtifactSink{}
	router := gin.New()
	NewHandler(sink, &fakeProjectMaterialReader{}).RegisterRoutes(router)
	rec := postProjectMaterial(router, "vp-1", validProjectMaterialJSON("sha256:audio-1"))
	if rec.Code != http.StatusUnauthorized || sink.lastReq != nil {
		t.Fatalf("status=%d sink=%+v body=%s", rec.Code, sink.lastReq, rec.Body.String())
	}
}

func TestRegisterProjectMaterialHidesForbiddenProject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := &fakeRegisterArtifactSink{}
	router := authenticatedMaterialRouter(sink, &fakeProjectMaterialReader{err: errors.New("not found")})
	rec := postProjectMaterial(router, "vp-hidden", validProjectMaterialJSON("sha256:audio-1"))
	if rec.Code != http.StatusNotFound || sink.lastReq != nil {
		t.Fatalf("status=%d sink=%+v body=%s", rec.Code, sink.lastReq, rec.Body.String())
	}
}

func TestRegisterProjectMaterialDoesNotLeakArtifactServiceFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := authenticatedMaterialRouter(fakeMaterialFailureSink{}, &fakeProjectMaterialReader{project: &model.VideoProject{ID: "vp-1", UserID: "u-auth"}})
	rec := postProjectMaterial(router, "vp-1", validProjectMaterialJSON("sha256:audio-1"))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "database connection password") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRegisterProjectMaterialIsIdempotentByContentHash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := &fakeIdempotentMaterialSink{}
	router := authenticatedMaterialRouter(sink, &fakeProjectMaterialReader{project: &model.VideoProject{ID: "vp-1", UserID: "u-auth"}})

	first := postProjectMaterial(router, "vp-1", validProjectMaterialJSON("sha256:audio-1"))
	second := postProjectMaterial(router, "vp-1", validProjectMaterialJSON("sha256:audio-1"))
	third := postProjectMaterial(router, "vp-1", validProjectMaterialJSON("sha256:audio-2"))
	for _, rec := range []*httptest.ResponseRecorder{first, second, third} {
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
	if sink.createCalls != 3 || len(sink.byHash) != 2 {
		t.Fatalf("calls=%d identities=%d", sink.createCalls, len(sink.byHash))
	}
	if materialArtifactID(t, first) != materialArtifactID(t, second) || materialArtifactID(t, first) == materialArtifactID(t, third) {
		t.Fatalf("same hash must return the same artifact, different hash must not")
	}
}

func authenticatedMaterialRouter(sink ArtifactSink, projects ProjectMaterialReader) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "u-auth"))
		c.Next()
	})
	NewHandler(sink, projects).RegisterRoutes(router)
	return router
}

func postProjectMaterial(router *gin.Engine, projectID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/"+projectID+"/materials", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func validProjectMaterialJSON(contentHash string) string {
	return `{"name":"a.wav","kind":"audio","storageRef":"local://vp-1/materials/a","mimeType":"audio/wav","sizeBytes":0,"contentHash":"` + contentHash + `"}`
}

func materialArtifactID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Data struct {
			Artifact artifact.Artifact `json:"artifact"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body.Data.Artifact.ID
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

type fakeProjectMaterialReader struct {
	project   *model.VideoProject
	err       error
	userID    string
	projectID string
}

func (f *fakeProjectMaterialReader) GetProject(_ context.Context, userID, projectID string) (*model.VideoProject, error) {
	f.userID = userID
	f.projectID = projectID
	return f.project, f.err
}

type fakeIdempotentMaterialSink struct {
	byHash      map[string]*artifact.Artifact
	createCalls int
}

type fakeMaterialFailureSink struct{}

func (fakeMaterialFailureSink) CreateArtifact(context.Context, *artifact.CreateArtifactRequest) (*artifact.Artifact, error) {
	return nil, errors.New("database connection password leaked")
}

func (f *fakeIdempotentMaterialSink) CreateArtifact(_ context.Context, req *artifact.CreateArtifactRequest) (*artifact.Artifact, error) {
	f.createCalls++
	if f.byHash == nil {
		f.byHash = map[string]*artifact.Artifact{}
	}
	if existing := f.byHash[req.ContentHash]; existing != nil {
		return existing, nil
	}
	record := &artifact.Artifact{
		ID:          "material-" + string(rune('1'+len(f.byHash))),
		ProjectID:   req.ProjectID,
		StageName:   req.StageName,
		UnitID:      req.UnitID,
		Kind:        req.Kind,
		Name:        req.Name,
		StorageType: req.StorageType,
		StorageRef:  req.StorageRef,
		MimeType:    req.MimeType,
		SizeBytes:   req.SizeBytes,
		ContentHash: req.ContentHash,
		Metadata:    req.Metadata,
	}
	f.byHash[req.ContentHash] = record
	return record, nil
}
