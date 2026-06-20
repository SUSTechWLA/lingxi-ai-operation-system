package artifact

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/model"
	modelRepo "github.com/tangying-ai/aios-core/internal/core/model/repository"
	"github.com/tangying-ai/aios-core/internal/core/workflow"
)

type Handler struct {
	service  *Service
	runRepo  *workflow.RunRepository
	nodeRepo modelRepo.NodeRepo
}

func NewHandler(service *Service, runRepo *workflow.RunRepository, nodeRepo modelRepo.NodeRepo) *Handler {
	return &Handler{service: service, runRepo: runRepo, nodeRepo: nodeRepo}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.GET("/video-projects/:id/artifacts", h.ListProjectArtifacts)
		api.GET("/artifacts/:id", h.GetArtifact)
		api.GET("/artifacts/:id/content", h.GetArtifactContent)
		api.GET("/artifacts/:id/history", h.GetArtifactHistory)
		api.POST("/artifacts/:id/revise", h.ReviseArtifact)
	}
}

func (h *Handler) ListProjectArtifacts(c *gin.Context) {
	projectID := c.Param("id")
	if err := h.materializeProject(c.Request.Context(), projectID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}
	artifacts, err := h.service.ListByProject(c.Request.Context(), projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}
	if artifacts == nil {
		artifacts = []*Artifact{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": gin.H{"artifacts": artifacts}})
}

func (h *Handler) GetArtifact(c *gin.Context) {
	artifact, err := h.service.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "artifact not found", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": gin.H{"artifact": artifact}})
}

func (h *Handler) GetArtifactContent(c *gin.Context) {
	artifact, err := h.service.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "artifact not found", "data": nil})
		return
	}
	content, mediaURL, mediaURLs := artifactContent(artifact)
	mediaURLs = normalizeMediaURLs(mediaURLs)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": gin.H{
		"artifact":  artifact,
		"content":   content,
		"mediaUrl":  mediaURL,
		"mediaUrls": mediaURLs,
	}})
}

func (h *Handler) GetArtifactHistory(c *gin.Context) {
	artifact, err := h.service.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "artifact not found", "data": nil})
		return
	}
	history, err := h.service.GetHistory(c.Request.Context(), artifact.ProjectID, artifact.StageName, artifact.UnitID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": gin.H{"history": history}})
}

