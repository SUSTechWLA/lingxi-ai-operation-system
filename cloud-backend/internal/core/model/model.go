package model

import (
	"encoding/json"
	"time"
)

// Task status
type TaskStatus string

const (
	TaskCreated TaskStatus = "CREATED"
	TaskRunning TaskStatus = "RUNNING"
	TaskPaused  TaskStatus = "PAUSED"
	TaskSuccess TaskStatus = "SUCCESS"
	TaskFailed  TaskStatus = "FAILED"
)

// Node status
type NodeStatus string

const (
	NodeCreated          NodeStatus = "CREATED"
	NodeReady            NodeStatus = "READY"
	NodeRunning          NodeStatus = "RUNNING"
	NodeWaitingLocal     NodeStatus = "WAITING_LOCAL"
	NodeLocalClaimed     NodeStatus = "LOCAL_CLAIMED"
	NodeLocalRunning     NodeStatus = "LOCAL_RUNNING"
	NodeLocalCompleted   NodeStatus = "LOCAL_COMPLETED"
	NodeLocalFailed      NodeStatus = "LOCAL_FAILED"
	NodeRetrying         NodeStatus = "RETRYING"
	NodeHeartbeatTimeout NodeStatus = "HEARTBEAT_TIMEOUT"
	NodeSuccess          NodeStatus = "SUCCESS"
	NodeFailed           NodeStatus = "FAILED"
	NodeSkipped          NodeStatus = "SKIPPED"
	NodeCancelled        NodeStatus = "CANCELLED"
)

// Node type
type NodeType string

const (
	NodeTypeTool       NodeType = "TOOL"
	NodeTypeLLM        NodeType = "LLM"
	NodeTypeLog        NodeType = "LOG"
	NodeTypeControl    NodeType = "CONTROL"
	NodeTypeReviewGate NodeType = "REVIEW_GATE"
)

// Context type
type ContextType string

const (
	ContextTaskCreated          ContextType = "TASK_CREATED"
	ContextDagValidated         ContextType = "DAG_VALIDATED"
	ContextDagSubmitted         ContextType = "DAG_SUBMITTED"
	ContextNodeReady            ContextType = "NODE_READY"
	ContextNodeScheduled        ContextType = "NODE_SCHEDULED"
	ContextNodeSuccess          ContextType = "NODE_SUCCESS"
	ContextNodeFailed           ContextType = "NODE_FAILED"
	ContextNodeRetry            ContextType = "NODE_RETRY"
	ContextTaskSuccess          ContextType = "TASK_SUCCESS"
	ContextTaskFailed           ContextType = "TASK_FAILED"
	ContextAIRevise             ContextType = "AI_REVISE"
	ContextAICancelled          ContextType = "AI_CANCELLED"
	ContextNodeProgress         ContextType = "NODE_PROGRESS"          // 长任务进度更新
	ContextNodeCheckpoint       ContextType = "NODE_CHECKPOINT"        // 长任务断点
	ContextNodeHeartbeatTimeout ContextType = "NODE_HEARTBEAT_TIMEOUT" // 心跳超时
	ContextNodeReviewRequired   ContextType = "NODE_REVIEW_REQUIRED"   // 人工审核节点等待确认
	ContextNodeSkipped          ContextType = "NODE_SKIPPED"           // 条件未满足跳过
)

type Task struct {
	ID          string                 `json:"id"`
	UserID      string                 `json:"userId,omitempty"`
	Status      TaskStatus             `json:"status"`
	Input       map[string]interface{} `json:"input,omitempty"`
	Output      map[string]interface{} `json:"output,omitempty"`
	PauseReason string                 `json:"pauseReason,omitempty"`
	CreatedAt   time.Time              `json:"createdAt"`
}

type Node struct {
	ID             string                 `json:"id"`
	TaskID         string                 `json:"taskId"`
	Type           NodeType               `json:"type"`
	Name           string                 `json:"name"`
	Status         NodeStatus             `json:"status"`
	Input          map[string]interface{} `json:"input,omitempty"`
	Output         map[string]interface{} `json:"output,omitempty"`
	ErrorMessage   string                 `json:"errorMessage,omitempty"`
	Condition      string                 `json:"condition,omitempty"`
	RetryCount     int                    `json:"retryCount"`
	MaxRetry       int                    `json:"maxRetry"`
	Priority       int                    `json:"priority"`
	WorkerGroup    string                 `json:"workerGroup"`
	Version        int                    `json:"version"`
	IdempotencyKey string                 `json:"idempotencyKey,omitempty"`
	CreatedAt      time.Time              `json:"createdAt"`
	StartedAt      *time.Time             `json:"startedAt,omitempty"`
	CompletedAt    *time.Time             `json:"completedAt,omitempty"`
	// Long-running task support
	LongRunning         bool       `json:"longRunning"`                   // 是否为长任务节点
	Progress            float64    `json:"progress"`                      // 执行进度 0.0 ~ 1.0
	CurrentStep         string     `json:"currentStep,omitempty"`         // 当前步骤描述
	HeartbeatTimeoutSec int        `json:"heartbeatTimeoutSec,omitempty"` // 自定义心跳超时秒数
	HeartbeatAt         *time.Time `json:"heartbeatAt,omitempty"`         // 最后心跳时间
}

type NodeDependency struct {
	ParentNodeID string `json:"parentNodeId"`
	ChildNodeID  string `json:"childNodeId"`
}

