package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/common/crypto"
	"github.com/tangying-ai/aios-core/internal/core/common/jsonx"
	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/hyperframes"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

var videoCreationExternalTools = []string{
	"skill_stage_agent",
	"image_asset_generator",
	"hyperframes_project_builder",
	"hyperframes_renderer",
	"hypergen_keyframes",
	"material_library_matcher",
	"video_keyframe_prompt_builder",
	"storyboard_assembler",
	"text_image_to_video_generator",
	"video_final_assembler",
	"video_shot_extractor",
	"shot_motion_analyzer",
	"asset_guard",
	"material_library_importer",
	"voice_post_process",
	"audio_artifact_packager",
	// Dynamic agent prompt_tool entries — used by LLMPlanner for video creation workflows.
	"knowledge_researcher",
	"fact_checker",
	"video_script_generator",
	"shot_splitter",
	"keyframe_prompt_generator",
	"video_prompt_generator",
	"script_quality_checker",
	"shot_quality_checker",
	"video_prompt_quality_checker",
	"publish_copy_generator",
	"video_package_exporter",
	"package_quality_checker",
}

var (
	videoCreationOpenAICfg config.OpenAIConfig
	videoCreationSkillRoot string
	modelGateway           *modelgateway.Gateway
)

// SetModelGateway stores the model gateway for image generation tools.
func SetModelGateway(gw *modelgateway.Gateway) {
	modelGateway = gw
}

// SetVideoCreationConfig stores configuration needed by local video creation
// tools so skill_stage_agent can call the LLM API.
func SetVideoCreationConfig(cfg config.OpenAIConfig, skillRoot string) {
	videoCreationOpenAICfg = cfg
	videoCreationSkillRoot = skillRoot
}

// GetEnvOpenAIConfig returns the env-based OpenAI config (for display purposes).
func GetEnvOpenAIConfig() config.OpenAIConfig {
	return videoCreationOpenAICfg
}

// GetVideoCreationOpenAIConfig returns the effective OpenAI config by merging
// the env-based config with persisted runtime overrides synced from the frontend.
// Runtime BaseURL/Model/APIKey take priority; env values serve as fallbacks.
func GetVideoCreationOpenAIConfig() config.OpenAIConfig {
	cfg := videoCreationOpenAICfg // env defaults
	runtime := GetRuntimeModelProviderConfig()
	if runtime.BaseURL != "" {
		cfg.BaseURL = runtime.BaseURL
	}
	if runtime.APIKey != "" {
		cfg.APIKey = runtime.APIKey
	}
	if runtime.Model != "" {
		cfg.Model = runtime.Model
	}
	return cfg
}

// --- HyperFrames CLI support (deprecated, use service mode below) ---

var hyperFramesCLIPath string

// SetHyperFramesCLIPath sets the path to the HyperFrames CLI binary.
// Deprecated: use SetHyperFramesConfig for service mode instead.
func SetHyperFramesCLIPath(path string) {
	hyperFramesCLIPath = path
}

// detectHyperFramesCLI checks whether a HyperFrames CLI is available.
// Returns the resolved command path and whether launch was attempted.
func detectHyperFramesCLI() (command string, found bool) {
	if hyperFramesCLIPath != "" {
		if _, err := os.Stat(hyperFramesCLIPath); err == nil {
			zap.L().Info("HyperFrames CLI found via config", zap.String("path", hyperFramesCLIPath))
			return hyperFramesCLIPath, true
		}
		zap.L().Warn("HyperFrames CLI path configured but not found on disk",
			zap.String("path", hyperFramesCLIPath))
	}
	// Try to find it in PATH
	for _, candidate := range []string{"hyperframes", "npx"} {
		if p, err := exec.LookPath(candidate); err == nil {
			if candidate == "npx" {
				zap.L().Info("HyperFrames CLI found via npx", zap.String("path", p))
				return "npx", true
			}
			zap.L().Info("HyperFrames CLI found in PATH", zap.String("path", p))
			return p, true
		}
	}
	zap.L().Info("HyperFrames CLI not found — tools will return guidance output")
	return "", false
}

// hyperFramesAvailable reports whether CLI is detected for build/render stages.
func hyperFramesAvailable() bool {
	_, found := detectHyperFramesCLI()
	return found
}

// --- HyperFrames Render Service (replaces CLI) ---

var (
	hyperFramesConfig hyperframes.Config
	hyperFramesClient *hyperframes.Client
)

// SetHyperFramesConfig stores the HyperFrames service configuration and
// initializes the HTTP client. When Mode is "service", the renderer and
// project builder tools will call the Render Service HTTP API instead of
// shelling out to npx.
func SetHyperFramesConfig(cfg hyperframes.Config) {
	hyperFramesConfig = cfg
	if cfg.IsEnabled() {
		hyperFramesClient = hyperframes.NewClientFromConfig(cfg)
		zap.L().Info("HyperFrames Render Service client initialized",
			zap.String("serviceURL", cfg.ServiceURL),
			zap.String("mode", string(cfg.Mode)),
			zap.Int("timeoutSec", cfg.TimeoutSec))
	} else {
		hyperFramesClient = nil
		zap.L().Info("HyperFrames Render Service is disabled",
			zap.String("mode", string(cfg.Mode)))
	}
}

// hyperFramesServiceAvailable reports whether the Render Service client is ready.
func hyperFramesServiceAvailable() bool {
	return hyperFramesClient != nil && hyperFramesConfig.IsEnabled()
}

// RuntimeModelProviderConfig holds model-provider settings synced from the
// frontend Desktop page at runtime (overrides env-based config when set).
type RuntimeModelProviderConfig struct {
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
}

var (
	runtimeConfigMu            sync.RWMutex
	runtimeModelProviderConfig RuntimeModelProviderConfig
	runtimeConfigPersistPath   string
	runtimeConfigLoaded        bool
	encryptionKey              []byte // 32-byte AES-256 key derived from secret
)

// onDiskConfig is the persisted format — APIKey is encrypted at rest.
type onDiskConfig struct {
	BaseURL      string `json:"baseUrl"`
	APIKeyCipher string `json:"apiKeyCipher"` // AES-256-GCM + base64
	Model        string `json:"model"`
}

// SetRuntimeConfigPersistPath sets the file path for persisting runtime config.
func SetRuntimeConfigPersistPath(path string) {
	runtimeConfigMu.Lock()
	defer runtimeConfigMu.Unlock()
	runtimeConfigPersistPath = path
	loadRuntimeConfigLocked()
}

// SetEncryptionSecret derives an AES-256 key from the given secret phrase
// using the common crypto package. Call before config I/O to enable APIKey
// encryption at rest. Falls back to a hostname-derived key when secret is empty.
func SetEncryptionSecret(secret string) {
	if secret == "" {
		hostname, _ := os.Hostname()
		secret = "tangying-default-key/" + hostname
		zap.L().Warn("AUTH_TOKEN_SECRET not set — using hostname-derived encryption key. Set AUTH_TOKEN_SECRET for production.",
			zap.String("hostname", hostname))
	}
	runtimeConfigMu.Lock()
	defer runtimeConfigMu.Unlock()
	encryptionKey = crypto.DeriveKey(secret)
}

// SetRuntimeModelProviderConfig updates the runtime model-provider config and persists it.
func SetRuntimeModelProviderConfig(cfg RuntimeModelProviderConfig) {
	runtimeConfigMu.Lock()
	defer runtimeConfigMu.Unlock()
	runtimeModelProviderConfig = cfg
	persistRuntimeConfigLocked()
}

// GetRuntimeModelProviderConfig returns the current persisted runtime config.
func GetRuntimeModelProviderConfig() RuntimeModelProviderConfig {
	runtimeConfigMu.Lock()
	defer runtimeConfigMu.Unlock()
	if !runtimeConfigLoaded {
		loadRuntimeConfigLocked()
	}
	return runtimeModelProviderConfig
}

// ClearRuntimeModelProviderConfig removes the persisted runtime config so the
// backend falls back to env-based defaults.
func ClearRuntimeModelProviderConfig() {
	runtimeConfigMu.Lock()
	defer runtimeConfigMu.Unlock()
	runtimeModelProviderConfig = RuntimeModelProviderConfig{}
	if runtimeConfigPersistPath != "" {
		_ = os.Remove(runtimeConfigPersistPath)
	}
}

func loadRuntimeConfig() {
	runtimeConfigMu.Lock()
	defer runtimeConfigMu.Unlock()
	loadRuntimeConfigLocked()
}

func loadRuntimeConfigLocked() {
	runtimeConfigLoaded = true
	if runtimeConfigPersistPath == "" {
		return
	}
	data, err := os.ReadFile(runtimeConfigPersistPath)
	if err != nil {
		return
	}
	var disk onDiskConfig
	if json.Unmarshal(data, &disk) == nil {
		apiKey := ""
		if disk.APIKeyCipher != "" && len(encryptionKey) > 0 {
			if decrypted, decErr := crypto.Decrypt(disk.APIKeyCipher, encryptionKey); decErr == nil {
				apiKey = decrypted
			} else {
				zap.L().Warn("Failed to decrypt persisted API key", zap.Error(decErr))
			}
		}
		runtimeModelProviderConfig = RuntimeModelProviderConfig{
			BaseURL: disk.BaseURL,
			APIKey:  apiKey,
			Model:   disk.Model,
		}
	}
}

