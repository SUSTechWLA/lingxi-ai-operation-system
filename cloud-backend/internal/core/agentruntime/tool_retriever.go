package agentruntime

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

// ToolRetriever selects relevant tool candidates for a planning request.
type ToolRetriever interface {
	Retrieve(ctx context.Context, req ToolRetrieveRequest) ([]ToolCandidate, error)
}

type ToolRetrieveRequest struct {
	UserInput       string
	Domain          string
	Stage           string
	MaxCandidates   int
	KnowledgePolicy *KnowledgePolicy

	// Compatibility fields used by existing planner call sites.
	Query               string
	PreviousTools       []string
	MaxCostLevel        string
	MaxRiskLevel        string
	CoarseTopK          int
	PlannerTopK         int
	IncludeCapabilities []string
	ExcludeCapabilities []string
}

type RetrieveRequest = ToolRetrieveRequest

type ToolCandidate struct {
	Name                 string                   `json:"name"`
	Description          string                   `json:"description,omitempty"`
	Capabilities         []string                 `json:"capabilities,omitempty"`
	Tags                 []string                 `json:"tags,omitempty"`
	Reason               string                   `json:"reason"`
	Score                float64                  `json:"score"`
	InputSchema          map[string]interface{}   `json:"inputSchema"`
	OutputSchema         map[string]interface{}   `json:"outputSchema,omitempty"`
	LegacyParameters     map[string]tool.ParamDef `json:"parameters,omitempty"`
	LegacyOutput         map[string]tool.ParamDef `json:"output,omitempty"`
	ProviderCapabilities map[string]interface{}   `json:"providerCapabilities,omitempty"`
	CostLevel            string                   `json:"costLevel,omitempty"`
	RiskLevel            string                   `json:"riskLevel,omitempty"`
	Type                 string                   `json:"type,omitempty"`
	Trace                ToolCandidateTrace       `json:"trace,omitempty"`
	Manifest             *tool.ToolManifest       `json:"-"`
}

type HybridToolRetriever struct {
	allTools []*tool.ToolManifest
}

func NewHybridToolRetriever(manifests []*tool.ToolManifest) *HybridToolRetriever {
	return &HybridToolRetriever{allTools: manifests}
}

