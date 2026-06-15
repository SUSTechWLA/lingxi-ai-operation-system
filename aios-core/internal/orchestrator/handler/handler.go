package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	contextSvc "github.com/tangying-ai/aios-core/internal/context/service"
	"github.com/tangying-ai/aios-core/internal/model"
	"github.com/tangying-ai/aios-core/internal/orchestrator/service"
)

type OrchestratorHandler struct {
	orchestratorService *service.OrchestratorService
	stateMachine        *service.StateMachine
	taskExecutionCtrl   *service.TaskExecutionControl
	contextService      *contextSvc.ContextService
}

func NewOrchestratorHandler(
	orchestratorService *service.OrchestratorService,
	stateMachine *service.StateMachine,
	taskExecutionCtrl *service.TaskExecutionControl,
	contextService *contextSvc.ContextService,
) *OrchestratorHandler {
	return &OrchestratorHandler{
		orchestratorService: orchestratorService,
		stateMachine:        stateMachine,
		taskExecutionCtrl:   taskExecutionCtrl,
		contextService:      contextService,
	}
}

func (h *OrchestratorHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.POST("/task/create", h.CreateTask)
		api.POST("/task/:taskId/dag", h.SubmitDAG)
		api.GET("/task/:taskId", h.GetTask)
		api.GET("/task/:taskId/context", h.GetTaskContext)
		api.POST("/task/:taskId/pause", h.PauseTask)
api.POST("/task/:taskId/fail", h.FailTask)
		api.POST("/task/:taskId/resume", h.ResumeTask)
		api.GET("/task/:taskId/pause-reason", h.GetPauseReason)
		api.POST("/node/:nodeId/success", h.OnNodeSuccess)
		api.POST("/node/:nodeId/failure", h.OnNodeFailure)
		api.GET("/node/:nodeId/snapshot/latest", h.GetLatestSnapshot)
		api.POST("/node/:nodeId/restore", h.RestoreFromSnapshot)
		api.POST("/node/:nodeId/retry", h.RetryNode)
		api.POST("/node", h.SubmitDAGFromNL)
		api.GET("/health", h.Health)
	}
}

func (h *OrchestratorHandler) CreateTask(c *gin.Context) {
	var request map[string]interface{}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, err := h.orchestratorService.CreateTask(c.Request.Context(), request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"taskId": task.ID,
		"status": task.Status,
	})
}

func (h *OrchestratorHandler) SubmitDAG(c *gin.Context) {
	taskID := c.Param("taskId")

	var dagReq model.DAGRequest
	if err := c.ShouldBindJSON(&dagReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.orchestratorService.SubmitDAG(c.Request.Context(), taskID, &dagReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"taskId":  taskID,
		"message": "DAG submitted successfully",
	})
}

func (h *OrchestratorHandler) GetTask(c *gin.Context) {
	taskID := c.Param("taskId")

	result, err := h.orchestratorService.GetTaskWithDetails(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if result == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *OrchestratorHandler) GetTaskContext(c *gin.Context) {
	taskID := c.Param("taskId")

	contexts, err := h.contextService.GetContextForTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, contexts)
}

func (h *OrchestratorHandler) OnNodeSuccess(c *gin.Context) {
	nodeID := c.Param("nodeId")

	var output map[string]interface{}
	if err := c.ShouldBindJSON(&output); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.stateMachine.OnSuccess(c.Request.Context(), nodeID, output); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Node success recorded"})
}

func (h *OrchestratorHandler) OnNodeFailure(c *gin.Context) {
	nodeID := c.Param("nodeId")

	var request map[string]string
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	errorMessage := request["errorMessage"]
	if errorMessage == "" {
		errorMessage = "Unknown error"
	}

	if err := h.stateMachine.OnFailure(c.Request.Context(), nodeID, errorMessage); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Node failure recorded"})
}

func (h *OrchestratorHandler) GetLatestSnapshot(c *gin.Context) {
	nodeID := c.Param("nodeId")

	snapshot, err := h.contextService.GetLatestSnapshotForNode(c.Request.Context(), nodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if snapshot == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No snapshot found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"nodeId":   nodeID,
		"snapshot": snapshot,
	})
}

