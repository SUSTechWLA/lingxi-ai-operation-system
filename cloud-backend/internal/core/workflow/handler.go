package workflow

import (
	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
)

// Handler serves HTTP endpoints for workflow templates.
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(r *gin.Engine, middleware ...gin.HandlerFunc) {
	api := r.Group("/api/workflows", middleware...)
	{
		api.GET("", h.List)
		api.GET("/:id", h.Get)
		api.POST("", h.Create)
		api.PUT("/:id", h.Update)
		api.DELETE("/:id", h.Delete)
		api.POST("/:id/instantiate", h.Instantiate)
	}
}

func ok(c *gin.Context, data interface{}) {
	httpx.OK(c, data)
}

func fail(c *gin.Context, status int, msg string) {
	httpx.Fail(c, status, msg)
}

// GET /api/workflows
func (h *Handler) List(c *gin.Context) {
	templates, err := h.svc.List(c.Request.Context())
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	if templates == nil {
		templates = []*Template{}
	}
	ok(c, gin.H{"templates": templates})
}

// GET /api/workflows/:id
func (h *Handler) Get(c *gin.Context) {
	t, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, 404, "template not found")
		return
	}
	ok(c, gin.H{"template": t})
}

// POST /api/workflows
func (h *Handler) Create(c *gin.Context) {
	var req CreateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "invalid request: "+err.Error())
		return
	}
	t, err := h.svc.Create(c.Request.Context(), &req)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"template": t})
}

// PUT /api/workflows/:id
func (h *Handler) Update(c *gin.Context) {
	var req UpdateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "invalid request: "+err.Error())
		return
	}
	t, err := h.svc.Update(c.Request.Context(), c.Param("id"), &req)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"template": t})
}

// DELETE /api/workflows/:id
func (h *Handler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"message": "deleted"})
}

// POST /api/workflows/:id/instantiate
func (h *Handler) Instantiate(c *gin.Context) {
	var req InstantiateRequest
	_ = c.ShouldBindJSON(&req)
	taskID, err := h.svc.Instantiate(c.Request.Context(), c.Param("id"), req.Overrides)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{
		"task_id":   taskID,
		"message":   "workflow instantiated, task created",
		"trace_url": "/api/trace/" + taskID,
	})
}
