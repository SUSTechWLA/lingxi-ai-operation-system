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
	NodeCreated   NodeStatus = "CREATED"
	NodeReady     NodeStatus = "READY"
	NodeRunning   NodeStatus = "RUNNING"
	NodeRetrying  NodeStatus = "RETRYING"
	NodeSuccess   NodeStatus = "SUCCESS"
	NodeFailed    NodeStatus = "FAILED"
	NodeSkipped   NodeStatus = "SKIPPED"
)

// Node type
type NodeType string

const (
	NodeTypeTool    NodeType = "TOOL"
	NodeTypeLLM     NodeType = "LLM"
	NodeTypeLog     NodeType = "LOG"
	NodeTypeControl NodeType = "CONTROL"
)

// Context type
type ContextType string

const (
	ContextTaskCreated    ContextType = "TASK_CREATED"
	ContextDagValidated   ContextType = "DAG_VALIDATED"
	ContextDagSubmitted   ContextType = "DAG_SUBMITTED"
	ContextNodeReady      ContextType = "NODE_READY"
	ContextNodeScheduled  ContextType = "NODE_SCHEDULED"
	ContextNodeSuccess    ContextType = "NODE_SUCCESS"
	ContextNodeFailed     ContextType = "NODE_FAILED"
	ContextNodeRetry      ContextType = "NODE_RETRY"
	ContextTaskSuccess    ContextType = "TASK_SUCCESS"
	ContextTaskFailed     ContextType = "TASK_FAILED"
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
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Name        string                 `json:"name"`
	Input       map[string]interface{} `json:"input,omitempty"`
	Condition   string                 `json:"condition,omitempty"`
	MaxRetry    *int                   `json:"maxRetry,omitempty"`
	Priority    *int                   `json:"priority,omitempty"`
	WorkerGroup string                 `json:"workerGroup,omitempty"`
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

// Helper for JSONB
func ToJSONB(v interface{}) (json.RawMessage, error) {
	return json.Marshal(v)
}

func FromJSONB(data json.RawMessage, v interface{}) error {
	return json.Unmarshal(data, v)
}
