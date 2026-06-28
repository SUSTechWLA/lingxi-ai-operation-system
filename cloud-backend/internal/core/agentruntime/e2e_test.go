package agentruntime

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/model"
	videodirector "github.com/tangying-ai/aios-core/internal/core/video/director"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

// TestE2E_DragonBoatFestival_FullPipeline tests the full Planner→Guard→Compiler→DAG
// pipeline for the input "把端午节和粽子的来源做成 60 秒口播知识视频".
func TestE2E_DragonBoatFestival_FullPipeline(t *testing.T) {
	catalog := dragonBoatToolCatalog()

	plan := dragonBoatPlan()

	// Stage 1: PlanGuard validation.
	guard := NewPlanGuard(catalog, nil)
	if err := guard.Validate(plan); err != nil {
		t.Fatalf("PlanGuard rejected the plan: %v", err)
	}
	t.Log("✅ PlanGuard passed")

	// Stage 1b: Check for quality checker warnings.
	warnings, err := guard.ValidateWithWarnings(plan)
	if err != nil {
		t.Fatalf("ValidateWithWarnings failed: %v", err)
	}
	if len(warnings) > 0 {
		t.Logf("⚠ Quality gate warnings: %v", warnings)
	}

	// Stage 2: PlanCompiler → DAG.
	compiler := NewPlanCompiler(catalog)
	dag, err := compiler.Compile(plan)
	if err != nil {
		t.Fatalf("PlanCompiler failed: %v", err)
	}
	t.Logf("✅ Compiler produced DAG with %d nodes, %d edges", len(dag.Nodes), len(dag.Edges))

	// Stage 3: Verify DAG structure.
	nodeMap := make(map[string]model.NodeRequest, len(dag.Nodes))
	for _, n := range dag.Nodes {
		nodeMap[n.ID] = n
	}

	// 3a: All production tools should have TOOL nodes.
	expectedTools := []string{
		"knowledge_research", "fact_check", "script_generation",
		"shot_split", "video_prompt_generation",
		"publish_copy", "package_export",
	}
	for _, stepID := range expectedTools {
		execID := stepID + "_exec"
		if _, ok := nodeMap[execID]; !ok {
			// Check without _exec suffix (for tools with ApprovalNone).
			if _, ok := nodeMap[stepID]; !ok {
				t.Errorf("❌ missing exec node for step %s", stepID)
			}
		} else {
			t.Logf("  ✓ TOOL node: %s", execID)
		}
	}

	// 3b: Steps with ApprovalPolicy.Required=true should have CONTROL review nodes.
	reviewSteps := map[string]bool{
		"knowledge_research_review":      true,
		"script_generation_review":       true,
		"shot_split_review":              true,
		"video_prompt_generation_review": true,
	}
	foundReviews := 0
	for _, n := range dag.Nodes {
		if n.Type == string(model.NodeTypeControl) || n.Type == string(model.NodeTypeReviewGate) {
			t.Logf("  ✓ CONTROL node: %s (phase: %v)", n.ID, n.Input["reviewPhase"])
			if reviewSteps[n.ID] {
				foundReviews++
			}
		}
	}
	if foundReviews < len(reviewSteps) {
		t.Errorf("❌ expected at least %d review nodes, found %d", len(reviewSteps), foundReviews)
	}

	// 3c: Quality gates should be present for qualityPolicy.Required tools.
	qualityGates := 0
	for _, n := range dag.Nodes {
		if n.Type == string(model.NodeTypeControl) || n.Type == string(model.NodeTypeReviewGate) && n.Input["reviewPhase"] == "quality_gate" {
			qualityGates++
			t.Logf("  ✓ QUALITY_GATE: %s (autoApproveWhenPassed=%v, minScore=%v)",
				n.ID, n.Input["autoApproveWhenPassed"], n.Input["minScore"])
		}
	}
	if qualityGates == 0 {
		t.Error("❌ no quality gate CONTROL nodes found")
	}

	// 3d: Verify edge connectivity — every exec node should have edges.
	if len(dag.Edges) == 0 {
		t.Error("❌ DAG has no edges")
	} else {
		t.Logf("✅ DAG has %d edges (dependency chain intact)", len(dag.Edges))
	}

	// Stage 4: Verify reference scoping works.
	taskID := "task-dragon-boat-001"
	scoped := scopeDAGToTask(taskID, dag)
	if len(scoped.Nodes) != len(dag.Nodes) {
		t.Errorf("scoped DAG node count mismatch: %d vs %d", len(scoped.Nodes), len(dag.Nodes))
	}
	// All scoped node IDs should be <= 64 chars (DB limit).
	for _, n := range scoped.Nodes {
		if len(n.ID) > 64 {
			t.Errorf("scoped node ID too long (%d chars): %s", len(n.ID), n.ID)
		}
	}
	t.Log("✅ Reference scoping passed, all node IDs within DB limits")
}

// TestE2E_PlanGuardOutputFieldValidation tests that referencing non-existent
// output fields is caught.
func TestE2E_PlanGuardOutputFieldValidation(t *testing.T) {
	catalog := dragonBoatToolCatalog()

	plan := dragonBoatPlan()
	// Corrupt a reference: fact_checker output doesn't have "checkedFacts_broken".
	// script_generation (step[2]) depends on fact_check and references its checkedFacts.
	plan.Steps[2].Arguments["facts"] = "{{fact_check.output.checkedFacts_broken}}"

	guard := NewPlanGuard(catalog, nil)
	err := guard.Validate(plan)
	if err == nil {
		t.Fatal("❌ PlanGuard should have rejected invalid output field reference")
	}
	if !strings.Contains(err.Error(), "does not declare this output field") {
		t.Errorf("unexpected error: %v", err)
	}
	t.Logf("✅ PlanGuard correctly rejected invalid field reference: %v", err)
}

