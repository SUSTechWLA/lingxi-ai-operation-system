package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
	"github.com/tangying-ai/aios-core/internal/core/artifact"
	"github.com/tangying-ai/aios-core/internal/core/auth"
	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
	"github.com/tangying-ai/aios-core/internal/core/localrunner"
	"go.uber.org/zap"
)

type projectService interface {
	CreateProject(ctx context.Context, userID string, req *model.CreateProjectRequest) (*model.VideoProject, error)
	ListProjects(ctx context.Context, userID string, modeFilter, statusFilter string, offset, limit int) ([]*model.VideoProject, int, error)
	GetProject(ctx context.Context, userID string, id string) (*model.VideoProject, error)
	UpdateProject(ctx context.Context, userID string, id string, req *model.UpdateProjectRequest) (*model.VideoProject, error)
	ArchiveProject(ctx context.Context, userID string, id string) error
}

// ProjectHandler serves HTTP endpoints for video project management.
type ProjectHandler struct {
	svc                 projectService
	middleware          []gin.HandlerFunc
	artifactSvc         *artifact.Service
	localRunnerSvc      *localrunner.Service
	sessionRouteHandler *SessionRouteHandler
}

func NewProjectHandler(svc projectService, middleware ...gin.HandlerFunc) *ProjectHandler {
	return &ProjectHandler{svc: svc, middleware: middleware}
}

// WithSessionDependencies injects the services needed for the project session endpoint.
func (h *ProjectHandler) WithSessionDependencies(artifactSvc *artifact.Service, localRunnerSvc *localrunner.Service) *ProjectHandler {
	h.artifactSvc = artifactSvc
	h.localRunnerSvc = localRunnerSvc
	h.sessionRouteHandler = &SessionRouteHandler{
		artifactSvc:    artifactSvc,
		localRunnerSvc: localRunnerSvc,
	}
	return h
}

// SessionRouteHandler handles the GET /api/video-projects/:projectId/session endpoint.
type SessionRouteHandler struct {
	artifactSvc    *artifact.Service
	localRunnerSvc *localrunner.Service
}

func (h *ProjectHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/video-projects", h.middleware...)
	{
		api.GET("", h.List)
		api.POST("", h.Create)
		api.GET("/:id", h.Get)
		api.GET("/:id/session", h.GetSession)
		api.PATCH("/:id", h.Update)
		api.DELETE("/:id", h.Delete)
	}
}

func ok(c *gin.Context, data interface{}) {
	httpx.OK(c, data)
}

func fail(c *gin.Context, status int, msg string) {
	httpx.Fail(c, status, msg)
}

// POST /api/video-projects
func (h *ProjectHandler) Create(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	var req model.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "invalid request: "+err.Error())
		return
	}
	project, err := h.svc.CreateProject(c.Request.Context(), userID, &req)
	if err != nil {
		fail(c, 400, err.Error())
		return
	}
	ok(c, gin.H{"project": project})
}

// GET /api/video-projects
func (h *ProjectHandler) List(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	modeFilter := c.Query("mode")
	statusFilter := c.Query("status")
	offset := 0
	limit := 20
	// Simple defaults for now; query param parsing can be added later
	projects, total, err := h.svc.ListProjects(c.Request.Context(), userID, modeFilter, statusFilter, offset, limit)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	if projects == nil {
		projects = []*model.VideoProject{}
	}
	ok(c, gin.H{"projects": projects, "total": total})
}

// GET /api/video-projects/:id
func (h *ProjectHandler) Get(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	project, err := h.svc.GetProject(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		fail(c, 404, "project not found")
		return
	}
	ok(c, gin.H{"project": project})
}

// PATCH /api/video-projects/:id
func (h *ProjectHandler) Update(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	var req model.UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "invalid request: "+err.Error())
		return
	}
	project, err := h.svc.UpdateProject(c.Request.Context(), userID, c.Param("id"), &req)
	if err != nil {
		fail(c, 400, err.Error())
		return
	}
	ok(c, gin.H{"project": project})
}

// DELETE /api/video-projects/:id (soft delete)
func (h *ProjectHandler) Delete(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	if err := h.svc.ArchiveProject(c.Request.Context(), userID, c.Param("id")); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"message": "archived"})
}

func authenticatedUserID(c *gin.Context) (string, bool) {
	userID, ok := auth.UserIDFromContext(c.Request.Context())
	if !ok {
		fail(c, 401, "unauthorized")
		return "", false
	}
	return userID, true
}

