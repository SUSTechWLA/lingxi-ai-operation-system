package agentruntime

import (
	"context"
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
