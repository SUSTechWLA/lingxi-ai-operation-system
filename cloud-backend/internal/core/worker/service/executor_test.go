package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/eventbus"
	"github.com/tangying-ai/aios-core/internal/core/localrunner"
	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool/builtin"
)

func TestExecuteNodeLocalToolCreatesLocalJobAndWaits(t *testing.T) {
	ctx := context.Background()
	registry := tool.NewToolRegistry()
	registry.RegisterExternal(&tool.ToolManifest{
		Name:               "hyperframes_renderer",
		Type:               "local_tool",
		ExecutionPlane:     tool.ExecutionPlaneLocal,
		RequiresUserDevice: true,
		ArtifactLocation:   tool.ArtifactLocationLocal,
		LocalCommand:       "HYPERFRAMES_RENDER",
		ArtifactPolicy: tool.ArtifactPolicy{
			ProduceArtifact: true,
			ArtifactKinds:   []string{"VIDEO"},
		},
	})
	nodeRepo := newFakeNodeRepo(&model.Node{
		ID:     "node_hyperframes_render",
		TaskID: "task_001",
		Type:   model.NodeTypeTool,
		Status: model.NodeReady,
		Input: map[string]interface{}{
			"tool": "hyperframes_renderer",
		},
	})
	dispatcher := &fakeLocalJobDispatcher{job: &localrunner.LocalJob{ID: "local_job_001"}}

	nodeExecutor := NewNodeExecutor(registry, nil, config.WorkerConfig{}, nil, nil, nodeRepo)
	nodeExecutor.SetLocalJobDispatcher(dispatcher)
	nodeExecutor.ExecuteNode(ctx, eventbus.Event{
		TaskID: "task_001",
		NodeID: "node_hyperframes_render",
		Type:   string(model.NodeTypeTool),
		Payload: map[string]interface{}{
			"tool": "hyperframes_renderer",
			"parameters": map[string]interface{}{
				"projectDir":      "local://projects/project_001/hyperframes",
				"previewApproved": true,
				"fps":             float64(30),
			},
		},
	})

	if dispatcher.req.NodeID != "node_hyperframes_render" {
		t.Fatalf("local job was not dispatched: %#v", dispatcher.req)
	}
	if dispatcher.req.ToolName != "hyperframes_renderer" || dispatcher.req.Command != "HYPERFRAMES_RENDER" {
		t.Fatalf("unexpected local dispatch routing: %#v", dispatcher.req)
	}
	if dispatcher.req.Payload["projectDir"] != "local://projects/project_001/hyperframes" {
		t.Fatalf("parameters not preserved in local job payload: %#v", dispatcher.req.Payload)
	}
	if nodeRepo.updatedStatus != model.NodeWaitingLocal {
		t.Fatalf("node status = %s, want WAITING_LOCAL", nodeRepo.updatedStatus)
	}
	if nodeRepo.updatedOutput["localJobId"] != "local_job_001" || nodeRepo.updatedOutput["executionPlane"] != tool.ExecutionPlaneLocal {
		t.Fatalf("node output should include local dispatch metadata: %#v", nodeRepo.updatedOutput)
	}
}

func TestExecuteToolUsesExternalManifestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(1500 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"report_path":"E:/bid/out/analysis.md"}}`))
	}))
	defer server.Close()

	registry := tool.NewToolRegistry()
	registry.Register(builtin.NewExternalTool(registry))
	registry.RegisterExternal(&tool.ToolManifest{
		Name:     "parse_bid_files",
		Type:     "http",
		Endpoint: server.URL,
		Timeout:  2,
	})

	nodeExecutor := NewNodeExecutor(registry, nil, config.WorkerConfig{ToolTimeoutSeconds: 1}, nil, nil, nil)
	result, err := nodeExecutor.executeTool(
		context.Background(),
		"external",
		map[string]interface{}{"tool": "parse_bid_files", "file_path": "E:/bid/test_bid.txt"},
		tool.ToolContext{TaskID: "task-1", NodeID: "node-1"},
		false,
		nil,
	)

	if err != nil {
		t.Fatalf("executeTool returned error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("external manifest timeout should allow tool to finish; error=%q", result.Error)
	}
	var output map[string]interface{}
	if err := json.Unmarshal(result.Stdout, &output); err != nil {
		t.Fatalf("stdout should be JSON output: %v", err)
	}
	data, _ := output["data"].(map[string]interface{})
	if data["report_path"] != "E:/bid/out/analysis.md" {
		t.Fatalf("unexpected output: %#v", output)
	}
}

type fakeLocalJobDispatcher struct {
	req localrunner.DispatchLocalJobRequest
	job *localrunner.LocalJob
}

func (f *fakeLocalJobDispatcher) DispatchLocalJob(_ context.Context, req localrunner.DispatchLocalJobRequest) (*localrunner.LocalJob, error) {
	f.req = req
	return f.job, nil
}

type fakeNodeRepo struct {
	nodes         map[string]*model.Node
	updatedStatus model.NodeStatus
	updatedOutput map[string]interface{}
}

func newFakeNodeRepo(nodes ...*model.Node) *fakeNodeRepo {
	repo := &fakeNodeRepo{nodes: map[string]*model.Node{}}
	for _, node := range nodes {
		repo.nodes[node.ID] = node
	}
	return repo
}

func (f *fakeNodeRepo) FindByID(_ context.Context, id string) (*model.Node, error) {
	return f.nodes[id], nil
}

func (f *fakeNodeRepo) FindByTaskID(_ context.Context, taskID string) ([]*model.Node, error) {
	var nodes []*model.Node
	for _, node := range f.nodes {
		if node.TaskID == taskID {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func (f *fakeNodeRepo) FindByStatus(_ context.Context, status model.NodeStatus) ([]*model.Node, error) {
	var nodes []*model.Node
	for _, node := range f.nodes {
		if node.Status == status {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func (f *fakeNodeRepo) FindChildNodes(_ context.Context, _ string) ([]*model.Node, error) {
	return nil, nil
}

func (f *fakeNodeRepo) Save(_ context.Context, node *model.Node) error {
	f.nodes[node.ID] = node
	return nil
}

func (f *fakeNodeRepo) UpdateStatus(_ context.Context, id string, status model.NodeStatus, output map[string]interface{}, errMsg string) error {
	f.updatedStatus = status
	f.updatedOutput = output
	if node := f.nodes[id]; node != nil {
		node.Status = status
		node.Output = output
		node.ErrorMessage = errMsg
	}
	return nil
}

func (f *fakeNodeRepo) FindStaleRunningNodes(_ context.Context, _ int) ([]*model.Node, error) {
	return nil, nil
}

func (f *fakeNodeRepo) UpdateHeartbeat(_ context.Context, _ string, _ float64, _ string) error {
	return nil
}

var _ = time.Second
