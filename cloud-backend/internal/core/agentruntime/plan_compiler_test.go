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

func TestPlanCompiler_PreparePlanCompletesPartialVideoBetaPlan(t *testing.T) {
	compiler := NewPlanCompiler(staticToolCatalog{
		"shot_splitter":                 {Name: "shot_splitter"},
		"video_prompt_generator":        {Name: "video_prompt_generator"},
		"hyperframes_project_generator": {Name: "hyperframes_project_generator"},
		"hyperframes_renderer":          {Name: "hyperframes_renderer"},
		"publish_copy_generator":        {Name: "publish_copy_generator"},
	})
	plan := &AgentPlan{
		Goal:   "帮我介绍一下佛得角国家以及说明佛得角世界杯从小组赛出线是一个奇迹",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{ID: "brief", Tool: "proposal_generator", Arguments: map[string]interface{}{"brief": "佛得角国家介绍"}},
			{ID: "script", Tool: "video_script_generator", DependsOn: []string{"brief"}, Arguments: map[string]interface{}{"topic": "佛得角世界杯出线奇迹"}, ExpectedOutput: []string{"voiceover_script"}},
		},
		Budget: AgentBudget{MaxSteps: 2, MaxToolCalls: 2},
	}

	prepared := compiler.PreparePlan(plan)

	expected := []struct {
		id   string
		tool string
		deps []string
	}{
		{"brief", "proposal_generator", nil},
		{"script", "video_script_generator", []string{"brief"}},
		{"beat_plan", "shot_splitter", []string{"script"}},
		{"video_prompt", "video_prompt_generator", []string{"beat_plan"}},
		{"preview", "hyperframes_project_generator", []string{"beat_plan", "video_prompt", "script"}},
		{"render", "hyperframes_renderer", []string{"preview"}},
		{"publish_copy", "publish_copy_generator", []string{"render", "script", "beat_plan"}},
	}
	if len(prepared.Steps) != len(expected) {
		t.Fatalf("expected completed beta plan with %d steps, got %#v", len(expected), prepared.Steps)
	}
	for i, want := range expected {
		step := prepared.Steps[i]
		if step.ID != want.id || step.Tool != want.tool {
			t.Fatalf("step %d = %s/%s, want %s/%s", i, step.ID, step.Tool, want.id, want.tool)
		}
		requireStepDeps(t, step, want.deps)
	}
	if prepared.Budget.MaxSteps < len(expected) || prepared.Budget.MaxToolCalls < len(expected) {
		t.Fatalf("budget should expand with completed beta plan, got %+v", prepared.Budget)
	}
}

func TestPlanCompiler_PreparePlanInsertsShotGenerationPlanner(t *testing.T) {
	catalog := staticToolCatalog{
		"video_script_generator": {
			Name: "video_script_generator",
			Output: map[string]tool.ParamDef{
				"script": {Type: "string"},
			},
		},
		"shot_splitter": {
			Name: "shot_splitter",
			Parameters: map[string]tool.ParamDef{
				"script": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"shotList":          {Type: "array"},
				"shotAssetPackages": {Type: "array"},
			},
		},
		"shot_generation_planner": {
			Name: "shot_generation_planner",
			Parameters: map[string]tool.ParamDef{
				"shotList": {Type: "array", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"shotGenerationPlans": {Type: "array"},
				"shotAssetPackages":   {Type: "array"},
			},
		},
		"video_prompt_generator": {
			Name: "video_prompt_generator",
			Parameters: map[string]tool.ParamDef{
				"shotList":            {Type: "array", Required: true},
				"shotGenerationPlans": {Type: "array", Required: false},
				"shotAssetPackages":   {Type: "array", Required: false},
			},
			Output: map[string]tool.ParamDef{
				"videoPrompts":      {Type: "array"},
				"shotAssetPackages": {Type: "array"},
			},
		},
		"hyperframes_project_generator": {
			Name: "hyperframes_project_generator",
			Parameters: map[string]tool.ParamDef{
				"topic":             {Type: "string", Required: true},
				"script":            {Type: "string", Required: true},
				"shotList":          {Type: "array", Required: true},
				"videoPrompts":      {Type: "array", Required: false},
				"shotAssetPackages": {Type: "array", Required: false},
			},
			Output: map[string]tool.ParamDef{
				"projectDir": {Type: "string"},
				"entry":      {Type: "string"},
			},
		},
		"hyperframes_renderer": {
			Name: "hyperframes_renderer",
			Parameters: map[string]tool.ParamDef{
				"projectDir": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"outputPath": {Type: "string"},
			},
		},
		"publish_copy_generator": {
			Name: "publish_copy_generator",
			Output: map[string]tool.ParamDef{
				"title": {Type: "string"},
			},
		},
	}
	compiler := NewPlanCompiler(catalog)
	plan := &AgentPlan{
		Goal:   "请做一条端午节来历的口播知识分享视频",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{
				ID:              "script_generation",
				Tool:            "video_script_generator",
				Arguments:       map[string]interface{}{"topic": "端午节来历"},
				ExpectedOutput:  []string{"script"},
				ProduceArtifact: true,
			},
		},
	}

	prepared := compiler.PreparePlan(plan)

	generation := findStep(t, prepared, "shot_generation")
	if generation.Tool != "shot_generation_planner" {
		t.Fatalf("shot_generation tool = %s, want shot_generation_planner", generation.Tool)
	}
	if got := generation.Arguments["shotList"]; got != "{{beat_plan.output.shotList}}" {
		t.Fatalf("shot_generation should reference beat_plan shot list, got %#v", generation.Arguments)
	}
	prompt := findStep(t, prepared, "video_prompt")
	if got := prompt.Arguments["shotGenerationPlans"]; got != "{{shot_generation.output.shotGenerationPlans}}" {
		t.Fatalf("video_prompt should reference shot generation plans, got %#v", prompt.Arguments)
	}
	if got := prompt.Arguments["shotAssetPackages"]; got != "{{shot_generation.output.shotAssetPackages}}" {
		t.Fatalf("video_prompt should reference generation asset packages, got %#v", prompt.Arguments)
	}
	preview := findStep(t, prepared, "preview")
	if got := preview.Arguments["shotGenerationPlans"]; got != "{{shot_generation.output.shotGenerationPlans}}" {
		t.Fatalf("preview should reference shot generation plans, got %#v", preview.Arguments)
	}
	if got := preview.Arguments["shotAssetPackages"]; got != "{{video_prompt.output.shotAssetPackages}}" {
		t.Fatalf("preview should prefer prompt asset packages, got %#v", preview.Arguments)
	}
	if err := NewPlanGuard(catalog, nil).Validate(prepared); err != nil {
		t.Fatalf("prepared plan should pass PlanGuard: %v", err)
	}
}

