package tool

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ManifestHandler struct {
	service *ToolManifestService
}

func NewManifestHandler(service *ToolManifestService) *ManifestHandler {
	return &ManifestHandler{service: service}
}

func (h *ManifestHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/tools")
	api.GET("", h.List)
	api.POST("/register", h.Register)
	api.GET("/:name", h.Get)
	api.DELETE("/:name", h.Delete)
}

func (h *ManifestHandler) Register(c *gin.Context) {
	var manifest ToolManifest
	if err := c.ShouldBindJSON(&manifest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid tool manifest", "data": nil})
		return
	}
	if manifest.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "tool name is required", "data": nil})
		return
	}
	if err := h.service.RegisterExternal(c.Request.Context(), &manifest); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": gin.H{"tool": manifest.Name}})
}

func (h *ManifestHandler) List(c *gin.Context) {
	records, err := h.service.ListAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": gin.H{"tools": records}})
}

func (h *ManifestHandler) Get(c *gin.Context) {
	name := c.Param("name")
	record, err := h.service.repo.FindByName(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}
	if record == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "tool not found", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": gin.H{"tool": record}})
}

func (h *ManifestHandler) Delete(c *gin.Context) {
	name := c.Param("name")
	if err := h.service.DeregisterExternal(c.Request.Context(), name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": gin.H{"tool": name}})
}
