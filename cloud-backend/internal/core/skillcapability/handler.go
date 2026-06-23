package skillcapability

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
	api := r.Group("/api/skill-capabilities")
	{
		api.GET("", h.List)
		api.GET("/:id", h.Get)
	}
}

func (h *Handler) List(c *gin.Context) {
	caps := []*Manifest{}
	if h.registry != nil {
		caps = h.registry.List()
	}
	httpx.OK(c, gin.H{"capabilities": caps})
}

func (h *Handler) Get(c *gin.Context) {
	if h.registry == nil {
		httpx.Fail(c, http.StatusNotFound, "skill capability not found")
		return
	}
	id := c.Param("id")
	for _, cap := range h.registry.List() {
		if cap.ID == id {
			httpx.OK(c, gin.H{"capability": cap})
			return
		}
	}
	httpx.Fail(c, http.StatusNotFound, "skill capability not found")
}
