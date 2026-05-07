package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/tangying-ai/tangying-ai-operation-system/internal/media"
	"github.com/tangying-ai/tangying-ai-operation-system/internal/model"
	"github.com/tangying-ai/tangying-ai-operation-system/internal/skill/service"
)

// SessionHandler handles HTTP requests for the skill dialog system.
type SessionHandler struct {
	sessionManager  *service.SessionManager
	planService     *service.PlanService
	resultAssembler *service.ResultAssembler
	mediaService    *media.MediaService
}

func NewSessionHandler(
	sessionManager *service.SessionManager,
	planService *service.PlanService,
	resultAssembler *service.ResultAssembler,
	mediaService *media.MediaService,
) *SessionHandler {
	return &SessionHandler{
		sessionManager:  sessionManager,
		planService:     planService,
		resultAssembler: resultAssembler,
		mediaService:    mediaService,
	}
}

// RegisterRoutes registers all skill dialog routes.
func (h *SessionHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/skill/dialog")
	{
		api.POST("/session/create", h.CreateSession)
		api.GET("/session/:session_id", h.GetSession)
		api.POST("/session/:session_id/chat", h.Chat)
		api.GET("/session/:session_id/progress", h.GetProgress)
		api.POST("/session/:session_id/terminate", h.TerminateSession)
	}
}

// CreateSession initializes a new conversation session.
func (h *SessionHandler) CreateSession(c *gin.Context) {
	var req model.CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = model.CreateSessionRequest{}
	}

	mediaCtx := model.MediaContext{
		Title:       req.Title,
		Description: req.Description,
		Keywords:    req.Keywords,
		MediaCount:  req.MediaCount,
		MediaNames:  req.MediaNames,
		MediaIDs:    req.MediaIDs,
		Platforms:   req.Platforms,
	}

	// Resolve media IDs to presigned URLs for multimodal vision
	if h.mediaService != nil && len(req.MediaIDs) > 0 {
		urls, err := h.mediaService.GetURLs(c.Request.Context(), req.MediaIDs)
		if err != nil {
			zap.L().Warn("Failed to resolve media URLs", zap.Error(err))
		} else {
			mediaCtx.MediaURLs = urls
		}
	}

	session, err := h.sessionManager.CreateSession(c.Request.Context(), req.UserID, mediaCtx)
	if err != nil {
		zap.L().Error("Failed to create session", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": model.CreateSessionResponse{
			SessionID: session.SessionID,
		},
	})
}

// GetSession returns the full session state including message history (for conversation resume).
func (h *SessionHandler) GetSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "session_id is required", "data": nil})
		return
	}

	session, err := h.sessionManager.GetSession(c.Request.Context(), sessionID)
	if err != nil {
		zap.L().Error("Failed to load session", zap.String("session_id", sessionID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "session not found", "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"session_id":    session.SessionID,
			"messages":      session.Messages,
			"media_context": session.MediaContext,
			"task_ids":      session.TaskIDs,
			"terminated":    session.Terminated,
			"created_at":    session.CreatedAt,
		},
	})
}

// Chat handles a new message in the conversation.
// Flow: build prompt with history → LLM generates DAG → create task → submit DAG → poll → return.
func (h *SessionHandler) Chat(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "session_id is required", "data": nil})
		return
	}

	var req model.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error(), "data": nil})
		return
	}

	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "message is required", "data": nil})
		return
	}

	ctx := c.Request.Context()

	// Load session
	session, err := h.sessionManager.GetSession(ctx, sessionID)
	if err != nil {
		zap.L().Error("Failed to load session", zap.String("session_id", sessionID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "session not found", "data": nil})
		return
	}
	if session.Terminated {
		c.JSON(http.StatusGone, gin.H{"code": 410, "message": "session has been terminated", "data": nil})
		return
	}

	// Append user message
	userMsg := model.ChatMessage{
		Role:      "user",
		Content:   req.Message,
		Timestamp: time.Now(),
	}
	if err := h.sessionManager.AppendMessage(ctx, session, userMsg); err != nil {
		zap.L().Error("Failed to append message", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	// Step 1: Generate DAG from conversation context
	dag, err := h.planService.GeneratePlan(ctx, session.Messages, req.Message, session.MediaContext)
	if err != nil {
		zap.L().Error("Plan generation failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "plan generation failed: " + err.Error(), "data": nil})
		return
	}

	// Step 2: Create task and submit DAG to orchestrator
	// Each chat turn = one Task, fully tracked by context module
	task, err := h.resultAssembler.CreateTask(ctx, map[string]interface{}{
		"source":    "skill_assistant",
		"message":   req.Message,
		"user_id":   session.UserID,
		"session_id": sessionID,
	})
	if err != nil {
		zap.L().Error("Task creation failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "task creation failed: " + err.Error(), "data": nil})
		return
	}

	if err := h.resultAssembler.SubmitDAG(ctx, task.ID, dag); err != nil {
		zap.L().Error("DAG submission failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "DAG submission failed: " + err.Error(), "data": nil})
		return
	}

	session.CurrentTaskID = task.ID
	session.TaskIDs = append(session.TaskIDs, task.ID)
	h.sessionManager.SaveSession(ctx, session)

	zap.L().Info("Skill task submitted to orchestrator",
		zap.String("taskId", task.ID),
		zap.String("sessionId", sessionID),
		zap.Int("nodeCount", len(dag.Nodes)))

	// Step 3: Poll for results and extract fields
	reply, fields, err := h.resultAssembler.PollAndExtract(ctx, task.ID, dag.Nodes)
	if err != nil {
		zap.L().Error("Task execution failed", zap.Error(err))
		errReply := fmt.Sprintf("抱歉，内容生成遇到问题：%s。请重试或换个方式描述需求。", err.Error())
		assistantMsg := model.ChatMessage{Role: "assistant", Content: errReply, Timestamp: time.Now()}
		h.sessionManager.AppendMessage(ctx, session, assistantMsg)
		c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": model.ChatResponse{
			Reply: errReply,
		}})
		return
	}

	// Attach task ID to fields for frontend trace
	if fields != nil {
		fields.TaskID = task.ID
	}

	// Step 4: Save assistant reply to session history
	assistantMsg := model.ChatMessage{
		Role:      "assistant",
		Content:   reply,
		Timestamp: time.Now(),
	}
	h.sessionManager.AppendMessage(ctx, session, assistantMsg)

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": model.ChatResponse{
		Reply:  reply,
		Fields: fields,
	}})
}

// GetProgress returns the current progress state for a session.
func (h *SessionHandler) GetProgress(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "session_id is required", "data": nil})
		return
	}

	session, err := h.sessionManager.GetSession(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "session not found", "data": nil})
		return
	}

	resp := model.ProgressResponse{
		Status: "IDLE",
	}

	if session.Terminated {
		resp.Status = "TERMINATED"
	} else if session.CurrentTaskID != "" {
		resp.Status = "EXECUTING"
		resp.TaskID = session.CurrentTaskID
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": resp})
}

// TerminateSession ends a session.
func (h *SessionHandler) TerminateSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "session_id is required", "data": nil})
		return
	}

	if err := h.sessionManager.TerminateSession(c.Request.Context(), sessionID); err != nil {
		zap.L().Error("Failed to terminate session", zap.String("session_id", sessionID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error(), "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "session terminated", "data": nil})
}