// TestE2E_PlanGuardUnknownTool tests that unknown tools are caught.
func TestE2E_PlanGuardUnknownTool(t *testing.T) {
	catalog := dragonBoatToolCatalog()

	plan := dragonBoatPlan()
	plan.Steps = append(plan.Steps, AgentStep{
		ID:   "evil",
		Tool: "delete_all_files",
	})

	guard := NewPlanGuard(catalog, nil)
	err := guard.Validate(plan)
	if err == nil {
		t.Fatal("❌ PlanGuard should have rejected unknown tool")
	}
	if !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("unexpected error: %v", err)
	}
	t.Logf("✅ PlanGuard correctly rejected unknown tool: %v", err)
}

// TestE2E_PlanCompilerQualityGateAutoInsert tests that quality gates are
// auto-inserted for tools with QualityPolicy.Required.
func TestE2E_PlanCompilerQualityGateAutoInsert(t *testing.T) {
	catalog := dragonBoatToolCatalog()

	// Simple plan: just script generation (which has qualityPolicy.Required).
	plan := &AgentPlan{
		Goal:   "test quality gate",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{ID: "script_gen", Tool: "video_script_generator", Arguments: map[string]interface{}{
				"topic": "test",
			}, ProduceArtifact: true},
		},
	}

	guard := NewPlanGuard(catalog, nil)
	if err := guard.Validate(plan); err != nil {
		t.Fatalf("PlanGuard: %v", err)
	}

	compiler := NewPlanCompiler(catalog)
	dag, err := compiler.Compile(plan)
	if err != nil {
		t.Fatalf("PlanCompiler: %v", err)
	}

	// The DAG should contain: script_gen_exec → script_quality_checker_exec → script_gen_quality_gate → script_gen_review
	foundChecker := false
	foundGate := false
	foundReview := false
	for _, n := range dag.Nodes {
		switch {
		case n.ID == "script_quality_checker":
			foundChecker = true
		case n.ID == "script_gen_quality_gate" && (n.Type == string(model.NodeTypeControl) || n.Type == string(model.NodeTypeReviewGate)):
			foundGate = true
			if n.Input["reviewPhase"] != "quality_gate" {
				t.Errorf("quality gate has wrong reviewPhase: %v", n.Input["reviewPhase"])
			}
			if n.Input["autoApproveWhenPassed"] != true {
				t.Errorf("quality gate should have autoApproveWhenPassed=true")
			}
		case n.ID == "script_gen_review" && (n.Type == string(model.NodeTypeControl) || n.Type == string(model.NodeTypeReviewGate)):
			foundReview = true
		}
	}
	if !foundChecker {
		t.Error("❌ missing quality checker node")
	}
	if !foundGate {
		t.Error("❌ missing quality gate CONTROL node")
	}
	if !foundReview {
		t.Error("❌ missing review CONTROL node")
	}
	t.Log("✅ Quality gate auto-insertion verified: checker → quality_gate → review")
}

// TestE2E_PlanStructureValidation verifies the plan has all required steps
// in the correct dependency chain for a knowledge video.
func TestE2E_PlanStructureValidation(t *testing.T) {
	plan := dragonBoatPlan()
	planJSON, _ := json.MarshalIndent(plan, "", "  ")
	t.Logf("AgentPlan:\n%s", planJSON)

	// Verify required steps.
	requiredSteps := []string{
		"knowledge_research",
		"fact_check",
		"script_generation",
		"shot_split",
		"video_prompt_generation",
		"publish_copy",
		"package_export",
	}
	stepMap := make(map[string]AgentStep, len(plan.Steps))
	for _, s := range plan.Steps {
		stepMap[s.ID] = s
	}
	for _, id := range requiredSteps {
		if _, ok := stepMap[id]; !ok {
			t.Errorf("❌ missing required step: %s", id)
		} else {
			t.Logf("  ✓ step: %s → tool: %s", id, stepMap[id].Tool)
		}
	}

	// Verify dependency chain is acyclic and ordered.
	seenOrder := make(map[string]int)
	for i, s := range plan.Steps {
		seenOrder[s.ID] = i
	}
	for _, s := range plan.Steps {
		for _, dep := range s.DependsOn {
			if depPos, ok := seenOrder[dep]; ok && seenOrder[s.ID] <= depPos {
				t.Errorf("❌ step %s depends on %s but appears before or at same position", s.ID, dep)
			}
		}
	}
	t.Log("✅ Dependency chain is valid (acyclic, ordered)")

	// Verify budget is reasonable.
	if plan.Budget.MaxSteps < len(plan.Steps) {
		t.Errorf("plan has %d steps but budget maxSteps is %d", len(plan.Steps), plan.Budget.MaxSteps)
	}
	t.Logf("✅ Budget: maxSteps=%d, maxToolCalls=%d", plan.Budget.MaxSteps, plan.Budget.MaxToolCalls)
}

