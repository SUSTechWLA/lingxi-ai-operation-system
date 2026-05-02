package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/common/jsonx"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
)

type PublishService struct {
	cfg             config.OpenAIConfig
	orchestratorURL string
	httpClient      *http.Client
}

func NewPublishService(cfg config.OpenAIConfig, orchestratorURL string) *PublishService {
	return &PublishService{
		cfg:             cfg,
		orchestratorURL: orchestratorURL,
		httpClient:      &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second},
	}
}

type PublishRequest struct {
	Title       string                `json:"title"`
	Description string                `json:"description"`
	Keywords    string                `json:"keywords"`
	Platforms   []string              `json:"platforms"`
	ContentType string                `json:"contentType"`
	VideoFiles  []*multipart.FileHeader
	ImageFiles  []*multipart.FileHeader
	CoverFile   *multipart.FileHeader
}

type PublishResponse struct {
	TaskID  string `json:"taskId"`
	Message string `json:"message"`
}

func (s *PublishService) PublishContent(ctx context.Context, req *PublishRequest) (*PublishResponse, error) {
	contentSummary := fmt.Sprintf("标题：%s\n简介：%s\n关键词：%s",
		req.Title, req.Description, req.Keywords)

	// Describe uploaded media for the AI prompt
	mediaInfo := ""
	if len(req.VideoFiles) > 0 {
		names := make([]string, 0, len(req.VideoFiles))
		for _, f := range req.VideoFiles {
			names = append(names, f.Filename)
		}
		mediaInfo += fmt.Sprintf("视频文件：%v\n", names)
	}
	if len(req.ImageFiles) > 0 {
		names := make([]string, 0, len(req.ImageFiles))
		for _, f := range req.ImageFiles {
			names = append(names, f.Filename)
		}
		mediaInfo += fmt.Sprintf("图片文件：%v\n", names)
	}
	if req.CoverFile != nil {
		mediaInfo += fmt.Sprintf("封面图片：%s\n", req.CoverFile.Filename)
	}
	if mediaInfo != "" {
		contentSummary += "\n" + mediaInfo
	}

	// Create a DAG that polishes content via LLM, then marks as published
	nodes := []map[string]interface{}{
		{
			"id":   "polish_content",
			"type": "LLM",
			"name": "llm_api",
			"input": map[string]interface{}{
				"prompt": fmt.Sprintf(`You are a content polishing assistant. Polish the following content for social media publishing.
Return ONLY the polished version without extra formatting or quotes.

Content to polish:
%s`, contentSummary),
			},
		},
		{
			"id":   "log_result",
			"type": "TOOL",
			"name": "bash",
			"input": map[string]interface{}{
				"parameters": map[string]interface{}{
					"command": fmt.Sprintf(`echo "[PUBLISH] Task completed: %s" >> /tmp/lingxi-publish.log`, req.Title),
				},
			},
		},
	}

	edges := []map[string]interface{}{
		{"from": "polish_content", "to": "log_result"},
	}

	dagBody, _ := json.Marshal(map[string]interface{}{
		"nodes": nodes,
		"edges": edges,
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
		// The /api/node response uses "taskId"
		if id, ok := result["taskId"]; ok {
			taskID, _ = id.(string)
		}
		if taskID == "" {
			return nil, fmt.Errorf("orchestrator did not return taskId: %s", string(respBody))
		}
	}

	zap.L().Info("Content published as task",
		zap.String("taskId", taskID),
		zap.String("title", req.Title),
		zap.Int("videoCount", len(req.VideoFiles)),
		zap.Int("imageCount", len(req.ImageFiles)))

	return &PublishResponse{TaskID: taskID, Message: "内容已提交发布任务"}, nil
}

type AIGenerateResponse struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Body        string   `json:"body,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	TaskID      string   `json:"taskId,omitempty"`
}

type PolishSubmitResponse struct {
	TaskID   string `json:"taskId"`
	NodeID   string `json:"nodeId"`
	Message  string `json:"message"`
	TraceURL string `json:"traceUrl"`
}

type PolishQueryResponse struct {
	TaskID   string      `json:"taskId"`
	NodeID   string      `json:"nodeId"`
	Status   string      `json:"status"`
	Content  string      `json:"content,omitempty"`
	Error    string      `json:"error,omitempty"`
	TraceURL string      `json:"traceUrl"`
}

// AIGenerateSubmit submits a content generation task through the Orchestrator -> Worker -> llm_api pipeline.
func (s *PublishService) AIGenerateSubmit(ctx context.Context, prompt string) (*PolishSubmitResponse, error) {
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	nodeID := fmt.Sprintf("ai-generate-%d", time.Now().UnixMilli())

	fullPrompt := fmt.Sprintf(`你是一个自媒体内容创作助手。请根据用户的提示，生成适合自媒体发布的标题和简介。
要求：
1. 标题吸引眼球，不超过30字
2. 简介详细介绍内容亮点，200字以内
3. 返回纯JSON格式：{"title": "标题", "description": "简介", "keywords": ["关键词1"]}

用户需求：%s`, prompt)

	dagPayload := map[string]interface{}{
		"nodes": []map[string]interface{}{
			{
				"id":   nodeID,
				"type": "TOOL",
				"name": "llm_api",
				"input": map[string]interface{}{
					"prompt": fullPrompt,
				},
			},
		},
		"edges": []map[string]interface{}{},
	}

	dagBody, _ := json.Marshal(dagPayload)

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
		return nil, fmt.Errorf("orchestrator did not return taskId: %s", string(respBody))
	}

	zap.L().Info("AI generate task submitted",
		zap.String("taskId", taskID),
		zap.String("nodeId", nodeID))

	traceURL := fmt.Sprintf("/api/trace/%s", taskID)

	return &PolishSubmitResponse{
		TaskID:   taskID,
		NodeID:   nodeID,
		Message:  "内容生成任务已提交",
		TraceURL: traceURL,
	}, nil
}

// AIGenerateQueryResult queries the result of an AI generate task and returns the parsed response.
func (s *PublishService) AIGenerateQueryResult(ctx context.Context, taskID, nodeID string) (*AIGenerateResponse, error) {
	result := &AIGenerateResponse{}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", s.orchestratorURL+"/api/task/"+taskID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpResp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to query task: %w", err)
	}
	defer httpResp.Body.Close()

	body, _ := io.ReadAll(httpResp.Body)

	var taskResult map[string]interface{}
	if err := json.Unmarshal(body, &taskResult); err != nil {
		return nil, fmt.Errorf("failed to parse task response: %w", err)
	}

	// Find the target node
	var nodeStatus string
	var nodeOutput map[string]interface{}

	if nodes, ok := taskResult["nodes"].([]interface{}); ok {
		for _, n := range nodes {
			node, ok := n.(map[string]interface{})
			if !ok {
				continue
			}
			nid, _ := node["id"].(string)
			if nid != nodeID {
				continue
			}

			nodeStatus, _ = node["status"].(string)
			if output, ok := node["output"].(map[string]interface{}); ok {
				nodeOutput = output
			}
			break
		}
	}

	if nodeStatus == "" {
		return nil, fmt.Errorf("node %s not found in task %s", nodeID, taskID)
	}

	switch nodeStatus {
	case "SUCCESS":
		// Parse output
	case "FAILED":
		return nil, fmt.Errorf("generate task failed for node %s", nodeID)
	default:
		// Still pending/running
		return nil, nil
	}

	if nodeOutput == nil {
		return nil, fmt.Errorf("node %s has no output", nodeID)
	}

	// Extract content from llm_api tool output
	// LlmApiTool returns: {"content": "LLM response string", "model": "...", "rawResponse": {...}}
	// The LLM response is a JSON string like: {"title": "...", "description": "...", "keywords": [...]}
	stdout, _ := nodeOutput["stdout"].(string)
	if stdout == "" {
		return nil, fmt.Errorf("node %s stdout is empty", nodeID)
	}

	var toolOutput map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &toolOutput); err != nil {
		return nil, fmt.Errorf("failed to parse tool stdout: %w", err)
	}

	contentStr, _ := toolOutput["content"].(string)
	if contentStr == "" {
		return nil, fmt.Errorf("tool output content is empty")
	}

	if err := jsonx.ExtractJSON(contentStr, &result); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}

	result.TaskID = taskID
	return result, nil
}

// AIGenerateContent generates content via the Orchestrator -> Worker -> llm_api pipeline.
func (s *PublishService) AIGenerateContent(ctx context.Context, prompt string) (*AIGenerateResponse, error) {
	submitResp, err := s.AIGenerateSubmit(ctx, prompt)
	if err != nil {
		return nil, err
	}

	taskID := submitResp.TaskID
	nodeID := submitResp.NodeID

	// Poll for result (max 60s, every 800ms)
	maxAttempts := 75
	pollInterval := 800 * time.Millisecond

	for i := 0; i < maxAttempts; i++ {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled while polling generate result: %w", ctx.Err())
		default:
		}

		result, err := s.AIGenerateQueryResult(ctx, taskID, nodeID)
		if err != nil {
			return nil, fmt.Errorf("generate query failed: %w", err)
		}
		if result != nil {
			zap.L().Info("AI generate task completed",
				zap.String("taskId", taskID),
				zap.String("nodeId", nodeID))
			return result, nil
		}

		time.Sleep(pollInterval)
	}

	return nil, fmt.Errorf("polling timed out for generate task %s", taskID)
}

// AIGenerateFromMediaDAG submits a content generation task with media descriptions through the DAG pipeline.
func (s *PublishService) AIGenerateFromMediaSubmit(ctx context.Context, prompt string, images []*multipart.FileHeader, videos []*multipart.FileHeader) (*PolishSubmitResponse, error) {
	mediaDesc := ""
	if len(images) > 0 {
		names := make([]string, 0, len(images))
		for _, f := range images {
			names = append(names, f.Filename)
		}
		mediaDesc += fmt.Sprintf("上传的图片：%v\n", names)
	}
	if len(videos) > 0 {
		names := make([]string, 0, len(videos))
		for _, f := range videos {
			names = append(names, f.Filename)
		}
		mediaDesc += fmt.Sprintf("上传的视频：%v\n", names)
	}

	fullPrompt := prompt
	if mediaDesc != "" {
		fullPrompt = fmt.Sprintf(`用户上传了以下素材：
%s

用户补充说明：%s

请根据以上素材和说明，生成适合自媒体发布的标题和简介。`, mediaDesc, prompt)
	}

	return s.AIGenerateSubmit(ctx, fullPrompt)
}

// AIGenerateFromMedia generates content from media via the DAG pipeline.
func (s *PublishService) AIGenerateFromMedia(ctx context.Context, prompt string, images []*multipart.FileHeader, videos []*multipart.FileHeader) (*AIGenerateResponse, error) {
	submitResp, err := s.AIGenerateFromMediaSubmit(ctx, prompt, images, videos)
	if err != nil {
		return nil, err
	}

	taskID := submitResp.TaskID
	nodeID := submitResp.NodeID

	// Poll for result (max 60s, every 800ms)
	maxAttempts := 75
	pollInterval := 800 * time.Millisecond

	for i := 0; i < maxAttempts; i++ {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled while polling generate-from-media result: %w", ctx.Err())
		default:
		}

		result, err := s.AIGenerateQueryResult(ctx, taskID, nodeID)
		if err != nil {
			return nil, fmt.Errorf("generate query failed: %w", err)
		}
		if result != nil {
			zap.L().Info("AI generate-from-media task completed",
				zap.String("taskId", taskID),
				zap.String("nodeId", nodeID))
			return result, nil
		}

		time.Sleep(pollInterval)
	}

	return nil, fmt.Errorf("polling timed out for generate-from-media task %s", taskID)
}

// AIPolishSubmit submits a polish task through the Orchestrator -> Worker pipeline.
// Creates a DAG with a single polisher node and submits it to the orchestrator.
func (s *PublishService) AIPolishSubmit(ctx context.Context, text string, polishType string) (*PolishSubmitResponse, error) {
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}

	nodeID := fmt.Sprintf("polish-%d", time.Now().UnixMilli())

	// Build DAG with polisher node
	dagPayload := map[string]interface{}{
		"nodes": []map[string]interface{}{
			{
				"id":   nodeID,
				"type": "TOOL",
				"name": "polisher",
				"input": map[string]interface{}{
					"text":       text,
					"polishType": polishType,
				},
			},
		},
		"edges": []map[string]interface{}{},
	}

	dagBody, _ := json.Marshal(dagPayload)

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
		return nil, fmt.Errorf("orchestrator did not return taskId: %s", string(respBody))
	}

	zap.L().Info("Polish task submitted",
		zap.String("taskId", taskID),
		zap.String("nodeId", nodeID),
		zap.String("polishType", polishType),
		zap.Int("textLength", len(text)))

	traceURL := fmt.Sprintf("/api/trace/%s", taskID)

	return &PolishSubmitResponse{
		TaskID:   taskID,
		NodeID:   nodeID,
		Message:  "润色任务已提交",
		TraceURL: traceURL,
	}, nil
}

// AIPolishQueryResult queries the result of a polish task by taskId and nodeId.
func (s *PublishService) AIPolishQueryResult(ctx context.Context, taskID, nodeID string) (*PolishQueryResponse, error) {
	resp := &PolishQueryResponse{
		TaskID:   taskID,
		NodeID:   nodeID,
		TraceURL: fmt.Sprintf("/api/trace/%s", taskID),
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", s.orchestratorURL+"/api/task/"+taskID, nil)
	if err != nil {
		resp.Status = "ERROR"
		resp.Error = "Failed to create request: " + err.Error()
		return resp, nil
	}

	httpResp, err := s.httpClient.Do(httpReq)
	if err != nil {
		resp.Status = "ERROR"
		resp.Error = "Failed to query task: " + err.Error()
		return resp, nil
	}
	defer httpResp.Body.Close()

	body, _ := io.ReadAll(httpResp.Body)

	var taskResult map[string]interface{}
	if err := json.Unmarshal(body, &taskResult); err != nil {
		resp.Status = "ERROR"
		resp.Error = "Failed to parse task response: " + err.Error()
		return resp, nil
	}

	// Extract task status
	if taskData, ok := taskResult["task"].(map[string]interface{}); ok {
		status, _ := taskData["status"].(string)
		resp.Status = status
	} else {
		status, _ := taskResult["status"].(string)
		resp.Status = status
	}

	// Find the node and extract content
	if nodes, ok := taskResult["nodes"].([]interface{}); ok {
		for _, n := range nodes {
			node, ok := n.(map[string]interface{})
			if !ok {
				continue
			}
			nid, _ := node["id"].(string)
			if nid != nodeID {
				continue
			}

			nodeStatus, _ := node["status"].(string)
			resp.Status = nodeStatus

			if nodeStatus == "SUCCESS" {
				if output, ok := node["output"].(map[string]interface{}); ok {
					if content, ok := output["content"].(string); ok {
						resp.Content = content
					}
					// Also check for nested output (from ExecutableTool path)
					if stdout, ok := output["stdout"].(string); ok && resp.Content == "" {
						// stdout contains fmt.Sprintf("%v", toolResult.Data)
						var parsedData map[string]interface{}
						if err := json.Unmarshal([]byte(stdout), &parsedData); err == nil {
							if c, ok := parsedData["content"].(string); ok {
								resp.Content = c
							}
						}
					}
				}
				if resp.Content == "" {
					// Try to extract from node output directly
					if stdout, ok := node["stdout"].(string); ok {
						resp.Content = stdout
					}
				}
			} else if nodeStatus == "FAILED" {
				if errMsg, ok := node["error_message"].(string); ok {
					resp.Error = errMsg
				}
			}
			break
		}
	}

	if resp.Status == "" {
		resp.Status = "UNKNOWN"
	}

	return resp, nil
}

// AIPolishText submits a polish task and polls until completion.
// Returns the polished text with full traceability.
func (s *PublishService) AIPolishText(ctx context.Context, text string, polishType string) (string, string, error) {
	submitResp, err := s.AIPolishSubmit(ctx, text, polishType)
	if err != nil {
		return "", "", err
	}

	taskID := submitResp.TaskID
	nodeID := submitResp.NodeID

	// Poll for result with timeout (max 60s, check every 800ms)
	maxAttempts := 75
	pollInterval := 800 * time.Millisecond

	for i := 0; i < maxAttempts; i++ {
		select {
		case <-ctx.Done():
			return "", taskID, fmt.Errorf("context cancelled while polling polish result: %w", ctx.Err())
		default:
		}

		queryResp, err := s.AIPolishQueryResult(ctx, taskID, nodeID)
		if err != nil {
			return "", taskID, fmt.Errorf("failed to query polish result: %w", err)
		}

		switch queryResp.Status {
		case "SUCCESS":
			if queryResp.Content != "" {
				zap.L().Info("Polish task completed",
					zap.String("taskId", taskID),
					zap.String("nodeId", nodeID))
				return queryResp.Content, taskID, nil
			}
			return "", taskID, fmt.Errorf("polish completed but no content found")
		case "FAILED", "ERROR":
			errMsg := queryResp.Error
			if errMsg == "" {
				errMsg = "polish task failed"
			}
			return "", taskID, fmt.Errorf("polish failed: %s", errMsg)
		case "RUNNING", "READY", "CREATED":
			// Still processing, continue polling
			time.Sleep(pollInterval)
		default:
			// Unknown status, wait and retry
			time.Sleep(pollInterval)
		}
	}

	return "", taskID, fmt.Errorf("polling timed out for polish task %s", taskID)
}
