package service

import (
	"context"
	"testing"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/eventbus"
	"github.com/tangying-ai/aios-core/internal/core/localrunner"
	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
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
		Timeout:            1800,
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
				"timeoutSec":      float64(45),
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
	if dispatcher.req.Payload["timeoutSec"] != float64(45) {
		t.Fatalf("payload timeoutSec should preserve render service timeout: %#v", dispatcher.req.Payload)
	}
	if dispatcher.req.TimeoutSec != 90 {
		t.Fatalf("timeoutSec = %d, want render job timeout with fallback buffer 90", dispatcher.req.TimeoutSec)
	}
	if nodeRepo.updatedStatus != model.NodeWaitingLocal {
		t.Fatalf("node status = %s, want WAITING_LOCAL", nodeRepo.updatedStatus)
	}
	if nodeRepo.updatedOutput["localJobId"] != "local_job_001" || nodeRepo.updatedOutput["executionPlane"] != tool.ExecutionPlaneLocal {
		t.Fatalf("node output should include local dispatch metadata: %#v", nodeRepo.updatedOutput)
	}
}

func TestExecuteNodeExternalBridgeDispatchesDelegatedLocalTool(t *testing.T) {
	ctx := context.Background()
	registry := tool.NewToolRegistry()
	registry.RegisterExternal(&tool.ToolManifest{
		Name:               "mcp_generation_runner",
		Type:               "builtin",
		ExecutionPlane:     tool.ExecutionPlaneLocal,
		RequiresUserDevice: true,
		ArtifactLocation:   tool.ArtifactLocationLocal,
		LocalCommand:       "LOCAL_MCP_TOOL_CALL",
		Timeout:            1800,
	})
	nodeRepo := newFakeNodeRepo(&model.Node{
		ID:     "node_mcp_generation",
		TaskID: "task_001",
		Type:   model.NodeTypeTool,
		Status: model.NodeReady,
		Input: map[string]interface{}{
			"tool": "external",
		},
	})
	dispatcher := &fakeLocalJobDispatcher{job: &localrunner.LocalJob{ID: "local_job_mcp_001"}}

	nodeExecutor := NewNodeExecutor(registry, nil, config.WorkerConfig{}, nil, nil, nodeRepo)
	nodeExecutor.SetLocalJobDispatcher(dispatcher)
	nodeExecutor.ExecuteNode(ctx, eventbus.Event{
		TaskID: "task_001",
		NodeID: "node_mcp_generation",
		Type:   string(model.NodeTypeTool),
		Payload: map[string]interface{}{
			"tool": "external",
			"parameters": map[string]interface{}{
				"tool":       "mcp_generation_runner",
				"providerId": "jimeng",
				"mcpTool":    "jimeng.generate_video",
			},
		},
	})

	if dispatcher.req.NodeID != "node_mcp_generation" {
		t.Fatalf("local job was not dispatched: %#v", dispatcher.req)
	}
	if dispatcher.req.ToolName != "mcp_generation_runner" || dispatcher.req.Command != "LOCAL_MCP_TOOL_CALL" {
		t.Fatalf("unexpected delegated local dispatch routing: %#v", dispatcher.req)
	}
	if dispatcher.req.Payload["providerId"] != "jimeng" || dispatcher.req.Payload["mcpTool"] != "jimeng.generate_video" {
		t.Fatalf("delegated local payload not preserved: %#v", dispatcher.req.Payload)
	}
	if nodeRepo.updatedStatus != model.NodeWaitingLocal {
		t.Fatalf("node status = %s, want WAITING_LOCAL", nodeRepo.updatedStatus)
	}
	if nodeRepo.updatedOutput["localJobId"] != "local_job_mcp_001" || nodeRepo.updatedOutput["toolName"] != "mcp_generation_runner" {
		t.Fatalf("node output should include delegated local dispatch metadata: %#v", nodeRepo.updatedOutput)
	}
}

