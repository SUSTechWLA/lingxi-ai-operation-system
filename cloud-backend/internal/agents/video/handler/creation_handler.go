package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
	videoSvc "github.com/tangying-ai/aios-core/internal/agents/video/service"
)

type creationService interface {
	GetSpec(ctx context.Context, userID, projectID string) (*model.VideoCreationSpec, error)
	UpsertSpec(ctx context.Context, userID, projectID string, spec *model.VideoCreationSpec) (*model.VideoCreationSpec, error)
	GenerateSpec(ctx context.Context, userID, projectID string, req videoSvc.GenerateSpecRequest) (*model.VideoCreationSpec, error)
	ApproveSpec(ctx context.Context, userID, projectID string) (*model.VideoCreationSpec, error)
	RejectSpec(ctx context.Context, userID, projectID string, reason string) (*model.VideoCreationSpec, error)
	ListShots(ctx context.Context, userID, projectID string) ([]model.ShotUnit, error)
	ListShotPage(ctx context.Context, userID, projectID string, query model.ShotPageQuery) (model.ShotPage, error)
	GetShotSummary(ctx context.Context, userID, projectID string) (model.ShotSummary, error)
	GetShotWorkspace(ctx context.Context, userID, projectID, shotID string) (model.ShotWorkspace, error)
	GetShotHistory(ctx context.Context, userID, projectID, shotID string) ([]model.ShotRevision, error)
	PreviewShotRegeneration(ctx context.Context, userID, projectID, shotID string) (videoSvc.ShotRegenerationImpact, error)
	RegenerateShotV2(ctx context.Context, userID, projectID, shotID string, req videoSvc.RegenerateShotRequest) (videoSvc.RegenerateShotResult, error)
	AcceptShotCandidate(ctx context.Context, userID, projectID, shotID, candidateID string, baseVersion int) (*model.ShotUnit, error)
	RestoreShotCandidate(ctx context.Context, userID, projectID, shotID, candidateID string, baseVersion int) (*model.ShotUnit, error)
	GetShot(ctx context.Context, userID, projectID, shotID string) (*model.ShotUnit, error)
	UpsertShot(ctx context.Context, userID, projectID string, shot *model.ShotUnit) (*model.ShotUnit, error)
	GenerateShots(ctx context.Context, userID, projectID string) ([]model.ShotUnit, error)
	ApproveShot(ctx context.Context, userID, projectID, shotID string) (*model.ShotUnit, error)
	RejectShot(ctx context.Context, userID, projectID, shotID string, reason string) (*model.ShotUnit, error)
	LockShot(ctx context.Context, userID, projectID, shotID string) (*model.ShotUnit, error)
	UnlockShot(ctx context.Context, userID, projectID, shotID string) (*model.ShotUnit, error)
	RegenerateShot(ctx context.Context, userID, projectID, shotID string, req videoSvc.RegenerateShotRequest) (*model.ShotUnit, error)
	GenerateVisualPlan(ctx context.Context, userID, projectID, shotID string) (*model.VisualPlan, error)
	DecideShotRenderStrategy(ctx context.Context, userID, projectID, shotID string) (*model.RenderStrategy, error)
	GenerateTextLayers(ctx context.Context, userID, projectID, shotID string) ([]model.TextLayerSpec, error)
	Assemble(ctx context.Context, userID, projectID string) ([]videoSvc.ValidationIssue, error)
	GeneratePublishPackage(ctx context.Context, userID, projectID string) (map[string]interface{}, error)
}

type CreationHandler struct {
	svc        creationService
	middleware []gin.HandlerFunc
}

func NewCreationHandler(svc creationService, middleware ...gin.HandlerFunc) *CreationHandler {
	return &CreationHandler{svc: svc, middleware: middleware}
}

