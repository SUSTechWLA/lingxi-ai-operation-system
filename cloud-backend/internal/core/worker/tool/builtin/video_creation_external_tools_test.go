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
	"github.com/tangying-ai/aios-core/internal/core/hyperframes"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestOptionalLocalAgentModelConfigDisabledByDefault(t *testing.T) {
	t.Setenv("AIOS_ENABLE_LOCAL_AGENT_MODEL_CONFIG", "")
	ClearRuntimeModelProviderConfig()
	SetVideoCreationConfig(config.OpenAIConfig{
		APIKey:  "env-key",
		BaseURL: "https://env.example/v1",
		Model:   "env-model",
	}, "")

	previousFetcher := localAgentConfigFetcher
	called := false
	localAgentConfigFetcher = func() (RuntimeModelProviderConfig, bool) {
		called = true
		return RuntimeModelProviderConfig{
			APIKey:  "local-key",
			BaseURL: "https://local.example/v1",
			Model:   "local-model",
		}, true
	}
	t.Cleanup(func() {
		SetVideoCreationConfig(config.OpenAIConfig{}, "")
		ClearRuntimeModelProviderConfig()
		localAgentConfigFetcher = previousFetcher
	})

	cfg := applyOptionalLocalAgentConfig(GetVideoCreationOpenAIConfig())
	if called {
		t.Fatalf("local agent model config fetcher must not run unless explicitly enabled")
	}
	if cfg.APIKey != "env-key" || cfg.BaseURL != "https://env.example/v1" || cfg.Model != "env-model" {
		t.Fatalf("expected env config to remain effective, got %+v", cfg)
	}
}

func TestOptionalLocalAgentModelConfigRequiresExplicitOptIn(t *testing.T) {
	t.Setenv("AIOS_ENABLE_LOCAL_AGENT_MODEL_CONFIG", "true")
	ClearRuntimeModelProviderConfig()
	SetVideoCreationConfig(config.OpenAIConfig{
		APIKey:  "env-key",
		BaseURL: "https://env.example/v1",
		Model:   "env-model",
	}, "")

	previousFetcher := localAgentConfigFetcher
	localAgentConfigFetcher = func() (RuntimeModelProviderConfig, bool) {
		return RuntimeModelProviderConfig{
			APIKey:  "local-key",
			BaseURL: "https://local.example/v1",
			Model:   "local-model",
		}, true
	}
	t.Cleanup(func() {
		SetVideoCreationConfig(config.OpenAIConfig{}, "")
		ClearRuntimeModelProviderConfig()
		localAgentConfigFetcher = previousFetcher
	})

	cfg := applyOptionalLocalAgentConfig(GetVideoCreationOpenAIConfig())
	if cfg.APIKey != "local-key" || cfg.BaseURL != "https://local.example/v1" || cfg.Model != "local-model" {
		t.Fatalf("expected opted-in local config to override env config, got %+v", cfg)
	}
}

func TestEffectiveVideoCreationConfigUsesOptedInLocalAgentConfig(t *testing.T) {
	t.Setenv("AIOS_ENABLE_LOCAL_AGENT_MODEL_CONFIG", "true")
	ClearRuntimeModelProviderConfig()
	SetVideoCreationConfig(config.OpenAIConfig{}, "")

	previousFetcher := localAgentConfigFetcher
	localAgentConfigFetcher = func() (RuntimeModelProviderConfig, bool) {
		return RuntimeModelProviderConfig{
			APIKey:  "local-key",
			BaseURL: "https://local.example/v1",
			Model:   "local-model",
		}, true
	}
	t.Cleanup(func() {
		SetVideoCreationConfig(config.OpenAIConfig{}, "")
		ClearRuntimeModelProviderConfig()
		localAgentConfigFetcher = previousFetcher
	})

	cfg := effectiveVideoCreationOpenAIConfig(map[string]interface{}{})
	if cfg.APIKey != "local-key" || cfg.BaseURL != "https://local.example/v1" || cfg.Model != "local-model" {
		t.Fatalf("expected effective config to use opted-in local agent config, got %+v", cfg)
	}
}

func TestKnowledgeResearcherUsesClientModelProviderParam(t *testing.T) {
	SetVideoCreationConfig(config.OpenAIConfig{}, "")
	ClearRuntimeModelProviderConfig()
	t.Setenv("SEARCH_API_KEY", "")

	var gotAuthorization string
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		gotAuthorization = r.Header.Get("Authorization")
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		gotModel, _ = body["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"佛得角奇迹\",\"facts\":[{\"claim\":\"佛得角小组出线进入淘汰赛是历史性突破\"}],\"sources\":[]}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	result := executeLocalVideoCreationTool("knowledge_researcher", map[string]interface{}{
		"topic": "佛得角世界杯小组赛出线进入淘汰赛",
		"modelProvider": map[string]interface{}{
			"baseUrl": server.URL,
			"apiKey":  "sk-client",
			"model":   "client-model",
		},
	}, tool.ToolContext{TaskID: "task-1", NodeID: "knowledge"})

	if !result.Success {
		t.Fatalf("expected knowledge_researcher to use client provider, got %s", result.Error)
	}
	content, _ := result.Data["content"].(string)
	if strings.Contains(content, "LLM API Key 未配置") {
		t.Fatalf("should not return missing-key placeholder when client provider is present: %s", content)
	}
	if gotAuthorization != "Bearer sk-client" {
		t.Fatalf("expected client API key to be used, got %q", gotAuthorization)
	}
	if gotModel != "client-model" {
		t.Fatalf("expected client model, got %q", gotModel)
	}
}

func TestImageAssetGeneratorUsesClientOpenAICompatibleImageProvider(t *testing.T) {
	var gotAuthorization string
	var gotPath string
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		gotModel, _ = body["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/cape-verde.png"}]}`))
	}))
	defer server.Close()

	result := executeLocalVideoCreationTool("image_asset_generator", map[string]interface{}{
		"brief": "佛得角地图和足球",
		"modelProvider": map[string]interface{}{
			"baseUrl": server.URL + "/v1",
			"apiKey":  "sk-image",
			"model":   "gpt-image-1",
		},
	}, tool.ToolContext{TaskID: "task-image", NodeID: "image_asset_generator_exec"})

	if !result.Success {
		t.Fatalf("image_asset_generator failed: %s", result.Error)
	}
	if gotPath != "/v1/images/generations" {
		t.Fatalf("expected OpenAI-compatible image path, got %s", gotPath)
	}
	if gotAuthorization != "Bearer sk-image" {
		t.Fatalf("expected image API key, got %q", gotAuthorization)
	}
	if gotModel != "gpt-image-1" {
		t.Fatalf("expected configured image model, got %q", gotModel)
	}
	requests, ok := result.Data["imageRequests"].([]map[string]interface{})
	if !ok || len(requests) == 0 || requests[0]["storageRef"] != "https://cdn.example/cape-verde.png" {
		t.Fatalf("expected generated image storage ref, got %#v", result.Data["imageRequests"])
	}
}

func TestTextImageToVideoGeneratorUsesClientOpenAICompatibleVideoProvider(t *testing.T) {
	previousGateway := modelGateway
	SetModelGateway(nil)
	t.Cleanup(func() {
		SetModelGateway(previousGateway)
	})

	var gotAuthorization string
	var gotPath string
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		gotModel, _ = body["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example/cape-verde.mp4"}]}`))
	}))
	defer server.Close()

	result := executeLocalVideoCreationTool("text_image_to_video_generator", map[string]interface{}{
		"prompt": "佛得角世界杯奇迹",
		"modelProvider": map[string]interface{}{
			"baseUrl": server.URL + "/v1",
			"apiKey":  "sk-video",
			"model":   "sora",
		},
	}, tool.ToolContext{TaskID: "task-video", NodeID: "text_image_to_video_generator_exec"})

	if !result.Success {
		t.Fatalf("text_image_to_video_generator failed: %s", result.Error)
	}
	if gotPath != "/v1/videos/generations" {
		t.Fatalf("expected OpenAI-compatible video path, got %s", gotPath)
	}
	if gotAuthorization != "Bearer sk-video" {
		t.Fatalf("expected video API key, got %q", gotAuthorization)
	}
	if gotModel != "sora" {
		t.Fatalf("expected configured video model, got %q", gotModel)
	}
	if result.Data["video"] != "local://projects/task-video/renders/model-gateway-final.mp4" {
		t.Fatalf("expected remote video to be converted to local storage ref, got %#v", result.Data["video"])
	}
}

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
		"shot_generation_planner",
	} {
		if registry.GetExternalManifest(name) == nil {
			t.Fatalf("expected VideoForge tool %q to be registered", name)
		}
	}
	manifest := registry.GetExternalManifest("shot_generation_planner")
	if manifest.Parameters["shotList"].Type != "array" || !manifest.Parameters["shotList"].Required {
		t.Fatalf("shot_generation_planner should require shotList array, got %#v", manifest.Parameters["shotList"])
	}
	if manifest.Output["shotGenerationPlans"].Type != "array" {
		t.Fatalf("shot_generation_planner should output shotGenerationPlans array, got %#v", manifest.Output["shotGenerationPlans"])
	}
}

