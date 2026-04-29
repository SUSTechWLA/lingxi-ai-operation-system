package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

type ToolHandler struct {
	registry *tool.ToolRegistry
}

func NewToolHandler(registry *tool.ToolRegistry) *ToolHandler {
	return &ToolHandler{registry: registry}
}

func (h *ToolHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/tools")
	{
		api.GET("", h.ListTools)
		api.GET("/:name", h.GetTool)
		api.POST("/register", h.RegisterTool)
		api.DELETE("/:name", h.DeregisterTool)
	}
}

// ListTools returns all registered tools (built-in + external) with their full manifests.
// This is the knowledge base that AI assistants and developers query to understand
// available tools and their capabilities.
func (h *ToolHandler) ListTools(c *gin.Context) {
	manifests := h.registry.ListManifests()
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data":    manifests,
	})
}

// GetTool returns the manifest for a specific tool by name.
func (h *ToolHandler) GetTool(c *gin.Context) {
	name := c.Param("name")
	manifest := h.registry.GetManifest(name)
	if manifest == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "tool not found: " + name,
			"data":    nil,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data":    manifest,
	})
}

// RegisterTool registers an external tool manifest.
// External tools must provide endpoint, type, parameter definitions, and output definitions.
func (h *ToolHandler) RegisterTool(c *gin.Context) {
	var manifest tool.ToolManifest
	if err := c.ShouldBindJSON(&manifest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid manifest: " + err.Error(),
			"data":    nil,
		})
		return
	}

	if manifest.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "tool name is required",
			"data":    nil,
		})
		return
	}

	if manifest.Endpoint == "" && manifest.Type != "builtin" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "endpoint is required for external tools",
			"data":    nil,
		})
		return
	}

	h.registry.RegisterExternal(&manifest)

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "tool registered successfully",
		"data": gin.H{
			"name": manifest.Name,
			"type": manifest.Type,
		},
	})
}

// DeregisterTool removes an external tool registration.
func (h *ToolHandler) DeregisterTool(c *gin.Context) {
	name := c.Param("name")
	if !h.registry.DeregisterExternal(name) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "external tool not found: " + name,
			"data":    nil,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "tool deregistered successfully",
		"data":    nil,
	})
}