func (r *HybridToolRetriever) Retrieve(ctx context.Context, req ToolRetrieveRequest) ([]ToolCandidate, error) {
	_ = ctx
	query := strings.TrimSpace(req.UserInput)
	if query == "" {
		query = strings.TrimSpace(req.Query)
	}
	maxCandidates := req.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = req.PlannerTopK
	}
	if maxCandidates <= 0 {
		maxCandidates = 8
	}
	coarseK := req.CoarseTopK
	if coarseK <= 0 {
		coarseK = max(30, maxCandidates)
	}

	policy := req.KnowledgePolicy
	if policy == nil {
		policy = DefaultKnowledgePolicy(query, req.Domain)
	}
	prevSet := toSet(req.PreviousTools)
	candidates := make([]ToolCandidate, 0, len(r.allTools))
	for _, manifest := range r.hardFilter(req, query, policy) {
		score, reasonParts, trace := r.score(manifest, query, req.Domain, policy, prevSet)
		costPenaltyValue := costPenalty(manifest.CostLevel)
		riskPenaltyValue := riskPenalty(manifest.RiskLevel)
		score -= costPenaltyValue
		score -= riskPenaltyValue
		trace.CostRiskPenalty = costPenaltyValue + riskPenaltyValue
		if manifest.SideEffect {
			score -= 0.4
			trace.CostRiskPenalty += 0.4
			reasonParts = append(reasonParts, "sideEffect=true penalty")
		} else {
			score += 0.05
		}
		if score <= 0 {
			continue
		}
		candidates = append(candidates, ToolCandidate{
			Name:                 manifest.Name,
			Description:          manifest.Description,
			Capabilities:         append([]string(nil), manifest.Capabilities...),
			Tags:                 append([]string(nil), manifest.Tags...),
			Reason:               strings.Join(reasonParts, "; "),
			Score:                score,
			InputSchema:          canonicalToolSchema(manifest.InputSchema, manifest.Parameters),
			OutputSchema:         canonicalToolSchema(manifest.OutputSchema, manifest.Output),
			LegacyParameters:     cloneLegacyParamDefs(manifest.Parameters),
			LegacyOutput:         cloneLegacyParamDefs(manifest.Output),
			ProviderCapabilities: cloneJSONMap(manifest.ProviderCapabilities),
			CostLevel:            manifest.CostLevel,
			RiskLevel:            manifest.RiskLevel,
			Type:                 manifest.Type,
			Trace:                trace,
			Manifest:             manifest,
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Name < candidates[j].Name
		}
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > coarseK {
		candidates = candidates[:coarseK]
	}
	if len(candidates) > maxCandidates {
		candidates = candidates[:maxCandidates]
	}
	return candidates, nil
}

func (r *HybridToolRetriever) hardFilter(req ToolRetrieveRequest, query string, policy *KnowledgePolicy) []*tool.ToolManifest {
	filtered := make([]*tool.ToolManifest, 0, len(r.allTools))
	freshAllowed := knowledgePolicyAllowsFreshTools(policy)
	freshRequired := policy != nil && policy.RetrievalPolicy == RetrievalRequired
	for _, manifest := range r.allTools {
		if manifest == nil || manifest.Name == "" || manifest.Name == "__quality_gate__" {
			continue
		}
		if plannerDisallowsToolForDomain(manifest.Name, req.Domain) {
			continue
		}
		if req.MaxCostLevel != "" && manifest.CostLevel != "" && costRankStr(manifest.CostLevel) > costRankStr(req.MaxCostLevel) {
			continue
		}
		if req.MaxRiskLevel != "" && manifest.RiskLevel != "" && riskRankStr(manifest.RiskLevel) > riskRankStr(req.MaxRiskLevel) {
			continue
		}
		if capabilityOverlaps(manifest, req.ExcludeCapabilities) {
			continue
		}
		if toolForbiddenByKnowledgePolicy(manifest, policy) {
			continue
		}
		if hasFreshKnowledgeCapability(manifest) && !freshAllowed {
			continue
		}
		if len(matchUsageHints(query, manifest.WhenNotToUse)) > 0 {
			continue
		}
		if req.Domain != "" && req.Domain != "general" && len(manifest.Capabilities) == 0 {
			continue
		}
		if len(req.IncludeCapabilities) > 0 && !capabilityOverlaps(manifest, req.IncludeCapabilities) {
			if !(freshRequired && hasFreshKnowledgeCapability(manifest)) {
				continue
			}
		}
		if req.Domain != "" && len(manifest.Capabilities) > 0 &&
			!hasCapability(manifest, req.Domain) &&
			!(freshRequired && hasFreshKnowledgeCapability(manifest)) &&
			!isContentGenerationTool(manifest.Name, manifest) {
			continue
		}
		if !toolCanAcceptRequest(manifest, query, freshRequired) {
			continue
		}
		filtered = append(filtered, manifest)
	}
	return filtered
}

func plannerDisallowsToolForDomain(toolName, domain string) bool {
	if domain != "video_creation" {
		return false
	}
	// Quality checkers are structural companions to producer tools. The plan
	// compiler inserts them immediately after the matching producer and wires
	// the quality gate. Selecting one as a standalone heuristic step lets later
	// profile canonicalization move its producer while leaving an invalid
	// forward output reference behind.
	if isQualityCheckerTool(toolName) {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "bash", "python", "llm_api", "external", "video_frame_qa":
		return true
	default:
		return false
	}
}

func (r *HybridToolRetriever) score(m *tool.ToolManifest, query, domain string, policy *KnowledgePolicy, prevSet map[string]bool) (float64, []string, ToolCandidateTrace) {
	var score float64
	reasons := make([]string, 0, 5)
	trace := ToolCandidateTrace{Name: m.Name}
	freshRequired := policy != nil && policy.RetrievalPolicy == RetrievalRequired

	if domain != "" && hasCapability(m, domain) {
		score += 0.8
		trace.MatchedCapabilities = append(trace.MatchedCapabilities, domain)
		reasons = append(reasons, "matched domain "+domain)
	}
	if freshRequired && hasFreshKnowledgeCapability(m) {
		score += 1.4
		trace.KnowledgePolicyReason = policy.Reason
		trace.MatchedCapabilities = append(trace.MatchedCapabilities, intersectCapabilities(m.Capabilities, FreshKnowledgeCapabilities())...)
		reasons = append(reasons, "matched fresh_knowledge by knowledge policy: "+policy.Reason)
	}
	if policy != nil && policy.RetrievalPolicy == RetrievalOptional && hasFreshKnowledgeCapability(m) {
		score += 0.45
		trace.KnowledgePolicyReason = policy.Reason
		reasons = append(reasons, "fresh knowledge optional by knowledge policy: "+policy.Reason)
	}
	if policy == nil && !freshRequired && hasFreshKnowledgeCapability(m) {
		score -= 0.8
		reasons = append(reasons, "fresh knowledge not required")
	}
	if isContentGenerationTool(m.Name, m) {
		score += 0.5
		reasons = append(reasons, "matched content generation stage")
	}
	if kw := keywordScore(m, query); kw > 0 {
		score += kw
		reasons = append(reasons, fmt.Sprintf("matched request text %.2f", kw))
	}
	if ts := tagScore(m, query); ts > 0 {
		score += ts
		trace.MatchedTags = matchedTags(m.Tags, query)
		reasons = append(reasons, fmt.Sprintf("matched tags %.2f", ts))
	}
	if hints := matchUsageHints(query, m.WhenToUse); len(hints) > 0 {
		score += 0.35 * float64(len(hints))
		trace.MatchedWhenToUse = hints
		reasons = append(reasons, "matched whenToUse "+strings.Join(hints, ","))
	}
	if ns := nextToolScore(m, prevSet); ns > 0 {
		score += ns
		reasons = append(reasons, "matched recommended tool chain")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "low-confidence manifest match")
	}
	trace.Score = score
	trace.Reason = strings.Join(reasons, "; ")
	return score, reasons, trace
}

