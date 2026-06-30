package builtin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway/providers/fake"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestRegisterVideoCreationExternalToolsInstallsOpinionVideoDependencies(t *testing.T) {
	registry := tool.NewToolRegistry()
	RegisterVideoCreationExternalTools(registry)

	for _, name := range []string{
		"skill_stage_agent",
		"image_asset_generator",
		"hyperframes_project_builder",
		"hyperframes_renderer",
		"hypergen_keyframes",
	} {
		if registry.GetExternalManifest(name) == nil {
			t.Fatalf("expected default video tool %q to be registered", name)
		}
	}
}

func TestRegisterVideoCreationExternalToolsInstallsVideoForgeDependencies(t *testing.T) {
	registry := tool.NewToolRegistry()
	RegisterVideoCreationExternalTools(registry)

	for _, name := range []string{
		"pipeline_selector",
		"capability_preflight",
		"proposal_generator",
		"visual_feasibility_analyzer",
		"render_strategy_planner",
	} {
		if registry.GetExternalManifest(name) == nil {
			t.Fatalf("expected VideoForge tool %q to be registered", name)
		}
	}
}

func TestRegisterVideoCreationExternalToolsInstallsKnowledgeTools(t *testing.T) {
	registry := tool.NewToolRegistry()
	RegisterVideoCreationExternalTools(registry)

	for _, name := range []string{"news_search", "fact_extractor"} {
		manifest := registry.GetExternalManifest(name)
		if manifest == nil {
			t.Fatalf("expected knowledge tool %q to be registered", name)
		}
		if len(manifest.Output) == 0 {
			t.Fatalf("knowledge tool %q should declare output schema", name)
		}
	}
}

func TestVideoScriptGeneratorBlocksRequiredRetrievalWithEmptyFacts(t *testing.T) {
	result := executeLocalVideoCreationTool("video_script_generator", map[string]interface{}{
		"topic":                 "佛得角世界杯出线",
		"retrievalPolicy":       "required",
		"mustUseFreshKnowledge": true,
		"knowledgePack":         []interface{}{},
	}, tool.ToolContext{TaskID: "task-1", NodeID: "script"})

	if result.Success {
		t.Fatalf("expected script generator to fail when required facts are empty: %#v", result.Data)
	}
}

func TestVideoScriptGeneratorConsumesKnowledgeContextWithoutModelFallback(t *testing.T) {
	SetVideoCreationConfig(config.OpenAIConfig{}, "")
	previousFetcher := localAgentConfigFetcher
	localAgentConfigFetcher = func() (RuntimeModelProviderConfig, bool) {
		return RuntimeModelProviderConfig{}, false
	}
	t.Cleanup(func() {
		localAgentConfigFetcher = previousFetcher
	})
	result := executeLocalVideoCreationTool("video_script_generator", map[string]interface{}{
		"topic":             "佛得角世界杯出线",
		"retrievalPolicy":   "required",
		"requireFreshFacts": true,
		"knowledgeContext": map[string]interface{}{
			"items": []interface{}{
				[]interface{}{
					map[string]interface{}{
						"claim":       "佛得角已经获得世界杯出线资格，这是该国足球历史上的里程碑。",
						"source":      "mock_news",
						"url":         "https://example.test/cape-verde",
						"publishedAt": "2026-06-28",
					},
				},
			},
			"sources": []interface{}{
				[]interface{}{map[string]interface{}{"source": "mock_news", "url": "https://example.test/cape-verde"}},
			},
			"generatedBy": []interface{}{"custom_news_search"},
		},
	}, tool.ToolContext{TaskID: "task-1", NodeID: "script"})

	if !result.Success {
		t.Fatalf("expected script generator to accept non-empty knowledgeContext: %s", result.Error)
	}
	usedFacts, ok := result.Data["usedFacts"].([]map[string]interface{})
	if !ok || len(usedFacts) != 1 {
		t.Fatalf("expected usedFacts from knowledgeContext: %#v", result.Data["usedFacts"])
	}
	if usedFacts[0]["claim"] != "佛得角已经获得世界杯出线资格，这是该国足球历史上的里程碑。" {
		t.Fatalf("unexpected used fact: %#v", usedFacts[0])
	}
	trace, ok := result.Data["knowledgeTrace"].(map[string]interface{})
	if !ok || trace["hasKnowledgeContext"] != true || trace["knowledgeItemCount"] != 1 {
		t.Fatalf("expected knowledgeTrace to describe consumed context: %#v", result.Data["knowledgeTrace"])
	}
}