func (h *CreationHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/video-projects", h.middleware...)
	{
		api.GET("/:id/spec", h.GetSpec)
		api.POST("/:id/spec", h.UpsertSpec)
		api.POST("/:id/spec/generate", h.GenerateSpec)
		api.POST("/:id/spec/approve", h.ApproveSpec)
		api.POST("/:id/spec/reject", h.RejectSpec)
		api.GET("/:id/shots", h.ListShots)
		api.GET("/:id/shots/summary", h.GetShotSummary)
		api.POST("/:id/shots", h.UpsertShot)
		api.POST("/:id/shots/generate", h.GenerateShots)
		api.GET("/:id/shots/:shotId/workspace", h.GetShotWorkspace)
		api.GET("/:id/shots/:shotId/history", h.GetShotHistory)
		api.GET("/:id/shots/:shotId", h.GetShot)
		api.PATCH("/:id/shots/:shotId", h.UpdateShot)
		api.POST("/:id/shots/:shotId/approve", h.ApproveShot)
		api.POST("/:id/shots/:shotId/reject", h.RejectShot)
		api.POST("/:id/shots/:shotId/lock", h.LockShot)
		api.POST("/:id/shots/:shotId/unlock", h.UnlockShot)
		api.POST("/:id/shots/:shotId/regenerate", h.RegenerateShot)
		api.POST("/:id/shots/:shotId/regeneration-impact", h.PreviewShotRegeneration)
		api.POST("/:id/shots/:shotId/regenerations", h.RegenerateShotV2)
		api.POST("/:id/shots/:shotId/candidates/:candidateId/accept", h.AcceptShotCandidate)
		api.POST("/:id/shots/:shotId/candidates/:candidateId/restore", h.RestoreShotCandidate)
		api.POST("/:id/shots/:shotId/visual-plan/generate", h.GenerateVisualPlan)
		api.POST("/:id/shots/:shotId/render-strategy/decide", h.DecideRenderStrategy)
		api.POST("/:id/shots/:shotId/text-layers/generate", h.GenerateTextLayers)
		api.POST("/:id/assemble", h.Assemble)
		api.POST("/:id/publish-package/generate", h.GeneratePublishPackage)
	}
}

func (h *CreationHandler) GetSpec(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	spec, err := h.svc.GetSpec(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, gin.H{"spec": spec})
}

func (h *CreationHandler) UpsertSpec(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	var spec model.VideoCreationSpec
	if err := c.ShouldBindJSON(&spec); err != nil {
		fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	saved, err := h.svc.UpsertSpec(c.Request.Context(), userID, c.Param("id"), &spec)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"spec": saved})
}

func (h *CreationHandler) GenerateSpec(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	var req videoSvc.GenerateSpecRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	spec, err := h.svc.GenerateSpec(c.Request.Context(), userID, c.Param("id"), req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"spec": spec})
}

func (h *CreationHandler) ApproveSpec(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	spec, err := h.svc.ApproveSpec(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"spec": spec})
}

func (h *CreationHandler) RejectSpec(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	var req videoSvc.RejectRequest
	_ = c.ShouldBindJSON(&req)
	spec, err := h.svc.RejectSpec(c.Request.Context(), userID, c.Param("id"), req.Reason)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"spec": spec})
}

func (h *CreationHandler) ListShots(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	query := model.ShotPageQuery{
		Cursor: c.Query("cursor"), Status: c.Query("status"), Chapter: c.Query("chapter"), Query: c.Query("query"), Limit: 24,
	}
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil {
			fail(c, http.StatusBadRequest, "invalid limit")
			return
		}
		query.Limit = limit
	}
	page, err := h.svc.ListShotPage(c.Request.Context(), userID, c.Param("id"), query)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"shots": page.Items, "nextCursor": page.NextCursor, "total": page.Total})
}

func (h *CreationHandler) GetShotSummary(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	summary, err := h.svc.GetShotSummary(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"summary": summary})
}

func (h *CreationHandler) GetShotWorkspace(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	workspace, err := h.svc.GetShotWorkspace(c.Request.Context(), userID, c.Param("id"), c.Param("shotId"))
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, gin.H{"workspace": workspace})
}

func (h *CreationHandler) GetShotHistory(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	history, err := h.svc.GetShotHistory(c.Request.Context(), userID, c.Param("id"), c.Param("shotId"))
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, gin.H{"history": history})
}

func (h *CreationHandler) GetShot(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	shot, err := h.svc.GetShot(c.Request.Context(), userID, c.Param("id"), c.Param("shotId"))
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	ok(c, gin.H{"shot": shot})
}

func (h *CreationHandler) UpsertShot(c *gin.Context) {
	h.saveShot(c)
}

func (h *CreationHandler) UpdateShot(c *gin.Context) {
	h.saveShot(c)
}

func (h *CreationHandler) saveShot(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	var shot model.ShotUnit
	if err := c.ShouldBindJSON(&shot); err != nil {
		fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	if shot.ID == "" {
		shot.ID = c.Param("shotId")
	}
	saved, err := h.svc.UpsertShot(c.Request.Context(), userID, c.Param("id"), &shot)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"shot": saved})
}