func TestPlanCompiler_PreparePlanUsesTalkingHeadProfileTemplate(t *testing.T) {
	catalog := videoProfileTemplateCatalog()
	compiler := NewPlanCompiler(catalog)
	plan := &AgentPlan{
		Goal:   "请帮我根据端午节的来历创作一个口播知识分享视频",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{
				ID:              "script_generation",
				Tool:            "video_script_generator",
				Arguments:       map[string]interface{}{"topic": "端午节来历"},
				ExpectedOutput:  []string{"script"},
				ProduceArtifact: true,
			},
		},
	}

	prepared := compiler.PreparePlan(plan)

	assertStepOrder(t, prepared, []string{
		"profile_selection",
		"script_generation",
		"time_window",
		"visual_alignment",
		"shot_generation",
		"video_prompt",
		"preview",
		"render",
	})
	timeWindow := findStep(t, prepared, "time_window")
	if got := timeWindow.Arguments["creationProfile"]; got != "{{profile_selection.output.creationProfile}}" {
		t.Fatalf("time_window creationProfile = %#v, want profile selection output", got)
	}
	if got := timeWindow.Arguments["scriptSpans"]; got != "{{script_generation.output.script}}" {
		t.Fatalf("time_window scriptSpans = %#v, want script output", got)
	}
	alignment := findStep(t, prepared, "visual_alignment")
	if got := alignment.Arguments["timeWindows"]; got != "{{time_window.output.timeWindows}}" {
		t.Fatalf("visual_alignment timeWindows = %#v, want time window output", got)
	}
	generation := findStep(t, prepared, "shot_generation")
	if got := generation.Arguments["shotList"]; got != "{{visual_alignment.output.shotList}}" {
		t.Fatalf("shot_generation shotList = %#v, want visual alignment shot list", got)
	}
	if got := generation.Arguments["timeWindows"]; got != "{{time_window.output.timeWindows}}" {
		t.Fatalf("shot_generation timeWindows = %#v, want time window output", got)
	}
	if got := generation.Arguments["creationProfile"]; got != "{{profile_selection.output.creationProfile}}" {
		t.Fatalf("shot_generation creationProfile = %#v, want profile selection output", got)
	}
	if err := NewPlanGuard(catalog, nil).Validate(prepared); err != nil {
		t.Fatalf("prepared plan should pass PlanGuard: %v", err)
	}
}

func TestPlanCompiler_PreparePlanUsesCinematicProfileTemplate(t *testing.T) {
	catalog := videoProfileTemplateCatalog()
	compiler := NewPlanCompiler(catalog)
	plan := &AgentPlan{
		Goal:   "创作一支有角色、场景和道具连续性的 cinematic story 影视短片",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
	}

	prepared := compiler.PreparePlan(plan)

	assertStepOrder(t, prepared, []string{
		"profile_selection",
		"story_foundation",
		"cinematic_script",
		"continuity_bible",
		"reference_assets",
		"cinematic_shot_design",
		"time_window",
		"keyframes_storyboards",
		"shot_generation",
	})
	timeWindow := findStep(t, prepared, "time_window")
	if got := timeWindow.Arguments["shotList"]; got != "{{cinematic_shot_design.output.shotList}}" {
		t.Fatalf("time_window shotList = %#v, want cinematic shot design shot list", got)
	}
	if got := timeWindow.Arguments["creationProfile"]; got != "{{profile_selection.output.creationProfile}}" {
		t.Fatalf("time_window creationProfile = %#v, want profile selection output", got)
	}
	script := findStep(t, prepared, "cinematic_script")
	if got := script.Arguments["topic"]; got != plan.Goal {
		t.Fatalf("cinematic_script topic = %#v, want plan goal", got)
	}
	if got := script.Arguments["proposal"]; got != "{{story_foundation.output.proposalPacket}}" {
		t.Fatalf("cinematic_script proposal = %#v, want proposalPacket ref", got)
	}
	continuity := findStep(t, prepared, "continuity_bible")
	if !containsString(continuity.ExpectedOutput, "continuityReport") {
		t.Fatalf("continuity_bible should declare real continuityReport output, got %#v", continuity.ExpectedOutput)
	}
	referenceAssets := findStep(t, prepared, "reference_assets")
	if got := referenceAssets.Arguments["continuityBible"]; got != "{{continuity_bible.output.continuityReport}}" {
		t.Fatalf("reference_assets continuity context = %#v, want continuityReport ref", got)
	}
	generation := findStep(t, prepared, "shot_generation")
	if got := generation.Arguments["timeWindows"]; got != "{{time_window.output.timeWindows}}" {
		t.Fatalf("shot_generation timeWindows = %#v, want time window output", got)
	}
	if got := generation.Arguments["shotList"]; got != "{{time_window.output.timeWindows}}" {
		t.Fatalf("shot_generation shotList = %#v, want fine time-window shot list", got)
	}
	if got := generation.Arguments["creationProfile"]; got != "{{profile_selection.output.creationProfile}}" {
		t.Fatalf("shot_generation creationProfile = %#v, want profile selection output", got)
	}
	if got := generation.Arguments["continuityBible"]; got != "{{continuity_bible.output.continuityReport}}" {
		t.Fatalf("shot_generation continuity context = %#v, want continuityReport ref", got)
	}
	if got := generation.Arguments["keyframePrompts"]; got != "{{keyframes_storyboards.output.keyframePrompts}}" {
		t.Fatalf("shot_generation keyframe prompts = %#v, want keyframePrompts ref", got)
	}
	if _, ok := generation.Arguments["keyframeStoryboards"]; ok {
		t.Fatalf("shot_generation must not reference virtual keyframeStoryboards output, got %#v", generation.Arguments)
	}
	keyframes := findStep(t, prepared, "keyframes_storyboards")
	requireStepDeps(t, keyframes, []string{"reference_assets", "time_window"})
	if got := keyframes.Arguments["shotList"]; got != "{{time_window.output.timeWindows}}" {
		t.Fatalf("keyframes_storyboards shotList = %#v, want time window shot list", got)
	}
	if err := NewPlanGuard(catalog, nil).Validate(prepared); err != nil {
		t.Fatalf("prepared plan should pass PlanGuard: %v", err)
	}
}