func TestScriptQualityCheckerPromptUsesRequestedDurationAndFreshFacts(t *testing.T) {
	facts := appendKnowledgePolicyForPrompt(`knowledgeContext facts:
1. 佛得角已经获得2026年世界杯出线资格，这是该国足球历史上的里程碑。 来源：mock_news 日期：2026-06-28 URL：https://example.test/cape-verde`, map[string]interface{}{
		"retrievalPolicy":       "required",
		"mustUseFreshKnowledge": true,
		"currentDate":           "2026-06-28",
	})

	prompt := buildDynamicAgentUserPrompt(
		"script_quality_checker",
		"佛得角世界杯出线",
		facts,
		"",
		"佛得角首次进入世界杯，这是小国足球的奇迹。",
		"",
		"",
		"",
		"视频创作平台",
		30,
	)

	if !strings.Contains(prompt, "目标时长：30秒") {
		t.Fatalf("script quality checker should use requested target duration, got:\n%s", prompt)
	}
	if strings.Contains(prompt, "目标时长：90秒") {
		t.Fatalf("script quality checker should not hardcode 90 seconds, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "佛得角已经获得2026年世界杯出线资格") || !strings.Contains(prompt, "2026-06-28") {
		t.Fatalf("script quality checker should receive fresh knowledge facts, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "不得用模型内置旧知识否定") {
		t.Fatalf("script quality checker should prioritize provided facts over stale model memory, got:\n%s", prompt)
	}
}

func TestScriptQualityCheckerSystemPromptRequiresExplainableReport(t *testing.T) {
	prompt := buildDynamicAgentSystemPrompt("script_quality_checker", "端午节知识视频", "", "视频创作平台")
	for _, field := range []string{"analysisSummary", "rubricBreakdown", "keepDoing"} {
		if !strings.Contains(prompt, field) {
			t.Fatalf("script quality checker prompt should require %s, got:\n%s", field, prompt)
		}
	}
}

func TestShotSplitterPromptRequiresShotProductionPackets(t *testing.T) {
	prompt := buildDynamicAgentSystemPrompt("shot_splitter", "佛得角世界杯出线", "", "视频创作平台")
	for _, required := range []string{
		"不论 AIGC 还是 HyperFrames",
		"每个 shot 都是最小生产、审核和返工单元",
		"每个 shot 时长 3-15 秒",
		"每个 shot 的素材包必须完全独立",
		"narrationText",
		"materialLibraryHints",
		"referenceRequirements",
		"expectedArtifacts",
		"reviewPacket",
		"shotAssetPackages",
		"SHOT_REVIEW_PACKET",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("shot splitter prompt should contain %q, got:\n%s", required, prompt)
		}
	}
}

func TestKeyframePromptGeneratorRequiresPerShotReferenceCoverage(t *testing.T) {
	prompt := buildDynamicAgentSystemPrompt("keyframe_prompt_generator", "佛得角世界杯出线", "", "视频创作平台")
	for _, required := range []string{
		"每个 shot 都必须引用相关参考图",
		"主要人物、场景、道具",
		"三视角",
		"referenceCoverage",
		"relatedShotId",
		"SHOT_KEYFRAME",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("keyframe prompt generator prompt should contain %q, got:\n%s", required, prompt)
		}
	}
}