func TestExecuteNodeExternalBridgeResolvesProjectIDFromTask(t *testing.T) {
	ctx := context.Background()
	registry := tool.NewToolRegistry()
	registry.RegisterExternal(&tool.ToolManifest{
		Name:           "mcp_generation_runner",
		Type:           "builtin",
		ExecutionPlane: tool.ExecutionPlaneLocal,
		LocalCommand:   "LOCAL_MCP_TOOL_CALL",
	})
	nodeRepo := newFakeNodeRepo(&model.Node{
		ID:     "node_mcp_generation",
		TaskID: "task_001",
		Type:   model.NodeTypeTool,
		Status: model.NodeReady,
		Input:  map[string]interface{}{"tool": "external"},
	})
	dispatcher := &fakeLocalJobDispatcher{job: &localrunner.LocalJob{ID: "local_job_mcp_001"}}

	nodeExecutor := NewNodeExecutor(registry, nil, config.WorkerConfig{}, nil, nil, nodeRepo)
	nodeExecutor.SetLocalJobDispatcher(dispatcher)
	nodeExecutor.SetProjectIDResolver(fakeProjectIDResolver{projectID: "vp-resolved"})
	nodeExecutor.ExecuteNode(ctx, eventbus.Event{
		TaskID: "task_001",
		NodeID: "node_mcp_generation",
		Type:   string(model.NodeTypeTool),
		Payload: map[string]interface{}{
			"tool": "external",
			"parameters": map[string]interface{}{
				"tool":       "mcp_generation_runner",
				"providerId": "jimeng",
			},
		},
	})

	if dispatcher.req.ProjectID != "vp-resolved" {
		t.Fatalf("local dispatch should resolve project ID from task context, got %#v", dispatcher.req)
	}
}

func TestExecuteNodeLocalToolUsesRetryAttemptInIdempotencyKey(t *testing.T) {
	ctx := context.Background()
	registry := tool.NewToolRegistry()
	registry.RegisterExternal(&tool.ToolManifest{
		Name:           "hyperframes_renderer",
		ExecutionPlane: tool.ExecutionPlaneLocal,
		LocalCommand:   "HYPERFRAMES_RENDER",
	})
	nodeRepo := newFakeNodeRepo(&model.Node{
		ID:         "render_exec",
		TaskID:     "task_001",
		Type:       model.NodeTypeTool,
		Status:     model.NodeReady,
		RetryCount: 1,
		Input:      map[string]interface{}{"tool": "hyperframes_renderer"},
	})
	dispatcher := &fakeLocalJobDispatcher{job: &localrunner.LocalJob{ID: "local_job_retry_002"}}

	nodeExecutor := NewNodeExecutor(registry, nil, config.WorkerConfig{}, nil, nil, nodeRepo)
	nodeExecutor.SetLocalJobDispatcher(dispatcher)
	nodeExecutor.ExecuteNode(ctx, eventbus.Event{
		TaskID: "task_001",
		NodeID: "render_exec",
		Type:   string(model.NodeTypeTool),
		Payload: map[string]interface{}{
			"tool": "hyperframes_renderer",
			"parameters": map[string]interface{}{
				"projectDir":      "local://projects/project_001/hyperframes",
				"previewApproved": true,
			},
		},
	})

	if dispatcher.req.IdempotencyKey != "task_001-render_exec-attempt-2" {
		t.Fatalf("retry dispatch should use an attempt-scoped idempotency key, got %#v", dispatcher.req)
	}
}

func TestResolveSingleRefReadsStructuredStdoutContent(t *testing.T) {
	ctx := context.Background()
	nodeRepo := newFakeNodeRepo(&model.Node{
		ID:     "task_001-beat_plan_exec",
		TaskID: "task_001",
		Output: map[string]interface{}{
			"stdout": `{"content":"{\"shotList\":[{\"shotId\":\"SHOT_01\",\"durationSec\":8}],\"summary\":\"ok\"}"}`,
		},
	})

	resolved, ok := resolveSingleRef(ctx, nodeRepo, "task_001", "{{beat_plan.output.shotList}}")
	if !ok {
		t.Fatalf("reference should resolve")
	}
	shots, ok := resolved.([]interface{})
	if !ok || len(shots) != 1 {
		t.Fatalf("shotList should resolve as typed array, got %#v", resolved)
	}
	shot, ok := shots[0].(map[string]interface{})
	if !ok || shot["shotId"] != "SHOT_01" {
		t.Fatalf("unexpected shot payload: %#v", resolved)
	}
}

