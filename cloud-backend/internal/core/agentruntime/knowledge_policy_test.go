package agentruntime

import (
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestDefaultKnowledgePolicyRequiresRetrievalForCurrentEventVideo(t *testing.T) {
	policy := DefaultKnowledgePolicy("请帮我做一个30秒视频，讲佛得角国家以及佛得角世界杯出线是一个奇迹", "video_creation")
	if policy == nil {
		t.Fatal("expected a knowledge policy")
	}
	if policy.RetrievalPolicy != RetrievalRequired {
		t.Fatalf("retrieval policy = %q, want %q", policy.RetrievalPolicy, RetrievalRequired)
	}
	if policy.FreshnessLevel != FreshnessHigh {
		t.Fatalf("freshness level = %q, want %q", policy.FreshnessLevel, FreshnessHigh)
	}
	if !policy.BlockOnEmptyFacts || !policy.MustUseFacts {
		t.Fatalf("current event policy should block empty facts and require facts: %#v", policy)
	}
	if len(policy.SearchQueries) == 0 {
		t.Fatalf("current event policy should include search queries: %#v", policy)
	}
}

func TestDefaultKnowledgePolicyDoesNotSearchOpinionVideo(t *testing.T) {
	policy := DefaultKnowledgePolicy("请帮我做一个30秒视频，讲 AI 替代的不是岗位，而是工作流程", "video_creation")
	if policy == nil {
		t.Fatal("expected a knowledge policy")
	}
	if policy.RetrievalPolicy != RetrievalNone {
		t.Fatalf("retrieval policy = %q, want %q", policy.RetrievalPolicy, RetrievalNone)
	}
	if containsString(policy.ForbiddenCapabilities, "fresh_knowledge") == false {
		t.Fatalf("opinion policy should forbid fresh knowledge capabilities: %#v", policy)
	}
}

func TestDefaultKnowledgePolicyFreshKnowledgeSignals(t *testing.T) {
	tests := []struct {
		name                  string
		message               string
		wantRetrieval         RetrievalPolicy
		wantFreshForbidden    bool
		wantExternalForbidden bool
		wantMustUseFacts      bool
		wantMustCiteFacts     bool
	}{
		{
			name:               "fiction short forbids fresh knowledge",
			message:            "帮我写一个赛博朋克虚构短片",
			wantRetrieval:      RetrievalNone,
			wantFreshForbidden: true,
		},
		{
			name:                  "no web forbids external api",
			message:               "不要联网，帮我写一个产品宣传视频脚本",
			wantRetrieval:         RetrievalNone,
			wantFreshForbidden:    true,
			wantExternalForbidden: true,
		},
		{
			name:              "today AI news requires fresh knowledge",
			message:           "帮我做今天 AI 新闻短视频",
			wantRetrieval:     RetrievalRequired,
			wantMustUseFacts:  true,
			wantMustCiteFacts: true,
		},
		{
			name:              "recent company event requires fresh knowledge",
			message:           "最近某公司发生了什么，做成短视频",
			wantRetrieval:     RetrievalRequired,
			wantMustUseFacts:  true,
			wantMustCiteFacts: true,
		},
		{
			name:               "stable science does not require fresh knowledge",
			message:            "写一个关于牛顿三定律的科普视频",
			wantRetrieval:      RetrievalNone,
			wantFreshForbidden: true,
		},
		{
			name:              "latest model comparison requires fresh knowledge",
			message:           "写一个 2026 年最新 AI 视频模型对比",
			wantRetrieval:     RetrievalRequired,
			wantMustUseFacts:  true,
			wantMustCiteFacts: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := DefaultKnowledgePolicy(tt.message, "video_creation")
			if policy == nil {
				t.Fatal("expected a knowledge policy")
			}
			if policy.RetrievalPolicy != tt.wantRetrieval {
				t.Fatalf("retrieval policy = %q, want %q: %#v", policy.RetrievalPolicy, tt.wantRetrieval, policy)
			}
			if tt.wantFreshForbidden {
				for _, capability := range FreshKnowledgeCapabilities() {
					if !containsString(policy.ForbiddenCapabilities, capability) {
						t.Fatalf("policy should forbid fresh capability %q: %#v", capability, policy)
					}
				}
			}
			if tt.wantExternalForbidden && !containsString(policy.ForbiddenCapabilities, "external_api") {
				t.Fatalf("no-web policy should forbid external_api: %#v", policy)
			}
			if policy.MustUseFacts != tt.wantMustUseFacts {
				t.Fatalf("mustUseFacts = %v, want %v: %#v", policy.MustUseFacts, tt.wantMustUseFacts, policy)
			}
			if policy.MustCiteFacts != tt.wantMustCiteFacts {
				t.Fatalf("mustCiteFacts = %v, want %v: %#v", policy.MustCiteFacts, tt.wantMustCiteFacts, policy)
			}
			if strings.TrimSpace(policy.Reason) == "" {
				t.Fatalf("policy should include a traceable reason: %#v", policy)
			}
		})
	}
}

