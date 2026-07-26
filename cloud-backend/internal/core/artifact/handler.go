package artifact

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tangying-ai/aios-core/internal/core/auth"
	"github.com/tangying-ai/aios-core/internal/core/model"
	modelRepo "github.com/tangying-ai/aios-core/internal/core/model/repository"
	"github.com/tangying-ai/aios-core/internal/core/workflow"
)

// ReviseLLMFunc is retained for callers configuring the legacy HTTP endpoint.
type ReviseLLMFunc = RevisionGenerator

type ReviseLLMOptions struct {
	ModelProvider map[string]interface{}
}

type Handler struct {
	service    handlerArtifactStore
	runRepo    *workflow.RunRepository
	nodeRepo   modelRepo.NodeRepo
	agentTasks AgentTaskStore
	access     ProjectAccessChecker

	revisions *RevisionService
}

type handlerArtifactStore interface {
	revisionArtifactStore
	GetHistory(context.Context, string, string, string) ([]*Artifact, error)
	ListByProject(context.Context, string) ([]*Artifact, error)
	ListAllVersionsByProject(context.Context, string) ([]*Artifact, error)
}

// ProjectAccessChecker resolves project ownership without coupling the core
// artifact package to the video-project model package.
type ProjectAccessChecker interface {
	CanAccessProject(context.Context, string, string) bool
}

type ProjectAccessFunc func(context.Context, string, string) bool

func (f ProjectAccessFunc) CanAccessProject(ctx context.Context, userID, projectID string) bool {
	return f != nil && f(ctx, userID, projectID)
}

type AgentTaskStore interface {
	FindAgentRuntimeTasksByProject(ctx context.Context, projectID string) ([]*model.Task, error)
}

type taskNodeFinder interface {
	FindByTaskID(ctx context.Context, taskID string) ([]*model.Node, error)
}

func NewHandler(service *Service, runRepo *workflow.RunRepository, nodeRepo modelRepo.NodeRepo) *Handler {
	return newHandlerForStore(service, runRepo, nodeRepo)
}

func newHandlerForStore(service handlerArtifactStore, runRepo *workflow.RunRepository, nodeRepo modelRepo.NodeRepo) *Handler {
	handler := &Handler{service: service, runRepo: runRepo, nodeRepo: nodeRepo}
	handler.revisions = NewRevisionService(service)
	handler.revisions.SetContentResolver(handler.hydrateLocalTextArtifactContent)
	return handler
}

func (h *Handler) WithAgentTaskStore(store AgentTaskStore) *Handler {
	h.agentTasks = store
	return h
}

func (h *Handler) WithProjectAccess(access ProjectAccessChecker) *Handler {
	h.access = access
	return h
}

// SetRevisionConfig wires the skill root path and LLM call function needed for
// the /artifacts/:id/revise endpoint to actually process revisions.
func (h *Handler) SetRevisionConfig(skillRoot string, llm ReviseLLMFunc) {
	h.revisions.SetConfig(skillRoot, llm)
}

// RevisionService exposes the configured immutable revision coordinator to
// other authenticated backend surfaces without duplicating its LLM/content setup.
func (h *Handler) RevisionService() *RevisionService {
	return h.revisions
}

func (h *Handler) RegisterRoutes(r *gin.Engine, middleware ...gin.HandlerFunc) {
	api := r.Group("/api", middleware...)
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
	if !h.authorizeProject(c, projectID) {
		return
	}
	if err := h.materializeProject(c.Request.Context(), projectID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}
	artifacts, err := listArtifactsByProject(c.Request.Context(), h.service, projectID, c.Query("includeHistory") == "true")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}
	if artifacts == nil {
		artifacts = []*Artifact{}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": gin.H{"artifacts": artifacts}})
}

// ReconcileProjectArtifacts materializes reviewable artifacts that still live
// only in successful workflow-node output. It is idempotent and is used by the
// creator view so historical projects repair themselves before being displayed.
func (h *Handler) ReconcileProjectArtifacts(ctx context.Context, projectID string) error {
	if h == nil || h.service == nil || h.runRepo == nil || h.nodeRepo == nil {
		return nil
	}
	return h.materializeProject(ctx, projectID)
}

