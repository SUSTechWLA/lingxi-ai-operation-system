package workflow

import (
	"encoding/json"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/skillruntime"
)

func TestCompileSkillToDAG_SimpleLinear(t *testing.T) {
	skill := &skillruntime.SkillManifest{
		Name:    "test-skill",
		Version: "1.0.0",
		Stages: []skillruntime.StageDefinition{
			{Name: "brief", Instruction: "stages/brief.md"},
			{Name: "script", Instruction: "stages/script.md"},
			{Name: "generate", Instruction: "stages/generate.md"},
		},
	}

	dag, err := CompileSkillToDAG(skill)
	if err != nil {
		t.Fatal(err)
	}

	var parsed dagRequest
	if err := json.Unmarshal(dag, &parsed); err != nil {
		t.Fatal(err)
	}

	if len(parsed.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d: %+v", len(parsed.Nodes), parsed.Nodes)
	}
	if len(parsed.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(parsed.Edges))
	}

	// Verify chain: brief → script → generate
	if parsed.Edges[0].From != "brief" || parsed.Edges[0].To != "script" {
		t.Errorf("edge 0 should be brief→script, got %s→%s", parsed.Edges[0].From, parsed.Edges[0].To)
	}
	if parsed.Edges[1].From != "script" || parsed.Edges[1].To != "generate" {
		t.Errorf("edge 1 should be script→generate, got %s→%s", parsed.Edges[1].From, parsed.Edges[1].To)
	}

	// Verify nodes have stage input
	for _, node := range parsed.Nodes {
		if node.Input["stage"] == nil {
			t.Errorf("node %s missing stage in input", node.ID)
		}
	}
}

func TestCompileSkillToDAG_DefaultAgentStageUsesExternalSkillAgent(t *testing.T) {
	skill := &skillruntime.SkillManifest{
		Name:    "create-opinion-videos",
		Version: "1.0.0",
		Stages: []skillruntime.StageDefinition{
			{Name: "recording_script", Instruction: "stages/recording_script.md"},
		},
	}

	dag, err := CompileSkillToDAG(skill)
	if err != nil {
		t.Fatal(err)
	}

	var parsed dagRequest
	if err := json.Unmarshal(dag, &parsed); err != nil {
		t.Fatal(err)
	}

	node := parsed.Nodes[0]
	if node.Type != "TOOL" || node.Name != "external" {
		t.Fatalf("expected stage to route through external tool bridge, got %s/%s", node.Type, node.Name)
	}
	params, ok := node.Input["parameters"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected parameters map in node input: %+v", node.Input)
	}
	if params["tool"] != "skill_stage_agent" {
		t.Errorf("expected default external tool skill_stage_agent, got %v", params["tool"])
	}
	if params["skill_name"] != "create-opinion-videos" || params["skill_version"] != "1.0.0" {
		t.Errorf("expected skill identity in parameters, got %+v", params)
	}
}

func TestCompileSkillToDAG_ToolStageRoutesToDeclaredExternalTool(t *testing.T) {
	skill := &skillruntime.SkillManifest{
		Name:    "voice-post-production",
		Version: "1.0.0",
		Stages: []skillruntime.StageDefinition{
			{
				Name:                "voice_process",
				Kind:                "TOOL",
				Tool:                "voice_post_process",
				Instruction:         "stages/voice_process.md",
				LongRunning:         true,
				HeartbeatTimeoutSec: 900,
			},
		},
	}

	dag, err := CompileSkillToDAG(skill)
	if err != nil {
		t.Fatal(err)
	}

	var parsed dagRequest
	if err := json.Unmarshal(dag, &parsed); err != nil {
		t.Fatal(err)
	}

	node := parsed.Nodes[0]
	if node.Name != "external" {
		t.Fatalf("expected external bridge node, got %s", node.Name)
	}
	params := node.Input["parameters"].(map[string]interface{})
	if params["tool"] != "voice_post_process" {
		t.Errorf("expected voice_post_process tool, got %v", params["tool"])
	}
	if !node.LongRunning {
		t.Error("expected long-running metadata on tool stage")
	}
	if node.HeartbeatTimeoutSec == nil || *node.HeartbeatTimeoutSec != 900 {
		t.Errorf("expected heartbeat timeout 900, got %v", node.HeartbeatTimeoutSec)
	}
}