func TestVideoPromptGeneratorPromptRequiresIndependentShotGeneration(t *testing.T) {
	prompt := buildDynamicAgentSystemPrompt("video_prompt_generator", "佛得角世界杯出线", "", "视频创作平台")
	for _, required := range []string{
		"每个 shot 都必须作为独立视频片段生成",
		"shotAssetPackages",
		"referenceImages",
		"voiceover",
		"aigcVideo",
		"不要使用尾帧",
		"只使用首帧",
		"参考故事板",
		"转场设计写在本 shot 内部",
		"转场覆盖在本 shot 结尾",
		"ffmpeg",
		"简单剪辑拼接",
		"每个 shot 都必须绑定自己的口播",
		"素材库",
		"SHOT_VIDEO_CLIP",
		"SHOT_AUDIO",
		"SHOT_SUBTITLE",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("video prompt generator prompt should contain %q, got:\n%s", required, prompt)
		}
	}
}

func TestExecuteDynamicAgentPromptToolExposesShotAssetPackages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"videoPrompts\":[{\"shotId\":\"SHOT_01\",\"durationSec\":6,\"narrationText\":\"佛得角是西非岛国。\",\"prompt\":\"独立生成6秒视频，结尾0.5秒完成淡出转场。\"}],\"shotAssetPackages\":[{\"shotId\":\"SHOT_01\",\"durationSec\":6,\"referenceImages\":[{\"id\":\"keyframe_SHOT_01\",\"role\":\"keyframe\",\"storageRef\":\"local://projects/p/keyframes/SHOT_01.png\"}],\"prompts\":{\"videoPrompt\":\"独立生成6秒视频\",\"negativePrompt\":\"禁止真人写实\"},\"voiceover\":{\"text\":\"佛得角是西非岛国。\",\"artifactKind\":\"SHOT_AUDIO\"},\"aigcVideo\":{\"requestId\":\"extgen_video_SHOT_01\",\"artifactKind\":\"SHOT_VIDEO_CLIP\",\"concatMode\":\"simple_cut\",\"transitionAtEnd\":\"结尾0.5秒淡出\"},\"subtitle\":{\"text\":\"佛得角是西非岛国。\",\"artifactKind\":\"SHOT_SUBTITLE\"}}],\"summary\":\"ok\"}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	SetVideoCreationConfig(config.OpenAIConfig{
		APIKey:    "test-key",
		BaseURL:   server.URL,
		Model:     "deepseek-v4-pro",
		MaxTokens: 512,
		Timeout:   5,
	}, "")
	previousFetcher := localAgentConfigFetcher
	localAgentConfigFetcher = func() (RuntimeModelProviderConfig, bool) {
		return RuntimeModelProviderConfig{}, false
	}
	t.Cleanup(func() {
		SetVideoCreationConfig(config.OpenAIConfig{}, "")
		localAgentConfigFetcher = previousFetcher
	})

	result := executeDynamicAgentPromptTool("video_prompt_generator", "video_prompt", "video", "佛得角世界杯出线", "", map[string]interface{}{
		"topic": "佛得角世界杯出线",
		"shotList": []interface{}{
			map[string]interface{}{"shotId": "SHOT_01", "durationSec": 6, "narrationText": "佛得角是西非岛国。"},
		},
	}, tool.ToolContext{TaskID: "task-shot-packages", NodeID: "video_prompt_exec"})

	if !result.Success {
		t.Fatalf("expected video prompt generation to succeed: %s", result.Error)
	}
	packages, ok := result.Data["shotAssetPackages"].([]interface{})
	if !ok || len(packages) != 1 {
		t.Fatalf("expected shotAssetPackages to be exposed from structured output, got %#v", result.Data["shotAssetPackages"])
	}
}