func (h *Handler) ReviseArtifact(c *gin.Context) {
	var req struct {
		Message string `json:"message" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request: " + err.Error(), "data": nil})
		return
	}
	base, err := h.service.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "artifact not found", "data": nil})
		return
	}

	data := buildLocalRevisionData(base, req.Message)
	revision, err := h.service.CreateArtifact(c.Request.Context(), BuildRevisionRequest(base, req.Message, data))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}
	content, mediaURL, mediaURLs := artifactContent(revision)
	mediaURLs = normalizeMediaURLs(mediaURLs)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": gin.H{
		"artifact":  revision,
		"content":   content,
		"mediaUrl":  mediaURL,
		"mediaUrls": mediaURLs,
	}})
}

func (h *Handler) materializeProject(ctx context.Context, projectID string) error {
	runs, err := h.runRepo.FindByProject(ctx, projectID)
	if err != nil {
		return err
	}
	for _, run := range runs {
		nodes, err := h.nodeRepo.FindByTaskID(ctx, run.TaskID)
		if err != nil {
			return err
		}
		for _, node := range nodes {
			if node.Status != model.NodeSuccess {
				continue
			}
			for _, req := range BuildArtifactRequestsFromNode(projectID, run.ID, node) {
				if _, err := h.service.CreateArtifact(ctx, req); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func artifactContent(artifact *Artifact) (interface{}, string, []string) {
	if artifact.StorageType == "minio" || artifact.StorageType == "url" {
		return nil, artifact.StorageRef, mediaURLsFromString(artifact.StorageRef)
	}
	raw := artifact.InlineJSON
	if artifact.Kind == KindMarkdown || strings.HasPrefix(artifact.MimeType, "text/") {
		mediaURLs := mediaURLsFromString(raw)
		return raw, firstMediaURL(mediaURLs), mediaURLs
	}
	var parsed interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		mediaURLs := mediaURLsFromValue(parsed)
		return parsed, firstMediaURL(mediaURLs), mediaURLs
	}
	mediaURLs := mediaURLsFromString(raw)
	return raw, firstMediaURL(mediaURLs), mediaURLs
}

func mediaURLsFromValue(value interface{}) []string {
	seen := map[string]bool{}
	return mediaURLsFromValueSeen(value, seen)
}

func mediaURLsFromValueSeen(value interface{}, seen map[string]bool) []string {
	switch typed := value.(type) {
	case map[string]interface{}:
		urls := make([]string, 0)
		mediaKeys := []string{
			"url", "mediaUrl", "dataUrl", "imageUrl", "videoUrl", "audioUrl", "coverUrl", "posterUrl",
			"thumbnailUrl", "previewUrl", "downloadUrl", "src",
		}
		traversalKeys := []string{
			"imageRequests", "referenceFrames", "frames", "keyframes", "images",
			"clips", "videoClips", "videos", "videoImportPackage",
			"audioTracks", "audioPackage", "audio", "assets", "files", "items", "outputs",
		}
		visitedKeys := map[string]bool{}
		for _, key := range mediaKeys {
			visitedKeys[key] = true
			if url, ok := typed[key].(string); ok {
				urls = appendMediaURL(urls, url, seen)
			}
		}
		for _, key := range traversalKeys {
			visitedKeys[key] = true
			urls = append(urls, mediaURLsFromValueSeen(typed[key], seen)...)
		}
		rest := make([]string, 0, len(typed))
		for key := range typed {
			if !visitedKeys[key] {
				rest = append(rest, key)
			}
		}
		sort.Strings(rest)
		for _, key := range rest {
			urls = append(urls, mediaURLsFromValueSeen(typed[key], seen)...)
		}
		return urls
	case []interface{}:
		urls := make([]string, 0)
		for _, item := range typed {
			urls = append(urls, mediaURLsFromValueSeen(item, seen)...)
		}
		return urls
	case string:
		return mediaURLsFromStringSeen(typed, seen)
	}
	return nil
}

func mediaURLsFromString(value string) []string {
	return mediaURLsFromStringSeen(value, map[string]bool{})
}

func mediaURLsFromStringSeen(value string, seen map[string]bool) []string {
	trimmed := strings.TrimSpace(value)
	if isMediaURL(trimmed) {
		return appendMediaURL(nil, trimmed, seen)
	}
	return nil
}

func appendMediaURL(urls []string, value string, seen map[string]bool) []string {
	trimmed := strings.TrimSpace(value)
	if !isMediaURL(trimmed) || seen[trimmed] {
		return urls
	}
	seen[trimmed] = true
	return append(urls, trimmed)
}

func firstMediaURL(urls []string) string {
	if len(urls) == 0 {
		return ""
	}
	return urls[0]
}

func normalizeMediaURLs(urls []string) []string {
	if urls == nil {
		return []string{}
	}
	return urls
}

func isMediaURL(value string) bool {
	return strings.HasPrefix(value, "http://") ||
		strings.HasPrefix(value, "https://") ||
		strings.HasPrefix(value, "data:") ||
		strings.HasPrefix(value, "blob:") ||
		strings.HasPrefix(value, "/")
}

func buildLocalRevisionData(base *Artifact, instruction string) []byte {
	content, _, _ := artifactContent(base)
	switch base.Kind {
	case KindMarkdown:
		return []byte("## 返工版本\n\n返工要求：" + instruction + "\n\n" + stringifyContent(content))
	case KindJSON, KindImage, KindVideo, KindAudio, KindBundle:
		payload := map[string]interface{}{
			"revisionInstruction": instruction,
			"previous":            content,
			"status":              "regenerated",
		}
		data, _ := json.MarshalIndent(payload, "", "  ")
		return data
	default:
		return []byte("返工要求：" + instruction + "\n\n" + stringifyContent(content))
	}
}

func stringifyContent(content interface{}) string {
	switch typed := content.(type) {
	case string:
		return typed
	default:
		data, _ := json.MarshalIndent(typed, "", "  ")
		return string(data)
	}
}