func persistRuntimeConfig() {
	runtimeConfigMu.Lock()
	defer runtimeConfigMu.Unlock()
	persistRuntimeConfigLocked()
}

func persistRuntimeConfigLocked() {
	if runtimeConfigPersistPath == "" {
		return
	}
	disk := onDiskConfig{
		BaseURL: runtimeModelProviderConfig.BaseURL,
		Model:   runtimeModelProviderConfig.Model,
	}
	if runtimeModelProviderConfig.APIKey != "" && len(encryptionKey) > 0 {
		if ciphertext, encErr := crypto.Encrypt(runtimeModelProviderConfig.APIKey, encryptionKey); encErr == nil {
			disk.APIKeyCipher = ciphertext
		} else {
			zap.L().Error("Failed to encrypt API key for persistence", zap.Error(encErr))
		}
	}
	data, err := json.Marshal(disk)
	if err != nil {
		return
	}
	_ = os.WriteFile(runtimeConfigPersistPath, data, 0600)
}

// RegisterVideoCreationExternalTools registers local tool manifests for video
// skills while provider APIs are not connected. They return reviewable assets
// through the same external-tool bridge used by real HTTP tools.
func RegisterVideoCreationExternalTools(registry *tool.ToolRegistry) {
	for _, name := range videoCreationExternalTools {
		registry.RegisterExternal(&tool.ToolManifest{
			Name:        name,
			Description: "Local video creation tool for reviewable intermediate assets",
			Version:     "local-1.0.0",
			Type:        "builtin",
			Endpoint:    "builtin://video-creation/" + name,
			Timeout:     120,
			Parameters: map[string]tool.ParamDef{
				"skill_name":      {Type: "string", Description: "Skill name", Required: false},
				"skill_version":   {Type: "string", Description: "Skill version", Required: false},
				"stage":           {Type: "string", Description: "Workflow stage", Required: false},
				"instruction_ref": {Type: "string", Description: "Stage instruction reference", Required: false},
				"brief":           {Type: "string", Description: "User content brief", Required: false},
			},
			Output: map[string]tool.ParamDef{
				"content":   {Type: "string", Description: "Human-reviewable markdown content"},
				"artifacts": {Type: "object", Description: "Structured intermediate assets"},
			},
			Sandbox: false,
		})
	}
}

func executeLocalVideoCreationTool(toolName string, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	stage := stringParam(params, "stage", toolName)
	skillName := stringParam(params, "skill_name", "video-skill")
	brief := stringParam(params, "brief", "")
	instructionRef := stringParam(params, "instruction_ref", "")

	// skill_stage_agent is the default tool for LLM stages. Call the LLM API
	// with a prompt built from the stage instruction and user's brief.
	if toolName == "skill_stage_agent" {
		return executeSkillStageAgent(stage, skillName, brief, instructionRef, toolCtx)
	}

	// Dynamic agent prompt_tool entries — use the LLM API to generate content
	// from the tool parameters (topic, facts, style, script, etc.).
	if isDynamicAgentPromptTool(toolName) {
		return executeDynamicAgentPromptTool(toolName, stage, skillName, brief, instructionRef, params, toolCtx)
	}

	// Dispatch to tool-specific implementations.
	switch toolName {
	case "image_asset_generator":
		return executeImageAssetGenerator(stage, skillName, brief, instructionRef, params, toolCtx)
	case "hyperframes_project_builder", "storyboard_assembler":
		return executeHyperframesProjectBuilder(stage, skillName, brief, instructionRef, params, toolCtx)
	case "hyperframes_renderer", "text_image_to_video_generator", "video_final_assembler":
		return executeHyperframesRenderer(stage, skillName, brief, instructionRef, params, toolCtx)
	default:
		// Other tools (material_library_matcher, video_keyframe_prompt_builder, etc.)
		// return a general-purpose reviewable placeholder.
		content := fmt.Sprintf(
			"## %s\n\nSkill: %s\nTask: %s\nBrief: %s\n\n该阶段已生成可审核中间态。",
			stage, skillName, toolCtx.TaskID, brief,
		)
		artifacts := []map[string]interface{}{
			{
				"unitId":   stage,
				"kind":     "MARKDOWN",
				"name":     fmt.Sprintf("%s.md", stage),
				"mimeType": "text/markdown",
				"metadata": map[string]interface{}{
					"stage":           stage,
					"skillName":       skillName,
					"source":          "local-video-creation-tool",
					"requiresReview":  true,
					"canReviseByChat": true,
				},
			},
		}
		return tool.SuccessResult(map[string]interface{}{
			"content":   content,
			"artifacts": artifacts,
		})
	}
}

// tryFetchLocalAgentConfig attempts to read the user's model-provider config
// from the local agent (127.0.0.1:18080). This covers the case where the
// frontend Desktop page saved config to the local agent but the cloud sync
// (PUT /api/config/model-provider) failed.
// TryFetchLocalAgentConfig fetches model-provider config from the local
// desktop agent (127.0.0.1:18080). Returns the config and true when found.
func TryFetchLocalAgentConfig() (RuntimeModelProviderConfig, bool) {
	url := "http://127.0.0.1:18080/api/local/model-providers?include_key=true"
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return RuntimeModelProviderConfig{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return RuntimeModelProviderConfig{}, false
	}
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Providers map[string]struct {
			BaseURL string `json:"baseUrl"`
			APIKey  string `json:"apiKey"`
			Model   string `json:"model"`
		} `json:"providers"`
	}
	if json.Unmarshal(body, &result) != nil {
		return RuntimeModelProviderConfig{}, false
	}
	t2t, ok := result.Providers["text_to_text"]
	if !ok || t2t.BaseURL == "" || t2t.APIKey == "" {
		return RuntimeModelProviderConfig{}, false
	}
	zap.L().Info("Pulled model-provider config from local agent",
		zap.String("baseUrl", t2t.BaseURL),
		zap.String("model", t2t.Model))
	return RuntimeModelProviderConfig{BaseURL: t2t.BaseURL, APIKey: t2t.APIKey, Model: t2t.Model}, true
}

func stringParam(params map[string]interface{}, key string, fallback string) string {
	if value, ok := params[key].(string); ok && value != "" {
		return value
	}
	return fallback
}

// executeSkillStageAgent is the LLM-backed implementation of skill_stage_agent.
// It reads the stage instruction markdown, combines it with the user's brief and
// upstream outputs, builds a prompt, and calls the LLM API to generate content.
func executeSkillStageAgent(stage, skillName, brief, instructionRef string, toolCtx tool.ToolContext) tool.ToolResult {
	// Read instruction file content
	instructionContent := ""
	if instructionRef != "" {
		data, err := os.ReadFile(instructionRef)
		if err != nil {
			zap.L().Warn("Cannot read instruction file, continuing without it",
				zap.String("ref", instructionRef), zap.Error(err))
		} else {
			instructionContent = string(data)
		}
	}

	if brief == "" {
		brief = "创作一篇内容"
	}

	// Build system prompt: instruction provides the stage-specific guidance.
	// The prompt is format-agnostic — the stage instruction file defines the
	// expected output format (JSON for structured data, Markdown for documents).
	systemPrompt := fmt.Sprintf(`你是一个专业的自媒体内容创作助手。当前阶段：%s。

严格按照用户需求中指定的主题进行创作，不要偏离用户指定的内容方向。

根据阶段说明中指定的输出格式，生成完整的创作内容。

如果阶段说明要求输出JSON格式，输出严格符合指定JSON schema的内容。
如果阶段说明要求输出Markdown文档，直接输出完整的Markdown内容。
如果阶段说明要求其他格式，严格遵循阶段说明中的格式规范。

重要规则：
- 不要引入阶段说明之外的额外字段、章节或内容
- 所有内容必须紧扣用户指定的主题
- 如果是JSON输出，确保字段值类型正确（字符串用双引号，数组用方括号）
- 字段值必须是纯字符串或数组，不要输出嵌套对象（除非阶段说明明确要求）`, stage)

	if instructionContent != "" {
		systemPrompt += "\n\n阶段说明：\n" + instructionContent
	}

	userPrompt := fmt.Sprintf("请严格围绕以下主题进行创作，不要偏离：\n\n%s", brief)

	// Use runtime model-provider config if available, otherwise fall back to env config.
	// Also try to pull from local agent (127.0.0.1:18080) — this catches config set
	// by the frontend Desktop page even when cloud sync is unavailable.
	effectiveCfg := GetVideoCreationOpenAIConfig()
	if localCfg, ok := TryFetchLocalAgentConfig(); ok {
		if localCfg.BaseURL != "" {
			effectiveCfg.BaseURL = localCfg.BaseURL
		}
		if localCfg.APIKey != "" {
			effectiveCfg.APIKey = localCfg.APIKey
		}
		if localCfg.Model != "" {
			effectiveCfg.Model = localCfg.Model
		}
	}

	// Call LLM via the configured OpenAI endpoint
	if effectiveCfg.APIKey == "" {
		// No API key configured — return a structured placeholder
		content := fmt.Sprintf("# %s\n\n用户需求：%s\n\n阶段说明：\n%s\n\n> ⚠️ LLM API Key 未配置。请设置 OPENAI_API_KEY 环境变量以启用 AI 内容生成。",
			stage, brief, instructionContent)
		return tool.SuccessResult(map[string]interface{}{
			"content":   content,
			"artifacts": buildSkillStageArtifacts(stage, skillName, false, false),
		})
	}

	fullPrompt := systemPrompt + "\n\n---\n\n" + userPrompt
	callTool := &LlmApiTool{cfg: effectiveCfg}
	result := callTool.Execute(context.Background(), map[string]interface{}{
		"prompt":     fullPrompt,
		"max_tokens": 8000,
	}, toolCtx)

	if !result.Success {
		zap.L().Error("LLM API call failed for skill_stage_agent",
			zap.String("stage", stage),
			zap.String("error", result.Error))
		return result
	}

	rawContent, _ := result.Data["content"].(string)
	finishReason, _ := result.Data["finishReason"].(string)
	rawContent = continueSkillStageIfNeeded(callTool, stage, fullPrompt, rawContent, finishReason, toolCtx)

	// Try to parse LLM output as JSON; use as-is if parsing fails
	var contentPkg map[string]interface{}
	scriptText := rawContent
	if err := jsonx.ExtractJSON(rawContent, &contentPkg); err == nil {
		scriptText = bestString(contentPkg, "script", "narration", "core_opinion", "logline", "description")
		if scriptText == "" {
			scriptText = rawContent
		}
	}

	// Detect JSON output so the frontend formats it as structured JSON instead
	// of rendering it as unformatted markdown (e.g., viewpoint_dossier produces JSON).
	isJSONOutput := jsonx.ExtractJSON(rawContent, &contentPkg) == nil
	includePublishCopy := isPublishPackageStage(stage) && hasPublishCopyFields(contentPkg)
	artifacts := buildSkillStageArtifacts(stage, skillName, includePublishCopy, isJSONOutput)

	data := map[string]interface{}{
		"content":   rawContent,
		"package":   contentPkg,
		"script":    scriptText,
		"artifacts": artifacts,
	}
	if title, ok := nonEmptyStringField(contentPkg, "title"); ok {
		data["title"] = title
	}
	if desc, ok := contentPkg["description"]; ok {
		if text := ensureStringValue(desc); strings.TrimSpace(text) != "" {
			data["description"] = text
		}
	}
	if keywords, ok := contentPkg["keywords"]; ok && hasPublishValue(keywords) {
		data["keywords"] = keywords
	} else if tags, ok := contentPkg["tags"]; ok && hasPublishValue(tags) {
		data["keywords"] = tags
		data["tags"] = tags
	}

	return tool.SuccessResult(data)
}