func TestHyperFramesShotFirstFallbackUsesShotSectionsAndNarration(t *testing.T) {
	shotList := `{"shotList":[{"shotId":"SHOT_01","durationSec":6,"scriptText":"佛得角是一个西非岛国。","visual":"地图上出现佛得角群岛","camera":"缓慢推近"},{"shotId":"SHOT_02","durationSec":7,"narrationText":"世界杯出线对它来说是奇迹。","visual":"球场灯光亮起","camera":"横向移动"}]}`
	html := buildMinimalHyperFramesHTML("佛得角奇迹", "完整口播", shotList, "16:9")
	for _, required := range []string{
		`data-shot-id="SHOT_01"`,
		`data-shot-id="SHOT_02"`,
		"shot-review-packet",
		"佛得角是一个西非岛国。",
		"世界杯出线对它来说是奇迹。",
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("hyperframes fallback should contain %q, got:\n%s", required, html)
		}
	}
}

func TestHyperFramesDataJSONDeclaresShotFirstMode(t *testing.T) {
	data := buildHyperFramesDataJSON("佛得角奇迹", "完整口播", `[{"shotId":"SHOT_01","scriptText":"..."}]`, `[]`, `[]`, "16:9", "{}")
	for _, required := range []string{
		`"productionMode": "shot_first"`,
		`"reviewUnit": "shot"`,
		`"requiresNarrationSync": true`,
		`"shotAssetPackages"`,
		`"shots"`,
	} {
		if !strings.Contains(data, required) {
			t.Fatalf("hyperframes data json should contain %q, got:\n%s", required, data)
		}
	}
}

func TestPublishCopyGeneratorPromptRequiresScriptDerivedStructuredCopy(t *testing.T) {
	if !isStructuredOutputTool("publish_copy_generator") {
		t.Fatal("publish_copy_generator should require structured JSON output")
	}
	prompt := buildDynamicAgentSystemPrompt("publish_copy_generator", "帮我介绍一下佛得角国家以及说明佛得角世界杯从小组赛出线是一个奇迹", "", "视频创作平台")
	for _, required := range []string{
		"必须根据口播稿生成",
		"不要直接把用户原始输入当标题",
		"publishCopies",
		"keywords",
		"xiaohongshu",
		"bilibili",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("publish copy generator prompt should contain %q, got:\n%s", required, prompt)
		}
	}
}

func TestVideoScriptGeneratorPromptIncludesRequestedDuration(t *testing.T) {
	prompt := buildDynamicAgentUserPrompt(
		"video_script_generator",
		"佛得角世界杯出线",
		"retrievalPolicy=required\ncurrentDate=2026-06-28",
		"",
		"",
		"",
		"",
		"",
		"视频创作平台",
		30,
	)

	if !strings.Contains(prompt, "目标时长：30秒") {
		t.Fatalf("script generator should receive concrete target duration, got:\n%s", prompt)
	}
}

func TestNormalizeStructuredToolContentUsesCanonicalJSON(t *testing.T) {
	raw := `我们被要求输出一个JSON，先分析事实。

{
  "facts": ["佛得角是西非岛国"],
  "timeline": [{"date": "2025-10-13", "event": "首次晋级世界杯"}],
  "storyAngles": ["小国奇迹"],
  "risks": ["赔率数据需谨慎"],
  "sourceNotes": ["ESPN"],
  "summary": "佛得角首次晋级世界杯。"
}

以上是最终结果。`

	content, pkg, ok := normalizeStructuredToolContent("knowledge_researcher", raw)
	if !ok {
		t.Fatalf("expected structured content to parse")
	}
	if strings.Contains(content, "先分析事实") || strings.Contains(content, "以上是最终结果") {
		t.Fatalf("content should strip model reasoning, got %q", content)
	}
	if !json.Valid([]byte(content)) {
		t.Fatalf("content should be canonical JSON, got %q", content)
	}
	if pkg["summary"] != "佛得角首次晋级世界杯。" {
		t.Fatalf("expected parsed package, got %#v", pkg)
	}
}

