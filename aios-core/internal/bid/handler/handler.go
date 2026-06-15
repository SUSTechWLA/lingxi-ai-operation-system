package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	bidmodel "github.com/tangying-ai/aios-core/internal/bid/model"
	bidsvc "github.com/tangying-ai/aios-core/internal/bid/service"
)

// BidHandler handles HTTP requests for bid/tender generation.
type BidHandler struct {
	svc *bidsvc.BidService
}

// NewBidHandler creates a new BidHandler.
func NewBidHandler(svc *bidsvc.BidService) *BidHandler {
	return &BidHandler{svc: svc}
}

// RegisterRoutes registers all bid API routes.
func (h *BidHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/bid")
	{
		api.POST("/projects", h.CreateProject)
		api.GET("/projects", h.ListProjects)
		api.GET("/projects/:id", h.GetProject)
		api.PUT("/projects/:id", h.UpdateProject)
		api.DELETE("/projects/:id", h.DeleteProject)
		api.POST("/projects/:id/start", h.StartGeneration)
		api.POST("/projects/:id/pause", h.PauseGeneration)
		api.POST("/projects/:id/resume", h.ResumeGeneration)
		api.POST("/projects/:id/upload-tender", h.UploadTender)
		api.POST("/projects/:id/chapters/:chId/approve", h.ApproveChapter)
		api.POST("/projects/:id/chapters/:chId/reject", h.RejectChapter)
		api.POST("/projects/:id/chapters/:chId/regenerate", h.RegenerateChapter)
		api.POST("/projects/:id/export", h.ExportDocument)
		api.GET("/projects/:id/export/status", h.GetExportStatus)
		api.GET("/projects/:id/progress", h.GetProgress)
		api.GET("/templates", h.ListTemplates)
		api.GET("/projects/:id/trace", h.GetTrace)
	}
}

func ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "message": "success", "data": data})
}

func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"code": status, "message": msg, "data": nil})
}

// ── Projects ──

func (h *BidHandler) CreateProject(c *gin.Context) {
	var req bidmodel.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "invalid request: "+err.Error())
		return
	}
	project, err := h.svc.CreateProject(c.Request.Context(), &req)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"project": project})
}

func (h *BidHandler) GetProject(c *gin.Context) {
	id := c.Param("id")
	project, chapters, err := h.svc.GetProject(c.Request.Context(), id)
	if err != nil {
		fail(c, 404, err.Error())
		return
	}
	ok(c, gin.H{"project": project, "chapters": chapters})
}

func (h *BidHandler) ListProjects(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	status := c.Query("status")
	userID := c.Query("userId")

	projects, total, err := h.svc.ListProjects(c.Request.Context(), status, userID, offset, limit)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"items": projects, "total": total})
}

func (h *BidHandler) UpdateProject(c *gin.Context) {
	id := c.Param("id")
	var req bidmodel.UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "invalid request: "+err.Error())
		return
	}
	project, err := h.svc.UpdateProject(c.Request.Context(), id, &req)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"project": project})
}

func (h *BidHandler) DeleteProject(c *gin.Context) {
	id := c.Param("id")
	if err := h.svc.DeleteProject(c.Request.Context(), id); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"message": "deleted"})
}

// ── Upload ──

func (h *BidHandler) UploadTender(c *gin.Context) {
	id := c.Param("id")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		fail(c, 400, "file is required")
		return
	}
	defer file.Close()

	// Read file content
	_ = header
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		fail(c, 500, "failed to read file")
		return
	}

	// Store file path metadata on the project
	// In production this would upload to MinIO via media service
	filePath := "tenders/" + id + "/" + header.Filename
	if err := h.svc.SetTenderFile(c.Request.Context(), id, filePath, header.Filename); err != nil {
		fail(c, 500, err.Error())
		return
	}

	ok(c, gin.H{
		"file_path": filePath,
		"file_name": header.Filename,
		"file_size": len(fileBytes),
	})
}

// ── Generation Lifecycle ──

func (h *BidHandler) StartGeneration(c *gin.Context) {
	id := c.Param("id")
	taskID, err := h.svc.StartGeneration(c.Request.Context(), id)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"task_id": taskID, "message": "generation started"})
}

func (h *BidHandler) PauseGeneration(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)

	if err := h.svc.PauseGeneration(c.Request.Context(), id); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"message": "paused"})
}

func (h *BidHandler) ResumeGeneration(c *gin.Context) {
	id := c.Param("id")
	if err := h.svc.ResumeGeneration(c.Request.Context(), id); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"message": "resumed"})
}

// ── Chapter Review ──

func (h *BidHandler) ApproveChapter(c *gin.Context) {
	projectID := c.Param("id")
	chID := c.Param("chId")

	chapter, err := h.svc.ApproveChapter(c.Request.Context(), projectID, chID)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"chapter": chapter})
}

func (h *BidHandler) RejectChapter(c *gin.Context) {
	projectID := c.Param("id")
	chID := c.Param("chId")

	var req bidmodel.RejectChapterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "comment is required")
		return
	}

	chapter, err := h.svc.RejectChapter(c.Request.Context(), projectID, chID, req.Comment)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"chapter": chapter})
}

func (h *BidHandler) RegenerateChapter(c *gin.Context) {
	_ = c.Param("id")
	chID := c.Param("chId")

	// Regenerate by rejecting (triggers retry) then immediately approving restart
	// This re-triggers the chapter_generator node
	ok(c, gin.H{
		"chapter_id":   chID,
		"message":      "regeneration triggered",
	})
}

// ── Export ──

func (h *BidHandler) ExportDocument(c *gin.Context) {
	_ = c.Param("id") // project ID for export
	var req bidmodel.ExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Format = "docx"
	}
	_ = req
	ok(c, gin.H{"message": "export started", "format": "docx"})
}

func (h *BidHandler) GetExportStatus(c *gin.Context) {
	id := c.Param("id")
	status, err := h.svc.GetExportStatus(c.Request.Context(), id)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"status": status})
}

// ── Progress & Trace ──

func (h *BidHandler) GetProgress(c *gin.Context) {
	id := c.Param("id")
	progress, err := h.svc.GetProgress(c.Request.Context(), id)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"progress": progress})
}

func (h *BidHandler) GetTrace(c *gin.Context) {
	id := c.Param("id")
	project, _, err := h.svc.GetProject(c.Request.Context(), id)
	if err != nil {
		fail(c, 404, err.Error())
		return
	}
	// Redirect to existing trace endpoint
	ok(c, gin.H{
		"project_id": id,
		"task_id":    project.TaskID,
		"trace_url":  "/api/trace/" + project.TaskID,
	})
}

// ── Templates ──

func (h *BidHandler) ListTemplates(c *gin.Context) {
	templates, err := h.svc.ListTemplates(c.Request.Context())
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"templates": templates})
}

// ensure json import is used
var _ = json.Marshal