func TestResolveSingleRefPrefersFuzzyExecNodeWithRequestedField(t *testing.T) {
	ctx := context.Background()
	nodeRepo := newFakeNodeRepo(
		&model.Node{
			ID:     "task_001-beat_plan_review",
			TaskID: "task_001",
			Output: map[string]interface{}{
				"approved": true,
			},
		},
		&model.Node{
			ID:     "task_001-beat_plan_exec",
			TaskID: "task_001",
			Output: map[string]interface{}{
				"stdout": `{"content":"{\"shotList\":[{\"shotId\":\"SHOT_02\",\"durationSec\":9}]}"}`,
			},
		},
	)

	resolved, ok := resolveSingleRef(ctx, nodeRepo, "task_001", "{{beat_plan.output.shotList}}")
	if !ok {
		t.Fatalf("reference should resolve from exec node, not review node")
	}
	shots, ok := resolved.([]interface{})
	if !ok || len(shots) != 1 {
		t.Fatalf("shotList should resolve as typed array, got %#v", resolved)
	}
	shot := shots[0].(map[string]interface{})
	if shot["shotId"] != "SHOT_02" {
		t.Fatalf("unexpected shot payload: %#v", resolved)
	}
}

func TestResolveSingleRefReadsLocalVideoArtifactPath(t *testing.T) {
	ctx := context.Background()
	nodeRepo := newFakeNodeRepo(&model.Node{
		ID:     "task_001-render_exec",
		TaskID: "task_001",
		Output: map[string]interface{}{
			"outputRef": "local://projects/vp-001/artifacts/final-video/hash/final.mp4",
			"artifacts": []interface{}{
				map[string]interface{}{
					"kind":       "VIDEO",
					"storageRef": "local://projects/vp-001/artifacts/final-video/hash/final.mp4",
					"metadata": map[string]interface{}{
						"localPath": "/tmp/tangying-final.mp4",
						"width":     1920,
						"height":    1080,
					},
				},
			},
		},
	})

	resolved, ok := resolveSingleRef(ctx, nodeRepo, "task_001", "{{render.output.outputPath}}")
	if !ok {
		t.Fatalf("reference should resolve from local VIDEO artifact")
	}
	if resolved != "/tmp/tangying-final.mp4" {
		t.Fatalf("outputPath should resolve to artifact metadata.localPath, got %#v", resolved)
	}

	resolved, ok = resolveSingleRef(ctx, nodeRepo, "task_001", "{{render.output.finalVideo}}")
	if !ok {
		t.Fatalf("finalVideo reference should resolve from local VIDEO artifact")
	}
	if resolved != "/tmp/tangying-final.mp4" {
		t.Fatalf("finalVideo should resolve to artifact metadata.localPath, got %#v", resolved)
	}
}

func TestExecutableToolTimeoutUsesDelegatedExternalManifest(t *testing.T) {
	registry := tool.NewToolRegistry()
	registry.RegisterExternal(&tool.ToolManifest{
		Name:    "hyperframes_renderer",
		Timeout: 1800,
	})

	nodeExecutor := NewNodeExecutor(registry, nil, config.WorkerConfig{ToolTimeoutSeconds: 120}, nil, nil, nil)
	timeout := nodeExecutor.executableToolTimeout("external", map[string]interface{}{
		"tool": "hyperframes_renderer",
	}, &tool.ToolManifest{Name: "external"})

	if timeout != 1800*time.Second {
		t.Fatalf("expected delegated hyperframes_renderer timeout, got %s", timeout)
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

type fakeProjectIDResolver struct {
	projectID string
}

func (f fakeProjectIDResolver) ResolveProjectID(_ context.Context, _ string) (string, error) {
	return f.projectID, nil
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
