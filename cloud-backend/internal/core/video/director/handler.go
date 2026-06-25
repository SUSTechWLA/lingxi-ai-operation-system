package director

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
)

type Handler struct {
	registry *Registry
}

func NewHandler(registry *Registry) *Handler {
	return &Handler{registry: registry}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/video/role-agents")
	{
		api.GET("", h.List)
		api.GET("/:roleId", h.Get)
	}
}

func (h *Handler) List(c *gin.Context) {
	if h.registry == nil {
		httpx.OK(c, gin.H{"roleAgents": []interface{}{}})
		return
	}
	httpx.OK(c, gin.H{"roleAgents": h.registry.List()})
}

func (h *Handler) Get(c *gin.Context) {
	if h.registry == nil {
		httpx.Fail(c, http.StatusNotFound, "role agent not found")
		return
	}
	role := h.registry.GetByID(c.Param("roleId"))
	if role == nil {
		httpx.Fail(c, http.StatusNotFound, "role agent not found")
		return
	}
	httpx.OK(c, gin.H{"roleAgent": role.Agent()})
}