func TestPlanCompiler_PreparePlanProfileRewiresExistingVideoPrompt(t *testing.T) {
	catalog := videoProfileTemplateCatalog()
	compiler := NewPlanCompiler(catalog)
	plan := &AgentPlan{
		Goal:   "请做一条端午节来历的口播知识分享视频",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{
				ID:              "script_generation",
				Tool:            "video_script_generator",
				Arguments:       map[string]interface{}{"topic": "端午节来历"},
				ExpectedOutput:  []string{"script"},
				ProduceArtifact: true,
			},
			{
				ID:        "beat_plan",
				Tool:      "shot_splitter",
				DependsOn: []string{"script_generation"},
				Arguments: map[string]interface{}{
					"script": "{{script_generation.output.script}}",
				},
				ExpectedOutput:  []string{"shotList"},
				ProduceArtifact: true,
			},
			{
				ID:        "video_prompt",
				Tool:      "video_prompt_generator",
				DependsOn: []string{"beat_plan"},
				Arguments: map[string]interface{}{
					"stage":    "video_prompt",
					"shotList": "{{beat_plan.output.shotList}}",
				},
				ExpectedOutput:  []string{"videoPrompts"},
				ProduceArtifact: true,
			},
		},
	}

	prepared := compiler.PreparePlan(plan)

	prompt := findStep(t, prepared, "video_prompt")
	if got := prompt.Arguments["shotList"]; got != "{{visual_alignment.output.shotList}}" {
		t.Fatalf("existing video_prompt shotList = %#v, want profile visual alignment shot list", got)
	}
	if !containsString(prompt.DependsOn, "visual_alignment") {
		t.Fatalf("existing video_prompt should depend on visual_alignment, got %#v", prompt.DependsOn)
	}
	if !containsString(prompt.DependsOn, "shot_generation") {
		t.Fatalf("existing video_prompt should depend on shot_generation, got %#v", prompt.DependsOn)
	}
	if containsString(prompt.DependsOn, "beat_plan") {
		t.Fatalf("existing video_prompt should not keep beat_plan dependency, got %#v", prompt.DependsOn)
	}
	if err := NewPlanGuard(catalog, nil).Validate(prepared); err != nil {
		t.Fatalf("prepared plan should pass PlanGuard: %v", err)
	}
}

func TestPlanCompiler_PreparePlanCinematicProfileDoesNotReuseLegacyKeyframesAsPrompt(t *testing.T) {
	catalog := videoProfileTemplateCatalog()
	compiler := NewPlanCompiler(catalog)
	plan := &AgentPlan{
		Goal:   "创作一个有角色和场景连续性的 cinematic story 短片",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{
				ID:   "legacy_keyframes",
				Tool: "keyframe_prompt_generator",
				Arguments: map[string]interface{}{
					"shotList":           []interface{}{},
					"timeWindows":        []interface{}{},
					"referenceAssetPlan": map[string]interface{}{},
				},
				ExpectedOutput:  []string{"keyframePrompts"},
				ProduceArtifact: true,
			},
		},
	}

	prepared := compiler.PreparePlan(plan)

	keyframes := findStep(t, prepared, "keyframes_storyboards")
	if keyframes.Tool != "keyframe_prompt_generator" {
		t.Fatalf("profile keyframes tool = %s, want keyframe_prompt_generator", keyframes.Tool)
	}
	videoPrompt := findStep(t, prepared, "video_prompt")
	if videoPrompt.Tool != "video_prompt_generator" {
		t.Fatalf("video_prompt tool = %s, want video_prompt_generator", videoPrompt.Tool)
	}
	preview := findStep(t, prepared, "preview")
	if got := preview.Arguments["videoPrompts"]; got != "{{video_prompt.output.videoPrompts}}" {
		t.Fatalf("preview videoPrompts = %#v, want canonical video_prompt output", got)
	}
	if err := NewPlanGuard(catalog, nil).Validate(prepared); err != nil {
		t.Fatalf("prepared plan should pass PlanGuard: %v", err)
	}
}

func TestPlanCompiler_PreparePlanCinematicProfileDoesNotReuseUnrelatedProposalStep(t *testing.T) {
	catalog := videoProfileTemplateCatalog()
	compiler := NewPlanCompiler(catalog)
	plan := &AgentPlan{
		Goal:   "创作一个有角色和道具连续性的 cinematic story 短片",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{
				ID:              "marketing_proposal",
				Tool:            "proposal_generator",
				Arguments:       map[string]interface{}{"brief": "另一个营销提案"},
				ExpectedOutput:  []string{"proposalPacket"},
				ProduceArtifact: true,
			},
		},
	}

	prepared := compiler.PreparePlan(plan)

	story := findStep(t, prepared, "story_foundation")
	if story.Tool != "proposal_generator" {
		t.Fatalf("story_foundation tool = %s, want proposal_generator", story.Tool)
	}
	if got := story.Arguments["brief"]; got != plan.Goal {
		t.Fatalf("story_foundation brief = %#v, want plan goal", got)
	}
	marketing := findStep(t, prepared, "marketing_proposal")
	if got := marketing.Arguments["brief"]; got != "另一个营销提案" {
		t.Fatalf("unrelated proposal step was rewritten: %#v", marketing.Arguments)
	}
	if err := NewPlanGuard(catalog, nil).Validate(prepared); err != nil {
		t.Fatalf("prepared plan should pass PlanGuard: %v", err)
	}
}