func (h *Handler) GetArtifact(c *gin.Context) {
	artifact, ok := h.authorizedArtifact(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": gin.H{"artifact": artifact}})
}

func (h *Handler) GetArtifactContent(c *gin.Context) {
	artifact, ok := h.authorizedArtifact(c)
	if !ok {
		return
	}
	content, mediaURL, mediaURLs := artifactContent(artifact)
	data := gin.H{
		"artifact":  artifact,
		"content":   content,
		"mediaUrl":  mediaURL,
		"mediaUrls": mediaURLs,
	}
	if reviewText, err := h.ResolveReviewableText(c.Request.Context(), artifact); err == nil {
		data["reviewText"] = reviewText
		if artifact.StorageType == StorageLocal {
			content = reviewText
			mediaURLs = mediaURLsFromString(reviewText)
			data["content"] = content
			data["mediaUrls"] = mediaURLs
		}
		mediaURL = firstMediaURL(mediaURLs)
		data["mediaUrl"] = mediaURL
	}
	mediaURLs = normalizeMediaURLs(mediaURLs)
	data["mediaUrls"] = mediaURLs
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": data})
}

// ResolveReviewableText returns the canonical source string used by both
// artifact review responses and conflict-safe creator text selections.
func (h *Handler) ResolveReviewableText(ctx context.Context, item *Artifact) (string, error) {
	if item == nil {
		return "", ErrRevisionContentUnavailable
	}
	if item.InlineJSON != "" || item.StorageType == StorageInline {
		return item.InlineJSON, nil
	}
	if hydrated, ok := h.hydrateLocalTextArtifactContent(ctx, item); ok {
		return string(hydrated), nil
	}
	return "", ErrRevisionContentUnavailable
}

func (h *Handler) GetArtifactHistory(c *gin.Context) {
	artifact, ok := h.authorizedArtifact(c)
	if !ok {
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
	if _, ok := h.authorizedArtifact(c); !ok {
		return
	}
	var req struct {
		Message        string                 `json:"message" binding:"required"`
		ModelProvider  map[string]interface{} `json:"modelProvider,omitempty"`
		ModelProviders map[string]interface{} `json:"modelProviders,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request: " + err.Error(), "data": nil})
		return
	}
	result, err := h.revisions.Revise(c.Request.Context(), ReviseRequest{
		ArtifactID: c.Param("id"), Message: req.Message,
		ModelProvider: req.ModelProvider, ModelProviders: req.ModelProviders,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrRevisionArtifactNotFound):
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "artifact not found", "data": nil})
		case errors.Is(err, ErrRevisionContentUnavailable):
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无法读取原始产物内容，请确保产物已生成后再返工", "data": nil})
		case errors.Is(err, ErrRevisionGeneration):
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "返工生成失败: " + strings.TrimPrefix(err.Error(), ErrRevisionGeneration.Error()+": "), "data": nil})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		}
		return
	}
	content, mediaURL, mediaURLs := artifactContent(result.Artifact)
	mediaURLs = normalizeMediaURLs(mediaURLs)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": gin.H{
		"artifact":  result.Artifact,
		"content":   content,
		"mediaUrl":  mediaURL,
		"mediaUrls": mediaURLs,
	}})
}

func (h *Handler) authorizedArtifact(c *gin.Context) (*Artifact, bool) {
	var artifact *Artifact
	var err error
	if h.service != nil {
		artifact, err = h.service.GetByID(c.Request.Context(), c.Param("id"))
	} else if h.revisions != nil && h.revisions.artifacts != nil {
		artifact, err = h.revisions.artifacts.GetByID(c.Request.Context(), c.Param("id"))
	} else {
		err = errors.New("artifact store unavailable")
	}
	if err != nil || artifact == nil || !h.authorizeProject(c, artifact.ProjectID) {
		if !c.IsAborted() {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "artifact not found", "data": nil})
		}
		return nil, false
	}
	return artifact, true
}

func (h *Handler) authorizeProject(c *gin.Context, projectID string) bool {
	userID, ok := auth.UserIDFromContext(c.Request.Context())
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized", "data": nil})
		return false
	}
	if h.access == nil || !h.access.CanAccessProject(c.Request.Context(), userID, projectID) {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"code": 404, "message": "artifact not found", "data": nil})
		return false
	}
	return true
}

func (h *Handler) materializeProject(ctx context.Context, projectID string) error {
	if h.runRepo == nil || h.nodeRepo == nil {
		return nil
	}
	seenTaskIDs := map[string]bool{}
	runs, err := h.runRepo.FindByProject(ctx, projectID)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.TaskID != "" {
			seenTaskIDs[run.TaskID] = true
		}
		nodes, err := h.nodeRepo.FindByTaskID(ctx, run.TaskID)
		if err != nil {
			return err
		}
		for _, node := range nodes {
			if node.Status != model.NodeSuccess {
				continue
			}
			requests, err := BuildArtifactRequestsFromNodeChecked(projectID, run.ID, node)
			if err != nil {
				return err
			}
			for _, req := range requests {
				if _, err := h.service.CreateArtifact(ctx, req); err != nil {
					return err
				}
			}
		}
	}
	requests, err := buildAgentTaskArtifactRequests(ctx, projectID, h.agentTasks, h.nodeRepo, seenTaskIDs)
	if err != nil {
		return err
	}
	for _, req := range requests {
		if _, err := h.service.CreateArtifact(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

func buildAgentTaskArtifactRequests(ctx context.Context, projectID string, tasks AgentTaskStore, nodes taskNodeFinder, seenTaskIDs map[string]bool) ([]*CreateArtifactRequest, error) {
	if tasks == nil || nodes == nil || projectID == "" {
		return nil, nil
	}
	agentTasks, err := tasks.FindAgentRuntimeTasksByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	requests := make([]*CreateArtifactRequest, 0)
	for _, task := range agentTasks {
		if task == nil || task.ID == "" || taskProjectID(task) != projectID {
			continue
		}
		if seenTaskIDs != nil && seenTaskIDs[task.ID] {
			continue
		}
		taskNodes, err := nodes.FindByTaskID(ctx, task.ID)
		if err != nil {
			return nil, err
		}
		for _, node := range taskNodes {
			if node == nil || node.Status != model.NodeSuccess {
				continue
			}
			next, err := BuildArtifactRequestsFromNodeChecked(projectID, task.ID, node)
			if err != nil {
				return nil, err
			}
			requests = append(requests, next...)
		}
	}
	return requests, nil
}

func taskProjectID(task *model.Task) string {
	if task == nil || task.Input == nil {
		return ""
	}
	if projectID, ok := task.Input["projectId"].(string); ok && strings.TrimSpace(projectID) != "" {
		return strings.TrimSpace(projectID)
	}
	if projectID, ok := task.Input["projectID"].(string); ok && strings.TrimSpace(projectID) != "" {
		return strings.TrimSpace(projectID)
	}
	contextMap, _ := task.Input["context"].(map[string]interface{})
	if contextMap == nil {
		return ""
	}
	if projectID, ok := contextMap["projectId"].(string); ok {
		return strings.TrimSpace(projectID)
	}
	if projectID, ok := contextMap["projectID"].(string); ok {
		return strings.TrimSpace(projectID)
	}
	return ""
}

func (h *Handler) hydrateLocalTextArtifactContent(ctx context.Context, artifact *Artifact) ([]byte, bool) {
	if !shouldHydrateLocalTextArtifact(artifact) || h.runRepo == nil || h.nodeRepo == nil {
		return nil, false
	}
	taskID := strings.TrimSpace(artifact.TaskID)
	run, err := h.runRepo.FindByID(ctx, artifact.WorkflowRunID)
	if err == nil && run != nil && strings.TrimSpace(run.TaskID) != "" {
		taskID = strings.TrimSpace(run.TaskID)
	}
	if taskID == "" {
		taskID = strings.TrimSpace(artifact.WorkflowRunID)
	}
	if taskID == "" {
		return nil, false
	}
	nodes, err := h.nodeRepo.FindByTaskID(ctx, taskID)
	if err != nil {
		return nil, false
	}
	for _, node := range nodes {
		if content, ok := contentFromMatchingNodeArtifact(artifact.ProjectID, artifact.WorkflowRunID, artifact, node); ok {
			return content, true
		}
	}
	return nil, false
}

func shouldHydrateLocalTextArtifact(artifact *Artifact) bool {
	if artifact == nil || artifact.StorageType != StorageLocal {
		return false
	}
	if strings.TrimSpace(artifact.WorkflowRunID) == "" && strings.TrimSpace(artifact.TaskID) == "" {
		return false
	}
	if strings.TrimSpace(artifact.InlineJSON) != "" {
		return false
	}
	return artifact.Kind == KindMarkdown || strings.HasPrefix(artifact.MimeType, "text/")
}

func contentFromMatchingNodeArtifact(projectID, workflowRunID string, artifact *Artifact, node *model.Node) ([]byte, bool) {
	if artifact == nil || node == nil || node.Status != model.NodeSuccess {
		return nil, false
	}
	requests, err := BuildArtifactRequestsFromNodeChecked(projectID, workflowRunID, node)
	if err != nil {
		return nil, false
	}
	if len(requests) == 0 {
		return nil, false
	}
	for _, req := range requests {
		if exactArtifactRequestMatch(artifact, req) && req.Data != nil {
			return req.Data, true
		}
	}

	var fallback []byte
	for _, req := range requests {
		if req == nil || len(req.Data) == 0 {
			continue
		}
		if req.StageName != artifact.StageName || req.Kind != artifact.Kind {
			continue
		}
		if artifact.Name != "" && req.Name == artifact.Name {
			return req.Data, true
		}
		if fallback != nil {
			return nil, false
		}
		fallback = req.Data
	}
	if fallback != nil {
		return fallback, true
	}
	return nil, false
}

func exactArtifactRequestMatch(artifact *Artifact, req *CreateArtifactRequest) bool {
	if artifact == nil || req == nil {
		return false
	}
	return req.StageName == artifact.StageName &&
		req.UnitID == artifact.UnitID &&
		req.Kind == artifact.Kind
}

func artifactContent(artifact *Artifact) (interface{}, string, []string) {
	if artifact.StorageType == StorageLocal {
		// For markdown/text artifacts without inline content, return a
		// readable placeholder instead of the metadata object.  The
		// frontend's MarkdownDocument component would otherwise render
		// String(metadataObject) → "[object Object]".
		if artifact.Kind == KindMarkdown || strings.HasPrefix(artifact.MimeType, "text/") {
			return "内容保存在本地系统中。请确保本地后台正在运行以查看完整内容。", "", []string{}
		}
		return map[string]interface{}{
			"storageRef":          artifact.StorageRef,
			"localOnly":           true,
			"cloudPayloadStored":  false,
			"contentAvailability": "local-agent",
		}, "", []string{}
	}
	if artifact.StorageType == StorageMinIO || artifact.StorageType == "url" {
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
