package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/model"
	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/publish/service"
)

type ChatHandler struct {
	chatService         *service.ChatService
	orchestratorService interface {
		CreateTask(ctx context.Context, input map[string]interface{}) (*model.Task, error)
		SubmitDAG(ctx context.Context, taskID string, dagReq *model.DAGRequest) error
		GetTaskWithDetails(ctx context.Context, taskID string) (map[string]interface{}, error)
	}
	contextService interface {
		RecordContext(ctx context.Context, taskID, nodeID string, ctxType model.ContextType, sourceModule, message string, metadata map[string]interface{}) error
	}
}

func NewChatHandler(chatService *service.ChatService, orchestratorService interface {
	CreateTask(ctx context.Context, input map[string]interface{}) (*model.Task, error)
	SubmitDAG(ctx context.Context, taskID string, dagReq *model.DAGRequest) error
	GetTaskWithDetails(ctx context.Context, taskID string) (map[string]interface{}, error)
}, contextService interface {
	RecordContext(ctx context.Context, taskID, nodeID string, ctxType model.ContextType, sourceModule, message string, metadata map[string]interface{}) error
}) *ChatHandler {
	return &ChatHandler{
		chatService:         chatService,
		orchestratorService: orchestratorService,
		contextService:      contextService,
	}
}

func (h *ChatHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.POST("/chat/generate", h.ChatGenerate)
		api.POST("/chat/revise", h.ChatRevise)
	}
}

func (h *ChatHandler) ChatGenerate(c *gin.Context) {
	if h.chatService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "service not available", "data": nil})
		return
	}

	var req service.ChatGenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error(), "data": nil})
		return
	}

	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "message is required", "data": nil})
		return
	}

	result, err := h.chatService.Generate(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": result})
}

