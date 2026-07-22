package agentruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

type fixedRequestToolResolver struct {
	snapshot *RequestToolSnapshot
	userID   string
	deviceID string
	runnerID string
}

func (r *fixedRequestToolResolver) Resolve(_ context.Context, userID, deviceID, runnerID string) (*RequestToolSnapshot, error) {
	r.userID, r.deviceID, r.runnerID = userID, deviceID, runnerID
	return r.snapshot, nil
}

type chooseDynamicPlanner struct{}

func (chooseDynamicPlanner) GeneratePlan(_ context.Context, req StartRunRequest) (*AgentPlan, error) {
	if req.requestToolSnapshot == nil || req.requestToolSnapshot.GetManifest("studio.render") == nil {
		return nil, context.Canceled
	}
	return &AgentPlan{
		Goal: "render", Domain: "general", Mode: "dynamic_agent",
		Steps:  []AgentStep{{ID: "render", Tool: "studio.render", Arguments: map[string]interface{}{"prompt": "hello"}}},
		Budget: AgentBudget{MaxSteps: 1, MaxToolCalls: 1},
	}, nil
}

type snapshotRepairPlanner struct {
	sawSnapshot bool
}

func (p *snapshotRepairPlanner) GeneratePlan(_ context.Context, _ StartRunRequest) (*AgentPlan, error) {
	return &AgentPlan{
		Goal: "render", Domain: "general", Steps: []AgentStep{{ID: "render", Tool: "studio.render", Arguments: map[string]interface{}{}}},
		Budget: AgentBudget{MaxSteps: 1, MaxToolCalls: 1},
	}, nil
}

func (p *snapshotRepairPlanner) RepairPlanForRequest(_ context.Context, req StartRunRequest, _ *AgentPlan, _ string) (*AgentPlan, error) {
	p.sawSnapshot = req.requestToolSnapshot != nil && req.requestToolSnapshot.GetManifest("studio.render") != nil
	return &AgentPlan{
		Goal: "render", Domain: "general", Steps: []AgentStep{{ID: "render", Tool: "studio.render", Arguments: map[string]interface{}{"prompt": "fixed"}}},
		Budget: AgentBudget{MaxSteps: 1, MaxToolCalls: 1},
	}, nil
}

