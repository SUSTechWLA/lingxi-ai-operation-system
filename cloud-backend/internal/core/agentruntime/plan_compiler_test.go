package agentruntime

import (
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestPlanCompiler_InsertsAfterArtifactReviewFromToolManifest(t *testing.T) {
	compiler := NewPlanCompiler(staticToolCatalog{
		"video_script_generator": &tool.ToolManifest{
			Name:     "video_script_generator",
			Type:     "builtin_prompt_tool",
			Endpoint: "builtin://video-creation/video_script_generator",
			ApprovalPolicy: tool.ApprovalPolicy{
				Required:         true,
				Mode:             tool.ApprovalAfterArtifact,
				BlocksDownstream: true,
				Reason:           "script requires review",
			},
			ArtifactPolicy: tool.ArtifactPolicy{
				ProduceArtifact:       true,
				ArtifactKinds:         []string{"MARKDOWN"},
				DefaultReviewRequired: true,
			},
		},
		"shot_splitter": &tool.ToolManifest{Name: "shot_splitter", Type: "builtin_prompt_tool", Endpoint: "builtin://video-creation/shot_splitter"},
	})

	dag, err := compiler.Compile(&AgentPlan{
		Goal:   "make a video package",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{ID: "script_generation", Tool: "video_script_generator", Arguments: map[string]interface{}{"topic": "AI workflows"}},
			{ID: "shot_split", Tool: "shot_splitter", DependsOn: []string{"script_generation"}, Arguments: map[string]interface{}{"script": "{{script_generation_exec.output.script}}"}},
		},
	})
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}

	requireNode(t, dag, "script_generation_exec", string(model.NodeTypeTool), "external")
	review := requireNode(t, dag, "script_generation_review", string(model.NodeTypeReviewGate), "审核-script_generation")
	if reason, _ := review.Input["reviewReason"].(string); reason != "script requires review" {
		t.Fatalf("review reason not copied from manifest: %#v", review.Input)
	}
	requireNode(t, dag, "shot_split", string(model.NodeTypeTool), "external")

	requireEdge(t, dag, "script_generation_exec", "script_generation_review")
	requireEdge(t, dag, "script_generation_review", "shot_split")
}

func TestPlanCompiler_ExternalToolNodeRoutesThroughExternalBridge(t *testing.T) {
	compiler := NewPlanCompiler(staticToolCatalog{
		"video_prompt_generator": &tool.ToolManifest{
			Name:     "video_prompt_generator",
			Type:     "builtin_prompt_tool",
			Endpoint: "builtin://video-creation/video_prompt_generator",
		},
	})

	dag, err := compiler.Compile(&AgentPlan{
		Goal:   "make prompts",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{ID: "video_prompts", Tool: "video_prompt_generator", Arguments: map[string]interface{}{"shotList": "{{shot_split.output.shotList}}"}},
		},
	})
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}

	node := requireNode(t, dag, "video_prompts", string(model.NodeTypeTool), "external")
	if got, _ := node.Input["tool"].(string); got != "external" {
		t.Fatalf("node input must route worker to external bridge, got %q", got)
	}
	params, ok := node.Input["parameters"].(map[string]interface{})
	if !ok {
		t.Fatalf("parameters missing: %#v", node.Input)
	}
	if got, _ := params["tool"].(string); got != "video_prompt_generator" {
		t.Fatalf("parameters.tool must preserve capability tool, got %q", got)
	}
}