func TestExecuteDynamicAgentPromptToolRetriesStructuredJSONOutput(t *testing.T) {
	requests := make([]map[string]interface{}, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		requests = append(requests, body)
		w.Header().Set("Content-Type", "application/json")
		if len(requests) == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"我先分析一下，这不是 JSON。"},"finish_reason":"stop"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"checkedFacts\":[\"佛得角是西非岛国\"],\"warnings\":[],\"corrections\":[],\"passed\":true,\"summary\":\"核查完成\"}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	SetVideoCreationConfig(config.OpenAIConfig{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "deepseek-v4-pro",
		MaxTokens:   512,
		Temperature: 0.7,
		Timeout:     5,
	}, "")
	previousFetcher := localAgentConfigFetcher
	localAgentConfigFetcher = func() (RuntimeModelProviderConfig, bool) {
		return RuntimeModelProviderConfig{}, false
	}
	t.Cleanup(func() {
		SetVideoCreationConfig(config.OpenAIConfig{}, "")
		localAgentConfigFetcher = previousFetcher
	})

	result := executeDynamicAgentPromptTool("fact_checker", "proposal", "video", "佛得角世界杯出线", "", map[string]interface{}{
		"topic": "佛得角世界杯出线",
		"facts": "佛得角首次晋级世界杯。",
	}, tool.ToolContext{TaskID: "task-1", NodeID: "fact_checker_exec"})
	if !result.Success {
		t.Fatalf("expected retry to recover structured JSON, got %s", result.Error)
	}
	if len(requests) != 2 {
		t.Fatalf("expected one retry after non-JSON output, got %d requests", len(requests))
	}
	if requests[0]["max_tokens"] != float64(8000) {
		t.Fatalf("expected max_tokens 8000 on first request, got %#v", requests[0]["max_tokens"])
	}
	if requests[0]["temperature"] != float64(0) {
		t.Fatalf("expected temperature 0 for structured request, got %#v", requests[0]["temperature"])
	}
	rf, ok := requests[0]["response_format"].(map[string]interface{})
	if !ok || rf["type"] != "json_object" {
		t.Fatalf("expected json_object response_format, got %#v", requests[0]["response_format"])
	}
	messages, ok := requests[1]["messages"].([]interface{})
	if !ok || len(messages) == 0 {
		t.Fatalf("expected retry messages, got %#v", requests[1]["messages"])
	}
	last, ok := messages[len(messages)-1].(map[string]interface{})
	if !ok || !strings.Contains(strings.ToLower(ensureStringValue(last["content"])), "json") {
		t.Fatalf("expected retry prompt to reinforce JSON mode, got %#v", last)
	}
	if passed, ok := result.Data["passed"].(bool); !ok || !passed {
		t.Fatalf("expected parsed JSON package fields in result, got %#v", result.Data)
	}
}

func TestExecutePipelineSelectorReturnsProposalFirstSelection(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "knowledge-video.yaml"), `
id: knowledge-video
name: 知识口播视频
inputKinds:
  - brief
keywords:
  - 知识
  - 分享
stages:
  - id: brief
    director: brief_director
    produces: video_brief
  - id: proposal
    director: proposal_director
    produces: proposal_packet
    reviewRequired: true
`)

	result := executeLocalVideoCreationTool("pipeline_selector", map[string]interface{}{
		"brief":        "请帮我根据端午节的来历创作一个 60 秒知识分享视频",
		"pipelineRoot": root,
	}, tool.ToolContext{TaskID: "task-1", NodeID: "node-1"})

	if !result.Success {
		t.Fatalf("pipeline_selector failed: %s", result.Error)
	}
	selection, ok := result.Data["pipelineSelection"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing pipelineSelection: %#v", result.Data)
	}
	if selection["pipelineId"] != "knowledge-video" || selection["firstApprovalStage"] != "proposal" {
		t.Fatalf("unexpected selection: %#v", selection)
	}
}