func TestCompileSkillToDAG_WithApproval(t *testing.T) {
	skill := &skillruntime.SkillManifest{
		Name:    "test-approval",
		Version: "1.0.0",
		Stages: []skillruntime.StageDefinition{
			{Name: "brief", Instruction: "stages/brief.md"},
			{Name: "script", Instruction: "stages/script.md", ApprovalReq: true},
			{Name: "generate", Instruction: "stages/generate.md"},
		},
	}

	dag, err := CompileSkillToDAG(skill)
	if err != nil {
		t.Fatal(err)
	}

	var parsed dagRequest
	json.Unmarshal(dag, &parsed)

	// Should have: brief, script_exec, script(CONTROL), generate = 4 nodes
	if len(parsed.Nodes) != 4 {
		t.Errorf("expected 4 nodes, got %d", len(parsed.Nodes))
	}

	// Verify CONTROL node exists
	foundControl := false
	for _, node := range parsed.Nodes {
		if node.Type == "CONTROL" && node.ID == "script" {
			foundControl = true
		}
	}
	if !foundControl {
		t.Error("expected CONTROL node 'script' for approval_required stage")
	}

	// Verify exec→control edge
	foundEdge := false
	for _, edge := range parsed.Edges {
		if edge.From == "script_exec" && edge.To == "script" {
			foundEdge = true
		}
	}
	if !foundEdge {
		t.Error("expected edge script_exec→script")
	}
}

func TestCompileSkillToDAG_OptionalStage(t *testing.T) {
	skill := &skillruntime.SkillManifest{
		Name:    "test-optional",
		Version: "1.0.0",
		Stages: []skillruntime.StageDefinition{
			{Name: "brief", Instruction: "stages/brief.md"},
			{Name: "visual_design", Instruction: "stages/visual.md", Optional: true, ApprovalReq: true},
			{Name: "generate", Instruction: "stages/generate.md"},
		},
	}

	dag, err := CompileSkillToDAG(skill)
	if err != nil {
		t.Fatal(err)
	}

	var parsed dagRequest
	json.Unmarshal(dag, &parsed)

	// Nodes: brief, visual_design_skip(CONTROL), visual_design_exec(TOOL), visual_design(CONTROL), generate = 5
	if len(parsed.Nodes) < 5 {
		t.Errorf("expected at least 5 nodes, got %d", len(parsed.Nodes))
	}

	// Verify skip node exists
	foundSkip := false
	foundExec := false
	for _, node := range parsed.Nodes {
		if node.ID == "visual_design_skip" && node.Type == "CONTROL" {
			foundSkip = true
		}
		if node.ID == "visual_design_exec" && node.Type == "TOOL" {
			foundExec = true
		}
	}
	if !foundSkip {
		t.Error("expected skip CONTROL node for optional stage")
	}
	if !foundExec {
		t.Error("expected exec TOOL node for optional stage")
	}

	// Verify skip and approval both connect to generate
	foundSkipToGen := false
	foundApprovalToGen := false
	for _, edge := range parsed.Edges {
		if edge.From == "visual_design_skip" && edge.To == "generate" {
			foundSkipToGen = true
		}
		if edge.From == "visual_design" && edge.To == "generate" {
			foundApprovalToGen = true
		}
	}
	if !foundSkipToGen {
		t.Error("expected edge visual_design_skip→generate")
	}
	if !foundApprovalToGen {
		t.Error("expected edge visual_design→generate")
	}
}