// GetSession returns a project session view that aggregates artifact status,
// review state, and runner readiness. This is the single source of truth for
// the frontend to restore project state after a page refresh.
//
// GET /api/video-projects/:projectId/session
func (h *ProjectHandler) GetSession(c *gin.Context) {
	projectID := c.Param("id")
	if projectID == "" {
		fail(c, http.StatusBadRequest, "project id is required")
		return
	}

	if h.sessionRouteHandler == nil {
		fail(c, http.StatusServiceUnavailable, "session endpoint not configured")
		return
	}

	session, err := h.sessionRouteHandler.BuildSession(c.Request.Context(), projectID)
	if err != nil {
		zap.L().Error("failed to build project session",
			zap.String("projectId", projectID),
			zap.Error(err),
		)
		fail(c, http.StatusInternalServerError, "failed to build session: "+err.Error())
		return
	}

	ok(c, session)
}

// ProjectSessionView is the aggregated project state for frontend recovery.
type ProjectSessionView struct {
	ProjectID          string             `json:"projectId"`
	CurrentStage       string             `json:"currentStage"`
	CurrentTask        string             `json:"currentTask"`
	PendingReview      *PendingReviewInfo `json:"pendingReview,omitempty"`
	ConfirmedArtifacts []ArtifactSummary  `json:"confirmedArtifacts"`
	StaleArtifacts     []ArtifactSummary  `json:"staleArtifacts"`
	NextActions        []string           `json:"nextActions"`
	LocalRunner        *LocalRunnerStatus `json:"localRunner,omitempty"`
}

// PendingReviewInfo describes an artifact awaiting human review.
type PendingReviewInfo struct {
	ReviewID   string `json:"reviewId"`
	Title      string `json:"title"`
	ArtifactID string `json:"artifactId"`
	Stage      string `json:"stage"`
}

// ArtifactSummary is a lightweight artifact view for the session display.
type ArtifactSummary struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Version int    `json:"version"`
	Status  string `json:"status"`
}

// LocalRunnerStatus describes runner availability for the session view.
type LocalRunnerStatus struct {
	Online         bool `json:"online"`
	SupportsRender bool `json:"supportsRender"`
}

// BuildSession aggregates project state from all available data sources.
func (h *SessionRouteHandler) BuildSession(ctx context.Context, projectID string) (*ProjectSessionView, error) {
	session := &ProjectSessionView{
		ProjectID: projectID,
	}

	// 1. Gather artifact data
	if h.artifactSvc != nil {
		artifacts, err := h.artifactSvc.ListCurrentByProject(ctx, projectID)
		if err != nil {
			zap.L().Warn("session: cannot list artifacts", zap.String("projectId", projectID), zap.Error(err))
		}

		// Track the last artifact with a pipeline stage that has content.
		var lastCompletedStage string
		pipelineOrder := []string{"proposal", "script", "storyboard", "composition", "reference", "continuity", "preview", "render", "quality", "package"}
		completedOrPending := map[string]string{} // stage → status

		for _, art := range artifacts {
			summary := ArtifactSummary{
				Kind:    art.StageName,
				Name:    art.Name,
				Version: art.Version,
				Status:  art.Status,
			}
			if art.Status == "stale" {
				session.StaleArtifacts = append(session.StaleArtifacts, summary)
			} else if art.Status == "valid" && art.HumanApproved {
				session.ConfirmedArtifacts = append(session.ConfirmedArtifacts, summary)
			}

			// Track where we are in the pipeline.
			if art.IsCurrent {
				completedOrPending[art.StageName] = art.Status
			}
		}

		// Determine the furthest completed stage.
		for _, stage := range pipelineOrder {
			if status, exists := completedOrPending[stage]; exists {
				if status == "valid" {
					lastCompletedStage = stage
				}
			}
		}

		// Find the first stage without a valid artifact as current.
		for _, stage := range pipelineOrder {
			status, exists := completedOrPending[stage]
			if !exists {
				session.CurrentStage = stage
				session.CurrentTask = stageActionName(stage)
				break
			}
			if status == "stale" || status == "pending" {
				session.CurrentStage = stage
				session.CurrentTask = stageActionName(stage)
				if status == "pending" {
					// Find the pending artifact for review info.
					for _, art := range artifacts {
						if art.StageName == stage && art.Status == "pending" {
							session.PendingReview = &PendingReviewInfo{
								ArtifactID: art.ID,
								Stage:      art.StageName,
								Title:      "审核" + stageDisplayName(art.StageName),
							}
							break
						}
					}
				}
				break
			}
		}

		// If all stages have valid artifacts, we're at or past render.
		if session.CurrentStage == "" {
			session.CurrentStage = lastCompletedStage
			session.CurrentTask = stageActionName(lastCompletedStage)
		}

		// 2. Generate smart nextActions based on pipeline state.
		session.NextActions = h.buildNextActions(ctx, projectID, completedOrPending)
	}

	// 3. Check local runner status
	if h.localRunnerSvc != nil {
		online, _ := h.localRunnerSvc.HasOnlineRunner(ctx, "")
		supportsRender, _ := h.localRunnerSvc.SupportsCommandForAnyUser(ctx, "HYPERFRAMES_RENDER")
		session.LocalRunner = &LocalRunnerStatus{
			Online:         online,
			SupportsRender: supportsRender,
		}
	}

	return session, nil
}