func continueSkillStageIfNeeded(callTool *LlmApiTool, stage, originalPrompt, content, finishReason string, toolCtx tool.ToolContext) string {
	combined := content
	reason := finishReason
	for attempt := 0; attempt < 2 && needsSkillStageContinuation(stage, combined, reason); attempt++ {
		prompt := buildSkillStageContinuationPrompt(originalPrompt, combined)
		result := callTool.Execute(context.Background(), map[string]interface{}{
			"prompt":     prompt,
			"max_tokens": 8000,
		}, toolCtx)
		if !result.Success {
			zap.L().Warn("LLM continuation failed for skill_stage_agent",
				zap.String("stage", stage),
				zap.String("error", result.Error))
			return combined
		}
		next, _ := result.Data["content"].(string)
		if strings.TrimSpace(next) == "" {
			return combined
		}
		combined = appendContinuation(combined, next)
		reason, _ = result.Data["finishReason"].(string)
	}
	return combined
}

func needsSkillStageContinuation(stage, content, finishReason string) bool {
	if strings.EqualFold(strings.TrimSpace(finishReason), "length") {
		return true
	}
	if stage != "hyperframes_reference" {
		return false
	}
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	required := []string{
		"一、观点档案",
		"二、视频总体设定",
		"三、重要制作原则",
		"四、逐段画面脚本",
		"五、imagegen 图片清单",
		"六、图片在视频中的处理方式",
		"七、字幕和文字规则",
		"八、整体节奏控制",
		"九、推荐项目素材目录",
	}
	for _, marker := range required {
		if !strings.Contains(trimmed, marker) {
			return true
		}
	}
	last := []rune(trimmed)
	if len(last) == 0 {
		return false
	}
	switch last[len(last)-1] {
	case '。', '！', '？', '.', '!', '?', '`', '）', ')':
		return false
	default:
		return true
	}
}

func buildSkillStageContinuationPrompt(originalPrompt, partialContent string) string {
	return fmt.Sprintf(`%s

---

上一次输出被截断，下面是已经生成的内容。请从最后一个未完成的位置继续写，先补完当前句子，然后继续完成剩余章节。

要求：
- 只输出续写内容，不要重复已经完整出现的章节和段落
- 保持原输出格式；如果原内容是 Markdown，保持 Markdown 层级；如果原内容是 JSON，继续补完整 JSON
- 如果正在 HyperFrames 参考文档的某个 BEAT 中间，继续完成该 BEAT 的剩余小节，再继续后续 BEAT 和第五到第九章
- 必须写到文档自然结束

已生成内容：

%s`, originalPrompt, partialContent)
}

func appendContinuation(content, continuation string) string {
	base := strings.TrimRight(content, "\r\n")
	next := strings.TrimLeft(continuation, "\r\n")
	if base == "" {
		return next
	}
	if shouldAppendContinuationInline(base, next) {
		return base + next
	}
	return base + "\n" + next
}

func shouldAppendContinuationInline(base, next string) bool {
	if base == "" || next == "" {
		return false
	}
	if strings.HasPrefix(strings.TrimSpace(next), "#") || strings.HasPrefix(strings.TrimSpace(next), "-") {
		return false
	}
	last := []rune(strings.TrimSpace(base))
	if len(last) == 0 {
		return false
	}
	switch last[len(last)-1] {
	case '。', '！', '？', '.', '!', '?', ':', '：', ';', '；', '`':
		return false
	default:
		return true
	}
}

func buildSkillStageArtifacts(stage, skillName string, includePublishCopy, isJSON bool) []map[string]interface{} {
	kind := "MARKDOWN"
	name := fmt.Sprintf("%s.md", stage)
	mime := "text/markdown"
	if isJSON {
		kind = "JSON"
		name = fmt.Sprintf("%s.json", stage)
		mime = "application/json"
	}
	artifacts := []map[string]interface{}{
		{
			"unitId":   stage,
			"kind":     kind,
			"name":     name,
			"mimeType": mime,
			"metadata": map[string]interface{}{
				"stage":           stage,
				"skillName":       skillName,
				"requiresReview":  true,
				"canReviseByChat": true,
				"source":          "skill-stage-agent",
			},
		},
	}
	if includePublishCopy {
		artifacts = append(artifacts, map[string]interface{}{
			"unitId":   "publish-copy",
			"kind":     "JSON",
			"name":     "发布文案.json",
			"mimeType": "application/json",
			"metadata": map[string]interface{}{
				"stage":     stage,
				"skillName": skillName,
				"source":    "skill-stage-agent",
			},
		})
	}
	return artifacts
}

func isPublishPackageStage(stage string) bool {
	normalized := strings.ToLower(strings.TrimSpace(stage))
	return normalized == "publish" ||
		normalized == "publish_copy" ||
		normalized == "publish_package"
}

func hasPublishCopyFields(pkg map[string]interface{}) bool {
	if len(pkg) == 0 {
		return false
	}
	for _, key := range []string{"title", "description", "keywords", "tags"} {
		if hasPublishValue(pkg[key]) {
			return true
		}
	}
	if nested, ok := pkg["publishCopy"].(map[string]interface{}); ok {
		return hasPublishCopyFields(nested)
	}
	return false
}

