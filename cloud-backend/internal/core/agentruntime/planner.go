package agentruntime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

type ToolListProvider interface {
	ListManifests() []*tool.ToolManifest
}

type HeuristicPlanner struct {
	tools    ToolListProvider
	maxTools int
}

func NewHeuristicPlanner(tools ToolListProvider) *HeuristicPlanner {
	return &HeuristicPlanner{tools: tools, maxTools: 6}
}

func NewHeuristicPlannerWithMaxTools(tools ToolListProvider, maxTools int) *HeuristicPlanner {
	if maxTools <= 0 {
		maxTools = 6
	}
	return &HeuristicPlanner{tools: tools, maxTools: maxTools}
}

func (p *HeuristicPlanner) GeneratePlan(_ context.Context, req StartRunRequest) (*AgentPlan, error) {
	if p == nil || p.tools == nil {
		return nil, fmt.Errorf("heuristic planner is not configured")
	}
	domain := req.Domain
	if domain == "" {
		domain = inferDomain(req.Message)
	}

	knowledgePolicy := defaultKnowledgePolicyForTools(req.Message, domain, p.tools.ListManifests())
	selected := p.selectTools(domain, req.Message)
	var traceCandidates []ToolCandidateTrace
	candidates, err := NewHybridToolRetriever(p.tools.ListManifests()).Retrieve(context.Background(), ToolRetrieveRequest{
		UserInput:       req.Message,
		Domain:          domain,
		KnowledgePolicy: knowledgePolicy,
		MaxCandidates:   p.maxTools,
		MaxCostLevel:    req.MaxCostLevel,
		MaxRiskLevel:    req.MaxRiskLevel,
	})
	if err != nil {
		return nil, err
	}
	if knowledgePolicyAllowsFreshTools(knowledgePolicy) && len(candidates) > 0 {
		selected = candidateManifests(candidates)
	}
	if len(candidates) > 0 {
		traceCandidates = candidateTrace(candidates)
	}
	selected = filterManifestsByKnowledgePolicy(selected, knowledgePolicy)
	if len(selected) == 0 {
		return nil, fmt.Errorf("no tools matched domain %q", domain)
	}

	manifests := manifestMap(p.tools.ListManifests())
	steps := make([]AgentStep, 0, len(selected))
	var previous string
	for _, manifest := range selected {
		stepID := sanitizeStepID(manifest.Name)
		args := copyRequestContext(req)
		step := AgentStep{
			ID:              stepID,
			Intent:          manifest.Description,
			Tool:            manifest.Name,
			Arguments:       args,
			ExpectedOutput:  outputKeys(manifest.Output),
			ProduceArtifact: manifest.ArtifactPolicy.ProduceArtifact,
		}
		if previous != "" {
			step.DependsOn = []string{previous}
		}
		steps = append(steps, step)
		previous = stepID
	}
	fillRequestRequiredInputs(steps, manifests, req)
	wireRequiredStepInputs(steps, manifests)

	plan := &AgentPlan{
		Goal:            req.Message,
		Domain:          domain,
		Mode:            "dynamic_agent",
		KnowledgePolicy: knowledgePolicy,
		ToolTrace:       &ToolTrace{CandidateTools: traceCandidates},
		Steps:           steps,
		Budget: AgentBudget{
			MaxToolCalls: len(steps),
			MaxSteps:     max(len(steps), p.maxTools),
			MaxReplans:   1,
			MaxCostLevel: tool.CostMedium,
		},
		StopPolicy: StopPolicy{StopWhenEnough: true},
	}

	catalog := toolManifestCatalog(manifests)
	plan = NewPlanCompiler(catalog).PreparePlan(plan)

	// Validate the prepared plan before returning, same as LLMPlanner does.
	if err := NewPlanGuard(catalog, nil).Validate(plan); err != nil {
		return nil, fmt.Errorf("heuristic planner returned invalid plan: %w", err)
	}

	return plan, nil
}