func TestRegisterVideoCreationExternalToolsInstallsMCPGenerationRunnerManifest(t *testing.T) {
	registry := tool.NewToolRegistry()
	RegisterVideoCreationExternalTools(registry)

	manifest := registry.GetExternalManifest("mcp_generation_runner")
	if manifest == nil {
		t.Fatal("expected mcp_generation_runner to be registered")
	}
	if manifest.ExecutionPlane != tool.ExecutionPlaneLocal {
		t.Fatalf("ExecutionPlane = %q, want local", manifest.ExecutionPlane)
	}
	if manifest.LocalCommand != "LOCAL_MCP_TOOL_CALL" {
		t.Fatalf("LocalCommand = %q, want LOCAL_MCP_TOOL_CALL", manifest.LocalCommand)
	}
	if !manifest.RequiresUserDevice {
		t.Fatal("mcp_generation_runner should require user device")
	}
	if !manifest.ApprovalPolicy.Required || manifest.ApprovalPolicy.Mode != tool.ApprovalBeforeExecute {
		t.Fatalf("mcp_generation_runner approval policy = %#v, want before_execute", manifest.ApprovalPolicy)
	}
}

func TestRegisterVideoCreationExternalToolsInstallsVideoFrameQAManifest(t *testing.T) {
	registry := tool.NewToolRegistry()
	RegisterVideoCreationExternalTools(registry)

	manifest := registry.GetExternalManifest("video_frame_qa")
	if manifest == nil {
		t.Fatal("expected video_frame_qa to be registered")
	}
	if manifest.ExecutionPlane != tool.ExecutionPlaneLocal {
		t.Fatalf("ExecutionPlane = %q, want local", manifest.ExecutionPlane)
	}
	if manifest.LocalCommand != "VIDEO_FRAME_QA" {
		t.Fatalf("LocalCommand = %q, want VIDEO_FRAME_QA", manifest.LocalCommand)
	}
	if !manifest.RequiresUserDevice {
		t.Fatal("video_frame_qa should require user device")
	}
	if !manifest.ApprovalPolicy.Required || manifest.ApprovalPolicy.Mode != tool.ApprovalAfterArtifact || !manifest.ApprovalPolicy.BlocksDownstream {
		t.Fatalf("video_frame_qa approval policy = %#v, want blocking after_artifact", manifest.ApprovalPolicy)
	}
	for _, kind := range []string{"VIDEO_VISUAL_QA_REPORT", "VIDEO_VISUAL_QA_CONTACT_SHEET", "SHOT_QA_REPORT", "SHOT_REPAIR_PLAN"} {
		if !containsString(manifest.ApprovalPolicy.ReviewArtifactKinds, kind) {
			t.Fatalf("video_frame_qa review artifact kinds missing %s: %#v", kind, manifest.ApprovalPolicy.ReviewArtifactKinds)
		}
	}
	if manifest.HumanReview == nil || !manifest.HumanReview.Required {
		t.Fatalf("video_frame_qa human review missing: %#v", manifest.HumanReview)
	}
	for _, kind := range []string{"VIDEO_VISUAL_QA_REPORT", "VIDEO_VISUAL_QA_CONTACT_SHEET", "SHOT_QA_REPORT", "SHOT_REPAIR_PLAN"} {
		if !containsString(manifest.ArtifactPolicy.ArtifactKinds, kind) {
			t.Fatalf("video_frame_qa artifact kinds missing %s: %#v", kind, manifest.ArtifactPolicy.ArtifactKinds)
		}
	}
	if !containsString(manifest.Capabilities, "shot_quality_gate") {
		t.Fatalf("video_frame_qa capabilities missing shot_quality_gate: %#v", manifest.Capabilities)
	}
	for _, output := range []string{"shotReports", "shotSpecLints", "repairPlan", "needsRegeneration"} {
		if _, ok := manifest.Output[output]; !ok {
			t.Fatalf("video_frame_qa output missing %s: %#v", output, manifest.Output)
		}
	}
}

func TestRegisterVideoCreationExternalToolsInstallsPreviewReviewManifest(t *testing.T) {
	registry := tool.NewToolRegistry()
	RegisterVideoCreationExternalTools(registry)

	manifest := registry.GetExternalManifest("hyperframes_project_generator")
	if manifest == nil {
		t.Fatal("expected hyperframes_project_generator to be registered")
	}
	if !manifest.ApprovalPolicy.Required || manifest.ApprovalPolicy.Mode != tool.ApprovalAfterArtifact {
		t.Fatalf("preview approval policy = %#v, want after_artifact", manifest.ApprovalPolicy)
	}
	if manifest.HumanReview == nil || !manifest.HumanReview.Required {
		t.Fatalf("preview human review missing: %#v", manifest.HumanReview)
	}
	for _, kind := range []string{"HYPERFRAMES_PROJECT", "PREVIEW_SNAPSHOTS", "PREVIEW_REPORT"} {
		if !containsString(manifest.ArtifactPolicy.ArtifactKinds, kind) {
			t.Fatalf("preview artifact kinds missing %s: %#v", kind, manifest.ArtifactPolicy.ArtifactKinds)
		}
	}
}

