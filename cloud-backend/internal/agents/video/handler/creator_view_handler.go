package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
	videoSvc "github.com/tangying-ai/aios-core/internal/agents/video/service"
	"github.com/tangying-ai/aios-core/internal/core/agentruntime"
	"github.com/tangying-ai/aios-core/internal/core/artifact"
	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
)

type creatorViewProjectReader interface {
	GetProject(ctx context.Context, userID, projectID string) (*model.VideoProject, error)
}

type creatorViewReader interface {
	GetCreationView(ctx context.Context, userID, projectID string) (*model.CreationView, error)
}

type creatorStepMutator interface {
	GetStepVersions(ctx context.Context, userID, projectID string, stepID model.CreatorStepID) (*model.StepVersions, error)
	PreviewStepRevision(ctx context.Context, userID, projectID string, stepID model.CreatorStepID, req model.StepRevisionRequest) (model.StepImpact, error)
	ReviseStep(ctx context.Context, userID, projectID string, stepID model.CreatorStepID, req model.StepRevisionRequest) (*model.StepMutationResult, error)
	ConfirmStep(ctx context.Context, userID, projectID string, stepID model.CreatorStepID, req model.StepConfirmRequest) (*model.CreationView, error)
	RestoreStepVersion(ctx context.Context, userID, projectID string, stepID model.CreatorStepID, version int, req model.StepRestoreRequest) (*model.StepMutationResult, error)
}

// CreatorViewHandler serves the creator-facing aggregate without exposing
// internal workflow stages or requiring the client to calculate progress.
type CreatorViewHandler struct {
	projects   creatorViewProjectReader
	views      creatorViewReader
	mutations  creatorStepMutator
	middleware []gin.HandlerFunc
}

func NewCreatorViewHandler(projects creatorViewProjectReader, views creatorViewReader, middleware ...gin.HandlerFunc) *CreatorViewHandler {
	h := &CreatorViewHandler{projects: projects, views: views, middleware: middleware}
	if mutations, ok := views.(creatorStepMutator); ok {
		h.mutations = mutations
	}
	return h
}

func (h *CreatorViewHandler) WithStepMutator(mutations creatorStepMutator) *CreatorViewHandler {
	h.mutations = mutations
	return h
}

func (h *CreatorViewHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/video-projects", h.middleware...)
	api.GET("/:id/creation-view", h.GetCreationView)
	api.GET("/:id/steps/:stepId/versions", h.GetStepVersions)
	api.POST("/:id/steps/:stepId/revision-impact", h.PreviewStepRevision)
	api.POST("/:id/steps/:stepId/revisions", h.ReviseStep)
	api.POST("/:id/steps/:stepId/confirm", h.ConfirmStep)
	api.POST("/:id/steps/:stepId/versions/:version/restore", h.RestoreStepVersion)
}

func (h *CreatorViewHandler) GetStepVersions(c *gin.Context) {
	userID, projectID, ok := h.authorizeProject(c)
	if !ok {
		return
	}
	if h.mutations == nil {
		fail(c, http.StatusInternalServerError, "content tools are unavailable")
		return
	}
	versions, err := h.mutations.GetStepVersions(c.Request.Context(), userID, projectID, model.CreatorStepID(c.Param("stepId")))
	if err != nil {
		h.failMutation(c, err)
		return
	}
	httpx.OK(c, versions)
}

func (h *CreatorViewHandler) PreviewStepRevision(c *gin.Context) {
	userID, projectID, ok := h.authorizeProject(c)
	if !ok {
		return
	}
	if !h.mutationsAvailable(c) {
		return
	}
	var req model.StepRevisionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ArtifactID == "" || req.BaseVersion <= 0 {
		fail(c, http.StatusBadRequest, "invalid content request")
		return
	}
	impact, err := h.mutations.PreviewStepRevision(c.Request.Context(), userID, projectID, model.CreatorStepID(c.Param("stepId")), req)
	if err != nil {
		h.failMutation(c, err)
		return
	}
	httpx.OK(c, impact)
}