func filterManifestsByKnowledgePolicy(manifests []*tool.ToolManifest, policy *KnowledgePolicy) []*tool.ToolManifest {
	if policy == nil {
		return manifests
	}
	out := manifests[:0]
	for _, manifest := range manifests {
		if toolForbiddenByKnowledgePolicy(manifest, policy) {
			continue
		}
		if hasFreshKnowledgeCapability(manifest) && !knowledgePolicyAllowsFreshTools(policy) {
			continue
		}
		out = append(out, manifest)
	}
	return out
}

func defaultKnowledgePolicyForTools(message, domain string, manifests []*tool.ToolManifest) *KnowledgePolicy {
	policy := DefaultKnowledgePolicy(message, domain)
	if policy == nil {
		return nil
	}
	_ = manifests
	return policy
}

func (p *HeuristicPlanner) selectTools(domain, message string) []*tool.ToolManifest {
	type scored struct {
		manifest *tool.ToolManifest
		score    int
		index    int
		phase    int
	}
	all := p.tools.ListManifests()
	scoredTools := make([]scored, 0, len(all))
	hasDomainCapability := false
	for i, manifest := range all {
		if manifest == nil || manifest.Name == "" {
			continue
		}
		if plannerDisallowsToolForDomain(manifest.Name, domain) {
			continue
		}
		if hasCapability(manifest, domain) {
			hasDomainCapability = true
		}
		score := scoreTool(manifest, domain, message)
		if score <= 0 {
			continue
		}
		scoredTools = append(scoredTools, scored{manifest: manifest, score: score, index: i, phase: phaseRank(manifest)})
	}
	if hasDomainCapability {
		filtered := scoredTools[:0]
		for _, item := range scoredTools {
			if hasCapability(item.manifest, domain) {
				filtered = append(filtered, item)
			}
		}
		scoredTools = filtered
	}
	sort.SliceStable(scoredTools, func(i, j int) bool {
		if scoredTools[i].score == scoredTools[j].score {
			if scoredTools[i].phase != scoredTools[j].phase {
				return scoredTools[i].phase < scoredTools[j].phase
			}
			return scoredTools[i].index < scoredTools[j].index
		}
		return scoredTools[i].score > scoredTools[j].score
	})

	limit := p.maxTools
	if limit <= 0 {
		limit = 6
	}
	if len(scoredTools) < limit {
		limit = len(scoredTools)
	}
	result := make([]*tool.ToolManifest, 0, limit)
	for i := 0; i < limit; i++ {
		result = append(result, scoredTools[i].manifest)
	}
	return result
}

func hasCapability(manifest *tool.ToolManifest, capability string) bool {
	if manifest == nil || capability == "" {
		return false
	}
	for _, cap := range manifest.Capabilities {
		if cap == capability {
			return true
		}
	}
	return false
}

func phaseRank(manifest *tool.ToolManifest) int {
	joined := strings.Join(append([]string{manifest.Name}, manifest.Capabilities...), " ")
	switch {
	case strings.Contains(joined, "pipeline_selection"):
		return 1
	case strings.Contains(joined, "capability_preflight"):
		return 2
	case strings.Contains(joined, "proposal_generation"):
		return 3
	case strings.Contains(joined, "script_generation"):
		return 10
	case strings.Contains(joined, "storyboard_generation") || strings.Contains(joined, "shot_planning"):
		return 20
	case strings.Contains(joined, "visual_feasibility"):
		return 25
	case strings.Contains(joined, "render_strategy"):
		return 26
	case strings.Contains(joined, "video_prompt_generation"):
		return 30
	default:
		return 100
	}
}

func scoreTool(manifest *tool.ToolManifest, domain, message string) int {
	score := 0
	haystack := strings.ToLower(strings.Join(append(append([]string{manifest.Name, manifest.Description}, manifest.Capabilities...), manifest.Tags...), " "))
	if domain != "" {
		for _, cap := range manifest.Capabilities {
			if cap == domain {
				score += 100
			}
		}
		for _, tag := range manifest.Tags {
			if tag == domain {
				score += 30
			}
		}
	}
	for _, token := range tokenize(message) {
		if strings.Contains(haystack, token) {
			score += 5
		}
	}
	if strings.Contains(message, "视频") && strings.Contains(haystack, "video") {
		score += 20
	}
	return score
}