func TestE2E_TangyingDirector_FirstUserBeta(t *testing.T) {
	catalog := tangyingDirectorBetaCatalog()
	registry := videodirector.DefaultRegistry()
	directors := videoDirectorAdapter{registry: registry}
	plan := tangyingDirectorBetaPlan()

	guard := NewPlanGuard(catalog, nil).WithDirectors(directors)
	if err := guard.Validate(plan); err != nil {
		t.Fatalf("PlanGuard rejected beta plan: %v", err)
	}

	compiler := NewPlanCompiler(catalog).WithDirectors(directors)
	dag, err := compiler.Compile(plan)
	if err != nil {
		t.Fatalf("PlanCompiler rejected beta plan: %v", err)
	}

	requiredStages := []string{"proposal", "script", "storyboard", "composition", "reference", "continuity", "preview", "render", "quality", "package"}
	requiredRoles := []string{"CreativeDirectorAgent", "ScriptWriterAgent", "StoryboardArtistAgent", "CompositionDirectorAgent", "ReferenceSelectorAgent", "ContinuityKeeperAgent", "PreviewDirectorAgent", "RenderProducerAgent", "QualityReviewerAgent", "PackageProducerAgent"}
	requiredArtifacts := []string{"VIDEO_PROPOSAL", "VIDEO_SCRIPT", "CARD_PLAN", "VIDEO_COMPOSITION_SPEC", "REFERENCE_ASSET_PLAN", "STYLE_PROFILE", "CONTINUITY_REPORT", "HYPERFRAMES_PROJECT", "PREVIEW_SNAPSHOTS", "VIDEO", "FFMPEG_PROBE_REPORT", "FINAL_REVIEW", "PROJECT_PACKAGE"}

	stepStages := map[string]bool{}
	for _, step := range plan.Steps {
		if stage, _ := step.Arguments["stage"].(string); stage != "" {
			stepStages[stage] = true
		}
	}
	for _, stage := range requiredStages {
		if !stepStages[stage] {
			t.Fatalf("beta plan missing stage %s", stage)
		}
	}

	roles := map[string]bool{}
	for _, role := range registry.List() {
		roles[role.Name] = true
	}
	for _, role := range requiredRoles {
		if !roles[role] {
			t.Fatalf("beta registry missing role %s", role)
		}
	}

	artifactKinds := map[string]bool{}
	for _, manifest := range catalog {
		for _, kind := range manifest.ArtifactPolicy.ArtifactKinds {
			artifactKinds[kind] = true
		}
	}
	for _, kind := range requiredArtifacts {
		if !artifactKinds[kind] {
			t.Fatalf("beta tool catalog missing artifact kind %s", kind)
		}
	}

	nodeByID := make(map[string]model.NodeRequest, len(dag.Nodes))
	for _, node := range dag.Nodes {
		nodeByID[node.ID] = node
	}
	requireDependencyPath(t, dag.Edges, "preview_review", "render_review_before")
	requireDependencyPath(t, dag.Edges, "quality", "package")
	if nodeByID["render_review_before"].Input["reviewPhase"] != "before_execute" {
		t.Fatalf("render review must be a before-execute gate: %#v", nodeByID["render_review_before"].Input)
	}
}

type videoDirectorAdapter struct {
	registry *videodirector.Registry
}

func (a videoDirectorAdapter) Get(stageName string) StageDirector {
	if a.registry == nil {
		return nil
	}
	d := a.registry.Get(stageName)
	if d == nil {
		return nil
	}
	return videoStageDirector{Director: d}
}

type videoStageDirector struct {
	videodirector.Director
}

func (d videoStageDirector) RoleID() string {
	if role, ok := d.Director.(interface{ RoleID() string }); ok {
		return role.RoleID()
	}
	return ""
}

func (d videoStageDirector) DisplayName() string {
	if role, ok := d.Director.(interface{ DisplayName() string }); ok {
		return role.DisplayName()
	}
	return ""
}

func (d videoStageDirector) Goal() string {
	if role, ok := d.Director.(interface{ Goal() string }); ok {
		return role.Goal()
	}
	return ""
}

func (d videoStageDirector) RequiredInputs() []string {
	if role, ok := d.Director.(interface{ RequiredInputs() []string }); ok {
		return role.RequiredInputs()
	}
	return nil
}

func (d videoStageDirector) RequiredOutputs() []string {
	if role, ok := d.Director.(interface{ RequiredOutputs() []string }); ok {
		return role.RequiredOutputs()
	}
	return nil
}

func (d videoStageDirector) HumanReview() *tool.HumanReview {
	if role, ok := d.Director.(interface{ HumanReview() *tool.HumanReview }); ok {
		return role.HumanReview()
	}
	return nil
}

