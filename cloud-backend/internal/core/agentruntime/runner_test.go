package agentruntime

import (
	"context"
	"testing"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestRunnerStart_CreatesTaskScopesDAGAndStoresRun(t *testing.T) {
	store := newMemoryRunStore()
	orch := &fakeOrchestrator{taskID: "task-1"}
	planner := staticPlanner{plan: &AgentPlan{
		Goal:   "make video",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{ID: "script", Tool: "video_script_generator", Arguments: map[string]interface{}{"topic": "AI workflows"}},
		},
	}}
	catalog := staticToolCatalog{
		"video_script_generator": &tool.ToolManifest{
			Name:     "video_script_generator",
			Endpoint: "builtin://video-creation/video_script_generator",
		},
	}

	runner := NewRunner(orch, store, planner, NewPlanGuard(catalog), NewPlanCompiler(catalog))
	run, err := runner.Start(context.Background(), StartRunRequest{
		UserID:  "user-1",
		Message: "make a video about AI workflows",
		Domain:  "video_creation",
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if run.TaskID != "task-1" || run.Status != RunStatusRunning {
		t.Fatalf("unexpected run: %#v", run)
	}
	if orch.submittedTaskID != "task-1" {
		t.Fatalf("DAG submitted to wrong task: %q", orch.submittedTaskID)
	}
	expectedNodeID := scopedNodeID("task-1", "script")
	if len(orch.submitted.Nodes) != 1 || orch.submitted.Nodes[0].ID != expectedNodeID {
		t.Fatalf("node IDs were not scoped by task: %#v", orch.submitted.Nodes)
	}
	if _, ok := store.runs[run.ID]; !ok {
		t.Fatalf("run was not stored")
	}
}

func TestScopeDAGToTask_RewritesNodeReferences(t *testing.T) {
	scoped := scopeDAGToTask("task-1", &model.DAGRequest{
		Nodes: []model.NodeRequest{
			{ID: "script", Input: map[string]interface{}{"parameters": map[string]interface{}{"brief": "x"}}},
			{ID: "shot", Input: map[string]interface{}{
				"parameters": map[string]interface{}{
					"script": "{{script.output.content}}",
					"nested": []interface{}{"{{script.output.summary}}"},
				},
			}},
		},
		Edges: []model.Edge{{From: "script", To: "shot"}},
	})

	params := scoped.Nodes[1].Input["parameters"].(map[string]interface{})
	expectedScriptID := scopedNodeID("task-1", "script")
	if params["script"] != "{{"+expectedScriptID+".output.content}}" {
		t.Fatalf("reference was not scoped: %#v", params["script"])
	}
	nested := params["nested"].([]interface{})
	if nested[0] != "{{"+expectedScriptID+".output.summary}}" {
		t.Fatalf("nested reference was not scoped: %#v", nested)
	}
}

func TestScopeDAGToTask_KeepsNodeIDsWithinDatabaseLimit(t *testing.T) {
	longStepID := "script_generation_with_reference_content_review_before_and_extra_suffix"
	scoped := scopeDAGToTask("20260623152045-a3f2", &model.DAGRequest{
		Nodes: []model.NodeRequest{{ID: longStepID, Input: map[string]interface{}{}}},
	})
	if got := len(scoped.Nodes[0].ID); got > 64 {
		t.Fatalf("scoped node id length = %d, want <= 64: %q", got, scoped.Nodes[0].ID)
	}
	if scoped.Nodes[0].Input["agentOriginalNodeId"] != longStepID {
		t.Fatalf("original node id not recorded: %#v", scoped.Nodes[0].Input)
	}
}

type staticPlanner struct {
	plan *AgentPlan
}

func (p staticPlanner) GeneratePlan(context.Context, StartRunRequest) (*AgentPlan, error) {
	return p.plan, nil
}

type fakeOrchestrator struct {
	taskID          string
	submittedTaskID string
	submitted       *model.DAGRequest
}

func (o *fakeOrchestrator) CreateTask(context.Context, map[string]interface{}) (*model.Task, error) {
	return &model.Task{ID: o.taskID, Status: model.TaskCreated, CreatedAt: time.Now()}, nil
}

func (o *fakeOrchestrator) SubmitDAG(_ context.Context, taskID string, dag *model.DAGRequest) error {
	o.submittedTaskID = taskID
	o.submitted = dag
	return nil
}

func (o *fakeOrchestrator) GetTaskWithDetails(context.Context, string) (map[string]interface{}, error) {
	return map[string]interface{}{"status": string(model.TaskRunning)}, nil
}

type memoryRunStore struct {
	runs map[string]*Run
}

func newMemoryRunStore() *memoryRunStore {
	return &memoryRunStore{runs: make(map[string]*Run)}
}

func (s *memoryRunStore) SaveRun(_ context.Context, run *Run) error {
	s.runs[run.ID] = run
	return nil
}

func (s *memoryRunStore) FindRun(_ context.Context, id string) (*Run, error) {
	return s.runs[id], nil
}
