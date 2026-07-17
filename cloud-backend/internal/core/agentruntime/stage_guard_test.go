package agentruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestPlanGuardStageGuardRejectsForbiddenRoleTool(t *testing.T) {
	guard := NewPlanGuard(stageGuardCatalog(), nil).WithDirectors(testRoleRegistry{
		"script": testRoleDirector{
			roleID:         "script_writer",
			stage:          "script",
			allowedTools:   []string{"video_script_generator", "script_quality_checker"},
			forbiddenTools: []string{"hyperframes_renderer", "artifact_packager"},
			outputs:        []string{"VIDEO_SCRIPT"},
			review:         &tool.HumanReview{Required: true, Title: "审核口播脚本"},
		},
	})

	err := guard.ValidatePlan(context.Background(), "", &AgentPlan{
		Goal:   "make a video",
		Domain: "video_creation",
		Steps: []AgentStep{
			{
				ID:        "script",
				Tool:      "hyperframes_renderer",
				Arguments: map[string]interface{}{"stage": "script", "projectDir": "local://preview"},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "forbidden tool hyperframes_renderer") {
		t.Fatalf("expected forbidden tool error, got %v", err)
	}
}

func TestPlanGuardStageGuardRejectsMissingRequiredOutputContract(t *testing.T) {
	catalog := stageGuardCatalog()
	catalog["video_script_generator"].ArtifactPolicy.ArtifactKinds = []string{"MARKDOWN"}
	guard := NewPlanGuard(catalog, nil).WithDirectors(testRoleRegistry{
		"script": testRoleDirector{
			roleID:       "script_writer",
			stage:        "script",
			allowedTools: []string{"video_script_generator"},
			outputs:      []string{"VIDEO_SCRIPT"},
			review:       &tool.HumanReview{Required: true, Title: "审核口播脚本"},
		},
	})

	err := guard.ValidatePlan(context.Background(), "", &AgentPlan{
		Goal:   "make a video",
		Domain: "video_creation",
		Steps: []AgentStep{
			{
				ID:        "script",
				Tool:      "video_script_generator",
				Arguments: map[string]interface{}{"stage": "script", "topic": "AI workflows"},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "missing required output VIDEO_SCRIPT") {
		t.Fatalf("expected required output error, got %v", err)
	}
}

func TestPlanGuardRenderStageRequiresApprovedPreviewDependency(t *testing.T) {
	guard := NewPlanGuard(stageGuardCatalog(), nil).WithDirectors(testRoleRegistry{
		"preview": testRoleDirector{
			roleID:       "preview_director",
			stage:        "preview",
			allowedTools: []string{"hyperframes_snapshot"},
			outputs:      []string{"PREVIEW_SNAPSHOTS"},
			review:       &tool.HumanReview{Required: true, Title: "审核画面预览"},
		},
		"render": testRoleDirector{
			roleID:       "render_producer",
			stage:        "render",
			allowedTools: []string{"hyperframes_renderer"},
			inputs:       []string{"PREVIEW_SNAPSHOTS"},
			outputs:      []string{"VIDEO", "RENDER_REPORT"},
			review:       &tool.HumanReview{Required: true, Gate: tool.ApprovalBeforeExecute, Title: "确认最终渲染"},
		},
	})

	err := guard.ValidatePlan(context.Background(), "", &AgentPlan{
		Goal:   "make a video",
		Domain: "video_creation",
		Steps: []AgentStep{
			{
				ID:        "render",
				Tool:      "hyperframes_renderer",
				Arguments: map[string]interface{}{"stage": "render", "projectDir": "local://project"},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "requires an approved preview dependency") {
		t.Fatalf("expected missing preview dependency error, got %v", err)
	}

	err = guard.ValidatePlan(context.Background(), "", &AgentPlan{
		Goal:   "make a video",
		Domain: "video_creation",
		Steps: []AgentStep{
			{
				ID:        "preview",
				Tool:      "hyperframes_snapshot",
				Arguments: map[string]interface{}{"stage": "preview", "projectDir": "local://project"},
			},
			{
				ID:        "render",
				Tool:      "hyperframes_renderer",
				DependsOn: []string{"preview"},
				Arguments: map[string]interface{}{"stage": "render", "projectDir": "local://project"},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected render dependency to pass, got %v", err)
	}
}

func TestPlanGuardAllowsVideoPlanStartWhenFutureLocalRunnerIsOffline(t *testing.T) {
	catalog := stageGuardCatalog()
	catalog["hyperframes_snapshot"].ExecutionPlane = tool.ExecutionPlaneLocal
	catalog["hyperframes_renderer"].ExecutionPlane = tool.ExecutionPlaneLocal
	guard := NewPlanGuard(catalog, offlineLocalCapabilityProvider{}).WithDirectors(testRoleRegistry{
		"script": testRoleDirector{
			roleID:       "script_writer",
			stage:        "script",
			allowedTools: []string{"video_script_generator"},
			outputs:      []string{"VIDEO_SCRIPT"},
			review:       &tool.HumanReview{Required: true, Title: "审核口播脚本"},
		},
		"preview": testRoleDirector{
			roleID:       "preview_director",
			stage:        "preview",
			allowedTools: []string{"hyperframes_snapshot"},
			outputs:      []string{"PREVIEW_SNAPSHOTS"},
			review:       &tool.HumanReview{Required: true, Title: "审核画面预览"},
		},
		"render": testRoleDirector{
			roleID:       "render_producer",
			stage:        "render",
			allowedTools: []string{"hyperframes_renderer"},
			inputs:       []string{"PREVIEW_SNAPSHOTS"},
			outputs:      []string{"VIDEO", "RENDER_REPORT"},
			review:       &tool.HumanReview{Required: true, Gate: tool.ApprovalBeforeExecute, Title: "确认最终渲染"},
		},
	})

	err := guard.ValidatePlan(context.Background(), "user-1", &AgentPlan{
		Goal:   "make a video",
		Domain: "video_creation",
		Steps: []AgentStep{
			{
				ID:        "script",
				Tool:      "video_script_generator",
				Arguments: map[string]interface{}{"stage": "script", "topic": "AI workflows"},
			},
			{
				ID:        "preview",
				Tool:      "hyperframes_snapshot",
				DependsOn: []string{"script"},
				Arguments: map[string]interface{}{"stage": "preview", "projectDir": "local://project"},
			},
			{
				ID:        "render",
				Tool:      "hyperframes_renderer",
				DependsOn: []string{"preview"},
				Arguments: map[string]interface{}{"stage": "render", "projectDir": "local://project"},
			},
		},
	})
	if err != nil {
		t.Fatalf("video plan startup should not require future local runner availability: %v", err)
	}
}

func TestPlanCompilerAddsRoleAgentAndHumanReviewMetadataToReviewNode(t *testing.T) {
	compiler := NewPlanCompiler(stageGuardCatalog()).WithDirectors(testRoleRegistry{
		"script": testRoleDirector{
			roleID:       "script_writer",
			name:         "ScriptWriterAgent",
			displayName:  "脚本编剧",
			stage:        "script",
			goal:         "生成适合中文平台图文视频的口播脚本。",
			allowedTools: []string{"video_script_generator", "script_quality_checker"},
			inputs:       []string{"VIDEO_PROPOSAL"},
			outputs:      []string{"VIDEO_SCRIPT"},
			review: &tool.HumanReview{
				Required:    true,
				Gate:        tool.ApprovalAfterArtifact,
				Title:       "审核口播脚本",
				ReviewFocus: []string{"开头是否有吸引力"},
				UserActions: []string{"approve", "edit", "regenerate", "reject"},
			},
		},
	})

	dag, err := compiler.Compile(&AgentPlan{
		Goal:   "make a video",
		Domain: "video_creation",
		Steps: []AgentStep{
			{
				ID:        "script",
				Tool:      "video_script_generator",
				Arguments: map[string]interface{}{"stage": "script", "topic": "AI workflows"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}

	toolNode := requireNode(t, dag, "script_exec", string(model.NodeTypeTool), "external")
	params := toolNode.Input["parameters"].(map[string]interface{})
	if params["roleAgentId"] != "script_writer" {
		t.Fatalf("tool node should carry roleAgentId: %#v", params)
	}

	review := requireNode(t, dag, "script_review", string(model.NodeTypeReviewGate), "审核-script")
	roleAgent, ok := review.Input["roleAgent"].(map[string]interface{})
	if !ok {
		t.Fatalf("review node missing roleAgent metadata: %#v", review.Input)
	}
	if roleAgent["id"] != "script_writer" || roleAgent["displayName"] != "脚本编剧" {
		t.Fatalf("unexpected roleAgent metadata: %#v", roleAgent)
	}
	humanReview, ok := review.Input["humanReview"].(map[string]interface{})
	if !ok {
		t.Fatalf("review node missing humanReview metadata: %#v", review.Input)
	}
	if humanReview["title"] != "审核口播脚本" {
		t.Fatalf("unexpected humanReview metadata: %#v", humanReview)
	}
}

func stageGuardCatalog() staticToolCatalog {
	return staticToolCatalog{
		"video_script_generator": &tool.ToolManifest{
			Name: "video_script_generator",
			Type: "builtin_prompt_tool",
			ApprovalPolicy: tool.ApprovalPolicy{
				Required:         true,
				Mode:             tool.ApprovalAfterArtifact,
				BlocksDownstream: true,
				Reason:           "script requires review",
			},
			HumanReview:    &tool.HumanReview{Required: true, Title: "审核口播脚本"},
			ArtifactPolicy: tool.ArtifactPolicy{ProduceArtifact: true, ArtifactKinds: []string{"VIDEO_SCRIPT"}},
		},
		"script_quality_checker": &tool.ToolManifest{Name: "script_quality_checker", Type: "builtin_prompt_tool"},
		"hyperframes_snapshot": &tool.ToolManifest{
			Name: "hyperframes_snapshot",
			Type: "local_tool",
			ApprovalPolicy: tool.ApprovalPolicy{
				Required:         true,
				Mode:             tool.ApprovalAfterArtifact,
				BlocksDownstream: true,
			},
			HumanReview:    &tool.HumanReview{Required: true, Title: "审核画面预览"},
			ArtifactPolicy: tool.ArtifactPolicy{ProduceArtifact: true, ArtifactKinds: []string{"PREVIEW_SNAPSHOTS"}},
		},
		"hyperframes_renderer": &tool.ToolManifest{
			Name:       "hyperframes_renderer",
			Type:       "local_tool",
			SideEffect: true,
			ApprovalPolicy: tool.ApprovalPolicy{
				Required:         true,
				Mode:             tool.ApprovalBeforeExecute,
				BlocksDownstream: true,
			},
			HumanReview:    &tool.HumanReview{Required: true, Gate: tool.ApprovalBeforeExecute, Title: "确认最终渲染"},
			ArtifactPolicy: tool.ArtifactPolicy{ProduceArtifact: true, ArtifactKinds: []string{"VIDEO", "RENDER_REPORT"}},
		},
	}
}

type testRoleRegistry map[string]testRoleDirector

func (r testRoleRegistry) Get(stageName string) StageDirector {
	if d, ok := r[stageName]; ok {
		return d
	}
	return nil
}

type testRoleDirector struct {
	roleID         string
	name           string
	displayName    string
	stage          string
	goal           string
	inputs         []string
	outputs        []string
	allowedTools   []string
	forbiddenTools []string
	maxToolCalls   int
	review         *tool.HumanReview
}

func (d testRoleDirector) Name() string {
	if d.name != "" {
		return d.name
	}
	return d.roleID
}

func (d testRoleDirector) StageName() string              { return d.stage }
func (d testRoleDirector) AllowedTools() []string         { return d.allowedTools }
func (d testRoleDirector) ForbiddenTools() []string       { return d.forbiddenTools }
func (d testRoleDirector) MaxToolCalls() int              { return d.maxToolCalls }
func (d testRoleDirector) RequiresApproval() bool         { return d.review != nil && d.review.Required }
func (d testRoleDirector) RoleID() string                 { return d.roleID }
func (d testRoleDirector) DisplayName() string            { return d.displayName }
func (d testRoleDirector) Goal() string                   { return d.goal }
func (d testRoleDirector) RequiredInputs() []string       { return d.inputs }
func (d testRoleDirector) RequiredOutputs() []string      { return d.outputs }
func (d testRoleDirector) HumanReview() *tool.HumanReview { return d.review }

type offlineLocalCapabilityProvider struct{}

func (offlineLocalCapabilityProvider) HasOnlineRunner(context.Context, string) (bool, error) {
	return false, nil
}

func (offlineLocalCapabilityProvider) SupportsCommand(context.Context, string, string) (bool, error) {
	return false, nil
}

func (offlineLocalCapabilityProvider) SatisfiesRequirements(context.Context, string, *tool.LocalRequirements) (bool, []string, error) {
	return false, []string{"runner offline"}, nil
}