func TestExecuteProposalGeneratorReturnsDecisionLoggedPacket(t *testing.T) {
	SetVideoCreationConfig(config.OpenAIConfig{}, "")
	previousFetcher := localAgentConfigFetcher
	localAgentConfigFetcher = func() (RuntimeModelProviderConfig, bool) {
		return RuntimeModelProviderConfig{}, false
	}
	t.Cleanup(func() {
		SetVideoCreationConfig(config.OpenAIConfig{}, "")
		localAgentConfigFetcher = previousFetcher
	})

	result := executeLocalVideoCreationTool("proposal_generator", map[string]interface{}{
		"brief":             "请帮我根据端午节的来历创作一个 60 秒知识分享视频",
		"targetDurationSec": float64(60),
		"seedanceAvailable": true,
	}, tool.ToolContext{TaskID: "task-1", NodeID: "node-1"})

	if !result.Success {
		t.Fatalf("proposal_generator failed: %s", result.Error)
	}
	packet, ok := result.Data["proposalPacket"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing proposalPacket: %#v", result.Data)
	}
	if packet["artifactKind"] != "proposal_packet" || packet["recommendedOptionId"] != "option_a" {
		t.Fatalf("unexpected proposal packet: %#v", packet)
	}
	if result.Data["decisionLog"] == nil {
		t.Fatalf("proposal_generator should expose decisionLog: %#v", result.Data)
	}
}

func TestExecuteRenderStrategyPlannerReturnsHybridStrategy(t *testing.T) {
	result := executeLocalVideoCreationTool("render_strategy_planner", map[string]interface{}{
		"shots": []interface{}{
			map[string]interface{}{
				"shotId": "SHOT_01",
				"elements": []interface{}{
					map[string]interface{}{"id": "title", "type": "text", "requiresExactText": true},
				},
			},
			map[string]interface{}{
				"shotId": "SHOT_02",
				"elements": []interface{}{
					map[string]interface{}{"id": "background", "type": "natural_motion", "requiresComplexMotion": true},
					map[string]interface{}{"id": "caption", "type": "text", "requiresExactText": true},
				},
			},
		},
		"seedanceAvailable": true,
	}, tool.ToolContext{TaskID: "task-1", NodeID: "node-1"})

	if !result.Success {
		t.Fatalf("render_strategy_planner failed: %s", result.Error)
	}
	strategy, ok := result.Data["renderStrategy"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing renderStrategy: %#v", result.Data)
	}
	if strategy["artifactKind"] != "render_strategy" || strategy["overallMode"] != "hybrid" {
		t.Fatalf("unexpected render strategy: %#v", strategy)
	}
	if result.Data["decisionLog"] == nil {
		t.Fatalf("render_strategy_planner should expose decisionLog: %#v", result.Data)
	}
}

func TestExecuteAssetDecisionAgentReturnsReferenceAssetPlan(t *testing.T) {
	result := executeLocalVideoCreationTool("asset_decision_agent", map[string]interface{}{
		"stage": "reference",
		"cardPlan": map[string]interface{}{
			"cards": []interface{}{
				map[string]interface{}{"cardId": "card_001", "title": "佛得角奇迹"},
			},
		},
		"shotList": []interface{}{
			map[string]interface{}{"shotId": "shot_001", "visual": "佛得角群岛地图与世界杯标志", "durationSec": float64(6)},
			map[string]interface{}{"shotId": "shot_002", "visual": "数据卡片和时间轴动画", "durationSec": float64(8)},
		},
		"compositionSpec": map[string]interface{}{
			"totalDurationSec": float64(30),
		},
	}, tool.ToolContext{TaskID: "task-asset", NodeID: "asset_decision_agent_exec"})

	if !result.Success {
		t.Fatalf("asset_decision_agent failed: %s", result.Error)
	}
	plan, ok := result.Data["referenceAssetPlan"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing referenceAssetPlan: %#v", result.Data)
	}
	shots, ok := plan["shots"].([]map[string]interface{})
	if !ok || len(shots) != 2 {
		t.Fatalf("expected two shot asset decisions, got %#v", plan["shots"])
	}
	allowed := map[string]bool{
		"open_asset_search":    true,
		"hyperframes_html":     true,
		"aigc_image_video_api": true,
		"manual_upload":        true,
		"placeholder_fallback": true,
	}
	for _, shot := range shots {
		source := ensureStringValue(shot["source"])
		if !allowed[source] {
			t.Fatalf("unexpected asset source %q in shot decision %#v", source, shot)
		}
	}
	artifacts, ok := result.Data["artifacts"].([]map[string]interface{})
	if !ok || len(artifacts) == 0 {
		t.Fatalf("asset_decision_agent should expose artifacts: %#v", result.Data["artifacts"])
	}
	if artifacts[0]["kind"] != "REFERENCE_ASSET_PLAN" {
		t.Fatalf("expected REFERENCE_ASSET_PLAN artifact, got %#v", artifacts[0])
	}
}