func TestPlanCompiler_InsertsCloudBidAnalysisAfterParseBidFiles(t *testing.T) {
	compiler := NewPlanCompiler(staticToolCatalog{
		"parse_bid_files": &tool.ToolManifest{
			Name: "parse_bid_files",
			Type: "http",
			Output: map[string]tool.ParamDef{
				"stdout":        {Type: "string"},
				"raw_text_path": {Type: "string"},
				"report_path":   {Type: "string"},
			},
		},
		"bid_analysis_report": &tool.ToolManifest{
			Name: "bid_analysis_report",
			Type: "builtin",
			Parameters: map[string]tool.ParamDef{
				"raw_text_path": {Type: "string", Required: true},
				"report_path":   {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"report_path": {Type: "string"},
			},
		},
		"convert_to_word": &tool.ToolManifest{
			Name: "convert_to_word",
			Type: "http",
			Parameters: map[string]tool.ParamDef{
				"input_file": {Type: "string"},
			},
		},
	})

	plan := compiler.PreparePlan(&AgentPlan{
		Goal:   "parse tender and export word",
		Domain: "bid_writing",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{ID: "parse_bid_files", Tool: "parse_bid_files", Arguments: map[string]interface{}{"file_path": "E:/bid/test.docx"}},
			{
				ID:        "convert_to_word",
				Tool:      "convert_to_word",
				DependsOn: []string{"parse_bid_files"},
				Arguments: map[string]interface{}{"input_file": "{{parse_bid_files.output.report_path}}"},
			},
		},
	})

	if len(plan.Steps) != 2 {
		t.Fatalf("expected parse and convert steps only (no auto-injected analysis), got %#v", plan.Steps)
	}
	convert := plan.Steps[1]
	if len(convert.DependsOn) != 1 || convert.DependsOn[0] != "parse_bid_files" {
		t.Fatalf("convert should depend on parse step, got %#v", convert.DependsOn)
	}
	if err := NewPlanGuard(staticToolCatalog{
		"parse_bid_files":     compiler.manifestFor("parse_bid_files"),
		"bid_analysis_report": compiler.manifestFor("bid_analysis_report"),
		"convert_to_word":     compiler.manifestFor("convert_to_word"),
	}, nil).Validate(plan); err != nil {
		t.Fatalf("prepared bid plan should pass guard: %v", err)
	}
}

func TestPlanCompiler_RewiresExistingBidAnalysisAfterParseBidFiles(t *testing.T) {
	compiler := NewPlanCompiler(staticToolCatalog{
		"parse_bid_files": &tool.ToolManifest{
			Name: "parse_bid_files",
			Type: "http",
			Output: map[string]tool.ParamDef{
				"raw_text_path": {Type: "string"},
				"report_path":   {Type: "string"},
			},
		},
		"bid_analysis_report": &tool.ToolManifest{
			Name: "bid_analysis_report",
			Type: "builtin",
			Parameters: map[string]tool.ParamDef{
				"raw_text_path": {Type: "string", Required: true},
				"report_path":   {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"report_path": {Type: "string"},
			},
		},
		"convert_to_word": &tool.ToolManifest{
			Name: "convert_to_word",
			Type: "http",
			Parameters: map[string]tool.ParamDef{
				"input_file": {Type: "string"},
			},
		},
	})

	plan := compiler.PreparePlan(&AgentPlan{
		Goal:   "parse tender and export word",
		Domain: "bid_writing",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{ID: "parse_bid_files", Tool: "parse_bid_files", Arguments: map[string]interface{}{"file_path": "E:/bid/test.docx"}},
			{ID: "bid_analysis_report", Tool: "bid_analysis_report", Arguments: map[string]interface{}{"raw_text_path": "", "report_path": ""}},
			{
				ID:        "convert_to_word",
				Tool:      "convert_to_word",
				DependsOn: []string{"parse_bid_files"},
				Arguments: map[string]interface{}{"input_file": "{{parse_bid_files.output.report_path}}"},
			},
		},
	})

	if len(plan.Steps) != 3 {
		t.Fatalf("expected existing analysis step to remain, got %#v", plan.Steps)
	}
	analysis := plan.Steps[1]
	if analysis.ID != "bid_analysis_report" || analysis.Tool != "bid_analysis_report" {
		t.Fatalf("analysis step should remain in original position, got %#v", plan.Steps)
	}
	if got := analysis.Arguments["raw_text_path"]; got != "" {
		t.Fatalf("analysis raw_text_path should remain unchanged: %#v", analysis.Arguments)
	}
	if got := analysis.Arguments["report_path"]; got != "" {
		t.Fatalf("analysis report_path should remain unchanged: %#v", analysis.Arguments)
	}
	if got := plan.Steps[2].DependsOn; len(got) != 1 || got[0] != "parse_bid_files" {
		t.Fatalf("convert should depend on parse step, got %#v", plan.Steps[2].DependsOn)
	}
}

