package handler

import (
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/publish/service"
)

type PublishHandler struct {
	publishService *service.PublishService
}

func NewPublishHandler(publishService *service.PublishService) *PublishHandler {
	return &PublishHandler{publishService: publishService}
}

func (h *PublishHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.POST("/publish", h.PublishContent)
		api.POST("/ai/generate", h.AIGenerateContent)
		api.POST("/ai/generate-from-media", h.AIGenerateFromMedia)
		api.POST("/ai/polish", h.AIPolishText)
		api.POST("/ai/polish/submit", h.AIPolishSubmit)
		api.GET("/ai/polish/result", h.AIPolishQuery)
	}
}

func (h *PublishHandler) PublishContent(c *gin.Context) {
	if h.publishService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "service not available", "data": nil})
		return
	}

	// Parse multipart form (32 MB max memory, files go to temp dir)
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "failed to parse form: " + err.Error(), "data": nil})
		return
	}

	req := service.PublishRequest{
		Title:       c.Request.FormValue("title"),
		Description: c.Request.FormValue("description"),
		Keywords:    c.Request.FormValue("keywords"),
	}

	platformsStr := c.Request.FormValue("platforms")
	if platformsStr != "" {
		if err := json.Unmarshal([]byte(platformsStr), &req.Platforms); err != nil {
			req.Platforms = []string{platformsStr}
		}
	}

	if req.Title == "" || req.Description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "title and description are required", "data": nil})
		return
	}

	// Handle file uploads
	form := c.Request.MultipartForm
	if form != nil {
		for _, fileHeaders := range form.File {
			for _, fh := range fileHeaders {
				ext := strings.ToLower(filepath.Ext(fh.Filename))
				if ext == ".mp4" || ext == ".mov" || ext == ".avi" || ext == ".mkv" {
					req.VideoFiles = append(req.VideoFiles, fh)
				} else if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".gif" {
					req.ImageFiles = append(req.ImageFiles, fh)
				}
			}
		}
	}

	result, err := h.publishService.PublishContent(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": result})
}



func (h *PublishHandler) AIGenerateContent(c *gin.Context) {
	var request struct {
		Prompt string `json:"prompt" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error(), "data": nil})
		return
	}

	if h.publishService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "service not available", "data": nil})
		return
	}

	result, err := h.publishService.AIGenerateContent(c.Request.Context(), request.Prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": result})
}

func (h *PublishHandler) AIGenerateFromMedia(c *gin.Context) {
	if h.publishService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "service not available", "data": nil})
		return
	}

	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "failed to parse form: " + err.Error(), "data": nil})
		return
	}

	prompt := c.Request.FormValue("prompt")
	images := make([]*multipart.FileHeader, 0)
	videos := make([]*multipart.FileHeader, 0)

	form := c.Request.MultipartForm
	if form != nil {
		for field, fileHeaders := range form.File {
			for _, fh := range fileHeaders {
				ext := strings.ToLower(filepath.Ext(fh.Filename))
				if field == "images" || ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".gif" {
					images = append(images, fh)
				} else {
					videos = append(videos, fh)
				}
			}
		}
	}

	result, err := h.publishService.AIGenerateFromMedia(c.Request.Context(), prompt, images, videos)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": result})
}

func (h *PublishHandler) AIPolishText(c *gin.Context) {
	var request struct {
		Text string `json:"text" binding:"required"`
		Type string `json:"type"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error(), "data": nil})
		return
	}

	if request.Type == "" {
		request.Type = "description"
	}

	if h.publishService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "service not available", "data": nil})
		return
	}

	// Routes through Orchestrator → Worker → PolisherTool pipeline with polling
	content, taskID, err := h.publishService.AIPolishText(c.Request.Context(), request.Text, request.Type)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"content":  content,
			"taskId":   taskID,
			"traceUrl": fmt.Sprintf("/api/trace/%s", taskID),
		},
	})
}

func (h *PublishHandler) AIPolishSubmit(c *gin.Context) {
	var request struct {
		Text string `json:"text" binding:"required"`
		Type string `json:"type"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error(), "data": nil})
		return
	}

	if request.Type == "" {
		request.Type = "description"
	}

	if h.publishService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "service not available", "data": nil})
		return
	}

	result, err := h.publishService.AIPolishSubmit(c.Request.Context(), request.Text, request.Type)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": result})
}

func (h *PublishHandler) AIPolishQuery(c *gin.Context) {
	taskID := c.Query("taskId")
	nodeID := c.Query("nodeId")

	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "taskId is required", "data": nil})
		return
	}

	if h.publishService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "service not available", "data": nil})
		return
	}

	result, err := h.publishService.AIPolishQueryResult(c.Request.Context(), taskID, nodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": result})
}