func tangyingDirectorBetaPlan() *AgentPlan {
	return &AgentPlan{
		Goal:   "躺营导演台 v1.0 内测：生成 45 秒中文 16:9 图文视频并导出本地成片",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{
				ID:     "proposal",
				Intent: "确认内测版图文视频创作方向",
				Tool:   "proposal_generator",
				Arguments: map[string]interface{}{
					"stage":             "proposal",
					"brief":             "做一个45秒中文图文视频，讲智能体改变的是工作流",
					"aspectRatio":       "16:9",
					"targetDurationSec": float64(45),
				},
				ExpectedOutput:  []string{"proposal", "VIDEO_PROPOSAL"},
				ProduceArtifact: true,
			},
			{
				ID:        "script",
				Intent:    "写中文口播脚本",
				Tool:      "video_script_generator",
				DependsOn: []string{"proposal"},
				Arguments: map[string]interface{}{
					"stage":             "script",
					"topic":             "智能体改变的是工作流",
					"proposal":          "{{proposal.output.proposal}}",
					"targetDurationSec": float64(45),
				},
				ExpectedOutput:  []string{"script", "VIDEO_SCRIPT"},
				ProduceArtifact: true,
			},
			{
				ID:        "storyboard",
				Intent:    "把脚本拆成图文卡片和字幕节奏",
				Tool:      "card_plan_generator",
				DependsOn: []string{"script"},
				Arguments: map[string]interface{}{
					"stage":       "storyboard",
					"script":      "{{script.output.script}}",
					"aspectRatio": "16:9",
				},
				ExpectedOutput:  []string{"cardPlan", "CARD_PLAN"},
				ProduceArtifact: true,
			},
			{
				ID:        "composition",
				Intent:    "生成 HyperFrames 可渲染的视频结构和时间轴",
				Tool:      "video_composition_builder",
				DependsOn: []string{"storyboard"},
				Arguments: map[string]interface{}{
					"stage":       "composition",
					"cardPlan":    "{{storyboard.output.cardPlan}}",
					"aspectRatio": "16:9",
				},
				ExpectedOutput:  []string{"compositionSpec", "VIDEO_COMPOSITION_SPEC"},
				ProduceArtifact: true,
			},
			{
				ID:        "reference",
				Intent:    "按审核卡片、分镜计划和视频结构判断每个 shot 的素材来源",
				Tool:      "asset_decision_agent",
				DependsOn: []string{"storyboard", "composition"},
				Arguments: map[string]interface{}{
					"stage":           "reference",
					"cardPlan":        "{{storyboard.output.cardPlan}}",
					"compositionSpec": "{{composition.output.compositionSpec}}",
				},
				ExpectedOutput:  []string{"referenceAssetPlan", "REFERENCE_ASSET_PLAN"},
				ProduceArtifact: true,
			},
			{
				ID:        "continuity",
				Intent:    "检查风格、字幕、术语和产物依赖一致性",
				Tool:      "continuity_checker",
				DependsOn: []string{"reference"},
				Arguments: map[string]interface{}{
					"stage":              "continuity",
					"referenceAssetPlan": "{{reference.output.referenceAssetPlan}}",
				},
				ExpectedOutput:  []string{"continuityReport", "styleProfile", "CONTINUITY_REPORT", "STYLE_PROFILE"},
				ProduceArtifact: true,
			},
			{
				ID:        "preview",
				Intent:    "生成 HyperFrames 项目和预览截图",
				Tool:      "hyperframes_project_generator",
				DependsOn: []string{"composition", "reference", "continuity"},
				Arguments: map[string]interface{}{
					"stage":            "preview",
					"compositionSpec":  "{{composition.output.compositionSpec}}",
					"styleProfile":     "{{continuity.output.styleProfile}}",
					"previewFrameMode": "snapshots",
				},
				ExpectedOutput:  []string{"hyperframesProject", "previewSnapshots", "previewReport", "HYPERFRAMES_PROJECT", "PREVIEW_SNAPSHOTS", "PREVIEW_REPORT"},
				ProduceArtifact: true,
			},
			{
				ID:        "render",
				Intent:    "确认预览后创建本地 HyperFrames 渲染任务",
				Tool:      "hyperframes_renderer",
				DependsOn: []string{"preview"},
				Arguments: map[string]interface{}{
					"stage":           "render",
					"hyperframesPath": "{{preview.output.hyperframesProject}}",
					"previewApproved": true,
					"outputName":      "final.mp4",
				},
				ExpectedOutput:  []string{"video", "renderReport", "VIDEO", "RENDER_REPORT"},
				ProduceArtifact: true,
			},
			{
				ID:        "quality",
				Intent:    "对最终视频做体检并生成最终审核报告",
				Tool:      "final_review_generator",
				DependsOn: []string{"render"},
				Arguments: map[string]interface{}{
					"stage": "quality",
					"video": "{{render.output.video}}",
				},
				ExpectedOutput:  []string{"ffmpegProbeReport", "finalReview", "FFMPEG_PROBE_REPORT", "FINAL_REVIEW"},
				ProduceArtifact: true,
			},
			{
				ID:        "package",
				Intent:    "打包最终视频、预览图、决策日志和审核报告",
				Tool:      "artifact_packager",
				DependsOn: []string{"quality"},
				Arguments: map[string]interface{}{
					"stage":       "package",
					"finalReview": "{{quality.output.finalReview}}",
					"packageName": "tangying-director-beta-package",
				},
				ExpectedOutput:  []string{"projectPackage", "PROJECT_PACKAGE"},
				ProduceArtifact: true,
			},
		},
		Budget: AgentBudget{
			MaxSteps:     10,
			MaxToolCalls: 10,
			MaxLLMCalls:  8,
			MaxReplans:   1,
			MaxCostLevel: "medium",
		},
		StopPolicy: StopPolicy{StopWhenEnough: true},
	}
}