func (h *CreationHandler) GenerateShots(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	shots, err := h.svc.GenerateShots(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"shots": shots})
}

func (h *CreationHandler) ApproveShot(c *gin.Context) {
	h.shotAction(c, h.svc.ApproveShot)
}

func (h *CreationHandler) LockShot(c *gin.Context) {
	h.shotAction(c, h.svc.LockShot)
}

func (h *CreationHandler) UnlockShot(c *gin.Context) {
	h.shotAction(c, h.svc.UnlockShot)
}

func (h *CreationHandler) shotAction(
	c *gin.Context,
	action func(context.Context, string, string, string) (*model.ShotUnit, error),
) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	shot, err := action(c.Request.Context(), userID, c.Param("id"), c.Param("shotId"))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"shot": shot})
}

func (h *CreationHandler) RejectShot(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	var req videoSvc.RejectRequest
	_ = c.ShouldBindJSON(&req)
	shot, err := h.svc.RejectShot(c.Request.Context(), userID, c.Param("id"), c.Param("shotId"), req.Reason)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"shot": shot})
}

func (h *CreationHandler) RegenerateShot(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	var req videoSvc.RegenerateShotRequest
	_ = c.ShouldBindJSON(&req)
	shot, err := h.svc.RegenerateShot(c.Request.Context(), userID, c.Param("id"), c.Param("shotId"), req)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"shot": shot})
}

func (h *CreationHandler) PreviewShotRegeneration(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	impact, err := h.svc.PreviewShotRegeneration(c.Request.Context(), userID, c.Param("id"), c.Param("shotId"))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"impact": impact})
}

func (h *CreationHandler) RegenerateShotV2(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	var req videoSvc.RegenerateShotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	req.IdempotencyKey = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if req.IdempotencyKey == "" {
		fail(c, http.StatusBadRequest, "Idempotency-Key header is required")
		return
	}
	result, err := h.svc.RegenerateShotV2(c.Request.Context(), userID, c.Param("id"), c.Param("shotId"), req)
	if err != nil {
		failShotReview(c, err)
		return
	}
	ok(c, gin.H{"shot": result.Shot, "task": result.Task})
}

type candidateMutationRequest struct {
	BaseVersion int `json:"baseVersion"`
}

func (h *CreationHandler) AcceptShotCandidate(c *gin.Context) {
	h.mutateShotCandidate(c, h.svc.AcceptShotCandidate)
}

func (h *CreationHandler) RestoreShotCandidate(c *gin.Context) {
	h.mutateShotCandidate(c, h.svc.RestoreShotCandidate)
}

func (h *CreationHandler) mutateShotCandidate(c *gin.Context, action func(context.Context, string, string, string, string, int) (*model.ShotUnit, error)) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	var req candidateMutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	shot, err := action(c.Request.Context(), userID, c.Param("id"), c.Param("shotId"), c.Param("candidateId"), req.BaseVersion)
	if err != nil {
		failShotReview(c, err)
		return
	}
	ok(c, gin.H{"shot": shot})
}

func failShotReview(c *gin.Context, err error) {
	if errors.Is(err, videoSvc.ErrShotVersionConflict) {
		fail(c, http.StatusConflict, err.Error())
		return
	}
	fail(c, http.StatusBadRequest, err.Error())
}

func (h *CreationHandler) GenerateVisualPlan(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	plan, err := h.svc.GenerateVisualPlan(c.Request.Context(), userID, c.Param("id"), c.Param("shotId"))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"visualPlan": plan})
}

func (h *CreationHandler) DecideRenderStrategy(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	strategy, err := h.svc.DecideShotRenderStrategy(c.Request.Context(), userID, c.Param("id"), c.Param("shotId"))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"renderStrategy": strategy})
}

func (h *CreationHandler) GenerateTextLayers(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	layers, err := h.svc.GenerateTextLayers(c.Request.Context(), userID, c.Param("id"), c.Param("shotId"))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"textLayers": layers})
}

func (h *CreationHandler) Assemble(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	issues, err := h.svc.Assemble(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"issues": issues})
}

func (h *CreationHandler) GeneratePublishPackage(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	pkg, err := h.svc.GeneratePublishPackage(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"publishPackage": pkg})
}