func hasPublishValue(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case []interface{}:
		for _, item := range typed {
			if hasPublishValue(item) {
				return true
			}
		}
		return false
	case []string:
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				return true
			}
		}
		return false
	case map[string]interface{}:
		for _, key := range []string{"keyword", "tag", "label", "name", "text", "value"} {
			if hasPublishValue(typed[key]) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

func nonEmptyStringField(pkg map[string]interface{}, key string) (string, bool) {
	if len(pkg) == 0 {
		return "", false
	}
	value, ok := pkg[key]
	if !ok {
		return "", false
	}
	text := strings.TrimSpace(ensureStringValue(value))
	return text, text != ""
}

// ensureStringValue converts a value to its string representation,
// preventing nested objects from leaking to downstream consumers.
func ensureStringValue(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// --- Dedicated video creation tool implementations ---

// executeImageAssetGenerator generates image prompts from the HyperFrames reference
// and optionally calls an image generation API.
func executeImageAssetGenerator(stage, skillName, brief, instructionRef string, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	// Extract the upstream HyperFrames reference content (resolved by executor).
	assetPlanRef := stringParam(params, "asset_plan_ref", "")
	if assetPlanRef == "" {
		assetPlanRef = brief
	}

	// Parse the reference to extract image prompts from section 五.
	imageRequests := extractImagePromptsFromReference(assetPlanRef)

	// If no prompts found or LLM is available, use LLM to generate/elaborate prompts.
	if len(imageRequests) == 0 {
		imageRequests = []map[string]interface{}{
			{
				"id":     "default-01",
				"prompt": buildDefaultImagePrompt(brief),
				"dimensions": map[string]int{
					"width":  1920,
					"height": 1080,
				},
				"status":     "prompt_only",
				"outputPath": "assets/generated/default-scene.png",
			},
		}
	} else {
		// Elaborate each prompt with the default template using LLM if available.
		cfg := GetVideoCreationOpenAIConfig()
		if cfg.APIKey != "" {
			for i, req := range imageRequests {
				if prompt, ok := req["prompt"].(string); ok && !strings.Contains(prompt, "Use case:") {
					elaborated := elaborateImagePrompt(cfg, prompt, toolCtx)
					if elaborated != "" {
						imageRequests[i]["prompt"] = elaborated
					}
				}
			}
		}
	}

	// Generate images via the model gateway (supports OpenAI DALL-E, Stability AI, etc.)
	generatedCount := 0
	if modelGateway != nil {
		for i, req := range imageRequests {
			if prompt, ok := req["prompt"].(string); ok && prompt != "" {
				result, err := modelGateway.Execute(context.Background(), &modelgateway.ModelRequest{
					Capability: modelgateway.CapTextToImage,
					Parameters: map[string]interface{}{
						"prompt":       prompt,
						"size":         "1792x1024",
						"aspect_ratio": "16:9",
					},
				})
				if err == nil && len(result.Images) > 0 {
					imageRequests[i]["status"] = "generated"
					imageRequests[i]["storageRef"] = result.Images[0].URL
					generatedCount++
				} else if err != nil {
					zap.L().Warn("Image generation via gateway failed, keeping as prompt-only",
						zap.String("id", stringParam(req, "id", "unknown")),
						zap.Error(err))
				}
			}
		}
	}

	providerNote := "图片生成服务未配置，已输出完整提示词"
	if modelGateway != nil {
		providerNote = "已通过模型网关生成图片"
	}

	summary := map[string]interface{}{
		"total":      len(imageRequests),
		"generated":  generatedCount,
		"promptOnly": len(imageRequests) - generatedCount,
		"note":       providerNote,
	}

	content := buildImageAssetContent(stage, skillName, imageRequests, summary)
	artifacts := buildImageAssetArtifacts(stage, skillName, imageRequests)

	return tool.SuccessResult(map[string]interface{}{
		"content":       content,
		"imageRequests": imageRequests,
		"summary":       summary,
		"artifacts":     artifacts,
	})
}

// executeHyperframesProjectBuilder builds or provides guidance for a HyperFrames project.
func executeHyperframesProjectBuilder(stage, skillName, brief, instructionRef string, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	referenceRef := stringParam(params, "reference_ref", "")
	assetsRef := stringParam(params, "assets_ref", "")

	cliCmd, cliFound := detectHyperFramesCLI()
	status := "guidance_only"
	var projectRef, previewURL, lintOutput string
	var beatsBuilt int

	if cliFound {
		// Attempt to build the project with HyperFrames CLI.
		zap.L().Info("Attempting HyperFrames project build",
			zap.String("taskId", toolCtx.TaskID),
			zap.String("cli", cliCmd))

		args := buildHyperFramesBuildArgs(cliCmd, referenceRef, assetsRef, toolCtx.TaskID)
		if output, err := runHyperFramesCommand(cliCmd, args, toolCtx.TaskID); err == nil {
			projectRef = strings.TrimSpace(output)
			previewURL = fmt.Sprintf("file://%s/index.html", projectRef)
			status = "built"
			beatsBuilt = countBeatsFromReference(referenceRef)
			zap.L().Info("HyperFrames project built successfully",
				zap.String("projectRef", projectRef))
		} else {
			zap.L().Warn("HyperFrames build failed, falling back to guidance",
				zap.Error(err))
			lintOutput = err.Error()
		}
	}

	if status == "guidance_only" {
		projectRef = fmt.Sprintf("projects/%s-hyperframes", toolCtx.TaskID)
	}

	content := buildProjectContent(stage, skillName, status, cliFound, projectRef, previewURL, lintOutput, referenceRef)
	artifacts := buildProjectArtifacts(stage, skillName, status, projectRef, referenceRef)

	return tool.SuccessResult(map[string]interface{}{
		"content":      content,
		"projectRef":   projectRef,
		"previewUrl":   previewURL,
		"lintOutput":   lintOutput,
		"beatsBuilt":   beatsBuilt,
		"status":       status,
		"cliAvailable": cliFound,
		"guidance":     buildHyperFramesGuidance(cliCmd, referenceRef, assetsRef, toolCtx.TaskID),
		"artifacts":    artifacts,
	})
}

// executeHyperframesRenderer renders a HyperFrames project to MP4.
//
// When HyperFrames mode is "service", it calls the Render Service HTTP API.
// Otherwise it falls back to the legacy CLI detection (deprecated).
func executeHyperframesRenderer(stage, skillName, brief, instructionRef string, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	projectRef := stringParam(params, "project_ref", "")
	projectDir := stringParam(params, "projectDir", projectRef)

	status := "guidance_only"
	var renderPath, duration, resolution, codec, fileSize, jobID string
	serviceUsed := false

	// Prefer service mode over CLI.
	if hyperFramesServiceAvailable() {
		zap.L().Info("HyperFrames render via Render Service",
			zap.String("taskId", toolCtx.TaskID),
			zap.String("projectDir", projectDir))

		outputPath := fmt.Sprintf("%s/%s.mp4", hyperFramesConfig.OutputRoot, toolCtx.TaskID)
		ctx, cancel := context.WithTimeout(context.Background(), hyperFramesConfig.Timeout())
		defer cancel()

		result, err := hyperFramesClient.Render(ctx, hyperframes.RenderRequest{
			ProjectDir: projectDir,
			Entry:      stringParam(params, "entry", "index.html"),
			OutputPath: outputPath,
			FPS:        hyperFramesConfig.DefaultFPS,
			Quality:    hyperFramesConfig.DefaultQuality,
			Format:     hyperFramesConfig.DefaultFormat,
			Workers:    hyperFramesConfig.MaxWorkers,
			UseGPU:     hyperFramesConfig.UseGPU,
		})
		if err != nil {
			zap.L().Error("HyperFrames Render Service call failed",
				zap.String("taskId", toolCtx.TaskID),
				zap.Error(err))
			return tool.FailureResult(
				fmt.Sprintf("HyperFrames Render Service 渲染失败: %v。请检查本地渲染服务是否启动。", err),
			)
		}

		renderPath = result.OutputPath
		jobID = result.JobID
		duration = fmt.Sprintf("%.1fs", float64(result.DurationMs)/1000.0)
		status = "rendered"
		serviceUsed = true
		zap.L().Info("HyperFrames render completed via service",
			zap.String("jobId", jobID),
			zap.String("outputPath", renderPath),
			zap.Int64("durationMs", result.DurationMs))
	} else {
		// Legacy CLI fallback (deprecated, kept for transition).
		cliCmd, cliFound := detectHyperFramesCLI()

		if cliFound && projectRef != "" {
			zap.L().Info("Attempting HyperFrames render via CLI (deprecated)",
				zap.String("taskId", toolCtx.TaskID),
				zap.String("projectRef", projectRef))

			args := buildHyperFramesRenderArgs(cliCmd, projectRef, toolCtx.TaskID)
			if output, err := runHyperFramesCommand(cliCmd, args, toolCtx.TaskID); err == nil {
				renderPath = fmt.Sprintf("output/%s.mp4", toolCtx.TaskID)
				duration = extractDurationFromOutput(output)
				resolution = extractResolutionFromOutput(output)
				codec = "h264"
				fileSize = extractFileSizeFromOutput(output)
				status = "rendered"
				zap.L().Info("HyperFrames render completed via CLI",
					zap.String("path", renderPath))
			} else {
				zap.L().Warn("HyperFrames CLI render failed",
					zap.Error(err))
			}
		}
	}

	renderArtifacts := []map[string]interface{}{
		{
			"path":       renderPath,
			"duration":   duration,
			"resolution": resolution,
			"codec":      codec,
			"size":       fileSize,
			"status":     status,
			"jobId":      jobID,
		},
	}

	content := buildRenderContent(stage, skillName, status, true, renderPath, renderArtifacts)
	artifacts := buildRenderArtifacts(stage, skillName, status, renderPath)

	resultData := map[string]interface{}{
		"content":         content,
		"renderArtifacts": renderArtifacts,
		"serviceUsed":     serviceUsed,
		"renderLogs":      fmt.Sprintf("Status: %s, Service: %v", status, serviceUsed),
		"diagnostics": map[string]interface{}{
			"warnings": []string{},
		},
		"artifacts": artifacts,
	}

	if renderPath != "" {
		resultData["outputPath"] = renderPath
		resultData["artifact"] = map[string]interface{}{
			"kind":       "VIDEO",
			"name":       "final.mp4",
			"storageRef": fmt.Sprintf("local://%s", renderPath),
		}
	}
	if jobID != "" {
		resultData["jobId"] = jobID
	}

	return tool.SuccessResult(resultData)
}

// --- Helper functions for image asset generation ---

// extractImagePromptsFromReference scans a HyperFrames reference document for
// image entries in section 五 (imagegen 图片清单).
func extractImagePromptsFromReference(referenceContent string) []map[string]interface{} {
	var requests []map[string]interface{}
	lines := strings.Split(referenceContent, "\n")
	inImageSection := false
	var currentID, currentPrompt, currentOutputPath string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "五、imagegen") || strings.Contains(trimmed, "## 图片") {
			inImageSection = true
			continue
		}
		if inImageSection && (strings.Contains(trimmed, "六、") || strings.Contains(trimmed, "# 六")) {
			break
		}
		if !inImageSection {
			continue
		}

		if strings.HasPrefix(trimmed, "## 图片") {
			// Save previous entry
			if currentID != "" && currentPrompt != "" {
				requests = append(requests, map[string]interface{}{
					"id":         currentID,
					"prompt":     strings.TrimSpace(currentPrompt),
					"dimensions": map[string]int{"width": 1920, "height": 1080},
					"status":     "prompt_only",
					"outputPath": currentOutputPath,
				})
			}
			currentID = strings.TrimPrefix(trimmed, "## 图片")
			currentID = strings.TrimSpace(currentID)
			currentPrompt = ""
			currentOutputPath = ""
		} else if strings.Contains(trimmed, "### 输出路径") || strings.Contains(trimmed, "输出路径") {
			parts := strings.SplitN(trimmed, "`", 3)
			if len(parts) >= 2 {
				currentOutputPath = parts[1]
			}
		} else if strings.Contains(trimmed, "### 提示词") || strings.HasPrefix(trimmed, "Use case:") {
			continue
		} else if currentID != "" && trimmed != "" && !strings.HasPrefix(trimmed, "###") {
			if currentPrompt != "" {
				currentPrompt += "\n"
			}
			currentPrompt += trimmed
		}
	}

	// Save last entry
	if currentID != "" && currentPrompt != "" {
		requests = append(requests, map[string]interface{}{
			"id":         currentID,
			"prompt":     strings.TrimSpace(currentPrompt),
			"dimensions": map[string]int{"width": 1920, "height": 1080},
			"status":     "prompt_only",
			"outputPath": currentOutputPath,
		})
	}

	return requests
}

