package agentruntime

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestToolRetrieverPreservesCanonicalSchemasAndProviderMetadata(t *testing.T) {
	inputSchema := map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"request": map[string]interface{}{
				"oneOf": []interface{}{
					map[string]interface{}{"$ref": "#/$defs/byID"},
					map[string]interface{}{"$ref": "#/$defs/byQuery"},
				},
			},
		},
		"$defs": map[string]interface{}{
			"byID":    map[string]interface{}{"type": "object", "properties": map[string]interface{}{"id": map[string]interface{}{"type": "string"}}},
			"byQuery": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"query": map[string]interface{}{"type": "string", "enum": []interface{}{"new", "all"}}}},
		},
	}
	outputSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"items": map[string]interface{}{"type": "array", "items": map[string]interface{}{"$ref": "#/$defs/item"}},
		},
		"$defs": map[string]interface{}{"item": map[string]interface{}{"type": "object"}},
	}
	manifest := &tool.ToolManifest{
		Name:                 "mcp_search",
		Description:          "Search provider records",
		Type:                 "mcp",
		Capabilities:         []string{"general", "search"},
		Tags:                 []string{"search"},
		InputSchema:          inputSchema,
		OutputSchema:         outputSchema,
		Provider:             "records-provider",
		ProviderCapabilities: map[string]interface{}{"supportsProgress": true, "protocolVersion": "2025-11-25"},
		CostLevel:            tool.CostLow,
		RiskLevel:            tool.RiskLow,
	}

	candidates, err := NewHybridToolRetriever([]*tool.ToolManifest{manifest}).Retrieve(context.Background(), ToolRetrieveRequest{
		UserInput:     "search records",
		Domain:        "general",
		MaxCandidates: 1,
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(candidates))
	}
	candidate := candidates[0]
	if !reflect.DeepEqual(candidate.InputSchema, inputSchema) || !reflect.DeepEqual(candidate.OutputSchema, outputSchema) {
		t.Fatalf("canonical schemas were changed: input=%#v output=%#v", candidate.InputSchema, candidate.OutputSchema)
	}
	if !reflect.DeepEqual(candidate.ProviderCapabilities, manifest.ProviderCapabilities) {
		t.Fatalf("provider metadata was not copied: %#v", candidate.ProviderCapabilities)
	}

	// Candidate data is safe for prompt construction and cannot mutate the registry manifest.
	candidate.InputSchema["type"] = "mutated"
	candidate.ProviderCapabilities["supportsProgress"] = false
	if inputSchema["type"] != "object" || manifest.ProviderCapabilities["supportsProgress"] != true {
		t.Fatalf("candidate aliases manifest data: schema=%#v provider=%#v", inputSchema, manifest.ProviderCapabilities)
	}
}

func TestToolRetrieverDerivesClosedCanonicalSchemaFromLegacyParamDefs(t *testing.T) {
	manifest := &tool.ToolManifest{
		Name:         "legacy_lookup",
		Description:  "Look up a record",
		Capabilities: []string{"general", "search"},
		Tags:         []string{"lookup"},
		Parameters: map[string]tool.ParamDef{
			"query": {Type: "string", Description: "search query", Required: true, Enum: []string{"all", "new"}},
			"limit": {Type: "number", Default: 10},
		},
		Output: map[string]tool.ParamDef{
			"items": {Type: "array", Required: true},
		},
		CostLevel: tool.CostLow,
		RiskLevel: tool.RiskLow,
	}

	candidates, err := NewHybridToolRetriever([]*tool.ToolManifest{manifest}).Retrieve(context.Background(), ToolRetrieveRequest{
		UserInput:     "lookup records",
		Domain:        "general",
		MaxCandidates: 1,
	})
	if err != nil || len(candidates) != 1 {
		t.Fatalf("Retrieve = (%#v, %v), want one candidate", candidates, err)
	}
	candidate := candidates[0]
	if candidate.InputSchema["type"] != "object" || candidate.InputSchema["additionalProperties"] != false {
		t.Fatalf("legacy input must become a closed object schema: %#v", candidate.InputSchema)
	}
	if got := candidate.InputSchema["required"]; !reflect.DeepEqual(got, []string{"query"}) {
		t.Fatalf("required = %#v, want [query]", got)
	}
	properties, _ := candidate.InputSchema["properties"].(map[string]interface{})
	query, _ := properties["query"].(map[string]interface{})
	if _, exists := query["required"]; exists {
		t.Fatalf("legacy ParamDef.required must be projected to root required, not a property keyword: %#v", query)
	}
	if !reflect.DeepEqual(candidate.LegacyParameters, manifest.Parameters) || !reflect.DeepEqual(candidate.LegacyOutput, manifest.Output) {
		t.Fatalf("legacy projections not retained: %#v", candidate)
	}
}

