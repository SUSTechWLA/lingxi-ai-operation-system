package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
	videoSvc "github.com/tangying-ai/aios-core/internal/agents/video/service"
	"github.com/tangying-ai/aios-core/internal/core/artifact"
	"github.com/tangying-ai/aios-core/internal/core/auth"
)

func TestCreatorViewHandlerRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewCreatorViewHandler(&fakeCreatorViewProjectReader{}, &fakeCreatorViewReader{}).RegisterRoutes(router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/video-projects/vp-1/creation-view", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreatorViewHandlerVerifiesOwnerBeforeReturningView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	project := &fakeCreatorViewProjectReader{project: &model.VideoProject{ID: "vp-1", UserID: "u-auth"}}
	view := &fakeCreatorViewReader{view: &model.CreationView{Steps: []model.CreatorStep{{ID: model.CreatorStepRequirements}}}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "u-auth"))
		c.Next()
	})
	NewCreatorViewHandler(project, view).RegisterRoutes(router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/video-projects/vp-1/creation-view", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if project.userID != "u-auth" || project.projectID != "vp-1" || view.userID != "u-auth" || view.projectID != "vp-1" {
		t.Fatalf("project=%+v view=%+v", project, view)
	}
}

func TestCreatorViewHandlerDoesNotExposeAViewForMissingOrForbiddenProject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	project := &fakeCreatorViewProjectReader{err: errors.New("not found")}
	view := &fakeCreatorViewReader{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "u-auth"))
		c.Next()
	})
	NewCreatorViewHandler(project, view).RegisterRoutes(router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/video-projects/vp-hidden/creation-view", nil))
	if rec.Code != http.StatusNotFound || view.projectID != "" {
		t.Fatalf("status=%d view=%+v", rec.Code, view)
	}
}

func TestCreatorViewRouteDoesNotConflictWithProjectIDRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "u-auth"))
		c.Next()
	})
	NewProjectHandler(&fakeProjectService{}).RegisterRoutes(router)
	NewCreatorViewHandler(
		&fakeCreatorViewProjectReader{project: &model.VideoProject{ID: "vp-1", UserID: "u-auth"}},
		&fakeCreatorViewReader{view: &model.CreationView{}},
	).RegisterRoutes(router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/video-projects/vp-1/creation-view", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreatorStepRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewCreatorViewHandler(&fakeCreatorViewProjectReader{}, &fakeCreatorViewReader{}).WithStepMutator(&fakeCreatorStepMutator{}).RegisterRoutes(router)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/video-projects/vp-1/steps/script/versions", ""},
		{http.MethodPost, "/api/video-projects/vp-1/steps/script/revision-impact", `{"artifactId":"a","baseVersion":1}`},
		{http.MethodPost, "/api/video-projects/vp-1/steps/script/revisions", `{"artifactId":"a","baseVersion":1,"mode":"direct","directContent":"x"}`},
		{http.MethodPost, "/api/video-projects/vp-1/steps/script/confirm", `{"artifactId":"a"}`},
		{http.MethodPost, "/api/video-projects/vp-1/steps/script/versions/1/restore", `{"baseVersion":2}`},
		{http.MethodPost, "/api/video-projects/vp-1/steps/script/regeneration-impact", `{}`},
		{http.MethodPost, "/api/video-projects/vp-1/steps/script/regenerations", `{"confirmedAffectedStepIds":["shots","preview","delivery"]}`},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "test-key")
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestCreatorAssemblyRebuildRejectsMissingIdempotencyKey(t *testing.T) {
	router := authenticatedCreatorRouter(&fakeCreatorViewProjectReader{project: &model.VideoProject{ID: "vp-1", UserID: "u-auth"}}, &fakeCreatorStepMutator{err: videoSvc.ErrCreatorInvalidRequest})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/video-projects/vp-1/assembly/rebuild", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreatorAssemblyRebuildMapsSnapshotConflictToConflict(t *testing.T) {
	router := authenticatedCreatorRouter(&fakeCreatorViewProjectReader{project: &model.VideoProject{ID: "vp-1", UserID: "u-auth"}}, &fakeCreatorStepMutator{err: videoSvc.ErrShotIdempotencyConflict})
	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/vp-1/assembly/rebuild", nil)
	req.Header.Set("Idempotency-Key", "assembly-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreatorStepRoutesVerifyOwnerAndDelegateAllContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	project := &fakeCreatorViewProjectReader{project: &model.VideoProject{ID: "vp-1", UserID: "u-auth"}}
	mutations := &fakeCreatorStepMutator{
		impact:             model.StepImpact{AffectedStepIDs: []model.CreatorStepID{model.CreatorStepShots}, RequiresConfirmation: true},
		result:             &model.StepMutationResult{Artifact: &artifact.Artifact{ID: "script-v4", Version: 4}, View: &model.CreationView{}},
		regenerationResult: &model.StepRegenerationResult{RunID: "run-1", ReviewID: "review-1", Attempt: 2, View: &model.CreationView{}},
		view:               &model.CreationView{}, versions: &model.StepVersions{Versions: []model.CreatorArtifactVersion{{ArtifactID: "script-v3", Version: 3, IsCurrent: true}}},
	}
	router := authenticatedCreatorRouter(project, mutations)

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/video-projects/vp-1/steps/script/versions", ""},
		{http.MethodPost, "/api/video-projects/vp-1/steps/script/revision-impact", `{"artifactId":"script-v3","baseVersion":3}`},
		{http.MethodPost, "/api/video-projects/vp-1/steps/script/revisions", `{"artifactId":"script-v3","baseVersion":3,"mode":"direct","directContent":"新版"}`},
		{http.MethodPost, "/api/video-projects/vp-1/steps/script/confirm", `{"artifactId":"script-v4"}`},
		{http.MethodPost, "/api/video-projects/vp-1/steps/script/versions/2/restore", `{"baseVersion":4}`},
		{http.MethodPost, "/api/video-projects/vp-1/steps/script/regeneration-impact", `{}`},
		{http.MethodPost, "/api/video-projects/vp-1/steps/script/regenerations", `{"instruction":"rewrite","confirmedAffectedStepIds":["shots"]}`},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "test-key")
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
	if project.userID != "u-auth" || mutations.userID != "u-auth" || mutations.projectID != "vp-1" || mutations.stepID != model.CreatorStepScript || mutations.restoreVersion != 2 || mutations.regenerateCalls != 1 {
		t.Fatalf("project=%+v mutations=%+v", project, mutations)
	}
}

