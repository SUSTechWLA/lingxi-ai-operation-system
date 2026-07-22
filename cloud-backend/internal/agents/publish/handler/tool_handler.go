package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

type ToolHandler struct {
	registry                  *tool.ToolRegistry
	manifestSvc               toolManifestWriter
	registrationInternalToken string
}

type toolManifestWriter interface {
	RegisterExternal(ctx context.Context, manifest *tool.ToolManifest) error
	DeregisterExternal(ctx context.Context, name string) error
}

func NewToolHandler(registry *tool.ToolRegistry, manifestSvc toolManifestWriter) *ToolHandler {
	return &ToolHandler{registry: registry, manifestSvc: manifestSvc}
}

func (h *ToolHandler) WithRegistrationInternalToken(token string) *ToolHandler {
	h.registrationInternalToken = token
	return h
}

func (h *ToolHandler) RegisterRoutes(r *gin.Engine, middleware ...gin.HandlerFunc) {
	api := r.Group("/api/tools", middleware...)
	{
		api.GET("", h.ListTools)
		api.GET("/:name", h.GetTool)
		api.POST("/register", h.RegisterTool)
		api.DELETE("/:name", h.DeregisterTool)
	}
}

// ListTools returns all registered tools (built-in + external) with their full manifests.
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

// RegisterTool registers an external tool manifest and persists it to the database.
func (h *ToolHandler) RegisterTool(c *gin.Context) {
	if !h.authorizeGlobalMutation(c) {
		return
	}
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

	// Persist to DB + in-memory registry + invalidate cache
	if err := h.manifestSvc.RegisterExternal(c.Request.Context(), &manifest); err != nil {
		if errors.Is(err, tool.ErrInputSchemaInvalid) || errors.Is(err, tool.ErrOutputSchemaInvalid) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "invalid tool contract: " + err.Error(),
				"data":    nil,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "failed to register tool: " + err.Error(),
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "tool registered successfully",
		"data": gin.H{
			"name": manifest.Name,
			"type": manifest.Type,
		},
	})
}

// DeregisterTool removes an external tool registration from DB and registry.
func (h *ToolHandler) DeregisterTool(c *gin.Context) {
	if !h.authorizeGlobalMutation(c) {
		return
	}
	name := c.Param("name")
	if err := h.manifestSvc.DeregisterExternal(c.Request.Context(), name); err != nil {
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

func (h *ToolHandler) authorizeGlobalMutation(c *gin.Context) bool {
	expected := h.registrationInternalToken
	provided := c.GetHeader("X-Internal-Tool-Token")
	if strings.TrimSpace(expected) == "" || len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    http.StatusForbidden,
			"message": "global tool registration is restricted to the internal control plane",
			"data":    nil,
		})
		return false
	}
	return true
}
