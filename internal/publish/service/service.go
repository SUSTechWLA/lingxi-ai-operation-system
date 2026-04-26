package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
)

type PublishService struct {
	cfg            config.OpenAIConfig
	orchestratorURL string
	httpClient     *http.Client
}

func NewPublishService(cfg config.OpenAIConfig, orchestratorURL string) *PublishService {
	return &PublishService{
		cfg:             cfg,
		orchestratorURL: orchestratorURL,
		httpClient:      &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second},
	}
}

type PublishRequest struct {
	Title       string   `json:"title" binding:"required"`
	Description string   `json:"description" binding:"required"`
	Keywords    string   `json:"keywords"`
	Platforms   []string `json:"platforms" binding:"required"`
}

type PublishResponse struct {
	TaskID  string `json:"taskId"`
	Message string `json:"message"`
}

func (s *PublishService) PublishContent(ctx context.Context, req *PublishRequest) (*PublishResponse, error) {
	dagBody, _ := json.Marshal(map[string]interface{}{
		"nodes": []map[string]interface{}{
			{
				"id":   "publish_content",
				"type": "TOOL",
				"name": "publish",
				"input": map[string]interface{}{
					"title":       req.Title,
					"description": req.Description,
					"keywords":    req.Keywords,
					"platforms":   req.Platforms,
				},
			},
		},
		"edges": []interface{}{},
	})

	httpReq, err := http.NewRequestWithContext(ctx, "POST", s.orchestratorURL+"/api/node", bytes.NewReader(dagBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to submit to orchestrator: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse orchestrator response: %w", err)
	}

	taskID, _ := result["taskId"].(string)
	if taskID == "" {
		return nil, fmt.Errorf("orchestrator did not return taskId")
	}

	zap.L().Info("Content published as task", zap.String("taskId", taskID))
	return &PublishResponse{TaskID: taskID, Message: "内容已提交发布任务"}, nil
}

type AIGenerateResponse struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func (s *PublishService) AIGenerateContent(ctx context.Context, prompt string) (*AIGenerateResponse, error) {
	systemPrompt := `你是一个自媒体内容创作助手。请根据用户的提示，生成适合自媒体发布的标题和简介。
要求：
1. 标题吸引眼球，不超过30字
2. 简介详细介绍内容亮点，200字以内
3. 返回纯JSON格式：{"title": "标题", "description": "简介"}`

	content, err := s.callOpenAI(ctx, systemPrompt, prompt)
	if err != nil {
		return nil, err
	}

	var result AIGenerateResponse
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	return &result, nil
}

func (s *PublishService) AIPolishText(ctx context.Context, text string, polishType string) (string, error) {
	systemPrompt := fmt.Sprintf(`你是一个自媒体内容润色专家。请润色以下%s，使其更加吸引人、专业。
要求：
1. 保持原意不变
2. 使表达更加流畅、吸引人
3. 直接返回润色后的文本，不要加引号或额外格式`, polishType)

	content, err := s.callOpenAI(ctx, systemPrompt, text)
	if err != nil {
		return "", err
	}

	return content, nil
}

func (s *PublishService) callOpenAI(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if s.cfg.APIKey == "" {
		return "", fmt.Errorf("API key is not configured")
	}

	requestBody := map[string]interface{}{
		"model":       s.cfg.Model,
		"temperature": s.cfg.Temperature,
		"max_tokens":  s.cfg.MaxTokens,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}

	body, _ := json.Marshal(requestBody)

	baseURL := s.cfg.BaseURL
	if len(baseURL) > 0 && baseURL[len(baseURL)-1] != '/' {
		baseURL += "/"
	}
	endpoint := baseURL + "chat/completions"

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var responseMap map[string]interface{}
	if err := json.Unmarshal(respBody, &responseMap); err != nil {
		return "", fmt.Errorf("failed to parse LLM response: %w", err)
	}

	choices, ok := responseMap["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("no choices in LLM response")
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid choice format")
	}

	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid message format")
	}

	content, _ := message["content"].(string)
	return content, nil
}