func TestTextImageToVideoGeneratorUsesFakeModelGatewayForFinalVideoArtifact(t *testing.T) {
	gw := modelgateway.NewGateway("fake")
	fp := fake.NewProvider()
	fp.LatencyMs = 0
	gw.RegisterProvider(fp, modelgateway.CapTextToVideo, modelgateway.CapImageToVideo)
	previousGateway := modelGateway
	SetModelGateway(gw)
	t.Cleanup(func() {
		SetModelGateway(previousGateway)
	})

	result := executeLocalVideoCreationTool("text_image_to_video_generator", map[string]interface{}{
		"stage":    "render",
		"prompt":   "生成一段佛得角世界杯奇迹的动态图文视频",
		"imageUrl": "local://projects/vp-1/keyframes/shot_001.png",
	}, tool.ToolContext{TaskID: "task-video", NodeID: "text_image_to_video_generator_exec"})

	if !result.Success {
		t.Fatalf("text_image_to_video_generator failed: %s", result.Error)
	}
	artifacts, ok := result.Data["artifacts"].([]map[string]interface{})
	if !ok || len(artifacts) == 0 {
		t.Fatalf("expected final video artifact manifest, got %#v", result.Data["artifacts"])
	}
	if artifacts[0]["kind"] != "VIDEO" {
		t.Fatalf("expected VIDEO artifact, got %#v", artifacts[0])
	}
	if ensureStringValue(artifacts[0]["storageRef"]) == "" {
		t.Fatalf("VIDEO artifact should include storageRef: %#v", artifacts[0])
	}
}

func TestOneSentenceFakeProviderChainReachesFinalVideoArtifact(t *testing.T) {
	gw := modelgateway.NewGateway("fake")
	fp := fake.NewProvider()
	fp.LatencyMs = 0
	gw.RegisterProvider(fp, modelgateway.CapTextToVideo, modelgateway.CapImageToVideo)
	previousGateway := modelGateway
	SetModelGateway(gw)
	t.Cleanup(func() {
		SetModelGateway(previousGateway)
	})

	assetResult := executeLocalVideoCreationTool("asset_decision_agent", map[string]interface{}{
		"stage": "reference",
		"brief": "请帮我做一个30秒视频，讲佛得角国家以及佛得角世界杯出线是一个奇迹。",
		"shotList": []interface{}{
			map[string]interface{}{"shotId": "shot_001", "visual": "佛得角地图、国旗和数据卡片动画", "durationSec": float64(8)},
		},
		"compositionSpec": map[string]interface{}{"totalDurationSec": float64(30)},
	}, tool.ToolContext{TaskID: "task-chain", NodeID: "asset_decision_agent_exec"})
	if !assetResult.Success {
		t.Fatalf("asset_decision_agent failed: %s", assetResult.Error)
	}
	if _, ok := assetResult.Data["referenceAssetPlan"].(map[string]interface{}); !ok {
		t.Fatalf("asset decision should return referenceAssetPlan: %#v", assetResult.Data)
	}

	videoResult := executeLocalVideoCreationTool("text_image_to_video_generator", map[string]interface{}{
		"stage":  "render",
		"prompt": "根据审核后的卡片、分镜和视频结构生成最终视频占位产物",
	}, tool.ToolContext{TaskID: "task-chain", NodeID: "text_image_to_video_generator_exec"})
	if !videoResult.Success {
		t.Fatalf("fake video generation failed: %s", videoResult.Error)
	}
	artifacts, ok := videoResult.Data["artifacts"].([]map[string]interface{})
	if !ok || len(artifacts) == 0 || artifacts[0]["kind"] != "VIDEO" {
		t.Fatalf("fake chain should reach final VIDEO artifact, got %#v", videoResult.Data["artifacts"])
	}
}