// buildDefaultImagePrompt creates a default imagegen prompt from a user brief.
func buildDefaultImagePrompt(brief string) string {
	return fmt.Sprintf(`Use case: productivity-visual
Asset type: HyperFrames scene background
Primary request: %s
Style/medium: clean premium non-realistic editorial 3D illustration, minimal shapes, subtle texture, modern business technology tone
Composition/framing: 16:9, safe central subject area, usable negative space for HTML captions and overlays
Lighting/mood: soft controlled studio lighting, calm cinematic contrast
Color palette: white, light gray, deep blue, restrained teal accents, occasional orange-red only for pressure/risk
Text: no readable text, no letters, no numbers
Constraints: no watermark, no logo, no photorealism, no clutter, no distorted hands or faces`, brief)
}

// elaborateImagePrompt uses the LLM to expand a brief image prompt into the
// full default template format.
func elaborateImagePrompt(cfg config.OpenAIConfig, briefPrompt string, toolCtx tool.ToolContext) string {
	systemPrompt := `你是一个专业的自媒体视频图片提示词工程师。
将给定的简短图片描述扩展为完整的 imagegen 提示词，必须包含以下所有维度：

Use case: <用途>
Asset type: HyperFrames scene background
Primary request: <具体场景描述>
Style/medium: clean premium non-realistic editorial 3D illustration, minimal shapes, subtle texture, modern business technology tone
Composition/framing: 16:9, safe central subject area, usable negative space for HTML captions and overlays
Lighting/mood: soft controlled studio lighting, calm cinematic contrast
Color palette: white, light gray, deep blue, restrained teal accents, occasional orange-red only for pressure/risk
Text: no readable text, no letters, no numbers
Constraints: no watermark, no logo, no photorealism, no clutter, no distorted hands or faces

只输出完整的提示词文本，不要添加额外解释。`

	callTool := &LlmApiTool{cfg: cfg}
	result := callTool.Execute(context.Background(), map[string]interface{}{
		"prompt":     systemPrompt + "\n\n简短描述：" + briefPrompt,
		"max_tokens": 500,
	}, toolCtx)

	if result.Success {
		if content, ok := result.Data["content"].(string); ok {
			return strings.TrimSpace(content)
		}
	}
	return ""
}

func buildImageAssetContent(stage, skillName string, requests []map[string]interface{}, summary map[string]interface{}) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s - Image Assets\n\n", stage))
	b.WriteString(fmt.Sprintf("Skill: %s\n\n", skillName))
	b.WriteString(fmt.Sprintf("Generated %d image requests (%d with prompts only)\n\n", summary["total"], summary["promptOnly"]))

	for _, req := range requests {
		id, _ := req["id"].(string)
		prompt, _ := req["prompt"].(string)
		status, _ := req["status"].(string)
		b.WriteString(fmt.Sprintf("## %s (Status: %s)\n\n", id, status))
		b.WriteString("```text\n")
		b.WriteString(prompt)
		b.WriteString("\n```\n\n")
	}
	return b.String()
}

func buildImageAssetArtifacts(stage, skillName string, requests []map[string]interface{}) []map[string]interface{} {
	artifacts := []map[string]interface{}{
		{
			"unitId":   stage,
			"kind":     "MARKDOWN",
			"name":     fmt.Sprintf("%s.md", stage),
			"mimeType": "text/markdown",
			"metadata": map[string]interface{}{
				"stage":           stage,
				"skillName":       skillName,
				"requiresReview":  true,
				"canReviseByChat": true,
				"source":          "image-asset-generator",
			},
		},
		{
			"unitId":   "image-requests",
			"kind":     "JSON",
			"name":     fmt.Sprintf("%s-image-requests.json", stage),
			"mimeType": "application/json",
			"metadata": map[string]interface{}{
				"stage":     stage,
				"total":     len(requests),
				"generated": countByStatus(requests, "generated"),
			},
		},
	}
	return artifacts
}

// --- Helper functions for project building ---

func buildHyperFramesBuildArgs(cliCmd, referenceRef, assetsRef, taskID string) []string {
	if cliCmd == "npx" {
		return []string{"hyperframes", "build",
			"--ref", referenceRef,
			"--assets", assetsRef,
			"--task-id", taskID,
			"--output", fmt.Sprintf("projects/%s-hyperframes", taskID),
		}
	}
	return []string{"build",
		"--ref", referenceRef,
		"--assets", assetsRef,
		"--task-id", taskID,
		"--output", fmt.Sprintf("projects/%s-hyperframes", taskID),
	}
}

func buildHyperFramesRenderArgs(cliCmd, projectRef, taskID string) []string {
	outputPath := fmt.Sprintf("output/%s.mp4", taskID)
	if cliCmd == "npx" {
		return []string{"hyperframes", "render",
			"--project", projectRef,
			"--output", outputPath,
			"--format", "mp4",
		}
	}
	return []string{"render",
		"--project", projectRef,
		"--output", outputPath,
		"--format", "mp4",
	}
}

func runHyperFramesCommand(cliCmd string, args []string, taskID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, cliCmd, args...)
	cmd.Env = append(os.Environ(), "TANGYING_TASK_ID="+taskID)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("hyperframes command failed: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
}

func countBeatsFromReference(referenceContent string) int {
	count := 0
	for _, line := range strings.Split(referenceContent, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "## BEAT ") {
			count++
		}
	}
	return count
}

func buildProjectContent(stage, skillName, status string, cliFound bool, projectRef, previewURL, lintOutput, referenceRef string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s - HyperFrames Project\n\n", stage))
	b.WriteString(fmt.Sprintf("Skill: %s\n", skillName))
	b.WriteString(fmt.Sprintf("Status: %s\n", status))
	b.WriteString(fmt.Sprintf("CLI Available: %v\n\n", cliFound))

	if status == "built" {
		b.WriteString(fmt.Sprintf("Project built successfully.\n"))
		b.WriteString(fmt.Sprintf("- Project: `%s`\n", projectRef))
		b.WriteString(fmt.Sprintf("- Preview: %s\n", previewURL))
	} else {
		b.WriteString("## Build Guidance\n\n")
		b.WriteString("HyperFrames CLI not available. To build this project manually:\n\n")
		b.WriteString("```bash\n")
		b.WriteString(fmt.Sprintf("# 1. Create project directory\n"))
		b.WriteString(fmt.Sprintf("mkdir -p %s\n\n", projectRef))
		b.WriteString(fmt.Sprintf("# 2. Copy the HyperFrames reference as the build instruction\n"))
		b.WriteString(fmt.Sprintf("# 3. Run: npx hyperframes build --ref <ref> --output %s\n", projectRef))
		b.WriteString("```\n\n")
		b.WriteString("### Reference Content:\n\n")
		if len(referenceRef) > 2000 {
			b.WriteString(referenceRef[:2000])
			b.WriteString("\n\n... (truncated)\n")
		} else {
			b.WriteString(referenceRef)
		}
	}

	if lintOutput != "" {
		b.WriteString(fmt.Sprintf("\n## Lint Output\n\n```\n%s\n```\n", lintOutput))
	}

	return b.String()
}