func (h *OrchestratorHandler) RestoreFromSnapshot(c *gin.Context) {
	nodeID := c.Param("nodeId")

	result, err := h.contextService.RestoreNodeFromSnapshot(c.Request.Context(), nodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if result == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No snapshot found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"nodeId":   nodeID,
		"message":  "Node snapshot retrieved",
		"snapshot": result,
	})
}

func (h *OrchestratorHandler) PauseTask(c *gin.Context) {
	taskID := c.Param("taskId")

	var request map[string]string
	_ = c.ShouldBindJSON(&request)

	reason := request["reason"]
	if reason == "" {
		reason = "Manual pause"
	}

	if err := h.taskExecutionCtrl.PauseTask(c.Request.Context(), taskID, reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"taskId":  taskID,
		"message": "Task paused successfully",
	})
}

func (h *OrchestratorHandler) FailTask(c *gin.Context) {
	taskID := c.Param("taskId")

	if err := h.taskExecutionCtrl.FailTask(c.Request.Context(), taskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"taskId":  taskID,
		"message": "Task failed successfully",
	})
}

func (h *OrchestratorHandler) ResumeTask(c *gin.Context) {
	taskID := c.Param("taskId")

	if err := h.taskExecutionCtrl.ResumeTask(c.Request.Context(), taskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"taskId":  taskID,
		"message": "Task resumed successfully",
	})
}

func (h *OrchestratorHandler) RetryNode(c *gin.Context) {
	nodeID := c.Param("nodeId")

	if err := h.taskExecutionCtrl.RetryNode(c.Request.Context(), nodeID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"nodeId":  nodeID,
		"message": "Node retry initiated",
	})
}

func (h *OrchestratorHandler) GetPauseReason(c *gin.Context) {
	taskID := c.Param("taskId")

	reason := h.taskExecutionCtrl.GetTaskPauseReason(c.Request.Context(), taskID)
	c.JSON(http.StatusOK, gin.H{
		"taskId": taskID,
		"reason": reason,
	})
}

func (h *OrchestratorHandler) SubmitDAGFromNL(c *gin.Context) {
	var dagPayload map[string]interface{}
	if err := c.ShouldBindJSON(&dagPayload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	input := map[string]interface{}{"source": "nl-translator"}
	task, err := h.orchestratorService.CreateTask(c.Request.Context(), input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	dagReq := convertToDAGRequest(dagPayload)
	if err := h.orchestratorService.SubmitDAG(c.Request.Context(), task.ID, dagReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"taskId":  task.ID,
		"status":  task.Status,
		"message": "DAG submitted successfully",
	})
}

func (h *OrchestratorHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "UP",
		"service": "ai-orchestrator",
	})
}

func convertToDAGRequest(payload map[string]interface{}) *model.DAGRequest {
	dagReq := &model.DAGRequest{}

	if nodesData, ok := payload["nodes"].([]interface{}); ok {
		for _, n := range nodesData {
			nodeMap, ok := n.(map[string]interface{})
			if !ok {
				continue
			}

			nodeID, _ := nodeMap["nodeId"].(string)
			if nodeID == "" {
				nodeID, _ = nodeMap["id"].(string)
			}

			nodeType, _ := nodeMap["type"].(string)
			nodeName, _ := nodeMap["name"].(string)
			nodeInput, _ := nodeMap["input"].(map[string]interface{})

			nodeReq := model.NodeRequest{
				ID:    nodeID,
				Type:  nodeType,
				Name:  nodeName,
				Input: nodeInput,
			}
			dagReq.Nodes = append(dagReq.Nodes, nodeReq)
		}
	}

	if edgesData, ok := payload["edges"].([]interface{}); ok {
		for _, e := range edgesData {
			edgeMap, ok := e.(map[string]interface{})
			if !ok {
				continue
			}

			from, _ := edgeMap["from"].(string)
			to, _ := edgeMap["to"].(string)

			dagReq.Edges = append(dagReq.Edges, model.Edge{From: from, To: to})
		}
	}

	return dagReq
}
