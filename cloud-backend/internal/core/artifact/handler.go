package artifact

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/model"
	modelRepo "github.com/tangying-ai/aios-core/internal/core/model/repository"
	"github.com/tangying-ai/aios-core/internal/core/workflow"
)

// ReviseLLMFunc is called to generate revised content via an LLM.
// systemPrompt provides the stage instruction context; userPrompt contains
// the original content and the user's revision instruction.
type ReviseLLMFunc func(ctx context.Context, systemPrompt, userPrompt string) (string, error)

type Handler struct {
	service  *Service
	runRepo  *workflow.RunRepository
	nodeRepo modelRepo.NodeRepo

	// Revision support — set via SetRevisionConfig.
	skillRoot string
	reviseLLM ReviseLLMFunc
}

func NewHandler(service *Service, runRepo *workflow.RunRepository, nodeRepo modelRepo.NodeRepo) *Handler {
	return &Handler{service: service, runRepo: runRepo, nodeRepo: nodeRepo}
}

// SetRevisionConfig wires the skill root path and LLM call function needed for
// the /artifacts/:id/revise endpoint to actually process revisions.
func (h *Handler) SetRevisionConfig(skillRoot string, llm ReviseLLMFunc) {
	h.skillRoot = skillRoot
	h.reviseLLM = llm
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
	if hydrated, ok := h.hydrateLocalTextArtifactContent(c.Request.Context(), artifact); ok {
		content = string(hydrated)
		mediaURLs = mediaURLsFromString(string(hydrated))
		mediaURL = firstMediaURL(mediaURLs)
	}
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

	// 1. Resolve the original content.
	originalContent := h.resolveOriginalContent(c.Request.Context(), base)
	if originalContent == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无法读取原始产物内容，请确保产物已生成后再返工", "data": nil})
		return
	}

	// 2. Build the stage instruction context.
	stageInstruction := h.readStageInstruction(base)
	systemPrompt := buildRevisionSystemPrompt(base.StageName, stageInstruction)
	userPrompt := fmt.Sprintf("原始内容：\n\n%s\n\n---\n\n修改意见：\n%s\n\n请根据修改意见重新生成完整内容，保持原有的格式结构。", originalContent, req.Message)

	// 3. Call the LLM to generate revised content.
	var revisedData []byte
	if h.reviseLLM != nil {
		revisedText, err := h.reviseLLM(c.Request.Context(), systemPrompt, userPrompt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "返工生成失败: " + err.Error(), "data": nil})
			return
		}
		revisedData = []byte(revisedText)
	} else {
		// Fallback: embed the revision instruction in a local-only record.
		revisedData = buildLocalRevisionData(base, req.Message)
	}

	// 4. Create the revision artifact with inline content.
	revisionReq := BuildRevisionRequest(base, req.Message, revisedData)
	revisionReq.StorageType = StorageInline // store the LLM response inline
	revision, err := h.service.CreateArtifact(c.Request.Context(), revisionReq)
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

// resolveOriginalContent returns the full original artifact content as a string.
func (h *Handler) resolveOriginalContent(ctx context.Context, a *Artifact) string {
	// If inline content exists, use it.
	if strings.TrimSpace(a.InlineJSON) != "" {
		return a.InlineJSON
	}
	// Try hydrating from the workflow node output.
	if hydrated, ok := h.hydrateLocalTextArtifactContent(ctx, a); ok {
		return string(hydrated)
	}
	// Fallback: use the artifact content helper result.
	content, _, _ := artifactContent(a)
	return stringifyContent(content)
}

// readStageInstruction reads the stage instruction markdown file for the given artifact.
func (h *Handler) readStageInstruction(a *Artifact) string {
	if h.skillRoot == "" || a.StageName == "" {
		return ""
	}
	// Look for <skillRoot>/<anything>/<version>/stages/<stageName>.md
	// Walk the skill root to find the matching stage file.
	entries, err := os.ReadDir(h.skillRoot)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Try version subdirectories
		versions, err := os.ReadDir(filepath.Join(h.skillRoot, entry.Name()))
		if err != nil {
			continue
		}
		for _, ver := range versions {
			if !ver.IsDir() {
				continue
			}
			stagePath := filepath.Join(h.skillRoot, entry.Name(), ver.Name(), "stages", a.StageName+".md")
			if data, err := os.ReadFile(stagePath); err == nil {
				return string(data)
			}
		}
	}
	return ""
}

func buildRevisionSystemPrompt(stageName string, stageInstruction string) string {
	prompt := fmt.Sprintf(`你是一个专业的内容返工助手，正在帮助用户修改「%s」阶段的产物。

重要规则：
- 严格根据用户的修改意见，在原始内容的基础上进行修改
- 保持原始内容的整体结构和格式风格
- 只修改用户明确要求修改的部分，不要擅自改动其他内容
- 如果原始内容是 Markdown 格式，输出 Markdown
- 如果原始内容是 JSON 格式，输出严格符合相同结构的 JSON
- 不要引入原始内容中没有的新字段、新章节或额外内容
- 输出完整内容，不要省略或截断`, stageName)

	if stageInstruction != "" {
		prompt += "\n\n阶段说明（参考上下文）：\n" + stageInstruction
	}
	return prompt
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

func (h *Handler) hydrateLocalTextArtifactContent(ctx context.Context, artifact *Artifact) ([]byte, bool) {
	if !shouldHydrateLocalTextArtifact(artifact) || h.runRepo == nil || h.nodeRepo == nil {
		return nil, false
	}
	run, err := h.runRepo.FindByID(ctx, artifact.WorkflowRunID)
	if err != nil || run == nil || strings.TrimSpace(run.TaskID) == "" {
		return nil, false
	}
	nodes, err := h.nodeRepo.FindByTaskID(ctx, run.TaskID)
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
	if artifact == nil || artifact.StorageType != StorageLocal || strings.TrimSpace(artifact.WorkflowRunID) == "" {
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
	requests := BuildArtifactRequestsFromNode(projectID, workflowRunID, node)
	if len(requests) == 0 {
		return nil, false
	}
	for _, req := range requests {
		if exactArtifactRequestMatch(artifact, req) && len(req.Data) > 0 {
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