func buildProjectArtifacts(stage, skillName, status, projectRef, referenceRef string) []map[string]interface{} {
	artifacts := []map[string]interface{}{
		{
			"unitId":   stage,
			"kind":     "MARKDOWN",
			"name":     fmt.Sprintf("%s.md", stage),
			"mimeType": "text/markdown",
			"metadata": map[string]interface{}{
				"stage":           stage,
				"skillName":       skillName,
				"requiresReview":  true,
				"canReviseByChat": true,
				"source":          "hyperframes-project-builder",
			},
		},
		{
			"unitId":   "project-files",
			"kind":     "JSON",
			"name":     fmt.Sprintf("%s-project-files.json", stage),
			"mimeType": "application/json",
			"metadata": map[string]interface{}{
				"stage":      stage,
				"status":     status,
				"projectRef": projectRef,
			},
		},
	}
	return artifacts
}

func buildHyperFramesGuidance(cliCmd, referenceRef, assetsRef, taskID string) string {
	if cliCmd == "" {
		cliCmd = "npx hyperframes"
	}
	projectDir := fmt.Sprintf("projects/%s-hyperframes", taskID)
	return fmt.Sprintf(`# HyperFrames 项目构建指引

## CLI 命令
%s build --ref <reference.md> --assets <assets/> --output %s

## 手动构建步骤
1. 创建项目目录：mkdir -p %s
2. 将参考文档保存为 reference.md
3. 将图片素材放入 assets/ 目录
4. 按参考文档第四章逐 BEAT 构建画面
5. 运行验证：%s validate --project %s
`, cliCmd, projectDir, projectDir, cliCmd, projectDir)
}

// --- Helper functions for rendering ---

func buildRenderContent(stage, skillName, status string, cliFound bool, renderPath string, renderArtifacts []map[string]interface{}) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s - Video Render\n\n", stage))
	b.WriteString(fmt.Sprintf("Skill: %s\n", skillName))
	b.WriteString(fmt.Sprintf("Status: %s\n", status))
	b.WriteString(fmt.Sprintf("CLI Available: %v\n\n", cliFound))

	if status == "rendered" {
		b.WriteString(fmt.Sprintf("Render completed.\n"))
		b.WriteString(fmt.Sprintf("- Output: `%s`\n", renderPath))
	} else {
		b.WriteString("## Render Guidance\n\n")
		b.WriteString("HyperFrames CLI not available. To render this project manually:\n\n")
		b.WriteString("```bash\n")
		if cliFound {
			b.WriteString(fmt.Sprintf("hyperframes render --project <project> --output output/video.mp4 --format mp4\n"))
		} else {
			b.WriteString(fmt.Sprintf("npx hyperframes render --project <project> --output output/video.mp4 --format mp4\n"))
		}
		b.WriteString("```\n\n")
		b.WriteString("Expected output: MP4, h264 codec, matching the aspect ratio from the reference document.\n")
	}

	return b.String()
}

func buildRenderArtifacts(stage, skillName, status, renderPath string) []map[string]interface{} {
	artifacts := []map[string]interface{}{
		{
			"unitId":   stage,
			"kind":     "MARKDOWN",
			"name":     fmt.Sprintf("%s.md", stage),
			"mimeType": "text/markdown",
			"metadata": map[string]interface{}{
				"stage":           stage,
				"skillName":       skillName,
				"requiresReview":  true,
				"canReviseByChat": true,
				"source":          "hyperframes-renderer",
			},
		},
		{
			"unitId":   "video-import-package",
			"kind":     "JSON",
			"name":     fmt.Sprintf("%s-video-package.json", stage),
			"mimeType": "application/json",
			"metadata": map[string]interface{}{
				"stage":      stage,
				"status":     status,
				"renderPath": renderPath,
			},
		},
	}
	return artifacts
}

func buildRenderGuidance(cliCmd, projectRef, taskID string) string {
	if cliCmd == "" {
		cliCmd = "npx hyperframes"
	}
	outputPath := fmt.Sprintf("output/%s.mp4", taskID)
	return fmt.Sprintf(`# HyperFrames 渲染指引

## CLI 命令
%s render --project %s --output %s --format mp4

## 渲染参数建议
- 分辨率：与参考文档画幅一致（默认 1920x1080 或 1080x1920）
- 编码器：h264
- 比特率：建议 8-15 Mbps（根据时长和画质需求）
- 帧率：30fps
- 音频：包含 TTS 预览音轨或真人录音
`, cliCmd, projectRef, outputPath)
}

// --- Output extraction helpers ---

func extractDurationFromOutput(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "duration") || strings.Contains(line, "Duration") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func extractResolutionFromOutput(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "resolution") || strings.Contains(line, "Resolution") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func extractFileSizeFromOutput(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "size") || strings.Contains(line, "Size") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func countByStatus(requests []map[string]interface{}, status string) int {
	count := 0
	for _, req := range requests {
		if s, ok := req["status"].(string); ok && s == status {
			count++
		}
	}
	return count
}

// isDynamicAgentPromptTool reports whether toolName is a dynamic-agent prompt_tool
// that should be executed via the LLM API (like skill_stage_agent).
func isDynamicAgentPromptTool(toolName string) bool {
	switch toolName {
	case "knowledge_researcher", "fact_checker",
		"video_script_generator", "shot_splitter",
		"keyframe_prompt_generator", "video_prompt_generator",
		"script_quality_checker", "shot_quality_checker",
		"video_prompt_quality_checker", "package_quality_checker",
		"publish_copy_generator", "video_package_exporter":
		return true
	default:
		return false
	}
}

// executeDynamicAgentPromptTool executes a dynamic-agent prompt_tool by building
// an LLM prompt from the tool parameters and calling the LLM API.
func executeDynamicAgentPromptTool(toolName, stage, skillName, brief, instructionRef string, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	topic := stringParam(params, "topic", brief)
	facts := stringParam(params, "facts", "")
	style := stringParam(params, "outputStyle", "")
	platform := stringParam(params, "platform", "通用平台")
	script := stringParam(params, "script", "")
	shotList := stringParam(params, "shotList", "")
	videoPrompts := stringParam(params, "videoPrompts", "")
	publishCopy := stringParam(params, "publishCopy", "")

	systemPrompt := buildDynamicAgentSystemPrompt(toolName, topic, style, platform)
	userPrompt := buildDynamicAgentUserPrompt(toolName, topic, facts, style, script, shotList, videoPrompts, publishCopy, platform)

	effectiveCfg := GetVideoCreationOpenAIConfig()
	if localCfg, ok := TryFetchLocalAgentConfig(); ok {
		if localCfg.BaseURL != "" {
			effectiveCfg.BaseURL = localCfg.BaseURL
		}
		if localCfg.APIKey != "" {
			effectiveCfg.APIKey = localCfg.APIKey
		}
		if localCfg.Model != "" {
			effectiveCfg.Model = localCfg.Model
		}
	}

	if effectiveCfg.APIKey == "" {
		content := fmt.Sprintf("# %s\n\n主题：%s\n\n> ⚠️ LLM API Key 未配置。请设置 API Key 以启用 AI 内容生成。", toolName, topic)
		return tool.SuccessResult(map[string]interface{}{
			"content":   content,
			"artifacts": buildSkillStageArtifacts(toolName, skillName, false, false),
		})
	}

	callTool := &LlmApiTool{cfg: effectiveCfg}
	result := callTool.Execute(context.Background(), map[string]interface{}{
		"prompt":     systemPrompt + "\n\n---\n\n" + userPrompt,
		"max_tokens": 8000,
	}, toolCtx)

	if !result.Success {
		zap.L().Error("LLM API call failed for dynamic agent prompt tool",
			zap.String("tool", toolName),
			zap.Error(fmt.Errorf("%s", result.Error)))
		return result
	}

	rawContent, _ := result.Data["content"].(string)
	finishReason, _ := result.Data["finishReason"].(string)
	rawContent = continueSkillStageIfNeeded(callTool, toolName, systemPrompt+"\n\n---\n\n"+userPrompt, rawContent, finishReason, toolCtx)

	var contentPkg map[string]interface{}
	isJSON := jsonx.ExtractJSON(rawContent, &contentPkg) == nil

	// For tools that require structured JSON output, fail on parse error
	// instead of silently accepting markdown.
	if !isJSON && isStructuredOutputTool(toolName) {
		preview := rawContent
		if len(preview) > 500 {
			preview = preview[:500]
		}
		zap.L().Error("dynamic agent prompt tool failed to produce structured JSON",
			zap.String("tool", toolName),
			zap.String("rawContent", preview))
		return tool.FailureResult(fmt.Sprintf("%s: LLM 未返回结构化 JSON，请重试", toolName))
	}

	artifacts := buildSkillStageArtifacts(toolName, skillName, toolName == "publish_copy_generator", isJSON)

	data := map[string]interface{}{
		"content":   rawContent,
		"package":   contentPkg,
		"artifacts": artifacts,
	}

	if title, ok := nonEmptyStringField(contentPkg, "title"); ok {
		data["title"] = title
	}
	if desc, ok := contentPkg["description"]; ok {
		if text := ensureStringValue(desc); strings.TrimSpace(text) != "" {
			data["description"] = text
		}
	}
	if keywords, ok := contentPkg["keywords"]; ok && hasPublishValue(keywords) {
		data["keywords"] = keywords
	}
	if scriptText, ok := nonEmptyStringField(contentPkg, "script"); ok {
		data["script"] = scriptText
	}
	if factsOut, ok := contentPkg["facts"]; ok {
		data["facts"] = factsOut
	}
	if storyAngles, ok := contentPkg["storyAngles"]; ok {
		data["storyAngles"] = storyAngles
	}
	if risks, ok := contentPkg["risks"]; ok {
		data["risks"] = risks
	}
	if warnings, ok := contentPkg["warnings"]; ok {
		data["warnings"] = warnings
	}
	if summary, ok := nonEmptyStringField(contentPkg, "summary"); ok {
		data["summary"] = summary
	}
	if estimatedDurationSec, ok := contentPkg["estimatedDurationSec"]; ok {
		data["estimatedDurationSec"] = estimatedDurationSec
	}
	// Structured fields for shot_splitter and video_prompt_generator
	if shotList, ok := contentPkg["shotList"]; ok {
		data["shotList"] = shotList
	}
	if videoPromptsOut, ok := contentPkg["videoPrompts"]; ok {
		data["videoPrompts"] = videoPromptsOut
	}
	if totalDurationSec, ok := contentPkg["totalDurationSec"]; ok {
		data["totalDurationSec"] = totalDurationSec
	}
	if sections, ok := contentPkg["sections"]; ok {
		data["sections"] = sections
	}
	if qualityHints, ok := contentPkg["qualityHints"]; ok {
		data["qualityHints"] = qualityHints
	}
	if keyframePromptsOut, ok := contentPkg["keyframePrompts"]; ok {
		data["keyframePrompts"] = keyframePromptsOut
	}
	// Quality checker output fields
	if passed, ok := contentPkg["passed"]; ok {
		data["passed"] = passed
	}
	if score, ok := contentPkg["score"]; ok {
		data["score"] = score
	}
	if issues, ok := contentPkg["issues"]; ok {
		data["issues"] = issues
	}
	if repairSuggestions, ok := contentPkg["repairSuggestions"]; ok {
		data["repairSuggestions"] = repairSuggestions
	}
	if missingArtifacts, ok := contentPkg["missingArtifacts"]; ok {
		data["missingArtifacts"] = missingArtifacts
	}
	if unreviewedArtifacts, ok := contentPkg["unreviewedArtifacts"]; ok {
		data["unreviewedArtifacts"] = unreviewedArtifacts
	}

	return tool.SuccessResult(data)
}

