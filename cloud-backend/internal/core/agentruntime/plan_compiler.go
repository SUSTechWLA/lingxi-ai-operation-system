package agentruntime

import (
	"fmt"
	"strings"

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
	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("agent plan has no steps")
	}

	// Detect missing quality checkers and auto-insert them.
	steps := c.injectQualityGates(plan.Steps)

	// Enforce stage-level tool restrictions via Directors.
	steps = c.enforceDirectors(steps, plan.Domain)

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
				"productionStep":        step.ID,
				"productionTool":        step.Tool,
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

	return out
}

func qualityCheckerFor(toolName string, manifest *tool.ToolManifest) (string, bool) {
	if manifest != nil && manifest.QualityPolicy.Required && manifest.QualityPolicy.CheckerTool != "" {
		return manifest.QualityPolicy.CheckerTool, true
	}
	name := QualityCheckerFor(toolName)
	return name, name != ""
}

// buildQualityCheckArgs constructs arguments for an auto-inserted quality checker step.
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
	return args
}

// enforceDirectors checks each step's tool against the stage director's constraints.
// Forbidden tools and tools exceeding the max call limit are removed from the plan.
// This is a safety net that prevents the LLM from calling restricted tools regardless
// of what the prompt says.
func (c *PlanCompiler) enforceDirectors(steps []AgentStep, domain string) []AgentStep {
	if c.directors == nil {
		return steps
	}
	if domain != "video_creation" {
		return steps
	}

	// Try to resolve a stage name from the first step's context.
	stageName := ""
	for _, s := range steps {
		if s.Arguments != nil {
			if sn, ok := s.Arguments["stage"].(string); ok && sn != "" {
				stageName = sn
				break
			}
		}
	}
	if stageName == "" {
		return steps
	}

	d := c.directors.Get(stageName)
	if d == nil {
		return steps
	}

	// Build forbid-set and allow-set for fast lookup.
	forbidden := stringSet(d.ForbiddenTools())
	allowed := stringSet(d.AllowedTools())
	maxCalls := d.MaxToolCalls()
	if maxCalls <= 0 {
		maxCalls = 10
	}

	callCount := 0
	filtered := make([]AgentStep, 0, len(steps))
	for _, s := range steps {
		// Skip internal marker tools.
		if s.Tool == "__quality_gate__" {
			filtered = append(filtered, s)
			continue
		}
		// Quality-checker tools are always allowed.
		if isQualityCheckerTool(s.Tool) {
			filtered = append(filtered, s)
			continue
		}

		if forbidden[s.Tool] {
			continue // silently drop forbidden tool
		}
		if len(allowed) > 0 && !allowed[s.Tool] {
			continue // tool not in allow-list
		}
		if callCount >= maxCalls {
			continue // exceeded stage tool limit
		}
		callCount++
		filtered = append(filtered, s)
	}
	return filtered
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
				buildReviewNode(beforeID, step, policy, "before_execute"),
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
				buildReviewNode(reviewID, step, policy, "after_artifact"),
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
				buildReviewNode(beforeID, step, policy, "before_execute"),
				buildToolNode(execID, step, manifest),
				buildReviewNode(afterID, step, policy, "after_artifact"),
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

func buildReviewNode(nodeID string, step AgentStep, policy tool.ApprovalPolicy, phase string) model.NodeRequest {
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

	return model.NodeRequest{
		ID:    nodeID,
		Type:  string(model.NodeTypeReviewGate),
		Name:  "审核-" + step.ID,
		Input: input,
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