func inferDomain(message string) string {
	if strings.Contains(message, "视频") || strings.Contains(strings.ToLower(message), "video") {
		return "video_creation"
	}
	return "general"
}

func copyRequestContext(req StartRunRequest) map[string]interface{} {
	args := make(map[string]interface{}, len(req.Context)+2)
	for k, v := range req.Context {
		if isSensitiveModelProviderContextKey(k) {
			continue
		}
		args[k] = v
	}
	args["brief"] = req.Message
	if topic, ok := args["topic"].(string); !ok || strings.TrimSpace(topic) == "" {
		args["topic"] = req.Message
	}
	return args
}

func manifestMap(manifests []*tool.ToolManifest) map[string]*tool.ToolManifest {
	result := make(map[string]*tool.ToolManifest, len(manifests))
	for _, manifest := range manifests {
		if manifest == nil || manifest.Name == "" {
			continue
		}
		result[manifest.Name] = manifest
	}
	return result
}

func wireRequiredStepInputs(steps []AgentStep, manifests map[string]*tool.ToolManifest) {
	producedByField := make(map[string]string)
	for i := range steps {
		step := &steps[i]
		if step.Arguments == nil {
			step.Arguments = map[string]interface{}{}
		}
		manifest := manifests[step.Tool]
		if manifest != nil {
			for _, name := range requiredParamNames(manifest.Parameters) {
				if _, ok := step.Arguments[name]; ok {
					continue
				}
				producerID, ok := producedByField[name]
				if !ok {
					continue
				}
				step.Arguments[name] = fmt.Sprintf("{{%s.output.%s}}", producerID, name)
				appendDependencyIfMissing(step, producerID)
			}
		}
		if manifest == nil {
			continue
		}
		for _, field := range outputKeys(manifest.Output) {
			producedByField[field] = step.ID
		}
	}
}

type stepOutputReference struct {
	stepID string
	field  string
}

func repairInvalidOutputReferences(steps []AgentStep, manifests map[string]*tool.ToolManifest) {
	producedByField := make(map[string]stepOutputReference)
	stepByID := make(map[string]AgentStep, len(steps))
	for _, step := range steps {
		stepByID[step.ID] = step
	}

	for i := range steps {
		step := &steps[i]
		if step.Arguments == nil {
			step.Arguments = map[string]interface{}{}
		}
		for argName, value := range step.Arguments {
			refStepID, refField, ok := outputReference(value)
			if !ok {
				continue
			}
			refStep, exists := stepByID[refStepID]
			if !exists {
				continue
			}
			refManifest := manifests[refStep.Tool]
			if manifestDeclaresOutput(refManifest, refField) {
				appendDependencyIfMissing(step, refStepID)
				continue
			}
			if replacement, ok := replacementOutputReference(argName, refField, producedByField); ok {
				step.Arguments[argName] = stepOutputRef(replacement.stepID, replacement.field)
				appendDependencyIfMissing(step, replacement.stepID)
			}
		}

		manifest := manifests[step.Tool]
		for field := range manifestOutputs(manifest) {
			ref := stepOutputReference{stepID: step.ID, field: field}
			producedByField[field] = ref
			for _, alias := range aliasesForOutputField(field) {
				producedByField[alias] = ref
			}
		}
		for _, field := range fallbackOutputsForTool(step.Tool, manifest) {
			ref := stepOutputReference{stepID: step.ID, field: field}
			producedByField[field] = ref
			for _, alias := range aliasesForOutputField(field) {
				producedByField[alias] = ref
			}
		}
	}
}

func outputReference(value interface{}) (string, string, bool) {
	s, ok := value.(string)
	if !ok {
		return "", "", false
	}
	matches := referencePattern.FindStringSubmatch(s)
	if matches == nil {
		return "", "", false
	}
	return matches[1], matches[2], true
}