// isStructuredOutputTool reports whether toolName requires structured JSON output
// (as opposed to free-form Markdown). Quality checkers and content production tools
// must output valid JSON.
func isStructuredOutputTool(toolName string) bool {
	switch toolName {
	case "knowledge_researcher", "fact_checker",
		"video_script_generator", "shot_splitter",
		"keyframe_prompt_generator", "video_prompt_generator",
		"script_quality_checker", "shot_quality_checker",
		"video_prompt_quality_checker", "package_quality_checker",
		"video_package_exporter":
		return true
	default:
		return false
	}
}

func buildDynamicAgentSystemPrompt(toolName, topic, style, platform string) string {
	switch toolName {
	case "knowledge_researcher":
		return fmt.Sprintf(`你是知识分享视频资料研究员。

任务：
根据 topic 整理适合短视频口播的事实材料、讲述角度、风险点。

要求：
1. 不要输出 Markdown。
2. 只输出 JSON。
3. 不要编造明确历史细节。
4. 对不确定内容写入 risks。
5. facts 应该短句化，适合后续口播稿生成。
6. storyAngles 给出 3-5 个短视频讲述角度。

输入：
topic=%s
style=%s

输出 JSON：
{
  "facts": ["事实短句1", "事实短句2", ...],
  "timeline": [{"date": "时间", "event": "事件"}, ...],
  "storyAngles": ["讲述角度1", "讲述角度2", ...],
  "risks": ["需要注意的事实风险点", ...],
  "sourceNotes": ["来源说明", ...],
  "summary": "资料整理摘要"
}`, topic, style)

	case "fact_checker":
		return "你是知识类短视频事实核查员。\n\n任务：\n检查 facts 中是否存在不稳妥、过度简化、容易误导或需要谨慎表达的内容。\n\n要求：\n1. 不要输出 Markdown。\n2. 只输出 JSON。\n3. 保留稳妥事实到 checkedFacts。\n4. 把不确定或争议内容写入 warnings。\n5. 对需要改写的内容写入 corrections。\n\n输出 JSON：\n{\n  \"checkedFacts\": [\"已核查的事实短句\"],\n  \"warnings\": [\"需要谨慎表达的内容\"],\n  \"corrections\": [{\"original\": \"原文\", \"corrected\": \"修正后\", \"reason\": \"修正原因\"}],\n  \"passed\": true,\n  \"summary\": \"核查摘要\"\n}"

	case "video_script_generator":
		return fmt.Sprintf(`你是短视频口播稿创作专家。

目标：
根据主题、事实材料和用户风格要求，生成适合中文短视频平台的口播知识分享视频脚本。

硬性要求：
1. 开头 5-8 秒必须有明确钩子。
2. 内容必须基于 facts，不允许编造历史事实。
3. 语言自然，适合真人或 AI 配音口播。
4. 总时长接近 targetDurationSec。
5. 分段清晰：开场、背景、核心讲述、总结。
6. 不要写成论文，不要堆砌百科。
7. 每句话尽量短，适合口播。
8. 输出严格 JSON。

输入：
topic=%s
style=%s

输出 JSON：
{
  "script": "完整口播稿正文",
  "summary": "内容摘要",
  "estimatedDurationSec": 90,
  "sections": [
    {
      "name": "开场",
      "startSec": 0,
      "endSec": 8,
      "text": "..."
    }
  ],
  "qualityHints": {
    "hasHook": true,
    "hasStory": true,
    "hasKnowledgeValue": true
  }
}`, topic, style)

	case "shot_splitter":
		return `你是短视频分镜导演。

目标：
把已确认口播稿拆成适合 AI 视频生成的镜头列表。

硬性要求：
1. 每个 shot 时长 3-15 秒。
2. 每个 shot 只表达一个主要画面变化。
3. 每个 shot 必须包含：shotId、durationSec、scriptText、visual、camera、composition、lighting、transitionIn、transitionOut。
4. 分镜必须覆盖完整口播稿，不要遗漏。
5. 视觉风格默认 16:9，非写实动画，去 AI 感。
6. 输出严格 JSON。

输入：
script=<script>
shotDurationRule=<shotDurationRule>
aspectRatio=<aspectRatio>

输出 JSON：
{
  "shotList": [
    {
      "shotId": "SHOT_01",
      "durationSec": 6,
      "scriptText": "...",
      "visual": "...",
      "camera": "...",
      "composition": "...",
      "lighting": "...",
      "transitionIn": "...",
      "transitionOut": "...",
      "notes": "..."
    }
  ],
  "totalDurationSec": 90,
  "summary": "..."
}`

	case "keyframe_prompt_generator":
		return `你是AI图像关键帧提示词导演。

目标：
根据分镜列表中的关键画面，为每个镜头的代表性画面生成 AI 图像生成提示词。

硬性要求：
1. 每个 shot 生成一个关键帧图像提示词。
2. 提示词必须描述：画面主体、场景、构图、光影、色彩、风格、景别。
3. 视觉风格统一为非写实动画，去 AI 感。
4. 关键帧画面应能代表该镜头的高潮或典型画面。
5. 输出严格 JSON。

输入：
shotList=<shotList>
style=<style>

输出 JSON：
{
  "keyframePrompts": [
    {
      "shotId": "SHOT_01",
      "prompt": "非写实动画风格，...",
      "styleNotes": "..."
    }
  ],
  "summary": "..."
}`

	case "video_prompt_generator":
		return `你是 AI 视频生成提示词导演。

目标：
根据分镜生成每个镜头可直接用于视频生成模型的 Prompt。

硬性要求：
1. 每个 shot 输出一个 video prompt。
2. prompt 必须包含：画面主体、场景、动作、镜头运动、光影、色彩、风格、持续时间、转场。
3. 必须写清楚该镜头内部的时间线变化。
4. 不依赖上下文记忆，因为视频模型每个 shot 独立生成。
5. 禁止真人写实，默认非写实动画，去 AI 感。
6. 输出严格 JSON。

输入：
shotList=<shotList>
keyframePrompts=<keyframePrompts>
style=<style>
modelHint=<modelHint>

输出 JSON：
{
  "videoPrompts": [
    {
      "shotId": "SHOT_01",
      "durationSec": 6,
      "prompt": "...",
      "negativePrompt": "...",
      "continuity": "...",
      "modelTips": {
        "cameraMotion": "...",
        "subjectMotion": "..."
      }
    }
  ],
  "summary": "..."
}`

	case "script_quality_checker":
		return `你是口播稿质量审核员。

检查以下内容：
1. 结构完整性：是否有钩子开头、核心内容、总结
2. 节奏：每段时长是否合理
3. 时长：总时长是否接近目标
4. 知识准确性：是否基于事实
5. 开头吸引力：前 5-8 秒是否有钩子
6. 语言自然度：是否适合口播

输出严格 JSON：
{
  "passed": true/false,
  "score": 0-100,
  "issues": [
    {"level": "error|warning|info", "field": "...", "message": "..."}
  ],
  "repairSuggestions": ["..."]
}`

	case "shot_quality_checker":
		return `你是分镜质量审核员。

检查以下内容：
1. shot 数量是否合理
2. 每个镜头时长是否在 3-15 秒
3. 画面连续性：前后镜头是否衔接
4. 字段完整性：每个 shot 是否包含必要字段
5. 视觉描述是否具体可执行
6. 总时长是否匹配目标

输出严格 JSON：
{
  "passed": true/false,
  "score": 0-100,
  "issues": [
    {"level": "error|warning|info", "field": "...", "message": "..."}
  ],
  "repairSuggestions": ["..."]
}`

	case "video_prompt_quality_checker":
		return `你是视频生成提示词质量审核员。

检查以下内容：
1. 每个镜头是否有对应 video prompt
2. prompt 是否包含主体、场景、动作、镜头运动
3. 是否包含时间线变化描述
4. 风格是否统一（非写实动画，去 AI 感）
5. negativePrompt 是否合理
6. modelTips 是否完整

输出严格 JSON：
{
  "passed": true/false,
  "score": 0-100,
  "issues": [
    {"level": "error|warning|info", "field": "...", "message": "..."}
  ],
  "repairSuggestions": ["..."]
}`

	case "package_quality_checker":
		return `你是视频创作包最终质检员。

检查以下内容：
1. 是否缺少必需文件（research、fact_check、script、shot_list、video_prompts、publish_copy）
2. 各产物是否已审核通过
3. 字段是否完整
4. 格式是否正确
5. 时长、风格是否一致

输出严格 JSON：
{
  "passed": true/false,
  "score": 0-100,
  "issues": [
    {"level": "error|warning|info", "field": "...", "message": "..."}
  ],
  "repairSuggestions": ["..."],
  "missingArtifacts": ["..."],
  "unreviewedArtifacts": ["..."]
}`

	case "publish_copy_generator":
		return fmt.Sprintf("你是一个专业的短视频平台运营专家。根据视频内容生成发布文案。\n\n目标平台：%s\n\n生成：1.视频标题（吸引眼球，不超过30字）2.视频简介（100-200字）3.话题标签（5-8个）4.平台适配建议。输出Markdown。", platform)

	case "video_package_exporter":
		return `你是视频创作包交付经理。

目标：
将所有上游产物（知识研究、事实核查、口播稿、分镜、视频提示词、发布文案、质量报告）打包为完整视频创作包。

硬性要求：
1. 输出两部分：Markdown 总包 + package_manifest.json。
2. Markdown 总包必须包含所有已存在的产物内容摘要。
3. package_manifest.json 必须逐项检查：
   - knowledge_research 是否存在
   - fact_check 是否存在
   - script（口播稿）是否存在且审批通过
   - shot_list（分镜）是否存在且审批通过
   - video_prompts（视频提示词）是否存在且审批通过
   - publish_copy（发布文案）是否存在
   - quality_report（质量报告）是否存在
4. 所有核心产物（script/shot_list/video_prompts）均审批通过 → status = "READY_FOR_PRODUCTION"
5. 缺少任一必须产物或未审批 → status = "INCOMPLETE"，并在 issues 中列出缺失项
6. 输出严格 JSON，包含 Markdown 正文和 manifest。

输出 JSON：
{
  "packageMarkdown": "# 视频创作包：...\n\n## 1. 项目概览\n...\n## 2. 知识资料整理\n...",
  "packageManifest": {
    "packageVersion": "1.0.0",
    "topic": "...",
    "platform": "...",
    "aspectRatio": "16:9",
    "targetDurationSec": 90,
    "status": "READY_FOR_PRODUCTION",
    "artifacts": [
      {"kind": "KNOWLEDGE_RESEARCH", "name": "01_knowledge_research.md", "reviewStatus": "APPROVED"},
      {"kind": "FACT_CHECK", "name": "02_fact_check.md", "reviewStatus": "APPROVED"},
      {"kind": "SCRIPT", "name": "03_oral_script.md", "reviewStatus": "APPROVED"},
      {"kind": "SHOT_LIST", "name": "04_shot_list.json", "reviewStatus": "APPROVED"},
      {"kind": "VIDEO_PROMPTS", "name": "07_video_prompts.md", "reviewStatus": "APPROVED"},
      {"kind": "PUBLISH_COPY", "name": "08_publish_copy.md", "reviewStatus": "NONE"},
      {"kind": "QUALITY_REPORT", "name": "09_quality_report.md", "reviewStatus": "NONE"}
    ],
    "quality": {"passed": true, "score": 88},
    "issues": [],
    "missingArtifacts": [],
    "unreviewedArtifacts": []
  }
}`

	default:
		return fmt.Sprintf("你是一个专业的自媒体内容创作助手。当前阶段：%s。根据用户需求生成高质量内容。", toolName)
	}
}

