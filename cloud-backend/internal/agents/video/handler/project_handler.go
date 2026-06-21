package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
	"github.com/tangying-ai/aios-core/internal/agents/video/service"
	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
)

// ProjectHandler serves HTTP endpoints for video project management.
type ProjectHandler struct {
	svc *service.ProjectService
}

func NewProjectHandler(svc *service.ProjectService) *ProjectHandler {
	return &ProjectHandler{svc: svc}
}

func (h *ProjectHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/video-projects")
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
	var req model.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "invalid request: "+err.Error())
		return
	}
	project, err := h.svc.CreateProject(c.Request.Context(), &req)
	if err != nil {
		fail(c, 400, err.Error())
		return
	}
	ok(c, gin.H{"project": project})
}

// GET /api/video-projects
func (h *ProjectHandler) List(c *gin.Context) {
	modeFilter := c.Query("mode")
	statusFilter := c.Query("status")
	offset := 0
	limit := 20
	// Simple defaults for now; query param parsing can be added later
	projects, total, err := h.svc.ListProjects(c.Request.Context(), modeFilter, statusFilter, offset, limit)
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
	project, err := h.svc.GetProject(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, 404, "project not found")
		return
	}
	ok(c, gin.H{"project": project})
}

// PATCH /api/video-projects/:id
func (h *ProjectHandler) Update(c *gin.Context) {
	var req model.UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "invalid request: "+err.Error())
		return
	}
	project, err := h.svc.UpdateProject(c.Request.Context(), c.Param("id"), &req)
	if err != nil {
		fail(c, 400, err.Error())
		return
	}
	ok(c, gin.H{"project": project})
}

// DELETE /api/video-projects/:id (soft delete)
func (h *ProjectHandler) Delete(c *gin.Context) {
	if err := h.svc.ArchiveProject(c.Request.Context(), c.Param("id")); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"message": "archived"})
}