func tangyingDirectorBetaCatalog() staticToolCatalog {
	return staticToolCatalog{
		"proposal_generator": betaManifest("proposal_generator", []string{"brief", "aspectRatio"}, map[string]string{
			"proposal":       "object",
			"VIDEO_PROPOSAL": "object",
		}, []string{"VIDEO_PROPOSAL"}, tool.ApprovalAfterArtifact, []string{"主题是否准确", "方向是否适合第一版图文视频"}),
		"video_script_generator": betaManifest("video_script_generator", []string{"topic", "proposal", "targetDurationSec"}, map[string]string{
			"script":       "string",
			"VIDEO_SCRIPT": "string",
		}, []string{"VIDEO_SCRIPT"}, tool.ApprovalAfterArtifact, []string{"表达是否自然", "节奏是否适合 45 秒"}),
		"card_plan_generator": betaManifest("card_plan_generator", []string{"script", "aspectRatio"}, map[string]string{
			"cardPlan":  "object",
			"CARD_PLAN": "object",
		}, []string{"CARD_PLAN"}, tool.ApprovalAfterArtifact, []string{"卡片顺序是否合理", "每页文字是否过长"}),
		"video_composition_builder": betaManifest("video_composition_builder", []string{"cardPlan", "aspectRatio"}, map[string]string{
			"compositionSpec":        "object",
			"VIDEO_COMPOSITION_SPEC": "object",
		}, []string{"VIDEO_COMPOSITION_SPEC"}, tool.ApprovalAfterArtifact, []string{"时间轴是否完整", "布局是否适合渲染"}),
		"reference_asset_planner": betaManifest("reference_asset_planner", []string{"compositionSpec"}, map[string]string{
			"referenceAssetPlan":   "object",
			"styleProfile":         "object",
			"REFERENCE_ASSET_PLAN": "object",
			"STYLE_PROFILE":        "object",
		}, []string{"REFERENCE_ASSET_PLAN", "STYLE_PROFILE"}, tool.ApprovalNone, nil),
		"asset_decision_agent": betaManifest("asset_decision_agent", []string{"cardPlan", "compositionSpec"}, map[string]string{
			"referenceAssetPlan":   "object",
			"REFERENCE_ASSET_PLAN": "object",
		}, []string{"REFERENCE_ASSET_PLAN"}, tool.ApprovalNone, nil),
		"continuity_checker": betaManifest("continuity_checker", []string{"referenceAssetPlan"}, map[string]string{
			"continuityReport":  "object",
			"styleProfile":      "object",
			"CONTINUITY_REPORT": "object",
			"STYLE_PROFILE":     "object",
		}, []string{"CONTINUITY_REPORT", "STYLE_PROFILE"}, tool.ApprovalNone, nil),
		"hyperframes_project_generator": betaManifest("hyperframes_project_generator", []string{"compositionSpec", "styleProfile", "previewFrameMode"}, map[string]string{
			"hyperframesProject":  "string",
			"previewSnapshots":    "array",
			"previewReport":       "object",
			"HYPERFRAMES_PROJECT": "object",
			"PREVIEW_SNAPSHOTS":   "object",
			"PREVIEW_REPORT":      "object",
		}, []string{"HYPERFRAMES_PROJECT", "PREVIEW_SNAPSHOTS", "PREVIEW_REPORT"}, tool.ApprovalAfterArtifact, []string{"画面是否可读", "是否允许最终渲染"}),
		"hyperframes_renderer": betaManifest("hyperframes_renderer", []string{"hyperframesPath", "previewApproved", "outputName"}, map[string]string{
			"video":         "string",
			"renderReport":  "object",
			"VIDEO":         "object",
			"RENDER_REPORT": "object",
		}, []string{"VIDEO", "RENDER_REPORT"}, tool.ApprovalBeforeExecute, []string{"预览是否已确认", "是否允许开始本地渲染"}),
		"final_review_generator": betaManifest("final_review_generator", []string{"video"}, map[string]string{
			"ffmpegProbeReport":   "object",
			"finalReview":         "object",
			"FFMPEG_PROBE_REPORT": "object",
			"FINAL_REVIEW":        "object",
		}, []string{"FFMPEG_PROBE_REPORT", "FINAL_REVIEW"}, tool.ApprovalNone, nil),
		"artifact_packager": betaManifest("artifact_packager", []string{"finalReview", "packageName"}, map[string]string{
			"projectPackage":  "object",
			"PROJECT_PACKAGE": "object",
		}, []string{"PROJECT_PACKAGE"}, tool.ApprovalNone, nil),
	}
}

func betaManifest(name string, requiredParams []string, outputs map[string]string, artifactKinds []string, approvalMode string, reviewFocus []string) *tool.ToolManifest {
	params := make(map[string]tool.ParamDef, len(requiredParams)+2)
	for _, param := range requiredParams {
		paramType := "string"
		if strings.Contains(strings.ToLower(param), "approved") {
			paramType = "boolean"
		}
		if strings.Contains(strings.ToLower(param), "duration") {
			paramType = "number"
		}
		params[param] = tool.ParamDef{Type: paramType, Required: true}
	}
	out := make(map[string]tool.ParamDef, len(outputs))
	for field, typ := range outputs {
		out[field] = tool.ParamDef{Type: typ}
	}
	manifest := &tool.ToolManifest{
		Name:        name,
		Description: "躺营导演台 v1.0 内测工具：" + name,
		Type:        "builtin",
		Parameters:  params,
		Output:      out,
		Capabilities: []string{
			"video_creation",
			"tangying_director_beta",
		},
		Tags:      []string{"video", "director", "beta"},
		CostLevel: tool.CostLow,
		RiskLevel: tool.RiskLow,
		ArtifactPolicy: tool.ArtifactPolicy{
			ProduceArtifact:       len(artifactKinds) > 0,
			ArtifactKinds:         artifactKinds,
			DefaultReviewRequired: approvalMode != "" && approvalMode != tool.ApprovalNone,
			Storage:               tool.ArtifactLocationLocal,
		},
	}
	if approvalMode != "" && approvalMode != tool.ApprovalNone {
		manifest.ApprovalPolicy = tool.ApprovalPolicy{
			Required:            true,
			Mode:                approvalMode,
			BlocksDownstream:    true,
			Reason:              "躺营导演台 v1.0 内测阶段需要人工确认",
			ReviewArtifactKinds: artifactKinds,
		}
		manifest.HumanReview = &tool.HumanReview{
			Required:    true,
			Gate:        approvalMode,
			Title:       "人工审核：" + name,
			ReviewFocus: reviewFocus,
			UserActions: []string{"approve", "reject", "edit", "regenerate"},
		}
	}
	if name == "hyperframes_renderer" {
		manifest.ExecutionPlane = tool.ExecutionPlaneLocal
		manifest.LocalCommand = "HYPERFRAMES_RENDER"
		manifest.RequiresUserDevice = true
	}
	return manifest
}

func requireDependencyPath(t *testing.T, edges []model.Edge, from, to string) {
	t.Helper()
	queue := []string{from}
	visited := map[string]bool{from: true}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range edges {
			if edge.From != current {
				continue
			}
			if edge.To == to {
				return
			}
			if !visited[edge.To] {
				visited[edge.To] = true
				queue = append(queue, edge.To)
			}
		}
	}
	t.Fatalf("dependency path %s -> %s not found in %#v", from, to, edges)
}