func (h *CreatorViewHandler) ReviseStep(c *gin.Context) {
	userID, projectID, ok := h.authorizeProject(c)
	if !ok {
		return
	}
	if !h.mutationsAvailable(c) {
		return
	}
	var req model.StepRevisionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ArtifactID == "" || req.BaseVersion <= 0 {
		fail(c, http.StatusBadRequest, "invalid content request")
		return
	}
	result, err := h.mutations.ReviseStep(c.Request.Context(), userID, projectID, model.CreatorStepID(c.Param("stepId")), req)
	if err != nil {
		h.failMutation(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *CreatorViewHandler) ConfirmStep(c *gin.Context) {
	userID, projectID, ok := h.authorizeProject(c)
	if !ok {
		return
	}
	if !h.mutationsAvailable(c) {
		return
	}
	var req model.StepConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ArtifactID == "" {
		fail(c, http.StatusBadRequest, "invalid confirmation request")
		return
	}
	view, err := h.mutations.ConfirmStep(c.Request.Context(), userID, projectID, model.CreatorStepID(c.Param("stepId")), req)
	if err != nil {
		h.failMutation(c, err)
		return
	}
	httpx.OK(c, view)
}

func (h *CreatorViewHandler) RestoreStepVersion(c *gin.Context) {
	userID, projectID, ok := h.authorizeProject(c)
	if !ok {
		return
	}
	if !h.mutationsAvailable(c) {
		return
	}
	version, err := strconv.Atoi(c.Param("version"))
	if err != nil || version <= 0 {
		fail(c, http.StatusBadRequest, "invalid version")
		return
	}
	var req model.StepRestoreRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.BaseVersion <= 0 {
		fail(c, http.StatusBadRequest, "invalid restore request")
		return
	}
	result, err := h.mutations.RestoreStepVersion(c.Request.Context(), userID, projectID, model.CreatorStepID(c.Param("stepId")), version, req)
	if err != nil {
		h.failMutation(c, err)
		return
	}
	httpx.OK(c, result)
}

func (h *CreatorViewHandler) mutationsAvailable(c *gin.Context) bool {
	if h.mutations != nil {
		return true
	}
	fail(c, http.StatusInternalServerError, "content tools are unavailable")
	return false
}

func (h *CreatorViewHandler) authorizeProject(c *gin.Context) (string, string, bool) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return "", "", false
	}
	projectID := c.Param("id")
	project, err := h.projects.GetProject(c.Request.Context(), userID, projectID)
	if err != nil || project == nil {
		fail(c, http.StatusNotFound, "project not found")
		return "", "", false
	}
	return userID, projectID, true
}

func (h *CreatorViewHandler) failMutation(c *gin.Context, err error) {
	switch {
	case errors.Is(err, videoSvc.ErrCreatorVersionConflict), errors.Is(err, artifact.ErrArtifactVersionConflict),
		errors.Is(err, agentruntime.ErrReviewNotPending), errors.Is(err, agentruntime.ErrReviewCannotReopen),
		errors.Is(err, agentruntime.ErrReviewGateAmbiguous):
		fail(c, http.StatusConflict, "content changed; reload and try again")
	case errors.Is(err, videoSvc.ErrCreatorArtifactNotFound), errors.Is(err, artifact.ErrRevisionArtifactNotFound),
		errors.Is(err, agentruntime.ErrReviewNotFound):
		fail(c, http.StatusNotFound, "content not found")
	case errors.Is(err, videoSvc.ErrCreatorStepInvalid), errors.Is(err, videoSvc.ErrCreatorInvalidRequest),
		errors.Is(err, videoSvc.ErrCreatorImpactMismatch), errors.Is(err, agentruntime.ErrReviewReferenceMismatch):
		fail(c, http.StatusBadRequest, "invalid content request")
	default:
		fail(c, http.StatusInternalServerError, "content update failed")
	}
}

// GetCreationView authenticates and verifies ownership before reading the
// aggregate. Project lookup failures deliberately use the project endpoint's
// 404 behavior so a caller cannot distinguish a forbidden project from one
// that does not exist.
func (h *CreatorViewHandler) GetCreationView(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	projectID := c.Param("id")
	project, err := h.projects.GetProject(c.Request.Context(), userID, projectID)
	if err != nil || project == nil {
		fail(c, http.StatusNotFound, "project not found")
		return
	}
	view, err := h.views.GetCreationView(c.Request.Context(), userID, projectID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load creation view")
		return
	}
	if view == nil {
		fail(c, http.StatusInternalServerError, "failed to load creation view")
		return
	}
	httpx.OK(c, view)
}
