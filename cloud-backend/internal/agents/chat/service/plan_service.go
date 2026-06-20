package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/common/llmutil"
	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/agents/chat/prompts"
)

// PlanService generates execution plans (DAGs) from conversation context via direct LLM call.
type PlanService struct {
	llmClient        *LLMClient
	toolManifestSvc *ToolManifestService
}

func NewPlanService(llmClient *LLMClient, toolManifestSvc *ToolManifestService) *PlanService {
	return &PlanService{
		llmClient:        llmClient,
		toolManifestSvc: toolManifestSvc,
	}
}

// GeneratePlan builds a prompt with full conversation history, media context, and all available tools,
// calls the LLM directly to produce a DAG, and returns it.
func (s *PlanService) GeneratePlan(
	ctx context.Context,
	messages []model.ChatMessage,
	userMessage string,
	mediaCtx model.MediaContext,
) (*model.DAGRequest, error) {
	history := buildConversationHistory(messages)
	mediaInfo := buildMediaContextInfo(mediaCtx)

	toolsDesc, err := s.toolManifestSvc.FormatForPrompt(ctx)
	if err != nil {
		zap.L().Warn("Failed to query tool manifests, using fallback", zap.Error(err))
		toolsDesc = "chat_generate: 通用内容生成\nchat_revise: 修改已有内容"
	}

	prompt := fmt.Sprintf(prompts.SystemPromptChatDAG, history, mediaInfo, toolsDesc, userMessage)

	llmMessages := []map[string]interface{}{
		{"role": "system", "content": prompt},
	}

	var dag model.DAGRequest
	if err := s.llmClient.ChatWithJSON(ctx, llmMessages, &dag); err != nil {
		return nil, fmt.Errorf("failed to generate DAG via LLM: %w", err)
	}

	if len(dag.Nodes) == 0 {
		return nil, fmt.Errorf("LLM generated empty DAG")
	}

	// Inject image URLs into LLM-calling nodes so multimodal models can see the actual images
	if len(mediaCtx.MediaURLs) > 0 {
		injectImageURLs(dag.Nodes, mediaCtx.MediaURLs)
	}

	zap.L().Info("Plan generated via LLM",
		zap.Int("nodes", len(dag.Nodes)),
		zap.Int("edges", len(dag.Edges)))

	return &dag, nil
}

func buildConversationHistory(messages []model.ChatMessage) string {
	if len(messages) == 0 {
		return "（无历史对话）"
	}
	var result string
	for i, m := range messages {
		role := "用户"
		if m.Role == "assistant" {
			role = "助手"
		}
		content := m.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		result += fmt.Sprintf("%d. [%s] %s\n", i+1, role, content)
	}
	return result
}

func buildMediaContextInfo(mediaCtx model.MediaContext) string {
	var parts []string

	if mediaCtx.Title != "" {
		parts = append(parts, fmt.Sprintf("- 当前页面标题：%s", mediaCtx.Title))
	}
	if mediaCtx.Description != "" {
		parts = append(parts, fmt.Sprintf("- 当前页面简介：%s", mediaCtx.Description))
	}
	if len(mediaCtx.Keywords) > 0 {
		kwStr := mediaCtx.Keywords[0]
		for i := 1; i < len(mediaCtx.Keywords); i++ {
			kwStr += ", " + mediaCtx.Keywords[i]
		}
		parts = append(parts, fmt.Sprintf("- 当前页面关键词：%s", kwStr))
	}

	if mediaCtx.MediaCount > 0 {
		parts = append(parts, fmt.Sprintf("- 已上传 %d 个素材文件：", mediaCtx.MediaCount))
		for i, name := range mediaCtx.MediaNames {
			idStr := ""
			if i < len(mediaCtx.MediaIDs) {
				idStr = fmt.Sprintf(" (ID: %s)", mediaCtx.MediaIDs[i])
			}
			parts = append(parts, fmt.Sprintf("    %d. %s%s", i+1, name, idStr))
		}
	} else {
		parts = append(parts, "- 用户当前未上传任何素材文件")
	}

	if len(mediaCtx.Platforms) > 0 {
		parts = append(parts, fmt.Sprintf("- 用户选择的目标发布平台：%s", strings.Join(mediaCtx.Platforms, "、")))
	}

	if len(parts) == 0 {
		return "无页面上下文信息。"
	}
	return "## 页面当前状态\n" + joinLines(parts)
}

func joinLines(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += "\n" + parts[i]
	}
	return result
}

// llmTools is the set of tool names that accept image_urls for multimodal vision.
var llmTools = map[string]bool{
	"chat_generate":    true,
	"chat_revise":      true,
	"llm_api":          true,
	"content_generator": true,
	"polisher":         true,
}

// injectImageURLs converts presigned HTTP URLs to base64 data URLs and adds them
// to every LLM-calling node's input. External LLM APIs cannot reach internal
// MinIO presigned URLs (e.g. localhost:9000), so we download and inline the images.
func injectImageURLs(nodes []model.NodeRequest, urls []string) {
	dataURLs := resolveToDataURLs(urls)
	if len(dataURLs) == 0 {
		return
	}

	urlsInterface := make([]interface{}, len(dataURLs))
	for i, u := range dataURLs {
		urlsInterface[i] = u
	}
	for i := range nodes {
		if llmTools[nodes[i].Name] {
			if nodes[i].Input == nil {
				nodes[i].Input = map[string]interface{}{}
			}
			nodes[i].Input["image_urls"] = urlsInterface
		}
	}
}

// resolveToDataURLs converts HTTP presigned URLs to base64 data URLs.
// External LLM services cannot reach internal MinIO endpoints, so we
// fetch the images from the backend (which can reach MinIO) and inline them.
func resolveToDataURLs(urls []string) []string {
	dataURLs := make([]string, 0, len(urls))
	client := &http.Client{Timeout: 10 * time.Second}

	for _, u := range urls {
		if strings.HasPrefix(u, "data:") {
			dataURLs = append(dataURLs, u)
			continue
		}

		resp, err := client.Get(u)
		if err != nil {
			zap.L().Warn("failed to fetch image from presigned URL, skipping",
				zap.String("url", u[:min(50, len(u))]), zap.Error(err))
			continue
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			zap.L().Warn("failed to read image data, skipping", zap.Error(err))
			continue
		}

		mimeType := resp.Header.Get("Content-Type")

		// Skip non-image media (e.g., videos). Multimodal vision APIs only
		// accept image formats; sending video bytes wrapped as image_url
		// causes the API to return an error or empty choices.
		if mimeType != "" && !llmutil.IsImageMimeType(mimeType) {
			zap.L().Warn("skipping non-image media for multimodal request",
				zap.String("mimeType", mimeType))
			continue
		}
		if mimeType == "" {
			mimeType = "image/jpeg"
		}

		// Compress to reduce token usage
		compressed, compressedMime := llmutil.CompressImageBytes(data)
		dataURL := llmutil.Base64DataURL(compressedMime, compressed)
		dataURLs = append(dataURLs, dataURL)
	}

	return dataURLs
}