func toolCanAcceptRequest(manifest *tool.ToolManifest, query string, freshRequired bool) bool {
	if manifest == nil {
		return false
	}
	if !freshRequired || !hasFreshKnowledgeCapability(manifest) {
		return true
	}
	if len(manifest.Parameters) == 0 {
		return true
	}
	for name, param := range manifest.Parameters {
		if !param.Required {
			continue
		}
		lower := strings.ToLower(name)
		if strings.Contains(lower, "query") || strings.Contains(lower, "queries") ||
			strings.Contains(lower, "q") || strings.Contains(lower, "keyword") ||
			strings.Contains(lower, "topic") {
			continue
		}
		return false
	}
	return strings.TrimSpace(query) != ""
}

func candidateManifests(candidates []ToolCandidate) []*tool.ToolManifest {
	manifests := make([]*tool.ToolManifest, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Manifest != nil {
			manifests = append(manifests, candidate.Manifest)
		}
	}
	return manifests
}

func candidateTrace(candidates []ToolCandidate) []ToolCandidateTrace {
	trace := make([]ToolCandidateTrace, 0, len(candidates))
	for _, candidate := range candidates {
		item := candidate.Trace
		item.Name = candidate.Name
		item.Score = candidate.Score
		item.Reason = candidate.Reason
		trace = append(trace, item)
	}
	return trace
}

func hasFreshKnowledgeCapability(manifest *tool.ToolManifest) bool {
	return capabilityOverlaps(manifest, FreshKnowledgeCapabilities())
}

func capabilityOverlaps(manifest *tool.ToolManifest, capabilities []string) bool {
	if manifest == nil || len(capabilities) == 0 {
		return false
	}
	allowed := toSetLower(capabilities)
	for _, capability := range manifest.Capabilities {
		if allowed[strings.ToLower(strings.TrimSpace(capability))] {
			return true
		}
	}
	return false
}

