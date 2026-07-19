package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
	videoSvc "github.com/tangying-ai/aios-core/internal/agents/video/service"
	"github.com/tangying-ai/aios-core/internal/core/auth"
)

func TestCreationHandlerRejectsUnauthenticatedSpecAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewCreationHandler(&fakeCreationService{}).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/video-projects/vp-1/spec", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreationHandlerUsesAuthenticatedUserForShotLock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeCreationService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "u-auth"))
		c.Next()
	})
	NewCreationHandler(fake).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/vp-1/shots/shot-1/lock", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fake.userID != "u-auth" || fake.projectID != "vp-1" || fake.shotID != "shot-1" {
		t.Fatalf("captured = user:%s project:%s shot:%s", fake.userID, fake.projectID, fake.shotID)
	}
}

func TestCreationHandlerRegenerateShotV2PassesHeaderAndRequestFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeCreationService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "u-auth"))
		c.Next()
	})
	NewCreationHandler(fake).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/vp-1/shots/shot-1/regenerations", strings.NewReader(`{"baseVersion":3,"scope":"base_media","locks":["duration"],"instruction":"make it warmer"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "request-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fake.userID != "u-auth" || fake.projectID != "vp-1" || fake.shotID != "shot-1" || fake.regenerateV2Req.BaseVersion != 3 || fake.regenerateV2Req.Scope != "base_media" || fake.regenerateV2Req.IdempotencyKey != "request-1" || len(fake.regenerateV2Req.Locks) != 1 || fake.regenerateV2Req.Instruction != "make it warmer" {
		t.Fatalf("captured request = %+v user=%q project=%q shot=%q", fake.regenerateV2Req, fake.userID, fake.projectID, fake.shotID)
	}
}

func TestCreationHandlerMapsShotVersionConflictToConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeCreationService{regenerateV2Err: videoSvc.ErrShotVersionConflict}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "u-auth"))
		c.Next()
	})
	NewCreationHandler(fake).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/vp-1/shots/shot-1/regenerations", strings.NewReader(`{"baseVersion":3,"scope":"base_media"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "request-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreationHandlerCandidateMutationRequiresHeaderAndPassesContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeCreationService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "u-auth"))
		c.Next()
	})
	NewCreationHandler(fake).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/video-projects/vp-1/shots/shot-1/candidates/candidate-1/accept", strings.NewReader(`{"baseVersion":3,"scope":"candidate_accept","locks":["duration"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "accept-1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || fake.candidateMutationReq.BaseVersion != 3 || fake.candidateMutationReq.Scope != "candidate_accept" || fake.candidateMutationReq.IdempotencyKey != "accept-1" || len(fake.candidateMutationReq.Locks) != 1 {
		t.Fatalf("status=%d request=%+v body=%s", rec.Code, fake.candidateMutationReq, rec.Body.String())
	}

	missing := httptest.NewRequest(http.MethodPost, "/api/video-projects/vp-1/shots/shot-1/candidates/candidate-1/accept", strings.NewReader(`{"baseVersion":3,"scope":"candidate_accept"}`))
	missing.Header.Set("Content-Type", "application/json")
	missingRec := httptest.NewRecorder()
	router.ServeHTTP(missingRec, missing)
	if missingRec.Code != http.StatusBadRequest {
		t.Fatalf("missing idempotency header status=%d body=%s", missingRec.Code, missingRec.Body.String())
	}
}

type fakeCreationService struct {
	userID               string
	projectID            string
	shotID               string
	regenerateV2Req      videoSvc.RegenerateShotRequest
	regenerateV2Err      error
	candidateMutationReq videoSvc.CandidateMutationRequest
}

func (f *fakeCreationService) GetSpec(ctx context.Context, userID, projectID string) (*model.VideoCreationSpec, error) {
	f.userID, f.projectID = userID, projectID
	return model.NewVideoCreationSpec(projectID, "test"), nil
}