func (h *ChatHandler) ChatRevise(c *gin.Context) {
	var req struct {
		Message       string   `json:"message"`
		Title         string   `json:"title"`
		Description   string   `json:"description"`
		Keywords      []string `json:"keywords"`
		MediaCount    int      `json:"media_count"`
		MediaNames    []string `json:"media_names"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error(), "data": nil})
		return
	}

	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "message is required", "data": nil})
		return
	}

	if h.orchestratorService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "orchestrator not available", "data": nil})
		return
	}

	// Step 1: Create task
	task, err := h.orchestratorService.CreateTask(c.Request.Context(), map[string]interface{}{
		"source":  "ai_assistant",
		"type":    "revise",
		"message": req.Message,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to create task: " + err.Error(), "data": nil})
		return
	}

	nodeID := fmt.Sprintf("chat_revise-%d", time.Now().UnixMilli())

	// Build tool input with media context
	toolInput := map[string]interface{}{
		"message":     req.Message,
		"title":       req.Title,
		"description": req.Description,
		"keywords":    req.Keywords,
	}
	if req.MediaCount > 0 {
		toolInput["media_count"] = req.MediaCount
		toolInput["media_names"] = req.MediaNames
	}

	// Step 2: Build and submit DAG with a single chat_revise node
	dagReq := &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{
				ID:   nodeID,
				Type: "TOOL",
				Name: "chat_revise",
				Input: toolInput,
			},
		},
		Edges: []model.Edge{},
	}

	if err := h.orchestratorService.SubmitDAG(c.Request.Context(), task.ID, dagReq); err != nil {
		zap.L().Error("failed to submit DAG for chat revise", zap.String("taskId", task.ID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to submit DAG: " + err.Error(), "data": nil})
		return
	}

	zap.L().Info("Chat revise DAG submitted",
		zap.String("taskId", task.ID),
		zap.String("nodeId", nodeID),
		zap.String("message", req.Message),
	)

	// Step 3: Poll for node completion (max 60s, every 500ms)
	result, err := h.pollNodeResult(c.Request.Context(), task.ID, nodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": gin.H{"task_id": task.ID}})
		return
	}

	// Step 4: Record AI_REVISE context
	if result.fields.Title != "" || result.fields.Description != "" || len(result.fields.Keywords) > 0 {
		var changed []string
		if result.fields.Title != "" {
			changed = append(changed, "标题=\""+result.fields.Title+"\"")
		}
		if result.fields.Description != "" {
			changed = append(changed, "简介=\""+result.fields.Description+"\"")
		}
		if len(result.fields.Keywords) > 0 {
			changed = append(changed, "关键词="+fmt.Sprintf("%v", result.fields.Keywords))
		}
		ctxMsg := "AI助手修改了内容: " + strings.Join(changed, "; ")
		if err := h.contextService.RecordContext(c.Request.Context(), task.ID, nodeID, "AI_REVISE", "ChatHandler", ctxMsg, nil); err != nil {
			zap.L().Error("failed to record AI_REVISE context", zap.Error(err))
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"reply":   result.reply,
			"fields":  result.fields,
			"task_id": task.ID,
		},
	})
}

type revisePollResult struct {
	reply  string
	fields struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Keywords    []string `json:"keywords"`
	}
}

func (h *ChatHandler) pollNodeResult(ctx context.Context, taskID, nodeID string) (*revisePollResult, error) {
	maxAttempts := 120  // 120 * 500ms = 60s
	pollInterval := 500 * time.Millisecond

	for i := 0; i < maxAttempts; i++ {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled while polling: %w", ctx.Err())
		default:
		}

		details, err := h.orchestratorService.GetTaskWithDetails(ctx, taskID)
		if err != nil {
			zap.L().Warn("failed to query task details", zap.String("taskId", taskID), zap.Error(err))
			time.Sleep(pollInterval)
			continue
		}

		nodesRaw, ok := details["nodes"].([]*model.Node)
		if !ok {
			// Try []interface{} format from JSON unmarshal
			nodesIface, ok2 := details["nodes"].([]interface{})
			if !ok2 {
				time.Sleep(pollInterval)
				continue
			}
			nodesRaw = make([]*model.Node, 0, len(nodesIface))
			for _, n := range nodesIface {
				if node, ok3 := n.(*model.Node); ok3 {
					nodesRaw = append(nodesRaw, node)
				}
			}
		}

		for _, node := range nodesRaw {
			if node.ID != nodeID {
				continue
			}

			switch node.Status {
			case "SUCCESS":
				return h.extractToolResult(node.Output)
			case "FAILED":
				errMsg := node.ErrorMessage
				if errMsg == "" {
					errMsg = "node execution failed"
				}
				return nil, fmt.Errorf("chat revise failed: %s", errMsg)
			case "CREATED", "READY", "RUNNING", "RETRYING":
				// Still processing, continue polling
			default:
				// Unknown status, wait and retry
			}
			break
		}

		time.Sleep(pollInterval)
	}

	return nil, fmt.Errorf("chat revise timed out for task %s", taskID)
}

func (h *ChatHandler) extractToolResult(output map[string]interface{}) (*revisePollResult, error) {
	result := &revisePollResult{}

	if len(output) == 0 {
		return nil, fmt.Errorf("node output is empty")
	}

	// stdout contains the JSON-serialized tool result
	stdout, _ := output["stdout"].(string)
	if stdout == "" {
		// Try direct output fields
		if reply, ok := output["reply"].(string); ok {
			result.reply = reply
		}
		if title, ok := output["title"].(string); ok {
			result.fields.Title = title
		}
		if desc, ok := output["description"].(string); ok {
			result.fields.Description = desc
		}
		if kw, ok := output["keywords"].([]interface{}); ok {
			for _, k := range kw {
				if s, ok := k.(string); ok {
					result.fields.Keywords = append(result.fields.Keywords, s)
				}
			}
		}
		return result, nil
	}

	// Parse stdout as JSON
	var toolData map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &toolData); err != nil {
		return nil, fmt.Errorf("failed to parse tool output: %w", err)
	}

	if reply, ok := toolData["reply"].(string); ok {
		result.reply = reply
	}
	if title, ok := toolData["title"].(string); ok {
		result.fields.Title = title
	}
	if desc, ok := toolData["description"].(string); ok {
		result.fields.Description = desc
	}
	if kw, ok := toolData["keywords"].([]interface{}); ok {
		for _, k := range kw {
			if s, ok := k.(string); ok {
				result.fields.Keywords = append(result.fields.Keywords, s)
			}
		}
	}

	return result, nil
}