func knowledgePolicyAllowsFreshTools(policy *KnowledgePolicy) bool {
	if policy == nil {
		return false
	}
	return policy.RetrievalPolicy == RetrievalRequired || policy.RetrievalPolicy == RetrievalOptional
}

func toolForbiddenByKnowledgePolicy(manifest *tool.ToolManifest, policy *KnowledgePolicy) bool {
	if manifest == nil || policy == nil {
		return false
	}
	if capabilityOverlaps(manifest, policy.ForbiddenCapabilities) {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(manifest.Name))
	for _, forbidden := range policy.ForbiddenTools {
		forbidden = strings.ToLower(strings.TrimSpace(forbidden))
		if forbidden == "" {
			continue
		}
		if name == forbidden || strings.Contains(name, forbidden) {
			return true
		}
	}
	return false
}

func intersectCapabilities(have, want []string) []string {
	wantSet := toSetLower(want)
	out := make([]string, 0)
	seen := map[string]bool{}
	for _, capability := range have {
		key := strings.ToLower(strings.TrimSpace(capability))
		if key == "" || !wantSet[key] || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, capability)
	}
	return out
}

func matchedTags(tags []string, query string) []string {
	queryLower := strings.ToLower(query)
	out := make([]string, 0)
	for _, tag := range tags {
		if tag = strings.TrimSpace(tag); tag != "" && strings.Contains(queryLower, strings.ToLower(tag)) {
			out = append(out, tag)
		}
	}
	return out
}

func matchUsageHints(query string, hints []string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || len(hints) == 0 {
		return nil
	}
	matches := make([]string, 0)
	for _, hint := range hints {
		hint = strings.TrimSpace(hint)
		if hint == "" {
			continue
		}
		parts := strings.Fields(strings.ToLower(hint))
		if len(parts) == 0 {
			continue
		}
		allMatched := true
		for _, part := range parts {
			if !strings.Contains(query, part) {
				allMatched = false
				break
			}
		}
		if allMatched {
			matches = append(matches, hint)
		}
	}
	return matches
}

func toSetLower(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		if trimmed := strings.ToLower(strings.TrimSpace(item)); trimmed != "" {
			out[trimmed] = true
		}
	}
	return out
}

func keywordScore(m *tool.ToolManifest, query string) float64 {
	if query == "" {
		return 0
	}
	haystack := strings.ToLower(strings.Join(append(append([]string{m.Name, m.Description}, m.Capabilities...), m.Tags...), " "))
	var score float64
	for _, token := range tokenize(query) {
		if len(token) < 2 {
			continue
		}
		if strings.Contains(haystack, token) {
			score += 0.12
		}
	}
	return score
}

func tagScore(m *tool.ToolManifest, query string) float64 {
	if query == "" || len(m.Tags) == 0 {
		return 0
	}
	queryLower := strings.ToLower(query)
	matches := 0
	for _, tag := range m.Tags {
		if strings.Contains(queryLower, strings.ToLower(tag)) {
			matches++
		}
	}
	if matches == 0 {
		return 0
	}
	return 0.2 * float64(matches) / float64(len(m.Tags))
}

func nextToolScore(m *tool.ToolManifest, prevSet map[string]bool) float64 {
	if len(prevSet) == 0 {
		return 0
	}
	for _, next := range m.NextRecommendedTools {
		if prevSet[next] {
			return 0.1
		}
	}
	return 0
}

func toSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[item] = true
	}
	return out
}

func costPenalty(level string) float64 {
	switch level {
	case tool.CostHigh:
		return 0.3
	case tool.CostMedium:
		return 0.1
	default:
		return 0
	}
}

func riskPenalty(level string) float64 {
	switch level {
	case tool.RiskHigh:
		return 0.3
	case tool.RiskMedium:
		return 0.1
	default:
		return 0
	}
}

func costRankStr(level string) int {
	switch level {
	case tool.CostHigh:
		return 3
	case tool.CostMedium:
		return 2
	default:
		return 1
	}
}

func riskRankStr(level string) int {
	switch level {
	case tool.RiskHigh:
		return 3
	case tool.RiskMedium:
		return 2
	default:
		return 1
	}
}
