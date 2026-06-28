package agentruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

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