func manifestDeclaresOutput(manifest *tool.ToolManifest, field string) bool {
	if manifest == nil || len(manifest.Output) == 0 {
		return true
	}
	_, ok := manifest.Output[field]
	return ok
}

func replacementOutputReference(argName, refField string, producedByField map[string]stepOutputReference) (stepOutputReference, bool) {
	for _, key := range referenceRepairKeys(argName, refField) {
		if ref, ok := producedByField[key]; ok {
			return ref, true
		}
	}
	return stepOutputReference{}, false
}

func referenceRepairKeys(argName, refField string) []string {
	keys := make([]string, 0, 6)
	add := func(key string) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		for _, existing := range keys {
			if existing == key {
				return
			}
		}
		keys = append(keys, key)
	}
	add(argName)
	add(refField)
	for _, alias := range aliasesForOutputField(argName) {
		add(alias)
	}
	for _, alias := range aliasesForOutputField(refField) {
		add(alias)
	}
	return keys
}

func manifestOutputs(manifest *tool.ToolManifest) map[string]tool.ParamDef {
	if manifest == nil || manifest.Output == nil {
		return nil
	}
	return manifest.Output
}

func fallbackOutputsForTool(toolName string, manifest *tool.ToolManifest) []string {
	if manifest != nil && len(manifest.Output) > 0 {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "video_script_generator", "script_generator":
		return []string{"script"}
	case "shot_splitter":
		return []string{"shotList"}
	case "hyperframes_project_generator":
		return []string{"projectDir"}
	case "hyperframes_renderer":
		return []string{"outputPath"}
	default:
		return nil
	}
}

func aliasesForOutputField(field string) []string {
	switch field {
	case "script":
		return []string{"voiceover_script"}
	case "shotList":
		return []string{"shot_list", "beat_plan", "visual_component_plan"}
	case "projectDir":
		return []string{"hyperframesPath", "hyperframesProject", "hyperframes_project"}
	case "outputPath":
		return []string{"finalVideo", "final_video", "video"}
	default:
		return nil
	}
}

func fillRequestRequiredInputs(steps []AgentStep, manifests map[string]*tool.ToolManifest, req StartRunRequest) {
	for i := range steps {
		step := &steps[i]
		if step.Arguments == nil {
			step.Arguments = map[string]interface{}{}
		}
		manifest := manifests[step.Tool]
		if manifest == nil {
			continue
		}
		for _, name := range requiredParamNames(manifest.Parameters) {
			if _, ok := step.Arguments[name]; ok {
				continue
			}
			if req.Context != nil {
				if value, ok := req.Context[name]; ok {
					step.Arguments[name] = value
					continue
				}
			}
			switch name {
			case "topic", "brief", "query", "searchQuery", "prompt":
				if req.Message != "" {
					step.Arguments[name] = req.Message
				}
			case "queries", "searchQueries":
				if req.Message != "" {
					step.Arguments[name] = []string{req.Message}
				}
			}
		}
	}
}

func requiredParamNames(params map[string]tool.ParamDef) []string {
	names := make([]string, 0, len(params))
	for name, param := range params {
		if param.Required {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func appendDependencyIfMissing(step *AgentStep, dep string) {
	if step == nil || dep == "" || dep == step.ID {
		return
	}
	for _, existing := range step.DependsOn {
		if existing == dep {
			return
		}
	}
	step.DependsOn = append(step.DependsOn, dep)
}

func outputKeys(output map[string]tool.ParamDef) []string {
	if len(output) == 0 {
		return nil
	}
	keys := make([]string, 0, len(output))
	for key := range output {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sanitizeStepID(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore && b.Len() > 0 {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func tokenize(message string) []string {
	fields := strings.FieldsFunc(strings.ToLower(message), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
	result := make([]string, 0, len(fields))
	for _, f := range fields {
		if len([]rune(f)) >= 2 {
			result = append(result, f)
		}
	}
	return result
}
