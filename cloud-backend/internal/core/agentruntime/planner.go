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

	selected := p.selectTools(domain, req.Message)
	var traceCandidates []ToolCandidateTrace
	if RequiresFreshKnowledge(req.Message) {
		candidates, err := NewHybridToolRetriever(p.tools.ListManifests()).Retrieve(context.Background(), ToolRetrieveRequest{
			UserInput:     req.Message,
			Domain:        domain,
			MaxCandidates: p.maxTools,
			MaxCostLevel:  req.MaxCostLevel,
			MaxRiskLevel:  req.MaxRiskLevel,
		})
		if err != nil {
			return nil, err
		}
		if len(candidates) > 0 {
			selected = candidateManifests(candidates)
			traceCandidates = candidateTrace(candidates)
		}
	}
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
	wireRequiredStepInputs(steps, manifests)

	return &AgentPlan{
		Goal:            req.Message,
		Domain:          domain,
		Mode:            "dynamic_agent",
		KnowledgePolicy: defaultKnowledgePolicyForTools(req.Message, domain, p.tools.ListManifests()),
		ToolTrace:       &ToolTrace{CandidateTools: traceCandidates},
		Steps:           steps,
		Budget: AgentBudget{
			MaxToolCalls: len(steps),
			MaxSteps:     max(len(steps), p.maxTools),
			MaxReplans:   1,
			MaxCostLevel: tool.CostMedium,
		},
		StopPolicy: StopPolicy{StopWhenEnough: true},
	}, nil
}

func defaultKnowledgePolicyForTools(message, domain string, manifests []*tool.ToolManifest) *KnowledgePolicy {
	policy := DefaultKnowledgePolicy(message, domain)
	if policy == nil {
		return nil
	}
	_ = manifests
	return policy
}

func manifestListHasTool(manifests []*tool.ToolManifest, name string) bool {
	for _, manifest := range manifests {
		if manifest != nil && manifest.Name == name {
			return true
		}
	}
	return false
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
		args[k] = v
	}
	args["brief"] = req.Message
	args["topic"] = req.Message
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
			case "topic", "brief":
				if req.Message != "" {
					step.Arguments[name] = req.Message
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