// buildNextActions generates human-readable next steps based on artifact pipeline state.
func (h *SessionRouteHandler) buildNextActions(ctx context.Context, projectID string, completed map[string]string) []string {
	actions := make([]string, 0)

	// Check render gate conditions.
	compositionOK := completed["composition"] == "valid"
	previewOK := completed["preview"] == "valid"

	// Check if we should recommend rendering.
	previewArt, _ := h.artifactSvc.FindCurrentByStageAndKind(ctx, projectID, "preview", "PREVIEW_SNAPSHOTS")
	previewApproved := previewArt != nil && previewArt.HumanApproved && previewArt.Status == "valid"

	if !compositionOK {
		actions = append(actions, "请先确认视频结构")
	} else if !previewOK {
		actions = append(actions, "请先生成预览快照")
	} else if !previewApproved {
		actions = append(actions, "确认预览后可开始渲染")
	} else {
		// Preview is confirmed — check render status.
		renderOK := completed["render"] == "valid"
		if !renderOK {
			if h.localRunnerSvc != nil {
				online, _ := h.localRunnerSvc.HasOnlineRunner(ctx, "")
				supportsRender, _ := h.localRunnerSvc.SupportsCommandForAnyUser(ctx, "HYPERFRAMES_RENDER")
				if !online || !supportsRender {
					actions = append(actions, "本地执行器未就绪，请启动本地后端后再渲染")
				} else {
					actions = append(actions, "可以开始渲染")
				}
			} else {
				actions = append(actions, "可以开始渲染")
			}
		} else {
			// Render is done — check quality gate.
			qualityOK := completed["quality"] == "valid"
			if !qualityOK {
				actions = append(actions, "正在执行质量检查...")
			} else {
				// Check FINAL_REVIEW passed.
				qualityArt, _ := h.artifactSvc.FindCurrentByStageAndKind(ctx, projectID, "quality", "FINAL_REVIEW")
				qualityPassed := false
				if qualityArt != nil && qualityArt.Metadata != nil {
					if passed, ok := qualityArt.Metadata["passed"].(bool); ok && passed {
						qualityPassed = true
					}
				}
				packageOK := completed["package"] == "valid"
				if !qualityPassed {
					actions = append(actions, "质量检查未通过，请检查并重新生成")
				} else if !packageOK {
					actions = append(actions, "质量检查已通过，可以打包导出")
				} else {
					actions = append(actions, "项目已完成，可以下载交付包")
				}
			}
		}
	}

	// If there are stale artifacts, warn about them.
	if h.artifactSvc != nil {
		staleArts, err := h.artifactSvc.ListCurrentByProject(ctx, projectID)
		if err == nil {
			hasStale := false
			for _, a := range staleArts {
				if a.Status == "stale" {
					hasStale = true
					break
				}
			}
			if hasStale {
				actions = append([]string{"部分上游产物已过期，需要重新生成"}, actions...)
			}
		}
	}

	return actions
}

// stageDisplayName returns the Chinese display name for a pipeline stage.
func stageDisplayName(stage string) string {
	names := map[string]string{
		"proposal":    "创意方案",
		"script":      "视频脚本",
		"storyboard":  "卡片分镜",
		"composition": "视频结构",
		"reference":   "素材计划",
		"continuity":  "一致性报告",
		"preview":     "预览快照",
		"render":      "最终视频",
		"quality":     "质量报告",
		"package":     "交付包",
	}
	if name, ok := names[stage]; ok {
		return name
	}
	return stage
}

// stageActionName returns the Chinese action name for a pipeline stage.
func stageActionName(stage string) string {
	actions := map[string]string{
		"proposal":    "定方向",
		"script":      "写脚本",
		"storyboard":  "拆画面",
		"composition": "排时间轴",
		"reference":   "定素材",
		"continuity":  "查一致",
		"preview":     "看预览",
		"render":      "出成片",
		"quality":     "做体检",
		"package":     "打包",
	}
	if action, ok := actions[stage]; ok {
		return action
	}
	return stage
}