func TestCompileSkillToDAG_OptionalStageExecDependsOnPreviousOutput(t *testing.T) {
	skill := &skillruntime.SkillManifest{
		Name:    "test-optional-dependencies",
		Version: "1.0.0",
		Stages: []skillruntime.StageDefinition{
			{Name: "brief", Instruction: "stages/brief.md"},
			{Name: "image_assets", Instruction: "stages/assets.md", Optional: true, ApprovalReq: true},
			{Name: "render", Instruction: "stages/render.md"},
		},
	}

	dag, err := CompileSkillToDAG(skill)
	if err != nil {
		t.Fatal(err)
	}

	var parsed dagRequest
	if err := json.Unmarshal(dag, &parsed); err != nil {
		t.Fatal(err)
	}

	hasBriefToExec := false
	hasBriefToApproval := false
	for _, edge := range parsed.Edges {
		if edge.From == "brief" && edge.To == "image_assets_exec" {
			hasBriefToExec = true
		}
		if edge.From == "brief" && edge.To == "image_assets" {
			hasBriefToApproval = true
		}
	}
	if !hasBriefToExec {
		t.Error("expected previous output to gate optional exec branch")
	}
	if hasBriefToApproval {
		t.Error("previous output should not bypass optional exec by connecting directly to approval")
	}
}

func TestCompileSkillToDAG_NilSkill(t *testing.T) {
	_, err := CompileSkillToDAG(nil)
	if err == nil {
		t.Error("expected error for nil skill")
	}
}

func TestCompileSkillToDAG_EmptyStages(t *testing.T) {
	_, err := CompileSkillToDAG(&skillruntime.SkillManifest{Stages: nil})
	if err == nil {
		t.Error("expected error for empty stages")
	}
}

func TestIsSkipToExec(t *testing.T) {
	if !isSkipToExec("visual_design_skip", "visual_design_exec") {
		t.Error("isSkipToExec should return true for _skip->_exec")
	}
	if isSkipToExec("brief", "script") {
		t.Error("isSkipToExec should return false for unrelated nodes")
	}
}

func TestCompileSkillToDAG_RealSkillAIGC(t *testing.T) {
	// Simulate the real aigc-shot-video skill (simplified)
	skill := &skillruntime.SkillManifest{
		Name:    "aigc-shot-video",
		Version: "1.0.0",
		Stages: []skillruntime.StageDefinition{
			{Name: "brief", Instruction: "stages/brief.md"},
			{Name: "script", Instruction: "stages/script.md"},
			{Name: "shot_plan", Instruction: "stages/shot_plan.md"},
			{Name: "storyboard", Instruction: "stages/storyboard.md", Optional: true, ApprovalReq: true},
			{Name: "keyframe", Instruction: "stages/keyframe.md", ApprovalReq: true},
			{Name: "generate", Instruction: "stages/generate.md"},
			{Name: "review", Instruction: "stages/review.md", ApprovalReq: true},
		},
	}

	dag, err := CompileSkillToDAG(skill)
	if err != nil {
		t.Fatal(err)
	}

	var parsed dagRequest
	if err := json.Unmarshal(dag, &parsed); err != nil {
		t.Fatal(err)
	}

	// Verify JSON output is valid
	if len(parsed.Nodes) == 0 {
		t.Error("expected non-empty nodes")
	}
	if len(parsed.Edges) == 0 {
		t.Error("expected non-empty edges")
	}

	// Verify all node IDs are unique
	nodeIDs := make(map[string]bool)
	for _, node := range parsed.Nodes {
		if nodeIDs[node.ID] {
			t.Errorf("duplicate node ID: %s", node.ID)
		}
		nodeIDs[node.ID] = true
	}

	// Verify all edges reference existing nodes
	for _, edge := range parsed.Edges {
		if !nodeIDs[edge.From] {
			t.Errorf("edge from=%s references non-existent node", edge.From)
		}
		if !nodeIDs[edge.To] {
			t.Errorf("edge to=%s references non-existent node", edge.To)
		}
	}

	t.Logf("Generated DAG: %d nodes, %d edges", len(parsed.Nodes), len(parsed.Edges))
}

func TestTemplateIDForSkill_IsStableAndVersioned(t *testing.T) {
	id := TemplateIDForSkill("create-opinion-videos", "1.0.0")
	if id != "wf-create-opinion-videos-1-0-0" {
		t.Fatalf("unexpected template id: %s", id)
	}

	if again := TemplateIDForSkill("create-opinion-videos", "1.0.0"); again != id {
		t.Fatalf("expected stable id %s, got %s", id, again)
	}
}