func TestBuildSkillStageArtifactsDoesNotEmitPublishCopyForNonPublishStage(t *testing.T) {
	artifacts := buildSkillStageArtifacts("viewpoint_dossier", "create-opinion-videos", false, true)
	for _, artifact := range artifacts {
		if artifact["unitId"] == "publish-copy" {
			t.Fatalf("non-publish stages should not emit publish-copy artifacts: %+v", artifact)
		}
	}
}

func TestBuildSkillStageArtifactsEmitsPublishCopyForPublishPackageStage(t *testing.T) {
	artifacts := buildSkillStageArtifacts("publish_package", "create-opinion-videos", true, true)
	found := false
	for _, artifact := range artifacts {
		if artifact["unitId"] == "publish-copy" {
			found = true
		}
	}
	if !found {
		t.Fatalf("publish_package stage should emit a publish-copy artifact: %+v", artifacts)
	}
}

func TestBuildSkillStageArtifactsUsesShotSemanticKinds(t *testing.T) {
	cases := map[string]string{
		"shot_splitter":             "SHOT_LIST",
		"keyframe_prompt_generator": "KEYFRAME_PROMPTS",
		"video_prompt_generator":    "VIDEO_PROMPTS",
	}
	for toolName, wantKind := range cases {
		artifacts := buildSkillStageArtifacts(toolName, "video-creator", false, true)
		if len(artifacts) == 0 || artifacts[0]["kind"] != wantKind {
			t.Fatalf("%s should produce semantic kind %s, got %+v", toolName, wantKind, artifacts)
		}
	}
}

func TestSkillStageContinuationDetectsLengthFinishReason(t *testing.T) {
	if !needsSkillStageContinuation("hyperframes_reference", "## BEAT 03\n证据", "length") {
		t.Fatal("length finish reason should request continuation")
	}
}

func TestSkillStageContinuationDetectsIncompleteHyperframesReference(t *testing.T) {
	partial := `# HyperFrames 视频参考

### 四、逐段画面脚本

## BEAT 03｜防疫实证
**参考时长**：20—36秒

### 口播
证据一：挂艾草、菖蒲。这两种植物在古代就是天然驱虫剂。证据`

	if !needsSkillStageContinuation("hyperframes_reference", partial, "stop") {
		t.Fatal("hyperframes_reference missing later chapters and ending mid-sentence should request continuation")
	}
}

func TestSkillStageContinuationDoesNotTriggerForCompleteHyperframesReference(t *testing.T) {
	complete := `# HyperFrames 视频参考

### 一、观点档案
ok
### 二、视频总体设定
ok
### 三、重要制作原则
ok
### 四、逐段画面脚本
## BEAT 01｜开场
### 转场
ok
### 五、imagegen 图片清单
ok
### 六、图片在视频中的处理方式
ok
### 七、字幕和文字规则
ok
### 八、整体节奏控制
ok
### 九、推荐项目素材目录
ok。`

	if needsSkillStageContinuation("hyperframes_reference", complete, "stop") {
		t.Fatal("complete hyperframes_reference should not request continuation")
	}
}

func TestAppendContinuationContinuesMidSentenceInline(t *testing.T) {
	got := appendContinuation("证据一：挂艾草、菖蒲。这两种植物在古代就是天然驱虫剂。证据", "二：雄黄酒。")
	want := "证据一：挂艾草、菖蒲。这两种植物在古代就是天然驱虫剂。证据二：雄黄酒。"
	if got != want {
		t.Fatalf("unexpected continuation join:\ngot  %q\nwant %q", got, want)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