func TestCreatorStepRegenerationRequiresIdempotencyKey(t *testing.T) {
	mutations := &fakeCreatorStepMutator{}
	router := authenticatedCreatorRouter(&fakeCreatorViewProjectReader{project: &model.VideoProject{ID: "vp-1"}}, mutations)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/vp-1/steps/script/regenerations", bytes.NewBufferString(`{"confirmedAffectedStepIds":["shots","preview","delivery"]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || mutations.regenerateCalls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", rec.Code, mutations.regenerateCalls, rec.Body.String())
	}
}

func TestCreatorStepRevisionRequiresPositiveBaseVersionBeforeServiceCall(t *testing.T) {
	mutations := &fakeCreatorStepMutator{}
	router := authenticatedCreatorRouter(&fakeCreatorViewProjectReader{project: &model.VideoProject{ID: "vp-1"}}, mutations)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/vp-1/steps/script/revisions", bytes.NewBufferString(`{"artifactId":"script-v3","mode":"direct","directContent":"新版"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "test-key")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || mutations.reviseCalls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", rec.Code, mutations.reviseCalls, rec.Body.String())
	}
}

func TestCreatorStepHandlerMapsStaleBaseToConflict(t *testing.T) {
	mutations := &fakeCreatorStepMutator{err: videoSvc.ErrCreatorVersionConflict}
	router := authenticatedCreatorRouter(&fakeCreatorViewProjectReader{project: &model.VideoProject{ID: "vp-1"}}, mutations)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/vp-1/steps/script/revisions", bytes.NewBufferString(`{"artifactId":"script-v3","baseVersion":3,"mode":"direct","directContent":"新版"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "test-key")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreatorStepHandlerMapsTextSelectionConflictToConflict(t *testing.T) {
	mutations := &fakeCreatorStepMutator{err: videoSvc.ErrCreatorSelectionConflict}
	router := authenticatedCreatorRouter(&fakeCreatorViewProjectReader{project: &model.VideoProject{ID: "vp-1"}}, mutations)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/vp-1/steps/script/revisions", bytes.NewBufferString(
		`{"artifactId":"script-v3","baseVersion":3,"mode":"instruction","instruction":"rewrite","selection":{"kind":"text","start":0,"end":1,"text":"a"}}`,
	))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "selection-conflict")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreatorStepHandlerExplainsMissingRevisionProvider(t *testing.T) {
	mutations := &fakeCreatorStepMutator{err: videoSvc.ErrCreatorModelProviderUnavailable}
	router := authenticatedCreatorRouter(&fakeCreatorViewProjectReader{project: &model.VideoProject{ID: "vp-1"}}, mutations)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/vp-1/steps/script/revisions", bytes.NewBufferString(`{"artifactId":"script-v3","baseVersion":3,"mode":"instruction","instruction":"rewrite"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "provider-key")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "model provider") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreatorRevisionAndRestoreRequireIdempotencyKey(t *testing.T) {
	mutations := &fakeCreatorStepMutator{}
	router := authenticatedCreatorRouter(&fakeCreatorViewProjectReader{project: &model.VideoProject{ID: "vp-1"}}, mutations)
	for _, tc := range []struct{ path, body string }{
		{"/api/video-projects/vp-1/steps/script/revisions", `{"artifactId":"script-v3","baseVersion":3,"mode":"direct","directContent":"new"}`},
		{"/api/video-projects/vp-1/steps/script/versions/2/restore", `{"baseVersion":3}`},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d body=%s", tc.path, rec.Code, rec.Body.String())
		}
	}
	if mutations.reviseCalls != 0 || mutations.restoreVersion != 0 {
		t.Fatalf("mutations=%+v", mutations)
	}
}