func (f *fakeCreationService) UpsertSpec(ctx context.Context, userID, projectID string, spec *model.VideoCreationSpec) (*model.VideoCreationSpec, error) {
	f.userID, f.projectID = userID, projectID
	return spec, nil
}

func (f *fakeCreationService) GenerateSpec(ctx context.Context, userID, projectID string, req videoSvc.GenerateSpecRequest) (*model.VideoCreationSpec, error) {
	f.userID, f.projectID = userID, projectID
	return model.NewVideoCreationSpec(projectID, req.SourceMessage), nil
}

func (f *fakeCreationService) ApproveSpec(ctx context.Context, userID, projectID string) (*model.VideoCreationSpec, error) {
	f.userID, f.projectID = userID, projectID
	spec := model.NewVideoCreationSpec(projectID, "test")
	spec.Status = model.ReviewStatusApproved
	return spec, nil
}

func (f *fakeCreationService) RejectSpec(ctx context.Context, userID, projectID string, reason string) (*model.VideoCreationSpec, error) {
	f.userID, f.projectID = userID, projectID
	spec := model.NewVideoCreationSpec(projectID, "test")
	spec.Status = model.ReviewStatusRejected
	return spec, nil
}

func (f *fakeCreationService) ListShots(ctx context.Context, userID, projectID string) ([]model.ShotUnit, error) {
	f.userID, f.projectID = userID, projectID
	return []model.ShotUnit{}, nil
}

func (f *fakeCreationService) ListShotPage(ctx context.Context, userID, projectID string, query model.ShotPageQuery) (model.ShotPage, error) {
	f.userID, f.projectID = userID, projectID
	return model.ShotPage{}, nil
}

func (f *fakeCreationService) GetShotSummary(ctx context.Context, userID, projectID string) (model.ShotSummary, error) {
	f.userID, f.projectID = userID, projectID
	return model.ShotSummary{}, nil
}

func (f *fakeCreationService) GetShotWorkspace(ctx context.Context, userID, projectID, shotID string) (model.ShotWorkspace, error) {
	f.userID, f.projectID, f.shotID = userID, projectID, shotID
	return model.ShotWorkspace{Shot: model.ShotUnit{ID: shotID, ProjectID: projectID}}, nil
}

func (f *fakeCreationService) GetShotHistory(ctx context.Context, userID, projectID, shotID string) ([]model.ShotRevision, error) {
	f.userID, f.projectID, f.shotID = userID, projectID, shotID
	return nil, nil
}

func (f *fakeCreationService) PreviewShotRegeneration(ctx context.Context, userID, projectID, shotID string) (videoSvc.ShotRegenerationImpact, error) {
	f.userID, f.projectID, f.shotID = userID, projectID, shotID
	return videoSvc.ShotRegenerationImpact{ShotID: shotID}, nil
}

func (f *fakeCreationService) AcceptShotCandidate(ctx context.Context, userID, projectID, shotID, candidateID string, req videoSvc.CandidateMutationRequest) (*model.ShotUnit, error) {
	f.userID, f.projectID, f.shotID, f.candidateMutationReq = userID, projectID, shotID, req
	return &model.ShotUnit{ID: shotID, ProjectID: projectID, AcceptedCandidateID: candidateID, Version: req.BaseVersion + 1}, nil
}

func (f *fakeCreationService) RestoreShotCandidate(ctx context.Context, userID, projectID, shotID, candidateID string, req videoSvc.CandidateMutationRequest) (*model.ShotUnit, error) {
	return f.AcceptShotCandidate(ctx, userID, projectID, shotID, candidateID, req)
}

func (f *fakeCreationService) GetShot(ctx context.Context, userID, projectID, shotID string) (*model.ShotUnit, error) {
	f.userID, f.projectID, f.shotID = userID, projectID, shotID
	return &model.ShotUnit{ID: shotID, ProjectID: projectID}, nil
}

func (f *fakeCreationService) UpsertShot(ctx context.Context, userID, projectID string, shot *model.ShotUnit) (*model.ShotUnit, error) {
	f.userID, f.projectID = userID, projectID
	shot.ProjectID = projectID
	return shot, nil
}

