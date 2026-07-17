package agentruntime

import (
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestPlanGuardRejectsExternalAPIWhenUserDisallowsWeb(t *testing.T) {
	guard := NewPlanGuard(guardPolicyCatalog(), nil)
	plan := singleStepGuardPlan("external_api", &KnowledgePolicy{
		ContentType:           "marketing_script",
		FreshnessLevel:        FreshnessNone,
		RetrievalPolicy:       RetrievalNone,
		ForbiddenCapabilities: append(FreshKnowledgeCapabilities(), "external_api"),
		Reason:                "user explicitly disabled web access",
	})

	err := guard.Validate(plan)
	if err == nil {
		t.Fatal("expected guard to reject external API when no-web policy is active")
	}
	if !strings.Contains(err.Error(), "external_api") {
		t.Fatalf("error should mention external_api, got %v", err)
	}
}

func TestPlanGuardRejectsPublishWithoutApproval(t *testing.T) {
	guard := NewPlanGuard(guardPolicyCatalog(), nil)
	plan := singleStepGuardPlan("publish.video", nil)

	err := guard.Validate(plan)
	if err == nil {
		t.Fatal("expected guard to reject publish without approval")
	}
	if !strings.Contains(err.Error(), "approval") {
		t.Fatalf("error should mention approval, got %v", err)
	}
}

func TestPlanGuardRejectsDeleteWithoutApproval(t *testing.T) {
	guard := NewPlanGuard(guardPolicyCatalog(), nil)
	plan := singleStepGuardPlan("delete.artifact", nil)

	err := guard.Validate(plan)
	if err == nil {
		t.Fatal("expected guard to reject delete without approval")
	}
	if !strings.Contains(err.Error(), "approval") {
		t.Fatalf("error should mention approval, got %v", err)
	}
}

func TestPlanGuardRejectsHighCostAIGCWithoutApproval(t *testing.T) {
	guard := NewPlanGuard(guardPolicyCatalog(), nil)
	plan := singleStepGuardPlan("video.generate_aigc", nil)
	plan.Budget.MaxCostLevel = ""

	err := guard.Validate(plan)
	if err == nil {
		t.Fatal("expected guard to reject high cost AIGC without approval or budget gate")
	}
	if !strings.Contains(err.Error(), "high-cost") {
		t.Fatalf("error should mention high-cost, got %v", err)
	}
}

func TestPlanGuardRejectsFileWriteWithoutArtifactPolicy(t *testing.T) {
	guard := NewPlanGuard(guardPolicyCatalog(), nil)
	plan := singleStepGuardPlan("artifact.raw_write", nil)

	err := guard.Validate(plan)
	if err == nil {
		t.Fatal("expected guard to reject unsafe file write")
	}
	if !strings.Contains(err.Error(), "file_write") {
		t.Fatalf("error should mention file_write, got %v", err)
	}
}

func TestPlanGuardRejectsDisabledMCPProvider(t *testing.T) {
	guard := NewPlanGuard(guardPolicyCatalog(), nil)
	plan := singleStepGuardPlan("jimeng.generate_video", nil)

	err := guard.Validate(plan)
	if err == nil {
		t.Fatal("expected guard to reject disabled MCP provider")
	}
	if !strings.Contains(err.Error(), "provider") {
		t.Fatalf("error should mention provider, got %v", err)
	}
}

func TestPlanGuardRejectsDisabledMCPTool(t *testing.T) {
	catalog := guardPolicyCatalog()
	m := *catalog["jimeng.generate_video"]
	m.ProviderCapabilities = map[string]interface{}{"enabled": true, "disabledTools": []interface{}{"jimeng.generate_video"}}
	catalog["jimeng.generate_video"] = &m

	guard := NewPlanGuard(catalog, nil)
	plan := singleStepGuardPlan("jimeng.generate_video", nil)

	err := guard.Validate(plan)
	if err == nil {
		t.Fatal("expected guard to reject disabled MCP tool")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("error should mention disabled tool, got %v", err)
	}
}

func singleStepGuardPlan(toolName string, policy *KnowledgePolicy) *AgentPlan {
	return &AgentPlan{
		Goal:            "guard policy test",
		Domain:          "video_creation",
		Mode:            "dynamic_agent",
		KnowledgePolicy: policy,
		Steps: []AgentStep{
			{ID: "step", Tool: toolName, Arguments: map[string]interface{}{}},
		},
		Budget: AgentBudget{MaxSteps: 1, MaxToolCalls: 1, MaxCostLevel: tool.CostHigh},
	}
}

func guardPolicyCatalog() staticToolCatalog {
	return staticToolCatalog{
		"external_api": {
			Name:         "external_api",
			Type:         "http",
			Boundary:     tool.BoundaryRemoteHTTP,
			Description:  "Call an external API.",
			Capabilities: []string{"external_api", "fact_retrieval"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskMedium,
			Parameters:   map[string]tool.ParamDef{},
		},
		"publish.video": {
			Name:         "publish.video",
			Type:         "http",
			Boundary:     tool.BoundaryRemoteHTTP,
			Description:  "Publish a finished video.",
			Capabilities: []string{"platform_publish"},
			SideEffect:   true,
			RiskLevel:    tool.RiskHigh,
			CostLevel:    tool.CostLow,
			Parameters:   map[string]tool.ParamDef{},
		},
		"delete.artifact": {
			Name:         "delete.artifact",
			Type:         "builtin",
			Boundary:     tool.BoundaryCloudBuiltin,
			Description:  "Delete an artifact.",
			Capabilities: []string{"artifact_delete"},
			SideEffect:   true,
			RiskLevel:    tool.RiskHigh,
			CostLevel:    tool.CostLow,
			Parameters:   map[string]tool.ParamDef{},
		},
		"video.generate_aigc": {
			Name:         "video.generate_aigc",
			Type:         "mcp",
			Boundary:     tool.BoundaryMCPProvider,
			Description:  "Generate an AIGC video through a provider.",
			Capabilities: []string{"aigc_generation", "video_generation"},
			CostLevel:    tool.CostHigh,
			RiskLevel:    tool.RiskMedium,
			Parameters:   map[string]tool.ParamDef{},
		},
		"artifact.raw_write": {
			Name:         "artifact.raw_write",
			Type:         "builtin",
			Boundary:     tool.BoundaryLocalNative,
			Description:  "Write a file to disk.",
			Capabilities: []string{"file_write"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskMedium,
			Parameters:   map[string]tool.ParamDef{},
		},
		"jimeng.generate_video": {
			Name:               "jimeng.generate_video",
			Type:               "mcp",
			Boundary:           tool.BoundaryMCPProvider,
			Description:        "Generate video through JiMeng MCP.",
			ExecutionPlane:     tool.ExecutionPlaneLocal,
			RequiresUserDevice: true,
			LocalCommand:       "LOCAL_MCP_TOOL_CALL",
			Provider:           "jimeng",
			ProviderBinding: &tool.ProviderBinding{
				ProviderID:      "jimeng",
				RemoteToolName:  "generate_video",
				LogicalToolName: "jimeng.generate_video",
				ToolPrefix:      "jimeng.",
			},
			ProviderCapabilities: map[string]interface{}{"enabled": false},
			Capabilities:         []string{"aigc_generation", "video_generation"},
			CostLevel:            tool.CostMedium,
			RiskLevel:            tool.RiskMedium,
			Parameters:           map[string]tool.ParamDef{},
		},
	}
}