func TestPlanCompiler_PreparePlanAugmentsExistingShotGenerationConsumers(t *testing.T) {
	catalog := staticToolCatalog{
		"video_script_generator": {
			Name: "video_script_generator",
			Output: map[string]tool.ParamDef{
				"script": {Type: "string"},
			},
		},
		"shot_splitter": {
			Name: "shot_splitter",
			Parameters: map[string]tool.ParamDef{
				"script": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"shotList": {Type: "array"},
			},
		},
		"shot_generation_planner": {
			Name: "shot_generation_planner",
			Parameters: map[string]tool.ParamDef{
				"shotList": {Type: "array", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"shotGenerationPlans": {Type: "array"},
				"shotAssetPackages":   {Type: "array"},
			},
		},
		"video_prompt_generator": {
			Name: "video_prompt_generator",
			Parameters: map[string]tool.ParamDef{
				"shotList":            {Type: "array", Required: true},
				"shotGenerationPlans": {Type: "array", Required: false},
				"shotAssetPackages":   {Type: "array", Required: false},
			},
			Output: map[string]tool.ParamDef{
				"videoPrompts":      {Type: "array"},
				"shotAssetPackages": {Type: "array"},
			},
		},
		"hyperframes_project_generator": {
			Name: "hyperframes_project_generator",
			Parameters: map[string]tool.ParamDef{
				"topic":               {Type: "string", Required: true},
				"script":              {Type: "string", Required: true},
				"shotList":            {Type: "array", Required: true},
				"videoPrompts":        {Type: "array", Required: false},
				"shotGenerationPlans": {Type: "array", Required: false},
				"shotAssetPackages":   {Type: "array", Required: false},
			},
			Output: map[string]tool.ParamDef{
				"projectDir": {Type: "string"},
			},
		},
		"hyperframes_renderer": {
			Name: "hyperframes_renderer",
			Parameters: map[string]tool.ParamDef{
				"projectDir": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"outputPath": {Type: "string"},
			},
		},
		"publish_copy_generator": {
			Name: "publish_copy_generator",
			Output: map[string]tool.ParamDef{
				"title": {Type: "string"},
			},
		},
	}
	compiler := NewPlanCompiler(catalog)
	plan := &AgentPlan{
		Goal:   "请做一条端午节来历的口播知识分享视频",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{
				ID:              "script_generation",
				Tool:            "video_script_generator",
				Arguments:       map[string]interface{}{"topic": "端午节来历"},
				ExpectedOutput:  []string{"script"},
				ProduceArtifact: true,
			},
			{
				ID:        "beat_plan",
				Tool:      "shot_splitter",
				DependsOn: []string{"script_generation"},
				Arguments: map[string]interface{}{
					"script": "{{script_generation.output.script}}",
				},
				ExpectedOutput:  []string{"shotList"},
				ProduceArtifact: true,
			},
			{
				ID:        "video_prompt",
				Tool:      "video_prompt_generator",
				DependsOn: []string{"beat_plan"},
				Arguments: map[string]interface{}{
					"shotList": "{{beat_plan.output.shotList}}",
				},
				ExpectedOutput:  []string{"videoPrompts", "shotAssetPackages"},
				ProduceArtifact: true,
			},
			{
				ID:        "preview",
				Tool:      "hyperframes_project_generator",
				DependsOn: []string{"beat_plan", "video_prompt", "script_generation"},
				Arguments: map[string]interface{}{
					"topic":             "端午节来历",
					"script":            "{{script_generation.output.script}}",
					"shotList":          "{{beat_plan.output.shotList}}",
					"videoPrompts":      "{{video_prompt.output.videoPrompts}}",
					"shotAssetPackages": []string{"custom-preview-packages"},
				},
				ExpectedOutput:  []string{"projectDir"},
				ProduceArtifact: true,
			},
			{
				ID:        "render",
				Tool:      "hyperframes_renderer",
				DependsOn: []string{"preview"},
				Arguments: map[string]interface{}{
					"projectDir": "{{preview.output.projectDir}}",
				},
				ExpectedOutput:  []string{"outputPath"},
				ProduceArtifact: true,
			},
		},
	}

	prepared := compiler.PreparePlan(plan)

	beatIndex := stepIndex(t, prepared, "beat_plan")
	if got := prepared.Steps[beatIndex+1].ID; got != "shot_generation" {
		t.Fatalf("shot_generation should be inserted immediately after beat_plan, got next step %s in %#v", got, prepared.Steps)
	}
	prompt := findStep(t, prepared, "video_prompt")
	if got := prompt.Arguments["shotGenerationPlans"]; got != "{{shot_generation.output.shotGenerationPlans}}" {
		t.Fatalf("existing video_prompt should reference shot generation plans, got %#v", prompt.Arguments)
	}
	if got := prompt.Arguments["shotAssetPackages"]; got != "{{shot_generation.output.shotAssetPackages}}" {
		t.Fatalf("existing video_prompt should reference generation asset packages, got %#v", prompt.Arguments)
	}
	if !containsString(prompt.DependsOn, "shot_generation") {
		t.Fatalf("existing video_prompt should depend on shot_generation, got %#v", prompt.DependsOn)
	}
	preview := findStep(t, prepared, "preview")
	if got := preview.Arguments["shotGenerationPlans"]; got != "{{shot_generation.output.shotGenerationPlans}}" {
		t.Fatalf("existing preview should reference shot generation plans, got %#v", preview.Arguments)
	}
	packages, ok := preview.Arguments["shotAssetPackages"].([]string)
	if !ok || len(packages) != 1 || packages[0] != "custom-preview-packages" {
		t.Fatalf("existing preview shotAssetPackages should be preserved, got %#v", preview.Arguments)
	}
	if !containsString(preview.DependsOn, "shot_generation") {
		t.Fatalf("existing preview should depend on shot_generation, got %#v", preview.DependsOn)
	}
	if err := NewPlanGuard(catalog, nil).Validate(prepared); err != nil {
		t.Fatalf("prepared plan should pass PlanGuard: %v", err)
	}
}