// dragonBoatPlan returns the expected AgentPlan for the Dragon Boat Festival
// 60-second knowledge video use case.
func dragonBoatPlan() *AgentPlan {
	return &AgentPlan{
		Goal:   "把端午节和粽子的来源做成 60 秒口播知识视频",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{
				ID:     "knowledge_research",
				Intent: "整理端午节和粽子的历史来源、关键事实、讲述角度",
				Tool:   "knowledge_researcher",
				Arguments: map[string]interface{}{
					"topic":       "端午节和粽子的来源",
					"outputStyle": "适合60秒短视频口播",
				},
				ExpectedOutput:  []string{"facts", "timeline", "storyAngles", "risks"},
				ProduceArtifact: true,
			},
			{
				ID:        "fact_check",
				Intent:    "核查端午节和粽子来源的事实准确性",
				Tool:      "fact_checker",
				DependsOn: []string{"knowledge_research"},
				Arguments: map[string]interface{}{
					"facts": "{{knowledge_research.output.facts}}",
					"topic": "端午节和粽子的来源",
				},
				ExpectedOutput:  []string{"checkedFacts", "warnings", "corrections", "passed"},
				ProduceArtifact: true,
			},
			{
				ID:        "script_generation",
				Intent:    "生成60秒口播知识视频脚本",
				Tool:      "video_script_generator",
				DependsOn: []string{"fact_check"},
				Arguments: map[string]interface{}{
					"topic":             "端午节和粽子的来源",
					"facts":             "{{fact_check.output.checkedFacts}}",
					"style":             "真诚、有知识感、适合中文短视频平台",
					"targetDurationSec": float64(60),
				},
				ExpectedOutput:  []string{"script", "summary", "estimatedDurationSec"},
				ProduceArtifact: true,
			},
			{
				ID:        "shot_split",
				Intent:    "把60秒口播稿拆成视频分镜",
				Tool:      "shot_splitter",
				DependsOn: []string{"script_generation"},
				Arguments: map[string]interface{}{
					"script":           "{{script_generation.output.script}}",
					"shotDurationRule": "3-10秒",
					"aspectRatio":      "16:9",
				},
				ExpectedOutput:  []string{"shotList", "totalDurationSec"},
				ProduceArtifact: true,
			},
			{
				ID:        "video_prompt_generation",
				Intent:    "根据分镜生成视频生成 Prompt",
				Tool:      "video_prompt_generator",
				DependsOn: []string{"shot_split"},
				Arguments: map[string]interface{}{
					"shotList": "{{shot_split.output.shotList}}",
					"style":    "非写实动画，去AI感，中文知识分享视频，16:9",
				},
				ExpectedOutput:  []string{"videoPrompts"},
				ProduceArtifact: true,
			},
			{
				ID:        "publish_copy",
				Intent:    "生成标题、简介和标签",
				Tool:      "publish_copy_generator",
				DependsOn: []string{"script_generation"},
				Arguments: map[string]interface{}{
					"script":   "{{script_generation.output.script}}",
					"topic":    "端午节和粽子的来源",
					"platform": "小红书",
				},
				ExpectedOutput:  []string{"title", "description", "tags"},
				ProduceArtifact: true,
			},
			{
				ID:     "package_export",
				Intent: "导出完整视频创作包",
				Tool:   "video_package_exporter",
				DependsOn: []string{
					"knowledge_research",
					"fact_check",
					"script_generation",
					"shot_split",
					"video_prompt_generation",
					"publish_copy",
				},
				Arguments: map[string]interface{}{
					"topic":        "端午节和粽子的来源",
					"script":       "{{script_generation.output.script}}",
					"shotList":     "{{shot_split.output.shotList}}",
					"videoPrompts": "{{video_prompt_generation.output.videoPrompts}}",
					"facts":        "{{knowledge_research.output.facts}}",
					"checkedFacts": "{{fact_check.output.checkedFacts}}",
					"publishCopy":  "{{publish_copy.output.title}}",
				},
				ExpectedOutput:  []string{"packageMarkdown", "packageManifest"},
				ProduceArtifact: true,
			},
		},
		Budget: AgentBudget{
			MaxSteps:     10,
			MaxToolCalls: 12,
			MaxLLMCalls:  10,
			MaxReplans:   1,
			MaxCostLevel: "medium",
		},
		StopPolicy: StopPolicy{StopWhenEnough: true},
	}
}

