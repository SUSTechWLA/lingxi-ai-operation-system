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
	review := requireNode(t, dag, "script_generation_review", string(model.NodeTypeControl), "审核-script_generation")
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
