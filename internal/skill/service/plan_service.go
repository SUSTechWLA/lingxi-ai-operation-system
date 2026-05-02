package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/skill/prompts"
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

	prompt := fmt.Sprintf(prompts.SystemPromptSkillDAG, history, mediaInfo, toolsDesc, userMessage)

	llmMessages := []map[string]string{
		{"role": "system", "content": prompt},
	}

	var dag model.DAGRequest
	if err := s.llmClient.ChatWithJSON(ctx, llmMessages, &dag); err != nil {
		return nil, fmt.Errorf("failed to generate DAG via LLM: %w", err)
	}

	if len(dag.Nodes) == 0 {
		return nil, fmt.Errorf("LLM generated empty DAG")
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