func TestRunnerUsesOneRequestScopedSnapshotForPlanningGuardAndCompilation(t *testing.T) {
	revision := strings.Repeat("a", 64)
	manifest := &tool.ToolManifest{
		Name: "studio.render", Type: "mcp", Boundary: tool.BoundaryMCPProvider,
		ExecutionPlane: tool.ExecutionPlaneLocal, LocalCommand: "LOCAL_MCP_TOOL_CALL", RequiresUserDevice: true,
		InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"prompt": map[string]interface{}{"type": "string"}}, "required": []interface{}{"prompt"},
		},
		Provider: "studio", ProviderBinding: &tool.ProviderBinding{
			ProviderID: "studio", LogicalToolName: "studio.render", RemoteToolName: "render",
			TargetRunnerID: "runner-a", CatalogRevision: revision, DeviceID: "device-a",
		},
	}
	snapshot := newRequestToolSnapshot([]*tool.ToolManifest{manifest}, []RequestMCPRunnerRevision{{RunnerID: "runner-a", DeviceID: "device-a", Revision: revision}})
	resolver := &fixedRequestToolResolver{snapshot: snapshot}
	orch := &fakeOrchestrator{taskID: "task-dynamic"}
	runner := NewRunner(orch, newMemoryRunStore(), chooseDynamicPlanner{}, NewPlanGuard(nil, nil), NewPlanCompiler(nil)).WithRequestToolResolver(resolver)

	run, err := runner.Start(context.Background(), StartRunRequest{
		UserID: "user-a", DeviceID: "device-a", TargetRunnerID: "runner-a", Message: "render",
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if resolver.userID != "user-a" || resolver.deviceID != "device-a" || resolver.runnerID != "runner-a" {
		t.Fatalf("resolver scope mismatch: %#v", resolver)
	}
	if run.Plan == nil || run.Plan.Steps[0].Tool != "studio.render" {
		t.Fatalf("dynamic plan was not persisted: %#v", run.Plan)
	}
	if orch.submitted == nil {
		t.Fatal("compiled DAG missing")
	}
	if len(orch.submitted.Nodes) != 1 {
		t.Fatalf("compiled nodes=%#v", orch.submitted.Nodes)
	}
	node := orch.submitted.Nodes[0]
	if node.Type != string(model.NodeTypeTool) || node.Name != LocalMCPGatewayToolName || node.Input["agentOriginalNodeId"] != "render" {
		t.Fatalf("unexpected hidden gateway node: %#v", node)
	}
	params := node.Input["parameters"].(map[string]interface{})
	for key, want := range map[string]interface{}{
		"localCommand": "LOCAL_MCP_TOOL_CALL", "targetRunnerId": "runner-a", "catalogRevision": revision,
		"providerId": "studio", "remoteToolName": "render", "logicalToolName": "studio.render",
	} {
		if params[key] != want {
			t.Fatalf("params[%s]=%#v, want %#v; params=%#v", key, params[key], want, params)
		}
	}
	if got := params["arguments"].(map[string]interface{})["prompt"]; got != "hello" {
		t.Fatalf("gateway arguments=%#v", params["arguments"])
	}
}

func TestLLMPlannerReadsDynamicToolsOnlyFromRequestSnapshot(t *testing.T) {
	dynamic := &tool.ToolManifest{
		Name: "studio.render", Description: "render studio video", Type: "mcp",
		Capabilities: []string{"video_creation", "video_render"}, Tags: []string{"studio", "render"},
		InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"prompt": map[string]interface{}{"type": "string"}}, "required": []interface{}{"prompt"},
		},
		ProviderBinding: &tool.ProviderBinding{ProviderID: "studio", RemoteToolName: "render", LogicalToolName: "studio.render"},
	}
	snapshot := newRequestToolSnapshot([]*tool.ToolManifest{dynamic}, nil)
	client := &fakePlannerLLM{response: `{
		"goal":"render studio video","domain":"video_creation","mode":"dynamic_agent",
		"steps":[{"id":"render","tool":"studio.render","arguments":{"prompt":"hello"}}],
		"budget":{"maxToolCalls":1,"maxSteps":1,"maxReplans":0,"maxCostLevel":"medium"},
		"stopPolicy":{"stopWhenEnough":true}
	}`}
	planner := NewLLMPlanner(staticToolList{{
		Name: "global.unrelated", Description: "unrelated global tool", Capabilities: []string{"video_creation"},
	}}, client, LLMPlannerOptions{MaxTools: 1})

	plan, err := planner.GeneratePlan(context.Background(), StartRunRequest{
		Message: "render studio video", Domain: "video_creation", requestToolSnapshot: snapshot,
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned error: %v", err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Tool != "studio.render" {
		t.Fatalf("dynamic tool was not planned: %#v", plan.Steps)
	}
	if !strings.Contains(client.lastUserPrompt, "studio.render") || strings.Contains(client.lastUserPrompt, "global.unrelated") {
		t.Fatalf("planner did not use the immutable request catalog: %s", client.lastUserPrompt)
	}
}

func TestRunnerRepairUsesSameRequestToolSnapshot(t *testing.T) {
	manifest := &tool.ToolManifest{
		Name: "studio.render", Type: "mcp", Boundary: tool.BoundaryMCPProvider,
		ExecutionPlane: tool.ExecutionPlaneLocal, LocalCommand: "LOCAL_MCP_TOOL_CALL",
		InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"prompt": map[string]interface{}{"type": "string"}},
			"required": []interface{}{"prompt"}, "additionalProperties": false,
		},
		ProviderBinding: &tool.ProviderBinding{
			ProviderID: "studio", LogicalToolName: "studio.render", RemoteToolName: "render",
			TargetRunnerID: "runner-a", CatalogRevision: strings.Repeat("a", 64),
		},
	}
	snapshot := newRequestToolSnapshot([]*tool.ToolManifest{manifest}, nil)
	planner := &snapshotRepairPlanner{}
	runner := NewRunner(&fakeOrchestrator{taskID: "task-repair"}, newMemoryRunStore(), planner, NewPlanGuard(nil, nil), NewPlanCompiler(nil)).
		WithRequestToolResolver(&fixedRequestToolResolver{snapshot: snapshot})

	run, err := runner.Start(context.Background(), StartRunRequest{UserID: "user-a", Message: "render"})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if !planner.sawSnapshot || run.Plan.Steps[0].Arguments["prompt"] != "fixed" {
		t.Fatalf("repair did not preserve request snapshot: planner=%#v plan=%#v", planner, run.Plan)
	}
}
