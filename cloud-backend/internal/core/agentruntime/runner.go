package agentruntime

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

type RunStatus string

const (
	RunStatusCreated RunStatus = "CREATED"
	RunStatusRunning RunStatus = "RUNNING"
	RunStatusFailed  RunStatus = "FAILED"
)

type StartRunRequest struct {
	UserID       string                 `json:"userId,omitempty"`
	Message      string                 `json:"message"`
	Domain       string                 `json:"domain,omitempty"`
	Context      map[string]interface{} `json:"context,omitempty"`
	Mode         string                 `json:"mode,omitempty"`
	MaxCostLevel string                 `json:"maxCostLevel,omitempty"`
	MaxRiskLevel string                 `json:"maxRiskLevel,omitempty"`
}

type Run struct {
	ID        string                 `json:"id"`
	TaskID    string                 `json:"taskId,omitempty"`
	UserID    string                 `json:"userId,omitempty"`
	Domain    string                 `json:"domain,omitempty"`
	Message   string                 `json:"message"`
	Plan      *AgentPlan             `json:"plan,omitempty"`
	Status    RunStatus              `json:"status"`
	Budget    AgentBudget            `json:"budget,omitempty"`
	CreatedAt time.Time              `json:"createdAt"`
	UpdatedAt time.Time              `json:"updatedAt"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type Planner interface {
	GeneratePlan(ctx context.Context, req StartRunRequest) (*AgentPlan, error)
}

type Orchestrator interface {
	CreateTask(ctx context.Context, input map[string]interface{}) (*model.Task, error)
	SubmitDAG(ctx context.Context, taskID string, dagReq *model.DAGRequest) error
	GetTaskWithDetails(ctx context.Context, taskID string) (map[string]interface{}, error)
}

type RunStore interface {
	SaveRun(ctx context.Context, run *Run) error
	FindRun(ctx context.Context, id string) (*Run, error)
}

type Runner struct {
	orchestrator Orchestrator
	store        RunStore
	planner      Planner
	guard        *PlanGuard
	compiler     *PlanCompiler
}

func NewRunner(orchestrator Orchestrator, store RunStore, planner Planner, guard *PlanGuard, compiler *PlanCompiler) *Runner {
	return &Runner{
		orchestrator: orchestrator,
		store:        store,
		planner:      planner,
		guard:        guard,
		compiler:     compiler,
	}
}

func (r *Runner) Start(ctx context.Context, req StartRunRequest) (*Run, error) {
	if req.Message == "" {
		return nil, fmt.Errorf("message is required")
	}
	if r == nil || r.orchestrator == nil || r.store == nil || r.planner == nil || r.guard == nil || r.compiler == nil {
		return nil, fmt.Errorf("agent runner is not configured")
	}

	plan, err := r.planner.GeneratePlan(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate agent plan: %w", err)
	}
	if plan.Domain == "" {
		plan.Domain = req.Domain
	}
	if plan.Mode == "" {
		plan.Mode = "dynamic_agent"
	}
	if err := r.guard.ValidatePlan(ctx, req.UserID, plan); err != nil {
		return nil, fmt.Errorf("guard agent plan: %w", err)
	}

	dag, err := r.compiler.Compile(plan)
	if err != nil {
		return nil, fmt.Errorf("compile agent plan: %w", err)
	}

	run := &Run{
		ID:        "agent_run_" + uuid.NewString(),
		UserID:    req.UserID,
		Domain:    plan.Domain,
		Message:   req.Message,
		Plan:      plan,
		Status:    RunStatusCreated,
		Budget:    plan.Budget,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  map[string]interface{}{"mode": req.Mode},
	}

	task, err := r.orchestrator.CreateTask(ctx, map[string]interface{}{
		"source":     "agentruntime",
		"agentRunId": run.ID,
		"userId":     req.UserID,
		"message":    req.Message,
		"domain":     plan.Domain,
		"context":    req.Context,
		"plan":       plan,
	})
	if err != nil {
		return nil, fmt.Errorf("create agent task: %w", err)
	}
	run.TaskID = task.ID

	scoped := scopeDAGToTask(task.ID, dag)
	if err := r.orchestrator.SubmitDAG(ctx, task.ID, scoped); err != nil {
		run.Status = RunStatusFailed
		run.UpdatedAt = time.Now()
		_ = r.store.SaveRun(ctx, run)
		return nil, fmt.Errorf("submit agent DAG: %w", err)
	}

	run.Status = RunStatusRunning
	run.UpdatedAt = time.Now()
	if err := r.store.SaveRun(ctx, run); err != nil {
		return nil, fmt.Errorf("store agent run: %w", err)
	}
	return run, nil
}

func (r *Runner) Get(ctx context.Context, id string) (*Run, map[string]interface{}, error) {
	if r == nil || r.store == nil {
		return nil, nil, fmt.Errorf("agent runner is not configured")
	}
	run, err := r.store.FindRun(ctx, id)
	if err != nil || run == nil {
		return run, nil, err
	}
	if run.TaskID == "" || r.orchestrator == nil {
		return run, nil, nil
	}
	task, err := r.orchestrator.GetTaskWithDetails(ctx, run.TaskID)
	return run, task, err
}

func scopeDAGToTask(taskID string, dag *model.DAGRequest) *model.DAGRequest {
	if dag == nil {
		return nil
	}
	idMap := make(map[string]string, len(dag.Nodes))
	scoped := &model.DAGRequest{
		Nodes: make([]model.NodeRequest, len(dag.Nodes)),
		Edges: make([]model.Edge, len(dag.Edges)),
	}
	for i, node := range dag.Nodes {
		oldID := node.ID
		node.ID = scopedNodeID(taskID, oldID)
		idMap[oldID] = node.ID
		if node.Input != nil {
			node.Input = copyMap(node.Input)
			node.Input["agentOriginalNodeId"] = oldID
		}
		scoped.Nodes[i] = node
	}
	for i, edge := range dag.Edges {
		if mapped, ok := idMap[edge.From]; ok {
			edge.From = mapped
		}
		if mapped, ok := idMap[edge.To]; ok {
			edge.To = mapped
		}
		scoped.Edges[i] = edge
	}
	for i := range scoped.Nodes {
		if scoped.Nodes[i].Input != nil {
			if rewritten, ok := rewriteNodeReferences(scoped.Nodes[i].Input, idMap).(map[string]interface{}); ok {
				scoped.Nodes[i].Input = rewritten
			}
		}
	}
	return scoped
}

func scopedNodeID(taskID, nodeID string) string {
	const maxNodeIDLength = 64
	prefix := "t" + shortHash(taskID, 10) + "-"
	candidate := prefix + nodeID
	if len(candidate) <= maxNodeIDLength {
		return candidate
	}
	nodeHash := shortHash(nodeID, 8)
	maxBase := maxNodeIDLength - len(prefix) - len(nodeHash) - 1
	if maxBase < 1 {
		return prefix + nodeHash
	}
	return prefix + nodeID[:maxBase] + "-" + nodeHash
}

func shortHash(value string, length int) string {
	sum := sha1.Sum([]byte(value))
	encoded := hex.EncodeToString(sum[:])
	if length > len(encoded) {
		length = len(encoded)
	}
	return encoded[:length]
}

func rewriteNodeReferences(value interface{}, idMap map[string]string) interface{} {
	switch v := value.(type) {
	case string:
		out := v
		for oldID, newID := range idMap {
			out = strings.ReplaceAll(out, "{{"+oldID+".output.", "{{"+newID+".output.")
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for key, item := range v {
			out[key] = rewriteNodeReferences(item, idMap)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = rewriteNodeReferences(item, idMap)
		}
		return out
	default:
		return value
	}
}
