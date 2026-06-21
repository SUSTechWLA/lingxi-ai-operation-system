package media

import (
	"context"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/auth"
)

type serviceAPI interface {
	Upload(ctx context.Context, userID string, files []*multipart.FileHeader) ([]*MediaAsset, error)
	List(ctx context.Context, userID string, offset, limit int, tag string) ([]*MediaAsset, int, error)
	GetForUser(ctx context.Context, userID string, id string) (*MediaAsset, error)
	UpdateTagsForUser(ctx context.Context, userID string, id string, tags []string) error
}

type MediaHandler struct {
	service    serviceAPI
	middleware []gin.HandlerFunc
}

func NewMediaHandler(service serviceAPI, middleware ...gin.HandlerFunc) *MediaHandler {
	return &MediaHandler{service: service, middleware: middleware}
}

func (h *MediaHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/media", h.middleware...)
	{
		api.POST("/upload", h.Upload)
		api.GET("/list", h.List)
		api.GET("/:id", h.Get)
		api.PUT("/:id/tags", h.UpdateTags)
	}
}

func (h *MediaHandler) Upload(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(128 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "failed to parse form: " + err.Error(), "data": nil})
		return
	}

	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	var files []*multipart.FileHeader
	form := c.Request.MultipartForm
	if form != nil {
		for _, fhs := range form.File {
			files = append(files, fhs...)
		}
	}

	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "no files uploaded", "data": nil})
		return
	}

	assets, err := h.service.Upload(c.Request.Context(), userID, files)
	if err != nil {
		zap.L().Error("media upload failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": assets})
}

func (h *MediaHandler) List(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	tag := c.Query("tag")

	assets, total, err := h.service.List(c.Request.Context(), userID, offset, limit, tag)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"items": assets,
			"total": total,
		},
	})
}

func (h *MediaHandler) Get(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	id := c.Param("id")
	asset, err := h.service.GetForUser(c.Request.Context(), userID, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": err.Error(), "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": asset})
}

func (h *MediaHandler) UpdateTags(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	id := c.Param("id")
	var req struct {
		Tags []string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error(), "data": nil})
		return
	}

	if err := h.service.UpdateTagsForUser(c.Request.Context(), userID, id, req.Tags); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": nil})
}

func authenticatedUserID(c *gin.Context) (string, bool) {
	userID, ok := auth.UserIDFromContext(c.Request.Context())
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized", "data": nil})
		return "", false
	}
	return userID, true
}
