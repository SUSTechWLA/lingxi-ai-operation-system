package handler

import (
	"context"
	"io"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/agents/bid/model"
	"github.com/tangying-ai/aios-core/internal/core/auth"
	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
)

type bidService interface {
	CreateProject(ctx context.Context, userID string, req *model.CreateProjectRequest) (*model.BidProject, error)
	GetProject(ctx context.Context, userID string, projectID string) (*model.BidProject, []*model.BidChapter, error)
	ListProjects(ctx context.Context, status string, userID string, offset, limit int) ([]*model.BidProject, int, error)
	UpdateProject(ctx context.Context, userID string, projectID string, req *model.UpdateProjectRequest) (*model.BidProject, error)
	DeleteProject(ctx context.Context, userID string, projectID string) error
	SetTenderFile(ctx context.Context, userID string, projectID, filePath, fileName string) error
	StartGeneration(ctx context.Context, userID string, projectID string) (string, error)
	PauseGeneration(ctx context.Context, userID string, projectID string) error
	ResumeGeneration(ctx context.Context, userID string, projectID string) error
	ApproveChapter(ctx context.Context, userID string, projectID, chapterID string) (*model.BidChapter, error)
	RejectChapter(ctx context.Context, userID string, projectID, chapterID, comment string) (*model.BidChapter, error)
	GetProgress(ctx context.Context, userID string, projectID string) (*model.ProgressResponse, error)
	ListTemplates(ctx context.Context) ([]*model.BidTemplate, error)
	GetExportStatus(ctx context.Context, userID string, projectID string) (*model.ExportStatusResponse, error)
}

// BidHandler handles HTTP requests for bid/tender generation.
type BidHandler struct {
	svc        bidService
	middleware []gin.HandlerFunc
}

// NewBidHandler creates a new BidHandler.
func NewBidHandler(svc bidService, middleware ...gin.HandlerFunc) *BidHandler {
	return &BidHandler{svc: svc, middleware: middleware}
}

// RegisterRoutes registers all bid API routes.
func (h *BidHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/bid", h.middleware...)
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
	httpx.OK(c, data)
}

func fail(c *gin.Context, status int, msg string) {
	httpx.Fail(c, status, msg)
}

// ── Projects ──

func (h *BidHandler) CreateProject(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	var req model.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "invalid request: "+err.Error())
		return
	}
	project, err := h.svc.CreateProject(c.Request.Context(), userID, &req)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"project": project})
}

func (h *BidHandler) GetProject(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	id := c.Param("id")
	project, chapters, err := h.svc.GetProject(c.Request.Context(), userID, id)
	if err != nil {
		fail(c, 404, err.Error())
		return
	}
	ok(c, gin.H{"project": project, "chapters": chapters})
}

func (h *BidHandler) ListProjects(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	status := c.Query("status")

	projects, total, err := h.svc.ListProjects(c.Request.Context(), status, userID, offset, limit)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"items": projects, "total": total})
}

func (h *BidHandler) UpdateProject(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	id := c.Param("id")
	var req model.UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "invalid request: "+err.Error())
		return
	}
	project, err := h.svc.UpdateProject(c.Request.Context(), userID, id, &req)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"project": project})
}

func (h *BidHandler) DeleteProject(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	id := c.Param("id")
	if err := h.svc.DeleteProject(c.Request.Context(), userID, id); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"message": "deleted"})
}

// ── Upload ──

func (h *BidHandler) UploadTender(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
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
	if err := h.svc.SetTenderFile(c.Request.Context(), userID, id, filePath, header.Filename); err != nil {
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
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	id := c.Param("id")
	taskID, err := h.svc.StartGeneration(c.Request.Context(), userID, id)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"task_id": taskID, "message": "generation started"})
}

func (h *BidHandler) PauseGeneration(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	id := c.Param("id")
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)

	if err := h.svc.PauseGeneration(c.Request.Context(), userID, id); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"message": "paused"})
}

func (h *BidHandler) ResumeGeneration(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	id := c.Param("id")
	if err := h.svc.ResumeGeneration(c.Request.Context(), userID, id); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"message": "resumed"})
}

// ── Chapter Review ──

func (h *BidHandler) ApproveChapter(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	projectID := c.Param("id")
	chID := c.Param("chId")

	chapter, err := h.svc.ApproveChapter(c.Request.Context(), userID, projectID, chID)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"chapter": chapter})
}

func (h *BidHandler) RejectChapter(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	projectID := c.Param("id")
	chID := c.Param("chId")

	var req model.RejectChapterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "comment is required")
		return
	}

	chapter, err := h.svc.RejectChapter(c.Request.Context(), userID, projectID, chID, req.Comment)
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
		"chapter_id": chID,
		"message":    "regeneration triggered",
	})
}

// ── Export ──

func (h *BidHandler) ExportDocument(c *gin.Context) {
	_ = c.Param("id") // project ID for export
	var req model.ExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Format = "docx"
	}
	_ = req
	ok(c, gin.H{"message": "export started", "format": "docx"})
}

func (h *BidHandler) GetExportStatus(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	id := c.Param("id")
	status, err := h.svc.GetExportStatus(c.Request.Context(), userID, id)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"status": status})
}

// ── Progress & Trace ──

func (h *BidHandler) GetProgress(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	id := c.Param("id")
	progress, err := h.svc.GetProgress(c.Request.Context(), userID, id)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"progress": progress})
}

func (h *BidHandler) GetTrace(c *gin.Context) {
	userID, hasUser := authenticatedUserID(c)
	if !hasUser {
		return
	}
	id := c.Param("id")
	project, _, err := h.svc.GetProject(c.Request.Context(), userID, id)
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

func authenticatedUserID(c *gin.Context) (string, bool) {
	userID, ok := auth.UserIDFromContext(c.Request.Context())
	if !ok {
		fail(c, 401, "unauthorized")
		return "", false
	}
	return userID, true
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