func TestPlanCompiler_InsertsQualityCheckerFromManifestPolicy(t *testing.T) {
	compiler := NewPlanCompiler(staticToolCatalog{
		"proposal_generator": &tool.ToolManifest{
			Name:     "proposal_generator",
			Type:     "builtin_prompt_tool",
			Endpoint: "builtin://video-creation/proposal_generator",
			QualityPolicy: tool.QualityPolicy{
				Required:    true,
				CheckerTool: "proposal_quality_checker",
				MinScore:    90,
			},
		},
		"proposal_quality_checker": &tool.ToolManifest{
			Name:     "proposal_quality_checker",
			Type:     "builtin_prompt_tool",
			Endpoint: "builtin://video-creation/proposal_quality_checker",
		},
	})

	dag, err := compiler.Compile(&AgentPlan{
		Goal:   "make a proposal",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{ID: "proposal", Tool: "proposal_generator", Arguments: map[string]interface{}{"brief": "知识视频"}},
		},
	})
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}

	requireNode(t, dag, "proposal", string(model.NodeTypeTool), "external")
	requireNode(t, dag, "proposal_quality_checker", string(model.NodeTypeTool), "external")
	gate := requireNode(t, dag, "proposal_quality_gate", string(model.NodeTypeReviewGate), "质量门禁-proposal_quality_gate")
	if got, _ := gate.Input["checkerStep"].(string); got != "proposal_quality_checker" {
		t.Fatalf("quality gate should reference manifest checker tool: %#v", gate.Input)
	}
	if got, ok := gate.Input["minScore"].(int); !ok || got != 90 {
		t.Fatalf("quality gate should copy minScore: %#v", gate.Input)
	}
	requireEdge(t, dag, "proposal", "proposal_quality_checker")
	requireEdge(t, dag, "proposal_quality_checker", "proposal_quality_gate")
}

func TestPlanCompiler_QualityGateReviewsProductionOutput(t *testing.T) {
	compiler := NewPlanCompiler(staticToolCatalog{
		"video_script_generator": &tool.ToolManifest{
			Name:     "video_script_generator",
			Type:     "builtin_prompt_tool",
			Endpoint: "builtin://video-creation/video_script_generator",
			QualityPolicy: tool.QualityPolicy{
				Required:    true,
				CheckerTool: "script_quality_checker",
				MinScore:    85,
			},
			ApprovalPolicy: tool.ApprovalPolicy{
				Required:         true,
				Mode:             tool.ApprovalAfterArtifact,
				BlocksDownstream: true,
			},
		},
		"script_quality_checker": &tool.ToolManifest{
			Name:     "script_quality_checker",
			Type:     "builtin_prompt_tool",
			Endpoint: "builtin://video-creation/script_quality_checker",
		},
	})

	dag, err := compiler.Compile(&AgentPlan{
		Goal:   "make a script",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{ID: "video_script_generator", Tool: "video_script_generator", Arguments: map[string]interface{}{"topic": "佛得角世界杯出线"}},
		},
	})
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}

	gate := requireNode(t, dag, "video_script_generator_quality_gate", string(model.NodeTypeReviewGate), "质量门禁-video_script_generator_quality_gate")
	if got, _ := gate.Input["sourceNode"].(string); got != "video_script_generator_exec" {
		t.Fatalf("quality gate should review production output, got input %#v", gate.Input)
	}
	if got, _ := gate.Input["reviewTool"].(string); got != "video_script_generator" {
		t.Fatalf("quality gate should expose production tool as review tool, got input %#v", gate.Input)
	}
	if got, _ := gate.Input["qualityCheckerNode"].(string); got != "script_quality_checker" {
		t.Fatalf("quality gate should expose checker node for score metadata, got input %#v", gate.Input)
	}
}

type staticToolCatalog map[string]*tool.ToolManifest

func (c staticToolCatalog) GetManifest(name string) *tool.ToolManifest {
	return c[name]
}

func requireNode(t *testing.T, dag *model.DAGRequest, id, typ, name string) model.NodeRequest {
	t.Helper()
	for _, n := range dag.Nodes {
		if n.ID != id {
			continue
		}
		if n.Type != typ || n.Name != name {
			t.Fatalf("node %s = type %q name %q, want type %q name %q", id, n.Type, n.Name, typ, name)
		}
		return n
	}
	t.Fatalf("node %s not found in %#v", id, dag.Nodes)
	return model.NodeRequest{}
}

func requireEdge(t *testing.T, dag *model.DAGRequest, from, to string) {
	t.Helper()
	for _, e := range dag.Edges {
		if e.From == from && e.To == to {
			return
		}
	}
	t.Fatalf("edge %s -> %s not found in %#v", from, to, dag.Edges)
}
