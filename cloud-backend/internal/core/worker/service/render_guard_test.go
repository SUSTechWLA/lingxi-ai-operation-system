package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/eventbus"
	"github.com/tangying-ai/aios-core/internal/core/localrunner"
	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestExecuteNodeLocalRenderChecksDependenciesBeforeDispatch(t *testing.T) {
	ctx := context.Background()
	registry := tool.NewToolRegistry()
	registry.RegisterExternal(&tool.ToolManifest{
		Name:           "hyperframes_renderer",
		Type:           "local_tool",
		ExecutionPlane: tool.ExecutionPlaneLocal,
		LocalCommand:   localrunner.CommandHyperFramesRender,
	})
	nodeRepo := newFakeNodeRepo(&model.Node{
		ID:     "render_exec",
		TaskID: "task_001",
		Type:   model.NodeTypeTool,
		Status: model.NodeReady,
		Input:  map[string]interface{}{"tool": "hyperframes_renderer"},
	})
	dispatcher := &fakeLocalJobDispatcher{job: &localrunner.LocalJob{ID: "local_job_001"}}
	checker := &fakeRenderDependencyChecker{err: errors.New("RENDER_DEPENDENCY_MISSING: 预览尚未确认，禁止开始最终渲染")}

	nodeExecutor := NewNodeExecutor(registry, nil, config.WorkerConfig{}, nil, nil, nodeRepo)
	nodeExecutor.SetLocalJobDispatcher(dispatcher)
	nodeExecutor.SetRenderDependencyChecker(checker)
	nodeExecutor.ExecuteNode(ctx, eventbus.Event{
		TaskID: "task_001",
		NodeID: "render_exec",
		Type:   string(model.NodeTypeTool),
		Payload: map[string]interface{}{
			"tool":       "hyperframes_renderer",
			"parameters": map[string]interface{}{"projectId": "project_1"},
		},
	})

	if checker.req.NodeID != "render_exec" || checker.req.Command != localrunner.CommandHyperFramesRender {
		t.Fatalf("render checker not called with dispatch context: %#v", checker.req)
	}
	if dispatcher.req.NodeID != "" {
		t.Fatalf("render job should not dispatch when dependency guard fails: %#v", dispatcher.req)
	}
	if nodeRepo.updatedStatus != "" {
		t.Fatalf("node should not be marked waiting local after guard failure: %s", nodeRepo.updatedStatus)
	}
}

type fakeRenderDependencyChecker struct {
	req RenderDependencyCheckRequest
	err error
}

func (f *fakeRenderDependencyChecker) CheckRenderDependencies(_ context.Context, req RenderDependencyCheckRequest) error {
	f.req = req
	return f.err
}

func TestDefaultRenderDependencyCheckerRejectsUnapprovedPreview(t *testing.T) {
	err := DefaultRenderDependencyChecker{}.CheckRenderDependencies(context.Background(), RenderDependencyCheckRequest{
		Command: localrunner.CommandHyperFramesRender,
		Payload: map[string]interface{}{
			"previewApproved": false,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "RENDER_DEPENDENCY_MISSING") {
		t.Fatalf("expected render dependency missing error, got %v", err)
	}
}
