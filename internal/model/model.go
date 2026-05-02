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
	ContextAIRevise       ContextType = "AI_REVISE"
	ContextAICancelled    ContextType = "AI_CANCELLED"
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
	StartedAt     *time.Time             `json:"startedAt,omitempty"`
	CompletedAt   *time.Time             `json:"completedAt,omitempty"`
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

// ── Skill / Conversational AI types ──

// ConversationContext stores a skill session's state in Redis.
// Each chat turn creates a Task tracked via TaskIDs; the full conversation
// history is stored in Messages for prompt context and resume.
type ConversationContext struct {
	SessionID     string        `json:"session_id"`
	UserID        string        `json:"user_id"`
	Messages      []ChatMessage `json:"messages"`
	CurrentTaskID string        `json:"current_task_id,omitempty"`
	TaskIDs       []string      `json:"task_ids,omitempty"`
	MediaContext  MediaContext  `json:"media_context"`
	Version       int           `json:"version"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	Terminated    bool          `json:"terminated"`
}

// ChatMessage represents a single message in the conversation history.
type ChatMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Actions   []Action  `json:"actions,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Action represents a structured action within a chat message.
type Action struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// MediaContext describes the media files associated with a skill session.
type MediaContext struct {
	MediaCount  int      `json:"media_count"`
	MediaNames  []string `json:"media_names"`
	MediaIDs    []string `json:"media_ids,omitempty"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
}

// ── Skill API contracts ──

// CreateSessionRequest is the request body for creating a skill session.
type CreateSessionRequest struct {
	UserID      string   `json:"user_id,omitempty"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	Body        string   `json:"body,omitempty"`
	MediaCount  int      `json:"media_count,omitempty"`
	MediaNames  []string `json:"media_names,omitempty"`
	MediaIDs    []string `json:"media_ids,omitempty"`
}

// CreateSessionResponse is the response for a created skill session.
type CreateSessionResponse struct {
	SessionID string `json:"session_id"`
}

// ChatRequest is the request body for a skill chat message.
type ChatRequest struct {
	Message string `json:"message"`
}

// ChatResponse is the response for a skill chat message.
type ChatResponse struct {
	Reply       string        `json:"reply"`
	Suggestions []Suggestion  `json:"suggestions,omitempty"`
	Fields      *ChatFields   `json:"fields,omitempty"`
	Progress    *ChatProgress `json:"progress,omitempty"`
}

// ChatFields contains generated content fields returned to the frontend.
type ChatFields struct {
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Body        string   `json:"body,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	TaskID      string   `json:"task_id,omitempty"`
}

// ChatProgress describes the current progress of a skill task.
type ChatProgress struct {
	Status string `json:"status"`
	Phase  string `json:"phase"`
	TaskID string `json:"task_id,omitempty"`
}

// Suggestion is an optional action the user can take.
type Suggestion struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

// ProgressResponse is the response for the progress polling endpoint.
type ProgressResponse struct {
	Status       string `json:"status"`
	CurrentPhase string `json:"current_phase"`
	TaskID       string `json:"task_id,omitempty"`
}

// ToolManifestRecord is the database-persisted tool manifest row.
// It mirrors tool.ToolManifest for JSONB storage in PostgreSQL.
type ToolManifestRecord struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Type        string           `json:"type"`
	Version     string           `json:"version,omitempty"`
	Endpoint    string           `json:"endpoint,omitempty"`
	TimeoutMs   int              `json:"timeout_ms,omitempty"`
	Parameters  json.RawMessage  `json:"parameters"`
	Output      json.RawMessage  `json:"output"`
	Examples    json.RawMessage  `json:"examples"`
	Sandbox     bool             `json:"sandbox"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}
