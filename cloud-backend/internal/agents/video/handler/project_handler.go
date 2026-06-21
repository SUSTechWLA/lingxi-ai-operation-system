package handler

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
	"github.com/tangying-ai/aios-core/internal/core/auth"
	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
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
	svc        projectService
	middleware []gin.HandlerFunc
}

func NewProjectHandler(svc projectService, middleware ...gin.HandlerFunc) *ProjectHandler {
	return &ProjectHandler{svc: svc, middleware: middleware}
}

func (h *ProjectHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/video-projects", h.middleware...)
	{
		api.GET("", h.List)
		api.POST("", h.Create)
		api.GET("/:id", h.Get)
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