func TestRegisterVideoCreationExternalToolsInstallsRenderReviewManifest(t *testing.T) {
	registry := tool.NewToolRegistry()
	RegisterVideoCreationExternalTools(registry)

	manifest := registry.GetExternalManifest("hyperframes_renderer")
	if manifest == nil {
		t.Fatal("expected hyperframes_renderer to be registered")
	}
	if !manifest.ApprovalPolicy.Required || manifest.ApprovalPolicy.Mode != tool.ApprovalBeforeExecute {
		t.Fatalf("render approval policy = %#v, want before_execute", manifest.ApprovalPolicy)
	}
	if manifest.HumanReview == nil || !manifest.HumanReview.Required {
		t.Fatalf("render human review missing: %#v", manifest.HumanReview)
	}
	for _, kind := range []string{"VIDEO", "RENDER_REPORT"} {
		if !containsString(manifest.ArtifactPolicy.ArtifactKinds, kind) {
			t.Fatalf("render artifact kinds missing %s: %#v", kind, manifest.ArtifactPolicy.ArtifactKinds)
		}
	}
	if !manifest.SideEffect {
		t.Fatal("hyperframes_renderer should be marked as a side-effecting local tool")
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

func TestVideoProfileClassifierReturnsTalkingHeadProfile(t *testing.T) {
	result := executeLocalVideoCreationTool("video_profile_classifier", map[string]interface{}{
		"stage":       "profile_selection",
		"route":       "talking_head",
		"deliverable": "publish_pack",
		"brief":       "做一期60秒口播知识视频",
	}, tool.ToolContext{TaskID: "task-profile", NodeID: "profile_exec"})

	if !result.Success {
		t.Fatalf("video_profile_classifier failed: %s", result.Error)
	}
	profile, ok := result.Data["creationProfile"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing creationProfile: %#v", result.Data)
	}
	if profile["profileId"] != "talking_head" {
		t.Fatalf("profileId = %#v", profile["profileId"])
	}
	artifacts, ok := result.Data["artifacts"].([]map[string]interface{})
	if !ok || len(artifacts) == 0 {
		t.Fatalf("profile classifier should create reviewable artifacts")
	}
	if artifacts[0]["kind"] != "VIDEO_CREATION_PROFILE" {
		t.Fatalf("profile artifact kind = %#v", artifacts[0]["kind"])
	}
}

func TestTimeWindowPlannerSplitsCinematicShot(t *testing.T) {
	result := executeLocalVideoCreationTool("time_window_planner", map[string]interface{}{
		"stage": "time_window",
		"creationProfile": map[string]interface{}{
			"profileId": "cinematic_story",
		},
		"shotList": []interface{}{
			map[string]interface{}{
				"shotId":      "SHOT_01",
				"durationSec": float64(40),
				"visual":      "夜晚街道追逐",
				"mainAction":  "角色穿过街道并躲入巷子",
			},
		},
	}, tool.ToolContext{TaskID: "task-time-window", NodeID: "time_window_exec"})

	if !result.Success {
		t.Fatalf("time_window_planner failed: %s", result.Error)
	}
	windows, ok := result.Data["timeWindows"].([]map[string]interface{})
	if !ok || len(windows) != 4 {
		t.Fatalf("expected four time windows, got %#v", result.Data["timeWindows"])
	}
	for _, window := range windows {
		duration := intFromInterface(window["durationSec"], 0)
		if duration < 3 || duration > 15 {
			t.Fatalf("duration outside 3-15s: %#v", window)
		}
	}
	artifacts, ok := result.Data["artifacts"].([]map[string]interface{})
	if !ok || len(artifacts) == 0 {
		t.Fatalf("time window planner should create reviewable artifacts")
	}
	if artifacts[0]["kind"] != "TIME_WINDOW_PLAN" {
		t.Fatalf("time window artifact kind = %#v", artifacts[0]["kind"])
	}
}

func TestTimeWindowPlannerRequiresScriptSpansForTalkingHead(t *testing.T) {
	result := executeLocalVideoCreationTool("time_window_planner", map[string]interface{}{
		"stage": "time_window",
		"creationProfile": map[string]interface{}{
			"profileId": "talking_head",
		},
	}, tool.ToolContext{TaskID: "task-time-window", NodeID: "time_window_exec"})

	if result.Success {
		t.Fatalf("talking-head time_window_planner should fail without scriptSpans: %#v", result.Data)
	}
	if !strings.Contains(result.Error, "scriptSpans") {
		t.Fatalf("failure should clearly mention scriptSpans, got %q", result.Error)
	}
}

func TestTimeWindowPlannerPrefersScriptTextOverRedactedTextField(t *testing.T) {
	result := executeLocalVideoCreationTool("time_window_planner", map[string]interface{}{
		"stage": "time_window",
		"creationProfile": map[string]interface{}{
			"profileId": "talking_head",
		},
		"scriptSpans": []interface{}{
			map[string]interface{}{
				"id":          "SPAN_01",
				"startSec":    float64(0),
				"endSec":      float64(6),
				"scriptText":  "躺营 AI OS 把脚本、分镜和渲染串成一条流水线。",
				"text":        map[string]interface{}{"field": "text", "reason": "USER_ASSET_REDACTED", "redacted": true},
				"durationSec": float64(6),
			},
		},
	}, tool.ToolContext{TaskID: "task-time-window", NodeID: "time_window_exec"})

	if !result.Success {
		t.Fatalf("time_window_planner failed: %s", result.Error)
	}
	windows, ok := result.Data["timeWindows"].([]map[string]interface{})
	if !ok || len(windows) != 1 {
		t.Fatalf("expected one window, got %#v", result.Data["timeWindows"])
	}
	if windows[0]["scriptText"] != "躺营 AI OS 把脚本、分镜和渲染串成一条流水线。" {
		t.Fatalf("redacted text field should not override scriptText: %#v", windows[0])
	}
}

func TestVisualAlignmentPlannerPreservesTimeWindowDuration(t *testing.T) {
	result := executeLocalVideoCreationTool("visual_alignment_planner", map[string]interface{}{
		"stage": "visual_alignment",
		"timeWindows": []interface{}{
			map[string]interface{}{
				"id":           "TW_SHORT",
				"shotId":       "SHOT_SHORT",
				"durationSec":  float64(2),
				"scriptText":   "短转场口播",
				"sceneSummary": "字幕和B-roll补充",
			},
		},
	}, tool.ToolContext{TaskID: "task-visual-alignment", NodeID: "visual_alignment_exec"})

	if !result.Success {
		t.Fatalf("visual_alignment_planner failed: %s", result.Error)
	}
	shotList, ok := result.Data["shotList"].([]map[string]interface{})
	if !ok || len(shotList) != 1 {
		t.Fatalf("expected one visual alignment shot, got %#v", result.Data["shotList"])
	}
	shot := shotList[0]
	if intFromInterface(shot["durationSec"], 0) != 2 {
		t.Fatalf("visual alignment should preserve 2s duration, got %#v", shot)
	}
	if shot["timeWindowId"] != "TW_SHORT" {
		t.Fatalf("expected timeWindowId TW_SHORT, got %#v", shot["timeWindowId"])
	}
	if shot["narrationText"] != "短转场口播" {
		t.Fatalf("expected narrationText from scriptText, got %#v", shot["narrationText"])
	}
	if shot["visual"] != "字幕和B-roll补充" {
		t.Fatalf("expected visual from sceneSummary, got %#v", shot["visual"])
	}
}

func TestVisualAlignmentPlannerDerivesPublicTitleFromNarration(t *testing.T) {
	result := executeLocalVideoCreationTool("visual_alignment_planner", map[string]interface{}{
		"stage": "visual_alignment",
		"timeWindows": []interface{}{
			map[string]interface{}{
				"id":          "TW_01",
				"shotId":      "SHOT_01",
				"durationSec": float64(8),
				"scriptText":  "躺营 AI OS 把选题、脚本、分镜、审核、生成和打包放进一条可追踪流水线，非技术用户也能按步骤推进。",
				"text":        map[string]interface{}{"field": "text", "reason": "USER_ASSET_REDACTED", "redacted": true},
			},
		},
	}, tool.ToolContext{TaskID: "task-visual-alignment", NodeID: "visual_alignment_exec"})

	if !result.Success {
		t.Fatalf("visual_alignment_planner failed: %s", result.Error)
	}
	shotList, ok := result.Data["shotList"].([]map[string]interface{})
	if !ok || len(shotList) != 1 {
		t.Fatalf("expected one visual alignment shot, got %#v", result.Data["shotList"])
	}
	shot := shotList[0]
	if shot["visual"] != "选题到成片，一条流水线" {
		t.Fatalf("expected public visual title, got %#v", shot["visual"])
	}
	visual, _ := shot["visual"].(string)
	if strings.Contains(visual, "围绕该口播时间窗") {
		t.Fatalf("visual title leaked internal placeholder: %#v", shot["visual"])
	}
	if shot["narrationText"] != "躺营 AI OS 把选题、脚本、分镜、审核、生成和打包放进一条可追踪流水线，非技术用户也能按步骤推进。" {
		t.Fatalf("redacted text field should not override narration: %#v", shot["narrationText"])
	}
	if shot["camera"] == "" {
		t.Fatalf("expected viewer-facing camera/subtitle hint, got %#v", shot)
	}
}

func TestCinematicShotDesignerManifestMatchesExecutor(t *testing.T) {
	registry := tool.NewToolRegistry()
	RegisterVideoCreationExternalTools(registry)

	manifest := registry.GetExternalManifest("cinematic_shot_designer")
	if manifest == nil {
		t.Fatal("expected cinematic_shot_designer manifest")
	}
	for _, name := range []string{"timeWindows", "timeWindowPlan", "shotList"} {
		if _, ok := manifest.Parameters[name]; !ok {
			t.Fatalf("cinematic_shot_designer should declare parameter %q: %#v", name, manifest.Parameters)
		}
	}
	for _, name := range []string{"directorDesign", "shotList", "content", "artifacts"} {
		if _, ok := manifest.Output[name]; !ok {
			t.Fatalf("cinematic_shot_designer should declare output %q: %#v", name, manifest.Output)
		}
	}
}

func TestCinematicShotDesignerUsesTimeWindowsWithoutGeneratingMedia(t *testing.T) {
	result := executeLocalVideoCreationTool("cinematic_shot_designer", map[string]interface{}{
		"stage": "cinematic_shot_design",
		"creationProfile": map[string]interface{}{
			"profileId": "cinematic_story",
		},
		"timeWindows": []interface{}{
			map[string]interface{}{
				"id":           "SHOT_01_TW_01",
				"shotId":       "SHOT_01_TW_01",
				"durationSec":  float64(2),
				"sceneSummary": "雨夜街口回头",
				"mainAction":   "角色停下并回望",
			},
		},
	}, tool.ToolContext{TaskID: "task-cinematic", NodeID: "cinematic_shot_design_exec"})

	if !result.Success {
		t.Fatalf("cinematic_shot_designer failed: %s", result.Error)
	}
	design, ok := result.Data["directorDesign"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing directorDesign: %#v", result.Data)
	}
	if design["mediaGenerated"] != false {
		t.Fatalf("cinematic designer must not claim media generation: %#v", design)
	}
	shotList, ok := result.Data["shotList"].([]map[string]interface{})
	if !ok || len(shotList) != 1 {
		t.Fatalf("expected shotList from timeWindows, got %#v", result.Data["shotList"])
	}
	if intFromInterface(shotList[0]["durationSec"], 0) != 2 {
		t.Fatalf("cinematic designer should preserve 2s duration, got %#v", shotList[0])
	}
}

func TestCinematicShotDesignerBuildsFallbackLaunchShotList(t *testing.T) {
	result := executeLocalVideoCreationTool("cinematic_shot_designer", map[string]interface{}{
		"stage":       "cinematic_shot_design",
		"brief":       "制作开源项目上线宣传视频，包含页面录屏、HyperFrames 图形包装和 JiMeng AIGC 素材。",
		"durationSec": 120,
		"creationProfile": map[string]interface{}{
			"profileId": "cinematic_story",
		},
	}, tool.ToolContext{TaskID: "task-cinematic-fallback", NodeID: "cinematic_shot_design_exec"})

	if !result.Success {
		t.Fatalf("cinematic_shot_designer failed: %s", result.Error)
	}
	shotList, ok := result.Data["shotList"].([]map[string]interface{})
	if !ok || len(shotList) < 6 {
		t.Fatalf("expected multi-shot fallback plan, got %#v", result.Data["shotList"])
	}
	routes := map[string]bool{}
	for _, shot := range shotList {
		routes[ensureStringValue(shot["plannedAssetRoute"])] = true
	}
	for _, route := range []string{"aigc_video", "screen_recording", "hyperframes"} {
		if !routes[route] {
			t.Fatalf("fallback shot list should include route %q, got %+v", route, routes)
		}
	}
	if strings.Contains(ensureStringValue(result.Data["content"]), "待导演设计的故事镜头") {
		t.Fatalf("fallback should not expose generic placeholder content: %s", result.Data["content"])
	}
}

func TestSoundDesignPlannerManifestMatchesExecutor(t *testing.T) {
	registry := tool.NewToolRegistry()
	RegisterVideoCreationExternalTools(registry)

	manifest := registry.GetExternalManifest("sound_design_planner")
	if manifest == nil {
		t.Fatal("expected sound_design_planner manifest")
	}
	for _, name := range []string{"timeWindows", "timeWindowPlan", "shotList"} {
		if _, ok := manifest.Parameters[name]; !ok {
			t.Fatalf("sound_design_planner should declare parameter %q: %#v", name, manifest.Parameters)
		}
	}
	for _, name := range []string{"soundDesignPlan", "content", "artifacts"} {
		if _, ok := manifest.Output[name]; !ok {
			t.Fatalf("sound_design_planner should declare output %q: %#v", name, manifest.Output)
		}
	}
}

func TestSoundDesignPlannerUsesTimeWindowsWithoutGeneratingAudio(t *testing.T) {
	result := executeLocalVideoCreationTool("sound_design_planner", map[string]interface{}{
		"stage": "sound_design",
		"timeWindows": []interface{}{
			map[string]interface{}{
				"id":            "TW_SOUND",
				"shotId":        "SHOT_SOUND",
				"durationSec":   float64(2),
				"narrationText": "脚步声渐近",
				"sceneSummary":  "巷口雨声",
			},
		},
	}, tool.ToolContext{TaskID: "task-sound", NodeID: "sound_design_exec"})

	if !result.Success {
		t.Fatalf("sound_design_planner failed: %s", result.Error)
	}
	plan, ok := result.Data["soundDesignPlan"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing soundDesignPlan: %#v", result.Data)
	}
	if plan["mediaGenerated"] != false {
		t.Fatalf("sound designer must not claim audio generation: %#v", plan)
	}
	if intFromInterface(plan["cueCount"], 0) != 1 {
		t.Fatalf("expected one sound cue, got %#v", plan)
	}
	cues, ok := plan["cues"].([]map[string]interface{})
	if !ok || len(cues) != 1 {
		t.Fatalf("expected cue list, got %#v", plan["cues"])
	}
	if intFromInterface(cues[0]["durationSec"], 0) != 2 {
		t.Fatalf("sound planner should preserve 2s duration, got %#v", cues[0])
	}
	if !strings.Contains(ensureStringValue(cues[0]), "不生成音频") {
		t.Fatalf("sound cue should avoid audio-generation claim, got %#v", cues[0])
	}
}

func TestNewsSearchDegradesWhenSearchAPIKeyMissing(t *testing.T) {
	t.Setenv("SEARCH_API_KEY", "")

	result := executeNewsSearch(map[string]interface{}{
		"query": "佛得角 世界杯 小组赛 出线 奇迹",
		"topK":  3,
	})
	if !result.Success {
		t.Fatalf("news_search should not block when SEARCH_API_KEY is missing: %s", result.Error)
	}
	if result.Data["manualRequired"] != true || result.Data["status"] != "manual_required" {
		t.Fatalf("expected manual-required fallback metadata: %#v", result.Data)
	}
	results, ok := result.Data["results"].([]map[string]interface{})
	if !ok || len(results) != 0 {
		t.Fatalf("expected empty search results in manual fallback, got %#v", result.Data["results"])
	}
	instruction, _ := result.Data["userInstruction"].(string)
	if !strings.Contains(instruction, "SEARCH_API_KEY") || !strings.Contains(instruction, "浏览器") {
		t.Fatalf("fallback should instruct browser/manual supplementation, got %q", instruction)
	}
}

func TestVideoScriptGeneratorBlocksRequiredRetrievalWithEmptyFacts(t *testing.T) {
	result := executeLocalVideoCreationTool("video_script_generator", map[string]interface{}{
		"topic":                 "佛得角世界杯出线",
		"retrievalPolicy":       "required",
		"mustUseFreshKnowledge": true,
		"blockOnEmptyFacts":     true,
		"knowledgePack":         []interface{}{},
	}, tool.ToolContext{TaskID: "task-1", NodeID: "script"})

	if result.Success {
		t.Fatalf("expected script generator to fail when required facts are empty: %#v", result.Data)
	}
}

func TestVideoScriptGeneratorFallsBackWhenRequiredRetrievalDoesNotBlockEmptyFacts(t *testing.T) {
	t.Setenv("AIOS_ENABLE_LOCAL_AGENT_MODEL_CONFIG", "")
	SetVideoCreationConfig(config.OpenAIConfig{}, "")
	previousFetcher := localAgentConfigFetcher
	localAgentConfigFetcher = func() (RuntimeModelProviderConfig, bool) {
		return RuntimeModelProviderConfig{}, false
	}
	t.Cleanup(func() {
		localAgentConfigFetcher = previousFetcher
	})
	result := executeLocalVideoCreationTool("video_script_generator", map[string]interface{}{
		"topic":                 "开源项目上线宣传片",
		"retrievalPolicy":       "required",
		"mustUseFreshKnowledge": true,
		"knowledgePack":         []interface{}{},
	}, tool.ToolContext{TaskID: "task-1", NodeID: "script"})

	if !result.Success {
		t.Fatalf("expected script generator to fall back when empty facts do not block: %s", result.Error)
	}
	if result.Data["script"] == "" {
		t.Fatalf("fallback should return a usable script: %#v", result.Data)
	}
}

func TestVideoScriptGeneratorConsumesKnowledgeContextWithoutModelFallback(t *testing.T) {
	t.Setenv("AIOS_ENABLE_LOCAL_AGENT_MODEL_CONFIG", "")
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

func TestVideoScriptGeneratorNoKeyFallbackExposesScriptField(t *testing.T) {
	t.Setenv("AIOS_ENABLE_LOCAL_AGENT_MODEL_CONFIG", "")
	SetVideoCreationConfig(config.OpenAIConfig{}, "")
	ClearRuntimeModelProviderConfig()
	previousFetcher := localAgentConfigFetcher
	localAgentConfigFetcher = func() (RuntimeModelProviderConfig, bool) {
		return RuntimeModelProviderConfig{}, false
	}
	t.Cleanup(func() {
		SetVideoCreationConfig(config.OpenAIConfig{}, "")
		ClearRuntimeModelProviderConfig()
		localAgentConfigFetcher = previousFetcher
	})

	result := executeLocalVideoCreationTool("video_script_generator", map[string]interface{}{
		"topic": "30秒躺营 AI OS 开源宣传",
	}, tool.ToolContext{TaskID: "task-script-fallback", NodeID: "script_generation"})

	if !result.Success {
		t.Fatalf("expected script generator fallback to succeed: %s", result.Error)
	}
	script, _ := result.Data["script"].(string)
	if !strings.Contains(script, "躺营 AI OS") {
		t.Fatalf("fallback should expose script output for downstream nodes, got %#v", result.Data["script"])
	}
	if result.Data["estimatedDurationSec"] != 30 {
		t.Fatalf("fallback should infer 30 second duration from topic, got %#v", result.Data["estimatedDurationSec"])
	}
	pkg, ok := result.Data["package"].(map[string]interface{})
	if !ok || pkg["script"] != script {
		t.Fatalf("fallback package should preserve script for artifact review: %#v", result.Data["package"])
	}
	spans, ok := result.Data["scriptSpans"].([]map[string]interface{})
	if !ok || len(spans) == 0 {
		t.Fatalf("fallback should expose scriptSpans for time-window planning: %#v", result.Data["scriptSpans"])
	}
	if spans[0]["startSec"] != 0 {
		t.Fatalf("first span should start at zero: %#v", spans[0])
	}
	if spans[len(spans)-1]["endSec"] != 30 {
		t.Fatalf("last span should end at inferred target duration: %#v", spans[len(spans)-1])
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
		"全局一致性资产包",
		"每个 shot 时长 3-15 秒",
		"每个 shot 的素材包必须完全独立",
		"narrationText",
		"materialLibraryHints",
		"referenceRequirements",
		"expectedArtifacts",
		"reviewPacket",
		"shotQueue",
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
	llmCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmCalled = true
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
	if llmCalled {
		t.Fatalf("video_prompt_generator should build browser/manual requests locally when shotList is present")
	}
	packages, ok := result.Data["shotAssetPackages"].([]interface{})
	if !ok || len(packages) != 1 {
		t.Fatalf("expected shotAssetPackages to be exposed from structured output, got %#v", result.Data["shotAssetPackages"])
	}
	requests, ok := result.Data["externalGenerationRequests"].([]interface{})
	if !ok || len(requests) != 1 {
		t.Fatalf("expected externalGenerationRequests to be exposed, got %#v", result.Data["externalGenerationRequests"])
	}
	if !strings.Contains(ensureStringValue(requests[0]), "佛得角是西非岛国") {
		t.Fatalf("external generation request should include shot narration, got %#v", requests[0])
	}
}

func TestVideoPromptGeneratorOnlySubmitsAIGCShotsToExternalGeneration(t *testing.T) {
	result := executeDynamicAgentPromptTool("video_prompt_generator", "video_prompt", "video", strings.Repeat("开源项目宣传 ", 300), "", map[string]interface{}{
		"topic": strings.Repeat("开源项目宣传 ", 300),
		"shotList": []interface{}{
			map[string]interface{}{
				"shotId":            "SHOT_01",
				"durationSec":       6,
				"plannedAssetRoute": "aigc_video",
				"visual":            "AIGC_VIDEO | 非真人电影感，云端编排和本地执行器连接。",
				"narrationText":     "真正可控的视频生产线来了。",
			},
			map[string]interface{}{
				"shotId":            "SHOT_02",
				"durationSec":       8,
				"plannedAssetRoute": "screen_recording",
				"visual":            "SCREEN_RECORDING | 真实页面录屏：登录、输入项目、审核门通过。",
				"narrationText":     "真实页面演示系统完整流程。",
			},
			map[string]interface{}{
				"shotId":            "SHOT_03",
				"durationSec":       8,
				"plannedAssetRoute": "hyperframes",
				"visual":            "HYPERFRAMES | 流程图和数据卡片展示 README、Wiki、release tag。",
				"narrationText":     "版本管理会写进开源文档。",
			},
		},
	}, tool.ToolContext{TaskID: "task-video-prompt-routes", NodeID: "video_prompt_exec"})

	if !result.Success {
		t.Fatalf("expected video prompt generation to succeed: %s", result.Error)
	}
	requests, ok := result.Data["externalGenerationRequests"].([]interface{})
	if !ok || len(requests) != 1 {
		t.Fatalf("expected only the AIGC shot to create an external request, got %#v", result.Data["externalGenerationRequests"])
	}
	req, ok := requests[0].(map[string]interface{})
	if !ok {
		t.Fatalf("request should be a map, got %#v", requests[0])
	}
	if req["shotId"] != "SHOT_01" {
		t.Fatalf("unexpected request shot: %#v", req)
	}
	if got := len([]rune(ensureStringValue(req["prompt"]))); got > 2000 {
		t.Fatalf("request prompt should be capped at 2000 runes, got %d", got)
	}
	packages, ok := result.Data["shotAssetPackages"].([]interface{})
	if !ok || len(packages) != 3 {
		t.Fatalf("all shots should still have packages, got %#v", result.Data["shotAssetPackages"])
	}
}

func TestVideoPromptGeneratorUsesTimeWindowSceneSummaryForAIGCRouting(t *testing.T) {
	result := executeDynamicAgentPromptTool("video_prompt_generator", "video_prompt", "video", "躺营 AI OS 开源宣传", "", map[string]interface{}{
		"topic": "躺营 AI OS 开源宣传\n\n创作要求：包含录屏、HyperFrames 和即梦 AIGC。",
		"shotList": []interface{}{
			map[string]interface{}{
				"shotId":          "SHOT_01_TW_01",
				"durationSec":     6,
				"recommendedMode": "aigc_video",
				"sceneSummary":    "AIGC_VIDEO | 非真人风格化：云端编排和本地执行器连接。",
				"mainAction":      "突出 MCP 可扩展。",
			},
			map[string]interface{}{
				"shotId":          "SHOT_02_TW_01",
				"durationSec":     8,
				"recommendedMode": "aigc_video",
				"sceneSummary":    "SCREEN_RECORDING | 真实页面录屏：输入项目、审核门通过。",
				"mainAction":      "展示真实页面流程。",
			},
			map[string]interface{}{
				"shotId":          "SHOT_03_TW_01",
				"durationSec":     7,
				"recommendedMode": "aigc_video",
				"sceneSummary":    "HYPERFRAMES | 数据卡片：README、Wiki、release tag。",
				"mainAction":      "展示版本管理。",
			},
		},
	}, tool.ToolContext{TaskID: "task-video-prompt-time-window-routes", NodeID: "video_prompt_exec"})

	if !result.Success {
		t.Fatalf("expected video prompt generation to succeed: %s", result.Error)
	}
	requests, ok := result.Data["externalGenerationRequests"].([]interface{})
	if !ok || len(requests) != 1 {
		t.Fatalf("expected only sceneSummary AIGC window to create request, got %#v", result.Data["externalGenerationRequests"])
	}
	req, _ := requests[0].(map[string]interface{})
	if req["shotId"] != "SHOT_01_TW_01" {
		t.Fatalf("unexpected request: %#v", req)
	}
	prompt := ensureStringValue(req["promptText"])
	if strings.Contains(prompt, "创作要求：") {
		t.Fatalf("prompt should use compact topic, got %s", prompt)
	}
	if !strings.Contains(prompt, "云端编排和本地执行器连接") {
		t.Fatalf("prompt should preserve sceneSummary, got %s", prompt)
	}
}

func TestExecuteDynamicAgentPromptToolSplitsShotsLocally(t *testing.T) {
	llmCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	SetVideoCreationConfig(config.OpenAIConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "deepseek-v4-pro",
		Timeout: 5,
	}, "")
	previousFetcher := localAgentConfigFetcher
	localAgentConfigFetcher = func() (RuntimeModelProviderConfig, bool) {
		return RuntimeModelProviderConfig{}, false
	}
	t.Cleanup(func() {
		SetVideoCreationConfig(config.OpenAIConfig{}, "")
		localAgentConfigFetcher = previousFetcher
	})

	result := executeDynamicAgentPromptTool("shot_splitter", "beat_plan", "video", "佛得角世界杯出线", "", map[string]interface{}{
		"topic":             "佛得角世界杯出线",
		"targetDurationSec": 45,
		"script":            "佛得角是西非大西洋上的岛国。它人口不多，却有鲜明的克里奥尔文化。足球让这个国家被更多人看见。世界杯小组赛出线进入淘汰赛，对这样的小国来说是巨大的奇迹。",
	}, tool.ToolContext{TaskID: "task-shot-split", NodeID: "beat_plan_exec"})

	if !result.Success {
		t.Fatalf("expected local shot split to succeed: %s", result.Error)
	}
	if llmCalled {
		t.Fatalf("shot_splitter should split locally when script is present")
	}
	shots, ok := result.Data["shotList"].([]interface{})
	if !ok || len(shots) == 0 {
		t.Fatalf("expected shotList, got %#v", result.Data["shotList"])
	}
	for _, item := range shots {
		shot, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("shot should be map, got %#v", item)
		}
		duration := intFromInterface(shot["durationSec"], 0)
		if duration < 3 || duration > 15 {
			t.Fatalf("shot duration should be 3-15s, got %#v", shot)
		}
		if !strings.Contains(ensureStringValue(shot), "crossShotDependencyForbidden") {
			t.Fatalf("shot should declare independence, got %#v", shot)
		}
	}
	packages, ok := result.Data["shotAssetPackages"].([]interface{})
	if !ok || len(packages) != len(shots) {
		t.Fatalf("expected per-shot packages, got %#v", result.Data["shotAssetPackages"])
	}
	content := ensureStringValue(result.Data["content"])
	if strings.HasPrefix(strings.TrimSpace(content), "{") || strings.Contains(content, `"shotAssetPackages"`) {
		t.Fatalf("shot splitter review content should be human-readable, got %s", content)
	}
	for _, required := range []string{"分镜队列", "当前先审核", "SHOT_01"} {
		if !strings.Contains(content, required) {
			t.Fatalf("shot splitter review content should contain %q, got %s", required, content)
		}
	}
}

func TestHyperFramesShotFirstFallbackUsesShotSectionsAndNarration(t *testing.T) {
	shotList := `{"shotList":[{"shotId":"SHOT_01","durationSec":6,"scriptText":"佛得角是一个西非岛国。","visual":"地图上出现佛得角群岛","camera":"缓慢推近"},{"shotId":"SHOT_02","durationSec":7,"narrationText":"世界杯出线对它来说是奇迹。","visual":"球场灯光亮起","camera":"横向移动"}]}`
	html := buildMinimalHyperFramesHTML("佛得角奇迹", "完整口播", shotList, "16:9")
	for _, required := range []string{
		`data-composition-id="main"`,
		`data-duration="13"`,
		`class="scene clip shot-review-packet`,
		`data-start="0"`,
		`data-start="6"`,
		`data-shot-id="SHOT_01"`,
		`data-shot-id="SHOT_02"`,
		`window.__timelines["main"]`,
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

func TestHyperFramesDataJSONSanitizesInternalPlaceholders(t *testing.T) {
	videoPrompts := `[{"shotId":"SHOT_01","narrationText":"开场口播","prompt":{"field":"prompt","reason":"USER_ASSET_REDACTED","redacted":true}}]`
	data := buildHyperFramesDataJSON("宣传片", "完整口播", `[{"shotId":"SHOT_01"}]`, videoPrompts, `{{mcp_generation.output.shotAssetPackages}}`, "16:9", "{}")
	if strings.Contains(data, "USER_ASSET_REDACTED") || strings.Contains(data, "{{") {
		t.Fatalf("hyperframes data json should not leak internal placeholders, got:\n%s", data)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(data), &decoded); err != nil {
		t.Fatalf("hyperframes data json should remain valid JSON: %v\n%s", err, data)
	}
	prompts, ok := decoded["videoPrompts"].([]interface{})
	if !ok || len(prompts) != 1 {
		t.Fatalf("expected one video prompt, got %#v", decoded["videoPrompts"])
	}
	prompt, ok := prompts[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected prompt map, got %#v", prompts[0])
	}
	if _, exists := prompt["prompt"]; exists {
		t.Fatalf("redacted prompt field should be removed, got %#v", prompt)
	}
	packages, ok := decoded["shotAssetPackages"].([]interface{})
	if !ok || len(packages) != 0 {
		t.Fatalf("unresolved shot asset packages template should fall back to empty array, got %#v", decoded["shotAssetPackages"])
	}
}

func TestHyperFramesRendererDisabledReturnsManualVideoArtifact(t *testing.T) {
	SetHyperFramesConfig(hyperframes.Config{Mode: hyperframes.ModeDisabled})
	t.Cleanup(func() {
		SetHyperFramesConfig(hyperframes.Config{})
	})

	result := executeLocalVideoCreationTool("hyperframes_renderer", map[string]interface{}{
		"projectDir": "projects/task-1-hyperframes",
	}, tool.ToolContext{TaskID: "task-1", NodeID: "render_exec"})

	if !result.Success {
		t.Fatalf("disabled renderer should return manual upload guidance, got error: %s", result.Error)
	}
	if result.Data["outputPath"] == "" {
		t.Fatalf("disabled renderer should expose a placeholder outputPath, got %#v", result.Data)
	}
	artifacts, ok := result.Data["artifacts"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected render artifacts, got %#v", result.Data["artifacts"])
	}
	hasVideo := false
	for _, artifact := range artifacts {
		if artifact["kind"] == "VIDEO" {
			hasVideo = true
		}
	}
	if !hasVideo {
		t.Fatalf("disabled renderer should emit a VIDEO artifact for manual upload, got %#v", artifacts)
	}
}

func TestHyperFramesLayoutLLMRequiresExplicitOptIn(t *testing.T) {
	if shouldUseLLMHyperFramesLayout(map[string]interface{}{}) {
		t.Fatal("HyperFrames layout generation should not call LLM by default")
	}
	if !shouldUseLLMHyperFramesLayout(map[string]interface{}{"useLLMLayout": true}) {
		t.Fatal("useLLMLayout=true should opt into LLM layout generation")
	}
	if !shouldUseLLMHyperFramesLayout(map[string]interface{}{"use_llm_layout": "yes"}) {
		t.Fatal("use_llm_layout=yes should opt into LLM layout generation")
	}
}

func TestResolveHyperFramesProjectRootUsesWritablePreferred(t *testing.T) {
	preferred := t.TempDir()
	if got := resolveHyperFramesProjectRoot(preferred); got != preferred {
		t.Fatalf("expected writable preferred root, got %q", got)
	}
}

func TestResolveHyperFramesProjectRootFallsBackWhenPreferredIsNotDirectory(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	got := resolveHyperFramesProjectRoot(filePath)
	if got == filePath {
		t.Fatalf("expected fallback root, got preferred file path %q", got)
	}
	if !strings.Contains(got, "hyperframes-projects") {
		t.Fatalf("fallback root should be hyperframes project cache, got %q", got)
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
		"modelProvider": map[string]interface{}{
			"baseUrl": server.URL,
			"apiKey":  "test-key",
			"model":   "deepseek-v4-pro",
		},
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

func TestShotGenerationPlannerRoutesHybridAndExternalNeeds(t *testing.T) {
	result := executeLocalVideoCreationTool("shot_generation_planner", map[string]interface{}{
		"stage": "generation_strategy",
		"shotList": []interface{}{
			map[string]interface{}{
				"shotId":        "SHOT_01",
				"durationSec":   float64(6),
				"visual":        "非真人风格化办公室里人物被文件包围，同时画面必须显示“几个表格”",
				"narrationText": "几个表格就能改变判断。",
				"screenText":    []interface{}{"几个表格"},
			},
			map[string]interface{}{
				"shotId":      "SHOT_02",
				"durationSec": float64(5),
				"visual":      "展示客户上传 logo 和产品截图",
			},
		},
		"aigcAvailable": true,
		"htmlAvailable": true,
	}, tool.ToolContext{TaskID: "task-generation-plan", NodeID: "shot_generation_planner_exec"})

	if !result.Success {
		t.Fatalf("shot_generation_planner failed: %s", result.Error)
	}
	plans, ok := result.Data["shotGenerationPlans"].([]map[string]interface{})
	if !ok || len(plans) != 2 {
		t.Fatalf("expected two shotGenerationPlans, got %#v", result.Data["shotGenerationPlans"])
	}
	if plans[0]["mode"] != "hybrid_aigc_bg_html_overlay" {
		t.Fatalf("SHOT_01 should route to hybrid mode, got %#v", plans[0])
	}
	if plans[1]["mode"] != "external_or_user_asset" {
		t.Fatalf("SHOT_02 should route to external/user asset mode, got %#v", plans[1])
	}
	packages, ok := result.Data["shotAssetPackages"].([]map[string]interface{})
	if !ok || len(packages) != 2 {
		t.Fatalf("expected shotAssetPackages, got %#v", result.Data["shotAssetPackages"])
	}
}

func TestShotGenerationPlannerMissingProviderCreatesExternalRequest(t *testing.T) {
	result := executeLocalVideoCreationTool("shot_generation_planner", map[string]interface{}{
		"stage": "generation_strategy",
		"shotList": []interface{}{
			map[string]interface{}{
				"shotId":      "SHOT_01",
				"durationSec": float64(6),
				"visual":      "人物跑过街口，镜头跟随，电影感运动",
			},
		},
		"aigcAvailable": false,
		"htmlAvailable": true,
	}, tool.ToolContext{TaskID: "task-generation-plan", NodeID: "shot_generation_planner_exec"})

	if !result.Success {
		t.Fatalf("shot_generation_planner failed: %s", result.Error)
	}
	requests, ok := result.Data["externalGenerationRequests"].([]map[string]interface{})
	if !ok || len(requests) != 1 {
		t.Fatalf("expected one externalGenerationRequest, got %#v", result.Data["externalGenerationRequests"])
	}
	if requests[0]["kind"] != "video" || requests[0]["shotId"] != "SHOT_01" {
		t.Fatalf("unexpected external generation request: %#v", requests[0])
	}
	if strings.TrimSpace(ensureStringValue(requests[0]["requestId"])) == "" {
		t.Fatalf("external generation request should include requestId: %#v", requests[0])
	}
	if strings.TrimSpace(ensureStringValue(requests[0]["prompt"])) == "" {
		t.Fatalf("external generation request should include prompt: %#v", requests[0])
	}
	target, ok := requests[0]["target"].(map[string]interface{})
	if !ok {
		t.Fatalf("external generation request should include target: %#v", requests[0])
	}
	if intFromInterface(target["durationSec"], 0) != 6 {
		t.Fatalf("external generation request target should preserve durationSec=6, got %#v", target)
	}
}

func TestShotGenerationPlannerExternalRequestIncludesReferencesAndDelivery(t *testing.T) {
	result := executeLocalVideoCreationTool("shot_generation_planner", map[string]interface{}{
		"stage": "generation_strategy",
		"shotList": []interface{}{
			map[string]interface{}{
				"shotId":       "SHOT_01_TW_01",
				"timeWindowId": "TW_01",
				"durationSec":  float64(8),
				"visual":       "角色在雨夜街口回头，镜头缓慢靠近",
				"referenceImages": []interface{}{
					map[string]interface{}{"role": "character_reference", "storageRef": "local://projects/p/artifacts/char/hash/char.png"},
					map[string]interface{}{"role": "scene_reference", "storageRef": "local://projects/p/artifacts/street/hash/street.png"},
				},
			},
		},
		"aigcAvailable": false,
		"htmlAvailable": true,
	}, tool.ToolContext{TaskID: "task-generation-plan", NodeID: "shot_generation_planner_exec"})

	if !result.Success {
		t.Fatalf("shot_generation_planner failed: %s", result.Error)
	}
	requests, ok := result.Data["externalGenerationRequests"].([]map[string]interface{})
	if !ok || len(requests) != 1 {
		t.Fatalf("expected one external request, got %#v", result.Data["externalGenerationRequests"])
	}
	req := requests[0]
	if intFromInterface(req["durationSec"], 0) != 8 {
		t.Fatalf("request should expose durationSec=8: %#v", req)
	}
	if req["directApiEligible"] != false {
		t.Fatalf("direct API should be false when AIGC provider unavailable: %#v", req)
	}
	if req["manualUploadRequired"] != true {
		t.Fatalf("manual upload should be required: %#v", req)
	}
	refs, ok := req["referenceImages"].([]interface{})
	if !ok || len(refs) != 2 {
		t.Fatalf("reference images should be preserved: %#v", req["referenceImages"])
	}
	compatRefs, ok := req["references"].([]interface{})
	if !ok || len(compatRefs) != 2 {
		t.Fatalf("request-level references should be preserved for consumers: %#v", req["references"])
	}
	promptPackageText := strings.TrimSpace(ensureStringValue(req["promptPackage"]))
	if promptPackageText == "" {
		t.Fatalf("promptPackage should be copyable for external clients: %#v", req)
	}
	var promptPackage map[string]interface{}
	if err := json.Unmarshal([]byte(promptPackageText), &promptPackage); err != nil {
		t.Fatalf("promptPackage should be JSON: %v, raw=%s", err, promptPackageText)
	}
	if strings.TrimSpace(ensureStringValue(promptPackage["prompt"])) == "" {
		t.Fatalf("promptPackage should include prompt: %#v", promptPackage)
	}
	if intFromInterface(promptPackage["duration"], 0) != 8 {
		t.Fatalf("promptPackage should include duration=8: %#v", promptPackage)
	}
	promptRefs, ok := promptPackage["references"].([]interface{})
	if !ok || len(promptRefs) != 2 {
		t.Fatalf("promptPackage should include references: %#v", promptPackage)
	}
	packages, ok := result.Data["shotAssetPackages"].([]map[string]interface{})
	if !ok || len(packages) != 1 {
		t.Fatalf("expected one shot asset package, got %#v", result.Data["shotAssetPackages"])
	}
	packageRefs, ok := packages[0]["referenceImages"].([]interface{})
	if !ok || len(packageRefs) != 2 {
		t.Fatalf("shot asset package should expose referenceImages at top level: %#v", packages[0])
	}
	if packages[0]["timeWindowId"] != "TW_01" {
		t.Fatalf("shot asset package should expose timeWindowId at top level: %#v", packages[0])
	}
}

func TestShotGenerationPlannerPreservesReferencesWhenVisualPlanLacksReferenceImages(t *testing.T) {
	result := executeLocalVideoCreationTool("shot_generation_planner", map[string]interface{}{
		"stage": "generation_strategy",
		"shotList": []interface{}{
			map[string]interface{}{
				"shotId":      "SHOT_REF_ONLY",
				"durationSec": float64(8),
				"visual":      "角色在雨夜街口回头，镜头缓慢靠近",
				"references": []interface{}{
					map[string]interface{}{"role": "character_reference", "storageRef": "local://projects/p/artifacts/char/hash/char.png"},
					map[string]interface{}{"role": "scene_reference", "storageRef": "local://projects/p/artifacts/street/hash/street.png"},
				},
			},
		},
		"visualPlans": []interface{}{
			map[string]interface{}{
				"shotId":     "SHOT_REF_ONLY",
				"background": map[string]interface{}{"description": "雨夜街口", "requiresAigc": true},
				"cameraPlan": map[string]interface{}{"description": "镜头缓慢靠近", "movement": "tracking shot", "requiresAigc": true},
			},
		},
		"aigcAvailable": false,
		"htmlAvailable": true,
	}, tool.ToolContext{TaskID: "task-generation-plan", NodeID: "shot_generation_planner_exec"})

	if !result.Success {
		t.Fatalf("shot_generation_planner failed: %s", result.Error)
	}
	requests, ok := result.Data["externalGenerationRequests"].([]map[string]interface{})
	if !ok || len(requests) != 1 {
		t.Fatalf("expected one external request, got %#v", result.Data["externalGenerationRequests"])
	}
	reqRefs, ok := requests[0]["references"].([]interface{})
	if !ok || len(reqRefs) != 2 {
		t.Fatalf("request references should survive visualPlans merge: %#v", requests[0])
	}
	reqReferenceImages, ok := requests[0]["referenceImages"].([]interface{})
	if !ok || len(reqReferenceImages) != 2 {
		t.Fatalf("request referenceImages should survive visualPlans merge: %#v", requests[0])
	}
	packages, ok := result.Data["shotAssetPackages"].([]map[string]interface{})
	if !ok || len(packages) != 1 {
		t.Fatalf("expected one shot asset package, got %#v", result.Data["shotAssetPackages"])
	}
	packageRefs, ok := packages[0]["references"].([]interface{})
	if !ok || len(packageRefs) != 2 {
		t.Fatalf("package references should survive visualPlans merge: %#v", packages[0])
	}
	packageReferenceImages, ok := packages[0]["referenceImages"].([]interface{})
	if !ok || len(packageReferenceImages) != 2 {
		t.Fatalf("package referenceImages should survive visualPlans merge: %#v", packages[0])
	}
	plans, ok := result.Data["shotGenerationPlans"].([]map[string]interface{})
	if !ok || len(plans) != 1 {
		t.Fatalf("expected one shot generation plan, got %#v", result.Data["shotGenerationPlans"])
	}
	renderInputs, ok := plans[0]["renderInputs"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected renderInputs, got %#v", plans[0])
	}
	renderRefs, ok := renderInputs["referenceImages"].([]interface{})
	if !ok || len(renderRefs) != 2 {
		t.Fatalf("renderInputs should retain references as referenceImages: %#v", renderInputs)
	}
	renderCompatRefs, ok := renderInputs["references"].([]interface{})
	if !ok || len(renderCompatRefs) != 2 {
		t.Fatalf("renderInputs should retain references: %#v", renderInputs)
	}
}

func TestShotGenerationPlannerRenderPreferenceDisablesHybrid(t *testing.T) {
	result := executeLocalVideoCreationTool("shot_generation_planner", map[string]interface{}{
		"stage": "generation_strategy",
		"shotList": []interface{}{
			map[string]interface{}{
				"shotId":      "SHOT_01",
				"durationSec": float64(6),
				"visual":      "办公室人物指向屏幕，同时画面必须显示“增长42%”",
				"screenText":  []interface{}{"增长42%"},
			},
		},
		"renderPreference": map[string]interface{}{
			"allowHybridRender": false,
		},
		"aigcAvailable": true,
		"htmlAvailable": true,
	}, tool.ToolContext{TaskID: "task-generation-plan", NodeID: "shot_generation_planner_exec"})

	if !result.Success {
		t.Fatalf("shot_generation_planner failed: %s", result.Error)
	}
	plans, ok := result.Data["shotGenerationPlans"].([]map[string]interface{})
	if !ok || len(plans) != 1 {
		t.Fatalf("expected one shotGenerationPlan, got %#v", result.Data["shotGenerationPlans"])
	}
	if plans[0]["mode"] != "placeholder_preview" {
		t.Fatalf("allowHybridRender=false should force placeholder_preview, got %#v", plans[0])
	}
}

func TestShotGenerationPlannerParsesRenderPreferenceJSONString(t *testing.T) {
	result := executeLocalVideoCreationTool("shot_generation_planner", map[string]interface{}{
		"stage": "generation_strategy",
		"shotList": []interface{}{
			map[string]interface{}{
				"shotId":      "SHOT_01",
				"durationSec": float64(6),
				"visual":      "办公室人物指向屏幕，同时画面必须显示“增长42%”",
				"screenText":  []interface{}{"增长42%"},
			},
		},
		"renderPreference": `{"allowHybridRender":false}`,
		"aigcAvailable":    true,
		"htmlAvailable":    true,
	}, tool.ToolContext{TaskID: "task-generation-plan", NodeID: "shot_generation_planner_exec"})

	if !result.Success {
		t.Fatalf("shot_generation_planner failed: %s", result.Error)
	}
	plans, ok := result.Data["shotGenerationPlans"].([]map[string]interface{})
	if !ok || len(plans) != 1 {
		t.Fatalf("expected one shotGenerationPlan, got %#v", result.Data["shotGenerationPlans"])
	}
	if plans[0]["mode"] != "placeholder_preview" {
		t.Fatalf("JSON renderPreference should disable hybrid, got %#v", plans[0])
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

func TestTextImageToVideoGeneratorRequiresClientProvider(t *testing.T) {
	result := executeLocalVideoCreationTool("text_image_to_video_generator", map[string]interface{}{
		"stage":    "render",
		"prompt":   "生成一段佛得角世界杯奇迹的动态图文视频",
		"imageUrl": "local://projects/vp-1/keyframes/shot_001.png",
	}, tool.ToolContext{TaskID: "task-video", NodeID: "text_image_to_video_generator_exec"})

	if result.Success {
		t.Fatalf("text_image_to_video_generator should stop without client provider: %#v", result.Data)
	}
	if !strings.Contains(result.Error, "文生视频 Provider") {
		t.Fatalf("expected provider configuration error, got %q", result.Error)
	}
}

func TestOneSentenceChainStopsAtMissingVideoProvider(t *testing.T) {
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
	if videoResult.Success {
		t.Fatalf("video generator should stop without client provider: %#v", videoResult.Data)
	}
	if !strings.Contains(videoResult.Error, "文生视频 Provider") {
		t.Fatalf("expected provider configuration error, got %q", videoResult.Error)
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

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
