package agentruntime

import (
	"fmt"
	"strings"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

type ToolCatalog interface {
	GetManifest(name string) *tool.ToolManifest
}

// StageDirector provides stage-level tool constraints.
type StageDirector interface {
	Name() string
	StageName() string
	AllowedTools() []string
	ForbiddenTools() []string
	MaxToolCalls() int
	RequiresApproval() bool
}

type RoleAgentDirector interface {
	StageDirector
	RoleID() string
	DisplayName() string
	Goal() string
	RequiredInputs() []string
	RequiredOutputs() []string
	HumanReview() *tool.HumanReview
}

// DirectorRegistry maps stage names to their Directors.
type DirectorRegistry interface {
	Get(stageName string) StageDirector
}

type PlanCompiler struct {
	tools     ToolCatalog
	directors DirectorRegistry
}

func NewPlanCompiler(tools ToolCatalog) *PlanCompiler {
	return &PlanCompiler{tools: tools}
}

// WithDirectors injects a DirectorRegistry for stage-level tool enforcement.
func (c *PlanCompiler) WithDirectors(directors DirectorRegistry) *PlanCompiler {
	c.directors = directors
	return c
}

func (c *PlanCompiler) Compile(plan *AgentPlan) (*model.DAGRequest, error) {
	if plan == nil {
		return nil, fmt.Errorf("agent plan is required")
	}
	plan = c.PreparePlan(plan)
	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("agent plan has no steps")
	}

	// Detect missing quality checkers and auto-insert them.
	steps := c.injectQualityGates(plan.Steps)

	var err error
	steps, err = c.applyDirectors(steps, plan.Domain)
	if err != nil {
		return nil, err
	}

	nodes := make([]model.NodeRequest, 0, len(steps)*2)
	edges := make([]model.Edge, 0, len(steps)*2)
	stepOutputs := make(map[string][]string, len(steps))

	for _, step := range steps {
		if step.ID == "" {
			return nil, fmt.Errorf("agent step id is required")
		}
		if step.Tool == "" {
			return nil, fmt.Errorf("agent step %s has no tool", step.ID)
		}

		manifest := c.manifestFor(step.Tool)
		compiled, err := compileStep(step, manifest)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, compiled.nodes...)
		edges = append(edges, compiled.internalEdges...)

		for _, depStep := range step.DependsOn {
			depOutputs, ok := stepOutputs[depStep]
			if !ok {
				return nil, fmt.Errorf("step %s depends on unknown step %s", step.ID, depStep)
			}
			for _, from := range depOutputs {
				for _, to := range compiled.entryIDs {
					edges = append(edges, model.Edge{From: from, To: to})
				}
			}
		}

		stepOutputs[step.ID] = compiled.outputIDs
	}

	return &model.DAGRequest{Nodes: nodes, Edges: edges}, nil
}

// PreparePlan completes policy-driven steps that must exist before Guard and
// DAG compilation. It is idempotent and mutates the supplied plan.
func (c *PlanCompiler) PreparePlan(plan *AgentPlan) *AgentPlan {
	if plan == nil {
		return nil
	}
	c.injectKnowledgeContext(plan)
	if !c.completeVideoPlanByProfile(plan) {
		c.completeVideoBetaPlan(plan)
	}
	c.injectKnowledgeContext(plan)
	c.injectMCPGenerationRunner(plan)
	repairInvalidOutputReferences(plan.Steps, c.manifestsByPlan(plan))
	normalizePreparedPlanDependencies(plan)
	c.expandPreparedPlanBudget(plan)
	return plan
}

func (c *PlanCompiler) expandPreparedPlanBudget(plan *AgentPlan) {
	if plan == nil {
		return
	}
	if plan.Budget.MaxSteps > 0 && len(plan.Steps) > plan.Budget.MaxSteps {
		plan.Budget.MaxSteps = len(plan.Steps)
	}
	if plan.Budget.MaxToolCalls > 0 && len(plan.Steps) > plan.Budget.MaxToolCalls {
		plan.Budget.MaxToolCalls = len(plan.Steps)
	}
	for _, step := range plan.Steps {
		manifest := c.manifestFor(step.Tool)
		if manifest == nil || manifest.CostLevel == "" {
			continue
		}
		if plan.Budget.MaxCostLevel == "" || costRank(manifest.CostLevel) > costRank(plan.Budget.MaxCostLevel) {
			plan.Budget.MaxCostLevel = manifest.CostLevel
		}
	}
}

func normalizePreparedPlanDependencies(plan *AgentPlan) {
	if plan == nil {
		return
	}
	indexByID := make(map[string]int, len(plan.Steps))
	for i, step := range plan.Steps {
		if step.ID != "" {
			indexByID[step.ID] = i
		}
	}
	for i := range plan.Steps {
		step := &plan.Steps[i]
		seen := map[string]bool{}
		depCandidates := make([]string, 0, len(step.DependsOn)+4)
		depCandidates = append(depCandidates, step.DependsOn...)
		depCandidates = append(depCandidates, referencedStepIDs(step.Arguments)...)
		deps := make([]string, 0, len(depCandidates))
		for _, dep := range depCandidates {
			dep = strings.TrimSpace(dep)
			if dep == "" || dep == step.ID || seen[dep] {
				continue
			}
			depIndex, ok := indexByID[dep]
			if !ok || depIndex >= i {
				continue
			}
			seen[dep] = true
			deps = append(deps, dep)
		}
		step.DependsOn = deps
	}
}