func (f *fakeCreationService) GenerateShots(ctx context.Context, userID, projectID string) ([]model.ShotUnit, error) {
	f.userID, f.projectID = userID, projectID
	return []model.ShotUnit{{ID: "shot-1", ProjectID: projectID}}, nil
}

func (f *fakeCreationService) ApproveShot(ctx context.Context, userID, projectID, shotID string) (*model.ShotUnit, error) {
	f.userID, f.projectID, f.shotID = userID, projectID, shotID
	return &model.ShotUnit{ID: shotID, ProjectID: projectID, ReviewStatus: model.ReviewStatusApproved}, nil
}

func (f *fakeCreationService) RejectShot(ctx context.Context, userID, projectID, shotID string, reason string) (*model.ShotUnit, error) {
	f.userID, f.projectID, f.shotID = userID, projectID, shotID
	return &model.ShotUnit{ID: shotID, ProjectID: projectID, ReviewStatus: model.ReviewStatusRejected, LastRejectReason: reason}, nil
}

func (f *fakeCreationService) LockShot(ctx context.Context, userID, projectID, shotID string) (*model.ShotUnit, error) {
	f.userID, f.projectID, f.shotID = userID, projectID, shotID
	return &model.ShotUnit{ID: shotID, ProjectID: projectID, Locked: true}, nil
}

func (f *fakeCreationService) UnlockShot(ctx context.Context, userID, projectID, shotID string) (*model.ShotUnit, error) {
	f.userID, f.projectID, f.shotID = userID, projectID, shotID
	return &model.ShotUnit{ID: shotID, ProjectID: projectID, Locked: false}, nil
}

func (f *fakeCreationService) RegenerateShot(ctx context.Context, userID, projectID, shotID string, req videoSvc.RegenerateShotRequest) (*model.ShotUnit, error) {
	f.userID, f.projectID, f.shotID = userID, projectID, shotID
	return &model.ShotUnit{ID: shotID, ProjectID: projectID, Stale: true}, nil
}

func (f *fakeCreationService) RegenerateShotV2(ctx context.Context, userID, projectID, shotID string, req videoSvc.RegenerateShotRequest) (videoSvc.RegenerateShotResult, error) {
	f.userID, f.projectID, f.shotID, f.regenerateV2Req = userID, projectID, shotID, req
	if f.regenerateV2Err != nil {
		return videoSvc.RegenerateShotResult{}, f.regenerateV2Err
	}
	return videoSvc.RegenerateShotResult{Shot: model.ShotUnit{ID: shotID, ProjectID: projectID, Version: req.BaseVersion + 1}}, nil
}

func (f *fakeCreationService) GenerateVisualPlan(ctx context.Context, userID, projectID, shotID string) (*model.VisualPlan, error) {
	f.userID, f.projectID, f.shotID = userID, projectID, shotID
	return &model.VisualPlan{Canvas: model.CanvasSpec{Width: 1920, Height: 1080}}, nil
}

func (f *fakeCreationService) DecideShotRenderStrategy(ctx context.Context, userID, projectID, shotID string) (*model.RenderStrategy, error) {
	f.userID, f.projectID, f.shotID = userID, projectID, shotID
	return &model.RenderStrategy{Mode: model.RenderModeHTMLOnly}, nil
}

func (f *fakeCreationService) GenerateTextLayers(ctx context.Context, userID, projectID, shotID string) ([]model.TextLayerSpec, error) {
	f.userID, f.projectID, f.shotID = userID, projectID, shotID
	return []model.TextLayerSpec{}, nil
}

func (f *fakeCreationService) Assemble(ctx context.Context, userID, projectID string) ([]videoSvc.ValidationIssue, error) {
	f.userID, f.projectID = userID, projectID
	return nil, nil
}

func (f *fakeCreationService) GeneratePublishPackage(ctx context.Context, userID, projectID string) (map[string]interface{}, error) {
	f.userID, f.projectID = userID, projectID
	return map[string]interface{}{"projectId": projectID}, nil
}