func TestToolRetrieverRanksFreshKnowledgeToolForCurrentEvent(t *testing.T) {
	retriever := NewHybridToolRetriever([]*tool.ToolManifest{
		{
			Name:         "custom_news_search",
			Description:  "Search latest news and current event facts for script generation.",
			Type:         "http",
			Capabilities: []string{"fresh_knowledge", "news_search", "web_search"},
			Tags:         []string{"news", "search", "fact"},
			Parameters: map[string]tool.ParamDef{
				"query": {Type: "string", Required: true},
				"topK":  {Type: "number"},
			},
			Output: map[string]tool.ParamDef{
				"facts":   {Type: "array"},
				"sources": {Type: "array"},
			},
			CostLevel:  tool.CostLow,
			RiskLevel:  tool.RiskLow,
			SideEffect: false,
		},
		{
			Name:         "image_generator",
			Description:  "Generate images for video covers.",
			Type:         "http",
			Capabilities: []string{"video_creation", "image_generation"},
			Tags:         []string{"image"},
			CostLevel:    tool.CostMedium,
			RiskLevel:    tool.RiskLow,
			SideEffect:   false,
		},
	})

	candidates, err := retriever.Retrieve(context.Background(), ToolRetrieveRequest{
		UserInput:     "请帮我做一个30秒视频，讲佛得角国家以及佛得角世界杯出线是一个奇迹。",
		Domain:        "video_creation",
		Stage:         "planning",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected fresh knowledge candidate")
	}
	if candidates[0].Name != "custom_news_search" {
		t.Fatalf("top candidate = %q, want custom_news_search: %#v", candidates[0].Name, candidates)
	}
	if candidates[0].Score <= 0 {
		t.Fatalf("candidate score should be positive: %#v", candidates[0])
	}
	if !strings.Contains(candidates[0].Reason, "fresh_knowledge") {
		t.Fatalf("candidate reason should explain fresh knowledge match: %q", candidates[0].Reason)
	}
}

func TestToolRetrieverDoesNotPromoteFreshKnowledgeForOpinionTask(t *testing.T) {
	retriever := NewHybridToolRetriever([]*tool.ToolManifest{
		{
			Name:         "custom_news_search",
			Description:  "Search latest news and current event facts.",
			Type:         "http",
			Capabilities: []string{"fresh_knowledge", "news_search", "web_search"},
			Tags:         []string{"news", "search", "fact"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskLow,
		},
		{
			Name:         "video_script_generator",
			Description:  "Generate video voiceover scripts.",
			Type:         "builtin_prompt_tool",
			Capabilities: []string{"video_creation", "script_generation"},
			Tags:         []string{"script", "video"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskLow,
		},
	})

	candidates, err := retriever.Retrieve(context.Background(), ToolRetrieveRequest{
		UserInput:     "请帮我做一个30秒视频，讲 AI 替代的不是岗位，而是工作流程。",
		Domain:        "video_creation",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected at least script candidate")
	}
	if candidates[0].Name == "custom_news_search" {
		t.Fatalf("fresh knowledge tool should not be top candidate for opinion task: %#v", candidates)
	}
}

func TestToolRetrieverFiltersFreshKnowledgeWhenPolicyForbidsIt(t *testing.T) {
	retriever := NewHybridToolRetriever(retrieverPolicyTestTools())

	candidates, err := retriever.Retrieve(context.Background(), ToolRetrieveRequest{
		UserInput: "帮我写一个赛博朋克虚构短片",
		Domain:    "video_creation",
		KnowledgePolicy: &KnowledgePolicy{
			ContentType:           "fiction",
			FreshnessLevel:        FreshnessNone,
			RetrievalPolicy:       RetrievalNone,
			ForbiddenCapabilities: FreshKnowledgeCapabilities(),
			Reason:                "fiction and creative writing do not need current facts",
		},
		MaxCandidates: 10,
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	for _, candidate := range candidates {
		if candidate.Name == "custom_news_search" {
			t.Fatalf("fresh knowledge tool should be filtered when policy forbids it: %#v", candidates)
		}
	}
	if len(candidates) == 0 || candidates[0].Name != "video_script_generator" {
		t.Fatalf("expected script generator candidate, got %#v", candidates)
	}
}

func TestToolRetrieverSelectsFreshKnowledgeWhenPolicyRequiresIt(t *testing.T) {
	retriever := NewHybridToolRetriever(retrieverPolicyTestTools())

	candidates, err := retriever.Retrieve(context.Background(), ToolRetrieveRequest{
		UserInput: "帮我做今天 AI 新闻短视频",
		Domain:    "video_creation",
		KnowledgePolicy: &KnowledgePolicy{
			ContentType:          "news_video",
			FreshnessLevel:       FreshnessHigh,
			RetrievalPolicy:      RetrievalRequired,
			RequiredCapabilities: FreshKnowledgeCapabilities(),
			Reason:               "user asked for today AI news",
		},
		MaxCandidates: 10,
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if len(candidates) == 0 || candidates[0].Name != "custom_news_search" {
		t.Fatalf("expected fresh knowledge candidate first, got %#v", candidates)
	}
	if !strings.Contains(candidates[0].Reason, "knowledge policy") {
		t.Fatalf("candidate reason should include knowledge policy reason, got %q", candidates[0].Reason)
	}
}

func TestToolRetrieverFiltersExternalAPIWhenUserDisallowsWeb(t *testing.T) {
	retriever := NewHybridToolRetriever(retrieverPolicyTestTools())

	candidates, err := retriever.Retrieve(context.Background(), ToolRetrieveRequest{
		UserInput: "不要联网，帮我写一个产品宣传视频脚本",
		Domain:    "video_creation",
		KnowledgePolicy: &KnowledgePolicy{
			ContentType:           "marketing_script",
			FreshnessLevel:        FreshnessNone,
			RetrievalPolicy:       RetrievalNone,
			ForbiddenCapabilities: append(FreshKnowledgeCapabilities(), "external_api"),
			Reason:                "user explicitly disabled web access",
		},
		MaxCandidates: 10,
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	for _, candidate := range candidates {
		if candidate.Name == "external_research_api" || candidate.Name == "custom_news_search" {
			t.Fatalf("external/fresh tool should be filtered by no-web policy: %#v", candidates)
		}
	}
}

func TestToolRetrieverStronglyDownranksWhenNotToUseMatch(t *testing.T) {
	retriever := NewHybridToolRetriever(retrieverPolicyTestTools())

	candidates, err := retriever.Retrieve(context.Background(), ToolRetrieveRequest{
		UserInput: "帮我改写和润色这段已有文案，不要查资料",
		Domain:    "video_creation",
		KnowledgePolicy: &KnowledgePolicy{
			ContentType:           "rewrite",
			FreshnessLevel:        FreshnessNone,
			RetrievalPolicy:       RetrievalNone,
			ForbiddenCapabilities: FreshKnowledgeCapabilities(),
			Reason:                "rewrite task with no-web instruction",
		},
		MaxCandidates: 10,
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	for _, candidate := range candidates {
		if candidate.Name == "custom_news_search" {
			t.Fatalf("whenNotToUse/no-web match should filter news search: %#v", candidates)
		}
	}
}

func retrieverPolicyTestTools() []*tool.ToolManifest {
	return []*tool.ToolManifest{
		{
			Name:         "custom_news_search",
			Description:  "Search latest news and current event facts.",
			Type:         "http",
			Boundary:     tool.BoundaryRemoteHTTP,
			Capabilities: []string{"fresh_knowledge", "news_search", "web_search", "current_event_retrieval"},
			Tags:         []string{"news", "search", "fact"},
			WhenToUse:    []string{"latest news", "今天 新闻", "最近 发生"},
			WhenNotToUse: []string{"虚构", "改写", "润色", "不要查资料", "不要联网"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskLow,
			Parameters: map[string]tool.ParamDef{
				"query": {Type: "string", Required: true},
			},
		},
		{
			Name:         "external_research_api",
			Description:  "Call external APIs for factual research.",
			Type:         "http",
			Boundary:     tool.BoundaryRemoteHTTP,
			Capabilities: []string{"external_api", "fact_retrieval"},
			Tags:         []string{"external", "api"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskMedium,
		},
		{
			Name:         "video_script_generator",
			Description:  "Generate video scripts and rewrite creative briefs.",
			Type:         "builtin_prompt_tool",
			Boundary:     tool.BoundaryCloudBuiltin,
			Capabilities: []string{"video_creation", "script_generation", "creative_writing"},
			Tags:         []string{"script", "video", "rewrite"},
			WhenToUse:    []string{"虚构", "改写", "润色", "产品宣传"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskLow,
		},
	}
}