func TestPlanGuardRejectsRequiredRetrievalWithoutFreshKnowledgeTool(t *testing.T) {
	guard := NewPlanGuard(knowledgeToolCatalog(), nil)
	plan := &AgentPlan{
		Goal:            "current event video",
		Domain:          "video_creation",
		Mode:            "dynamic_agent",
		KnowledgePolicy: requiredNewsPolicy(),
		Steps: []AgentStep{
			{ID: "script", Tool: "video_script_generator", Arguments: map[string]interface{}{"topic": "佛得角世界杯出线"}},
		},
	}

	if err := guard.Validate(plan); err == nil {
		t.Fatal("expected guard to reject missing fresh knowledge tool")
	}
}

func TestPlanGuardAcceptsRegisteredFreshKnowledgeToolForRequiredRetrieval(t *testing.T) {
	guard := NewPlanGuard(knowledgeToolCatalog(), nil)
	plan := &AgentPlan{
		Goal:            "current event video",
		Domain:          "video_creation",
		Mode:            "dynamic_agent",
		KnowledgePolicy: requiredNewsPolicy(),
		Steps: []AgentStep{
			{
				ID:             "retrieve_fresh_facts",
				Intent:         "获取与用户主题相关的最新事实",
				Tool:           "custom_news_search",
				Reason:         "用户提到世界杯出线，属于强时效体育事实，需要外部事实确认",
				Arguments:      map[string]interface{}{"query": "佛得角 世界杯 出线 最新", "topK": 5},
				ExpectedOutput: []string{"facts", "sources"},
			},
			{
				ID:        "script",
				Tool:      "video_script_generator",
				DependsOn: []string{"retrieve_fresh_facts"},
				Arguments: map[string]interface{}{
					"topic": "佛得角世界杯出线",
					"knowledgeContext": map[string]interface{}{
						"items": []interface{}{"{{retrieve_fresh_facts.output.facts}}"},
					},
				},
			},
		},
	}

	if err := guard.Validate(plan); err != nil {
		t.Fatalf("expected registered fresh knowledge tool to pass guard: %v", err)
	}
}

func TestPlanGuardRejectsFreshKnowledgeIntentWithWrongCapabilityTool(t *testing.T) {
	guard := NewPlanGuard(knowledgeToolCatalog(), nil)
	plan := &AgentPlan{
		Goal:            "current event video",
		Domain:          "video_creation",
		Mode:            "dynamic_agent",
		KnowledgePolicy: requiredNewsPolicy(),
		Steps: []AgentStep{
			{
				ID:        "bad_search",
				Intent:    "搜索最新新闻事实",
				Tool:      "image_generator",
				Reason:    "错误地尝试用图片工具搜索新闻",
				Arguments: map[string]interface{}{"prompt": "佛得角 世界杯 出线"},
			},
			{ID: "script", Tool: "video_script_generator", Arguments: map[string]interface{}{"topic": "佛得角世界杯出线"}},
		},
	}

	err := guard.Validate(plan)
	if err == nil {
		t.Fatal("expected capability mismatch")
	}
	if !strings.Contains(err.Error(), "capability mismatch") {
		t.Fatalf("error should mention capability mismatch, got %v", err)
	}
}