func buildDynamicAgentUserPrompt(toolName, topic, facts, style, script, shotList, videoPrompts, publishCopy, platform string) string {
	switch toolName {
	case "knowledge_researcher":
		return fmt.Sprintf("请围绕以下主题进行深度知识研究：%s\n\n输出风格：%s", topic, style)

	case "fact_checker":
		return fmt.Sprintf("请核查以下内容的准确性：\n\n%s\n\n原始主题：%s", facts, topic)

	case "video_script_generator":
		var parts []string
		parts = append(parts, "主题："+topic)
		if facts != "" {
			parts = append(parts, "事实材料：\n"+facts)
		}
		if style != "" {
			parts = append(parts, "风格要求："+style)
		}
		return strings.Join(parts, "\n\n")

	case "shot_splitter":
		var parts []string
		if script != "" {
			parts = append(parts, "口播稿：\n"+script)
		}
		parts = append(parts, "分镜时长规则：3-15秒")
		parts = append(parts, "画幅：16:9")
		return strings.Join(parts, "\n\n")

	case "keyframe_prompt_generator":
		var parts []string
		if shotList != "" {
			parts = append(parts, "分镜列表：\n"+shotList)
		}
		if style != "" {
			parts = append(parts, "风格要求："+style)
		}
		return strings.Join(parts, "\n\n")

	case "video_prompt_generator":
		var parts []string
		if shotList != "" {
			parts = append(parts, "分镜列表：\n"+shotList)
		}
		if style != "" {
			parts = append(parts, "风格要求："+style)
		}
		return strings.Join(parts, "\n\n")

	case "script_quality_checker":
		return fmt.Sprintf("请检查以下口播稿的质量：\n\n%s\n\n目标时长：90秒", script)

	case "shot_quality_checker":
		return fmt.Sprintf("请检查以下分镜的质量：\n\n%s", shotList)

	case "video_prompt_quality_checker":
		return fmt.Sprintf("请检查以下视频提示词的质量：\n\n%s", videoPrompts)

	case "package_quality_checker":
		var parts []string
		parts = append(parts, "主题："+topic)
		if script != "" {
			parts = append(parts, "口播稿：\n"+script)
		}
		if shotList != "" {
			parts = append(parts, "分镜：\n"+shotList)
		}
		if videoPrompts != "" {
			parts = append(parts, "视频提示词：\n"+videoPrompts)
		}
		if publishCopy != "" {
			parts = append(parts, "发布文案：\n"+publishCopy)
		}
		return "请对以下视频创作包进行最终质检：\n\n" + strings.Join(parts, "\n\n---\n\n")

	case "publish_copy_generator":
		var parts []string
		if script != "" {
			parts = append(parts, "口播稿：\n"+script)
		}
		if shotList != "" {
			parts = append(parts, "分镜：\n"+shotList)
		}
		parts = append(parts, "主题："+topic)
		parts = append(parts, "目标平台："+platform)
		return strings.Join(parts, "\n\n")

	case "video_package_exporter":
		var parts []string
		parts = append(parts, "请将以下所有产物打包为完整视频创作包。")
		parts = append(parts, "主题："+topic)
		if script != "" {
			parts = append(parts, "口播稿（已审核通过）：\n"+script)
		}
		if shotList != "" {
			parts = append(parts, "分镜（已审核通过）：\n"+shotList)
		}
		if videoPrompts != "" {
			parts = append(parts, "视频提示词（已审核通过）：\n"+videoPrompts)
		}
		if facts != "" {
			parts = append(parts, "知识研究资料：\n"+facts)
		}
		if publishCopy != "" {
			parts = append(parts, "发布文案：\n"+publishCopy)
		}
		parts = append(parts, "请输出：1. Markdown 总包 2. package_manifest.json")
		parts = append(parts, "注意：如果核心产物（script/shotList/videoPrompts）非空，则视为已审核通过，status 应为 READY_FOR_PRODUCTION。")
		parts = append(parts, "如果有任何核心产物为空，status 应为 INCOMPLETE，并在 missingArtifacts 中列出。")
		return strings.Join(parts, "\n\n---\n\n")

	default:
		return fmt.Sprintf("请围绕主题「%s」生成内容。", topic)
	}
}