func TestPlanCompiler_PreparePlanExpandsCostBudgetForInjectedRender(t *testing.T) {
	catalog := staticToolCatalog{
		"video_script_generator": {
			Name:      "video_script_generator",
			CostLevel: tool.CostLow,
			Output: map[string]tool.ParamDef{
				"script": {Type: "string"},
			},
		},
		"shot_splitter": {
			Name:      "shot_splitter",
			CostLevel: tool.CostLow,
			Parameters: map[string]tool.ParamDef{
				"script": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"shotList": {Type: "array"},
			},
		},
		"video_prompt_generator": {
			Name:      "video_prompt_generator",
			CostLevel: tool.CostMedium,
			Parameters: map[string]tool.ParamDef{
				"shotList": {Type: "array", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"videoPrompts": {Type: "array"},
			},
		},
		"hyperframes_project_generator": {
			Name:      "hyperframes_project_generator",
			CostLevel: tool.CostLow,
			Parameters: map[string]tool.ParamDef{
				"topic":    {Type: "string", Required: true},
				"script":   {Type: "string", Required: true},
				"shotList": {Type: "array", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"projectDir": {Type: "string"},
			},
		},
		"hyperframes_renderer": {
			Name:       "hyperframes_renderer",
			CostLevel:  tool.CostHigh,
			RiskLevel:  tool.RiskMedium,
			SideEffect: true,
			ApprovalPolicy: tool.ApprovalPolicy{
				Required: true,
			},
			Parameters: map[string]tool.ParamDef{
				"projectDir": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"outputPath": {Type: "string"},
			},
		},
		"publish_copy_generator": {
			Name:      "publish_copy_generator",
			CostLevel: tool.CostLow,
			Parameters: map[string]tool.ParamDef{
				"script":   {Type: "string", Required: true},
				"shotList": {Type: "array", Required: false},
			},
			Output: map[string]tool.ParamDef{
				"title": {Type: "string"},
			},
		},
	}
	compiler := NewPlanCompiler(catalog)
	plan := &AgentPlan{
		Goal:   "帮我介绍一下佛得角国家以及说明佛得角世界杯小组赛出线进入淘汰赛是一个奇迹",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{
				ID:              "script_generation",
				Tool:            "video_script_generator",
				Arguments:       map[string]interface{}{"topic": "佛得角世界杯奇迹"},
				ExpectedOutput:  []string{"script"},
				ProduceArtifact: true,
			},
		},
		Budget: AgentBudget{
			MaxSteps:     1,
			MaxToolCalls: 1,
			MaxCostLevel: tool.CostMedium,
		},
	}

	prepared := compiler.PreparePlan(plan)

	if got := prepared.Budget.MaxCostLevel; got != tool.CostHigh {
		t.Fatalf("budget should expand to render cost %q, got %q", tool.CostHigh, got)
	}
	if err := NewPlanGuard(catalog, nil).Validate(prepared); err != nil {
		t.Fatalf("prepared plan should pass PlanGuard: %v", err)
	}
}

func TestPlanCompiler_PreparePlanAlignsInjectedPreviewWithStageGuard(t *testing.T) {
	catalog := videoBetaCompletionCatalog()
	catalog["hyperframes_project_generator"].ApprovalPolicy = tool.ApprovalPolicy{
		Required:         true,
		Mode:             tool.ApprovalAfterArtifact,
		BlocksDownstream: true,
	}
	catalog["hyperframes_project_generator"].HumanReview = &tool.HumanReview{Required: true, Title: "审核画面预览"}
	catalog["hyperframes_project_generator"].ArtifactPolicy = tool.ArtifactPolicy{
		ProduceArtifact: true,
		ArtifactKinds:   []string{"HYPERFRAMES_PROJECT"},
	}
	catalog["hyperframes_renderer"].ArtifactPolicy = tool.ArtifactPolicy{
		ProduceArtifact: true,
		ArtifactKinds:   []string{"VIDEO", "RENDER_REPORT"},
	}
	directors := testRoleRegistry{
		"preview": testRoleDirector{
			roleID:       "preview_director",
			stage:        "preview",
			allowedTools: []string{"hyperframes_project_generator"},
			inputs:       []string{"VIDEO_COMPOSITION_SPEC"},
			outputs:      []string{"HYPERFRAMES_PROJECT", "PREVIEW_SNAPSHOTS", "PREVIEW_REPORT"},
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
	}
	compiler := NewPlanCompiler(catalog).WithDirectors(directors)
	plan := &AgentPlan{
		Goal:   "一段测试文字",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{
				ID:              "script_generation",
				Tool:            "video_script_generator",
				Arguments:       map[string]interface{}{"topic": "一段测试文字"},
				ExpectedOutput:  []string{"script"},
				ProduceArtifact: true,
			},
		},
		Budget: AgentBudget{MaxCostLevel: tool.CostMedium},
	}

	prepared := compiler.PreparePlan(plan)

	if err := NewPlanGuard(catalog, nil).WithDirectors(directors).Validate(prepared); err != nil {
		t.Fatalf("prepared plan should satisfy stage guard: %v", err)
	}
}

func TestPlanCompiler_PreparePlanUsesScriptProducerWhenCaptionSplitterIsLastStep(t *testing.T) {
	catalog := staticToolCatalog{
		"video_script_generator": {
			Name: "video_script_generator",
			Output: map[string]tool.ParamDef{
				"script": {Type: "string"},
			},
		},
		"caption_splitter": {
			Name: "caption_splitter",
			Output: map[string]tool.ParamDef{
				"captionPlan": {Type: "object"},
				"artifacts":   {Type: "object"},
			},
		},
		"shot_splitter": {
			Name: "shot_splitter",
			Parameters: map[string]tool.ParamDef{
				"script": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"shotList": {Type: "array"},
			},
		},
		"video_prompt_generator": {
			Name: "video_prompt_generator",
			Parameters: map[string]tool.ParamDef{
				"shotList": {Type: "array", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"videoPrompts": {Type: "array"},
			},
		},
		"hyperframes_project_generator": {
			Name: "hyperframes_project_generator",
			Parameters: map[string]tool.ParamDef{
				"shotList": {Type: "array", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"hyperframesPath": {Type: "string"},
			},
		},
		"hyperframes_renderer": {
			Name: "hyperframes_renderer",
			Parameters: map[string]tool.ParamDef{
				"hyperframesPath": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"finalVideo": {Type: "string"},
			},
		},
		"publish_copy_generator": {
			Name: "publish_copy_generator",
			Output: map[string]tool.ParamDef{
				"publish_copy": {Type: "object"},
			},
		},
	}
	compiler := NewPlanCompiler(catalog)
	plan := &AgentPlan{
		Goal:   "帮我介绍一下佛得角国家以及说明佛得角世界杯小组赛出线进入淘汰赛是一个奇迹",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{
				ID:              "script_generation",
				Tool:            "video_script_generator",
				Arguments:       map[string]interface{}{"topic": "佛得角世界杯奇迹"},
				ExpectedOutput:  []string{"script"},
				ProduceArtifact: true,
			},
			{
				ID:        "caption_splitter",
				Tool:      "caption_splitter",
				DependsOn: []string{"script_generation"},
				Arguments: map[string]interface{}{
					"script": "{{script_generation.output.script}}",
				},
				ExpectedOutput:  []string{"captionPlan"},
				ProduceArtifact: true,
			},
		},
	}

	prepared := compiler.PreparePlan(plan)

	beat := prepared.Steps[2]
	if beat.ID != "beat_plan" || beat.Tool != "shot_splitter" {
		t.Fatalf("expected injected beat_plan as third step, got %#v", prepared.Steps)
	}
	if got := beat.Arguments["script"]; got != "{{script_generation.output.script}}" {
		t.Fatalf("beat_plan should reference script producer, got %#v", beat.Arguments)
	}
	if len(beat.DependsOn) != 1 || beat.DependsOn[0] != "script_generation" {
		t.Fatalf("beat_plan should depend on script producer, got %#v", beat.DependsOn)
	}
	if err := NewPlanGuard(catalog, nil).Validate(prepared); err != nil {
		t.Fatalf("prepared plan should pass PlanGuard: %v", err)
	}
}

func TestPlanCompiler_PreparePlanIgnoresHallucinatedScriptExpectedOutputFromNewsSearch(t *testing.T) {
	catalog := staticToolCatalog{
		"video_script_generator": {
			Name: "video_script_generator",
			Output: map[string]tool.ParamDef{
				"script": {Type: "string"},
			},
		},
		"news_search": {
			Name: "news_search",
			Output: map[string]tool.ParamDef{
				"facts":   {Type: "array"},
				"sources": {Type: "array"},
			},
		},
		"shot_splitter": {
			Name: "shot_splitter",
			Parameters: map[string]tool.ParamDef{
				"script": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"shotList": {Type: "array"},
			},
		},
		"video_prompt_generator": {
			Name: "video_prompt_generator",
			Parameters: map[string]tool.ParamDef{
				"shotList": {Type: "array", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"videoPrompts":      {Type: "array"},
				"shotAssetPackages": {Type: "array"},
			},
		},
		"hyperframes_project_generator": {
			Name: "hyperframes_project_generator",
			Parameters: map[string]tool.ParamDef{
				"topic":             {Type: "string", Required: true},
				"script":            {Type: "string", Required: true},
				"shotList":          {Type: "array", Required: true},
				"shotAssetPackages": {Type: "array", Required: false},
			},
			Output: map[string]tool.ParamDef{
				"projectDir": {Type: "string"},
				"entry":      {Type: "string"},
			},
		},
		"hyperframes_renderer": {
			Name: "hyperframes_renderer",
			Parameters: map[string]tool.ParamDef{
				"projectDir": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"outputPath": {Type: "string"},
			},
		},
		"publish_copy_generator": {
			Name: "publish_copy_generator",
			Parameters: map[string]tool.ParamDef{
				"script":   {Type: "string", Required: true},
				"shotList": {Type: "array", Required: false},
			},
			Output: map[string]tool.ParamDef{
				"title":       {Type: "string"},
				"description": {Type: "string"},
			},
		},
	}
	compiler := NewPlanCompiler(catalog)
	plan := &AgentPlan{
		Goal:   "帮我介绍一下佛得角国家以及说明佛得角世界杯小组赛出线进入淘汰赛是一个奇迹",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{
				ID:              "script_generation",
				Tool:            "video_script_generator",
				Arguments:       map[string]interface{}{"topic": "佛得角世界杯奇迹"},
				ExpectedOutput:  []string{"script"},
				ProduceArtifact: true,
			},
			{
				ID:        "news_search",
				Tool:      "news_search",
				DependsOn: []string{"script_generation"},
				Arguments: map[string]interface{}{
					"query": "Cape Verde World Cup knockout latest",
				},
				ExpectedOutput: []string{"facts", "sources", "script"},
			},
		},
	}

	prepared := compiler.PreparePlan(plan)

	beat := findStep(t, prepared, "beat_plan")
	if got := beat.Arguments["script"]; got != "{{script_generation.output.script}}" {
		t.Fatalf("beat_plan should reference script producer, got %#v", beat.Arguments)
	}
	preview := findStep(t, prepared, "preview")
	if got := preview.Arguments["script"]; got != "{{script_generation.output.script}}" {
		t.Fatalf("preview should reference script producer, got %#v", preview.Arguments)
	}
	if got := preview.Arguments["shotList"]; got != "{{beat_plan.output.shotList}}" {
		t.Fatalf("preview should reference shot list producer, got %#v", preview.Arguments)
	}
	prompt := findStep(t, prepared, "video_prompt")
	if got := prompt.Arguments["shotList"]; got != "{{beat_plan.output.shotList}}" {
		t.Fatalf("video_prompt should reference shot list producer, got %#v", prompt.Arguments)
	}
	if got := preview.Arguments["videoPrompts"]; got != "{{video_prompt.output.videoPrompts}}" {
		t.Fatalf("preview should reference video prompt producer, got %#v", preview.Arguments)
	}
	if got := preview.Arguments["shotAssetPackages"]; got != "{{video_prompt.output.shotAssetPackages}}" {
		t.Fatalf("preview should reference shot asset package producer, got %#v", preview.Arguments)
	}
	render := findStep(t, prepared, "render")
	if got := render.Arguments["projectDir"]; got != "{{preview.output.projectDir}}" {
		t.Fatalf("render should reference projectDir producer, got %#v", render.Arguments)
	}
	if _, ok := render.Arguments["hyperframesPath"]; ok {
		t.Fatalf("render must not reference undeclared hyperframesPath, got %#v", render.Arguments)
	}
	publish := findStep(t, prepared, "publish_copy")
	if got := publish.Arguments["script"]; got != "{{script_generation.output.script}}" {
		t.Fatalf("publish_copy should reference script producer, got %#v", publish.Arguments)
	}
	if got := publish.Arguments["shotList"]; got != "{{beat_plan.output.shotList}}" {
		t.Fatalf("publish_copy should reference shot list producer, got %#v", publish.Arguments)
	}
	if _, ok := publish.Arguments["finalVideo"]; ok {
		t.Fatalf("publish_copy must not reference undeclared finalVideo, got %#v", publish.Arguments)
	}
	if err := NewPlanGuard(catalog, nil).Validate(prepared); err != nil {
		t.Fatalf("prepared plan should pass PlanGuard: %v", err)
	}
}

func TestPlanCompiler_PreparePlanRepairsExistingBeatPlanInvalidScriptReference(t *testing.T) {
	catalog := staticToolCatalog{
		"news_search": {
			Name: "news_search",
			Output: map[string]tool.ParamDef{
				"facts":   {Type: "array"},
				"sources": {Type: "array"},
			},
		},
		"video_script_generator": {
			Name: "video_script_generator",
			Output: map[string]tool.ParamDef{
				"script": {Type: "string"},
			},
		},
		"shot_splitter": {
			Name: "shot_splitter",
			Parameters: map[string]tool.ParamDef{
				"script": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"shotList": {Type: "array"},
			},
		},
		"video_prompt_generator": {
			Name: "video_prompt_generator",
			Parameters: map[string]tool.ParamDef{
				"shotList": {Type: "array", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"videoPrompts": {Type: "array"},
			},
		},
		"hyperframes_project_generator": {
			Name: "hyperframes_project_generator",
			Parameters: map[string]tool.ParamDef{
				"topic":    {Type: "string", Required: true},
				"script":   {Type: "string", Required: true},
				"shotList": {Type: "array", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"projectDir": {Type: "string"},
			},
		},
		"hyperframes_renderer": {
			Name: "hyperframes_renderer",
			Parameters: map[string]tool.ParamDef{
				"projectDir": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"outputPath": {Type: "string"},
			},
		},
		"publish_copy_generator": {
			Name: "publish_copy_generator",
			Parameters: map[string]tool.ParamDef{
				"script": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"title": {Type: "string"},
			},
		},
	}
	compiler := NewPlanCompiler(catalog)
	plan := &AgentPlan{
		Goal:   "帮我介绍一下佛得角国家以及说明佛得角世界杯小组赛出线进入淘汰赛是一个奇迹",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{
				ID:              "news_search",
				Tool:            "news_search",
				Arguments:       map[string]interface{}{"query": "Cape Verde World Cup"},
				ExpectedOutput:  []string{"facts", "sources", "script"},
				ProduceArtifact: true,
			},
			{
				ID:              "script_generation",
				Tool:            "video_script_generator",
				DependsOn:       []string{"news_search"},
				Arguments:       map[string]interface{}{"topic": "佛得角世界杯奇迹"},
				ExpectedOutput:  []string{"script"},
				ProduceArtifact: true,
			},
			{
				ID:        "beat_plan",
				Tool:      "shot_splitter",
				DependsOn: []string{"news_search"},
				Arguments: map[string]interface{}{
					"script": "{{news_search.output.script}}",
				},
				ExpectedOutput:  []string{"shotList"},
				ProduceArtifact: true,
			},
		},
	}

	prepared := compiler.PreparePlan(plan)

	beat := findStep(t, prepared, "beat_plan")
	if got := beat.Arguments["script"]; got != "{{script_generation.output.script}}" {
		t.Fatalf("beat_plan should reference script producer, got %#v", beat.Arguments)
	}
	if !containsString(beat.DependsOn, "script_generation") {
		t.Fatalf("beat_plan should depend on script_generation, got %#v", beat.DependsOn)
	}
	if err := NewPlanGuard(catalog, nil).Validate(prepared); err != nil {
		t.Fatalf("prepared plan should pass PlanGuard: %v", err)
	}
}

type staticToolCatalog map[string]*tool.ToolManifest

func (c staticToolCatalog) GetManifest(name string) *tool.ToolManifest {
	return c[name]
}

func findStep(t *testing.T, plan *AgentPlan, id string) AgentStep {
	t.Helper()
	for _, step := range plan.Steps {
		if step.ID == id {
			return step
		}
	}
	t.Fatalf("step %s not found in %#v", id, plan.Steps)
	return AgentStep{}
}

func stepIndex(t *testing.T, plan *AgentPlan, id string) int {
	t.Helper()
	for i, step := range plan.Steps {
		if step.ID == id {
			return i
		}
	}
	t.Fatalf("step %s not found in %#v", id, plan.Steps)
	return -1
}

func requireStepDeps(t *testing.T, step AgentStep, want []string) {
	t.Helper()
	if len(step.DependsOn) != len(want) {
		t.Fatalf("step %s depends on %#v, want %#v", step.ID, step.DependsOn, want)
	}
	for i, dep := range want {
		if step.DependsOn[i] != dep {
			t.Fatalf("step %s depends on %#v, want %#v", step.ID, step.DependsOn, want)
		}
	}
}

func assertStepOrder(t *testing.T, plan *AgentPlan, want []string) {
	t.Helper()
	last := -1
	for _, id := range want {
		index := stepIndex(t, plan, id)
		if index <= last {
			t.Fatalf("step %s index %d should be after prior index %d in %#v", id, index, last, plan.Steps)
		}
		last = index
	}
}

func videoProfileTemplateCatalog() staticToolCatalog {
	catalog := videoBetaCompletionCatalog()
	catalog["video_script_generator"].Parameters = map[string]tool.ParamDef{
		"topic": {Type: "string", Required: true},
	}
	catalog["video_profile_classifier"] = &tool.ToolManifest{
		Name: "video_profile_classifier",
		Parameters: map[string]tool.ParamDef{
			"stage": {Type: "string", Required: true},
			"brief": {Type: "string", Required: true},
			"route": {Type: "string", Required: true},
		},
		Output: map[string]tool.ParamDef{
			"creationProfile": {Type: "string"},
			"routingReason":   {Type: "string"},
		},
	}
	catalog["time_window_planner"] = &tool.ToolManifest{
		Name: "time_window_planner",
		Parameters: map[string]tool.ParamDef{
			"brief":           {Type: "string", Required: true},
			"creationProfile": {Type: "string", Required: true},
			"scriptSpans":     {Type: "string", Required: false},
			"shotList":        {Type: "array", Required: false},
		},
		Output: map[string]tool.ParamDef{
			"timeWindows": {Type: "array"},
		},
	}
	catalog["visual_alignment_planner"] = &tool.ToolManifest{
		Name: "visual_alignment_planner",
		Parameters: map[string]tool.ParamDef{
			"brief":           {Type: "string", Required: true},
			"script":          {Type: "string", Required: true},
			"timeWindows":     {Type: "array", Required: true},
			"creationProfile": {Type: "string", Required: true},
		},
		Output: map[string]tool.ParamDef{
			"shotList": {Type: "array"},
		},
	}
	catalog["shot_generation_planner"] = &tool.ToolManifest{
		Name: "shot_generation_planner",
		Parameters: map[string]tool.ParamDef{
			"shotList":           {Type: "array", Required: true},
			"timeWindows":        {Type: "array", Required: false},
			"creationProfile":    {Type: "string", Required: true},
			"referenceAssetPlan": {Type: "object", Required: false},
			"continuityBible":    {Type: "object", Required: false},
			"keyframePrompts":    {Type: "array", Required: false},
		},
		Output: map[string]tool.ParamDef{
			"shotGenerationPlans":        {Type: "array"},
			"shotAssetPackages":          {Type: "array"},
			"externalGenerationRequests": {Type: "array"},
		},
	}
	catalog["proposal_generator"] = &tool.ToolManifest{
		Name:       "proposal_generator",
		Parameters: map[string]tool.ParamDef{"brief": {Type: "string", Required: true}},
		Output:     map[string]tool.ParamDef{"proposalPacket": {Type: "object"}},
	}
	catalog["continuity_checker"] = &tool.ToolManifest{
		Name: "continuity_checker",
		Parameters: map[string]tool.ParamDef{
			"script": {Type: "string", Required: true},
		},
		Output: map[string]tool.ParamDef{
			"continuityReport": {Type: "object"},
			"styleProfile":     {Type: "object"},
		},
	}
	catalog["reference_asset_planner"] = &tool.ToolManifest{
		Name: "reference_asset_planner",
		Parameters: map[string]tool.ParamDef{
			"brief":           {Type: "string", Required: true},
			"script":          {Type: "string", Required: true},
			"continuityBible": {Type: "object", Required: true},
			"creationProfile": {Type: "string", Required: true},
		},
		Output: map[string]tool.ParamDef{
			"referenceAssetPlan": {Type: "object"},
		},
	}
	catalog["cinematic_shot_designer"] = &tool.ToolManifest{
		Name: "cinematic_shot_designer",
		Parameters: map[string]tool.ParamDef{
			"script":             {Type: "string", Required: true},
			"continuityBible":    {Type: "object", Required: true},
			"referenceAssetPlan": {Type: "object", Required: true},
			"creationProfile":    {Type: "string", Required: true},
		},
		Output: map[string]tool.ParamDef{
			"shotList": {Type: "array"},
		},
	}
	catalog["keyframe_prompt_generator"] = &tool.ToolManifest{
		Name: "keyframe_prompt_generator",
		Parameters: map[string]tool.ParamDef{
			"shotList":           {Type: "array", Required: true},
			"timeWindows":        {Type: "array", Required: true},
			"referenceAssetPlan": {Type: "object", Required: true},
			"continuityBible":    {Type: "object", Required: false},
			"creationProfile":    {Type: "string", Required: false},
		},
		Output: map[string]tool.ParamDef{
			"keyframePrompts": {Type: "array"},
			"summary":         {Type: "string"},
		},
	}
	return catalog
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