func referencedStepIDs(value interface{}) []string {
	seen := map[string]bool{}
	refs := make([]string, 0)
	var walk func(interface{})
	walk = func(current interface{}) {
		switch typed := current.(type) {
		case string:
			refStepID, _, ok := outputReference(typed)
			if ok && refStepID != "" && !seen[refStepID] {
				seen[refStepID] = true
				refs = append(refs, refStepID)
			}
		case []interface{}:
			for _, item := range typed {
				walk(item)
			}
		case []string:
			for _, item := range typed {
				walk(item)
			}
		case map[string]interface{}:
			for _, item := range typed {
				walk(item)
			}
		case map[string]string:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(value)
	return refs
}

func (c *PlanCompiler) manifestsByPlan(plan *AgentPlan) map[string]*tool.ToolManifest {
	result := make(map[string]*tool.ToolManifest, len(plan.Steps))
	if c == nil || c.tools == nil || plan == nil {
		return result
	}
	for _, step := range plan.Steps {
		if step.Tool == "" {
			continue
		}
		if _, ok := result[step.Tool]; ok {
			continue
		}
		if manifest := c.manifestFor(step.Tool); manifest != nil {
			result[step.Tool] = manifest
		}
	}
	return result
}

func (c *PlanCompiler) completeVideoBetaPlan(plan *AgentPlan) {
	if plan == nil || plan.Domain != "video_creation" || len(plan.Steps) == 0 {
		return
	}
	if !c.hasVideoBetaCompletionTools() {
		return
	}
	scriptAnchor, scriptField := c.lastProducerStepForFields(plan, []string{"script"}, []string{
		"video_script_generator",
		"script_generator",
	})
	if scriptAnchor == "" {
		return
	}
	scriptRef := stepOutputRef(scriptAnchor, scriptField)

	shotAnchor, shotField := c.lastProducerStepForFields(plan, []string{"shotList"}, []string{"shot_splitter"})
	if shotAnchor == "" {
		shotAnchor = appendPlanStep(plan, AgentStep{
			ID:        uniqueStepID(plan, "beat_plan"),
			Intent:    "将口播稿拆成可视化节奏、分镜和画面段落",
			Tool:      "shot_splitter",
			DependsOn: dependencyList(scriptAnchor),
			Arguments: map[string]interface{}{
				"stage":  "beat",
				"script": scriptRef,
				"topic":  plan.Goal,
			},
			ExpectedOutput:  []string{"beat_plan", "shot_list"},
			ProduceArtifact: true,
		})
		shotField = preferredOutputField(c.manifestFor("shot_splitter"), "shotList")
	}

	generationAnchor, generationField := c.lastProducerStepForFields(plan, []string{"shotGenerationPlans"}, []string{"shot_generation_planner"})
	if generationAnchor == "" && c.hasOptionalTool("shot_generation_planner") {
		generationAnchor = insertPlanStepAfter(plan, shotAnchor, AgentStep{
			ID:        uniqueStepID(plan, "shot_generation"),
			Intent:    "为每个分镜决定 AIGC、HyperFrames、混合生成、用户素材或占位素材策略",
			Tool:      "shot_generation_planner",
			DependsOn: dependencyList(shotAnchor),
			Arguments: map[string]interface{}{
				"stage":    "generation_strategy",
				"brief":    plan.Goal,
				"shotList": stepOutputRef(shotAnchor, shotField),
			},
			ExpectedOutput:  []string{"shotGenerationPlans", "shotAssetPackages", "externalGenerationRequests"},
			ProduceArtifact: true,
		})
		generationField = preferredOutputField(c.manifestFor("shot_generation_planner"), "shotGenerationPlans")
	}

	c.completeVideoOutputPlanFromAnchors(plan, scriptAnchor, scriptField, shotAnchor, shotField, generationAnchor, generationField)
}

func (c *PlanCompiler) completeVideoPlanByProfile(plan *AgentPlan) bool {
	if plan == nil || plan.Domain != "video_creation" {
		return false
	}
	if c.manifestFor("video_profile_classifier") == nil || c.manifestFor("time_window_planner") == nil {
		return false
	}
	profile := requestedVideoCreationProfile(plan)
	if profile == "" {
		profile = inferVideoCreationProfile(plan.Goal)
	}
	switch profile {
	case "cinematic_story":
		if !c.canCompleteCinematicProfilePlan() {
			return false
		}
		profileAnchor := c.ensureProfileSelectionStep(plan, profile)
		c.completeCinematicProfilePlan(plan, profileAnchor)
		return true
	default:
		if !c.canCompleteTalkingHeadProfilePlan(plan) {
			return false
		}
		profileAnchor := c.ensureProfileSelectionStep(plan, "talking_head")
		return c.completeTalkingHeadProfilePlan(plan, profileAnchor)
	}
}

func inferVideoCreationProfile(goal string) string {
	normalized := strings.ToLower(goal)
	for _, term := range []string{
		"影视", "剧情", "角色", "场景", "道具", "导演", "短片",
		"cinematic", "story",
	} {
		if strings.Contains(normalized, strings.ToLower(term)) {
			return "cinematic_story"
		}
	}
	for _, term := range []string{
		"口播", "知识", "讲解", "分享", "voiceover", "talking head",
	} {
		if strings.Contains(normalized, strings.ToLower(term)) {
			return "talking_head"
		}
	}
	return "talking_head"
}

func requestedVideoCreationProfile(plan *AgentPlan) string {
	if plan == nil {
		return ""
	}
	for _, step := range plan.Steps {
		for _, key := range []string{
			"profileId",
			"profile_id",
			"videoType",
			"video_type",
			"creationProfile",
			"creation_profile",
			"pipeline",
			"pipelineId",
			"pipeline_id",
		} {
			raw, ok := step.Arguments[key].(string)
			if !ok {
				continue
			}
			switch canonicalVideoCreationProfile(raw) {
			case "cinematic_story":
				return "cinematic_story"
			case "talking_head":
				return "talking_head"
			}
		}
	}
	return ""
}

func canonicalVideoCreationProfile(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" || strings.Contains(normalized, "{{") {
		return ""
	}
	for _, term := range []string{
		"aigc_shot",
		"aigc-shot",
		"cinematic_story",
		"cinematic-story",
		"cinematic",
		"story",
		"wf-aigc-shot-video",
		"jimeng",
	} {
		if strings.Contains(normalized, term) {
			return "cinematic_story"
		}
	}
	for _, term := range []string{
		"voice_visual",
		"voice-visual",
		"talking_head",
		"talking-head",
		"voiceover",
		"guided_image_text",
		"guided-image-text",
		"wf-guided-image-text-video",
	} {
		if strings.Contains(normalized, term) {
			return "talking_head"
		}
	}
	return ""
}

func (c *PlanCompiler) canCompleteTalkingHeadProfilePlan(plan *AgentPlan) bool {
	if c.manifestFor("visual_alignment_planner") == nil ||
		c.manifestFor("shot_generation_planner") == nil ||
		!c.hasVideoOutputCompletionTools() {
		return false
	}
	if scriptAnchor, _ := c.lastProducerStepForFields(plan, []string{"script"}, []string{
		"video_script_generator",
		"script_generator",
	}); scriptAnchor != "" {
		return true
	}
	return c.manifestFor("video_script_generator") != nil || c.manifestFor("script_generator") != nil
}

func (c *PlanCompiler) canCompleteCinematicProfilePlan() bool {
	for _, name := range []string{
		"proposal_generator",
		"video_script_generator",
		"continuity_checker",
		"reference_asset_planner",
		"cinematic_shot_designer",
		"keyframe_prompt_generator",
		"shot_generation_planner",
	} {
		if c.manifestFor(name) == nil {
			return false
		}
	}
	return c.hasVideoOutputCompletionTools()
}

func (c *PlanCompiler) completeTalkingHeadProfilePlan(plan *AgentPlan, profileAnchor string) bool {
	scriptAnchor, scriptField := c.lastProducerStepForFields(plan, []string{"script"}, []string{
		"video_script_generator",
		"script_generator",
	})
	if scriptAnchor == "" {
		scriptTool := "video_script_generator"
		if c.manifestFor(scriptTool) == nil {
			scriptTool = "script_generator"
		}
		if c.manifestFor(scriptTool) == nil {
			return false
		}
		insertAfter := profileAnchor
		scriptDeps := dependencyList(profileAnchor)
		scriptArgs := map[string]interface{}{
			"stage":           "script_generation",
			"brief":           plan.Goal,
			"topic":           plan.Goal,
			"creationProfile": stepOutputRef(profileAnchor, "creationProfile"),
		}
		if proposalAnchor, proposalField := c.lastProducerStepForFields(plan, []string{"proposalPacket", "proposal", "creativeBrief"}, []string{"proposal_generator"}); proposalAnchor != "" {
			insertAfter = proposalAnchor
			scriptDeps = dependencyListUnique(proposalAnchor, profileAnchor)
			if proposalField != "" {
				scriptArgs["proposal"] = stepOutputRef(proposalAnchor, proposalField)
			}
		} else if knowledgeAnchor, _ := c.lastProducerStepForFields(plan, []string{"facts", "sources", "summary", "knowledge"}, []string{"knowledge_researcher", "news_search", "fact_checker"}); knowledgeAnchor != "" {
			insertAfter = knowledgeAnchor
			scriptDeps = dependencyListUnique(knowledgeAnchor, profileAnchor)
		}
		scriptAnchor = insertPlanStepAfter(plan, insertAfter, AgentStep{
			ID:              uniqueStepID(plan, "script_generation"),
			Intent:          "生成口播主线脚本",
			Tool:            scriptTool,
			DependsOn:       scriptDeps,
			Arguments:       scriptArgs,
			ExpectedOutput:  []string{"script", "scriptSpans"},
			ProduceArtifact: true,
		})
		scriptField = preferredOutputField(c.manifestFor(scriptTool), "script")
	} else {
		scriptAnchor = canonicalizePlanStep(plan, scriptAnchor, "script_generation")
		movePlanStepAfter(plan, scriptAnchor, profileAnchor)
		scriptStep := planStepByID(plan, scriptAnchor)
		if scriptStep != nil {
			if scriptStep.Arguments == nil {
				scriptStep.Arguments = map[string]interface{}{}
			}
			scriptStep.Arguments["stage"] = "script_generation"
			scriptStep.Arguments["brief"] = plan.Goal
			scriptStep.Arguments["topic"] = plan.Goal
			scriptStep.Arguments["creationProfile"] = stepOutputRef(profileAnchor, "creationProfile")
			scriptStep.DependsOn = dependencyList(profileAnchor)
			if len(scriptStep.ExpectedOutput) == 0 {
				scriptStep.ExpectedOutput = []string{"script", "scriptSpans"}
			} else if field := firstManifestOutput(c.manifestFor(scriptStep.Tool), "scriptSpans"); field != "" && !containsString(scriptStep.ExpectedOutput, field) {
				scriptStep.ExpectedOutput = append(scriptStep.ExpectedOutput, field)
			}
			scriptStep.ProduceArtifact = true
			scriptField = preferredOutputField(c.manifestFor(scriptStep.Tool), "script")
		}
	}
	scriptSpanField := "scriptSpans"
	if scriptStep := planStepByID(plan, scriptAnchor); scriptStep != nil {
		scriptSpanField = preferredOutputField(c.manifestFor(scriptStep.Tool), "scriptSpans", "sections", "script")
	}
	scriptRef := stepOutputRef(scriptAnchor, scriptField)
	scriptSpansRef := stepOutputRef(scriptAnchor, scriptSpanField)
	profileRef := stepOutputRef(profileAnchor, "creationProfile")

	if splitter := planStepByTool(plan, "shot_splitter"); splitter != nil {
		splitterID := canonicalizePlanStep(plan, splitter.ID, "shot_split")
		movePlanStepAfter(plan, splitterID, scriptAnchor)
		if splitterStep := planStepByID(plan, splitterID); splitterStep != nil {
			if splitterStep.Arguments == nil {
				splitterStep.Arguments = map[string]interface{}{}
			}
			splitterStep.Arguments["stage"] = "shot_split"
			splitterStep.Arguments["script"] = scriptRef
			splitterStep.DependsOn = dependencyList(scriptAnchor)
			if len(splitterStep.ExpectedOutput) == 0 {
				splitterStep.ExpectedOutput = []string{"shotList"}
			}
			splitterStep.ProduceArtifact = true
		}
	}

	timeWindowAnchor := c.ensureProfileStepAfter(plan, "time_window", scriptAnchor, AgentStep{
		ID:        "time_window",
		Intent:    "按口播稿时间轴规划可执行的画面时间窗",
		Tool:      "time_window_planner",
		DependsOn: dependencyListUnique(scriptAnchor, profileAnchor),
		Arguments: map[string]interface{}{
			"stage":           "time_window",
			"brief":           plan.Goal,
			"scriptSpans":     scriptSpansRef,
			"creationProfile": profileRef,
		},
		ExpectedOutput:  []string{"timeWindows"},
		ProduceArtifact: true,
	})
	timeWindowStep := planStepByID(plan, timeWindowAnchor)
	mergeStepArgsAndDeps(timeWindowStep, map[string]interface{}{
		"creationProfile": profileRef,
		"scriptSpans":     scriptSpansRef,
	}, scriptAnchor, profileAnchor)

	visualAnchor := c.ensureProfileStepAfter(plan, "visual_alignment", timeWindowAnchor, AgentStep{
		ID:        "visual_alignment",
		Intent:    "将口播脚本与素材、字幕和画面服务关系对齐成分镜清单",
		Tool:      "visual_alignment_planner",
		DependsOn: dependencyListUnique(scriptAnchor, timeWindowAnchor, profileAnchor),
		Arguments: map[string]interface{}{
			"stage":           "visual_alignment",
			"brief":           plan.Goal,
			"script":          scriptRef,
			"timeWindows":     stepOutputRef(timeWindowAnchor, "timeWindows"),
			"creationProfile": profileRef,
		},
		ExpectedOutput:  []string{"shotList"},
		ProduceArtifact: true,
	})
	visualStep := planStepByID(plan, visualAnchor)
	mergeStepArgsAndDeps(visualStep, map[string]interface{}{
		"script":          scriptRef,
		"timeWindows":     stepOutputRef(timeWindowAnchor, "timeWindows"),
		"creationProfile": profileRef,
	}, scriptAnchor, timeWindowAnchor, profileAnchor)

	generationAnchor := c.ensureProfileStepAfter(plan, "shot_generation", visualAnchor, AgentStep{
		ID:        "shot_generation",
		Intent:    "为口播画面段落决定 AIGC、素材、字幕和占位画面生成策略",
		Tool:      "shot_generation_planner",
		DependsOn: dependencyListUnique(visualAnchor, timeWindowAnchor, profileAnchor),
		Arguments: map[string]interface{}{
			"stage":           "generation_strategy",
			"brief":           plan.Goal,
			"shotList":        stepOutputRef(visualAnchor, "shotList"),
			"timeWindows":     stepOutputRef(timeWindowAnchor, "timeWindows"),
			"creationProfile": profileRef,
		},
		ExpectedOutput:  []string{"shotGenerationPlans", "shotAssetPackages", "externalGenerationRequests"},
		ProduceArtifact: true,
	})
	generationStep := planStepByID(plan, generationAnchor)
	mergeStepArgsAndDeps(generationStep, map[string]interface{}{
		"shotList":        stepOutputRef(visualAnchor, "shotList"),
		"timeWindows":     stepOutputRef(timeWindowAnchor, "timeWindows"),
		"creationProfile": profileRef,
	}, visualAnchor, timeWindowAnchor, profileAnchor)

	generationField := preferredOutputField(c.manifestFor("shot_generation_planner"), "shotGenerationPlans")
	c.completeVideoOutputPlanFromAnchors(plan, scriptAnchor, scriptField, visualAnchor, "shotList", generationAnchor, generationField)
	return true
}

func (c *PlanCompiler) completeCinematicProfilePlan(plan *AgentPlan, profileAnchor string) {
	profileRef := stepOutputRef(profileAnchor, "creationProfile")
	storyField := preferredOutputField(c.manifestFor("proposal_generator"), "proposalPacket", "proposal")
	storyAnchor := c.ensureProfileStepAfter(plan, "story_foundation", profileAnchor, AgentStep{
		ID:        "story_foundation",
		Intent:    "建立影视短片的故事基础、主题、人物和冲突方向",
		Tool:      "proposal_generator",
		DependsOn: dependencyList(profileAnchor),
		Arguments: map[string]interface{}{
			"stage":           "story_foundation",
			"brief":           plan.Goal,
			"creationProfile": profileRef,
		},
		ExpectedOutput:  []string{storyField},
		ProduceArtifact: true,
	})
	storyStep := planStepByID(plan, storyAnchor)
	mergeStepArgsAndDeps(storyStep, map[string]interface{}{"creationProfile": profileRef}, profileAnchor)
	if storyField != "" && !containsString(storyStep.ExpectedOutput, storyField) {
		storyStep.ExpectedOutput = append(storyStep.ExpectedOutput, storyField)
	}
	storyRef := stepOutputRef(storyAnchor, storyField)

	scriptAnchor := c.ensureProfileStepAfter(plan, "cinematic_script", storyAnchor, AgentStep{
		ID:        "cinematic_script",
		Intent:    "生成包含角色、场景和动作连续性的影视短片剧本",
		Tool:      "video_script_generator",
		DependsOn: dependencyListUnique(storyAnchor, profileAnchor),
		Arguments: map[string]interface{}{
			"stage":           "cinematic_script",
			"brief":           plan.Goal,
			"topic":           plan.Goal,
			"proposal":        storyRef,
			"creationProfile": profileRef,
		},
		ExpectedOutput:  []string{"script"},
		ProduceArtifact: true,
	})
	scriptStep := planStepByID(plan, scriptAnchor)
	mergeStepArgsAndDeps(scriptStep, map[string]interface{}{
		"topic":           plan.Goal,
		"proposal":        storyRef,
		"creationProfile": profileRef,
	}, storyAnchor, profileAnchor)
	scriptRef := stepOutputRef(scriptAnchor, "script")

	continuityField := preferredOutputField(c.manifestFor("continuity_checker"), "continuityBible", "continuityReport", "styleProfile", "report")
	continuityAnchor := c.ensureProfileStepAfter(plan, "continuity_bible", scriptAnchor, AgentStep{
		ID:        "continuity_bible",
		Intent:    "整理角色、场景、道具、风格和连续性圣经",
		Tool:      "continuity_checker",
		DependsOn: dependencyListUnique(scriptAnchor, profileAnchor),
		Arguments: map[string]interface{}{
			"stage":           "continuity_bible",
			"script":          scriptRef,
			"creationProfile": profileRef,
		},
		ExpectedOutput:  []string{continuityField},
		ProduceArtifact: true,
	})
	continuityStep := planStepByID(plan, continuityAnchor)
	mergeStepArgsAndDeps(continuityStep, map[string]interface{}{
		"script":          scriptRef,
		"creationProfile": profileRef,
	}, scriptAnchor, profileAnchor)
	if continuityField != "" && !containsString(continuityStep.ExpectedOutput, continuityField) {
		continuityStep.ExpectedOutput = append(continuityStep.ExpectedOutput, continuityField)
	}
	continuityRef := stepOutputRef(continuityAnchor, continuityField)

	referenceAnchor := c.ensureProfileStepAfter(plan, "reference_assets", continuityAnchor, AgentStep{
		ID:        "reference_assets",
		Intent:    "规划角色、场景、道具和风格参考资产",
		Tool:      "reference_asset_planner",
		DependsOn: dependencyListUnique(scriptAnchor, continuityAnchor, profileAnchor),
		Arguments: map[string]interface{}{
			"stage":           "reference_assets",
			"brief":           plan.Goal,
			"script":          scriptRef,
			"continuityBible": continuityRef,
			"creationProfile": profileRef,
		},
		ExpectedOutput:  []string{"referenceAssetPlan"},
		ProduceArtifact: true,
	})
	referenceStep := planStepByID(plan, referenceAnchor)
	mergeStepArgsAndDeps(referenceStep, map[string]interface{}{
		"script":          scriptRef,
		"continuityBible": continuityRef,
		"creationProfile": profileRef,
	}, scriptAnchor, continuityAnchor, profileAnchor)
	referenceRef := stepOutputRef(referenceAnchor, "referenceAssetPlan")

	shotDesignAnchor := c.ensureProfileStepAfter(plan, "cinematic_shot_design", referenceAnchor, AgentStep{
		ID:        "cinematic_shot_design",
		Intent:    "设计导演分镜、镜头调度和粗颗粒剧情镜头清单",
		Tool:      "cinematic_shot_designer",
		DependsOn: dependencyListUnique(scriptAnchor, continuityAnchor, referenceAnchor, profileAnchor),
		Arguments: map[string]interface{}{
			"stage":              "cinematic_shot_design",
			"script":             scriptRef,
			"continuityBible":    continuityRef,
			"referenceAssetPlan": referenceRef,
			"creationProfile":    profileRef,
		},
		ExpectedOutput:  []string{"shotList"},
		ProduceArtifact: true,
	})
	shotDesignStep := planStepByID(plan, shotDesignAnchor)
	mergeStepArgsAndDeps(shotDesignStep, map[string]interface{}{
		"script":             scriptRef,
		"continuityBible":    continuityRef,
		"referenceAssetPlan": referenceRef,
		"creationProfile":    profileRef,
	}, scriptAnchor, continuityAnchor, referenceAnchor, profileAnchor)

	timeWindowAnchor := c.ensureProfileStepAfter(plan, "time_window", shotDesignAnchor, AgentStep{
		ID:        "time_window",
		Intent:    "将影视粗分镜细拆成 3-15 秒 AIGC 生成时间窗",
		Tool:      "time_window_planner",
		DependsOn: dependencyListUnique(shotDesignAnchor, profileAnchor),
		Arguments: map[string]interface{}{
			"stage":           "time_window",
			"brief":           plan.Goal,
			"shotList":        stepOutputRef(shotDesignAnchor, "shotList"),
			"creationProfile": profileRef,
		},
		ExpectedOutput:  []string{"timeWindows"},
		ProduceArtifact: true,
	})
	timeWindowStep := planStepByID(plan, timeWindowAnchor)
	mergeStepArgsAndDeps(timeWindowStep, map[string]interface{}{
		"shotList":        stepOutputRef(shotDesignAnchor, "shotList"),
		"creationProfile": profileRef,
	}, shotDesignAnchor, profileAnchor)
	timeWindowRef := stepOutputRef(timeWindowAnchor, "timeWindows")

	keyframeField := preferredOutputField(c.manifestFor("keyframe_prompt_generator"), "keyframeStoryboards", "keyframePrompts", "summary")
	keyframesAnchor := c.ensureProfileStepAfter(plan, "keyframes_storyboards", timeWindowAnchor, AgentStep{
		ID:        "keyframes_storyboards",
		Intent:    "基于参考资产和细分时间窗生成关键帧与故事板提示",
		Tool:      "keyframe_prompt_generator",
		DependsOn: dependencyListUnique(referenceAnchor, timeWindowAnchor),
		Arguments: map[string]interface{}{
			"stage":              "keyframes_storyboards",
			"shotList":           timeWindowRef,
			"timeWindows":        timeWindowRef,
			"referenceAssetPlan": referenceRef,
		},
		ExpectedOutput:  []string{keyframeField},
		ProduceArtifact: true,
	})
	keyframeStep := planStepByID(plan, keyframesAnchor)
	mergeStepArgsAndDeps(keyframeStep, map[string]interface{}{
		"shotList":           timeWindowRef,
		"timeWindows":        timeWindowRef,
		"referenceAssetPlan": referenceRef,
	}, referenceAnchor, timeWindowAnchor)
	if keyframeField != "" && !containsString(keyframeStep.ExpectedOutput, keyframeField) {
		keyframeStep.ExpectedOutput = append(keyframeStep.ExpectedOutput, keyframeField)
	}
	keyframeRef := stepOutputRef(keyframesAnchor, keyframeField)

	generationAnchor := c.ensureProfileStepAfter(plan, "shot_generation", keyframesAnchor, AgentStep{
		ID:        "shot_generation",
		Intent:    "为细分影视时间窗规划生成策略、参考资产和外部 AIGC 请求",
		Tool:      "shot_generation_planner",
		DependsOn: dependencyListUnique(timeWindowAnchor, referenceAnchor, continuityAnchor, keyframesAnchor, profileAnchor),
		Arguments: map[string]interface{}{
			"stage":              "generation_strategy",
			"brief":              plan.Goal,
			"shotList":           timeWindowRef,
			"timeWindows":        timeWindowRef,
			"creationProfile":    profileRef,
			"referenceAssetPlan": referenceRef,
			"continuityBible":    continuityRef,
			"keyframePrompts":    keyframeRef,
		},
		ExpectedOutput:  []string{"shotGenerationPlans", "shotAssetPackages", "externalGenerationRequests"},
		ProduceArtifact: true,
	})
	generationStep := planStepByID(plan, generationAnchor)
	mergeStepArgsAndDeps(generationStep, map[string]interface{}{
		"shotList":           timeWindowRef,
		"timeWindows":        timeWindowRef,
		"creationProfile":    profileRef,
		"referenceAssetPlan": referenceRef,
		"continuityBible":    continuityRef,
		"keyframePrompts":    keyframeRef,
	}, timeWindowAnchor, referenceAnchor, continuityAnchor, keyframesAnchor, profileAnchor)

	generationField := preferredOutputField(c.manifestFor("shot_generation_planner"), "shotGenerationPlans")
	c.completeVideoOutputPlanFromAnchors(plan, scriptAnchor, "script", timeWindowAnchor, "timeWindows", generationAnchor, generationField)
}

func (c *PlanCompiler) completeVideoOutputPlanFromAnchors(plan *AgentPlan, scriptAnchor, scriptField, shotAnchor, shotField, generationAnchor, generationField string) {
	if plan == nil || scriptAnchor == "" || scriptField == "" || shotAnchor == "" || shotField == "" {
		return
	}
	scriptRef := stepOutputRef(scriptAnchor, scriptField)
	generationPackageField := c.outputFieldForStep(plan, generationAnchor, "shotAssetPackages")
	projectID := requestedProjectID(plan)

	promptAnchor, promptField := c.lastVideoPromptProducer(plan)
	if promptAnchor == "" {
		promptArgs := map[string]interface{}{
			"stage":    "video_prompt",
			"brief":    plan.Goal,
			"shotList": stepOutputRef(shotAnchor, shotField),
		}
		if generationAnchor != "" && generationField != "" {
			promptArgs["shotGenerationPlans"] = stepOutputRef(generationAnchor, generationField)
		}
		if generationPackageField != "" {
			promptArgs["shotAssetPackages"] = stepOutputRef(generationAnchor, generationPackageField)
		}
		if provider := requestedAIGCProvider(plan); provider != "" {
			promptArgs["aigcProvider"] = provider
		}
		promptAnchor = appendPlanStep(plan, AgentStep{
			ID:              uniqueStepID(plan, "video_prompt"),
			Intent:          "根据分镜生成可审核的视频生成提示词",
			Tool:            "video_prompt_generator",
			DependsOn:       dependencyListUnique(shotAnchor, generationAnchor),
			Arguments:       promptArgs,
			ExpectedOutput:  []string{"videoPrompts", "video_prompt"},
			ProduceArtifact: true,
		})
		promptField = preferredOutputField(c.manifestFor("video_prompt_generator"), "videoPrompts", "video_prompt")
	} else {
		promptAnchor = canonicalizePlanStep(plan, promptAnchor, "video_prompt")
		movePlanStepAfter(plan, promptAnchor, firstNonEmptyStepID(generationAnchor, shotAnchor))
		if promptStep := planStepByID(plan, promptAnchor); promptStep != nil {
			promptStep.DependsOn = dependencyListUnique(shotAnchor, generationAnchor)
		}
	}
	rewireVideoPromptShotSource(planStepByID(plan, promptAnchor), shotAnchor, shotField)
	if provider := requestedAIGCProvider(plan); provider != "" {
		if promptStep := planStepByID(plan, promptAnchor); promptStep != nil {
			if promptStep.Arguments == nil {
				promptStep.Arguments = map[string]interface{}{}
			}
			promptStep.Arguments["aigcProvider"] = provider
		}
	}
	applyProjectContextToStep(planStepByID(plan, promptAnchor), projectID)
	c.augmentVideoPromptGenerationInputs(plan, promptAnchor, generationAnchor, generationField, generationPackageField)

	projectAnchor, projectField := c.lastProducerStepForFields(plan, []string{"projectDir", "hyperframesPath"}, []string{"hyperframes_project_generator"})
	if projectAnchor == "" {
		previewArgs := map[string]interface{}{
			"stage":    "preview",
			"brief":    plan.Goal,
			"topic":    plan.Goal,
			"script":   scriptRef,
			"shotList": stepOutputRef(shotAnchor, shotField),
		}
		if promptAnchor != "" && (promptField == "videoPrompts" || promptField == "video_prompt") {
			previewArgs["videoPrompts"] = stepOutputRef(promptAnchor, promptField)
		}
		projectManifest := c.manifestFor("hyperframes_project_generator")
		if generationAnchor != "" && generationField != "" {
			previewArgs["shotGenerationPlans"] = stepOutputRef(generationAnchor, generationField)
		}
		if packageField := c.outputFieldForStep(plan, promptAnchor, "shotAssetPackages"); packageField != "" {
			if manifestAcceptsParam(projectManifest, "shotAssetPackages") {
				previewArgs["shotAssetPackages"] = stepOutputRef(promptAnchor, packageField)
			}
		} else if generationPackageField != "" && manifestAcceptsParam(projectManifest, "shotAssetPackages") {
			previewArgs["shotAssetPackages"] = stepOutputRef(generationAnchor, generationPackageField)
		}
		projectAnchor = appendPlanStep(plan, AgentStep{
			ID:              uniqueStepID(plan, "preview"),
			Intent:          "生成可审核的 HyperFrames 预览项目和画面预览",
			Tool:            "hyperframes_project_generator",
			DependsOn:       dependencyListUnique(shotAnchor, promptAnchor, generationAnchor, scriptAnchor),
			Arguments:       previewArgs,
			ExpectedOutput:  []string{"HYPERFRAMES_PROJECT", "PREVIEW_SNAPSHOTS", "PREVIEW_REPORT", "hyperframes_project", "preview"},
			ProduceArtifact: true,
		})
		projectField = preferredOutputField(c.manifestFor("hyperframes_project_generator"), "projectDir", "hyperframesPath")
	} else {
		projectAnchor = canonicalizePlanStep(plan, projectAnchor, "preview")
		movePlanStepAfter(plan, projectAnchor, firstNonEmptyStepID(promptAnchor, generationAnchor, shotAnchor))
		projectStep := planStepByID(plan, projectAnchor)
		if projectStep != nil {
			if projectStep.Arguments == nil {
				projectStep.Arguments = map[string]interface{}{}
			}
			projectStep.Arguments["stage"] = "preview"
			projectStep.Arguments["brief"] = plan.Goal
			projectStep.Arguments["topic"] = plan.Goal
			projectStep.Arguments["script"] = scriptRef
			projectStep.Arguments["shotList"] = stepOutputRef(shotAnchor, shotField)
			if promptAnchor != "" && (promptField == "videoPrompts" || promptField == "video_prompt") {
				projectStep.Arguments["videoPrompts"] = stepOutputRef(promptAnchor, promptField)
			}
			projectStep.DependsOn = dependencyListUnique(shotAnchor, promptAnchor, generationAnchor, scriptAnchor)
			ensureStepExpectedOutputs(projectStep, "HYPERFRAMES_PROJECT", "PREVIEW_SNAPSHOTS", "PREVIEW_REPORT", "hyperframes_project", "preview")
			projectStep.ProduceArtifact = true
		}
	}
	applyProjectContextToStep(planStepByID(plan, projectAnchor), projectID)
	c.augmentPreviewGenerationInputs(plan, projectAnchor, promptAnchor, generationAnchor, generationField, generationPackageField)

	renderAnchor, _ := c.lastProducerStepForFields(plan, []string{"outputPath", "finalVideo", "video"}, []string{"hyperframes_renderer"})
	renderManifest := c.manifestFor("hyperframes_renderer")
	projectParam := preferredParamName(renderManifest, "projectDir", "hyperframesPath")
	if renderAnchor == "" {
		renderArgs := map[string]interface{}{
			"stage":           "render",
			"brief":           plan.Goal,
			projectParam:      stepOutputRef(projectAnchor, projectField),
			"previewApproved": true,
			"outputName":      "final.mp4",
		}
		if projectID != "" {
			renderArgs["projectId"] = projectID
		}
		if firstManifestOutput(renderManifest, "entry") != "" {
			renderArgs["entry"] = stepOutputRef(projectAnchor, "entry")
		}
		renderAnchor = appendPlanStep(plan, AgentStep{
			ID:              uniqueStepID(plan, "render"),
			Intent:          "在预览确认后渲染本地视频文件",
			Tool:            "hyperframes_renderer",
			DependsOn:       dependencyList(projectAnchor),
			Arguments:       renderArgs,
			ExpectedOutput:  []string{"VIDEO", "RENDER_REPORT", "final_video"},
			ProduceArtifact: true,
		})
	} else {
		renderAnchor = canonicalizePlanStep(plan, renderAnchor, "render")
		movePlanStepAfter(plan, renderAnchor, projectAnchor)
		renderStep := planStepByID(plan, renderAnchor)
		if renderStep != nil {
			if renderStep.Arguments == nil {
				renderStep.Arguments = map[string]interface{}{}
			}
			renderStep.Arguments["stage"] = "render"
			renderStep.Arguments["brief"] = plan.Goal
			renderStep.Arguments[projectParam] = stepOutputRef(projectAnchor, projectField)
			renderStep.Arguments["previewApproved"] = true
			if _, exists := renderStep.Arguments["outputName"]; !exists {
				renderStep.Arguments["outputName"] = "final.mp4"
			}
			applyProjectContextToStep(renderStep, projectID)
			if firstManifestOutput(renderManifest, "entry") != "" {
				renderStep.Arguments["entry"] = stepOutputRef(projectAnchor, "entry")
			}
			renderStep.DependsOn = dependencyList(projectAnchor)
			ensureStepExpectedOutputs(renderStep, "VIDEO", "RENDER_REPORT", "final_video")
			renderStep.ProduceArtifact = true
		}
	}

	visualQAAnchor := ""
	if c.manifestFor("video_frame_qa") != nil {
		if step := planStepByID(plan, "visual_qa"); step != nil {
			visualQAAnchor = step.ID
			movePlanStepAfter(plan, visualQAAnchor, renderAnchor)
			if step := planStepByID(plan, visualQAAnchor); step != nil {
				if step.Arguments == nil {
					step.Arguments = map[string]interface{}{}
				}
				step.Tool = "video_frame_qa"
				step.Arguments["stage"] = "visual_qa"
				step.Arguments["input"] = stepOutputRef(renderAnchor, preferredOutputField(renderManifest, "finalVideo", "outputPath", "VIDEO"))
				step.Arguments["shotList"] = stepOutputRef(shotAnchor, shotField)
				step.Arguments["sampleIntervalSec"] = 4
				step.DependsOn = dependencyListUnique(renderAnchor, shotAnchor)
				ensureStepExpectedOutputs(step, "VIDEO_VISUAL_QA_REPORT", "visualQAReport", "passed", "score", "shotReports", "shotSpecLints", "shotSummaries", "repairPlan", "needsRegeneration")
				step.ProduceArtifact = true
				applyProjectContextToStep(step, projectID)
			}
		} else if renderAnchor != "" {
			visualQAAnchor = insertPlanStepAfter(plan, renderAnchor, AgentStep{
				ID:     uniqueStepID(plan, "visual_qa"),
				Intent: "抽帧检查最终视频的文字安全区、遮挡和画面复杂度",
				Tool:   "video_frame_qa",
				Arguments: map[string]interface{}{
					"stage":             "visual_qa",
					"input":             stepOutputRef(renderAnchor, preferredOutputField(renderManifest, "finalVideo", "outputPath", "VIDEO")),
					"shotList":          stepOutputRef(shotAnchor, shotField),
					"sampleIntervalSec": 4,
				},
				DependsOn:       dependencyListUnique(renderAnchor, shotAnchor),
				ExpectedOutput:  []string{"VIDEO_VISUAL_QA_REPORT", "visualQAReport", "passed", "score", "shotReports", "shotSpecLints", "shotSummaries", "repairPlan", "needsRegeneration"},
				ProduceArtifact: true,
			})
			applyProjectContextToStep(planStepByID(plan, visualQAAnchor), projectID)
		}
	}

	publishAnchor := ""
	if step := planStepByID(plan, "publish_copy"); step != nil {
		publishAnchor = step.ID
	} else if step := planStepByTool(plan, "publish_copy_generator"); step != nil {
		publishAnchor = canonicalizePlanStep(plan, step.ID, "publish_copy")
	}
	if publishAnchor == "" {
		publishAnchor = appendPlanStep(plan, AgentStep{
			ID:        uniqueStepID(plan, "publish_copy"),
			Intent:    "基于成片和脚本生成多平台发布文案",
			Tool:      "publish_copy_generator",
			DependsOn: dependencyListUnique(firstNonEmptyStepID(visualQAAnchor, renderAnchor), scriptAnchor, shotAnchor),
			Arguments: map[string]interface{}{
				"stage":    "publish",
				"brief":    plan.Goal,
				"script":   scriptRef,
				"shotList": stepOutputRef(shotAnchor, shotField),
			},
			ExpectedOutput:  []string{"publish_copy"},
			ProduceArtifact: true,
		})
	}
	movePlanStepAfter(plan, publishAnchor, firstNonEmptyStepID(visualQAAnchor, renderAnchor))
	publishStep := planStepByID(plan, publishAnchor)
	if publishStep != nil {
		if publishStep.Arguments == nil {
			publishStep.Arguments = map[string]interface{}{}
		}
		publishStep.Arguments["stage"] = "publish"
		publishStep.Arguments["brief"] = plan.Goal
		publishStep.Arguments["script"] = scriptRef
		publishStep.Arguments["shotList"] = stepOutputRef(shotAnchor, shotField)
		publishStep.DependsOn = dependencyListUnique(firstNonEmptyStepID(visualQAAnchor, renderAnchor), scriptAnchor, shotAnchor)
		if len(publishStep.ExpectedOutput) == 0 {
			publishStep.ExpectedOutput = []string{"publish_copy"}
		}
		publishStep.ProduceArtifact = true
	}
	applyProjectContextToStep(publishStep, projectID)
}

func (c *PlanCompiler) injectMCPGenerationRunner(plan *AgentPlan) {
	if plan == nil || plan.Domain != "video_creation" {
		return
	}
	runnerManifest := c.manifestFor("mcp_generation_runner")
	if runnerManifest == nil {
		return
	}
	providerID, mcpTool, ok := mcpGenerationProvider(requestedAIGCProvider(plan))
	existingMCPStep := planStepByTool(plan, "mcp_generation_runner")
	if !ok && existingMCPStep == nil {
		return
	}
	if providerID == "" {
		providerID = "jimeng"
	}
	if mcpTool == "" {
		mcpTool = providerID + ".generate_video"
	}
	promptAnchor, _ := c.lastVideoPromptProducer(plan)
	if promptAnchor == "" {
		return
	}
	externalField := c.outputFieldForStep(plan, promptAnchor, "externalGenerationRequests")
	if externalField == "" {
		externalField = "externalGenerationRequests"
	}
	projectID := requestedProjectID(plan)
	stepID := ""
	if existingMCPStep != nil {
		stepID = canonicalizePlanStep(plan, existingMCPStep.ID, "mcp_generation")
		movePlanStepAfter(plan, stepID, promptAnchor)
		step := planStepByID(plan, stepID)
		if step != nil {
			if step.Arguments == nil {
				step.Arguments = map[string]interface{}{}
			}
			step.Arguments["stage"] = "aigc_generation"
			step.Arguments["providerId"] = providerID
			step.Arguments["mcpTool"] = mcpTool
			step.Arguments["externalGenerationRequests"] = stepOutputRef(promptAnchor, externalField)
			step.DependsOn = dependencyList(promptAnchor)
			if len(step.ExpectedOutput) == 0 {
				step.ExpectedOutput = []string{"shotAssetPackages", "generationResults", "externalGenerationResults"}
			}
			step.ProduceArtifact = true
			applyProjectContextToStep(step, projectID)
		}
	} else {
		stepID = uniqueStepID(plan, "mcp_generation")
		insertPlanStepAfter(plan, promptAnchor, AgentStep{
			ID:     stepID,
			Intent: "调用用户本地 MCP provider，将外部 AIGC 请求转换为已生成素材包",
			Tool:   "mcp_generation_runner",
			Arguments: map[string]interface{}{
				"stage":                      "aigc_generation",
				"providerId":                 providerID,
				"mcpTool":                    mcpTool,
				"externalGenerationRequests": stepOutputRef(promptAnchor, externalField),
			},
			DependsOn:       dependencyList(promptAnchor),
			ExpectedOutput:  []string{"shotAssetPackages", "generationResults", "externalGenerationResults"},
			ProduceArtifact: true,
		})
		applyProjectContextToStep(planStepByID(plan, stepID), projectID)
	}
	mcpPackageField := preferredOutputField(runnerManifest, "shotAssetPackages")
	projectAnchor, _ := c.lastProducerStepForFields(plan, []string{"projectDir", "hyperframesPath"}, []string{"hyperframes_project_generator"})
	projectStep := planStepByID(plan, projectAnchor)
	if projectStep == nil || !manifestAcceptsParam(c.manifestFor(projectStep.Tool), "shotAssetPackages") {
		return
	}
	if projectStep.Arguments == nil {
		projectStep.Arguments = map[string]interface{}{}
	}
	projectStep.Arguments["shotAssetPackages"] = stepOutputRef(stepID, mcpPackageField)
	appendDependencyIfMissing(projectStep, stepID)
}

func mcpGenerationProvider(provider string) (string, string, bool) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "jimeng_mcp", "jimeng":
		return "jimeng", "jimeng.generate_video", true
	default:
		return "", "", false
	}
}

func (c *PlanCompiler) lastVideoPromptProducer(plan *AgentPlan) (string, string) {
	if plan == nil {
		return "", ""
	}
	for i := len(plan.Steps) - 1; i >= 0; i-- {
		step := plan.Steps[i]
		if step.Tool == "video_prompt_generator" {
			if field := firstManifestOutput(c.manifestFor(step.Tool), "videoPrompts", "video_prompt", "keyframePrompts", "keyframe_prompt"); field != "" {
				return step.ID, field
			}
		}
	}
	return "", ""
}

func (c *PlanCompiler) ensureProfileSelectionStep(plan *AgentPlan, profile string) string {
	if step := planStepByID(plan, "profile_selection"); step != nil {
		if step.Arguments == nil {
			step.Arguments = map[string]interface{}{}
		}
		step.Arguments["stage"] = "profile_selection"
		step.Arguments["brief"] = plan.Goal
		step.Arguments["route"] = profile
		step.DependsOn = nil
		if len(step.ExpectedOutput) == 0 {
			step.ExpectedOutput = []string{"creationProfile", "routingReason"}
		}
		movePlanStepToIndex(plan, step.ID, 0)
		return step.ID
	}
	if existing := planStepByTool(plan, "video_profile_classifier"); existing != nil {
		stepID := canonicalizePlanStep(plan, existing.ID, "profile_selection")
		movePlanStepToIndex(plan, stepID, 0)
		step := planStepByID(plan, stepID)
		if step != nil {
			if step.Arguments == nil {
				step.Arguments = map[string]interface{}{}
			}
			step.Arguments["stage"] = "profile_selection"
			step.Arguments["brief"] = plan.Goal
			step.Arguments["route"] = profile
			step.DependsOn = nil
			if step.Intent == "" {
				step.Intent = "识别视频创作主线并选择编排模板"
			}
			if len(step.ExpectedOutput) == 0 {
				step.ExpectedOutput = []string{"creationProfile", "routingReason"}
			}
			step.ProduceArtifact = true
		}
		return stepID
	}
	step := AgentStep{
		ID:     "profile_selection",
		Intent: "识别视频创作主线并选择编排模板",
		Tool:   "video_profile_classifier",
		Arguments: map[string]interface{}{
			"stage": "profile_selection",
			"brief": plan.Goal,
			"route": profile,
		},
		ExpectedOutput:  []string{"creationProfile", "routingReason"},
		ProduceArtifact: true,
	}
	insertPlanStepAt(plan, 0, step)
	return step.ID
}

func (c *PlanCompiler) ensureProfileStepAfter(plan *AgentPlan, id, afterID string, step AgentStep) string {
	if existing := planStepByID(plan, id); existing != nil {
		movePlanStepAfter(plan, existing.ID, afterID)
		existing = planStepByID(plan, existing.ID)
		if existing != nil {
			existing.DependsOn = append([]string(nil), step.DependsOn...)
		}
		return existing.ID
	}
	if canReuseProfileToolStep(step.Tool) {
		if existing := profileStepByToolForTarget(plan, step); existing != nil {
			stepID := canonicalizePlanStep(plan, existing.ID, id)
			movePlanStepAfter(plan, stepID, afterID)
			existing = planStepByID(plan, stepID)
			if existing != nil {
				if existing.Intent == "" {
					existing.Intent = step.Intent
				}
				if len(existing.ExpectedOutput) == 0 {
					existing.ExpectedOutput = step.ExpectedOutput
				}
				existing.DependsOn = append([]string(nil), step.DependsOn...)
				if step.ProduceArtifact {
					existing.ProduceArtifact = true
				}
			}
			return stepID
		}
	}
	return insertPlanStepAfter(plan, afterID, step)
}

func profileStepByToolForTarget(plan *AgentPlan, target AgentStep) *AgentStep {
	if plan == nil || target.Tool == "" {
		return nil
	}
	targetStage := stringArg(target.Arguments, "stage")
	for i := range plan.Steps {
		step := &plan.Steps[i]
		if step.Tool != target.Tool {
			continue
		}
		if step.ID == target.ID || step.ID == target.Tool {
			return step
		}
		if stage := stringArg(step.Arguments, "stage"); stage != "" && (stage == target.ID || stage == targetStage) {
			return step
		}
	}
	return nil
}

func stringArg(args map[string]interface{}, key string) string {
	if args == nil {
		return ""
	}
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func canReuseProfileToolStep(toolName string) bool {
	switch toolName {
	case "proposal_generator",
		"video_script_generator",
		"continuity_checker",
		"reference_asset_planner",
		"cinematic_shot_designer",
		"time_window_planner",
		"visual_alignment_planner",
		"keyframe_prompt_generator",
		"shot_generation_planner":
		return true
	default:
		return false
	}
}

func ensureStepExpectedOutputs(step *AgentStep, outputs ...string) {
	if step == nil {
		return
	}
	for _, output := range outputs {
		if output == "" || containsString(step.ExpectedOutput, output) {
			continue
		}
		step.ExpectedOutput = append(step.ExpectedOutput, output)
	}
}

func rewireVideoPromptShotSource(step *AgentStep, shotAnchor, shotField string) {
	if step == nil || shotAnchor == "" || shotField == "" {
		return
	}
	if step.Arguments == nil {
		step.Arguments = map[string]interface{}{}
	}
	oldStepID, _, hadOldShotRef := outputReference(step.Arguments["shotList"])
	step.Arguments["shotList"] = stepOutputRef(shotAnchor, shotField)
	if hadOldShotRef && oldStepID != "" && oldStepID != shotAnchor {
		step.DependsOn = removeDependency(step.DependsOn, oldStepID)
	}
	appendDependencyIfMissing(step, shotAnchor)
}

func removeDependency(deps []string, remove string) []string {
	if remove == "" || len(deps) == 0 {
		return deps
	}
	out := deps[:0]
	for _, dep := range deps {
		if dep == remove {
			continue
		}
		out = append(out, dep)
	}
	return out
}

func insertPlanStepAt(plan *AgentPlan, index int, step AgentStep) string {
	if plan == nil {
		return step.ID
	}
	if index < 0 {
		index = 0
	}
	if index > len(plan.Steps) {
		index = len(plan.Steps)
	}
	plan.Steps = append(plan.Steps, AgentStep{})
	copy(plan.Steps[index+1:], plan.Steps[index:])
	plan.Steps[index] = step
	return step.ID
}

func canonicalizePlanStep(plan *AgentPlan, oldID, desiredID string) string {
	if plan == nil || oldID == "" || desiredID == "" || oldID == desiredID {
		return oldID
	}
	if planStepByID(plan, desiredID) != nil {
		return oldID
	}
	renamePlanStepID(plan, oldID, desiredID)
	return desiredID
}

func renamePlanStepID(plan *AgentPlan, oldID, newID string) {
	if plan == nil || oldID == "" || newID == "" || oldID == newID {
		return
	}
	for i := range plan.Steps {
		if plan.Steps[i].ID == oldID {
			plan.Steps[i].ID = newID
		}
		for depIndex, dep := range plan.Steps[i].DependsOn {
			if dep == oldID {
				plan.Steps[i].DependsOn[depIndex] = newID
			}
		}
		for key, value := range plan.Steps[i].Arguments {
			plan.Steps[i].Arguments[key] = rewriteOutputRefStepID(value, oldID, newID)
		}
	}
}

func rewriteOutputRefStepID(value interface{}, oldID, newID string) interface{} {
	switch typed := value.(type) {
	case string:
		prefix := "{{" + oldID + ".output."
		if strings.HasPrefix(typed, prefix) {
			return "{{" + newID + ".output." + strings.TrimPrefix(typed, prefix)
		}
		return typed
	case []interface{}:
		for i := range typed {
			typed[i] = rewriteOutputRefStepID(typed[i], oldID, newID)
		}
		return typed
	case map[string]interface{}:
		for key, item := range typed {
			typed[key] = rewriteOutputRefStepID(item, oldID, newID)
		}
		return typed
	default:
		return value
	}
}

func movePlanStepToIndex(plan *AgentPlan, stepID string, index int) {
	if plan == nil || stepID == "" {
		return
	}
	from := planStepIndex(plan, stepID)
	if from < 0 {
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= len(plan.Steps) {
		index = len(plan.Steps) - 1
	}
	if from == index {
		return
	}
	step := plan.Steps[from]
	plan.Steps = append(plan.Steps[:from], plan.Steps[from+1:]...)
	if from < index {
		index--
	}
	plan.Steps = append(plan.Steps, AgentStep{})
	copy(plan.Steps[index+1:], plan.Steps[index:])
	plan.Steps[index] = step
}

func movePlanStepAfter(plan *AgentPlan, stepID, afterID string) {
	if plan == nil || stepID == "" || afterID == "" || stepID == afterID {
		return
	}
	from := planStepIndex(plan, stepID)
	to := planStepIndex(plan, afterID)
	if from < 0 || to < 0 {
		return
	}
	if from == to+1 {
		return
	}
	step := plan.Steps[from]
	plan.Steps = append(plan.Steps[:from], plan.Steps[from+1:]...)
	if from < to {
		to--
	}
	insertAt := to + 1
	plan.Steps = append(plan.Steps, AgentStep{})
	copy(plan.Steps[insertAt+1:], plan.Steps[insertAt:])
	plan.Steps[insertAt] = step
}

func mergeStepArgsAndDeps(step *AgentStep, args map[string]interface{}, deps ...string) {
	if step == nil {
		return
	}
	if step.Arguments == nil {
		step.Arguments = map[string]interface{}{}
	}
	for key, value := range args {
		step.Arguments[key] = value
	}
	for _, dep := range deps {
		appendDependencyIfMissing(step, dep)
	}
}

func appendPlanStep(plan *AgentPlan, step AgentStep) string {
	plan.Steps = append(plan.Steps, step)
	return step.ID
}

func insertPlanStepAfter(plan *AgentPlan, afterID string, step AgentStep) string {
	if plan == nil {
		return step.ID
	}
	for i := range plan.Steps {
		if plan.Steps[i].ID != afterID {
			continue
		}
		plan.Steps = append(plan.Steps, AgentStep{})
		copy(plan.Steps[i+2:], plan.Steps[i+1:])
		plan.Steps[i+1] = step
		return step.ID
	}
	return appendPlanStep(plan, step)
}

func (c *PlanCompiler) augmentVideoPromptGenerationInputs(plan *AgentPlan, promptAnchor, generationAnchor, generationField, generationPackageField string) {
	if generationAnchor == "" {
		return
	}
	step := planStepByID(plan, promptAnchor)
	if step == nil {
		return
	}
	if step.Arguments == nil {
		step.Arguments = map[string]interface{}{}
	}
	if generationField != "" {
		if _, exists := step.Arguments["shotGenerationPlans"]; !exists {
			step.Arguments["shotGenerationPlans"] = stepOutputRef(generationAnchor, generationField)
		}
	}
	if generationPackageField != "" && manifestAcceptsParam(c.manifestFor(step.Tool), "shotAssetPackages") {
		if _, exists := step.Arguments["shotAssetPackages"]; !exists {
			step.Arguments["shotAssetPackages"] = stepOutputRef(generationAnchor, generationPackageField)
		}
	}
	appendDependencyIfMissing(step, generationAnchor)
}

func (c *PlanCompiler) augmentPreviewGenerationInputs(plan *AgentPlan, projectAnchor, promptAnchor, generationAnchor, generationField, generationPackageField string) {
	if generationAnchor == "" {
		return
	}
	step := planStepByID(plan, projectAnchor)
	if step == nil {
		return
	}
	if step.Arguments == nil {
		step.Arguments = map[string]interface{}{}
	}
	if generationField != "" {
		if _, exists := step.Arguments["shotGenerationPlans"]; !exists {
			step.Arguments["shotGenerationPlans"] = stepOutputRef(generationAnchor, generationField)
		}
	}
	if _, exists := step.Arguments["shotAssetPackages"]; !exists {
		if packageField := c.outputFieldForStep(plan, promptAnchor, "shotAssetPackages"); packageField != "" {
			step.Arguments["shotAssetPackages"] = stepOutputRef(promptAnchor, packageField)
			appendDependencyIfMissing(step, promptAnchor)
		} else if generationPackageField != "" && manifestAcceptsParam(c.manifestFor(step.Tool), "shotAssetPackages") {
			step.Arguments["shotAssetPackages"] = stepOutputRef(generationAnchor, generationPackageField)
		}
	}
	appendDependencyIfMissing(step, generationAnchor)
}

func planStepByID(plan *AgentPlan, id string) *AgentStep {
	if plan == nil || id == "" {
		return nil
	}
	for i := range plan.Steps {
		if plan.Steps[i].ID == id {
			return &plan.Steps[i]
		}
	}
	return nil
}

func planStepByTool(plan *AgentPlan, toolName string) *AgentStep {
	if plan == nil || toolName == "" {
		return nil
	}
	for i := range plan.Steps {
		if plan.Steps[i].Tool == toolName {
			return &plan.Steps[i]
		}
	}
	return nil
}

func planStepIndex(plan *AgentPlan, id string) int {
	if plan == nil || id == "" {
		return -1
	}
	for i := range plan.Steps {
		if plan.Steps[i].ID == id {
			return i
		}
	}
	return -1
}

func firstNonEmptyStepID(stepIDs ...string) string {
	for _, stepID := range stepIDs {
		if stepID != "" {
			return stepID
		}
	}
	return ""
}

func dependencyList(stepID string) []string {
	if stepID == "" {
		return nil
	}
	return []string{stepID}
}

func dependencyListUnique(stepIDs ...string) []string {
	deps := make([]string, 0, len(stepIDs))
	seen := map[string]bool{}
	for _, stepID := range stepIDs {
		if stepID == "" || seen[stepID] {
			continue
		}
		seen[stepID] = true
		deps = append(deps, stepID)
	}
	return deps
}

func uniqueStepID(plan *AgentPlan, base string) string {
	existing := map[string]bool{}
	for _, step := range plan.Steps {
		existing[step.ID] = true
	}
	if !existing[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if !existing[candidate] {
			return candidate
		}
	}
}

func planHasToolOrTerm(plan *AgentPlan, terms []string) bool {
	return lastStepMatching(plan, terms) != ""
}

func lastStepMatching(plan *AgentPlan, terms []string) string {
	if plan == nil {
		return ""
	}
	for i := len(plan.Steps) - 1; i >= 0; i-- {
		if stepMentionsAny(plan.Steps[i], terms) {
			return plan.Steps[i].ID
		}
	}
	return ""
}

func (c *PlanCompiler) lastProducerStepForFields(plan *AgentPlan, fields []string, fallbackTools []string) (string, string) {
	if plan == nil {
		return "", ""
	}
	fallback := stringSet(fallbackTools)
	for i := len(plan.Steps) - 1; i >= 0; i-- {
		step := plan.Steps[i]
		manifest := c.manifestFor(step.Tool)
		if field := firstManifestOutput(manifest, fields...); field != "" {
			return step.ID, field
		}
		if (manifest == nil || len(manifest.Output) == 0) && fallback[strings.ToLower(strings.TrimSpace(step.Tool))] && len(fields) > 0 {
			return step.ID, fields[0]
		}
	}
	return "", ""
}

func firstManifestOutput(manifest *tool.ToolManifest, fields ...string) string {
	if manifest != nil {
		for _, field := range fields {
			if _, ok := manifest.Output[field]; ok {
				return field
			}
		}
	}
	return ""
}

func (c *PlanCompiler) outputFieldForStep(plan *AgentPlan, stepID string, fields ...string) string {
	if plan == nil || stepID == "" {
		return ""
	}
	for _, step := range plan.Steps {
		if step.ID != stepID {
			continue
		}
		return firstManifestOutput(c.manifestFor(step.Tool), fields...)
	}
	return ""
}

func preferredOutputField(manifest *tool.ToolManifest, fields ...string) string {
	if field := firstManifestOutput(manifest, fields...); field != "" {
		return field
	}
	if len(fields) > 0 {
		return fields[0]
	}
	return ""
}

func preferredParamName(manifest *tool.ToolManifest, names ...string) string {
	if manifest != nil {
		for _, name := range names {
			if _, ok := manifest.Parameters[name]; ok {
				return name
			}
		}
	}
	if len(names) > 0 {
		return names[0]
	}
	return ""
}

func manifestAcceptsParam(manifest *tool.ToolManifest, name string) bool {
	if manifest == nil || len(manifest.Parameters) == 0 {
		return true
	}
	_, ok := manifest.Parameters[name]
	return ok
}

func stepMentionsAny(step AgentStep, terms []string) bool {
	haystack := strings.ToLower(step.ID + " " + step.Intent + " " + step.Tool)
	if stage, ok := step.Arguments["stage"].(string); ok {
		haystack += " " + stage
	}
	for _, out := range step.ExpectedOutput {
		haystack += " " + strings.ToLower(out)
	}
	for _, term := range terms {
		if strings.Contains(haystack, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

func requestedAIGCProvider(plan *AgentPlan) string {
	if plan == nil {
		return ""
	}
	for _, step := range plan.Steps {
		if provider, ok := step.Arguments["aigcProvider"].(string); ok && strings.TrimSpace(provider) != "" {
			return strings.ToLower(strings.TrimSpace(provider))
		}
		if provider, ok := step.Arguments["aigc_provider"].(string); ok && strings.TrimSpace(provider) != "" {
			return strings.ToLower(strings.TrimSpace(provider))
		}
	}
	return ""
}

func requestedProjectID(plan *AgentPlan) string {
	if plan == nil {
		return ""
	}
	for _, step := range plan.Steps {
		for _, key := range []string{"projectId", "projectID", "project_id", "videoProjectId", "video_project_id"} {
			raw, ok := step.Arguments[key].(string)
			if !ok {
				continue
			}
			value := strings.TrimSpace(raw)
			if value == "" || strings.Contains(value, "{{") {
				continue
			}
			return value
		}
	}
	return ""
}

func applyProjectContextToStep(step *AgentStep, projectID string) {
	if step == nil || strings.TrimSpace(projectID) == "" {
		return
	}
	if step.Arguments == nil {
		step.Arguments = map[string]interface{}{}
	}
	if existing, ok := step.Arguments["projectId"].(string); ok && strings.TrimSpace(existing) != "" {
		return
	}
	step.Arguments["projectId"] = strings.TrimSpace(projectID)
}

func stepOutputRef(stepID, field string) string {
	if stepID == "" || field == "" {
		return ""
	}
	return fmt.Sprintf("{{%s.output.%s}}", stepID, field)
}

func (c *PlanCompiler) hasVideoBetaCompletionTools() bool {
	for _, name := range []string{"shot_splitter", "video_prompt_generator", "hyperframes_project_generator", "hyperframes_renderer", "publish_copy_generator"} {
		if c.manifestFor(name) == nil {
			return false
		}
	}
	return true
}

func (c *PlanCompiler) hasVideoOutputCompletionTools() bool {
	for _, name := range []string{"video_prompt_generator", "hyperframes_project_generator", "hyperframes_renderer", "publish_copy_generator"} {
		if c.manifestFor(name) == nil {
			return false
		}
	}
	return true
}

func (c *PlanCompiler) hasOptionalTool(name string) bool {
	return c.manifestFor(name) != nil
}

func (c *PlanCompiler) injectKnowledgeContext(plan *AgentPlan) {
	if plan == nil {
		return
	}
	knowledgeSteps := make([]knowledgeProducer, 0)
	for i := range plan.Steps {
		step := &plan.Steps[i]
		manifest := c.manifestFor(step.Tool)
		if isContentGenerationTool(step.Tool, manifest) {
			c.applyKnowledgeContextToGenerationStep(step, knowledgeSteps, plan.KnowledgePolicy)
			continue
		}
		if producer, ok := knowledgeProducerForStep(*step, manifest); ok {
			knowledgeSteps = append(knowledgeSteps, producer)
		}
	}
}

type knowledgeProducer struct {
	StepID       string
	ToolName     string
	ItemRefs     []interface{}
	SourceRefs   []interface{}
	EvidenceRefs []interface{}
}

func knowledgeProducerForStep(step AgentStep, manifest *tool.ToolManifest) (knowledgeProducer, bool) {
	if manifest == nil || len(manifest.Output) == 0 {
		return knowledgeProducer{}, false
	}
	itemRefs := refsForOutputFields(step.ID, manifest, "facts", "evidence", "searchResults", "knowledge", "context", "summary", "results")
	sourceRefs := refsForOutputFields(step.ID, manifest, "sources", "citations", "references")
	evidenceRefs := refsForOutputFields(step.ID, manifest, "evidence", "searchResults", "results")
	if len(itemRefs) == 0 && len(sourceRefs) == 0 && len(evidenceRefs) == 0 {
		return knowledgeProducer{}, false
	}
	return knowledgeProducer{
		StepID:       step.ID,
		ToolName:     step.Tool,
		ItemRefs:     itemRefs,
		SourceRefs:   sourceRefs,
		EvidenceRefs: evidenceRefs,
	}, true
}

func refsForOutputFields(stepID string, manifest *tool.ToolManifest, fields ...string) []interface{} {
	refs := make([]interface{}, 0, len(fields))
	for _, field := range fields {
		if _, ok := manifest.Output[field]; ok {
			refs = append(refs, fmt.Sprintf("{{%s.output.%s}}", stepID, field))
		}
	}
	return refs
}

func (c *PlanCompiler) applyKnowledgeContextToGenerationStep(step *AgentStep, producers []knowledgeProducer, policy *KnowledgePolicy) {
	if step == nil {
		return
	}
	if step.Arguments == nil {
		step.Arguments = map[string]interface{}{}
	}
	if len(producers) > 0 {
		for _, producer := range producers {
			appendDependencyIfMissing(step, producer.StepID)
		}
		if _, exists := step.Arguments["knowledgeContext"]; !exists {
			items := make([]interface{}, 0)
			sources := make([]interface{}, 0)
			evidence := make([]interface{}, 0)
			generatedBy := make([]interface{}, 0, len(producers))
			for _, producer := range producers {
				items = append(items, producer.ItemRefs...)
				sources = append(sources, producer.SourceRefs...)
				evidence = append(evidence, producer.EvidenceRefs...)
				generatedBy = append(generatedBy, producer.ToolName)
			}
			step.Arguments["knowledgeContext"] = map[string]interface{}{
				"items":       items,
				"sources":     sources,
				"evidence":    evidence,
				"generatedBy": generatedBy,
			}
		}
	}
	if policy != nil {
		step.Arguments["retrievalPolicy"] = string(policy.RetrievalPolicy)
		step.Arguments["requireFreshFacts"] = policy.MustUseFacts || policy.RetrievalPolicy == RetrievalRequired
		if _, ok := step.Arguments["currentDate"]; !ok {
			step.Arguments["currentDate"] = time.Now().Format("2006-01-02")
		}
	}
}

func isContentGenerationTool(toolName string, manifest *tool.ToolManifest) bool {
	joined := strings.ToLower(toolName)
	if manifest != nil {
		joined = strings.ToLower(strings.Join(append([]string{toolName, manifest.Description}, manifest.Capabilities...), " "))
	}
	for _, marker := range []string{
		"script_generation",
		"content_generation",
		"proposal_generation",
		"video_script_generator",
		"script_generator",
		"proposal_generator",
		"image_text_video_generator",
	} {
		if strings.Contains(joined, marker) {
			return true
		}
	}
	return false
}

// injectQualityGates scans the plan steps and auto-inserts quality checker steps
// after production tools that don't already have an explicit quality check in the plan.
func (c *PlanCompiler) injectQualityGates(steps []AgentStep) []AgentStep {
	// Build tool presence set to avoid duplicates.
	toolSet := make(map[string]bool, len(steps))
	for _, s := range steps {
		toolSet[s.Tool] = true
	}

	var out []AgentStep
	out = make([]AgentStep, 0, len(steps)*3)

	for _, step := range steps {
		out = append(out, step)

		manifest := c.manifestFor(step.Tool)
		checkerName, hasChecker := qualityCheckerFor(step.Tool, manifest)
		if !hasChecker || toolSet[checkerName] {
			continue
		}

		// Check if the production tool's manifest has qualityPolicy.Required.
		if manifest == nil || !manifest.QualityPolicy.Required {
			// Quality checker is recommended but not required by manifest; skip auto-insert.
			continue
		}

		// Auto-insert a quality check step.
		checkerStep := AgentStep{
			ID:              checkerName,
			Intent:          fmt.Sprintf("自动质量检查：%s 的输出", step.Tool),
			Tool:            checkerName,
			DependsOn:       []string{step.ID},
			Arguments:       buildQualityCheckArgs(step),
			ExpectedOutput:  []string{"passed", "score", "issues", "repairSuggestions"},
			ProduceArtifact: true,
		}
		out = append(out, checkerStep)
		toolSet[checkerName] = true

		// Auto-insert a quality gate step that blocks downstream when quality fails.
		// Uses __quality_gate__ marker tool compiled as a CONTROL node below.
		minScore := manifest.QualityPolicy.MinScore
		if minScore <= 0 {
			minScore = 85
		}
		gateID := step.ID + "_quality_gate"
		gateStep := AgentStep{
			ID:        gateID,
			Intent:    fmt.Sprintf("质量门禁：%s 评分需 >=%d", checkerName, minScore),
			Tool:      "__quality_gate__",
			DependsOn: []string{checkerName},
			Arguments: map[string]interface{}{
				"checkerStep":           checkerName,
				"qualityCheckerNode":    compiledToolOutputNodeID(checkerName, c.manifestFor(checkerName)),
				"productionStep":        step.ID,
				"productionTool":        step.Tool,
				"productionSourceNode":  compiledToolSourceNodeID(step.ID, manifest),
				"minScore":              minScore,
				"autoApproveWhenPassed": true,
				"autoRepair":            manifest.QualityPolicy.AutoRepair,
				"maxRepairAttempts":     manifest.QualityPolicy.MaxRepairAttempts,
			},
			ExpectedOutput:  []string{"gateResult"},
			ProduceArtifact: false,
		}
		out = append(out, gateStep)
		toolSet[gateID] = true
	}

	// Rewire downstream dependencies through quality gates.
	// Any step that depends on a production step with a quality gate
	// must wait for the gate instead of the production step directly.
	for i := range out {
		s := &out[i]
		for j, dep := range s.DependsOn {
			for _, prev := range out {
				if prev.Tool == "__quality_gate__" {
					prod, _ := prev.Arguments["productionStep"].(string)
					checker, _ := prev.Arguments["checkerStep"].(string)
					if prod != "" && dep == prod && s.ID != checker && s.Tool != checker {
						s.DependsOn[j] = prev.ID
					}
				}
			}
		}
	}

	return out
}

func compiledToolSourceNodeID(stepID string, manifest *tool.ToolManifest) string {
	policy := normalizedApprovalPolicy(manifest)
	switch policy.Mode {
	case tool.ApprovalBeforeExecute, tool.ApprovalBeforeSideEffect, tool.ApprovalAfterArtifact, tool.ApprovalBeforeDownstream, tool.ApprovalAlways:
		return stepID + "_exec"
	default:
		return stepID
	}
}

func compiledToolOutputNodeID(stepID string, manifest *tool.ToolManifest) string {
	policy := normalizedApprovalPolicy(manifest)
	switch policy.Mode {
	case tool.ApprovalBeforeExecute, tool.ApprovalBeforeSideEffect, tool.ApprovalNone, "":
		return compiledToolSourceNodeID(stepID, manifest)
	default:
		return stepID + "_review"
	}
}

func qualityCheckerFor(toolName string, manifest *tool.ToolManifest) (string, bool) {
	if manifest != nil && manifest.QualityPolicy.Required && manifest.QualityPolicy.CheckerTool != "" {
		return manifest.QualityPolicy.CheckerTool, true
	}
	name := QualityCheckerFor(toolName)
	return name, name != ""
}

// buildQualityCheckArgs constructs arguments for an auto-inserted quality checker step.
// It forwards the primary artifact reference AND the source step's context arguments
// (durationSec, knowledgeContext, knowledgePack, topic, etc.) so the quality checker
// can evaluate against the user's actual requirements and current facts.
func buildQualityCheckArgs(sourceStep AgentStep) map[string]interface{} {
	args := map[string]interface{}{}
	switch sourceStep.Tool {
	case "video_script_generator":
		args["script"] = fmt.Sprintf("{{%s.output.script}}", sourceStep.ID)
	case "shot_splitter":
		args["shotList"] = fmt.Sprintf("{{%s.output.shotList}}", sourceStep.ID)
	case "video_prompt_generator":
		args["videoPrompts"] = fmt.Sprintf("{{%s.output.videoPrompts}}", sourceStep.ID)
	case "video_package_exporter":
		args["script"] = fmt.Sprintf("{{%s.output.package}}", sourceStep.ID)
	}

	// Forward context arguments from the source step so the quality checker
	// has access to the user's target duration, current facts, and topic.
	for _, key := range []string{
		"durationSec", "targetDurationSec",
		"knowledgeContext", "knowledgePack", "knowledgeSources",
		"facts", "searchResults",
		"topic", "brief",
		"currentDate", "retrievalPolicy", "mustUseFreshKnowledge", "requireFreshFacts",
	} {
		if v, ok := sourceStep.Arguments[key]; ok && v != nil {
			args[key] = v
		}
	}

	return args
}

func (c *PlanCompiler) applyDirectors(steps []AgentStep, domain string) ([]AgentStep, error) {
	if c.directors == nil {
		return steps, nil
	}
	if domain != "video_creation" {
		return steps, nil
	}

	callCount := map[string]int{}
	annotated := make([]AgentStep, 0, len(steps))
	for _, s := range steps {
		// Skip internal marker tools.
		if s.Tool == "__quality_gate__" {
			annotated = append(annotated, s)
			continue
		}

		stage := resolveStepStage(s)
		if stage == "" {
			annotated = append(annotated, s)
			continue
		}
		director := c.directors.Get(stage)
		if director == nil {
			annotated = append(annotated, s)
			continue
		}

		forbidden := stringSet(director.ForbiddenTools())
		allowed := stringSet(director.AllowedTools())
		if forbidden[s.Tool] {
			return nil, fmt.Errorf("stage guard: role %s stage %s forbidden tool %s", roleLabel(director), stage, s.Tool)
		}
		if len(allowed) > 0 && !allowed[s.Tool] && !isQualityCheckerTool(s.Tool) {
			return nil, fmt.Errorf("stage guard: role %s stage %s does not allow tool %s", roleLabel(director), stage, s.Tool)
		}
		callCount[stage]++
		if maxCalls := director.MaxToolCalls(); maxCalls > 0 && callCount[stage] > maxCalls {
			return nil, fmt.Errorf("stage guard: role %s stage %s exceeds max tool calls %d", roleLabel(director), stage, maxCalls)
		}

		s.Arguments = annotateRoleAgentArgs(s.Arguments, stage, director)
		annotated = append(annotated, s)
	}
	return annotated, nil
}

func annotateRoleAgentArgs(args map[string]interface{}, stage string, director StageDirector) map[string]interface{} {
	out := copyMap(args)
	out["stage"] = stage
	if role, ok := director.(RoleAgentDirector); ok {
		if role.RoleID() != "" {
			out["roleAgentId"] = role.RoleID()
		}
		out["roleAgent"] = roleAgentMap(role)
		if len(role.RequiredInputs()) > 0 {
			out["requiredInputs"] = role.RequiredInputs()
		}
		if len(role.RequiredOutputs()) > 0 {
			out["requiredOutputs"] = role.RequiredOutputs()
		}
		if role.HumanReview() != nil {
			out["humanReview"] = humanReviewMap(role.HumanReview())
		}
	}
	return out
}

func roleAgentMap(role RoleAgentDirector) map[string]interface{} {
	return map[string]interface{}{
		"id":              role.RoleID(),
		"name":            role.Name(),
		"displayName":     role.DisplayName(),
		"stage":           role.StageName(),
		"goal":            role.Goal(),
		"requiredInputs":  role.RequiredInputs(),
		"requiredOutputs": role.RequiredOutputs(),
		"allowedTools":    role.AllowedTools(),
		"forbiddenTools":  role.ForbiddenTools(),
	}
}

func humanReviewMap(review *tool.HumanReview) map[string]interface{} {
	if review == nil {
		return nil
	}
	return map[string]interface{}{
		"required":    review.Required,
		"gate":        review.Gate,
		"title":       review.Title,
		"reviewFocus": review.ReviewFocus,
		"userActions": review.UserActions,
	}
}

func isQualityCheckerTool(toolName string) bool {
	return strings.HasSuffix(toolName, "_quality_checker") ||
		strings.Contains(toolName, "quality_check")
}

func stringSet(items []string) map[string]bool {
	if len(items) == 0 {
		return nil
	}
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

func (c *PlanCompiler) manifestFor(name string) *tool.ToolManifest {
	if c == nil || c.tools == nil {
		return nil
	}
	return c.tools.GetManifest(name)
}

type compiledStep struct {
	nodes         []model.NodeRequest
	internalEdges []model.Edge
	entryIDs      []string
	outputIDs     []string
}

func compileStep(step AgentStep, manifest *tool.ToolManifest) (compiledStep, error) {
	// Handle quality gate marker tool — compiled as a special CONTROL node
	// that auto-approves when the quality checker passes, blocks when it fails.
	if step.Tool == "__quality_gate__" {
		return compileQualityGate(step), nil
	}

	policy := normalizedApprovalPolicy(manifest)
	switch policy.Mode {
	case tool.ApprovalBeforeExecute, tool.ApprovalBeforeSideEffect:
		beforeID := step.ID + "_review_before"
		execID := step.ID + "_exec"
		return compiledStep{
			nodes: []model.NodeRequest{
				buildReviewNode(beforeID, step, manifest, policy, "before_execute"),
				buildToolNode(execID, step, manifest),
			},
			internalEdges: []model.Edge{{From: beforeID, To: execID}},
			entryIDs:      []string{beforeID},
			outputIDs:     []string{execID},
		}, nil
	case tool.ApprovalAfterArtifact, tool.ApprovalBeforeDownstream:
		execID := step.ID + "_exec"
		reviewID := step.ID + "_review"
		return compiledStep{
			nodes: []model.NodeRequest{
				buildToolNode(execID, step, manifest),
				buildReviewNode(reviewID, step, manifest, policy, "after_artifact"),
			},
			internalEdges: []model.Edge{{From: execID, To: reviewID}},
			entryIDs:      []string{execID},
			outputIDs:     []string{reviewID},
		}, nil
	case tool.ApprovalAlways:
		beforeID := step.ID + "_review_before"
		execID := step.ID + "_exec"
		afterID := step.ID + "_review"
		return compiledStep{
			nodes: []model.NodeRequest{
				buildReviewNode(beforeID, step, manifest, policy, "before_execute"),
				buildToolNode(execID, step, manifest),
				buildReviewNode(afterID, step, manifest, policy, "after_artifact"),
			},
			internalEdges: []model.Edge{
				{From: beforeID, To: execID},
				{From: execID, To: afterID},
			},
			entryIDs:  []string{beforeID},
			outputIDs: []string{afterID},
		}, nil
	case "", tool.ApprovalNone:
		nodeID := step.ID
		return compiledStep{
			nodes:     []model.NodeRequest{buildToolNode(nodeID, step, manifest)},
			entryIDs:  []string{nodeID},
			outputIDs: []string{nodeID},
		}, nil
	default:
		return compiledStep{}, fmt.Errorf("unsupported approval policy mode %q for step %s", policy.Mode, step.ID)
	}
}

func normalizedApprovalPolicy(manifest *tool.ToolManifest) tool.ApprovalPolicy {
	if manifest == nil || !manifest.ApprovalPolicy.Required {
		return tool.ApprovalPolicy{Mode: tool.ApprovalNone}
	}
	policy := manifest.ApprovalPolicy
	if policy.Mode == "" {
		policy.Mode = tool.ApprovalAfterArtifact
	}
	return policy
}

func buildToolNode(nodeID string, step AgentStep, manifest *tool.ToolManifest) model.NodeRequest {
	args := copyMap(step.Arguments)
	params := copyMap(args)
	params["tool"] = step.Tool
	if step.Intent != "" {
		params["intent"] = step.Intent
	}
	if len(step.ExpectedOutput) > 0 {
		params["expectedOutput"] = step.ExpectedOutput
	}
	if step.ProduceArtifact {
		params["produceArtifact"] = true
	}

	nodeName := step.Tool
	inputTool := step.Tool
	if requiresExternalBridge(manifest) {
		nodeName = "external"
		inputTool = "external"
	}
	input := map[string]interface{}{"tool": inputTool, "parameters": params}
	if manifest != nil {
		input["capabilityTool"] = manifest.Name
		input["skillPackageId"] = manifest.SkillPackageID
		input["promptRef"] = manifest.PromptRef
		input["resourceRefs"] = manifest.ResourceRefs
	}

	return model.NodeRequest{
		ID:    nodeID,
		Type:  string(model.NodeTypeTool),
		Name:  nodeName,
		Input: input,
	}
}

func requiresExternalBridge(manifest *tool.ToolManifest) bool {
	if manifest == nil {
		return false
	}
	if manifest.Endpoint != "" || manifest.SkillPackageID != "" {
		return true
	}
	toolType := strings.ToLower(manifest.Type)
	return toolType == "external" || strings.Contains(toolType, "prompt_tool") || toolType == "http" || toolType == "grpc"
}

func buildReviewNode(nodeID string, step AgentStep, manifest *tool.ToolManifest, policy tool.ApprovalPolicy, phase string) model.NodeRequest {
	execID := step.ID + "_exec"
	input := map[string]interface{}{
		"stepId":              step.ID,
		"tool":                step.Tool,
		"reviewPhase":         phase,
		"reviewReason":        policy.Reason,
		"blocksDownstream":    policy.BlocksDownstream,
		"reviewArtifactKinds": policy.ReviewArtifactKinds,
	}
	// Bind the source execution node so the handler knows which artifact to review.
	if phase == "after_artifact" || phase == "before_downstream" {
		input["sourceNode"] = execID
	}
	if len(policy.ReviewArtifactKinds) > 0 {
		input["artifactKinds"] = policy.ReviewArtifactKinds
	}
	input["requiresApprovedArtifacts"] = policy.BlocksDownstream
	if step.Arguments != nil {
		copyInputField(input, step.Arguments, "stage")
		copyInputField(input, step.Arguments, "roleAgentId")
		copyInputField(input, step.Arguments, "roleAgent")
		copyInputField(input, step.Arguments, "requiredInputs")
		copyInputField(input, step.Arguments, "requiredOutputs")
		copyInputField(input, step.Arguments, "artifactIndex")
		copyInputField(input, step.Arguments, "roleMemories")
	}
	if manifest != nil && manifest.HumanReview != nil {
		input["humanReview"] = humanReviewMap(manifest.HumanReview)
	} else if step.Arguments != nil {
		copyInputField(input, step.Arguments, "humanReview")
	}

	return model.NodeRequest{
		ID:    nodeID,
		Type:  string(model.NodeTypeReviewGate),
		Name:  "审核-" + step.ID,
		Input: input,
	}
}

func copyInputField(dst, src map[string]interface{}, key string) {
	if src == nil {
		return
	}
	if value, ok := src[key]; ok {
		dst[key] = value
	}
}

func copyMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// compileQualityGate creates a CONTROL node for a quality gate step.
// When autoApproveWhenPassed is true, the quality gate CONTROL node inspects
// the quality checker's output: if passed=true and score >= minScore, the node
// can be auto-approved; otherwise it blocks downstream and pauses for user review.
func compileQualityGate(step AgentStep) compiledStep {
	nodeID := step.ID
	input := map[string]interface{}{
		"stepId":                    step.ID,
		"tool":                      step.Tool,
		"reviewPhase":               "quality_gate",
		"reviewReason":              step.Intent,
		"blocksDownstream":          true,
		"requiresApprovedArtifacts": true,
	}

	// Copy quality gate metadata from step arguments.
	if step.Arguments != nil {
		if v, ok := step.Arguments["checkerStep"]; ok {
			input["checkerStep"] = v
		}
		if v, ok := step.Arguments["productionStep"]; ok {
			input["productionStep"] = v
		}
		if v, ok := step.Arguments["productionTool"]; ok {
			input["productionTool"] = v
			input["reviewTool"] = v
		}
		if v, ok := step.Arguments["productionSourceNode"]; ok {
			input["sourceNode"] = v
		}
		if v, ok := step.Arguments["qualityCheckerNode"]; ok {
			input["qualityCheckerNode"] = v
		}
		if v, ok := step.Arguments["minScore"]; ok {
			input["minScore"] = v
		}
		if v, ok := step.Arguments["autoApproveWhenPassed"]; ok {
			input["autoApproveWhenPassed"] = v
		}
		if v, ok := step.Arguments["autoRepair"]; ok {
			input["autoRepair"] = v
		}
		if v, ok := step.Arguments["maxRepairAttempts"]; ok {
			input["maxRepairAttempts"] = v
		}
	}

	return compiledStep{
		nodes: []model.NodeRequest{{
			ID:    nodeID,
			Type:  string(model.NodeTypeReviewGate),
			Name:  "质量门禁-" + step.ID,
			Input: input,
		}},
		entryIDs:  []string{nodeID},
		outputIDs: []string{nodeID},
	}
}