func TestPlanGuardRejectsFreshKnowledgeToolWhenRetrievalNoneWithoutReason(t *testing.T) {
	guard := NewPlanGuard(knowledgeToolCatalog(), nil)
	plan := &AgentPlan{
		Goal:   "opinion video",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		KnowledgePolicy: &KnowledgePolicy{
			ContentType:     "opinion",
			FreshnessLevel:  FreshnessNone,
			RetrievalPolicy: RetrievalNone,
			ForbiddenTools:  []string{"news_search"},
		},
		Steps: []AgentStep{
			{ID: "knowledge_search", Tool: "custom_news_search", Arguments: map[string]interface{}{"query": "AI workflows"}},
			{ID: "script", Tool: "video_script_generator", DependsOn: []string{"knowledge_search"}, Arguments: map[string]interface{}{"topic": "AI workflows"}},
		},
	}

	if err := guard.Validate(plan); err == nil {
		t.Fatal("expected guard to reject fresh knowledge tool for retrieval none without reason")
	}
}

func TestPlanGuardRejectsFreshKnowledgeToolWhenPolicyForbidsEvenWithReason(t *testing.T) {
	guard := NewPlanGuard(knowledgeToolCatalog(), nil)
	plan := &AgentPlan{
		Goal:   "creative fiction video",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		KnowledgePolicy: &KnowledgePolicy{
			ContentType:           "fiction",
			FreshnessLevel:        FreshnessNone,
			RetrievalPolicy:       RetrievalNone,
			ForbiddenCapabilities: FreshKnowledgeCapabilities(),
			Reason:                "creative fiction does not need current facts",
		},
		Steps: []AgentStep{
			{
				ID:        "knowledge_search",
				Tool:      "custom_news_search",
				Reason:    "planner thought search might help",
				Arguments: map[string]interface{}{"query": "cyberpunk story"},
			},
			{ID: "script", Tool: "video_script_generator", DependsOn: []string{"knowledge_search"}, Arguments: map[string]interface{}{"topic": "cyberpunk story"}},
		},
	}

	err := guard.Validate(plan)
	if err == nil {
		t.Fatal("expected guard to reject forbidden fresh knowledge tool even when step has reason")
	}
	if !strings.Contains(err.Error(), "knowledge policy forbids") {
		t.Fatalf("error should mention knowledge policy, got %v", err)
	}
}

