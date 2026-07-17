package agentruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestLLMPlanner_GeneratesAgentPlanFromTopKTools(t *testing.T) {
	client := &fakePlannerLLM{
		response: `{
			"goal": "生成小红书视频创作包",
			"domain": "video_creation",
			"mode": "dynamic_agent",
			"steps": [
				{
					"id": "script_generation",
					"intent": "生成口播稿",
					"tool": "video_script_generator",
					"arguments": {"topic": "AI替代的是工作流", "platform": "小红书"},
					"expectedOutput": ["script"],
					"produceArtifact": true
				}
			],
			"budget": {"maxLLMCalls": 2, "maxToolCalls": 3, "maxSteps": 3, "maxReplans": 1, "maxCostLevel": "medium"},
			"stopPolicy": {"stopWhenEnough": true}
		}`,
	}
	planner := NewLLMPlanner(staticToolList{
		{Name: "video_script_generator", Description: "生成视频口播稿", Capabilities: []string{"video_creation", "script_generation"}, Parameters: map[string]tool.ParamDef{"topic": {Type: "string", Required: true}}},
		{Name: "shot_splitter", Description: "拆分分镜", Capabilities: []string{"video_creation", "storyboard_generation"}},
		{Name: "video_prompt_generator", Description: "生成视频提示词", Capabilities: []string{"video_creation", "video_prompt_generation"}},
	}, client, LLMPlannerOptions{MaxTools: 2})

	plan, err := planner.GeneratePlan(context.Background(), StartRunRequest{
		Message: "帮我做一个小红书视频",
		Domain:  "video_creation",
		Context: map[string]interface{}{"platform": "小红书"},
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned error: %v", err)
	}
	if plan.Mode != "dynamic_agent" {
		t.Fatalf("plan mode = %q, want dynamic_agent", plan.Mode)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Tool != "video_script_generator" {
		t.Fatalf("unexpected plan steps: %#v", plan.Steps)
	}
	if !strings.Contains(client.lastUserPrompt, "video_script_generator") {
		t.Fatalf("planner prompt should include selected tool manifest, got: %s", client.lastUserPrompt)
	}
	if strings.Contains(client.lastUserPrompt, "video_prompt_generator") {
		t.Fatalf("planner prompt should only include top-k tools, got: %s", client.lastUserPrompt)
	}
	if !strings.Contains(client.lastUserPrompt, "AgentPlan") || !strings.Contains(client.lastSystemText, "禁止输出 DAGRequest") {
		t.Fatalf("planner prompt must ask for AgentPlan and forbid DAGRequest, system=%q user=%q", client.lastSystemText, client.lastUserPrompt)
	}
}

func TestClientProviderPlannerUsesRequestTextProvider(t *testing.T) {
	var gotAuthorization string
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected planner path: %s", r.URL.Path)
		}
		gotAuthorization = r.Header.Get("Authorization")
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode planner request: %v", err)
		}
		gotModel, _ = body["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"goal\":\"生成品牌故事视频\",\"domain\":\"video_creation\",\"mode\":\"dynamic_agent\",\"steps\":[{\"id\":\"script_generation\",\"intent\":\"生成口播稿\",\"tool\":\"video_script_generator\",\"arguments\":{\"topic\":\"独立咖啡店品牌故事\"},\"expectedOutput\":[\"script\"],\"produceArtifact\":true}],\"budget\":{\"maxLLMCalls\":1,\"maxToolCalls\":1,\"maxSteps\":1,\"maxReplans\":0,\"maxCostLevel\":\"medium\"},\"stopPolicy\":{\"stopWhenEnough\":true}}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	tools := staticToolList{
		{Name: "video_script_generator", Description: "生成视频口播稿", Capabilities: []string{"video_creation", "script_generation"}, Parameters: map[string]tool.ParamDef{"topic": {Type: "string", Required: true}}},
	}
	planner := NewClientProviderPlanner(tools, NewHeuristicPlannerWithMaxTools(tools, 1), 1)
	plan, err := planner.GeneratePlan(context.Background(), StartRunRequest{
		Message: "帮我做一个独立咖啡店品牌故事视频",
		Domain:  "video_creation",
		Context: map[string]interface{}{
			"modelProviders": map[string]interface{}{
				"text_to_text": map[string]interface{}{
					"baseUrl": server.URL,
					"apiKey":  "sk-client-planner",
					"model":   "client-planner-model",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned error: %v", err)
	}
	if gotAuthorization != "Bearer sk-client-planner" {
		t.Fatalf("expected planner to use client API key, got %q", gotAuthorization)
	}
	if gotModel != "client-planner-model" {
		t.Fatalf("expected planner to use client model, got %q", gotModel)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Tool != "video_script_generator" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}

func TestClientProviderPlannerAllowsSlowProviderBackedPlanning(t *testing.T) {
	cfg := providerConfigFromMap(map[string]interface{}{
		"baseUrl": "https://text.example/v1",
		"apiKey":  "sk-client-planner",
		"model":   "client-planner-model",
	})

	if cfg.Timeout < 180 {
		t.Fatalf("client provider planner timeout = %d, want at least 180 seconds", cfg.Timeout)
	}
}

func TestLLMPlannerPromptIncludesFreshKnowledgeCandidateForCurrentEvent(t *testing.T) {
	client := &fakePlannerLLM{
		response: `{
			"goal": "生成佛得角世界杯出线短视频",
			"domain": "video_creation",
			"mode": "dynamic_agent",
			"knowledgePolicy": {
				"contentType": "sports_event",
				"freshnessLevel": "high",
				"retrievalPolicy": "required",
				"knowledgeType": "latest_news",
				"searchQueries": ["佛得角 世界杯 出线 最新"],
				"mustUseFacts": true,
				"mustCiteFacts": true,
				"blockOnEmptyFacts": true
			},
			"steps": [
				{
					"id": "retrieve_fresh_facts",
					"intent": "获取与用户主题相关的最新事实",
					"tool": "custom_news_search",
					"reason": "用户提到世界杯出线，属于强时效体育事实，需要外部事实确认",
					"arguments": {"query": "佛得角 世界杯 出线 最新", "topK": 5},
					"expectedOutput": ["facts", "sources"]
				},
				{
					"id": "script_generation",
					"intent": "基于事实生成口播稿",
					"tool": "video_script_generator",
					"arguments": {"topic": "佛得角世界杯出线奇迹"},
					"dependsOn": ["retrieve_fresh_facts"],
					"expectedOutput": ["script", "usedFacts", "knowledgeTrace"],
					"produceArtifact": true
				}
			],
			"budget": {"maxLLMCalls": 2, "maxToolCalls": 3, "maxSteps": 3, "maxReplans": 1, "maxCostLevel": "medium"},
			"stopPolicy": {"stopWhenEnough": true}
		}`,
	}
	planner := NewLLMPlanner(staticToolList{
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
			Output: map[string]tool.ParamDef{"facts": {Type: "array"}, "sources": {Type: "array"}},
		},
		{
			Name:         "video_script_generator",
			Description:  "Generate video voiceover scripts.",
			Type:         "builtin_prompt_tool",
			Capabilities: []string{"video_creation", "script_generation"},
			Parameters:   map[string]tool.ParamDef{"topic": {Type: "string", Required: true}},
			Output:       map[string]tool.ParamDef{"script": {Type: "string"}},
		},
	}, client, LLMPlannerOptions{MaxTools: 3})

	plan, err := planner.GeneratePlan(context.Background(), StartRunRequest{
		Message: "请帮我做一个30秒视频，讲佛得角国家以及佛得角世界杯出线是一个奇迹。",
		Domain:  "video_creation",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned error: %v", err)
	}
	if !strings.Contains(client.lastUserPrompt, "custom_news_search") {
		t.Fatalf("planner prompt should include fresh knowledge candidate, got: %s", client.lastUserPrompt)
	}
	if !strings.Contains(client.lastUserPrompt, "matched fresh_knowledge") {
		t.Fatalf("planner prompt should include candidate reason, got: %s", client.lastUserPrompt)
	}
	if len(plan.Steps) == 0 || plan.Steps[0].Tool != "custom_news_search" {
		t.Fatalf("planner should preserve selected custom search tool: %#v", plan.Steps)
	}
	if plan.Steps[0].Reason == "" {
		t.Fatalf("planner should preserve tool selection reason: %#v", plan.Steps[0])
	}
}

func TestHybridPlanner_FallsBackWhenLLMPlannerFails(t *testing.T) {
	llmPlanner := NewLLMPlanner(staticToolList{
		{Name: "video_script_generator", Capabilities: []string{"video_creation", "script_generation"}},
	}, &fakePlannerLLM{errText: "llm unavailable"}, LLMPlannerOptions{MaxTools: 1})
	fallback := NewHeuristicPlanner(staticToolList{
		{Name: "video_script_generator", Capabilities: []string{"video_creation", "script_generation"}},
	})
	planner := NewHybridPlanner(llmPlanner, fallback)

	plan, err := planner.GeneratePlan(context.Background(), StartRunRequest{
		Message: "帮我做一个视频",
		Domain:  "video_creation",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned error: %v", err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Tool != "video_script_generator" {
		t.Fatalf("fallback plan not used: %#v", plan.Steps)
	}
}

func TestHybridPlanner_FallsBackWhenLLMPlannerReturnsEmptySteps(t *testing.T) {
	tools := staticToolList{
		{Name: "video_script_generator", Capabilities: []string{"video_creation", "script_generation"}},
	}
	llmPlanner := NewLLMPlanner(tools, &fakePlannerLLM{
		response: `{
			"goal": "生成 30 秒视频",
			"domain": "video_creation",
			"mode": "dynamic_agent",
			"steps": []
		}`,
	}, LLMPlannerOptions{MaxTools: 1})
	fallback := NewHeuristicPlanner(tools)
	planner := NewHybridPlanner(llmPlanner, fallback)

	plan, err := planner.GeneratePlan(context.Background(), StartRunRequest{
		Message: "帮我做一个30秒观点类视频",
		Domain:  "video_creation",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned error: %v", err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Tool != "video_script_generator" {
		t.Fatalf("fallback plan not used for empty LLM plan: %#v", plan.Steps)
	}
}

func TestLLMPlanner_WiresMissingRequiredInputsFromPriorOutputs(t *testing.T) {
	tools := staticToolList{
		{
			Name:         "video_script_generator",
			Capabilities: []string{"video_creation", "script_generation"},
			Parameters: map[string]tool.ParamDef{
				"topic": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"script": {Type: "string"},
			},
		},
		{
			Name:         "shot_splitter",
			Capabilities: []string{"video_creation", "shot_split"},
			Parameters: map[string]tool.ParamDef{
				"script": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"shotList": {Type: "array"},
			},
		},
	}
	planner := NewLLMPlanner(tools, &fakePlannerLLM{
		response: `{
			"goal": "生成 30 秒视频",
			"domain": "video_creation",
			"mode": "dynamic_agent",
			"steps": [
				{
					"id": "script_generation",
					"intent": "生成口播稿",
					"tool": "video_script_generator",
					"arguments": {"topic": "端午节和粽子的来历"},
					"expectedOutput": ["script"],
					"produceArtifact": true
				},
				{
					"id": "shot_split",
					"intent": "拆分分镜",
					"tool": "shot_splitter",
					"arguments": {},
					"dependsOn": ["script_generation"],
					"expectedOutput": ["shotList"],
					"produceArtifact": true
				}
			]
		}`,
	}, LLMPlannerOptions{MaxTools: 2})

	plan, err := planner.GeneratePlan(context.Background(), StartRunRequest{
		Message: "请帮我做一个30秒视频，讲端午节和粽子的来历。",
		Domain:  "video_creation",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned error: %v", err)
	}
	if got := plan.Steps[1].Arguments["script"]; got != "{{script_generation.output.script}}" {
		t.Fatalf("shot_splitter should reference generated script, got %#v", plan.Steps[1].Arguments)
	}
	if err := NewPlanGuard(tools, nil).Validate(plan); err != nil {
		t.Fatalf("wired LLM plan should pass PlanGuard: %v", err)
	}
}

func TestLLMPlanner_RewritesInvalidScriptReferenceFromNewsSearch(t *testing.T) {
	tools := staticToolList{
		{
			Name:         "news_search",
			Description:  "Search current sports news.",
			Type:         "http",
			Capabilities: []string{"video_creation", "fresh_knowledge", "news_search"},
			Parameters: map[string]tool.ParamDef{
				"query": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"facts":   {Type: "array"},
				"sources": {Type: "array"},
			},
		},
		{
			Name:         "video_script_generator",
			Description:  "Generate video voiceover scripts.",
			Type:         "builtin_prompt_tool",
			Capabilities: []string{"video_creation", "script_generation"},
			Parameters: map[string]tool.ParamDef{
				"topic": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"script": {Type: "string"},
			},
		},
		{
			Name:         "shot_splitter",
			Description:  "Split script into visual beats.",
			Type:         "builtin_prompt_tool",
			Capabilities: []string{"video_creation", "shot_planning"},
			Parameters: map[string]tool.ParamDef{
				"script": {Type: "string", Required: true},
			},
			Output: map[string]tool.ParamDef{
				"shotList": {Type: "array"},
			},
		},
	}
	planner := NewLLMPlanner(tools, &fakePlannerLLM{
		response: `{
			"goal": "生成佛得角世界杯奇迹视频",
			"domain": "video_creation",
			"mode": "dynamic_agent",
			"knowledgePolicy": {
				"contentType": "sports_event",
				"freshnessLevel": "high",
				"retrievalPolicy": "required",
				"knowledgeType": "latest_news",
				"searchQueries": ["Cape Verde World Cup knockout"],
				"mustUseFacts": true,
				"mustCiteFacts": true,
				"blockOnEmptyFacts": true
			},
			"steps": [
				{
					"id": "news_search",
					"intent": "检索最新事实",
					"tool": "news_search",
					"arguments": {"query": "Cape Verde World Cup knockout"},
					"expectedOutput": ["facts", "sources", "script"]
				},
				{
					"id": "script_generation",
					"intent": "生成口播稿",
					"tool": "video_script_generator",
					"arguments": {"topic": "佛得角世界杯奇迹"},
					"dependsOn": ["news_search"],
					"expectedOutput": ["script"],
					"produceArtifact": true
				},
				{
					"id": "beat_plan",
					"intent": "拆分分镜",
					"tool": "shot_splitter",
					"arguments": {"script": "{{news_search.output.script}}"},
					"dependsOn": ["news_search"],
					"expectedOutput": ["shotList"],
					"produceArtifact": true
				}
			]
		}`,
	}, LLMPlannerOptions{MaxTools: 3})

	plan, err := planner.GeneratePlan(context.Background(), StartRunRequest{
		Message: "帮我介绍一下佛得角国家以及说明佛得角世界杯小组赛出线进入淘汰赛是一个奇迹",
		Domain:  "video_creation",
	})
	if err != nil {
		t.Fatalf("GeneratePlan returned error: %v", err)
	}
	if got := plan.Steps[2].Arguments["script"]; got != "{{script_generation.output.script}}" {
		t.Fatalf("beat_plan should reference script_generation output, got %#v", plan.Steps[2].Arguments)
	}
	if !containsString(plan.Steps[2].DependsOn, "script_generation") {
		t.Fatalf("beat_plan should depend on script_generation, got %#v", plan.Steps[2].DependsOn)
	}
	if err := NewPlanGuard(tools, nil).Validate(plan); err != nil {
		t.Fatalf("rewritten LLM plan should pass PlanGuard: %v", err)
	}
}

type fakePlannerLLM struct {
	response       string
	errText        string
	lastSystemText string
	lastUserPrompt string
}

func (f *fakePlannerLLM) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	f.lastSystemText = systemPrompt
	f.lastUserPrompt = userPrompt
	if f.errText != "" {
		return "", errString(f.errText)
	}
	return f.response, nil
}

type errString string

func (e errString) Error() string { return string(e) }
