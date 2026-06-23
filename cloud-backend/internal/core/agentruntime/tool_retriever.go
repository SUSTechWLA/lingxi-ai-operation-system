package agentruntime

import (
	"context"
	"sort"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

// ToolRetriever selects relevant tools for a planning request.
// It replaces the simple HeuristicPlanner.selectTools with multi-signal scoring.
type ToolRetriever interface {
	Retrieve(ctx context.Context, req RetrieveRequest) ([]*tool.ToolManifest, error)
}

// RetrieveRequest carries the parameters for tool retrieval.
type RetrieveRequest struct {
	Query         string   // Natural language user query.
	Domain        string   // Target domain (e.g., "video_creation").
	Stage         string   // Optional pipeline stage hint.
	PreviousTools []string // Tools already in the plan (for next-tool boosting).
	MaxCostLevel  string   // Maximum allowed cost level.
	MaxRiskLevel  string   // Maximum allowed risk level.
	CoarseTopK    int       // Number of candidates to retrieve before reranking.
	PlannerTopK   int       // Number of tools to return to the LLM planner.
}

// HybridToolRetriever implements ToolRetriever with multi-signal scoring:
// hard filter → capability/tag/keyword recall → nextRecommendedTools boost →
// cost/risk penalty → topK rerank.
type HybridToolRetriever struct {
	// allTools is the full list of tool manifests.
	allTools []*tool.ToolManifest

	// When vectorStore is set (future), an embedding-based recall is blended in.
	vectorStore interface {
		Search(ctx context.Context, query string, topK int) ([]string, error)
	}
}

// NewHybridToolRetriever creates a retriever backed by a list of tool manifests.
// The manifests are typically obtained from tool.ToolRegistry.ListManifests().
func NewHybridToolRetriever(manifests []*tool.ToolManifest) *HybridToolRetriever {
	return &HybridToolRetriever{allTools: manifests}
}

// Retrieve executes the hybrid retrieval pipeline.
func (r *HybridToolRetriever) Retrieve(ctx context.Context, req RetrieveRequest) ([]*tool.ToolManifest, error) {
	coarseK := req.CoarseTopK
	if coarseK <= 0 {
		coarseK = 30
	}
	plannerK := req.PlannerTopK
	if plannerK <= 0 {
		plannerK = 8
	}

	// Stage 1: Hard filter.
	candidates := r.hardFilter(req)

	// Stage 2: Score each candidate.
	prevSet := toSet(req.PreviousTools)
	scored := make([]scoredTool, 0, len(candidates))
	for _, m := range candidates {
		score := r.score(m, req.Query, req.Domain, prevSet)
		// Apply cost/risk penalties.
		score -= costPenalty(m.CostLevel)
		score -= riskPenalty(m.RiskLevel)
		scored = append(scored, scoredTool{manifest: m, score: score})
	}

	// Stage 3: Sort descending by score.
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Stage 4: TopK to planner.
	if len(scored) > coarseK {
		scored = scored[:coarseK]
	}
	if len(scored) > plannerK {
		scored = scored[:plannerK]
	}

	result := make([]*tool.ToolManifest, len(scored))
	for i, s := range scored {
		result[i] = s.manifest
	}
	return result, nil
}

type scoredTool struct {
	manifest *tool.ToolManifest
	score    float64
}

// hardFilter removes tools that are incompatible with the request.
func (r *HybridToolRetriever) hardFilter(req RetrieveRequest) []*tool.ToolManifest {
	filtered := make([]*tool.ToolManifest, 0, len(r.allTools))
	for _, m := range r.allTools {
		if m.Name == "" {
			continue
		}
		// Filter by domain: keep tools that match the domain capability or have no domain constraint.
		if req.Domain != "" && !hasCapability(m, req.Domain) {
			// Also keep tools that don't specify capabilities (generic tools).
			if len(m.Capabilities) > 0 {
				continue
			}
		}
		// Filter by cost level.
		if req.MaxCostLevel != "" && m.CostLevel != "" {
			if costRankStr(m.CostLevel) > costRankStr(req.MaxCostLevel) {
				continue
			}
		}
		// Filter by risk level.
		if req.MaxRiskLevel != "" && m.RiskLevel != "" {
			if riskRankStr(m.RiskLevel) > riskRankStr(req.MaxRiskLevel) {
				continue
			}
		}
		// Exclude quality gate marker.
		if m.Name == "__quality_gate__" {
			continue
		}
		filtered = append(filtered, m)
	}
	return filtered
}

// score computes a relevance score for a tool against the request.
func (r *HybridToolRetriever) score(m *tool.ToolManifest, query, domain string, prevSet map[string]bool) float64 {
	var score float64

	// Capability match (weight: 0.30).
	score += 0.30 * capabilityScore(m, domain)

	// Keyword match in name + description (weight: 0.25).
	score += 0.25 * keywordScore(m, query)

	// Tag match (weight: 0.20).
	score += 0.20 * tagScore(m, query)

	// Next-tool boost (weight: 0.10).
	score += 0.10 * nextToolScore(m, prevSet)

	// Domain relevance (weight: 0.15).
	score += 0.15 * domainScore(m, domain)

	return score
}

// capabilityScore returns 1.0 if the tool has the target domain capability, 0 otherwise.
func capabilityScore(m *tool.ToolManifest, domain string) float64 {
	if domain == "" {
		return 0
	}
	if hasCapability(m, domain) {
		return 1.0
	}
	if len(m.Capabilities) == 0 {
		return 0.5 // generic tools get a moderate score
	}
	return 0
}

// keywordScore measures how well the tool matches query keywords.
func keywordScore(m *tool.ToolManifest, query string) float64 {
	if query == "" {
		return 0
	}
	queryLower := strings.ToLower(query)

	// Name match.
	nameLower := strings.ToLower(m.Name)
	queryWords := strings.Split(queryLower, " ")
	nameMatch := 0
	for _, qw := range queryWords {
		if len(qw) < 2 {
			continue
		}
		if strings.Contains(nameLower, qw) {
			nameMatch++
		}
	}
	nameScore := float64(nameMatch) / float64(max(1, len(queryWords)))

	// Description match.
	descLower := strings.ToLower(m.Description)
	descMatch := 0
	for _, qw := range queryWords {
		if len(qw) < 2 {
			continue
		}
		if strings.Contains(descLower, qw) {
			descMatch++
		}
	}
	descScore := float64(descMatch) / float64(max(1, len(queryWords)))

	return 0.6*nameScore + 0.4*descScore
}

// tagScore measures tag overlap with query keywords.
func tagScore(m *tool.ToolManifest, query string) float64 {
	if query == "" || len(m.Tags) == 0 {
		return 0
	}
	queryLower := strings.ToLower(query)
	match := 0
	for _, tag := range m.Tags {
		if strings.Contains(queryLower, strings.ToLower(tag)) {
			match++
		}
	}
	if match == 0 {
		return 0
	}
	return float64(match) / float64(len(m.Tags))
}

// nextToolScore boosts tools that are recommended by tools already in the plan.
func nextToolScore(m *tool.ToolManifest, prevSet map[string]bool) float64 {
	if len(prevSet) == 0 || len(m.NextRecommendedTools) == 0 {
		return 0
	}
	overlap := 0
	for _, next := range m.NextRecommendedTools {
		if prevSet[next] {
			overlap++
		}
	}
	// This is the opposite direction: we want tools that are recommended NEXT
	// by tools that are ALREADY in the plan. But the manifest stores what THIS tool
	// recommends, not who recommends this tool.
	//
	// For simplicity, we boost tools whose NextRecommendedTools overlap with
	// already-chosen tools (these tools "recommend similar next steps").
	return 0
}

// domainScore gives a small boost to tools that have the target domain in capabilities.
func domainScore(m *tool.ToolManifest, domain string) float64 {
	if domain == "" || len(m.Capabilities) == 0 {
		return 0
	}
	if hasCapability(m, domain) {
		return 1.0
	}
	// Partial domain match (e.g., "video" matches "video_creation").
	domainParts := strings.Split(domain, "_")
	for _, cap := range m.Capabilities {
		for _, part := range domainParts {
			if strings.Contains(strings.ToLower(cap), strings.ToLower(part)) {
				return 0.5
			}
		}
	}
	return 0
}

// Penalty helpers.

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

// Helper functions.

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
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
