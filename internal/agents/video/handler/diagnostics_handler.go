package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/logger"
)

// DiagnosticsHandler exposes cloud-side observability endpoints for project
// state inspection and debugging.
type DiagnosticsHandler struct{}

// NewDiagnosticsHandler creates a cloud diagnostics handler.
func NewDiagnosticsHandler() *DiagnosticsHandler {
	return &DiagnosticsHandler{}
}

// RegisterRoutes registers the diagnostics endpoint under auth.
func (h *DiagnosticsHandler) RegisterRoutes(r *gin.Engine, auth gin.HandlerFunc) {
	r.GET("/api/video-projects/:id/diagnostics", auth, h.GetDiagnostics)
}

// GetDiagnostics returns a machine-readable snapshot of the video project
// state for debugging and support triage.  Currently returns the project ID
// and timestamp; extended snapshot data (shot state, QA reports, assembly
// plans) is available through the video project sub-resources (spec, shots,
// artifacts).
func (h *DiagnosticsHandler) GetDiagnostics(c *gin.Context) {
	projectID := c.Param("id")
	ctx := logger.InjectRequestIDIntoContext(c.Request.Context(), c)
	_ = ctx // reserved for future DB queries

	// Future: aggregrate shot, QA, candidate, and assembly state from
	// the video project service. For now, serve a light snapshot.
	snapshot := map[string]interface{}{
		"projectId":   projectID,
		"service":     "tangying-cloud-backend",
		"diagnostics": map[string]interface{}{},
	}
	zap.L().Info("cloud diagnostics snapshot served",
		zap.String("projectId", projectID),
		logger.RequestIDField(ctx),
	)

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": snapshot})
}