func TestPlanCompilerInjectsGenericKnowledgeContextIntoScriptArguments(t *testing.T) {
	compiler := NewPlanCompiler(knowledgeToolCatalog())
	plan := &AgentPlan{
		Goal:            "current event video",
		Domain:          "video_creation",
		Mode:            "dynamic_agent",
		KnowledgePolicy: requiredNewsPolicy(),
		Steps: []AgentStep{
			{
				ID:             "retrieve_fresh_facts",
				Intent:         "获取最新事实",
				Tool:           "custom_news_search",
				Reason:         "用户提到世界杯出线，需要外部事实",
				Arguments:      map[string]interface{}{"query": "佛得角 世界杯 出线 最新", "topK": 5},
				ExpectedOutput: []string{"facts", "sources"},
			},
			{ID: "script", Tool: "video_script_generator", DependsOn: []string{"retrieve_fresh_facts"}, Arguments: map[string]interface{}{"topic": "佛得角世界杯出线"}},
		},
	}

	dag, err := compiler.Compile(plan)
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}

	script := requireNode(t, dag, "script", string(model.NodeTypeTool), "external")
	params := script.Input["parameters"].(map[string]interface{})
	kc, ok := params["knowledgeContext"].(map[string]interface{})
	if !ok {
		t.Fatalf("script should receive knowledgeContext, got %#v", params["knowledgeContext"])
	}
	items, _ := kc["items"].([]interface{})
	if len(items) != 1 || items[0] != "{{retrieve_fresh_facts.output.facts}}" {
		t.Fatalf("knowledgeContext should reference fresh facts output: %#v", kc)
	}
	sources, _ := kc["sources"].([]interface{})
	if len(sources) != 1 || sources[0] != "{{retrieve_fresh_facts.output.sources}}" {
		t.Fatalf("knowledgeContext should reference source output: %#v", kc)
	}
	generatedBy, _ := kc["generatedBy"].([]interface{})
	if len(generatedBy) != 1 || generatedBy[0] != "custom_news_search" {
		t.Fatalf("knowledgeContext should record source tool: %#v", kc)
	}
	if params["requireFreshFacts"] != true || params["retrievalPolicy"] != string(RetrievalRequired) {
		t.Fatalf("script should receive retrieval policy args: %#v", params)
	}
	requireEdge(t, dag, "retrieve_fresh_facts", "script")
}

func requiredNewsPolicy() *KnowledgePolicy {
	return &KnowledgePolicy{
		ContentType:          "current_event",
		FreshnessLevel:       FreshnessHigh,
		RetrievalPolicy:      RetrievalRequired,
		KnowledgeType:        "latest_news",
		AllowedTools:         []string{},
		RequiredCapabilities: []string{"fresh_knowledge", "news_search", "web_search", "fact_retrieval", "current_event_retrieval"},
		SearchQueries:        []string{"佛得角 2026 世界杯 出线 最新"},
		FreshnessDays:        60,
		MaxSearchResults:     8,
		MustUseFacts:         true,
		MustCiteFacts:        true,
		BlockOnEmptyFacts:    true,
	}
}

func knowledgeToolCatalog() staticToolCatalog {
	return staticToolCatalog{
		"custom_news_search": &tool.ToolManifest{
			Name:         "custom_news_search",
			Type:         "http",
			Endpoint:     "https://search.example.test/query",
			Description:  "Search latest news and current event facts for script generation.",
			Capabilities: []string{"fresh_knowledge", "news_search", "web_search"},
			Tags:         []string{"news", "search", "fact"},
			CostLevel:    tool.CostLow,
			RiskLevel:    tool.RiskLow,
			SideEffect:   false,
			Parameters: map[string]tool.ParamDef{
				"query": {Type: "string", Required: true},
				"topK":  {Type: "number", Required: false},
			},
			Output: map[string]tool.ParamDef{
				"facts":   {Type: "array"},
				"sources": {Type: "array"},
			},
		},
		"image_generator": &tool.ToolManifest{
			Name:         "image_generator",
			Type:         "http",
			Description:  "Generate images.",
			Capabilities: []string{"video_creation", "image_generation"},
			Parameters:   map[string]tool.ParamDef{"prompt": {Type: "string", Required: true}},
			Output:       map[string]tool.ParamDef{"imageUrl": {Type: "string"}},
			RiskLevel:    tool.RiskLow,
			CostLevel:    tool.CostLow,
		},
		"video_script_generator": &tool.ToolManifest{
			Name:     "video_script_generator",
			Type:     "builtin_prompt_tool",
			Endpoint: "builtin://video-creation/video_script_generator",
			Parameters: map[string]tool.ParamDef{
				"topic":            {Type: "string", Required: true},
				"knowledgeContext": {Type: "object"},
			},
			Output: map[string]tool.ParamDef{
				"script":            {Type: "string"},
				"usedFacts":         {Type: "array"},
				"factCheckWarnings": {Type: "array"},
				"knowledgeTrace":    {Type: "object"},
			},
		},
	}
}
