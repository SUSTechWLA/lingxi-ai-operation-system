package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
)

type creatorViewProjectReader interface {
	GetProject(ctx context.Context, userID, projectID string) (*model.VideoProject, error)
}

type creatorViewReader interface {
	GetCreationView(ctx context.Context, userID, projectID string) (*model.CreationView, error)
}

// CreatorViewHandler serves the creator-facing aggregate without exposing
// internal workflow stages or requiring the client to calculate progress.
type CreatorViewHandler struct {
	projects   creatorViewProjectReader
	views      creatorViewReader
	middleware []gin.HandlerFunc
}

func NewCreatorViewHandler(projects creatorViewProjectReader, views creatorViewReader, middleware ...gin.HandlerFunc) *CreatorViewHandler {
	return &CreatorViewHandler{projects: projects, views: views, middleware: middleware}
}

func (h *CreatorViewHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/video-projects", h.middleware...)
	api.GET("/:id/creation-view", h.GetCreationView)
}

// GetCreationView authenticates and verifies ownership before reading the
// aggregate. Project lookup failures deliberately use the project endpoint's
// 404 behavior so a caller cannot distinguish a forbidden project from one
// that does not exist.
func (h *CreatorViewHandler) GetCreationView(c *gin.Context) {
	userID, okAuth := authenticatedUserID(c)
	if !okAuth {
		return
	}
	projectID := c.Param("id")
	project, err := h.projects.GetProject(c.Request.Context(), userID, projectID)
	if err != nil || project == nil {
		fail(c, http.StatusNotFound, "project not found")
		return
	}
	view, err := h.views.GetCreationView(c.Request.Context(), userID, projectID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load creation view")
		return
	}
	if view == nil {
		fail(c, http.StatusInternalServerError, "failed to load creation view")
		return
	}
	httpx.OK(c, view)
}