type Context struct {
	ID           int64                  `json:"id"`
	ContextType  ContextType            `json:"contextType"`
	TaskID       string                 `json:"taskId,omitempty"`
	NodeID       string                 `json:"nodeId,omitempty"`
	SourceModule string                 `json:"sourceModule,omitempty"`
	SourceTopic  string                 `json:"sourceTopic,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	Message      string                 `json:"message,omitempty"`
	SnapshotData map[string]interface{} `json:"snapshotData,omitempty"`
	CreatedAt    time.Time              `json:"createdAt"`
}

// DAG request models
type DAGRequest struct {
	Nodes []NodeRequest `json:"nodes"`
	Edges []Edge        `json:"edges"`
}

type NodeRequest struct {
	ID                  string                 `json:"id"`
	Type                string                 `json:"type"`
	Name                string                 `json:"name"`
	Input               map[string]interface{} `json:"input,omitempty"`
	Condition           string                 `json:"condition,omitempty"`
	MaxRetry            *int                   `json:"maxRetry,omitempty"`
	Priority            *int                   `json:"priority,omitempty"`
	WorkerGroup         string                 `json:"workerGroup,omitempty"`
	LongRunning         bool                   `json:"longRunning,omitempty"`         // 是否长任务节点
	HeartbeatTimeoutSec *int                   `json:"heartbeatTimeoutSec,omitempty"` // 自定义心跳超时
}

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Event models
type NodeResultEvent struct {
	TaskID         string                 `json:"taskId"`
	NodeID         string                 `json:"nodeId"`
	Status         NodeStatus             `json:"status"`
	Data           map[string]interface{} `json:"data,omitempty"`
	TraceID        string                 `json:"traceId,omitempty"`
	ErrorMessage   string                 `json:"errorMessage,omitempty"`
	IdempotencyKey string                 `json:"idempotencyKey,omitempty"`
}

type NodeTaskEvent struct {
	TaskID         string                 `json:"taskId"`
	NodeID         string                 `json:"nodeId"`
	Type           string                 `json:"type"`
	Payload        map[string]interface{} `json:"payload"`
	TraceID        string                 `json:"traceId,omitempty"`
	IdempotencyKey string                 `json:"idempotencyKey,omitempty"`
}

// ProgressEvent reports stage progress for long-running tasks.
type ProgressEvent struct {
	ProjectID   string  `json:"project_id"`
	Stage       string  `json:"stage"`
	Progress    float64 `json:"progress"`
	CurrentStep string  `json:"current_step"`
	TotalSteps  int     `json:"total_steps"`
	NodeID      string  `json:"node_id,omitempty"`
	Timestamp   string  `json:"timestamp"`
}

// TaskProgressResponse is the response for the task-level progress query.
type TaskProgressResponse struct {
	TaskID         string             `json:"taskId"`
	Status         string             `json:"status"`
	Progress       float64            `json:"progress"` // 0.0 ~ 1.0
	TotalNodes     int                `json:"totalNodes"`
	CompletedNodes int                `json:"completedNodes"`
	Nodes          []NodeProgressInfo `json:"nodes"`
}

// NodeProgressInfo describes a single node's execution progress.
type NodeProgressInfo struct {
	NodeID      string     `json:"nodeId"`
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	Status      string     `json:"status"`
	Progress    float64    `json:"progress"`
	CurrentStep string     `json:"currentStep,omitempty"`
	HeartbeatAt *time.Time `json:"heartbeatAt,omitempty"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// ToolManifestRecord is the database-persisted tool manifest row.
// It mirrors tool.ToolManifest for JSONB storage in PostgreSQL.
type ToolManifestRecord struct {
	Name                 string          `json:"name"`
	Description          string          `json:"description"`
	Type                 string          `json:"type"`
	Boundary             string          `json:"boundary,omitempty"`
	Version              string          `json:"version,omitempty"`
	Endpoint             string          `json:"endpoint,omitempty"`
	Transport            json.RawMessage `json:"transport,omitempty"`
	TimeoutMs            int             `json:"timeout_ms,omitempty"`
	Parameters           json.RawMessage `json:"parameters"`
	Output               json.RawMessage `json:"output"`
	Examples             json.RawMessage `json:"examples"`
	Sandbox              bool            `json:"sandbox"`
	Capabilities         json.RawMessage `json:"capabilities,omitempty"`
	Tags                 json.RawMessage `json:"tags,omitempty"`
	WhenToUse            json.RawMessage `json:"when_to_use,omitempty"`
	WhenNotToUse         json.RawMessage `json:"when_not_to_use,omitempty"`
	CostLevel            string          `json:"cost_level,omitempty"`
	LatencyLevel         string          `json:"latency_level,omitempty"`
	RiskLevel            string          `json:"risk_level,omitempty"`
	SideEffect           bool            `json:"side_effect,omitempty"`
	Idempotent           bool            `json:"idempotent,omitempty"`
	ApprovalPolicy       json.RawMessage `json:"approval_policy,omitempty"`
	ArtifactPolicy       json.RawMessage `json:"artifact_policy,omitempty"`
	ExecutionPlane       string          `json:"execution_plane,omitempty"`
	RequiresUserDevice   bool            `json:"requires_user_device,omitempty"`
	ArtifactLocation     string          `json:"artifact_location,omitempty"`
	LocalCommand         string          `json:"local_command,omitempty"`
	LocalRequirements    json.RawMessage `json:"local_requirements,omitempty"`
	Provider             string          `json:"provider,omitempty"`
	ProviderBinding      json.RawMessage `json:"provider_binding,omitempty"`
	ProviderCapabilities json.RawMessage `json:"provider_capabilities,omitempty"`
	NextRecommendedTools json.RawMessage `json:"next_recommended_tools,omitempty"`
	FailureModes         json.RawMessage `json:"failure_modes,omitempty"`
	SkillPackageID       string          `json:"skill_package_id,omitempty"`
	PromptRef            string          `json:"prompt_ref,omitempty"`
	ResourceRefs         json.RawMessage `json:"resource_refs,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}
