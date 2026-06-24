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
	if len(selected) == 0 {
		return nil, fmt.Errorf("no tools matched domain %q", domain)
	}

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

	return &AgentPlan{
		Goal:   req.Message,
		Domain: domain,
		Mode:   "dynamic_agent",
		Steps:  steps,
		Budget: AgentBudget{
			MaxToolCalls: len(steps),
			MaxSteps:     max(len(steps), p.maxTools),
			MaxReplans:   1,
			MaxCostLevel: tool.CostMedium,
		},
		StopPolicy: StopPolicy{StopWhenEnough: true},
	}, nil
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