func authenticatedCreatorRouter(projects creatorViewProjectReader, mutations creatorStepMutator) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "u-auth"))
		c.Next()
	})
	NewCreatorViewHandler(projects, &fakeCreatorViewReader{}).WithStepMutator(mutations).RegisterRoutes(router)
	return router
}

type fakeCreatorViewProjectReader struct {
	project   *model.VideoProject
	err       error
	userID    string
	projectID string
}

func (f *fakeCreatorViewProjectReader) GetProject(_ context.Context, userID, projectID string) (*model.VideoProject, error) {
	f.userID, f.projectID = userID, projectID
	return f.project, f.err
}

type fakeCreatorViewReader struct {
	view      *model.CreationView
	err       error
	userID    string
	projectID string
}

func (f *fakeCreatorViewReader) GetCreationView(_ context.Context, userID, projectID string) (*model.CreationView, error) {
	f.userID, f.projectID = userID, projectID
	return f.view, f.err
}

type fakeCreatorStepMutator struct {
	impact             model.StepImpact
	result             *model.StepMutationResult
	regenerationResult *model.StepRegenerationResult
	view               *model.CreationView
	versions           *model.StepVersions
	err                error
	userID             string
	projectID          string
	stepID             model.CreatorStepID
	restoreVersion     int
	reviseCalls        int
	regenerateCalls    int
}

func (f *fakeCreatorStepMutator) capture(userID, projectID string, stepID model.CreatorStepID) {
	f.userID, f.projectID, f.stepID = userID, projectID, stepID
}

func (f *fakeCreatorStepMutator) GetStepVersions(_ context.Context, userID, projectID string, stepID model.CreatorStepID) (*model.StepVersions, error) {
	f.capture(userID, projectID, stepID)
	return f.versions, f.err
}

func (f *fakeCreatorStepMutator) PreviewStepRevision(_ context.Context, userID, projectID string, stepID model.CreatorStepID, req model.StepRevisionRequest) (model.StepImpact, error) {
	f.capture(userID, projectID, stepID)
	return f.impact, f.err
}

func (f *fakeCreatorStepMutator) ReviseStep(_ context.Context, userID, projectID string, stepID model.CreatorStepID, req model.StepRevisionRequest) (*model.StepMutationResult, error) {
	f.capture(userID, projectID, stepID)
	f.reviseCalls++
	return f.result, f.err
}

func (f *fakeCreatorStepMutator) ConfirmStep(_ context.Context, userID, projectID string, stepID model.CreatorStepID, req model.StepConfirmRequest) (*model.CreationView, error) {
	f.capture(userID, projectID, stepID)
	return f.view, f.err
}

func (f *fakeCreatorStepMutator) RestoreStepVersion(_ context.Context, userID, projectID string, stepID model.CreatorStepID, version int, req model.StepRestoreRequest) (*model.StepMutationResult, error) {
	f.capture(userID, projectID, stepID)
	f.restoreVersion = version
	return f.result, f.err
}

func (f *fakeCreatorStepMutator) PreviewStepRegeneration(_ context.Context, userID, projectID string, stepID model.CreatorStepID) (model.StepImpact, error) {
	f.capture(userID, projectID, stepID)
	return f.impact, f.err
}

func (f *fakeCreatorStepMutator) RegenerateStep(_ context.Context, userID, projectID string, stepID model.CreatorStepID, _ model.StepRegenerationRequest, _ string) (*model.StepRegenerationResult, error) {
	f.capture(userID, projectID, stepID)
	f.regenerateCalls++
	return f.regenerationResult, f.err
}

func (f *fakeCreatorStepMutator) RebuildFinalAssembly(_ context.Context, userID, projectID, _ string) (videoSvc.AssemblyRebuildResult, error) {
	f.userID, f.projectID = userID, projectID
	return videoSvc.AssemblyRebuildResult{}, f.err
}
