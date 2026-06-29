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
	c.completeVideoBetaPlan(plan)
	repairInvalidOutputReferences(plan.Steps, c.manifestsByPlan(plan))
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

	projectAnchor, projectField := c.lastProducerStepForFields(plan, []string{"projectDir", "hyperframesPath"}, []string{"hyperframes_project_generator"})
	if projectAnchor == "" {
		projectAnchor = appendPlanStep(plan, AgentStep{
			ID:        uniqueStepID(plan, "preview"),
			Intent:    "生成可审核的 HyperFrames 预览项目和画面预览",
			Tool:      "hyperframes_project_generator",
			DependsOn: dependencyListUnique(shotAnchor, scriptAnchor),
			Arguments: map[string]interface{}{
				"stage":    "preview",
				"brief":    plan.Goal,
				"topic":    plan.Goal,
				"script":   scriptRef,
				"shotList": stepOutputRef(shotAnchor, shotField),
			},
			ExpectedOutput:  []string{"HYPERFRAMES_PROJECT", "PREVIEW_SNAPSHOTS", "PREVIEW_REPORT", "hyperframes_project", "preview"},
			ProduceArtifact: true,
		})
		projectField = preferredOutputField(c.manifestFor("hyperframes_project_generator"), "projectDir", "hyperframesPath")
	}

	renderAnchor, _ := c.lastProducerStepForFields(plan, []string{"outputPath", "finalVideo", "video"}, []string{"hyperframes_renderer"})
	if renderAnchor == "" {
		renderManifest := c.manifestFor("hyperframes_renderer")
		projectParam := preferredParamName(renderManifest, "projectDir", "hyperframesPath")
		renderArgs := map[string]interface{}{
			"stage":           "render",
			"brief":           plan.Goal,
			projectParam:      stepOutputRef(projectAnchor, projectField),
			"previewApproved": true,
			"outputName":      "final.mp4",
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
	}

	if !planHasToolOrTerm(plan, []string{"publish_copy", "publishcopy", "publish-copy", "publish_copy_generator"}) {
		appendPlanStep(plan, AgentStep{
			ID:        uniqueStepID(plan, "publish_copy"),
			Intent:    "基于成片和脚本生成多平台发布文案",
			Tool:      "publish_copy_generator",
			DependsOn: dependencyListUnique(renderAnchor, scriptAnchor, shotAnchor),
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
}

func appendPlanStep(plan *AgentPlan, step AgentStep) string {
	plan.Steps = append(plan.Steps, step)
	return step.ID
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

func stepOutputRef(stepID, field string) string {
	if stepID == "" || field == "" {
		return ""
	}
	return fmt.Sprintf("{{%s.output.%s}}", stepID, field)
}

func (c *PlanCompiler) hasVideoBetaCompletionTools() bool {
	for _, name := range []string{"shot_splitter", "hyperframes_project_generator", "hyperframes_renderer", "publish_copy_generator"} {
		if c.manifestFor(name) == nil {
			return false
		}
	}
	return true
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
