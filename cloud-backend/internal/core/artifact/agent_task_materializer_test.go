package artifact

import (
	"context"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

type fakeAgentTaskStore struct {
	tasks []*model.Task
}

func (f fakeAgentTaskStore) FindAgentRuntimeTasksByProject(_ context.Context, projectID string) ([]*model.Task, error) {
	matches := make([]*model.Task, 0, len(f.tasks))
	for _, task := range f.tasks {
		if taskProjectID(task) == projectID {
			matches = append(matches, task)
		}
	}
	return matches, nil
}

type fakeTaskNodeFinder struct {
	nodes map[string][]*model.Node
}

func (f fakeTaskNodeFinder) FindByTaskID(_ context.Context, taskID string) ([]*model.Node, error) {
	return f.nodes[taskID], nil
}

func TestBuildAgentTaskArtifactRequestsMaterializesDynamicRunNodes(t *testing.T) {
	ctx := context.Background()
	tasks := fakeAgentTaskStore{tasks: []*model.Task{
		{
			ID: "task-agent-1",
			Input: map[string]interface{}{
				"source": "agentruntime",
				"context": map[string]interface{}{
					"projectId": "vp-1",
				},
			},
		},
	}}
	nodes := fakeTaskNodeFinder{nodes: map[string][]*model.Node{
		"task-agent-1": {
			{
				ID:     "script_exec",
				TaskID: "task-agent-1",
				Status: model.NodeSuccess,
				Input: map[string]interface{}{
					"stage":       "script",
					"roleAgentId": "script_writer",
				},
				Output: map[string]interface{}{
					"script": "这是一段已生成的口播稿。",
					"artifacts": []interface{}{
						map[string]interface{}{
							"unitId":   "script-content",
							"kind":     "VIDEO_SCRIPT",
							"name":     "口播脚本",
							"mimeType": "text/markdown",
						},
					},
				},
			},
		},
	}}

	requests, err := buildAgentTaskArtifactRequests(ctx, "vp-1", tasks, nodes, nil)
	if err != nil {
		t.Fatalf("dynamic agent task artifacts should materialize: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one artifact request, got %+v", requests)
	}
	req := requests[0]
	if req.ProjectID != "vp-1" || req.WorkflowRunID != "task-agent-1" || req.TaskID != "task-agent-1" {
		t.Fatalf("request should bind dynamic task as project artifact source: %+v", req)
	}
	if req.StageName != "script" || req.Kind != ArtifactKind("VIDEO_SCRIPT") {
		t.Fatalf("unexpected artifact scope: %+v", req)
	}
}