// dragonBoatToolCatalog returns a tool catalog with all video creation tools,
// including quality policies and approval policies.
func dragonBoatToolCatalog() staticToolCatalog {
	return staticToolCatalog{
		"knowledge_researcher": &tool.ToolManifest{
			Name:        "knowledge_researcher",
			Description: "知识研究助手",
			Type:        "builtin_prompt_tool",
			Endpoint:    "builtin://video-creation/knowledge_researcher",
			Parameters: map[string]tool.ParamDef{
				"topic":       {Type: "string", Required: true},
				"outputStyle": {Type: "string", Required: false},
			},
			Output: map[string]tool.ParamDef{
				"facts":       {Type: "array"},
				"timeline":    {Type: "array"},
				"storyAngles": {Type: "array"},
				"risks":       {Type: "array"},
				"sourceNotes": {Type: "array"},
				"summary":     {Type: "string"},
			},
			Capabilities: []string{"video_creation", "knowledge_research"},
			Tags:         []string{"video", "research", "knowledge"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskLow,
			ApprovalPolicy: tool.ApprovalPolicy{
				Required:         true,
				Mode:             tool.ApprovalAfterArtifact,
				BlocksDownstream: true,
				Reason:           "知识基础需用户确认",
			},
			ArtifactPolicy: tool.ArtifactPolicy{
				ProduceArtifact:       true,
				ArtifactKinds:         []string{"JSON", "MARKDOWN"},
				DefaultReviewRequired: true,
			},
			NextRecommendedTools: []string{"fact_checker", "video_script_generator"},
		},
		"fact_checker": &tool.ToolManifest{
			Name:        "fact_checker",
			Description: "事实核查员",
			Type:        "builtin_prompt_tool",
			Endpoint:    "builtin://video-creation/fact_checker",
			Parameters: map[string]tool.ParamDef{
				"facts": {Type: "array", Required: true},
				"topic": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"checkedFacts": {Type: "array"},
				"warnings":     {Type: "array"},
				"corrections":  {Type: "array"},
				"passed":       {Type: "boolean"},
				"summary":      {Type: "string"},
			},
			Capabilities: []string{"video_creation", "fact_checking"},
			Tags:         []string{"video", "fact_check", "review"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskLow,
			ApprovalPolicy: tool.ApprovalPolicy{
				Required: false,
			},
			ArtifactPolicy: tool.ArtifactPolicy{
				ProduceArtifact: true,
				ArtifactKinds:   []string{"JSON", "MARKDOWN"},
			},
			NextRecommendedTools: []string{"video_script_generator"},
		},
		"video_script_generator": &tool.ToolManifest{
			Name:        "video_script_generator",
			Description: "口播稿创作专家",
			Type:        "builtin_prompt_tool",
			Endpoint:    "builtin://video-creation/video_script_generator",
			Parameters: map[string]tool.ParamDef{
				"topic":             {Type: "string", Required: true},
				"facts":             {Type: "array", Required: false},
				"style":             {Type: "string", Required: false},
				"targetDurationSec": {Type: "number", Required: false},
			},
			Output: map[string]tool.ParamDef{
				"script":               {Type: "string"},
				"summary":              {Type: "string"},
				"estimatedDurationSec": {Type: "number"},
				"sections":             {Type: "array"},
				"qualityHints":         {Type: "object"},
			},
			Capabilities: []string{"video_creation", "script_generation"},
			Tags:         []string{"video", "script", "oral_script"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskLow,
			QualityPolicy: tool.QualityPolicy{
				Required:    true,
				CheckerTool: "script_quality_checker",
				MinScore:    85,
				AutoRepair:  true,
			},
			ApprovalPolicy: tool.ApprovalPolicy{
				Required:         true,
				Mode:             tool.ApprovalAfterArtifact,
				BlocksDownstream: true,
				Reason:           "口播稿会影响后续分镜和视频Prompt，必须确认后继续",
			},
			ArtifactPolicy: tool.ArtifactPolicy{
				ProduceArtifact:       true,
				ArtifactKinds:         []string{"JSON", "MARKDOWN"},
				DefaultReviewRequired: true,
			},
			NextRecommendedTools: []string{"script_quality_checker", "shot_splitter"},
		},
		"script_quality_checker": &tool.ToolManifest{
			Name:        "script_quality_checker",
			Description: "口播稿质量审核员",
			Type:        "builtin_prompt_tool",
			Endpoint:    "builtin://video-creation/script_quality_checker",
			Parameters: map[string]tool.ParamDef{
				"script":            {Type: "string", Required: true},
				"targetDurationSec": {Type: "number", Required: false},
			},
			Output: map[string]tool.ParamDef{
				"passed":            {Type: "boolean"},
				"score":             {Type: "number"},
				"issues":            {Type: "array"},
				"repairSuggestions": {Type: "array"},
			},
			Capabilities: []string{"video_creation", "quality_check"},
			Tags:         []string{"video", "quality", "script", "review"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskLow,
			ApprovalPolicy: tool.ApprovalPolicy{
				Required: false,
			},
			ArtifactPolicy: tool.ArtifactPolicy{
				ProduceArtifact: true,
				ArtifactKinds:   []string{"JSON"},
			},
		},
		"shot_splitter": &tool.ToolManifest{
			Name:        "shot_splitter",
			Description: "分镜导演",
			Type:        "builtin_prompt_tool",
			Endpoint:    "builtin://video-creation/shot_splitter",
			Parameters: map[string]tool.ParamDef{
				"script":           {Type: "string", Required: true},
				"shotDurationRule": {Type: "string", Required: false},
				"aspectRatio":      {Type: "string", Required: false},
			},
			Output: map[string]tool.ParamDef{
				"shotList":         {Type: "array"},
				"totalDurationSec": {Type: "number"},
				"summary":          {Type: "string"},
			},
			Capabilities: []string{"video_creation", "shot_split"},
			Tags:         []string{"video", "shot", "storyboard"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskLow,
			QualityPolicy: tool.QualityPolicy{
				Required:    true,
				CheckerTool: "shot_quality_checker",
				MinScore:    85,
				AutoRepair:  true,
			},
			ApprovalPolicy: tool.ApprovalPolicy{
				Required:         true,
				Mode:             tool.ApprovalAfterArtifact,
				BlocksDownstream: true,
				Reason:           "分镜会影响后续视频Prompt和生成成本，必须确认后继续",
			},
			ArtifactPolicy: tool.ArtifactPolicy{
				ProduceArtifact:       true,
				ArtifactKinds:         []string{"JSON", "MARKDOWN"},
				DefaultReviewRequired: true,
			},
			NextRecommendedTools: []string{"shot_quality_checker", "video_prompt_generator"},
		},
		"shot_quality_checker": &tool.ToolManifest{
			Name:        "shot_quality_checker",
			Description: "分镜质量审核员",
			Type:        "builtin_prompt_tool",
			Endpoint:    "builtin://video-creation/shot_quality_checker",
			Parameters: map[string]tool.ParamDef{
				"shotList": {Type: "array", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"passed":            {Type: "boolean"},
				"score":             {Type: "number"},
				"issues":            {Type: "array"},
				"repairSuggestions": {Type: "array"},
			},
			Capabilities: []string{"video_creation", "quality_check"},
			Tags:         []string{"video", "quality", "shot", "review"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskLow,
			ApprovalPolicy: tool.ApprovalPolicy{
				Required: false,
			},
			ArtifactPolicy: tool.ArtifactPolicy{
				ProduceArtifact: true,
				ArtifactKinds:   []string{"JSON"},
			},
		},
		"video_prompt_generator": &tool.ToolManifest{
			Name:        "video_prompt_generator",
			Description: "视频提示词导演",
			Type:        "builtin_prompt_tool",
			Endpoint:    "builtin://video-creation/video_prompt_generator",
			Parameters: map[string]tool.ParamDef{
				"shotList":  {Type: "array", Required: true},
				"style":     {Type: "string", Required: false},
				"modelHint": {Type: "string", Required: false},
			},
			Output: map[string]tool.ParamDef{
				"videoPrompts": {Type: "array"},
				"summary":      {Type: "string"},
			},
			Capabilities: []string{"video_creation", "video_prompt_generation"},
			Tags:         []string{"video", "prompt", "generation"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskLow,
			QualityPolicy: tool.QualityPolicy{
				Required:    true,
				CheckerTool: "video_prompt_quality_checker",
				MinScore:    85,
				AutoRepair:  true,
			},
			ApprovalPolicy: tool.ApprovalPolicy{
				Required:         true,
				Mode:             tool.ApprovalAfterArtifact,
				BlocksDownstream: true,
				Reason:           "视频Prompt直接影响生成质量和成本，必须确认后继续",
			},
			ArtifactPolicy: tool.ArtifactPolicy{
				ProduceArtifact:       true,
				ArtifactKinds:         []string{"JSON", "MARKDOWN"},
				DefaultReviewRequired: true,
			},
			NextRecommendedTools: []string{"video_prompt_quality_checker", "publish_copy_generator"},
		},
		"video_prompt_quality_checker": &tool.ToolManifest{
			Name:        "video_prompt_quality_checker",
			Description: "视频提示词质量审核员",
			Type:        "builtin_prompt_tool",
			Endpoint:    "builtin://video-creation/video_prompt_quality_checker",
			Parameters: map[string]tool.ParamDef{
				"videoPrompts": {Type: "array", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"passed":            {Type: "boolean"},
				"score":             {Type: "number"},
				"issues":            {Type: "array"},
				"repairSuggestions": {Type: "array"},
			},
			Capabilities: []string{"video_creation", "quality_check"},
			Tags:         []string{"video", "quality", "prompt", "review"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskLow,
			ApprovalPolicy: tool.ApprovalPolicy{
				Required: false,
			},
			ArtifactPolicy: tool.ArtifactPolicy{
				ProduceArtifact: true,
				ArtifactKinds:   []string{"JSON"},
			},
		},
		"publish_copy_generator": &tool.ToolManifest{
			Name:        "publish_copy_generator",
			Description: "短视频平台运营专家",
			Type:        "builtin_prompt_tool",
			Endpoint:    "builtin://video-creation/publish_copy_generator",
			Parameters: map[string]tool.ParamDef{
				"script":   {Type: "string", Required: false},
				"topic":    {Type: "string", Required: false},
				"platform": {Type: "string", Required: false},
			},
			Output: map[string]tool.ParamDef{
				"title":       {Type: "string"},
				"description": {Type: "string"},
				"tags":        {Type: "array"},
			},
			Capabilities: []string{"video_creation", "publish_copy"},
			Tags:         []string{"video", "publish", "copywriting"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskLow,
			ArtifactPolicy: tool.ArtifactPolicy{
				ProduceArtifact: true,
				ArtifactKinds:   []string{"MARKDOWN"},
			},
		},
		"video_package_exporter": &tool.ToolManifest{
			Name:        "video_package_exporter",
			Description: "视频创作包交付经理",
			Type:        "builtin_prompt_tool",
			Endpoint:    "builtin://video-creation/video_package_exporter",
			Parameters: map[string]tool.ParamDef{
				"topic":        {Type: "string", Required: true},
				"script":       {Type: "string", Required: false},
				"shotList":     {Type: "array", Required: false},
				"videoPrompts": {Type: "array", Required: false},
				"facts":        {Type: "array", Required: false},
				"publishCopy":  {Type: "string", Required: false},
			},
			Output: map[string]tool.ParamDef{
				"packageMarkdown": {Type: "string"},
				"packageManifest": {Type: "object"},
			},
			Capabilities: []string{"video_creation", "package_export"},
			Tags:         []string{"video", "package", "export"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskLow,
			ApprovalPolicy: tool.ApprovalPolicy{
				Required: false,
			},
			ArtifactPolicy: tool.ArtifactPolicy{
				ProduceArtifact: true,
				ArtifactKinds:   []string{"JSON", "MARKDOWN"},
			},
		},
	}
}

// TestE2E_PrintDAGStructure prints a human-readable DAG structure for debugging.
func TestE2E_PrintDAGStructure(t *testing.T) {
	catalog := dragonBoatToolCatalog()
	plan := dragonBoatPlan()

	compiler := NewPlanCompiler(catalog)
	dag, err := compiler.Compile(plan)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	t.Logf("\n=== DAG Structure ===")
	t.Logf("Nodes (%d):", len(dag.Nodes))
	for _, n := range dag.Nodes {
		phase := ""
		if n.Input != nil {
			if p, ok := n.Input["reviewPhase"].(string); ok {
				phase = fmt.Sprintf(" [phase=%s]", p)
			}
			if ap, ok := n.Input["autoApproveWhenPassed"]; ok && ap == true {
				phase += " [autoApprove]"
			}
		}
		t.Logf("  %-55s type=%-8s name=%-30s%s", n.ID, n.Type, n.Name, phase)
	}
	t.Logf("\nEdges (%d):", len(dag.Edges))
	for _, e := range dag.Edges {
		t.Logf("  %s → %s", e.From, e.To)
	}
}
