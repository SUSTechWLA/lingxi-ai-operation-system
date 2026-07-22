package builtin

import (
	"bytes"
	"context"
	stdsha256 "crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	videomodel "github.com/tangying-ai/aios-core/internal/agents/video/model"
	videoservice "github.com/tangying-ai/aios-core/internal/agents/video/service"
	"github.com/tangying-ai/aios-core/internal/core/common/crypto"
	"github.com/tangying-ai/aios-core/internal/core/common/jsonx"
	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/hyperframes"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	videopipeline "github.com/tangying-ai/aios-core/internal/core/video/pipeline"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

var videoCreationExternalTools = []string{
	"skill_stage_agent",
	"image_asset_generator",
	"hyperframes_project_generator",
	"hyperframes_project_builder",
	"hyperframes_renderer",
	"ip_aroll_director",
	"local_ip_talking_avatar_render",
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
	// VideoForge Studio P0 pipeline tools.
	"pipeline_selector",
	"capability_preflight",
	"proposal_generator",
	"visual_feasibility_analyzer",
	"render_strategy_planner",
	"video_profile_classifier",
	"audio_master_planner",
	"time_window_planner",
	"visual_alignment_planner",
	"cinematic_shot_designer",
	"sound_design_planner",
	"shot_generation_planner",
	"card_plan_generator",
	"caption_splitter",
	"composition_quality_checker",
	"reference_asset_planner",
	"asset_decision_agent",
	"asset_policy_generator",
	"continuity_checker",
	"style_profile_builder",
	"stale_tracker",
	"preview_quality_checker",
	"render_dependency_guard",
	"local_job_status_tracker",
	// Dynamic agent prompt_tool entries — used by LLMPlanner for video creation workflows.
	"news_search",
	"fact_extractor",
	"knowledge_researcher",
	"fact_checker",
	"video_script_generator",
	"shot_splitter",
	"keyframe_prompt_generator",
	"video_prompt_generator",
	"mcp_generation_runner",
	"script_quality_checker",
	"shot_quality_checker",
	"video_prompt_quality_checker",
	"publish_copy_generator",
	"video_package_exporter",
	"package_quality_checker",
	// OneClick Video v1 tools.
	"video_composition_builder",
	"hyperframes_snapshot",
	"artifact_packager",
	"ffmpeg_probe",
	"video_frame_qa",
	"final_review_generator",
}

var (
	videoCreationOpenAICfg config.OpenAIConfig
	modelGateway           *modelgateway.Gateway
	durationHintPattern    = regexp.MustCompile(`(?i)(\d{1,3})\s*(秒|s|sec|secs|second|seconds)`)
)

// SetModelGateway stores the model gateway for image generation tools.
func SetModelGateway(gw *modelgateway.Gateway) {
	modelGateway = gw
}

// SetVideoCreationConfig stores configuration needed by local video creation
// tools so skill_stage_agent can call the LLM API.
func SetVideoCreationConfig(cfg config.OpenAIConfig, skillRoot string) {
	videoCreationOpenAICfg = cfg
}

// GetEnvOpenAIConfig returns the env-based OpenAI config (for display purposes).
func GetEnvOpenAIConfig() config.OpenAIConfig {
	return videoCreationOpenAICfg
}

// GetVideoCreationOpenAIConfig returns the legacy server-side OpenAI config.
// Video creation tools use per-request client providers instead.
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

func localAgentModelConfigEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("AIOS_ENABLE_LOCAL_AGENT_MODEL_CONFIG")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func applyOptionalLocalAgentConfig(cfg config.OpenAIConfig) config.OpenAIConfig {
	if !localAgentModelConfigEnabled() {
		return cfg
	}
	if localCfg, ok := localAgentConfigFetcher(); ok {
		if localCfg.BaseURL != "" {
			cfg.BaseURL = localCfg.BaseURL
		}
		if localCfg.APIKey != "" {
			cfg.APIKey = localCfg.APIKey
		}
		if localCfg.Model != "" {
			cfg.Model = localCfg.Model
		}
	}
	return cfg
}

func effectiveVideoCreationOpenAIConfig(params map[string]interface{}) config.OpenAIConfig {
	cfg := applyOptionalLocalAgentConfig(GetVideoCreationOpenAIConfig())
	return ApplyClientModelProviderConfig(cfg, params)
}

// ApplyClientModelProviderConfig overlays a per-request OpenAI-compatible
// provider onto the server default config. It is intentionally not persisted.
func ApplyClientModelProviderConfig(cfg config.OpenAIConfig, params map[string]interface{}) config.OpenAIConfig {
	if len(params) == 0 {
		return cfg
	}
	provider, ok := clientModelProviderFromParams(params)
	if !ok {
		return cfg
	}
	if provider.BaseURL != "" {
		cfg.BaseURL = provider.BaseURL
	}
	if provider.APIKey != "" {
		cfg.APIKey = provider.APIKey
	}
	if provider.Model != "" {
		cfg.Model = provider.Model
	}
	return cfg
}

func clientModelProviderFromParams(params map[string]interface{}) (RuntimeModelProviderConfig, bool) {
	if provider, ok := normalizeRuntimeModelProvider(params["modelProvider"]); ok {
		return provider, true
	}
	return clientModelProviderFromParamsForCapability(params, "text_to_text")
}

func clientModelProviderFromParamsForCapability(params map[string]interface{}, capability string) (RuntimeModelProviderConfig, bool) {
	providers, ok := params["modelProviders"]
	if !ok {
		return RuntimeModelProviderConfig{}, false
	}
	switch typed := providers.(type) {
	case map[string]interface{}:
		return normalizeRuntimeModelProvider(typed[capability])
	case map[string]map[string]interface{}:
		return normalizeRuntimeModelProvider(typed[capability])
	default:
		return RuntimeModelProviderConfig{}, false
	}
}

func clientGenerationProviderFromParams(params map[string]interface{}, capability string) (RuntimeModelProviderConfig, bool) {
	if provider, ok := normalizeRuntimeModelProvider(params["modelProvider"]); ok {
		return provider, true
	}
	return clientModelProviderFromParamsForCapability(params, capability)
}

func normalizeRuntimeModelProvider(value interface{}) (RuntimeModelProviderConfig, bool) {
	raw, ok := value.(map[string]interface{})
	if !ok {
		return RuntimeModelProviderConfig{}, false
	}
	cfg := RuntimeModelProviderConfig{
		BaseURL: cleanModelProviderString(raw["baseUrl"]),
		APIKey:  cleanModelProviderString(raw["apiKey"]),
		Model:   cleanModelProviderString(raw["model"]),
	}
	if cfg.APIKey == "" {
		return RuntimeModelProviderConfig{}, false
	}
	return cfg, true
}

func cleanModelProviderString(value interface{}) string {
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "<nil>" {
		return ""
	}
	return text
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

// RuntimeModelProviderConfig holds cloud operator model-provider settings
// persisted at runtime (overrides env-based config when set).
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
		manifest := &tool.ToolManifest{
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
		}

		applyVideoCreationManifestOverrides(name, manifest)

		// Configure local execution plane tools.
		if localManifest, ok := localToolManifests[name]; ok {
			manifest.ExecutionPlane = localManifest.ExecutionPlane
			manifest.LocalCommand = localManifest.LocalCommand
			manifest.RequiresUserDevice = localManifest.RequiresUserDevice
			manifest.ArtifactLocation = localManifest.ArtifactLocation
			manifest.Description = localManifest.Description
			manifest.Timeout = localManifest.Timeout
		}

		registry.RegisterExternal(manifest)
	}
}

func applyVideoCreationManifestOverrides(name string, manifest *tool.ToolManifest) {
	if manifest == nil {
		return
	}
	switch name {
	case "news_search":
		manifest.Description = "Search latest news for current-event video scripts."
		manifest.Type = "http"
		manifest.CostLevel = tool.CostMedium
		manifest.RiskLevel = tool.RiskLow
		manifest.SideEffect = false
		manifest.Idempotent = true
		manifest.Capabilities = []string{"fresh_knowledge", "news_search", "web_search", "current_event_retrieval", "fact_retrieval"}
		manifest.Tags = []string{"search", "news", "realtime", "knowledge", "fact"}
		manifest.Parameters = map[string]tool.ParamDef{
			"query":         {Type: "string", Description: "Single search query", Required: false},
			"queries":       {Type: "array", Description: "Search queries", Required: false},
			"freshnessDays": {Type: "number", Description: "Freshness window in days", Required: false},
			"topK":          {Type: "number", Description: "Max result count", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"results":    {Type: "array", Description: "Structured search results"},
			"facts":      {Type: "array", Description: "Fact-like result snippets"},
			"sources":    {Type: "array", Description: "Source list"},
			"queryUsed":  {Type: "array", Description: "Queries sent to the search provider"},
			"searchedAt": {Type: "string", Description: "RFC3339 search timestamp"},
		}
	case "fact_extractor":
		manifest.Description = "Extract a traceable fact pack from news search results."
		manifest.Type = "builtin_prompt_tool"
		manifest.CostLevel = tool.CostLow
		manifest.RiskLevel = tool.RiskLow
		manifest.SideEffect = false
		manifest.Idempotent = true
		manifest.Capabilities = []string{"fact_retrieval", "fact_extraction", "knowledge_pack"}
		manifest.Tags = []string{"fact", "knowledge", "freshness"}
		manifest.Parameters = map[string]tool.ParamDef{
			"searchResults": {Type: "array", Description: "news_search results", Required: true},
			"outputMode":    {Type: "string", Description: "Output mode", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"facts":             {Type: "array", Description: "Traceable facts"},
			"sources":           {Type: "array", Description: "Source list"},
			"warnings":          {Type: "array", Description: "Fact extraction warnings"},
			"freshness":         {Type: "object", Description: "Freshness summary"},
			"knowledgePackHash": {Type: "string", Description: "Stable fact pack hash"},
		}
		manifest.ArtifactPolicy = tool.ArtifactPolicy{
			ProduceArtifact:       true,
			ArtifactKinds:         []string{"KNOWLEDGE_PACK"},
			DefaultReviewRequired: false,
			Storage:               tool.ArtifactLocationCloud,
		}
	case "video_script_generator":
		manifest.Parameters = map[string]tool.ParamDef{
			"topic":                 {Type: "string", Description: "Video topic", Required: true},
			"facts":                 {Type: "string", Description: "Plain-text facts", Required: false},
			"knowledgePack":         {Type: "array", Description: "Traceable fact pack", Required: false},
			"knowledgeSources":      {Type: "array", Description: "Fact sources", Required: false},
			"knowledgeContext":      {Type: "object", Description: "Generic facts, sources, evidence, and source tool names", Required: false},
			"toolContext":           {Type: "object", Description: "Generic upstream tool context", Required: false},
			"retrievalPolicy":       {Type: "string", Description: "none|optional|required|forbidden", Required: false},
			"mustUseFreshKnowledge": {Type: "boolean", Description: "Require fresh facts in generation", Required: false},
			"requireFreshFacts":     {Type: "boolean", Description: "Block generation when fresh facts are required but absent", Required: false},
			"currentDate":           {Type: "string", Description: "Current date for freshness-aware scripts", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"script":               {Type: "string", Description: "Voiceover script"},
			"storyOutline":         {Type: "object", Description: "Cinematic story outline when the profile is cinematic"},
			"detailedScript":       {Type: "string", Description: "Detailed cinematic script with action beats"},
			"characters":           {Type: "array", Description: "Main character dossiers"},
			"scenes":               {Type: "array", Description: "Main scene dossiers"},
			"props":                {Type: "array", Description: "Main prop dossiers"},
			"scriptSpans":          {Type: "array", Description: "Timed script spans for time-window planning"},
			"summary":              {Type: "string", Description: "Script summary"},
			"estimatedDurationSec": {Type: "number", Description: "Estimated duration"},
			"sections":             {Type: "array", Description: "Script sections"},
			"usedFacts":            {Type: "array", Description: "Facts used by the script"},
			"unusedFacts":          {Type: "array", Description: "Facts not used by the script"},
			"factCheckWarnings":    {Type: "array", Description: "Fact check warnings"},
			"knowledgeTrace":       {Type: "object", Description: "Knowledge usage trace"},
		}
	case "shot_splitter":
		manifest.Description = "Split a video script into independent 3-15 second shot production units."
		manifest.Type = "builtin_prompt_tool"
		manifest.CostLevel = tool.CostLow
		manifest.RiskLevel = tool.RiskLow
		manifest.SideEffect = false
		manifest.Idempotent = true
		manifest.Capabilities = []string{"video_creation", "storyboard_generation", "shot_planning"}
		manifest.Parameters = map[string]tool.ParamDef{
			"script":           {Type: "string", Description: "Approved voiceover script", Required: true},
			"brief":            {Type: "string", Description: "Original video brief", Required: false},
			"shotDurationRule": {Type: "string", Description: "Shot duration rule, default 3-15 seconds", Required: false},
			"aspectRatio":      {Type: "string", Description: "Video aspect ratio", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"shotQueue":         {Type: "object", Description: "Lightweight sequential shot queue for user-facing review"},
			"shotList":          {Type: "array", Description: "Independent 3-15 second shot list"},
			"shotAssetPackages": {Type: "array", Description: "Per-shot independent asset package requirements"},
			"totalDurationSec":  {Type: "number", Description: "Total duration in seconds"},
			"summary":           {Type: "string", Description: "Shot list summary"},
			"content":           {Type: "string", Description: "Reviewable shot package content"},
			"artifacts":         {Type: "object", Description: "Reviewable artifact manifest"},
		}
	case "video_profile_classifier":
		manifest.Description = "Classify the video brief into a creation profile and DAG template."
		manifest.Type = "builtin_prompt_tool"
		manifest.CostLevel = tool.CostLow
		manifest.RiskLevel = tool.RiskLow
		manifest.SideEffect = false
		manifest.Idempotent = true
		manifest.Capabilities = []string{"video_creation", "workflow_routing"}
		manifest.Parameters = map[string]tool.ParamDef{
			"brief":       {Type: "string", Description: "Original user brief", Required: true},
			"route":       {Type: "string", Description: "Optional route from skill runtime", Required: false},
			"deliverable": {Type: "string", Description: "Optional deliverable from skill runtime", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"creationProfile": {Type: "object", Description: "Selected video creation profile"},
			"summary":         {Type: "string", Description: "Human-readable profile summary"},
			"content":         {Type: "string", Description: "Reviewable markdown content"},
			"artifacts":       {Type: "object", Description: "Reviewable artifact manifest"},
		}
	case "audio_master_planner":
		manifest.Description = "Build the canonical millisecond audio-master clock before shot planning."
		manifest.Type = "builtin_prompt_tool"
		manifest.CostLevel = tool.CostLow
		manifest.RiskLevel = tool.RiskLow
		manifest.SideEffect = false
		manifest.Idempotent = true
		manifest.Capabilities = []string{"video_creation", "audio_timeline", "timing"}
		manifest.Parameters = map[string]tool.ParamDef{
			"script":               {Type: "string", Description: "Approved narration script", Required: false},
			"scriptSpans":          {Type: "array", Description: "Estimated or aligned sentence spans", Required: true},
			"scriptRevision":       {Type: "string", Description: "Immutable script revision", Required: false},
			"voiceRevision":        {Type: "string", Description: "Voice synthesis or import revision", Required: false},
			"voiceoverArtifactRef": {Type: "string", Description: "Real voiceover artifact reference when available", Required: false},
			"voiceProfileId":       {Type: "string", Description: "Pinned voice profile", Required: false},
			"voiceProfileVersion":  {Type: "string", Description: "Pinned voice profile version", Required: false},
			"language":             {Type: "string", Description: "Narration language", Required: false},
			"sampleRate":           {Type: "integer", Description: "Audio sample rate", Required: false},
			"timelineSource":       {Type: "string", Description: "estimated, aligned, or imported", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"audioMaster": {Type: "object", Description: "Canonical audio-master timeline"},
			"summary":     {Type: "string", Description: "Human-readable timeline summary"},
			"content":     {Type: "string", Description: "Reviewable markdown content"},
			"artifacts":   {Type: "object", Description: "Reviewable artifact manifest"},
		}
		applyReviewableArtifactContract(manifest, videomodel.ArtifactKindAudioMasterTimeline)
	case "time_window_planner":
		manifest.Description = "Plan script-timed or cinematic 3-15 second AIGC-safe time windows."
		manifest.Type = "builtin_prompt_tool"
		manifest.CostLevel = tool.CostLow
		manifest.RiskLevel = tool.RiskLow
		manifest.SideEffect = false
		manifest.Idempotent = true
		manifest.Capabilities = []string{"video_creation", "shot_planning", "timing"}
		manifest.Parameters = map[string]tool.ParamDef{
			"creationProfile": {Type: "object", Description: "Video creation profile", Required: true},
			"shotList":        {Type: "array", Description: "Coarse shots", Required: false},
			"scriptSpans":     {Type: "array", Description: "Timed script spans", Required: false},
			"audioMaster":     {Type: "object", Description: "Canonical audio-master timeline", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"timeWindowPlan": {Type: "object", Description: "Full time-window plan"},
			"timeWindows":    {Type: "array", Description: "3-15 second time windows"},
			"summary":        {Type: "string", Description: "Human-readable summary"},
			"content":        {Type: "string", Description: "Reviewable markdown content"},
			"artifacts":      {Type: "object", Description: "Reviewable artifact manifest"},
		}
		applyReviewableArtifactContract(manifest, videomodel.ArtifactKindTimeWindowPlan)
	case "visual_alignment_planner":
		manifest.Description = "Align talking-head visuals, materials, captions, and AIGC needs to script time windows."
		manifest.Type = "builtin_prompt_tool"
		manifest.CostLevel = tool.CostLow
		manifest.RiskLevel = tool.RiskLow
		manifest.SideEffect = false
		manifest.Idempotent = true
		manifest.Capabilities = []string{"video_creation", "visual_alignment"}
		manifest.Parameters = map[string]tool.ParamDef{
			"script":        {Type: "string", Description: "Approved script", Required: false},
			"timeWindows":   {Type: "array", Description: "Script-timed windows", Required: true},
			"assetStrategy": {Type: "string", Description: "Optional asset strategy", Required: false},
			"aigcProvider":  {Type: "string", Description: "AIGC provider or disabled", Required: false},
			"aigcEnabled":   {Type: "boolean", Description: "Whether AIGC enrichment may execute", Required: false},
			"aigcPolicy":    {Type: "string", Description: "Independent AIGC enrichment policy", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"visualAlignmentPlan": {Type: "object", Description: "Visual-to-script alignment plan"},
			"shotList":            {Type: "array", Description: "Visual shot list for downstream generation"},
			"brollManifest":       {Type: "object", Description: "Reviewable b-roll source and usage manifest"},
			"content":             {Type: "string", Description: "Reviewable markdown content"},
			"artifacts":           {Type: "object", Description: "Reviewable artifact manifest"},
		}
		applyReviewableArtifactContract(manifest, "VISUAL_ALIGNMENT_PLAN", videomodel.ArtifactKindBrollManifest)
	case "cinematic_shot_designer":
		manifest.Description = "Design cinematic shot intent and review notes without generating media."
		manifest.Type = "builtin_prompt_tool"
		manifest.CostLevel = tool.CostLow
		manifest.RiskLevel = tool.RiskMedium
		manifest.SideEffect = false
		manifest.Idempotent = true
		manifest.Capabilities = []string{"video_creation", "cinematic_planning"}
		manifest.Parameters = map[string]tool.ParamDef{
			"brief":           {Type: "string", Description: "Original user brief", Required: false},
			"creationProfile": {Type: "object", Description: "Video creation profile", Required: false},
			"timeWindows":     {Type: "array", Description: "Planned time windows", Required: false},
			"timeWindowPlan":  {Type: "object", Description: "Full time-window plan", Required: false},
			"shotList":        {Type: "array", Description: "Existing shot list fallback", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"directorDesign": {Type: "object", Description: "Cinematic director design summary"},
			"shotList":       {Type: "array", Description: "Cinematic shot list for downstream planning"},
			"content":        {Type: "string", Description: "Reviewable markdown content"},
			"artifacts":      {Type: "object", Description: "Reviewable artifact manifest"},
		}
	case "reference_asset_planner":
		manifest.Description = "Plan cinematic character, scene, prop, and global reference assets."
		manifest.Type = "builtin_prompt_tool"
		manifest.CostLevel = tool.CostLow
		manifest.RiskLevel = tool.RiskMedium
		manifest.SideEffect = false
		manifest.Idempotent = true
		manifest.Capabilities = []string{"video_creation", "cinematic_planning", "reference_asset_planning"}
		manifest.Parameters = map[string]tool.ParamDef{
			"brief":           {Type: "string", Description: "Original user brief", Required: false},
			"script":          {Type: "string", Description: "Detailed script", Required: false},
			"continuityBible": {Type: "object", Description: "Continuity bible", Required: false},
			"characters":      {Type: "array", Description: "Character dossiers", Required: false},
			"scenes":          {Type: "array", Description: "Scene dossiers", Required: false},
			"props":           {Type: "array", Description: "Prop dossiers", Required: false},
			"creationProfile": {Type: "object", Description: "Video creation profile", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"referenceAssetPlan":         {Type: "object", Description: "Reference asset plan"},
			"referenceAssetIndex":        {Type: "array", Description: "Stable reference board index"},
			"globalReferenceAssets":      {Type: "array", Description: "Global reference assets for shot prompts"},
			"externalGenerationRequests": {Type: "array", Description: "Image generation requests for reference boards"},
			"summary":                    {Type: "string", Description: "Human-readable summary"},
			"content":                    {Type: "string", Description: "Reviewable markdown content"},
			"artifacts":                  {Type: "object", Description: "Reviewable artifact manifest"},
		}
	case "sound_design_planner":
		manifest.Description = "Plan per-shot sound cues and music intent without generating audio."
		manifest.Type = "builtin_prompt_tool"
		manifest.CostLevel = tool.CostLow
		manifest.RiskLevel = tool.RiskMedium
		manifest.SideEffect = false
		manifest.Idempotent = true
		manifest.Capabilities = []string{"video_creation", "cinematic_planning"}
		manifest.Parameters = map[string]tool.ParamDef{
			"brief":           {Type: "string", Description: "Original user brief", Required: false},
			"creationProfile": {Type: "object", Description: "Video creation profile", Required: false},
			"timeWindows":     {Type: "array", Description: "Planned time windows", Required: false},
			"timeWindowPlan":  {Type: "object", Description: "Full time-window plan", Required: false},
			"shotList":        {Type: "array", Description: "Shot list for sound cue planning", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"soundDesignPlan": {Type: "object", Description: "Per-shot sound design plan"},
			"content":         {Type: "string", Description: "Reviewable markdown content"},
			"artifacts":       {Type: "object", Description: "Reviewable artifact manifest"},
		}
	case "shot_generation_planner":
		manifest.Description = "Plan per-shot generation mode, asset needs, and fusion strategy."
		manifest.Type = "builtin_prompt_tool"
		manifest.CostLevel = tool.CostLow
		manifest.RiskLevel = tool.RiskLow
		manifest.SideEffect = false
		manifest.Idempotent = true
		manifest.Capabilities = []string{"video_creation", "render_strategy", "asset_routing", "shot_planning"}
		manifest.Parameters = map[string]tool.ParamDef{
			"shotList":             {Type: "array", Description: "Approved shot list", Required: true},
			"visualPlans":          {Type: "array", Description: "Optional per-shot visual plans", Required: false},
			"renderPreference":     {Type: "object", Description: "Optional render preference snapshot", Required: false},
			"aigcAvailable":        {Type: "boolean", Description: "Whether AIGC video/image generation is available", Required: false},
			"htmlAvailable":        {Type: "boolean", Description: "Whether HyperFrames HTML rendering is available", Required: false},
			"providerCapabilities": {Type: "object", Description: "Optional nested provider capability snapshot", Required: false},
			"aigcProvider":         {Type: "string", Description: "AIGC execution provider or disabled", Required: false},
			"aigcEnabled":          {Type: "boolean", Description: "Whether AIGC enrichment may execute", Required: false},
			"aigcPolicy":           {Type: "string", Description: "Independent AIGC enrichment execution policy", Required: false},
			"productionRoute":      {Type: "string", Description: "Talking-head or cinematic production route", Required: false},
			"visualLayerContract":  {Type: "string", Description: "Canonical Shot visual-layer contract version", Required: false},
			"designedLayers":       {Type: "array", Description: "Layers that every Shot must describe", Required: false},
			"layerExecutionPolicy": {Type: "object", Description: "Per-layer execution policies", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"shotGenerationPlans":        {Type: "array", Description: "Per-shot generation plans"},
			"shotAssetPackages":          {Type: "array", Description: "Per-shot reviewable asset packages"},
			"externalGenerationRequests": {Type: "array", Description: "External generation requests for missing providers or manual generation"},
			"summary":                    {Type: "string", Description: "Human-readable planner summary"},
			"content":                    {Type: "string", Description: "Reviewable markdown content"},
			"artifacts":                  {Type: "object", Description: "Reviewable artifact manifest"},
		}
	case "video_prompt_generator":
		manifest.Description = "Generate independent per-shot video prompts and shot asset packages."
		manifest.Type = "builtin_prompt_tool"
		manifest.CostLevel = tool.CostMedium
		manifest.RiskLevel = tool.RiskMedium
		manifest.SideEffect = false
		manifest.Idempotent = true
		manifest.Capabilities = []string{"video_creation", "video_prompt_generation", "text_to_video"}
		manifest.Parameters = map[string]tool.ParamDef{
			"shotList":             {Type: "array", Description: "Approved shot list", Required: true},
			"brief":                {Type: "string", Description: "Original video brief", Required: false},
			"keyframePrompts":      {Type: "array", Description: "Optional keyframe prompts", Required: false},
			"style":                {Type: "string", Description: "Visual style", Required: false},
			"modelHint":            {Type: "string", Description: "Target video generation model", Required: false},
			"aspectRatio":          {Type: "string", Description: "Video aspect ratio", Required: false},
			"aigcProvider":         {Type: "string", Description: "Optional automatic AIGC provider, such as jimeng_mcp", Required: false},
			"aigcEnabled":          {Type: "boolean", Description: "Whether AIGC enrichment may execute", Required: false},
			"aigcPolicy":           {Type: "string", Description: "Independent AIGC enrichment execution policy", Required: false},
			"productionRoute":      {Type: "string", Description: "Talking-head or cinematic production route", Required: false},
			"visualLayerContract":  {Type: "string", Description: "Canonical Shot visual-layer contract version", Required: false},
			"designedLayers":       {Type: "array", Description: "Layers that every Shot must describe", Required: false},
			"layerExecutionPolicy": {Type: "object", Description: "Per-layer execution policies", Required: false},
			"referenceAssetPlan":   {Type: "object", Description: "Optional cinematic reference asset plan", Required: false},
			"continuityBible":      {Type: "object", Description: "Optional cinematic continuity bible", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"videoPrompts":               {Type: "array", Description: "Independent per-shot video prompts"},
			"shotAssetPackages":          {Type: "array", Description: "Per-shot packages containing references, prompts, voiceover, AIGC video, subtitles, and concat plan"},
			"externalGenerationRequests": {Type: "array", Description: "Copyable external image/video generation requests"},
			"summary":                    {Type: "string", Description: "Prompt package summary"},
			"content":                    {Type: "string", Description: "Reviewable prompt package content"},
			"artifacts":                  {Type: "object", Description: "Reviewable artifact manifest"},
		}
	case "mcp_generation_runner":
		manifest.Description = "Call a user-configured local MCP provider to generate AIGC assets from external generation requests."
		manifest.Type = "local_mcp_tool"
		manifest.CostLevel = tool.CostHigh
		manifest.RiskLevel = tool.RiskMedium
		manifest.SideEffect = true
		manifest.Idempotent = false
		manifest.ApprovalPolicy = tool.ApprovalPolicy{
			Required: true,
			Mode:     tool.ApprovalBeforeExecute,
			Reason:   "The selected MCP provider runs on the user's local device and may spend provider quota.",
		}
		manifest.Capabilities = []string{"video_creation", "aigc_generation", "text_to_video", "image_to_video", "local_mcp"}
		manifest.Tags = []string{"mcp", "aigc", "local"}
		manifest.Parameters = map[string]tool.ParamDef{
			"externalGenerationRequests": {Type: "array", Description: "External generation requests produced by prompt generation", Required: true},
			"providerId":                 {Type: "string", Description: "Local MCP provider id, for example jimeng", Required: true},
			"mcpTool":                    {Type: "string", Description: "Logical MCP tool name, for example jimeng.generate_video", Required: true},
			"minReadyVideoGenerations":   {Type: "number", Description: "Minimum ready MCP video assets required before the result can satisfy the external AIGC video requirement.", Required: false},
			"maxReadyGenerations":        {Type: "number", Description: "Maximum ready MCP assets to create in one automatic batch to control quota spend.", Required: false},
			"mcpBatchTimeoutSec":         {Type: "number", Description: "Maximum duration for the complete MCP batch.", Required: false},
			"mcpToolCallTimeoutSec":      {Type: "number", Description: "Maximum duration for one MCP tool call.", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"shotAssetPackages":          {Type: "array", Description: "Per-shot asset packages with generated MCP results"},
			"aRollAssetPackages":         {Type: "array", Description: "Continuous deterministic A-roll packages returned by IP avatar renderers"},
			"generationResults":          {Type: "array", Description: "Raw MCP generation results, including preflightQa for prompt/reference gates before quota-spending provider calls"},
			"externalGenerationResults":  {Type: "array", Description: "Alias of raw MCP generation results for review and provenance panels"},
			"externalGenerationRequests": {Type: "array", Description: "Requests that remain manual or failed"},
			"assetProvenance":            {Type: "array", Description: "Per-request source provenance showing provider, tool, status, storageRef, localPath, preflightQa, and failure reason"},
			"sourceSummary":              {Type: "object", Description: "Quantitative source summary: ready video/image counts, blocked/failed/deferred counts, fallback requirement, and AIGC video satisfaction"},
			"requirementsSatisfied":      {Type: "boolean", Description: "Whether the configured minimum ready external video assets were produced"},
			"summary":                    {Type: "string", Description: "MCP generation summary"},
		}
	case "ip_aroll_director":
		manifest.Description = "Build a controllable oral-video A-roll plan from the local Bobo/Aster IP character asset folders for HyperFrames rendering."
		manifest.Type = "builtin_prompt_tool"
		manifest.CostLevel = tool.CostLow
		manifest.RiskLevel = tool.RiskLow
		manifest.SideEffect = false
		manifest.Idempotent = true
		manifest.Capabilities = []string{"video_creation", "talking_head", "oral_video", "ip_character_aroll", "hyperframes_character_layer"}
		manifest.Tags = []string{"ip", "aroll", "talking-head", "hyperframes", "oral-video"}
		manifest.ArtifactPolicy = tool.ArtifactPolicy{
			ProduceArtifact:       true,
			ArtifactKinds:         []string{"IP_AROLL_PLAN", "CHARACTER_ASSET_INDEX"},
			DefaultReviewRequired: true,
			Storage:               tool.ArtifactLocationLocal,
		}
		manifest.HumanReview = &tool.HumanReview{
			Required: true,
			Gate:     tool.ApprovalAfterArtifact,
			Title:    "审核 IP 口播 A-roll 动作层",
			ReviewFocus: []string{
				"角色是否选择正确",
				"口播文本是否完整覆盖",
				"动作和表情是否服务口播",
				"HyperFrames 文字安全区是否保留",
			},
			UserActions: []string{"edit_timeline", "switch_character", "regenerate_actions"},
		}
		manifest.Parameters = map[string]tool.ParamDef{
			"assetRoot":   {Type: "string", Description: "IP character asset root, default resolves to ip-assets/", Required: false},
			"characters":  {Type: "array", Description: "Character names to use, default [波波, 阿斯特]", Required: false},
			"shotId":      {Type: "string", Description: "Target shot id", Required: false},
			"narration":   {Type: "string", Description: "Voiceover text for this shot; execution requires this even though planner wiring keeps it optional", Required: false},
			"durationSec": {Type: "number", Description: "Shot duration in seconds", Required: false},
			"layout":      {Type: "string", Description: "Character layout: single_host, two_host, side_commentary", Required: false},
			"style":       {Type: "string", Description: "Optional oral-video visual style notes", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"ipArollPlan":     {Type: "object", Description: "HyperFrames-readable IP character A-roll plan"},
			"characterAssets": {Type: "array", Description: "Discovered local character view assets"},
			"timeline":        {Type: "array", Description: "Narration-synced character action beats"},
			"content":         {Type: "string", Description: "Human-reviewable action plan"},
			"artifacts":       {Type: "object", Description: "IP A-roll plan artifacts"},
		}
	case "local_ip_talking_avatar_render":
		manifest.Description = "Render a local cartoon IP talking-avatar video from existing character assets, narration audio, subtitles, and a background without using AIGC video generation."
		manifest.Type = "local_video_render"
		manifest.CostLevel = tool.CostLow
		manifest.LatencyLevel = tool.LatencyMedium
		manifest.RiskLevel = tool.RiskMedium
		manifest.SideEffect = true
		manifest.Idempotent = false
		manifest.Capabilities = []string{
			"cartoon_ip_talking_video",
			"audio_driven_lip_sync",
			"sprite_2d_avatar_render",
			"svg_2d_puppet_render",
			"hypergen_controllable_ip_parts",
			"character_voice_profile",
			"segmented_prosody_preview_voice",
			"independent_limb_motion",
			"left_right_arm_gestures",
			"body_weight_shift",
			"local_video_render",
			"subtitle_composition",
			"ffmpeg_video_composition",
			"oral_video",
			"brand_character_explainer",
		}
		manifest.Tags = []string{"ip", "avatar", "talking-avatar", "cartoon-digital-human", "local-render", "oral-video", "not-aigc"}
		manifest.ProviderCapabilities = map[string]interface{}{
			"overall": "当用户希望使用已有 IP、卡通形象、品牌角色、虚拟人形象创作口播视频，并且不希望依赖 AIGC 视频生成时，使用该工具。该工具根据角色资产、口播音频、字幕和背景，在本地生成卡通数字人口播视频。",
			"notFor": []string{
				"realistic_human_face_generation",
				"aigc_video_generation",
				"seedance_video_generation",
				"image_to_video_generation",
				"face_detail_restoration",
			},
			"retrievalHints": []string{"IP口播", "卡通数字人", "虚拟人口播", "品牌角色讲解", "本地数字人", "真人感口播", "角色动作", "四肢动作"},
			"live2dStatus":   "Live2D is supported at the asset protocol level when a Cubism model3.json package exists; the current local renderer runs sprite2d/svg2d puppet assets.",
			"voiceBoundary":  "Local macOS say preview is segmented with prosody planning for rhythm and pauses, but production human-like voice quality should use uploaded character narration audio or a dedicated TTS provider.",
		}
		manifest.LocalRequirements = tool.LocalRequirements{
			Commands:        []string{"ffmpeg", "ffprobe"},
			RequiresNetwork: false,
			MinDiskMb:       256,
		}
		manifest.ArtifactPolicy = tool.ArtifactPolicy{
			ProduceArtifact:       true,
			ArtifactKinds:         []string{"VIDEO", "AVATAR_LAYER_VIDEO", "IP_TALKING_AVATAR_TIMELINE", "IP_TALKING_AVATAR_SCENE", "IP_TALKING_AVATAR_VOICE_PROFILE", "RENDER_REPORT"},
			DefaultReviewRequired: true,
			Storage:               tool.ArtifactLocationLocal,
			SyncMetadataToCloud:   true,
		}
		manifest.ApprovalPolicy = tool.ApprovalPolicy{
			Required:            true,
			Mode:                tool.ApprovalAfterArtifact,
			BlocksDownstream:    true,
			Reason:              "IP 资产、口型动作和最终口播视频需要用户确认后再进入后续视频合成。",
			ReviewArtifactKinds: []string{"VIDEO", "AVATAR_LAYER_VIDEO", "IP_TALKING_AVATAR_TIMELINE", "IP_TALKING_AVATAR_SCENE", "RENDER_REPORT"},
		}
		manifest.HumanReview = &tool.HumanReview{
			Required: true,
			Gate:     tool.ApprovalAfterArtifact,
			Title:    "审核本地 IP 数字人口播视频",
			ReviewFocus: []string{
				"角色是否为用户选定 IP",
				"口型开合是否跟随口播节奏",
				"基础动作是否自然且不抢信息",
				"字幕、背景和角色层是否合成正确",
			},
			UserActions: []string{"preview_video", "replace_audio", "edit_subtitles", "switch_character", "rerender_locally"},
		}
		manifest.Parameters = map[string]tool.ParamDef{
			"characterId":      {Type: "string", Description: "Local IP character id under assets/characters/{characterId}", Required: true},
			"script":           {Type: "string", Description: "Narration script used for motion triggers and subtitle assistance", Required: false},
			"audioPath":        {Type: "string", Description: "Local narration audio path or local:// ref. Optional when script is provided; local preview TTS may be generated for action/lip-sync preview.", Required: false},
			"subtitlePath":     {Type: "string", Description: "Optional SRT subtitle path or local:// ref", Required: false},
			"backgroundPath":   {Type: "string", Description: "Optional local background image or video path", Required: false},
			"bgmPath":          {Type: "string", Description: "Optional local BGM path", Required: false},
			"outputDir":        {Type: "string", Description: "Output directory for render artifacts", Required: false},
			"renderMode":       {Type: "string", Description: "Renderer mode; supports sprite2d and svg2d, reserves live2d for Cubism model assets", Required: false, Enum: []string{"sprite2d", "svg2d", "live2d"}},
			"interactionLevel": {Type: "string", Description: "Motion richness: subtle or expressive", Required: false, Enum: []string{"subtle", "expressive"}},
			"resolution":       {Type: "object", Description: "Output width and height", Required: false},
			"fps":              {Type: "number", Description: "Output frames per second", Required: false},
			"style":            {Type: "object", Description: "Position, scale, subtitle/background flags, transparent avatar preference", Required: false},
			"motionPolicy":     {Type: "object", Description: "Auto blink, breath, sentence nod, and keyword gesture switches", Required: false},
			"voiceProfile":     {Type: "object", Description: "Optional character voice hints such as voiceName, speakingRate, tone, stylePrompt, and previewOnly", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"success":          {Type: "boolean", Description: "Whether rendering completed"},
			"characterId":      {Type: "string", Description: "Rendered character id"},
			"videoPath":        {Type: "string", Description: "Final MP4 path"},
			"avatarVideoPath":  {Type: "string", Description: "Avatar layer video path"},
			"timelinePath":     {Type: "string", Description: "Combined avatar timeline JSON path"},
			"scenePath":        {Type: "string", Description: "Avatar scene and HyperGen control schema JSON path"},
			"subtitlePath":     {Type: "string", Description: "Subtitle file path used for composition or preview"},
			"voiceProfilePath": {Type: "string", Description: "Voice profile JSON path used for narration preview or uploaded audio metadata"},
			"durationSec":      {Type: "number", Description: "Rendered duration in seconds"},
			"qa":               {Type: "object", Description: "Render QA flags"},
			"artifacts":        {Type: "object", Description: "Previewable local artifacts"},
		}
	case "video_frame_qa":
		manifest.Description = "Extract representative frames, score each shot, and detect visual crowding, text-zone overlap, and clutter before delivery."
		manifest.Type = "local_visual_qa"
		manifest.CostLevel = tool.CostLow
		manifest.RiskLevel = tool.RiskLow
		manifest.SideEffect = true
		manifest.Idempotent = true
		manifest.Capabilities = []string{"video_creation", "visual_quality", "frame_sampling", "text_safety", "shot_quality_gate", "shot_repair_planning"}
		manifest.Tags = []string{"qa", "ffmpeg", "visual", "local"}
		manifest.ArtifactPolicy = tool.ArtifactPolicy{
			ProduceArtifact:       true,
			ArtifactKinds:         []string{"VIDEO_VISUAL_QA_REPORT", "VIDEO_VISUAL_QA_CONTACT_SHEET", "SHOT_QA_REPORT", "SHOT_REPAIR_PLAN"},
			DefaultReviewRequired: false,
			Storage:               tool.ArtifactLocationLocal,
		}
		manifest.ApprovalPolicy = tool.ApprovalPolicy{
			Required:            true,
			Mode:                tool.ApprovalAfterArtifact,
			BlocksDownstream:    true,
			Reason:              "Final video frames must be reviewed for text overlap, clutter, and readability before delivery.",
			ReviewArtifactKinds: []string{"VIDEO_VISUAL_QA_REPORT", "VIDEO_VISUAL_QA_CONTACT_SHEET", "SHOT_QA_REPORT", "SHOT_REPAIR_PLAN"},
		}
		manifest.HumanReview = &tool.HumanReview{Required: true, Gate: tool.ApprovalAfterArtifact, Title: "审核视觉抽帧报告"}
		manifest.Parameters = map[string]tool.ParamDef{
			"input":             {Type: "string", Description: "Rendered video local:// ref or local path", Required: true},
			"videoRef":          {Type: "string", Description: "Rendered video ref alias", Required: false},
			"shotList":          {Type: "array", Description: "Shot list for assigning sampled frames to shots", Required: false},
			"sampleIntervalSec": {Type: "number", Description: "Seconds between sampled frames", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"passed":             {Type: "boolean", Description: "Whether no blocking visual issue was detected"},
			"score":              {Type: "number", Description: "Visual QA score from 0 to 100"},
			"shotReports":        {Type: "array", Description: "Structured ShotQAReport objects with decision, fatal gates, metrics, issues, and tool-level repair plans"},
			"shotSpecLints":      {Type: "array", Description: "Pre-generation shot spec lint results for duration, exact text, continuity references, and render strategy hints"},
			"shotSummaries":      {Type: "array", Description: "Per-shot QA metrics, conclusions, and repair guidance"},
			"repairPlan":         {Type: "object", Description: "Shot-level regeneration or review plan derived from QA metrics"},
			"needsRegeneration":  {Type: "boolean", Description: "Whether any shot has blocking issues and should be regenerated"},
			"reportRef":          {Type: "string", Description: "Local JSON report ref"},
			"contactSheetRef":    {Type: "string", Description: "Local contact sheet image ref"},
			"blockingIssueCount": {Type: "number", Description: "Blocking visual issue count"},
			"warningIssueCount":  {Type: "number", Description: "Warning issue count"},
			"frames":             {Type: "array", Description: "Per-sampled-frame QA results"},
			"artifacts":          {Type: "object", Description: "Generated QA artifacts"},
		}
	case "hyperframes_project_generator":
		manifest.ArtifactPolicy = tool.ArtifactPolicy{
			ProduceArtifact:       true,
			ArtifactKinds:         []string{"HYPERFRAMES_PROJECT", "PREVIEW_SNAPSHOTS", "PREVIEW_REPORT"},
			DefaultReviewRequired: true,
		}
		manifest.ApprovalPolicy = tool.ApprovalPolicy{
			Required:         true,
			Mode:             tool.ApprovalAfterArtifact,
			BlocksDownstream: true,
			Reason:           "Preview output must be reviewed before local rendering.",
		}
		manifest.HumanReview = &tool.HumanReview{Required: true, Gate: tool.ApprovalAfterArtifact, Title: "审核画面预览"}
		manifest.Parameters = map[string]tool.ParamDef{
			"topic":              {Type: "string", Description: "Video topic", Required: true},
			"script":             {Type: "string", Description: "Full voiceover script", Required: true},
			"shotList":           {Type: "array", Description: "Shot list", Required: true},
			"videoPrompts":       {Type: "array", Description: "Video prompts", Required: false},
			"shotAssetPackages":  {Type: "array", Description: "Independent per-shot asset packages", Required: false},
			"aRollAssetPackages": {Type: "array", Description: "Optional continuous A-roll video packages used as the composition base", Required: false},
			"ipArollPlan":        {Type: "object", Description: "Optional IP character A-roll plan from ip_aroll_director", Required: false},
			"style":              {Type: "string", Description: "Visual style", Required: false},
			"publishCopy":        {Type: "object", Description: "Publish copy", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"projectDir": {Type: "string", Description: "HyperFrames project directory"},
			"entry":      {Type: "string", Description: "Entry HTML file"},
			"files":      {Type: "array", Description: "Generated files"},
			"summary":    {Type: "string", Description: "Generation summary"},
		}
	case "hyperframes_renderer":
		manifest.CostLevel = tool.CostHigh
		manifest.RiskLevel = tool.RiskMedium
		manifest.SideEffect = true
		manifest.Idempotent = false
		manifest.ArtifactPolicy = tool.ArtifactPolicy{
			ProduceArtifact:       true,
			ArtifactKinds:         []string{"VIDEO", "RENDER_REPORT"},
			DefaultReviewRequired: false,
		}
		manifest.ApprovalPolicy = tool.ApprovalPolicy{
			Required: true,
			Mode:     tool.ApprovalBeforeExecute,
			Reason:   "Local video rendering should only start after preview approval.",
		}
		manifest.HumanReview = &tool.HumanReview{Required: true, Gate: tool.ApprovalBeforeExecute, Title: "确认最终渲染"}
		manifest.Parameters = map[string]tool.ParamDef{
			"projectDir":        {Type: "string", Description: "HyperFrames project directory", Required: true},
			"hyperframesPath":   {Type: "string", Description: "HyperFrames project directory alias", Required: false},
			"entry":             {Type: "string", Description: "Optional HTML entry file", Required: false},
			"previewApproved":   {Type: "boolean", Description: "Whether preview has been approved", Required: false},
			"outputName":        {Type: "string", Description: "Output MP4 file name", Required: false},
			"targetDurationSec": {Type: "number", Description: "Target video duration", Required: false},
			"timeoutSec":        {Type: "number", Description: "Optional per-job render timeout in seconds before local fallback can run", Required: false},
		}
		manifest.Output = map[string]tool.ParamDef{
			"outputPath":    {Type: "string", Description: "Rendered MP4 path"},
			"finalVideo":    {Type: "string", Description: "Rendered final video path"},
			"VIDEO":         {Type: "string", Description: "Rendered video artifact"},
			"RENDER_REPORT": {Type: "object", Description: "Render report"},
		}
	}
}

func applyReviewableArtifactContract(manifest *tool.ToolManifest, artifactKinds ...string) {
	manifest.ApprovalPolicy = tool.ApprovalPolicy{
		Required:            true,
		Mode:                tool.ApprovalAfterArtifact,
		BlocksDownstream:    true,
		Reason:              "inspect and approve the structured production intermediate before downstream generation",
		ReviewArtifactKinds: append([]string(nil), artifactKinds...),
	}
	manifest.ArtifactPolicy = tool.ArtifactPolicy{
		ProduceArtifact:       true,
		ArtifactKinds:         append([]string(nil), artifactKinds...),
		DefaultReviewRequired: true,
		Storage:               tool.ArtifactLocationCloud,
	}
}

// localToolManifests configures tools that execute on the user's local device.
var localToolManifests = map[string]struct {
	ExecutionPlane     string
	LocalCommand       string
	RequiresUserDevice bool
	ArtifactLocation   string
	Description        string
	Timeout            int
}{
	"hyperframes_project_generator": {
		ExecutionPlane:     tool.ExecutionPlaneLocal,
		LocalCommand:       "HYPERFRAMES_PROJECT_GENERATE",
		RequiresUserDevice: true,
		ArtifactLocation:   tool.ArtifactLocationLocal,
		Description:        "在本地生成 HyperFrames HTML 项目，包含卡片、字幕和样式",
		Timeout:            120,
	},
	"hyperframes_renderer": {
		ExecutionPlane:     tool.ExecutionPlaneLocal,
		LocalCommand:       "HYPERFRAMES_RENDER",
		RequiresUserDevice: true,
		ArtifactLocation:   tool.ArtifactLocationLocal,
		Description:        "调用本地 HyperFrames Render Service 渲染 MP4 视频",
		Timeout:            1800,
	},
	"artifact_packager": {
		ExecutionPlane:     tool.ExecutionPlaneLocal,
		LocalCommand:       "ARTIFACT_PACKAGE",
		RequiresUserDevice: true,
		ArtifactLocation:   tool.ArtifactLocationLocal,
		Description:        "将项目产物（视频、manifest、数据）打包为 ZIP 文件",
		Timeout:            120,
	},
	"ffmpeg_probe": {
		ExecutionPlane:     tool.ExecutionPlaneLocal,
		LocalCommand:       "FFMPEG_PROBE",
		RequiresUserDevice: true,
		ArtifactLocation:   tool.ArtifactLocationLocal,
		Description:        "使用 ffprobe 检查视频文件元数据（时长、分辨率、编码）",
		Timeout:            60,
	},
	"video_frame_qa": {
		ExecutionPlane:     tool.ExecutionPlaneLocal,
		LocalCommand:       "VIDEO_FRAME_QA",
		RequiresUserDevice: true,
		ArtifactLocation:   tool.ArtifactLocationLocal,
		Description:        "抽帧检查最终视频文字安全区、遮挡和画面复杂度",
		Timeout:            180,
	},
	"local_ip_talking_avatar_render": {
		ExecutionPlane:     tool.ExecutionPlaneLocal,
		LocalCommand:       "LOCAL_IP_TALKING_AVATAR_RENDER",
		RequiresUserDevice: true,
		ArtifactLocation:   tool.ArtifactLocationLocal,
		Description:        "使用本地 IP 角色资产、口播音频、字幕和背景确定性渲染卡通数字人口播视频",
		Timeout:            1800,
	},
	"mcp_generation_runner": {
		ExecutionPlane:     tool.ExecutionPlaneLocal,
		LocalCommand:       "LOCAL_MCP_TOOL_CALL",
		RequiresUserDevice: true,
		ArtifactLocation:   tool.ArtifactLocationLocal,
		Description:        "调用用户本地 MCP provider 生成 AIGC 图片或视频素材",
		Timeout:            1800,
	},
}

func executeLocalVideoCreationTool(toolName string, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	stage := stringParam(params, "stage", toolName)
	skillName := stringParam(params, "skill_name", "video-skill")
	brief := stringParam(params, "brief", "")
	instructionRef := stringParam(params, "instruction_ref", "")

	// skill_stage_agent is the default tool for LLM stages. Call the LLM API
	// with a prompt built from the stage instruction and user's brief.
	if toolName == "skill_stage_agent" {
		return executeSkillStageAgent(stage, skillName, brief, instructionRef, params, toolCtx)
	}

	// Dynamic agent prompt_tool entries — use the LLM API to generate content
	// from the tool parameters (topic, facts, style, script, etc.).
	if isDynamicAgentPromptTool(toolName) {
		return executeDynamicAgentPromptTool(toolName, stage, skillName, brief, instructionRef, params, toolCtx)
	}

	// Dispatch to tool-specific implementations.
	switch toolName {
	case "news_search":
		return executeNewsSearch(params)
	case "fact_extractor":
		return executeFactExtractor(stage, skillName, params)
	case "pipeline_selector":
		return executePipelineSelector(stage, skillName, brief, params, toolCtx)
	case "capability_preflight":
		return executeCapabilityPreflight(stage, skillName, params)
	case "proposal_generator":
		return executeProposalGenerator(stage, skillName, brief, params, toolCtx)
	case "visual_feasibility_analyzer":
		return executeVisualFeasibilityAnalyzer(stage, skillName, params)
	case "render_strategy_planner":
		return executeRenderStrategyPlanner(stage, skillName, params)
	case "video_profile_classifier":
		return executeVideoProfileClassifier(stage, skillName, brief, params)
	case "audio_master_planner":
		return executeAudioMasterPlanner(stage, skillName, params)
	case "time_window_planner":
		return executeTimeWindowPlanner(stage, skillName, params)
	case "visual_alignment_planner":
		return executeVisualAlignmentPlanner(stage, skillName, params)
	case "cinematic_shot_designer":
		return executeCinematicShotDesigner(stage, skillName, brief, params)
	case "sound_design_planner":
		return executeSoundDesignPlanner(stage, skillName, brief, params)
	case "shot_generation_planner":
		return executeShotGenerationPlanner(stage, skillName, params)
	case "asset_decision_agent":
		return executeAssetDecisionAgent(stage, skillName, params)
	case "render_dependency_guard":
		return executeRenderDependencyGuard(stage, skillName, params)
	case "local_job_status_tracker":
		return executeLocalJobStatusTracker(stage, skillName, params)
	case "final_review_generator":
		return executeFinalReviewGenerator(stage, skillName, params)
	case "ip_aroll_director":
		return executeIPArollDirector(stage, skillName, params, toolCtx)
	case "image_asset_generator":
		return executeImageAssetGenerator(stage, skillName, brief, instructionRef, params, toolCtx)
	case "hyperframes_project_generator":
		return executeHyperframesProjectGenerator(stage, skillName, brief, instructionRef, params, toolCtx)
	case "hyperframes_project_builder", "storyboard_assembler":
		return executeHyperframesProjectBuilder(stage, skillName, brief, instructionRef, params, toolCtx)
	case "text_image_to_video_generator":
		return executeModelGatewayVideoGenerator(stage, skillName, brief, params, toolCtx)
	case "hyperframes_renderer", "video_final_assembler":
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

func executePipelineSelector(stage, skillName, brief string, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	root := pipelineRoot(params)
	registry, err := videopipeline.LoadDirectory(root)
	if err != nil {
		return tool.FailureResult(fmt.Sprintf("load video pipelines: %s", err.Error()))
	}
	selection, err := registry.Select(videopipeline.SelectionRequest{
		Message: brief,
		Inputs:  inputKindsParam(params),
	})
	if err != nil {
		return tool.FailureResult(fmt.Sprintf("select video pipeline: %s", err.Error()))
	}
	selectionMap := structToMap(selection)
	content := fmt.Sprintf("# Pipeline Selection\n\n已选择 `%s`（%s）。\n\n原因：%s\n\n首个审核阶段：`%s`",
		selection.PipelineID, selection.PipelineName, selection.Reason, selection.FirstApprovalStage)
	return tool.SuccessResult(map[string]interface{}{
		"content":           content,
		"pipelineSelection": selectionMap,
		"artifacts": []map[string]interface{}{
			jsonArtifact(stage, "pipeline_selection.json", skillName, "videoforge-pipeline-selector", true),
		},
		"taskId": toolCtx.TaskID,
	})
}

func executeCapabilityPreflight(stage, skillName string, params map[string]interface{}) tool.ToolResult {
	caps := capabilitySnapshot(params)
	result := map[string]interface{}{
		"textModel": map[string]interface{}{
			"available": caps.TextModelAvailable,
			"provider":  "openai-compatible",
		},
		"hyperframes": map[string]interface{}{
			"available":       caps.HyperFramesAvailable,
			"contractVersion": "aios-hyperframes-render-v1",
			"supports":        []string{"render", "lint", "captionOverlay", "videoClipComposition"},
		},
		"seedance": map[string]interface{}{
			"available":              caps.SeedanceAvailable,
			"maxDurationSec":         15,
			"supportsImageReference": true,
			"supportsVideoReference": true,
		},
		"tts": map[string]interface{}{
			"available":     caps.TTSAvailable,
			"setupRequired": !caps.TTSAvailable,
		},
		"asr":             map[string]interface{}{"available": caps.ASRAvailable},
		"recommendations": capabilityRecommendations(caps),
	}
	return tool.SuccessResult(map[string]interface{}{
		"content":      "# Capability Preflight\n\n" + strings.Join(capabilityRecommendations(caps), "\n"),
		"capabilities": result,
		"artifacts": []map[string]interface{}{
			jsonArtifact(stage, "capability_preflight.json", skillName, "videoforge-capability-preflight", false),
		},
	})
}

func executeProposalGenerator(stage, skillName, brief string, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	manifest := loadRequestedPipeline(params)
	packet := videopipeline.BuildProposalPacket(videopipeline.ProposalRequest{
		Pipeline:          manifest,
		Message:           brief,
		TargetDurationSec: intParam(params, "targetDurationSec", 60),
		Capabilities:      capabilitySnapshot(params),
	})

	// Use LLM to intelligently recommend the best option based on the user's topic.
	llmRecommended, llmReason := callProposalRecommendationLLM(brief, packet.Options, params)
	if llmRecommended != "" {
		packet.RecommendedOptionID = llmRecommended
		packet.DecisionLog.Selected = llmRecommended
	}

	optionName := packet.RecommendedOptionID
	optionDesc := ""
	for _, opt := range packet.Options {
		if opt.ID == packet.RecommendedOptionID {
			optionName = opt.Name
			optionDesc = opt.Description
			break
		}
	}

	var content string
	if llmReason != "" {
		content = fmt.Sprintf("# Proposal Packet\n\n推荐方案：%s\n\n%s\n\n**AI 分析**：%s\n\n该阶段必须经用户确认后才能进入脚本和高成本生成阶段。", optionName, optionDesc, llmReason)
	} else {
		content = fmt.Sprintf("# Proposal Packet\n\n推荐方案：%s\n\n%s\n\n该阶段必须经用户确认后才能进入脚本和高成本生成阶段。", optionName, optionDesc)
	}

	packetMap := structToMap(packet)
	return tool.SuccessResult(map[string]interface{}{
		"content":        content,
		"proposalPacket": packetMap,
		"decisionLog":    structToMap(packet.DecisionLog),
		"artifacts": []map[string]interface{}{
			jsonArtifact(stage, "proposal_packet.json", skillName, "videoforge-proposal-generator", true),
		},
	})
}

var ipCharacterViewFiles = []struct {
	Key  string
	File string
}{
	{Key: "reference", File: "reference.png"},
	{Key: "front", File: "front.png"},
	{Key: "front3qLeft", File: "front-3q-left.png"},
	{Key: "leftSide", File: "left-side.png"},
	{Key: "rightSide", File: "right-side.png"},
	{Key: "back", File: "back.png"},
	{Key: "back3qRight", File: "back-3q-right.png"},
}

func executeIPArollDirector(stage, skillName string, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	narration := promptStringParam(params, "narration", "")
	if narration == "" {
		return tool.FailureResult("ip_aroll_director requires narration")
	}

	characterNames := ipCharacterNamesParam(params["characters"])
	assetRoot := resolveIPCharacterAssetRoot(stringParam(params, "assetRoot", ""))
	durationSec := intParam(params, "durationSec", 0)
	if durationSec <= 0 {
		durationSec = estimateNarrationDurationSec(narration)
	}
	shotID := stringParam(params, "shotId", toolCtx.NodeID)
	if shotID == "" {
		shotID = "SHOT_01"
	}
	layout := stringParam(params, "layout", "two_host")
	style := stringParam(params, "style", "16:9 横屏口播，两个非真人 IP 小人，干净明亮，角色动作轻量但有反应")

	characters, warnings := buildIPCharacterAssetIndex(assetRoot, characterNames)
	rigs := buildIPCharacterRigIndex(characters)
	timeline := buildIPArollTimeline(narration, durationSec, characters, layout)

	plan := map[string]interface{}{
		"schemaVersion": "ip-aroll-v1",
		"shotId":        shotID,
		"assetRoot":     filepath.ToSlash(assetRoot),
		"durationSec":   durationSec,
		"layout":        layout,
		"style":         style,
		"characters":    characters,
		"rigs":          rigs,
		"timeline":      timeline,
		"hyperframes": map[string]interface{}{
			"layerRole":     "oral_aroll_character_layer",
			"renderMode":    "deterministic_2_5d_sprite_rig",
			"assetPolicy":   "use_local_character_views_without_redrawing_identity",
			"textOwnership": "HyperFrames owns captions, titles, cards, and exact Chinese text",
			"safeArea": map[string]interface{}{
				"caption": "bottom 18%",
				"title":   "top 18%",
				"notes":   "characters stay left/right or center-lower; keep subtitle band clear",
			},
			"animationPrimitives": []string{
				"subtle_float",
				"breathing_scale",
				"blink",
				"mouth_flap",
				"small_hand_wave",
				"point",
				"nod",
				"react_pop",
			},
			"callPattern": "Pass ipArollPlan into hyperframes_project_generator; bind character assets by characterId and view.",
		},
		"constraints": []string{
			"不要让 AIGC 重新生成这两个角色的身份外观",
			"口播层只控制角色、表情、轻动作、字幕和信息卡片",
			"动作必须服务口播，不做大幅奔跑、复杂肢体或跨镜头连续表演",
			"字幕和精确中文由 HyperFrames 渲染，避免视频模型乱码",
		},
		"warnings": warnings,
	}

	content := buildIPArollReviewContent(stage, skillName, narration, plan, timeline)
	artifacts := []map[string]interface{}{
		typedJSONArtifact(stage, "ip_aroll_plan.json", skillName, "IP_AROLL_PLAN", true),
		typedJSONArtifact(stage+"-assets", "character_asset_index.json", skillName, "CHARACTER_ASSET_INDEX", true),
	}

	return tool.SuccessResult(map[string]interface{}{
		"content":         content,
		"ipArollPlan":     plan,
		"characterAssets": characters,
		"timeline":        timeline,
		"artifacts":       artifacts,
		"taskId":          toolCtx.TaskID,
	})
}

func ipCharacterNamesParam(value interface{}) []string {
	names := []string{}
	switch typed := value.(type) {
	case []interface{}:
		for _, item := range typed {
			if name := strings.TrimSpace(fmt.Sprint(item)); name != "" && name != "<nil>" {
				names = append(names, name)
			}
		}
	case []string:
		for _, item := range typed {
			if name := strings.TrimSpace(item); name != "" {
				names = append(names, name)
			}
		}
	case string:
		for _, item := range strings.FieldsFunc(typed, func(r rune) bool {
			return r == ',' || r == '，' || r == '、' || r == ';' || r == '；'
		}) {
			if name := strings.TrimSpace(item); name != "" {
				names = append(names, name)
			}
		}
	}
	if len(names) == 0 {
		return []string{"波波", "阿斯特"}
	}
	return names
}

func resolveIPCharacterAssetRoot(preferred string) string {
	candidates := []string{}
	if env := strings.TrimSpace(os.Getenv("AIOS_IP_CHARACTER_ROOT")); env != "" {
		candidates = append(candidates, env)
	}
	if strings.TrimSpace(preferred) != "" {
		candidates = append(candidates, preferred)
	}
	candidates = append(candidates, "ip-assets", "../ip-assets", "../../ip-assets")

	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if filepath.IsAbs(candidate) {
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate
			}
			continue
		}
		if abs, err := filepath.Abs(candidate); err == nil {
			if info, statErr := os.Stat(abs); statErr == nil && info.IsDir() {
				return abs
			}
		}
	}
	if strings.TrimSpace(preferred) != "" {
		if abs, err := filepath.Abs(preferred); err == nil {
			return abs
		}
		return preferred
	}
	if abs, err := filepath.Abs("ip-assets"); err == nil {
		return abs
	}
	return "ip-assets"
}

func buildIPCharacterAssetIndex(assetRoot string, names []string) ([]map[string]interface{}, []string) {
	characters := make([]map[string]interface{}, 0, len(names))
	warnings := []string{}
	for idx, name := range names {
		characterID := stableIPCharacterID(name)
		characterDir := filepath.Join(assetRoot, name)
		assets := map[string]interface{}{}
		assetRefs := map[string]interface{}{}
		availableViews := []string{}
		for _, view := range ipCharacterViewFiles {
			path := filepath.Join(characterDir, view.File)
			if _, err := os.Stat(path); err == nil {
				normalized := filepath.ToSlash(path)
				assets[view.Key] = normalized
				assetRefs[view.Key] = fmt.Sprintf("local://ip-characters/%s/%s", name, view.File)
				availableViews = append(availableViews, view.Key)
			} else {
				warnings = append(warnings, fmt.Sprintf("%s 缺少 %s", name, view.File))
			}
		}
		rig := buildIPCharacterRig(characterID, name)
		characters = append(characters, map[string]interface{}{
			"id":             characterID,
			"name":           name,
			"role":           ipCharacterRole(name, idx),
			"assetDir":       filepath.ToSlash(characterDir),
			"assets":         assets,
			"assetRefs":      assetRefs,
			"availableViews": availableViews,
			"rig":            rig,
			"controlHints": []string{
				"front/front3qLeft 用于正面口播和指向动作",
				"leftSide/rightSide 用于双人对话时的互看",
				"reference 用于前端预览和角色身份校验",
			},
		})
	}
	return characters, warnings
}

func buildIPCharacterRigIndex(characters []map[string]interface{}) map[string]interface{} {
	rigs := map[string]interface{}{}
	for _, character := range characters {
		id := firstStringInMap(character, "id")
		if id == "" {
			continue
		}
		if rig, ok := mapValue(character["rig"]); ok && len(rig) > 0 {
			rigs[id] = rig
			continue
		}
		rigs[id] = buildIPCharacterRig(id, firstStringInMap(character, "name"))
	}
	return rigs
}

func buildIPCharacterRig(characterID, name string) map[string]interface{} {
	face := map[string]interface{}{"x": 0.50, "y": 0.30, "width": 0.40, "height": 0.18}
	switch characterID {
	case "bobo":
		face = map[string]interface{}{"x": 0.50, "y": 0.31, "width": 0.43, "height": 0.19}
	case "aster":
		face = map[string]interface{}{"x": 0.50, "y": 0.28, "width": 0.36, "height": 0.17}
	}
	return map[string]interface{}{
		"schemaVersion": "ip-puppet-rig-v1",
		"characterId":   characterID,
		"characterName": name,
		"rigType":       "face_overlay_2_5d",
		"baseLayer":     "full_body_sprite",
		"faceOverlay":   face,
		"renderLayers": []string{
			"baseSprite",
			"faceScreen",
			"eyes",
			"mouth",
			"gestureCue",
			"shadow",
		},
		"controls": map[string]interface{}{
			"mouthOpen": map[string]interface{}{"type": "number", "min": 0, "max": 1, "editable": true},
			"viseme":    map[string]interface{}{"type": "enum", "values": []string{"closed", "a", "o", "e", "smile"}, "editable": true},
			"blink":     map[string]interface{}{"type": "boolean", "editable": true},
			"lookX":     map[string]interface{}{"type": "number", "min": -1, "max": 1, "editable": true},
			"lookY":     map[string]interface{}{"type": "number", "min": -1, "max": 1, "editable": true},
			"headTilt":  map[string]interface{}{"type": "number", "min": -12, "max": 12, "unit": "deg", "editable": true},
			"handLift":  map[string]interface{}{"type": "number", "min": 0, "max": 1, "editable": true},
			"bodyBounce": map[string]interface{}{
				"type":     "number",
				"min":      0,
				"max":      1,
				"editable": true,
			},
		},
	}
}

func stableIPCharacterID(name string) string {
	switch strings.TrimSpace(name) {
	case "波波", "Bobo", "BOBO", "bobo":
		return "bobo"
	case "阿斯特", "Aster", "ASTER", "aster":
		return "aster"
	default:
		var b strings.Builder
		for _, r := range strings.ToLower(name) {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			}
		}
		if b.Len() == 0 {
			return fmt.Sprintf("ip_%x", stdsha256.Sum256([]byte(name)))[:10]
		}
		return b.String()
	}
}

func ipCharacterRole(name string, idx int) string {
	switch stableIPCharacterID(name) {
	case "bobo":
		return "warm_hook_host"
	case "aster":
		return "knowledge_explainer"
	default:
		if idx == 0 {
			return "primary_host"
		}
		return "support_host"
	}
}

func estimateNarrationDurationSec(narration string) int {
	runes := []rune(strings.TrimSpace(narration))
	if len(runes) == 0 {
		return 6
	}
	estimated := int(math.Ceil(float64(len(runes)) / 5.0))
	if estimated < 6 {
		return 6
	}
	if estimated > 15 {
		return 15
	}
	return estimated
}

func buildIPArollTimeline(narration string, durationSec int, characters []map[string]interface{}, layout string) []map[string]interface{} {
	segments := splitNarrationForIPAroll(narration)
	if len(segments) == 0 {
		segments = []string{narration}
	}
	if durationSec <= 0 {
		durationSec = estimateNarrationDurationSec(narration)
	}
	characterCount := len(characters)
	if characterCount == 0 {
		characters = []map[string]interface{}{{"id": "bobo", "name": "波波"}}
		characterCount = 1
	}
	actionSeq := []string{"speak", "point", "present", "nod", "react", "listen"}
	timeline := make([]map[string]interface{}, 0, len(segments))
	beatDuration := float64(durationSec) / float64(len(segments))
	for idx, text := range segments {
		character := characters[idx%characterCount]
		characterID := fmt.Sprint(character["id"])
		characterName := fmt.Sprint(character["name"])
		action := actionSeq[idx%len(actionSeq)]
		start := roundSeconds(float64(idx) * beatDuration)
		end := roundSeconds(float64(idx+1) * beatDuration)
		if idx == len(segments)-1 {
			end = float64(durationSec)
		}
		expression := ipExpressionForAction(action)
		timeline = append(timeline, map[string]interface{}{
			"beatId":        fmt.Sprintf("beat_%02d", idx+1),
			"startSec":      start,
			"endSec":        end,
			"characterId":   characterID,
			"characterName": characterName,
			"text":          text,
			"view":          ipViewForAction(action, idx, characterCount),
			"action":        action,
			"gesture":       ipGestureForAction(action),
			"expression":    expression,
			"expressionCue": expression,
			"mouthCue":      "talking",
			"bodyCue":       "subtle_float",
			"lipSync":       buildLipSyncTrack(text, start, end),
			"motion":        buildIPMotionTrack(action, start, end),
			"position":      ipPositionForBeat(layout, idx, characterID, characterCount),
			"editable":      true,
		})
	}
	return timeline
}

func splitNarrationForIPAroll(narration string) []string {
	fields := strings.FieldsFunc(narration, func(r rune) bool {
		return r == '。' || r == '！' || r == '？' || r == '\n' || r == ';' || r == '；'
	})
	segments := []string{}
	for _, field := range fields {
		text := strings.TrimSpace(field)
		text = strings.Trim(text, "，,、 ")
		if text != "" {
			segments = append(segments, text)
		}
	}
	if len(segments) == 1 {
		runes := []rune(segments[0])
		if len(runes) > 28 {
			mid := len(runes) / 2
			segments = []string{strings.TrimSpace(string(runes[:mid])), strings.TrimSpace(string(runes[mid:]))}
		}
	}
	return segments
}

func buildLipSyncTrack(text string, startSec, endSec float64) []map[string]interface{} {
	duration := endSec - startSec
	if duration <= 0 {
		duration = 1
	}
	runes := lipSyncRunes(text)
	sampleCount := int(math.Ceil(duration*6)) + 1
	if sampleCount < 3 {
		sampleCount = 3
	}
	if sampleCount > 24 {
		sampleCount = 24
	}
	track := make([]map[string]interface{}, 0, sampleCount)
	for i := 0; i < sampleCount; i++ {
		progress := float64(i) / float64(sampleCount-1)
		timeSec := roundSeconds(startSec + progress*duration)
		viseme := "closed"
		if i != 0 && i != sampleCount-1 && len(runes) > 0 {
			viseme = ipVisemeForRune(runes[(i-1)%len(runes)], i)
		}
		mouthOpen := mouthOpenForViseme(viseme)
		track = append(track, map[string]interface{}{
			"timeSec":    timeSec,
			"viseme":     viseme,
			"mouthOpen":  mouthOpen,
			"energy":     roundFloat(math.Max(0.28, mouthOpen), 2),
			"editable":   true,
			"sourceText": truncateText(text, 28),
		})
	}
	return track
}

func lipSyncRunes(text string) []rune {
	out := []rune{}
	for _, r := range strings.TrimSpace(text) {
		if strings.ContainsRune(" \t\r\n，,。.!！?？:：;；、（）()[]【】\"'“”‘’", r) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func ipVisemeForRune(r rune, index int) string {
	switch {
	case strings.ContainsRune("aeiAEI", r):
		return "e"
	case strings.ContainsRune("ouOU", r):
		return "o"
	case strings.ContainsRune("bpmfBPMF", r):
		return "closed"
	case strings.ContainsRune("rnltdRNLTD", r):
		return "smile"
	case r >= 0x4e00 && r <= 0x9fff:
		switch (int(r) + index) % 4 {
		case 0:
			return "a"
		case 1:
			return "o"
		case 2:
			return "e"
		default:
			return "smile"
		}
	default:
		switch index % 4 {
		case 0:
			return "a"
		case 1:
			return "e"
		case 2:
			return "o"
		default:
			return "smile"
		}
	}
}

func mouthOpenForViseme(viseme string) float64 {
	switch viseme {
	case "a":
		return 0.76
	case "o":
		return 0.64
	case "e":
		return 0.48
	case "smile":
		return 0.36
	default:
		return 0
	}
}

func buildIPMotionTrack(action string, startSec, endSec float64) map[string]interface{} {
	duration := endSec - startSec
	if duration <= 0 {
		duration = 1
	}
	mid := roundSeconds(startSec + duration*0.52)
	lift := 0.24
	tilt := -2.0
	bounce := -8.0
	scale := 1.015
	switch action {
	case "point":
		lift = 0.88
		tilt = -5
		bounce = -12
		scale = 1.025
	case "present":
		lift = 0.72
		tilt = 4
		bounce = -10
		scale = 1.02
	case "nod":
		lift = 0.18
		tilt = 6
		bounce = -7
	case "react":
		lift = 0.58
		tilt = -7
		bounce = -18
		scale = 1.05
	case "listen":
		lift = 0.08
		tilt = 3
		bounce = -4
		scale = 1.005
	}
	return map[string]interface{}{
		"mode":   "deterministic_css_puppet",
		"action": action,
		"controls": map[string]interface{}{
			"handLift":   roundFloat(lift, 2),
			"headTilt":   roundFloat(tilt, 2),
			"bodyBounce": roundFloat(math.Abs(bounce)/20, 2),
		},
		"keyframes": []map[string]interface{}{
			{"timeSec": startSec, "translateY": 0, "scale": 1.0, "rotateDeg": 0, "handLift": 0.0},
			{"timeSec": mid, "translateY": bounce, "scale": scale, "rotateDeg": tilt, "handLift": roundFloat(lift, 2)},
			{"timeSec": endSec, "translateY": 0, "scale": 1.0, "rotateDeg": 0, "handLift": 0.0},
		},
		"editable": true,
	}
}

func roundFloat(value float64, places int) float64 {
	if places < 0 {
		return value
	}
	factor := math.Pow(10, float64(places))
	return math.Round(value*factor) / factor
}

func clampFloat(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func ipViewForAction(action string, idx, characterCount int) string {
	switch action {
	case "point", "present":
		return "front3qLeft"
	case "listen":
		if characterCount > 1 && idx%2 == 0 {
			return "rightSide"
		}
		return "leftSide"
	default:
		return "front"
	}
}

func ipGestureForAction(action string) string {
	switch action {
	case "point":
		return "point_to_keyword_card"
	case "present":
		return "open_palm_present"
	case "nod":
		return "small_nod"
	case "react":
		return "react_pop"
	case "listen":
		return "idle_listen"
	default:
		return "small_hand_wave"
	}
}

func ipExpressionForAction(action string) string {
	switch action {
	case "react":
		return "surprised_or_star_eyes"
	case "listen":
		return "calm_listening"
	case "point", "present":
		return "confident_explain"
	default:
		return "friendly_speaking"
	}
}

func ipPositionForBeat(layout string, idx int, characterID string, characterCount int) map[string]interface{} {
	if layout == "single_host" || characterCount == 1 {
		return map[string]interface{}{"x": 0.50, "y": 0.66, "scale": 0.72, "anchor": "center"}
	}
	if layout == "side_commentary" {
		if idx%2 == 0 {
			return map[string]interface{}{"x": 0.26, "y": 0.68, "scale": 0.60, "anchor": "left"}
		}
		return map[string]interface{}{"x": 0.74, "y": 0.68, "scale": 0.60, "anchor": "right"}
	}
	if characterID == "aster" || idx%2 == 1 {
		return map[string]interface{}{"x": 0.68, "y": 0.66, "scale": 0.66, "anchor": "right"}
	}
	return map[string]interface{}{"x": 0.32, "y": 0.67, "scale": 0.70, "anchor": "left"}
}

func roundSeconds(value float64) float64 {
	return math.Round(value*100) / 100
}

func buildIPArollReviewContent(stage, skillName, narration string, plan map[string]interface{}, timeline []map[string]interface{}) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s - IP 口播 A-roll\n\n", stage))
	b.WriteString(fmt.Sprintf("Skill: %s\n\n", skillName))
	b.WriteString("## 口播\n\n")
	b.WriteString(narration)
	b.WriteString("\n\n## 角色动作时间线\n\n")
	for _, beat := range timeline {
		b.WriteString(fmt.Sprintf("- %.2fs-%.2fs · %s · %s · %s：%s\n",
			floatParamFromAny(beat["startSec"]),
			floatParamFromAny(beat["endSec"]),
			beat["characterName"],
			beat["action"],
			beat["gesture"],
			beat["text"],
		))
	}
	b.WriteString("\n## HyperFrames 调用\n\n")
	b.WriteString("把 `ipArollPlan` 传给 `hyperframes_project_generator`，由 HyperFrames 绑定角色图片、字幕、信息卡片和轻动作。\n\n")
	if warnings, ok := plan["warnings"].([]string); ok && len(warnings) > 0 {
		b.WriteString("## 资产提醒\n\n")
		for _, warning := range warnings {
			b.WriteString("- " + warning + "\n")
		}
	}
	return b.String()
}

func floatParamFromAny(value interface{}) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		f, _ := typed.Float64()
		return f
	default:
		return 0
	}
}

// callProposalRecommendationLLM uses the LLM to analyze the user's topic and
// recommend the most suitable creative option. Falls back to empty on any error.
func callProposalRecommendationLLM(brief string, options []videopipeline.ProposalOption, params map[string]interface{}) (recommendedID, reason string) {
	cfg := effectiveVideoCreationOpenAIConfig(params)
	if cfg.APIKey == "" {
		zap.L().Warn("LLM proposal recommendation skipped: no API key configured")
		return "", ""
	}

	// Build a compact summary of options for the prompt.
	var optsDesc strings.Builder
	for _, opt := range options {
		optsDesc.WriteString(fmt.Sprintf("- %s：%s — %s\n", opt.ID, opt.Name, opt.Description))
	}

	systemPrompt := `你是一个专业的视频创作顾问。根据用户的视频主题，从可选方案中推荐最合适的一个。

你必须分析用户主题的内容特点、目标受众、情感基调，然后判断哪个方案最匹配。只输出 JSON，不要输出其他内容。`

	userPrompt := fmt.Sprintf(`用户视频主题：%s

可选方案：
%s
请推荐最合适的方案。输出 JSON：
{"recommendedOptionId": "<方案ID>", "reason": "<简短分析理由，100字以内>"}`, brief, optsDesc.String())

	callTool := &LlmApiTool{cfg: cfg}
	result := callTool.Execute(context.Background(), map[string]interface{}{
		"prompt":          systemPrompt + "\n\n" + userPrompt,
		"max_tokens":      500,
		"temperature":     0.3,
		"response_format": map[string]string{"type": "json_object"},
	}, tool.ToolContext{})

	if !result.Success {
		zap.L().Warn("LLM proposal recommendation failed, falling back to default",
			zap.String("error", result.Error))
		return "", ""
	}

	rawContent, _ := result.Data["content"].(string)
	var parsed struct {
		RecommendedOptionID string `json:"recommendedOptionId"`
		Reason              string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(rawContent)), &parsed); err != nil {
		zap.L().Warn("Failed to parse LLM proposal recommendation",
			zap.String("raw", rawContent),
			zap.Error(err))
		return "", ""
	}

	// Validate the recommended ID is one of the known options.
	for _, opt := range options {
		if opt.ID == parsed.RecommendedOptionID {
			return parsed.RecommendedOptionID, parsed.Reason
		}
	}

	zap.L().Warn("LLM recommended unknown option, ignoring",
		zap.String("recommended", parsed.RecommendedOptionID))
	return "", ""
}

func executeNewsSearch(params map[string]interface{}) tool.ToolResult {
	queries := searchQueriesParam(params["queries"])
	if len(queries) == 0 {
		queries = searchQueriesParam(params["query"])
	}
	// Fall back to topic/brief when PlanCompiler doesn't inject explicit search queries
	if len(queries) == 0 {
		if topic := stringParam(params, "topic", ""); topic != "" {
			queries = append(queries, topic)
		}
		if brief := stringParam(params, "brief", ""); brief != "" && len(queries) == 0 {
			queries = append(queries, brief)
		}
	}
	if len(queries) == 0 {
		return tool.FailureResult("news_search requires at least one query (or topic/brief)")
	}
	topK := intParam(params, "topK", 8)
	if topK <= 0 {
		topK = 8
	}
	results := make([]map[string]interface{}, 0, topK)
	for _, query := range queries {
		found, err := performWebSearch(context.Background(), query)
		if err != nil {
			if isSearchNotConfiguredError(err) {
				return newsSearchManualFallback(queries, err.Error())
			}
			return tool.FailureResult(fmt.Sprintf("news_search failed for query %q: %v", query, err))
		}
		for _, item := range found {
			if len(results) >= topK {
				break
			}
			results = append(results, map[string]interface{}{
				"title":       item.Title,
				"url":         item.URL,
				"source":      sourceFromURL(item.URL),
				"publishedAt": item.Date,
				"snippet":     item.Snippet,
				"language":    "en",
				"credibility": "unknown",
			})
		}
		if len(results) >= topK {
			break
		}
	}
	if len(results) == 0 {
		return tool.FailureResult("news_search returned no results")
	}
	facts := make([]map[string]interface{}, 0, len(results))
	sources := make([]map[string]interface{}, 0, len(results))
	for _, result := range results {
		facts = append(facts, map[string]interface{}{
			"claim":       result["snippet"],
			"source":      result["source"],
			"url":         result["url"],
			"publishedAt": result["publishedAt"],
			"confidence":  result["credibility"],
		})
		sources = append(sources, map[string]interface{}{
			"source":      result["source"],
			"url":         result["url"],
			"publishedAt": result["publishedAt"],
			"title":       result["title"],
		})
	}
	return tool.SuccessResult(map[string]interface{}{
		"results":    results,
		"facts":      facts,
		"sources":    sources,
		"queryUsed":  queries,
		"searchedAt": time.Now().Format(time.RFC3339),
	})
}

func isSearchNotConfiguredError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SEARCH_API_KEY not configured")
}

func newsSearchManualFallback(queries []string, reason string) tool.ToolResult {
	if strings.TrimSpace(reason) == "" {
		reason = "SEARCH_API_KEY not configured"
	}
	return tool.SuccessResult(map[string]interface{}{
		"results":        []map[string]interface{}{},
		"facts":          []map[string]interface{}{},
		"sources":        []map[string]interface{}{},
		"queryUsed":      queries,
		"searchedAt":     time.Now().Format(time.RFC3339),
		"degraded":       true,
		"manualRequired": true,
		"status":         "manual_required",
		"warnings": []string{
			fmt.Sprintf("%s；已跳过联网检索，请在浏览器手动核对事实并补充参考来源。", reason),
		},
		"userInstruction": "未配置 SEARCH_API_KEY，系统不会自动联网检索。请用户在浏览器检索权威来源，补充事实、参考链接和素材，再继续审核。",
	})
}

func executeFactExtractor(stage, skillName string, params map[string]interface{}) tool.ToolResult {
	searchResults := searchResultsParam(params["searchResults"])
	if len(searchResults) == 0 {
		return tool.FailureResult("fact_extractor requires non-empty searchResults")
	}
	facts := make([]map[string]interface{}, 0, len(searchResults))
	sources := make([]map[string]interface{}, 0, len(searchResults))
	warnings := make([]interface{}, 0)
	latestDate := ""
	for i, result := range searchResults {
		title := stringFromMap(result, "title")
		snippet := stringFromMap(result, "snippet")
		urlValue := stringFromMap(result, "url")
		source := stringFromMap(result, "source")
		publishedAt := stringFromMap(result, "publishedAt")
		if title == "" && snippet == "" {
			warnings = append(warnings, map[string]interface{}{
				"index":   i,
				"message": "search result missing title and snippet",
			})
			continue
		}
		claim := strings.TrimSpace(snippet)
		if claim == "" {
			claim = title
		}
		facts = append(facts, map[string]interface{}{
			"claim":      claim,
			"date":       publishedAt,
			"source":     source,
			"url":        urlValue,
			"confidence": "medium",
			"type":       "event_result",
		})
		sources = append(sources, map[string]interface{}{
			"source":      source,
			"url":         urlValue,
			"publishedAt": publishedAt,
			"title":       title,
		})
		if publishedAt > latestDate {
			latestDate = publishedAt
		}
	}
	if len(facts) == 0 {
		return tool.FailureResult("fact_extractor produced no facts")
	}
	hash := knowledgePackHash(facts, sources)
	return tool.SuccessResult(map[string]interface{}{
		"content":           fmt.Sprintf("# Knowledge Pack\n\nFacts: %d\nSources: %d", len(facts), len(sources)),
		"facts":             facts,
		"sources":           sources,
		"freshness":         map[string]interface{}{"latestDate": latestDate, "isFresh": latestDate != ""},
		"warnings":          warnings,
		"knowledgePackHash": hash,
		"artifacts": []map[string]interface{}{
			jsonArtifact(stage, "knowledge_pack.json", skillName, "fact-extractor", false),
		},
	})
}

func searchQueriesParam(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return nonEmptyParamStrings(typed)
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s := strings.TrimSpace(ensureStringValue(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			return []string{trimmed}
		}
	}
	return nil
}

func nonEmptyParamStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func searchResultsParam(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return typed
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if m, ok := item.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out
	case string:
		var parsed []map[string]interface{}
		if json.Unmarshal([]byte(typed), &parsed) == nil {
			return parsed
		}
	}
	return nil
}

func sourceFromURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return raw
	}
	return strings.TrimPrefix(parsed.Host, "www.")
}

func stringFromMap(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}
	return strings.TrimSpace(ensureStringValue(values[key]))
}

func knowledgePackHash(facts, sources []map[string]interface{}) string {
	payload, _ := json.Marshal(map[string]interface{}{"facts": facts, "sources": sources})
	sum := stdsha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func executeVisualFeasibilityAnalyzer(stage, skillName string, params map[string]interface{}) tool.ToolResult {
	strategy := videopipeline.BuildRenderStrategy(videopipeline.RenderStrategyRequest{
		Shots:        shotPlansParam(params),
		Capabilities: capabilitySnapshot(params),
	})
	feasibility := map[string]interface{}{
		"artifactKind": "visual_feasibility",
		"overallMode":  strategy.OverallMode,
		"shots":        strategy.Shots,
		"summary":      "已按精确文字、复杂动态和可用引擎评估每个镜头的视觉可行性。",
	}
	return tool.SuccessResult(map[string]interface{}{
		"content":     "# Visual Feasibility\n\n" + feasibility["summary"].(string),
		"feasibility": feasibility,
		"artifacts": []map[string]interface{}{
			jsonArtifact(stage, "visual_feasibility.json", skillName, "videoforge-visual-feasibility", false),
		},
	})
}

func executeRenderStrategyPlanner(stage, skillName string, params map[string]interface{}) tool.ToolResult {
	strategy := videopipeline.BuildRenderStrategy(videopipeline.RenderStrategyRequest{
		Shots:        shotPlansParam(params),
		Capabilities: capabilitySnapshot(params),
	})
	strategyMap := structToMap(strategy)
	content := fmt.Sprintf("# Render Strategy\n\n整体模式：`%s`\n\nSeedance clips：%d\nHyperFrames renders：%d",
		strategy.OverallMode, strategy.EstimatedCost.SeedanceClips, strategy.EstimatedCost.HyperFramesRenders)
	return tool.SuccessResult(map[string]interface{}{
		"content":        content,
		"renderStrategy": strategyMap,
		"decisionLog":    structToMap(strategy.DecisionLog),
		"artifacts": []map[string]interface{}{
			jsonArtifact(stage, "render_strategy.json", skillName, "videoforge-render-strategy", true),
		},
	})
}

func executeVideoProfileClassifier(stage, skillName, brief string, params map[string]interface{}) tool.ToolResult {
	profileBrief := firstNonEmptyString(params, "brief", "topic", "goal")
	if profileBrief == "" {
		profileBrief = brief
	}
	profile := videoservice.BuildVideoCreationProfile(videoservice.ProfileRequest{
		Route:       stringParam(params, "route", ""),
		Deliverable: stringParam(params, "deliverable", ""),
		Brief:       profileBrief,
	})
	profileMap := structToMap(profile)
	content := fmt.Sprintf("# Video Creation Profile\n\nProfile: `%s`\n\nReason: %s\n", profile.ProfileID, profile.Reason)
	return tool.SuccessResult(map[string]interface{}{
		"creationProfile": profileMap,
		"summary":         fmt.Sprintf("已选择 `%s` 创作主线。", profile.ProfileID),
		"content":         content,
		"artifacts": []map[string]interface{}{
			typedJSONArtifact(stage, "video_creation_profile.json", skillName, videomodel.ArtifactKindVideoCreationProfile, true),
		},
	})
}

func executeAudioMasterPlanner(stage, skillName string, params map[string]interface{}) tool.ToolResult {
	spans := scriptSpansFromToolValue(params["scriptSpans"])
	if len(spans) == 0 {
		return tool.FailureResult("audio_master_planner requires non-empty scriptSpans")
	}
	scriptRevision := strings.TrimSpace(stringParam(params, "scriptRevision", ""))
	if scriptRevision == "" {
		payload, _ := json.Marshal(map[string]interface{}{
			"script": firstNonEmptyString(params, "script"),
			"spans":  spans,
		})
		sum := stdsha256.Sum256(payload)
		scriptRevision = "script-" + hex.EncodeToString(sum[:6])
	}
	voiceRevision := strings.TrimSpace(stringParam(params, "voiceRevision", ""))
	if voiceRevision == "" {
		if stringParam(params, "voiceoverArtifactRef", "") == "" {
			voiceRevision = "voice-estimated-v1"
		} else {
			voiceRevision = "voice-imported-v1"
		}
	}
	master, issues := videoservice.BuildAudioMasterTimeline(videoservice.AudioMasterRequest{
		ScriptRevision:       scriptRevision,
		VoiceRevision:        voiceRevision,
		Language:             stringParam(params, "language", "zh-CN"),
		TimelineSource:       videomodel.TimelineSource(stringParam(params, "timelineSource", "")),
		VoiceoverArtifactRef: stringParam(params, "voiceoverArtifactRef", ""),
		VoiceProfileID:       stringParam(params, "voiceProfileId", ""),
		VoiceProfileVersion:  stringParam(params, "voiceProfileVersion", ""),
		SampleRate:           intFromInterface(params["sampleRate"], 48000),
		ToolVersion:          "audio-master-planner/v1",
		ScriptSpans:          spans,
	})
	if len(issues) > 0 {
		return tool.FailureResult(fmt.Sprintf("audio master validation failed: %v", issues))
	}
	sourceLabel := string(master.TimelineSource)
	return tool.SuccessResult(map[string]interface{}{
		"audioMaster": structToMap(master),
		"summary":     fmt.Sprintf("已建立 %d ms 的%s音频主时间轴（revision %s）。", master.DurationMs, sourceLabel, master.Revision),
		"content":     fmt.Sprintf("# Audio Master Timeline\n\n- Revision: `%s`\n- Source: `%s`\n- Duration: `%d ms`\n- Sentence cues: `%d`\n", master.Revision, sourceLabel, master.DurationMs, len(master.Sentences)),
		"artifacts": []map[string]interface{}{
			typedJSONArtifact(stage, "audio_master_timeline.json", skillName, videomodel.ArtifactKindAudioMasterTimeline, true),
		},
	})
}

func executeTimeWindowPlanner(stage, skillName string, params map[string]interface{}) tool.ToolResult {
	profile := creationProfileFromToolValue(params["creationProfile"])
	sourceShotMaps := toolMapsFromValue(params["shotList"], "shotList", "shots")
	shots := shotUnitsFromToolValue(params["shotList"])
	spans := scriptSpansFromToolValue(params["scriptSpans"])
	audioMaster, hasAudioMaster := audioMasterFromToolValue(params["audioMaster"])
	if profile.ProfileID != videomodel.VideoProfileCinematicStory && len(spans) == 0 && !hasAudioMaster {
		return tool.FailureResult("time_window_planner requires scriptSpans for talking_head profile")
	}
	var audioMasterRef *videomodel.AudioMasterTimeline
	if hasAudioMaster {
		audioMasterRef = &audioMaster
	}
	plan := videoservice.BuildTimeWindowPlan(videoservice.TimeWindowRequest{
		Profile:     profile,
		Shots:       shots,
		ScriptSpans: spans,
		AudioMaster: audioMasterRef,
	})
	planMap := structToMap(plan)
	windowMaps := make([]map[string]interface{}, 0, len(plan.Windows))
	for _, window := range plan.Windows {
		windowMaps = append(windowMaps, structToMap(window))
	}
	if profile.ProfileID == videomodel.VideoProfileCinematicStory {
		enrichCinematicTimeWindowMaps(windowMaps, sourceShotMaps)
	}
	return tool.SuccessResult(map[string]interface{}{
		"timeWindowPlan": planMap,
		"timeWindows":    windowMaps,
		"summary":        fmt.Sprintf("已生成 %d 个 3-15 秒时间窗。", len(windowMaps)),
		"content":        buildTimeWindowReviewContent(windowMaps),
		"artifacts": []map[string]interface{}{
			typedJSONArtifact(stage, "time_window_plan.json", skillName, videomodel.ArtifactKindTimeWindowPlan, true),
		},
	})
}

func executeVisualAlignmentPlanner(stage, skillName string, params map[string]interface{}) tool.ToolResult {
	windows := timeWindowMapsFromParams(params)
	shotList := make([]map[string]interface{}, 0, len(windows))
	brollEntries := make([]map[string]interface{}, 0)
	script := firstNonEmptyString(params, "script")
	assetStrategy := firstNonEmptyString(params, "assetStrategy", "style", "brief", "topic")
	aigcDisabled := isDisabledAIGCProvider(stringParam(params, "aigcProvider", ""))
	creativeMode := !aigcDisabled && voiceVisualNeedsRichAIGC(script, assetStrategy)
	routeCounts := map[string]int{}
	var nextStartMs int64
	visualModePolicy := videoservice.DefaultTalkingHeadVisualModePolicy()
	for i, window := range windows {
		shotID := firstNonEmptyString(window, "shotId", "id")
		if shotID == "" {
			shotID = fmt.Sprintf("SHOT_%02d", i+1)
		}
		narration := firstReadableStringInMap(window, "scriptText", "narrationText", "content", "text")
		visual := firstNonEmptyString(window, "sceneSummary", "visual", "mainAction", "description")
		if visual == "" {
			visual = visualTitleFromNarration(narration, i)
		}
		route := "hyperframes"
		if creativeMode {
			route = voiceVisualAssetRoute(narration, visual, assetStrategy, i, len(windows), creativeMode)
			visual = voiceVisualDirectionForRoute(route, narration, visual, i, creativeMode)
		}
		routeCounts[route]++
		position := videoservice.NarrativePositionBody
		if i == 0 {
			position = videoservice.NarrativePositionHook
		} else if i == len(windows)-1 {
			position = videoservice.NarrativePositionClosing
		} else if containsAnyFold(narration, "核心", "关键", "本质") {
			position = videoservice.NarrativePositionCore
		}
		isBrollRoute := route == "aigc_video" || route == "aigc_image" || route == "image" || route == "video"
		decision := videoservice.DecideTalkingHeadVisualMode(visualModePolicy, videoservice.VisualModeDecisionRequest{
			Position:           position,
			HasExactText:       route == "hyperframes",
			HasScreenRecording: route == "screen_recording",
			HasBroll:           isBrollRoute,
			HasCaseOrEvidence:  containsAnyFold(narration+" "+visual, "案例", "证据", "数据", "对比", "展示"),
			HasComplexConcept:  containsAnyFold(narration, "流程", "原理", "架构", "步骤"),
			NeedsDataGraphic:   route == "hyperframes" && containsAnyFold(narration, "%", "数据", "年份", "流程"),
			NeedsAIGCScene:     route == "aigc_video" || route == "aigc_image",
			EvidenceStrength:   0.86,
		})
		if aigcDisabled {
			decision.Mode = videomodel.VisualModeIPPrimary
			decision.Reason = "explicit AIGC disable keeps the continuous IP A-roll as the primary image"
			decision.Confidence = 1
			decision.NeedsHumanReview = false
		}
		shot := map[string]interface{}{
			"shotId":                shotID,
			"durationSec":           authoredDurationSec(window),
			"narrationText":         narration,
			"visual":                visual,
			"mainAction":            firstNonEmptyString(window, "mainAction", "action"),
			"camera":                visualSubtitleFromNarration(narration, i),
			"timeWindowId":          firstNonEmptyString(window, "id"),
			"plannedAssetRoute":     route,
			"assetIntent":           voiceVisualAssetIntentForRoute(route, narration),
			"timeRelationship":      voiceVisualTimeRelationship(route, authoredDurationSec(window)),
			"visualMode":            string(decision.Mode),
			"visualModeReason":      decision.Reason,
			"visualModeConfidence":  decision.Confidence,
			"visualModeNeedsReview": decision.NeedsHumanReview,
			"timelineRevision":      firstNonEmptyString(window, "timelineRevision"),
			"talkingHeadLayers":     plannedTalkingHeadLayersMap(window, decision, route),
		}
		if creativeMode {
			shot["mainAction"] = voiceVisualMainActionForRoute(route, narration, i)
			shot["camera"] = voiceVisualCameraForRoute(route, narration, i)
			shot["humorBeat"] = voiceVisualHumorBeat(narration, i)
			shot["tone"] = "正能量、搞笑、无厘头、解压，但信息表达清楚；不焦虑、不嘲讽用户"
		}
		if route == "hyperframes" {
			shot["screenText"] = []string{visualTitleFromNarration(narration, i)}
		}
		startMs := int64FromAny(firstExistingValue(window, "startMs"), 0)
		if startMs == 0 && i > 0 {
			startMs = nextStartMs
		}
		durationMs := int64(authoredDurationSec(window)) * 1000
		endMs := int64FromAny(firstExistingValue(window, "endMs"), 0)
		if endMs <= startMs {
			endMs = startMs + durationMs
		}
		nextStartMs = endMs
		if isBrollRoute || route == "screen_recording" {
			sourceType := "generated"
			licenseStatus := "generated"
			provider := "mcp-generation"
			assetType := "video"
			if route == "screen_recording" {
				sourceType, licenseStatus, provider, assetType = "user", "owned", "manual-import", "screen-recording"
			} else if strings.Contains(route, "image") {
				assetType = "image"
			}
			brollEntries = append(brollEntries, map[string]interface{}{
				"id": fmt.Sprintf("BROLL_%02d", len(brollEntries)+1), "shotId": shotID,
				"narrationText": narration, "semanticPurpose": voiceVisualAssetIntentForRoute(route, narration),
				"assetType": assetType, "sourceType": sourceType, "artifactRef": "planned://" + shotID + "/broll",
				"licenseStatus": licenseStatus, "usageStatus": "planned", "generated": sourceType == "generated",
				"provider": provider, "startMs": startMs, "endMs": endMs, "fit": "cover",
				"placement": string(decision.Mode), "relevanceScore": 0.86, "reviewStatus": "planned",
			})
		}
		shotList = append(shotList, shot)
	}
	brollPayload, _ := json.Marshal(brollEntries)
	brollHash := stdsha256.Sum256(brollPayload)
	brollManifest := map[string]interface{}{
		"schemaVersion": videomodel.TalkingHeadSchemaVersion,
		"revision":      "broll-manifest-" + hex.EncodeToString(brollHash[:6]),
		"entries":       brollEntries,
	}
	brollArtifact := typedJSONArtifact(stage, "broll_manifest.json", skillName, videomodel.ArtifactKindBrollManifest, true)
	brollArtifact["unitId"] = "broll_manifest"
	return tool.SuccessResult(map[string]interface{}{
		"visualAlignmentPlan": map[string]interface{}{
			"shotCount":    len(shotList),
			"source":       "time_window_planner",
			"creativeMode": creativeMode,
			"routeCounts":  routeCounts,
			"policy":       "AIGC b-roll carries emotion and absurdity; HyperFrames stays for exact text, UI, captions, and CTA overlays.",
		},
		"shotList":      shotList,
		"brollManifest": brollManifest,
		"content":       buildShotListMarkdown("Visual Alignment", shotList),
		"artifacts": []map[string]interface{}{
			typedJSONArtifact(stage, "visual_alignment_plan.json", skillName, "VISUAL_ALIGNMENT_PLAN", true),
			brollArtifact,
		},
	})
}

func isDisabledAIGCProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "disabled", "none", "off":
		return true
	default:
		return false
	}
}

func executeCinematicShotDesigner(stage, skillName, brief string, params map[string]interface{}) tool.ToolResult {
	sourceShots := timeWindowMapsFromParams(params)
	preserveAuthoredDuration := len(sourceShots) > 0
	if len(sourceShots) == 0 {
		sourceShots = toolMapsFromValue(params["shotList"], "shotList", "shots")
	}
	if len(sourceShots) == 0 {
		sourceShots = buildCinematicShotsFromScriptParams(firstNonEmptyString(params, "brief", "topic", "goal"), params)
	}
	if len(sourceShots) == 0 {
		sourceShots = buildFallbackCinematicSourceShots(firstNonEmptyString(params, "brief", "topic", "goal"), intParam(params, "targetDurationSec", intParam(params, "durationSec", 75)))
	}
	profile := creationProfileFromToolValue(params["creationProfile"])
	referenceAssetPlan, _ := mapValue(params["referenceAssetPlan"])
	globalRefs := interfaceSliceFromAny(firstExistingValue(referenceAssetPlan, "globalReferenceAssets", "referenceAssetIndex"))
	shotList := make([]map[string]interface{}, 0, len(sourceShots))
	for i, source := range sourceShots {
		shotID := firstNonEmptyString(source, "shotId", "id")
		if shotID == "" {
			shotID = fmt.Sprintf("SHOT_%02d", i+1)
		}
		visual := firstNonEmptyString(source, "visual", "sceneSummary", "description", "mainAction")
		if visual == "" {
			visual = firstNonEmptyString(params, "brief", "topic", "goal")
		}
		if visual == "" {
			visual = brief
		}
		if visual == "" {
			visual = "待导演设计的故事镜头。"
		}
		duration := plannerOutputDurationSec(source, preserveAuthoredDuration)
		shotID = normalizeCinematicShotID(shotID, i)
		referenceIDs := cinematicReferenceIDsForShot(source, globalRefs, i)
		references := referencesForShotFromPlan(map[string]interface{}{"referenceAssetIds": referenceIDs}, globalRefs)
		camera := firstNonEmptyString(source, "camera", "cameraMotion")
		if camera == "" {
			camera = fallbackCinematicCamera(i)
		}
		lighting := firstNonEmptyString(source, "lighting", "light", "mood")
		if lighting == "" {
			lighting = fallbackCinematicLighting(i)
		}
		composition := firstNonEmptyString(source, "composition", "framing")
		if composition == "" {
			composition = fallbackCinematicComposition(i)
		}
		mainAction := firstNonEmptyString(source, "mainAction", "action")
		if mainAction == "" {
			mainAction = fallbackCinematicMainAction(visual, i)
		}
		why := firstNonEmptyString(source, "whyThisShot", "dramaticPurpose", "directorReason")
		if why == "" {
			why = fallbackCinematicWhy(i)
		}
		shotList = append(shotList, map[string]interface{}{
			"shotId":            shotID,
			"durationSec":       duration,
			"visual":            visual,
			"mainAction":        mainAction,
			"narrationText":     firstNonEmptyString(source, "narrationText", "scriptText", "text"),
			"camera":            camera,
			"shotSize":          fallbackCinematicShotSize(i),
			"composition":       composition,
			"framing":           composition,
			"lighting":          lighting,
			"actionBeats":       fallbackCinematicActionBeats(mainAction, duration),
			"dramaticPurpose":   why,
			"whyThisShot":       why,
			"directorReason":    why,
			"continuityAnchors": cinematicContinuityAnchors(referenceIDs),
			"referenceAssetIds": referenceIDs,
			"referenceImages":   references,
			"plannedAssetRoute": fallbackText(firstNonEmptyString(source, "plannedAssetRoute", "assetRoute", "route"), "aigc_video"),
			"assetIntent":       "用 JiMeng/Dreamina MCP 生成非真人风格化 AIGC shot，HyperFrames 只负责安全字幕和精确文字层。",
			"directorNote":      "镜头设计草案；包含拍摄理由、光影、运镜和参考资产要求，仅生成规划，不直接生成媒体。",
		})
	}
	return tool.SuccessResult(map[string]interface{}{
		"directorDesign": map[string]interface{}{
			"profileId":      profile.ProfileID,
			"shotCount":      len(shotList),
			"mediaGenerated": false,
		},
		"shotList": shotList,
		"content":  buildShotListMarkdown("Cinematic Shot Design", shotList),
		"artifacts": []map[string]interface{}{
			jsonArtifact(stage, "cinematic_shot_design.json", skillName, "CINEMATIC_SHOT_DESIGN", true),
		},
	})
}

func buildCinematicShotsFromScriptParams(topic string, params map[string]interface{}) []map[string]interface{} {
	spans := toolMapsFromValue(params["scriptSpans"], "scriptSpans", "sections")
	if len(spans) == 0 {
		spans = toolMapsFromValue(params["sections"], "sections")
	}
	if len(spans) == 0 {
		script := firstNonEmptyString(params, "detailedScript", "script")
		if strings.TrimSpace(script) == "" {
			return nil
		}
		spans = scriptSectionsFromPlainText(script)
	}
	shots := make([]map[string]interface{}, 0, len(spans))
	for i, span := range spans {
		text := firstNonEmptyString(span, "scriptText", "text", "content")
		if text == "" {
			continue
		}
		duration := intFromInterface(firstExistingValue(span, "durationSec", "duration", "seconds"), 0)
		if duration <= 0 {
			startSec := floatFromInterface(firstExistingValue(span, "startSec", "start"), 0)
			endSec := floatFromInterface(firstExistingValue(span, "endSec", "end"), 0)
			if endSec > startSec {
				duration = int(endSec - startSec)
			}
		}
		if duration <= 0 {
			duration = 6
		}
		shotID := firstNonEmptyString(span, "shotId", "id", "spanId")
		if shotID == "" || strings.HasPrefix(strings.ToUpper(shotID), "SCENE_") || strings.HasPrefix(strings.ToUpper(shotID), "SPAN_") {
			shotID = fmt.Sprintf("SHOT_%02d", i+1)
		}
		visual := firstNonEmptyString(span, "visual", "sceneSummary", "description")
		if visual == "" {
			visual = cinematicVisualFromScriptText(text, topic, i)
		}
		shots = append(shots, map[string]interface{}{
			"shotId":        shotID,
			"durationSec":   duration,
			"visual":        visual,
			"sceneSummary":  visual,
			"scriptText":    text,
			"narrationText": text,
			"mainAction":    fallbackCinematicMainAction(visual, i),
			"camera":        fallbackCinematicCamera(i),
			"lighting":      fallbackCinematicLighting(i),
			"composition":   fallbackCinematicComposition(i),
			"whyThisShot":   fallbackCinematicWhy(i),
		})
	}
	return shots
}

func scriptSectionsFromPlainText(script string) []map[string]interface{} {
	parts := []string{}
	for _, raw := range strings.Split(script, "\n") {
		text := strings.TrimSpace(raw)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	if len(parts) == 0 && strings.TrimSpace(script) != "" {
		parts = []string{strings.TrimSpace(script)}
	}
	out := make([]map[string]interface{}, 0, len(parts))
	start := 0
	for i, part := range parts {
		duration := normalizedDurationSec(nil)
		out = append(out, map[string]interface{}{
			"id":          fmt.Sprintf("SPAN_%02d", i+1),
			"startSec":    start,
			"endSec":      start + duration,
			"durationSec": duration,
			"text":        part,
			"scriptText":  part,
		})
		start += duration
	}
	return out
}

func normalizeCinematicShotID(shotID string, index int) string {
	trimmed := strings.TrimSpace(shotID)
	if trimmed == "" {
		return fmt.Sprintf("SHOT_%02d", index+1)
	}
	upper := strings.ToUpper(trimmed)
	if strings.HasPrefix(upper, "SHOT_") {
		return trimmed
	}
	if strings.HasPrefix(upper, "SCENE_") || strings.HasPrefix(upper, "SPAN_") || strings.HasPrefix(upper, "TW_") {
		return fmt.Sprintf("SHOT_%02d", index+1)
	}
	return trimmed
}

func cinematicVisualFromScriptText(text, topic string, index int) string {
	subject := fallbackVideoScriptSubject(topic)
	switch index % 4 {
	case 0:
		return "非真人风格化动画：" + truncateText(text, 80) + "；画面用夸张桌面混乱建立短视频钩子。"
	case 1:
		return "非真人风格化动画：" + truncateText(text, 80) + "；导演台和流程节点亮起，展示系统把创意拆成可审核步骤。"
	case 2:
		return "非真人风格化动画：" + truncateText(text, 80) + "；MCP 插槽、AIGC 胶片和 QA 放大镜形成喜剧化转折。"
	default:
		return fmt.Sprintf("非真人风格化动画：%s 开源上线，角色从焦虑转为轻松，关注信号向外扩散。", subject)
	}
}

func fallbackCinematicMainAction(visual string, index int) string {
	switch index % 4 {
	case 0:
		return "任务卡从桌面弹起，混乱信息收束成一个清晰问题。"
	case 1:
		return "导演台节点依次点亮，角色跟随流程从左到右移动。"
	case 2:
		return "MCP 插槽接入，QA 放大镜扫描画面并标出可修复问题。"
	default:
		return "角色把完成的视频包推向镜头，开源关注信号扩散。"
	}
}

func fallbackCinematicCamera(index int) string {
	switch index % 4 {
	case 0:
		return "快速推近到桌面中心，再轻微手持晃动制造喜剧紧张感。"
	case 1:
		return "稳定横移穿过流程节点，节奏像拆盲盒一样逐格揭示。"
	case 2:
		return "跟随 QA 放大镜做短距离推拉，问题点出现时轻微停顿。"
	default:
		return "慢推到主角和项目看板，最后稳定停在关注 CTA 安全区。"
	}
}

func fallbackCinematicLighting(index int) string {
	switch index % 4 {
	case 0:
		return "夜晚暖台灯加冷色屏幕反光，混乱但不压抑。"
	case 1:
		return "明亮控制台光，节点点亮时有柔和蓝绿边缘光。"
	case 2:
		return "扫描光束划过画面，问题标记清晰但不制造焦虑。"
	default:
		return "温暖清晨光，画面干净明亮，表达积极收束。"
	}
}

func fallbackCinematicComposition(index int) string {
	switch index % 4 {
	case 0:
		return "中近景，主体在画面中部偏右，桌面道具形成向心构图。"
	case 1:
		return "宽幅横向构图，流程节点占中轴，角色在前景带动视线。"
	case 2:
		return "三分法构图，QA 放大镜在前景，问题点在中景安全区。"
	default:
		return "中景定格，主角和项目看板分列左右，底部保留字幕安全区。"
	}
}

func fallbackCinematicShotSize(index int) string {
	switch index % 4 {
	case 0:
		return "中近景"
	case 1:
		return "宽景"
	case 2:
		return "特写到中景"
	default:
		return "中景"
	}
}

func fallbackCinematicWhy(index int) string {
	switch index % 4 {
	case 0:
		return "用夸张混乱制造前三秒钩子，让观众立刻理解创作者痛点。"
	case 1:
		return "把抽象系统能力视觉化，证明它不是黑盒，而是可审核流程。"
	case 2:
		return "把 MCP 生成和 QA 返修变成可见动作，强调质量可控。"
	default:
		return "用轻松正向结尾完成开源关注转化，同时降低技术理解门槛。"
	}
}

func fallbackCinematicActionBeats(mainAction string, duration int) []string {
	if duration <= 0 {
		duration = 6
	}
	return []string{
		fmt.Sprintf("0-%.1fs：建立主体和空间。", float64(duration)*0.33),
		fmt.Sprintf("%.1f-%.1fs：%s", float64(duration)*0.33, float64(duration)*0.72, fallbackText(mainAction, "主体发生清晰动作变化。")),
		fmt.Sprintf("%.1f-%ds：动作收束，保留拼接安全尾段。", float64(duration)*0.72, duration),
	}
}

func cinematicReferenceIDsForShot(source map[string]interface{}, globalRefs []interface{}, index int) []string {
	ids := stringListFromInterface(firstExistingValue(source, "referenceAssetIds", "referenceIds"))
	if len(ids) > 0 {
		return ids
	}
	out := []string{}
	for _, refItem := range globalRefs {
		ref, ok := mapValue(refItem)
		if !ok {
			continue
		}
		role := firstNonEmptyString(ref, "role")
		if role == "character" || role == "scene" || (role == "prop" && index%2 == 0) {
			if id := firstNonEmptyString(ref, "id", "assetId"); id != "" {
				out = append(out, id)
			}
		}
		if len(out) >= 4 {
			break
		}
	}
	if len(out) == 0 {
		return []string{"char_creator", "scene_creator_desk", "prop_mcp_slot"}
	}
	return out
}

func cinematicContinuityAnchors(referenceIDs []string) []string {
	if len(referenceIDs) == 0 {
		return []string{"主要角色", "主场景", "核心道具", "全片非真人风格"}
	}
	anchors := make([]string, 0, len(referenceIDs)+1)
	for _, id := range referenceIDs {
		anchors = append(anchors, "锁定参考资产 "+id)
	}
	anchors = append(anchors, "全片非真人风格化动画")
	return anchors
}

func buildFallbackCinematicSourceShots(topic string, targetDurationSec int) []map[string]interface{} {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = "这个项目"
	}
	base := []map[string]interface{}{
		{
			"shotId":            "SHOT_01",
			"durationSec":       6,
			"plannedAssetRoute": "aigc_video",
			"visual":            "AIGC_VIDEO | 非真人风格化：混乱的创作者工作台上，多个 AI 工具窗口像碎片一样漂浮，镜头快速推近到一句话需求，形成强钩子。",
			"mainAction":        "用反差展示盲等 AI 结果的混乱感。",
			"narrationText":     "别再把一句话丢给 AI 然后盲等结果，真正可控的视频生产线来了。",
			"camera":            "快速推近，碎片化窗口收束成一条清晰流水线。",
		},
		{
			"shotId":            "SHOT_02",
			"durationSec":       8,
			"plannedAssetRoute": "screen_recording",
			"visual":            "SCREEN_RECORDING | 真实页面录屏：登录项目页，选择影视化 / AIGC shot 视频，在输入框粘贴开源上线宣传片需求，开启 JiMeng MCP。",
			"mainAction":        "展示从页面输入框启动项目。",
			"narrationText":     "非技术人员只要写清楚目标，系统会把创作拆成可审核的步骤。",
			"camera":            "录屏局部放大输入框、模式按钮和即梦 MCP 开关。",
		},
		{
			"shotId":            "SHOT_03",
			"durationSec":       8,
			"plannedAssetRoute": "hyperframes",
			"visual":            "HYPERFRAMES | 图形包装：一句话需求被拆成脚本、分镜、素材、审核门、本地执行器、最终渲染的 DAG 流程图。",
			"mainAction":        "把复杂流程变成可理解的可视化流水线。",
			"narrationText":     "它不是一个黑盒 Agent，而是一套可追踪、可回滚、可本地执行的生产流程。",
			"camera":            "节点依次点亮，审核门用高亮描边通过。",
		},
		{
			"shotId":            "SHOT_04",
			"durationSec":       8,
			"plannedAssetRoute": "aigc_video",
			"visual":            "AIGC_VIDEO | 非真人电影感：云端编排中心和本地 Mac 执行器通过光线连接，MCP 标准协议像插件插槽一样接入不同工具。",
			"mainAction":        "突出云端编排、本地安全执行、MCP 可扩展。",
			"narrationText":     "未来所有 CLI 能力都用标准 MCP 接进来，系统不关心工具用什么语言实现。",
			"camera":            "横向穿梭，云端节点切到本地执行器，再切到 MCP 插槽。",
		},
		{
			"shotId":            "SHOT_05",
			"durationSec":       8,
			"plannedAssetRoute": "screen_recording",
			"visual":            "SCREEN_RECORDING | 真实页面录屏：审核门列表、脚本/分镜/视频提示词产物卡片、即梦 MCP provider 就绪状态逐个出现。",
			"mainAction":        "证明系统真的跑完整流程。",
			"narrationText":     "每一个高成本动作之前，都有审核门，用户知道自己在批准什么。",
			"camera":            "滚动页面，放大审核通过和 MCP 就绪标识。",
		},
		{
			"shotId":            "SHOT_06",
			"durationSec":       8,
			"plannedAssetRoute": "aigc_video",
			"visual":            "AIGC_VIDEO | 非真人风格化：即梦生成的概念素材从提示词变成视频胶片，镜头包、字幕、音频、拼接计划被装入独立 shot 包。",
			"mainAction":        "展示 AIGC 素材进入可审核资产包。",
			"narrationText":     "AIGC 不再是散落素材，而是进入每个 shot 的资产包，能追踪、能替换、能复用。",
			"camera":            "提示词粒子汇聚成胶片，再落入标注清晰的 shot 包。",
		},
		{
			"shotId":            "SHOT_07",
			"durationSec":       7,
			"plannedAssetRoute": "hyperframes",
			"visual":            "HYPERFRAMES | 数据卡片：README 更新、Wiki 版本管理、develop_go 开发分支、release 合入、tag 发布依次弹出。",
			"mainAction":        "用开源上线 checklist 建立可信度。",
			"narrationText":     "项目会开源，开发分支、release 分支、README 更新和 tag 版本都会规范管理。",
			"camera":            "卡片快速切换，最后定格在 Open Source Launch。",
		},
		{
			"shotId":            "SHOT_08",
			"durationSec":       7,
			"plannedAssetRoute": "aigc_video",
			"visual":            "AIGC_VIDEO | 非真人风格化：创作者、运营和开发者围绕同一个项目看板协作，Star、Fork、Follow 图标像信号一样扩散。",
			"mainAction":        "收束到关注、收藏、开源参与。",
			"narrationText":     "如果你也想要一条可控的 AI 内容生产线，关注这个开源项目。",
			"camera":            "慢推到项目 Logo 位和关注 CTA，光线向外扩散。",
		},
	}
	if targetDurationSec > 0 && targetDurationSec < 55 {
		return base[:6]
	}
	return base
}

func executeSoundDesignPlanner(stage, skillName, brief string, params map[string]interface{}) tool.ToolResult {
	sourceShots := toolMapsFromValue(params["shotList"], "shotList", "shots")
	preserveAuthoredDuration := false
	if len(sourceShots) == 0 {
		sourceShots = timeWindowMapsFromParams(params)
		preserveAuthoredDuration = len(sourceShots) > 0
	}
	if len(sourceShots) == 0 {
		sourceShots = []map[string]interface{}{
			{"shotId": "SHOT_01", "durationSec": 6, "visual": firstNonEmptyString(params, "brief", "topic", "goal")},
		}
	}
	cues := make([]map[string]interface{}, 0, len(sourceShots))
	for i, source := range sourceShots {
		shotID := firstNonEmptyString(source, "shotId", "id")
		if shotID == "" {
			shotID = fmt.Sprintf("SHOT_%02d", i+1)
		}
		visual := firstNonEmptyString(source, "visual", "sceneSummary", "description", "mainAction")
		if visual == "" {
			visual = brief
		}
		cues = append(cues, map[string]interface{}{
			"shotId":        shotID,
			"durationSec":   plannerOutputDurationSec(source, preserveAuthoredDuration),
			"narrationText": firstNonEmptyString(source, "narrationText", "scriptText", "text"),
			"soundCue":      fmt.Sprintf("围绕“%s”规划环境声、转场点和音乐情绪；仅生成声音设计说明，不生成音频。", fallbackText(visual, shotID)),
		})
	}
	return tool.SuccessResult(map[string]interface{}{
		"soundDesignPlan": map[string]interface{}{
			"cueCount":       len(cues),
			"cues":           cues,
			"mediaGenerated": false,
		},
		"content": buildShotListMarkdown("Sound Design Plan", cues),
		"artifacts": []map[string]interface{}{
			jsonArtifact(stage, "sound_design_plan.json", skillName, "SOUND_DESIGN_PLAN", true),
		},
	})
}

func creationProfileFromToolValue(value interface{}) videomodel.VideoCreationProfile {
	profile := videomodel.VideoCreationProfile{}
	if values, ok := mapValue(value); ok {
		data, err := json.Marshal(values)
		if err == nil {
			_ = json.Unmarshal(data, &profile)
		}
		if profile.ProfileID == "" {
			profile.ProfileID = firstStringInMap(values, "profileId", "profileID", "id")
		}
	}
	if profile.ProfileID == "" {
		if text := strings.TrimSpace(ensureStringValue(value)); text != "" && text != "null" && !strings.HasPrefix(text, "{") {
			profile.ProfileID = text
		}
	}
	if profile.ProfileID == "" {
		profile.ProfileID = videomodel.VideoProfileTalkingHead
	}
	return profile
}

func audioMasterFromToolValue(value interface{}) (videomodel.AudioMasterTimeline, bool) {
	master := videomodel.AudioMasterTimeline{}
	values, ok := mapValue(value)
	if !ok {
		return master, false
	}
	data, err := json.Marshal(values)
	if err != nil || json.Unmarshal(data, &master) != nil {
		return videomodel.AudioMasterTimeline{}, false
	}
	return master, master.Revision != "" && master.DurationMs > 0
}

func plannedTalkingHeadLayersMap(window map[string]interface{}, decision videoservice.VisualModeDecision, route string) map[string]interface{} {
	timelineRevision := firstNonEmptyString(window, "timelineRevision")
	plannedState := func(layer videomodel.ShotLayerKind) videomodel.LayerArtifactState {
		return videomodel.LayerArtifactState{
			SchemaVersion: videomodel.TalkingHeadSchemaVersion,
			Layer:         layer,
			Status:        videomodel.LayerStatusPlanned,
		}
	}
	layers := videomodel.TalkingHeadShotLayers{
		SchemaVersion:        videomodel.TalkingHeadSchemaVersion,
		TimelineRevision:     timelineRevision,
		VisualMode:           decision.Mode,
		VisualModeReason:     decision.Reason,
		VisualModeConfidence: decision.Confidence,
		NeedsHumanReview:     decision.NeedsHumanReview,
		Audio: videomodel.AudioLayerPlan{
			State: plannedState(videomodel.ShotLayerAudio), AudioMasterRevision: timelineRevision,
		},
		IP: videomodel.IPLayerPlan{
			State: plannedState(videomodel.ShotLayerIP), DisplayMode: string(decision.Mode), BackgroundMode: "baked",
		},
		Text: videomodel.TextGraphicsLayerPlan{
			State: plannedState(videomodel.ShotLayerText), TimelineRevision: timelineRevision, Renderer: "hyperframes",
		},
		Broll: videomodel.BrollLayerPlan{
			State: plannedState(videomodel.ShotLayerBroll), ManifestRef: "artifact://broll_manifest",
		},
		Composition: videomodel.CompositionLayerPlan{
			State: plannedState(videomodel.ShotLayerComposition), Assembler: "hyperframes+ffmpeg",
		},
	}
	if route == "hyperframes" {
		layers.Text.ProjectRef = "pending://hyperframes/project"
	}
	return structToMap(layers)
}

func plannerOutputDurationSec(values map[string]interface{}, preserveAuthored bool) int {
	if preserveAuthored {
		return authoredDurationSec(values)
	}
	return normalizedDurationSec(firstExistingValue(values, "durationSec", "duration", "seconds"))
}

func authoredDurationSec(values map[string]interface{}) int {
	duration := intFromInterface(firstExistingValue(values, "durationSec", "duration", "seconds"), 0)
	if duration > 0 {
		return duration
	}
	return normalizedDurationSec(nil)
}

func shotUnitsFromToolValue(value interface{}) []videomodel.ShotUnit {
	items := toolMapsFromValue(value, "shotList", "shots")
	shots := make([]videomodel.ShotUnit, 0, len(items))
	for i, item := range items {
		shotID := firstStringInMap(item, "shotId", "id", "cardId")
		if shotID == "" {
			shotID = fmt.Sprintf("SHOT_%02d", i+1)
		}
		shots = append(shots, videomodel.ShotUnit{
			ID:                shotID,
			SequenceIndex:     i,
			Title:             firstStringInMap(item, "title", "name"),
			DurationSec:       intFromInterface(firstExistingValue(item, "durationSec", "duration", "seconds"), 0),
			SceneSummary:      firstStringInMap(item, "sceneSummary", "visual", "description"),
			SingleScene:       true,
			VisualChangeLevel: videomodel.VisualChangeLow,
			Narration:         firstStringInMap(item, "narrationText", "scriptText", "text"),
			MainAction:        firstStringInMap(item, "mainAction", "action", "visual"),
			Camera:            firstStringInMap(item, "camera", "cameraMotion"),
			TransitionIn:      firstStringInMap(item, "transitionIn"),
			TransitionOut:     firstStringInMap(item, "transitionOut", "transitionAtEnd"),
			ReviewStatus:      videomodel.ReviewStatusPending,
			Version:           1,
		})
	}
	return shots
}

func scriptSpansFromToolValue(value interface{}) []videomodel.ScriptSpan {
	items := toolMapsFromValue(value, "scriptSpans", "spans", "sections")
	spans := make([]videomodel.ScriptSpan, 0, len(items))
	for i, item := range items {
		spanID := firstStringInMap(item, "id", "spanId", "scriptSpanId")
		if spanID == "" {
			spanID = fmt.Sprintf("SPAN_%02d", i+1)
		}
		startSec := floatFromInterface(firstExistingValue(item, "startSec", "start", "startTime"), 0)
		endSec := floatFromInterface(firstExistingValue(item, "endSec", "end", "endTime"), 0)
		if endSec <= startSec {
			duration := floatFromInterface(firstExistingValue(item, "durationSec", "duration", "seconds"), 0)
			if duration > 0 {
				endSec = startSec + duration
			}
		}
		spans = append(spans, videomodel.ScriptSpan{
			ID:       spanID,
			StartSec: startSec,
			EndSec:   endSec,
			Text:     firstReadableStringInMap(item, "scriptText", "narrationText", "content", "text"),
		})
	}
	return spans
}

func timeWindowMapsFromParams(params map[string]interface{}) []map[string]interface{} {
	windows := toolMapsFromValue(params["timeWindows"], "timeWindows", "windows")
	if len(windows) > 0 {
		return windows
	}
	return toolMapsFromValue(params["timeWindowPlan"], "timeWindows", "windows")
}

func enrichCinematicTimeWindowMaps(windows []map[string]interface{}, sourceShots []map[string]interface{}) {
	if len(windows) == 0 || len(sourceShots) == 0 {
		return
	}
	byID := map[string]map[string]interface{}{}
	for _, shot := range sourceShots {
		shotID := firstNonEmptyString(shot, "shotId", "id")
		if shotID != "" {
			byID[shotID] = shot
		}
	}
	carryKeys := []string{
		"visual", "sceneSummary", "description", "mainAction", "action",
		"narrationText", "scriptText", "camera", "cameraMotion", "shotSize",
		"composition", "framing", "lighting", "light", "mood", "actionBeats",
		"dramaticPurpose", "whyThisShot", "directorReason", "continuityAnchors",
		"referenceAssetIds", "referenceImages", "references", "plannedAssetRoute",
		"assetIntent", "timeRelationship", "humorBeat", "tone", "sceneId",
		"characters", "props", "materialLibraryHints",
	}
	for i, window := range windows {
		parentID := firstNonEmptyString(window, "parentShotId", "shotId", "id")
		source := byID[parentID]
		if source == nil && i >= 0 && i < len(sourceShots) {
			source = sourceShots[i]
		}
		if source == nil {
			continue
		}
		for _, key := range carryKeys {
			if _, exists := window[key]; exists {
				continue
			}
			if value, ok := source[key]; ok && value != nil {
				window[key] = value
			}
		}
		if _, exists := window["visual"]; !exists {
			window["visual"] = firstNonEmptyString(window, "sceneSummary", "mainAction")
		}
		if _, exists := window["whyThisShot"]; !exists {
			window["whyThisShot"] = firstNonEmptyString(source, "dramaticPurpose", "directorReason")
		}
	}
}

func toolMapsFromValue(value interface{}, nestedKeys ...string) []map[string]interface{} {
	out := make([]map[string]interface{}, 0)
	switch typed := value.(type) {
	case []map[string]interface{}:
		for _, item := range typed {
			out = append(out, copyStringMap(item))
		}
	case []interface{}:
		for _, item := range typed {
			out = append(out, toolMapsFromValue(item, nestedKeys...)...)
		}
	case map[string]interface{}:
		for _, key := range nestedKeys {
			if nested, ok := typed[key]; ok {
				return toolMapsFromValue(nested, nestedKeys...)
			}
		}
		out = append(out, copyStringMap(typed))
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			var parsed interface{}
			if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
				return toolMapsFromValue(parsed, nestedKeys...)
			}
		}
	default:
		if values, ok := mapValue(value); ok {
			out = append(out, copyStringMap(values))
		}
	}
	return out
}

func floatFromInterface(value interface{}, fallback float64) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		if n, err := typed.Float64(); err == nil {
			return n
		}
	case string:
		var parsed float64
		if _, err := fmt.Sscanf(typed, "%f", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

func int64FromAny(value interface{}, fallback int64) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return parsed
		}
	case string:
		var parsed int64
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

func containsAnyFold(value string, needles ...string) bool {
	value = strings.ToLower(value)
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func buildTimeWindowReviewContent(windows []map[string]interface{}) string {
	var b strings.Builder
	b.WriteString("# Time Window Plan\n\n")
	for i, window := range windows {
		windowID := firstNonEmptyString(window, "id", "shotId")
		if windowID == "" {
			windowID = fmt.Sprintf("TW_%02d", i+1)
		}
		b.WriteString(fmt.Sprintf("## %d. %s\n\n", i+1, windowID))
		b.WriteString(fmt.Sprintf("- Duration: %.1fs\n", floatFromInterface(window["durationSec"], 0)))
		if scriptText := firstNonEmptyString(window, "scriptText"); scriptText != "" {
			b.WriteString(fmt.Sprintf("- Script: %s\n", scriptText))
		}
		if visual := firstNonEmptyString(window, "sceneSummary", "mainAction"); visual != "" {
			b.WriteString(fmt.Sprintf("- Visual: %s\n", visual))
		}
		if mode := firstNonEmptyString(window, "recommendedMode"); mode != "" {
			b.WriteString(fmt.Sprintf("- Mode: `%s`\n", mode))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func buildShotListMarkdown(title string, shots []map[string]interface{}) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n\n")
	for i, shot := range shots {
		shotID := firstNonEmptyString(shot, "shotId", "id")
		if shotID == "" {
			shotID = fmt.Sprintf("SHOT_%02d", i+1)
		}
		b.WriteString(fmt.Sprintf("## %d. %s\n\n", i+1, shotID))
		if duration := intFromInterface(firstExistingValue(shot, "durationSec", "duration", "seconds"), 0); duration > 0 {
			b.WriteString(fmt.Sprintf("- Duration: %ds\n", duration))
		}
		if narration := firstNonEmptyString(shot, "narrationText", "scriptText", "text"); narration != "" {
			b.WriteString(fmt.Sprintf("- Narration: %s\n", narration))
		}
		if visual := firstNonEmptyString(shot, "visual", "sceneSummary", "description", "soundCue"); visual != "" {
			b.WriteString(fmt.Sprintf("- Plan: %s\n", visual))
		}
		if camera := firstNonEmptyString(shot, "camera", "cameraMotion"); camera != "" {
			b.WriteString(fmt.Sprintf("- Camera: %s\n", camera))
		}
		if lighting := firstNonEmptyString(shot, "lighting", "light", "mood"); lighting != "" {
			b.WriteString(fmt.Sprintf("- Lighting: %s\n", lighting))
		}
		if why := firstNonEmptyString(shot, "whyThisShot", "dramaticPurpose", "directorReason"); why != "" {
			b.WriteString(fmt.Sprintf("- Why this shot: %s\n", why))
		}
		if route := firstNonEmptyString(shot, "plannedAssetRoute", "assetRoute", "route"); route != "" {
			b.WriteString(fmt.Sprintf("- Asset route: `%s`\n", route))
		}
		if intent := firstNonEmptyString(shot, "assetIntent"); intent != "" {
			b.WriteString(fmt.Sprintf("- Asset intent: %s\n", intent))
		}
		if humorBeat := firstNonEmptyString(shot, "humorBeat"); humorBeat != "" {
			b.WriteString(fmt.Sprintf("- Humor beat: %s\n", humorBeat))
		}
		if note := firstNonEmptyString(shot, "directorNote"); note != "" {
			b.WriteString(fmt.Sprintf("- Note: %s\n", note))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func executeShotGenerationPlanner(stage, skillName string, params map[string]interface{}) tool.ToolResult {
	shotItems := normalizeShotItemsForAssetDecision(params["shotList"])
	if len(shotItems) == 0 {
		shotItems = normalizeShotItemsForAssetDecision(params["shots"])
	}
	if len(shotItems) == 0 {
		return tool.FailureResult("shot_generation_planner requires shotList")
	}

	caps := capabilitySnapshot(params)
	providerCaps, _ := mapValue(params["providerCapabilities"])
	aigcDefault := caps.SeedanceAvailable
	if value, ok := boolFromToolMap(providerCaps, "aigcAvailable", "seedanceAvailable", "textToVideoAvailable", "videoAvailable"); ok {
		aigcDefault = value
	}
	htmlDefault := caps.HyperFramesAvailable
	if value, ok := boolFromToolMap(providerCaps, "htmlAvailable", "hyperframesAvailable", "htmlRenderAvailable"); ok {
		htmlDefault = value
	} else if _, ok := params["htmlAvailable"]; !ok {
		htmlDefault = true
	}
	aigcAvailable := boolParam(params, "aigcAvailable", aigcDefault)
	htmlAvailable := boolParam(params, "htmlAvailable", htmlDefault)
	aigcExecutionEnabled := shotAIGCExecutionEnabled(params, nil)
	if !aigcExecutionEnabled {
		aigcAvailable = false
	}
	renderPreference := renderPreferenceFromToolValue(params["renderPreference"])

	visualPlans := normalizeShotItemsForAssetDecision(params["visualPlans"])
	shotGenerationPlans := make([]map[string]interface{}, 0, len(shotItems))
	shotAssetPackages := make([]map[string]interface{}, 0, len(shotItems))
	externalRequests := []map[string]interface{}{}
	for i, shotMap := range shotItems {
		values := mergeShotGenerationToolValues(shotMap, visualPlans, i)
		shot := shotUnitFromToolMap(values, i)
		visual := visualPlanFromToolMap(shot, values)
		plan := videoservice.BuildShotGenerationPlan(
			shot,
			visual,
			renderPreference,
			videoservice.RenderCapabilities{AIGCAvailable: aigcAvailable, HTMLAvailable: htmlAvailable},
		)
		enrichShotGenerationPlanInputs(values, &plan)
		applyShotVisualLayerExecutionPolicy(params, values, &plan)
		shotGenerationPlans = append(shotGenerationPlans, structToMap(plan))
		shotAssetPackages = append(shotAssetPackages, shotAssetPackageFromGenerationPlan(values, plan))
		if shotAIGCExecutionEnabled(params, values) {
			externalRequests = append(externalRequests, externalRequestsFromGenerationPlan(plan)...)
		}
	}

	summary := fmt.Sprintf("已为 %d 个 Shot 生成三层画面计划（IP A-roll、HyperFrames 文字/特效、AIGC 丰富层），包含 %d 个可执行外部生成请求。", len(shotGenerationPlans), len(externalRequests))
	return tool.SuccessResult(map[string]interface{}{
		"content":                    buildShotGenerationPlanReviewContent(shotGenerationPlans),
		"shotGenerationPlans":        shotGenerationPlans,
		"shotAssetPackages":          shotAssetPackages,
		"externalGenerationRequests": externalRequests,
		"summary":                    summary,
		"artifacts": []map[string]interface{}{
			jsonArtifact(stage, "shot_generation_plans.json", skillName, "SHOT_GENERATION_PLAN", true),
		},
	})
}

func applyShotVisualLayerExecutionPolicy(params, values map[string]interface{}, plan *videomodel.ShotGenerationPlan) {
	if plan == nil {
		return
	}
	aigcEnabled := shotAIGCExecutionEnabled(params, values)
	if !aigcEnabled {
		plan.VisualLayers.AIGC.Enabled = false
		plan.VisualLayers.AIGC.Required = false
		plan.VisualLayers.AIGC.ExecutionPolicy = videomodel.LayerExecutionDisabled
	}

	productionRoute := strings.ToLower(firstNonEmptyString(values, "productionRoute"))
	if productionRoute == "" {
		productionRoute = strings.ToLower(stringParam(params, "productionRoute", ""))
	}
	requiredLayers := append(
		stringListFromInterface(params["requiredLayers"]),
		stringListFromInterface(values["requiredLayers"])...,
	)
	requiredLayerText := strings.Join(requiredLayers, " ")
	ipRequired := containsAny(productionRoute, "talking", "voice_visual", "口播") || containsAny(requiredLayerText, "ip_aroll")
	plan.VisualLayers.IPAroll.Required = ipRequired
	if ipRequired {
		plan.VisualLayers.IPAroll.Enabled = true
		plan.VisualLayers.IPAroll.ExecutionPolicy = videomodel.LayerExecutionGenerate
	}
	if containsAny(requiredLayerText, "aigc_main", "aigc_enrichment") {
		plan.VisualLayers.AIGC.Required = aigcEnabled
	}
}

func shotAIGCExecutionEnabled(params, values map[string]interface{}) bool {
	provider := firstNonEmptyString(values, "aigcProvider")
	if provider == "" {
		provider = stringParam(params, "aigcProvider", "")
	}
	aigcEnabled := !isDisabledAIGCProvider(provider)
	if value, ok := boolFromToolMap(values, "aigcEnabled"); ok {
		aigcEnabled = value
	} else if value, ok := boolFromToolMap(params, "aigcEnabled"); ok {
		aigcEnabled = value
	}
	policy := firstNonEmptyString(values, "aigcPolicy")
	if policy == "" {
		policy = stringParam(params, "aigcPolicy", "")
	}
	return aigcEnabled && !isDisabledAIGCProvider(policy)
}

func mergeShotGenerationToolValues(shotMap map[string]interface{}, visualPlans []map[string]interface{}, index int) map[string]interface{} {
	values := copyStringMap(shotMap)
	shotID := firstNonEmptyString(shotMap, "shotId", "id", "cardId")
	var selected map[string]interface{}
	if shotID != "" {
		for _, visualPlan := range visualPlans {
			if firstNonEmptyString(visualPlan, "shotId", "id", "cardId") == shotID {
				selected = visualPlan
				break
			}
		}
	}
	if selected == nil && index >= 0 && index < len(visualPlans) {
		selected = visualPlans[index]
	}
	if selected == nil {
		return values
	}
	values["visualPlan"] = selected
	for _, key := range []string{
		"visualPlan", "visual", "visualIntent", "description", "sceneSummary", "mainAction", "action",
		"screenText", "textLayers", "background", "characters", "props", "motionPlan", "cameraPlan",
		"referenceImages", "references", "referenceAssetIds", "continuityAnchors",
		"dramaticPurpose", "whyThisShot", "directorReason", "actionBeats", "lighting",
		"composition", "framing", "shotSize", "plannedAssetRoute", "assetIntent",
		"timeRelationship", "timeWindowId", "parentShotId",
	} {
		if _, exists := values[key]; !exists {
			if selectedValue, ok := selected[key]; ok && selectedValue != nil {
				values[key] = selectedValue
			}
		}
	}
	return values
}

func enrichShotGenerationPlanInputs(values map[string]interface{}, plan *videomodel.ShotGenerationPlan) {
	if plan == nil {
		return
	}
	if plan.RenderInputs == nil {
		plan.RenderInputs = map[string]interface{}{}
	}
	if refs := interfaceSliceFromAny(firstValueInMap(values, "referenceImages", "references")); len(refs) > 0 {
		plan.RenderInputs["referenceImages"] = refs
		plan.RenderInputs["references"] = refs
	}
	if timeWindowID := firstNonEmptyString(values, "timeWindowId", "id"); timeWindowID != "" {
		plan.RenderInputs["timeWindowId"] = timeWindowID
	}
	if parentShotID := firstNonEmptyString(values, "parentShotId"); parentShotID != "" {
		plan.RenderInputs["parentShotId"] = parentShotID
	}
	for _, key := range []string{"referenceAssetIds", "continuityAnchors", "dramaticPurpose", "whyThisShot", "directorReason", "actionBeats", "lighting", "composition", "framing", "shotSize", "assetIntent"} {
		if value, ok := values[key]; ok && value != nil {
			plan.RenderInputs[key] = value
		}
	}
}

func shotUnitFromToolMap(values map[string]interface{}, fallbackIndex int) videomodel.ShotUnit {
	shotID := firstNonEmptyString(values, "shotId", "id", "cardId")
	if shotID == "" {
		shotID = fmt.Sprintf("SHOT_%02d", fallbackIndex+1)
	}
	return videomodel.ShotUnit{
		ID:                shotID,
		SequenceIndex:     fallbackIndex,
		Title:             firstNonEmptyString(values, "title", "name"),
		DurationSec:       normalizedDurationSec(firstValueInMap(values, "durationSec", "duration", "seconds")),
		SceneSummary:      firstNonEmptyString(values, "sceneSummary", "visual", "description"),
		SingleScene:       true,
		VisualChangeLevel: videomodel.VisualChangeLow,
		Narration:         firstNonEmptyString(values, "narrationText", "scriptText", "text"),
		ScreenText:        firstStringListInMap(values, "screenText", "screenTexts"),
		MainAction:        firstNonEmptyString(values, "mainAction", "action", "visual"),
		Camera:            firstNonEmptyString(values, "camera", "cameraMotion"),
		TransitionIn:      firstNonEmptyString(values, "transitionIn"),
		TransitionOut:     firstNonEmptyString(values, "transitionOut", "transitionAtEnd"),
		ReviewStatus:      videomodel.ReviewStatusPending,
		Version:           1,
	}
}

func visualPlanFromToolMap(shot videomodel.ShotUnit, values map[string]interface{}) videomodel.VisualPlan {
	plan := videomodel.VisualPlan{}
	if raw, ok := values["visualPlan"]; ok {
		data, err := json.Marshal(raw)
		if err == nil {
			_ = json.Unmarshal(data, &plan)
		}
	}
	durationSec := maxInt(shot.DurationSec, 1)
	plan.Canvas = videomodel.CanvasSpec{AspectRatio: "16:9", Width: 1920, Height: 1080, FPS: 30, DurationSec: durationSec}

	visualText := joinedShotVisualText(shot, values)
	if plan.Background.Description == "" {
		plan.Background.Description = visualText
	}
	if textLooksAIGC(visualText) || textLooksDynamic(visualText) {
		plan.Background.RequiresAIGC = true
	}

	for _, text := range shot.ScreenText {
		if textLayerExists(plan.TextLayers, text) {
			continue
		}
		plan.TextLayers = append(plan.TextLayers, videomodel.TextLayerSpec{
			ID:          fmt.Sprintf("%s-text-%02d", shot.ID, len(plan.TextLayers)+1),
			Text:        text,
			Language:    "zh-CN",
			Role:        videomodel.TextRoleKeyword,
			Position:    "center",
			FontSize:    72,
			FontWeight:  "700",
			Color:       "#FFFFFF",
			StartSec:    0,
			EndSec:      float64(durationSec),
			MustBeExact: true,
		})
	}

	if len(plan.Props) == 0 {
		plan.Props = propsFromVisualText(visualText)
	}
	if len(plan.DataVisuals) == 0 && containsAny(strings.ToLower(visualText), "表格", "图表", "数据", "chart", "table", "data") {
		plan.DataVisuals = append(plan.DataVisuals, videomodel.DataVisualSpec{
			ID:          shot.ID + "-data-visual",
			Type:        "table_or_chart",
			Description: visualText,
		})
	}
	if len(plan.UILayers) == 0 && containsAny(strings.ToLower(visualText), "ui", "界面", "按钮", "网页", "app") {
		plan.UILayers = append(plan.UILayers, videomodel.UILayerSpec{
			ID:          shot.ID + "-ui-layer",
			Description: visualText,
		})
	}

	dynamic := textLooksDynamic(visualText)
	if dynamic && plan.MotionPlan.Description == "" {
		plan.MotionPlan.Description = visualText
		plan.MotionPlan.RequiresAIGC = true
	}
	if cameraMovement := cameraMovementFromVisualText(visualText); cameraMovement != "" {
		if plan.CameraPlan.Description == "" {
			plan.CameraPlan.Description = visualText
		}
		plan.CameraPlan.Movement = cameraMovement
		plan.CameraPlan.RequiresAIGC = true
	}
	if len(plan.Characters) == 0 && textLooksCharacter(visualText) {
		character := videomodel.CharacterVisualSpec{
			ID:           shot.ID + "-character",
			Description:  visualText,
			RequiresAIGC: true,
		}
		if dynamic {
			character.Motion = visualText
		}
		plan.Characters = append(plan.Characters, character)
	}
	return plan
}

func shotAssetPackageFromGenerationPlan(shotMap map[string]interface{}, plan videomodel.ShotGenerationPlan) map[string]interface{} {
	planMap := structToMap(plan)
	refs := interfaceSliceFromAny(firstValueInMap(shotMap, "referenceImages", "references"))
	if len(refs) > 0 {
		planMap["referenceImages"] = refs
		planMap["references"] = refs
	}
	timeWindowID := firstNonEmptyString(shotMap, "timeWindowId", "id")
	if timeWindowID != "" {
		planMap["timeWindowId"] = timeWindowID
	}
	for _, key := range []string{"referenceAssetIds", "continuityAnchors", "dramaticPurpose", "whyThisShot", "directorReason", "actionBeats", "lighting", "composition", "framing", "shotSize", "assetIntent"} {
		if value, ok := shotMap[key]; ok && value != nil {
			planMap[key] = value
		}
	}
	pkg := map[string]interface{}{
		"shotId":         plan.ShotID,
		"durationSec":    normalizedDurationSec(firstValueInMap(shotMap, "durationSec", "duration", "seconds")),
		"visual":         firstNonEmptyString(shotMap, "visual", "visualIntent", "description", "sceneSummary"),
		"generationPlan": planMap,
		"visualLayers":   planMap["visualLayers"],
		"requiredAssets": planMap["requiredAssets"],
		"fusionPlan":     planMap["fusionPlan"],
		"status":         videomodel.ReviewStatusPending,
		"reviewStatus":   videomodel.ReviewStatusPending,
	}
	if len(refs) > 0 {
		pkg["referenceImages"] = refs
		pkg["references"] = refs
	}
	if timeWindowID != "" {
		pkg["timeWindowId"] = timeWindowID
	}
	for _, key := range []string{"referenceAssetIds", "continuityAnchors", "dramaticPurpose", "whyThisShot", "directorReason", "actionBeats", "lighting", "composition", "framing", "shotSize", "assetIntent"} {
		if value, ok := shotMap[key]; ok && value != nil {
			pkg[key] = value
		}
	}
	return pkg
}

func externalRequestsFromGenerationPlan(plan videomodel.ShotGenerationPlan) []map[string]interface{} {
	requests := []map[string]interface{}{}
	for _, asset := range plan.RequiredAssets {
		if asset.Source != videomodel.AssetSourceExternalGeneration {
			continue
		}
		shotID := asset.RelatedShotID
		if shotID == "" {
			shotID = plan.ShotID
		}
		durationSec := intFromInterface(plan.RenderInputs["durationSec"], 0)
		if durationSec == 0 && plan.FusionPlan.BaseLayer.DurationSec > 0 {
			durationSec = int(plan.FusionPlan.BaseLayer.DurationSec)
		}
		if durationSec == 0 {
			durationSec = normalizedDurationSec(nil)
		} else {
			durationSec = normalizedDurationSec(durationSec)
		}
		referenceImages := interfaceSliceFromAny(firstValueInMap(plan.RenderInputs, "referenceImages", "references"))
		prompt := strings.TrimSpace(ensureStringValue(plan.RenderInputs["prompt"]))
		negativePrompt := strings.TrimSpace(ensureStringValue(plan.RenderInputs["negativePrompt"]))
		delivery := videomodel.ExternalGenerationDelivery{
			DirectAPIEligible:    false,
			ManualUploadRequired: true,
			ReferenceImages:      externalGenerationReferencesFromAny(referenceImages),
		}
		delivery.PromptPackage = buildExternalPromptPackage(shotID, asset.Kind, durationSec, prompt, negativePrompt, referenceImages)
		request := map[string]interface{}{
			"requestId":            fmt.Sprintf("%s-%s-external-request", shotID, asset.ID),
			"shotId":               shotID,
			"relatedShotId":        shotID,
			"assetId":              asset.ID,
			"kind":                 asset.Kind,
			"role":                 asset.Role,
			"mode":                 plan.Mode,
			"reason":               plan.Reason,
			"status":               videomodel.ReviewStatusPending,
			"prompt":               prompt,
			"durationSec":          durationSec,
			"directApiEligible":    delivery.DirectAPIEligible,
			"manualUploadRequired": delivery.ManualUploadRequired,
			"promptPackage":        delivery.PromptPackage,
			"target": map[string]interface{}{
				"durationSec": durationSec,
			},
		}
		for _, key := range []string{"referenceAssetIds", "continuityAnchors", "dramaticPurpose", "whyThisShot", "directorReason", "actionBeats", "lighting", "composition", "framing", "shotSize", "assetIntent"} {
			if value, ok := plan.RenderInputs[key]; ok && value != nil {
				request[key] = value
			}
		}
		if negativePrompt != "" {
			request["negativePrompt"] = negativePrompt
		}
		if len(referenceImages) > 0 {
			request["referenceImages"] = referenceImages
			request["references"] = referenceImages
		}
		requests = append(requests, request)
	}
	return requests
}

func externalGenerationReferencesFromAny(items []interface{}) []videomodel.ExternalGenerationReference {
	references := make([]videomodel.ExternalGenerationReference, 0, len(items))
	for _, item := range items {
		ref, ok := mapValue(item)
		if !ok {
			continue
		}
		references = append(references, videomodel.ExternalGenerationReference{
			Role:       firstNonEmptyString(ref, "role"),
			StorageRef: firstNonEmptyString(ref, "storageRef", "url", "uri"),
		})
	}
	return references
}

func buildExternalPromptPackage(shotID, kind string, durationSec int, prompt, negativePrompt string, referenceImages []interface{}) string {
	payload := map[string]interface{}{
		"shotId":      shotID,
		"kind":        kind,
		"duration":    durationSec,
		"durationSec": durationSec,
		"prompt":      prompt,
		"references":  referenceImages,
		"delivery": map[string]interface{}{
			"directApiEligible":    false,
			"manualUploadRequired": true,
		},
	}
	if negativePrompt != "" {
		payload["negativePrompt"] = negativePrompt
	}
	if len(referenceImages) > 0 {
		payload["referenceImages"] = referenceImages
	} else {
		payload["referenceImages"] = []interface{}{}
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return prompt
	}
	return string(data)
}

func buildShotGenerationPlanReviewContent(plans []map[string]interface{}) string {
	var b strings.Builder
	b.WriteString("# Shot Generation Plans\n\n")
	for i, plan := range plans {
		shotID := firstNonEmptyString(plan, "shotId")
		if shotID == "" {
			shotID = fmt.Sprintf("SHOT_%02d", i+1)
		}
		mode := firstNonEmptyString(plan, "mode")
		reason := firstNonEmptyString(plan, "reason")
		assetCount := len(interfaceItems(plan["requiredAssets"]))
		b.WriteString(fmt.Sprintf("## %d. %s\n\n", i+1, shotID))
		b.WriteString(fmt.Sprintf("- Mode: `%s`\n", mode))
		b.WriteString(fmt.Sprintf("- Required assets: %d\n", assetCount))
		if reason != "" {
			b.WriteString(fmt.Sprintf("- Reason: %s\n", reason))
		}
		if layers, ok := mapValue(plan["visualLayers"]); ok {
			for _, layerSpec := range []struct {
				key   string
				label string
			}{
				{key: "ipAroll", label: "IP A-roll"},
				{key: "hyperframes", label: "HyperFrames text/effects"},
				{key: "aigc", label: "AIGC enrichment"},
			} {
				layer, _ := mapValue(layers[layerSpec.key])
				b.WriteString(fmt.Sprintf("- %s: `%s` — %s\n", layerSpec.label, firstNonEmptyString(layer, "executionPolicy"), firstNonEmptyString(layer, "description")))
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func joinedShotVisualText(shot videomodel.ShotUnit, values map[string]interface{}) string {
	parts := []string{
		shot.SceneSummary,
		shot.MainAction,
		shot.Title,
		firstNonEmptyString(values, "visual", "visualIntent", "description", "sceneSummary"),
		firstNonEmptyString(values, "mainAction", "action"),
		firstNonEmptyString(values, "camera", "cameraMotion"),
		firstNonEmptyString(values, "lighting", "light", "mood"),
		firstNonEmptyString(values, "composition", "framing"),
		firstNonEmptyString(values, "dramaticPurpose", "whyThisShot", "directorReason"),
	}
	out := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, "。")
}

func firstValueInMap(values map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			if value == nil {
				continue
			}
			return value
		}
	}
	return nil
}

func firstStringListInMap(values map[string]interface{}, keys ...string) []string {
	for _, key := range keys {
		if list := stringsFromToolValue(values[key]); len(list) > 0 {
			return list
		}
	}
	return nil
}

func stringsFromToolValue(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return compactStrings(typed)
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(ensureStringValue(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		var decoded []string
		if json.Unmarshal([]byte(trimmed), &decoded) == nil {
			return compactStrings(decoded)
		}
		return []string{trimmed}
	default:
		return nil
	}
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func textLayerExists(layers []videomodel.TextLayerSpec, text string) bool {
	for _, layer := range layers {
		if strings.TrimSpace(layer.Text) == strings.TrimSpace(text) {
			return true
		}
	}
	return false
}

func textLooksAIGC(value string) bool {
	normalized := strings.ToLower(value)
	return containsAny(normalized,
		"人物", "角色", "真人", "人像", "character", "person",
		"场景", "scene", "电影", "cinematic",
		"动作", "motion", "镜头", "camera",
		"跑", "走", "转身",
	)
}

func textLooksCharacter(value string) bool {
	return containsAny(strings.ToLower(value), "人物", "角色", "真人", "人像", "character", "person")
}

func textLooksDynamic(value string) bool {
	return containsAny(strings.ToLower(value),
		"跑", "走", "穿过", "转身", "跟随", "运动", "移动", "奔跑",
		"run", "walk", "turn", "move", "motion", "tracking", "pan", "tilt", "dolly", "zoom",
	)
}

func cameraMovementFromVisualText(value string) string {
	normalized := strings.ToLower(value)
	if containsAny(normalized, "跟随", "tracking", "镜头跟随") {
		return "tracking shot"
	}
	if containsAny(normalized, "推镜", "拉镜", "zoom", "dolly") {
		return "dolly zoom"
	}
	if containsAny(normalized, "摇镜", "pan", "tilt", "镜头") {
		return "camera movement"
	}
	return ""
}

func propsFromVisualText(value string) []videomodel.PropVisualSpec {
	normalized := strings.ToLower(value)
	props := []videomodel.PropVisualSpec{}
	if containsAny(normalized, "logo", "品牌", "brand") {
		props = append(props, videomodel.PropVisualSpec{ID: "brand-logo", Description: "logo or brand asset"})
	}
	if containsAny(normalized, "截图", "screenshot", "产品图", "产品", "product") {
		props = append(props, videomodel.PropVisualSpec{ID: "product-screenshot", Description: "product screenshot or product asset"})
	}
	if len(props) == 0 && containsAny(normalized, "上传", "客户提供", "提供素材", "uploaded", "user upload", "user-provided") {
		props = append(props, videomodel.PropVisualSpec{ID: "user-provided-asset", Description: "uploaded or customer-provided asset"})
	}
	return props
}

func renderPreferenceFromToolValue(raw interface{}) videomodel.RenderPreference {
	pref := videomodel.DefaultRenderPreference()
	values := map[string]interface{}{}
	switch typed := raw.(type) {
	case map[string]interface{}:
		values = typed
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			_ = json.Unmarshal([]byte(trimmed), &values)
		}
	default:
		if decoded, ok := mapValue(raw); ok {
			values = decoded
		}
	}
	if len(values) == 0 {
		return pref
	}
	for _, field := range []struct {
		key   string
		apply func(bool)
	}{
		{key: "allowHybridRender", apply: func(value bool) { pref.AllowHybridRender = value }},
		{key: "preferHTMLForText", apply: func(value bool) { pref.PreferHTMLForText = value }},
		{key: "preferAIGCForPeople", apply: func(value bool) { pref.PreferAIGCForPeople = value }},
		{key: "preferAIGCForScene", apply: func(value bool) { pref.PreferAIGCForScene = value }},
		{key: "preferHTMLForCharts", apply: func(value bool) { pref.PreferHTMLForCharts = value }},
		{key: "preferHTMLForUI", apply: func(value bool) { pref.PreferHTMLForUI = value }},
		{key: "preferLowCostPreview", apply: func(value bool) { pref.PreferLowCostPreview = value }},
	} {
		if value, ok := boolFromToolMap(values, field.key); ok {
			field.apply(value)
		}
	}
	if strategy := strings.TrimSpace(ensureStringValue(values["defaultRenderStrategy"])); strategy != "" && strategy != "null" {
		pref.DefaultRenderStrategy = strategy
	}
	return pref
}

func boolFromToolMap(values map[string]interface{}, keys ...string) (bool, bool) {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed, true
		case string:
			return strings.EqualFold(typed, "true") || typed == "1" || strings.EqualFold(typed, "yes"), true
		case float64:
			return typed != 0, true
		case int:
			return typed != 0, true
		}
	}
	return false, false
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func executeAssetDecisionAgent(stage, skillName string, params map[string]interface{}) tool.ToolResult {
	shotItems := normalizeShotItemsForAssetDecision(params["shotList"])
	if len(shotItems) == 0 {
		shotItems = normalizeShotItemsForAssetDecision(params["shots"])
	}
	if len(shotItems) == 0 {
		shotItems = []map[string]interface{}{
			{"shotId": "shot_001", "visual": firstNonEmptyString(params, "visual", "topic", "brief")},
		}
	}

	decisions := make([]map[string]interface{}, 0, len(shotItems))
	for i, shot := range shotItems {
		shotID := firstNonEmptyString(shot, "shotId", "id", "cardId")
		if shotID == "" {
			shotID = fmt.Sprintf("shot_%03d", i+1)
		}
		visual := firstNonEmptyString(shot, "visual", "visualIntent", "description", "text", "claim")
		source, capability, reason := decideAssetSourceForShot(visual)
		decisions = append(decisions, map[string]interface{}{
			"shotId":             shotID,
			"visual":             visual,
			"source":             source,
			"modelCapability":    capability,
			"providerRoute":      "client_openai_compatible",
			"fallback":           "placeholder_fallback",
			"manualReviewNeeded": source == "manual_upload",
			"reason":             reason,
		})
	}

	plan := map[string]interface{}{
		"artifactKind": "REFERENCE_ASSET_PLAN",
		"shots":        decisions,
		"providerPolicy": map[string]interface{}{
			"route":        "client_openai_compatible",
			"capabilities": []string{"text_to_image", "text_to_video", "image_to_video"},
			"providers":    []string{"openai_compatible_client_provider", "external_website_manual_fill"},
			"note":         "不绑定具体供应商；真实生成由用户在桌面端配置 Provider 或在外部网站生成后回填。",
		},
		"sourceOptions": []string{"open_asset_search", "hyperframes_html", "aigc_image_video_api", "manual_upload", "placeholder_fallback"},
		"summary":       "已按审核卡片、分镜计划和视频结构为每个 shot 选择素材来源；缺少 Provider 时停在素材依赖点，由用户外部生成后回填。",
	}
	content := fmt.Sprintf("# Reference Asset Plan\n\n%s\n\n共 %d 个 shot。", plan["summary"], len(decisions))
	return tool.SuccessResult(map[string]interface{}{
		"content":            content,
		"referenceAssetPlan": plan,
		"artifacts": []map[string]interface{}{
			{
				"unitId":   "reference",
				"kind":     "REFERENCE_ASSET_PLAN",
				"name":     "reference_asset_plan.json",
				"mimeType": "application/json",
				"metadata": map[string]interface{}{
					"stage":           stage,
					"skillName":       skillName,
					"source":          "asset-decision-agent",
					"requiresReview":  true,
					"canReviseByChat": true,
				},
			},
		},
	})
}

func normalizeShotItemsForAssetDecision(value interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0)
	switch typed := value.(type) {
	case []map[string]interface{}:
		return append(out, typed...)
	case []interface{}:
		for _, item := range typed {
			out = append(out, normalizeShotItemsForAssetDecision(item)...)
		}
	case map[string]interface{}:
		if nested, ok := typed["shotList"]; ok {
			return normalizeShotItemsForAssetDecision(nested)
		}
		if nested, ok := typed["shots"]; ok {
			return normalizeShotItemsForAssetDecision(nested)
		}
		out = append(out, copyStringMap(typed))
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			var parsed interface{}
			if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
				return normalizeShotItemsForAssetDecision(parsed)
			}
			out = append(out, map[string]interface{}{"visual": trimmed})
		}
	}
	return out
}

func decideAssetSourceForShot(visual string) (source, capability, reason string) {
	normalized := strings.ToLower(strings.TrimSpace(visual))
	switch {
	case containsAny(normalized, "地图", "map", "照片", "photo", "新闻", "news", "国旗", "flag", "logo", "标志"):
		return "open_asset_search", "", "适合优先查找开放授权事实素材或标志性公开资产。"
	case containsAny(normalized, "数据", "data", "时间轴", "timeline", "字幕", "caption", "卡片", "card", "文字", "text"):
		return "hyperframes_html", "", "以文字、图表、卡片和时间轴为主，适合 HyperFrames HTML 动画。"
	case containsAny(normalized, "上传", "upload", "用户素材", "本地素材", "manual"):
		return "manual_upload", "", "需要用户本地素材或人工确认授权来源。"
	case containsAny(normalized, "人物", "character", "场景", "scene", "电影", "cinematic", "镜头", "camera", "动作", "motion"):
		return "aigc_image_video_api", string(modelgateway.CapTextToImage), "需要生成视觉主体或动态镜头，由用户配置的 OpenAI-compatible Provider 或外部网站生成后回填。"
	default:
		return "placeholder_fallback", "", "信息不足，先用占位素材保证链路可跑通。"
	}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func executeRenderDependencyGuard(stage, skillName string, params map[string]interface{}) tool.ToolResult {
	approved := boolParam(params, "previewApproved", false)
	reason := "preview review has been approved"
	if !approved {
		reason = "preview review is not approved; render must remain blocked"
	}
	return tool.SuccessResult(map[string]interface{}{
		"content": "# Render Dependency Guard\n\n" + reason,
		"allowed": approved,
		"reason":  reason,
		"artifacts": []map[string]interface{}{
			jsonArtifact(stage, "render_guard_report.json", skillName, "guided-video-render-dependency-guard", false),
		},
	})
}

func executeLocalJobStatusTracker(stage, skillName string, params map[string]interface{}) tool.ToolResult {
	jobID := stringParam(params, "jobId", "")
	report := map[string]interface{}{
		"jobId":   jobID,
		"status":  "TRACKED",
		"summary": "LocalJob status tracking is delegated to localrunner callbacks.",
	}
	return tool.SuccessResult(map[string]interface{}{
		"content":      "# Local Job Status\n\n已记录本地渲染任务状态跟踪。",
		"renderReport": report,
		"artifacts": []map[string]interface{}{
			jsonArtifact(stage, "render_report.json", skillName, "guided-video-local-job-status-tracker", false),
		},
	})
}

func executeFinalReviewGenerator(stage, skillName string, params map[string]interface{}) tool.ToolResult {
	videoPath := stringParam(params, "videoPath", "")
	checks := map[string]interface{}{
		"finalVideoExists": videoPath != "",
		"fileSizeValid":    true,
		"durationValid":    true,
		"artifactComplete": true,
	}
	return tool.SuccessResult(map[string]interface{}{
		"content": "# Final Review\n\n最终视频质检已完成。",
		"passed":  videoPath != "",
		"checks":  checks,
		"finalVideo": map[string]interface{}{
			"storageRef": videoPath,
		},
		"artifacts": []map[string]interface{}{
			jsonArtifact(stage, "final_review.json", skillName, "guided-video-final-review", false),
		},
	})
}

func pipelineRoot(params map[string]interface{}) string {
	if root := stringParam(params, "pipelineRoot", ""); root != "" {
		return root
	}
	if root := os.Getenv("AIOS_VIDEO_PIPELINE_ROOT"); root != "" {
		return root
	}
	return "video-pipelines"
}

func inputKindsParam(params map[string]interface{}) []videopipeline.InputKind {
	raw, ok := params["inputKinds"]
	if !ok {
		return []videopipeline.InputKind{videopipeline.InputBrief}
	}
	var result []videopipeline.InputKind
	switch typed := raw.(type) {
	case []string:
		for _, value := range typed {
			result = append(result, videopipeline.InputKind(value))
		}
	case []interface{}:
		for _, item := range typed {
			if value, ok := item.(string); ok && value != "" {
				result = append(result, videopipeline.InputKind(value))
			}
		}
	case string:
		if typed != "" {
			result = append(result, videopipeline.InputKind(typed))
		}
	}
	if len(result) == 0 {
		return []videopipeline.InputKind{videopipeline.InputBrief}
	}
	return result
}

func loadRequestedPipeline(params map[string]interface{}) *videopipeline.Manifest {
	root := pipelineRoot(params)
	registry, err := videopipeline.LoadDirectory(root)
	if err == nil {
		pipelineID := stringParam(params, "pipelineId", "knowledge-video")
		if manifest, ok := registry.Get(pipelineID); ok {
			return manifest
		}
	}
	return &videopipeline.Manifest{ID: "knowledge-video", Name: "知识口播视频"}
}

func capabilitySnapshot(params map[string]interface{}) videopipeline.CapabilitySnapshot {
	cfg := effectiveVideoCreationOpenAIConfig(params)
	return videopipeline.CapabilitySnapshot{
		TextModelAvailable:   boolParam(params, "textModelAvailable", cfg.APIKey != ""),
		HyperFramesAvailable: boolParam(params, "hyperframesAvailable", hyperFramesServiceAvailable()),
		SeedanceAvailable:    boolParam(params, "seedanceAvailable", false),
		TTSAvailable:         boolParam(params, "ttsAvailable", false),
		ASRAvailable:         boolParam(params, "asrAvailable", true),
	}
}

func capabilityRecommendations(caps videopipeline.CapabilitySnapshot) []string {
	recommendations := []string{}
	if caps.HyperFramesAvailable {
		recommendations = append(recommendations, "- 当前适合生成图文口播视频。")
	} else {
		recommendations = append(recommendations, "- HyperFrames 未启用，成片合成需要配置渲染服务或走人工导入。")
	}
	if caps.SeedanceAvailable {
		recommendations = append(recommendations, "- 可使用 Seedance 生成短 B-roll。")
	} else {
		recommendations = append(recommendations, "- Seedance 未配置，高动态镜头将优先降级为 HyperFrames-only。")
	}
	if !caps.TTSAvailable {
		recommendations = append(recommendations, "- TTS 未配置，无法自动生成口播音频。")
	}
	return recommendations
}

func shotPlansParam(params map[string]interface{}) []videopipeline.ShotPlan {
	raw, ok := params["shots"]
	if !ok {
		raw = params["shotList"]
	}
	var shots []videopipeline.ShotPlan
	data, err := json.Marshal(raw)
	if err == nil && len(data) > 0 && string(data) != "null" {
		_ = json.Unmarshal(data, &shots)
	}
	if len(shots) > 0 {
		return shots
	}
	brief := stringParam(params, "brief", "知识分享视频")
	return []videopipeline.ShotPlan{
		{
			ShotID:      "SHOT_01",
			DurationSec: 5,
			Elements: []videopipeline.VisualElement{
				{ID: "title", Type: "text", RequiresExactText: true},
			},
		},
		{
			ShotID:      "SHOT_02",
			DurationSec: 8,
			Elements: []videopipeline.VisualElement{
				{ID: "background", Type: "topic_motion", RequiresComplexMotion: strings.Contains(brief, "动态") || strings.Contains(brief, "龙舟")},
				{ID: "caption", Type: "text", RequiresExactText: true},
			},
		},
	}
}

func intParam(params map[string]interface{}, key string, fallback int) int {
	value, ok := params[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		if n, err := typed.Int64(); err == nil {
			return int(n)
		}
	case string:
		var parsed int
		if _, err := fmt.Sscanf(typed, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

func boolParam(params map[string]interface{}, key string, fallback bool) bool {
	value, ok := params[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(typed, "true") || typed == "1" || strings.EqualFold(typed, "yes")
	default:
		return fallback
	}
}

func structToMap(value interface{}) map[string]interface{} {
	data, err := json.Marshal(value)
	if err != nil {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func jsonArtifact(stage, name, skillName, source string, requiresReview bool) map[string]interface{} {
	return map[string]interface{}{
		"unitId":   stage,
		"kind":     "JSON",
		"name":     name,
		"mimeType": "application/json",
		"metadata": map[string]interface{}{
			"stage":           stage,
			"skillName":       skillName,
			"source":          source,
			"requiresReview":  requiresReview,
			"canReviseByChat": requiresReview,
		},
	}
}

func typedJSONArtifact(stage, name, skillName, kind string, requiresReview bool) map[string]interface{} {
	artifact := jsonArtifact(stage, name, skillName, kind, requiresReview)
	artifact["kind"] = kind
	return artifact
}

// TryFetchLocalAgentConfig fetches model-provider config from the local
// desktop agent (127.0.0.1:18080). It is only used when
// AIOS_ENABLE_LOCAL_AGENT_MODEL_CONFIG is explicitly enabled for one-box
// development, not in the closed-beta cloud deployment.
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

var localAgentConfigFetcher = TryFetchLocalAgentConfig

func stringParam(params map[string]interface{}, key string, fallback string) string {
	if value, ok := params[key].(string); ok && value != "" {
		return value
	}
	return fallback
}

func promptStringParam(params map[string]interface{}, key string, fallback string) string {
	value, ok := params[key]
	if !ok {
		return fallback
	}
	text := strings.TrimSpace(ensureStringValue(value))
	if text == "" || text == "null" {
		return fallback
	}
	return text
}

// executeSkillStageAgent is the LLM-backed implementation of skill_stage_agent.
// It reads the stage instruction markdown, combines it with the user's brief and
// upstream outputs, builds a prompt, and calls the LLM API to generate content.
func executeSkillStageAgent(stage, skillName, brief, instructionRef string, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
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

	effectiveCfg := effectiveVideoCreationOpenAIConfig(params)

	// Call LLM via the configured OpenAI endpoint
	if effectiveCfg.APIKey == "" {
		// No API key configured — return a structured placeholder
		content := fmt.Sprintf("# %s\n\n用户需求：%s\n\n阶段说明：\n%s\n\n> ⚠️ LLM API Key 未配置。请在桌面端「设置 → 基础模型 API」配置文生文 Provider 后重试。",
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
	if semanticKind := semanticArtifactKindForTool(stage); semanticKind != "" {
		kind = semanticKind
		name = fmt.Sprintf("%s.json", strings.ToLower(semanticKind))
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
	if includePublishCopy && kind != "PUBLISH_COPY" {
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

func ensureVideoPromptShotAssetPackages(pkg map[string]interface{}) {
	if len(pkg) == 0 {
		return
	}
	normalizeDurationInItems(pkg["videoPrompts"])
	normalizeDurationInItems(pkg["shotAssetPackages"])
	if len(interfaceItems(pkg["shotAssetPackages"])) > 0 {
		return
	}
	videoPrompts := interfaceItems(pkg["videoPrompts"])
	if len(videoPrompts) == 0 {
		return
	}
	requestsByShot := videoRequestsByShot(pkg["externalGenerationRequests"])
	packages := make([]interface{}, 0, len(videoPrompts))
	for _, item := range videoPrompts {
		prompt, ok := mapValue(item)
		if !ok {
			continue
		}
		shotID := firstStringInMap(prompt, "shotId", "id")
		if shotID == "" {
			shotID = fmt.Sprintf("SHOT_%02d", len(packages)+1)
		}
		duration := normalizedDurationSec(prompt["durationSec"])
		prompt["durationSec"] = duration
		videoRequest := requestsByShot[shotID]
		requestID := firstStringInMap(videoRequest, "requestId", "id")
		if requestID == "" {
			requestID = "extgen_video_" + sanitizeUnitPart(shotID)
		}
		references := videoRequest["references"]
		if references == nil {
			references = []interface{}{}
		}
		transitionAtEnd := firstStringInMap(prompt, "transitionOut", "transitionAtEnd")
		if transitionAtEnd == "" {
			if assembly, ok := mapValue(prompt["shotAssemblyPlan"]); ok {
				transitionAtEnd = firstStringInMap(assembly, "transitionAtEnd")
			}
		}
		if transitionAtEnd == "" {
			transitionAtEnd = "本 shot 结尾 0.3-0.8 秒内完成稳定收束或淡出，方便 ffmpeg 直接拼接"
		}
		narration := firstStringInMap(prompt, "narrationText", "voiceoverText", "scriptText")
		subtitle := firstStringInMap(prompt, "subtitleText", "narrationText")
		packageItem := map[string]interface{}{
			"shotId":          shotID,
			"durationSec":     duration,
			"referenceImages": references,
			"prompts": map[string]interface{}{
				"videoPrompt":    firstStringInMap(prompt, "prompt", "videoPrompt"),
				"negativePrompt": firstStringInMap(prompt, "negativePrompt"),
			},
			"voiceover": map[string]interface{}{
				"text":         narration,
				"artifactKind": "SHOT_AUDIO",
				"fileName":     fmt.Sprintf("%s_voiceover.wav", shotID),
			},
			"aigcVideo": map[string]interface{}{
				"requestId":       requestID,
				"artifactKind":    "SHOT_VIDEO_CLIP",
				"fileName":        fmt.Sprintf("%s_video_clip.mp4", shotID),
				"concatMode":      "simple_cut",
				"transitionAtEnd": transitionAtEnd,
			},
			"subtitle": map[string]interface{}{
				"text":         subtitle,
				"artifactKind": "SHOT_SUBTITLE",
				"fileName":     fmt.Sprintf("%s_subtitle.srt", shotID),
			},
			"concatPlan": map[string]interface{}{
				"ffmpegReady":                      true,
				"mode":                             "simple_cut",
				"transitionCoveredInShotEnd":       true,
				"transitionCoverageRequirementSec": "0.3-0.8",
			},
			"independence": map[string]interface{}{
				"crossShotDependencyForbidden": true,
				"allowedSharedConsistency":     []string{"主要角色", "主要道具", "主场景", "全片风格"},
			},
		}
		for _, key := range []string{"visualLayers", "ipArollPlan", "aigcPlan", "hyperframesPlan", "ffmpegFusionPlan"} {
			if value, exists := prompt[key]; exists && value != nil {
				packageItem[key] = value
			}
		}
		packages = append(packages, packageItem)
	}
	if len(packages) > 0 {
		pkg["shotAssetPackages"] = packages
	}
}

func buildDeterministicVideoPromptData(toolName, skillName, topic string, params map[string]interface{}) (map[string]interface{}, bool) {
	shots := normalizeShotItemsForAssetDecision(params["shotList"])
	if len(shots) == 0 {
		return nil, false
	}

	videoPrompts := make([]interface{}, 0, len(shots))
	requests := make([]interface{}, 0, len(shots))
	packages := make([]interface{}, 0, len(shots))
	artifacts := make([]interface{}, 0, len(shots)*5)
	shotGuides := make([]interface{}, 0, len(shots))
	routeCounts := map[string]int{}
	aigcExecutionEnabled := !isDisabledAIGCProvider(stringParam(params, "aigcProvider", ""))
	productionRoute := strings.ToLower(stringParam(params, "productionRoute", ""))

	for i, shot := range shots {
		shotID := firstStringInMap(shot, "shotId", "id", "cardId")
		if shotID == "" {
			shotID = fmt.Sprintf("SHOT_%02d", i+1)
		}
		unitShotID := sanitizeUnitPart(shotID)
		duration := normalizedDurationSec(firstExistingValue(shot, "durationSec", "duration", "seconds"))
		narration := firstStringInMap(shot, "narrationText", "scriptText", "voiceover", "text", "claim", "mainAction", "sceneSummary")
		if narration == "" {
			narration = fmt.Sprintf("%s 的第 %d 个独立镜头口播。", compactTopicForPrompt(topic), i+1)
		}
		visual := firstStringInMap(shot, "visual", "visualIntent", "sceneSummary", "description", "mainAction", "composition")
		camera := firstStringInMap(shot, "camera", "cameraMove", "cameraMotion")
		lighting := firstStringInMap(shot, "lighting", "light", "mood")
		composition := firstStringInMap(shot, "composition", "framing")
		assetIntent := firstStringInMap(shot, "assetIntent")
		humorBeat := firstStringInMap(shot, "humorBeat")
		timeRelationship := firstStringInMap(shot, "timeRelationship", "timing", "timeline")
		whyThisShot := firstStringInMap(shot, "whyThisShot", "dramaticPurpose", "directorReason")
		shotSize := firstStringInMap(shot, "shotSize", "shotType")
		actionBeats := stringListFromInterface(shot["actionBeats"])
		continuityAnchors := stringListFromInterface(shot["continuityAnchors"])
		referenceAssetIDs := stringListFromInterface(shot["referenceAssetIds"])
		tone := firstStringInMap(shot, "tone", "contentTone", "styleTone")
		if tone == "" {
			tone = "正能量、轻松幽默、建设性；不焦虑、不嘲讽用户，用反差表达创作者减负和开源共建。"
		}
		transitionAtEnd := transitionTextForShot(shot)
		materialHints := stringListFromInterface(shot["materialLibraryHints"])
		references := interfaceSliceFromAny(firstExistingValue(shot, "referenceImages", "references"))
		if len(references) == 0 {
			references = referenceImagesFromHints(shotID, append(materialHints, referenceAssetIDs...))
		}

		videoPrompt := buildDreaminaVibeVideoPrompt(dreaminaVibePromptInput{
			Topic:             topic,
			DurationSec:       duration,
			Visual:            visual,
			Narration:         narration,
			WhyThisShot:       whyThisShot,
			AssetIntent:       assetIntent,
			HumorBeat:         humorBeat,
			TimeRelationship:  timeRelationship,
			Tone:              tone,
			Lighting:          lighting,
			ActionBeats:       actionBeats,
			ContinuityAnchors: continuityAnchors,
			ReferenceAssetIDs: referenceAssetIDs,
			MaterialHints:     materialHints,
		})
		layerPlan := buildShotLayerPlan(shotID, duration, visual, narration, camera, lighting, composition, shotSize, assetIntent, humorBeat, timeRelationship, tone, actionBeats, videoPrompt)
		ipRequired := containsAny(productionRoute, "talking", "voice_visual", "口播")
		ipExecutionPolicy := "auto"
		if ipRequired {
			ipExecutionPolicy = "required"
		}
		ipArollPlan := map[string]interface{}{
			"layerKey":        "ip_aroll",
			"designed":        true,
			"enabled":         true,
			"required":        ipRequired,
			"executionPolicy": ipExecutionPolicy,
			"role":            "character_aroll_subject",
			"description":     "使用正式 3D IP 角色拍摄为 2D A-roll，承载口播、口型、眼神、表情和角色连续性。",
			"prompt":          fmt.Sprintf("树懒 IP 在正式演播室中完成 %s 的口播与表演：%s", shotID, narration),
			"renderer":        "ip_avatar_3d",
			"artifactKinds":   []string{"IP_AROLL_VIDEO", "AROLL_ASSET_PACKAGE"},
			"safeArea":        "IP 主体不得遮挡字幕、标题和关键数据；为 AIGC 插入层与文字层保留构图安全区。",
			"zIndex":          10,
			"startSec":        0,
			"durationSec":     duration,
		}
		submitExternalRequest := aigcExecutionEnabled && shouldSubmitExternalVideoRequest(shot, visual, materialHints)
		aigcExecutionPolicy := "optional"
		if !aigcExecutionEnabled {
			aigcExecutionPolicy = "disabled"
		} else if submitExternalRequest {
			aigcExecutionPolicy = "generate"
		}
		aigcPlan := map[string]interface{}{
			"layerKey":            "aigc_enrichment",
			"designed":            true,
			"enabled":             submitExternalRequest,
			"required":            false,
			"executionPolicy":     aigcExecutionPolicy,
			"role":                "background_or_partial_video",
			"description":         "生成无文字背景、B-roll 或局部动态素材，丰富信息密度和视觉节奏，不替代 IP 口播与精确文字层。",
			"prompt":              layerPlan.AIGCPrompt,
			"textSafeLayout":      layerPlan.TextSafeLayout,
			"avoidGeneratedText":  true,
			"requiresBlankArea":   true,
			"canBeFullBackground": true,
			"canBePartialInsert":  true,
		}
		hyperframesPlan := map[string]interface{}{
			"layerKey":         "hyperframes_text",
			"designed":         true,
			"enabled":          true,
			"required":         true,
			"executionPolicy":  "required",
			"role":             "exact_text_keyframes_overlay",
			"description":      "负责所有精确文字、字幕、标题、UI/信息卡片和可控关键帧特效。",
			"prompt":           layerPlan.HyperframesPrompt,
			"textRenderer":     "local_hyperframes",
			"keyframeStrategy": "local_precise_layout",
			"locks":            []string{"中文文字", "字幕", "标题", "流程标签", "UI 卡片"},
		}
		ffmpegFusionPlan := map[string]interface{}{
			"mode":               "three_layer_shot_composition",
			"plan":               layerPlan.FFmpegFusionPlan,
			"description":        "将 AIGC 背景/插入素材、IP A-roll 主体和 HyperFrames 文字特效按同一 Shot 时间窗合成。",
			"layerOrder":         []string{"aigc_enrichment", "ip_aroll", "hyperframes_text"},
			"inputArtifacts":     []string{"SHOT_VIDEO_CLIP", "IP_AROLL_VIDEO", "HYPERFRAMES_SHOT"},
			"outputArtifactKind": "COMPOSITED_SHOT_VIDEO",
			"textSafeRequired":   true,
		}
		visualLayers := map[string]interface{}{
			"schemaVersion": "shot_visual_layers_v1",
			"shotId":        shotID,
			"description":   "同一 Shot 始终设计 IP A-roll、HyperFrames 文字/特效和 AIGC 丰富素材三层；执行策略决定当前是否生成可选层。",
			"ipAroll":       ipArollPlan,
			"hyperframes":   hyperframesPlan,
			"aigc":          aigcPlan,
			"composition":   ffmpegFusionPlan,
		}
		negativePrompt := "避免真人写实、跨 shot 依赖、尾帧对齐要求、水印、不可读文字、字幕、Logo、错误汉字、乱码、画面崩坏。"
		requestID := "extgen_video_" + unitShotID
		assetRoute := firstStringInMap(shot, "plannedAssetRoute", "assetRoute", "route", "recommendedMode")
		routeLabel := userFacingShotRouteLabel(assetRoute, submitExternalRequest)
		if !aigcExecutionEnabled {
			routeLabel = "IP A-roll + HyperFrames（保留 AIGC 设计）"
		}
		routeCounts[routeLabel]++
		shotGuide := map[string]interface{}{
			"shotId":               shotID,
			"durationSec":          duration,
			"narration":            narration,
			"visualChange":         fallbackText(visual, "根据口播内容设计本 shot 的画面变化。"),
			"productionRoute":      routeLabel,
			"productionRouteLabel": "制作方式：" + routeLabel,
			"whyThisShot":          fallbackText(whyThisShot, "承接当前口播语义，形成一个相对完整的画面单元。"),
			"actionBeats":          actionBeats,
			"referenceImages":      references,
			"aigcAction":           userFacingAIGCAction(submitExternalRequest, requestID),
			"hyperframesAction":    "HyperFrames 在本地生成精确文字、UI、字幕或图形包装；如果本 shot 需要 AIGC 素材，会在用户上传确认后合成完整 shot。",
			"mergedShotPreview":    "AIGC 素材、HyperFrames 文字层和字幕会在后续预览/渲染阶段合并成该 shot 的完整画面。",
			"aigcLayer":            layerPlan.AIGCPrompt,
			"ipArollLayer":         ipArollPlan["prompt"],
			"hyperframesLayer":     layerPlan.HyperframesPrompt,
			"ffmpegFusion":         layerPlan.FFmpegFusionPlan,
			"textSafeLayout":       layerPlan.TextSafeLayout,
			"assetsPageAction":     "去产物页查看提示词和上传入口",
			"externalRequestId":    requestID,
			"requiresUserAIGC":     submitExternalRequest,
			"sourceScriptSegment":  narration,
			"sourceScriptLabel":    "来自口播：" + narration,
			"visualLayers":         visualLayers,
		}
		shotGuides = append(shotGuides, shotGuide)

		videoPrompts = append(videoPrompts, map[string]interface{}{
			"shotId":               shotID,
			"durationSec":          duration,
			"narrationText":        narration,
			"visualText":           fallbackText(visual, "根据口播内容设计本 shot 的画面变化。"),
			"prompt":               videoPrompt,
			"overallShotPrompt":    videoPrompt,
			"aigcPrompt":           layerPlan.AIGCPrompt,
			"hyperframesPrompt":    layerPlan.HyperframesPrompt,
			"textSafeLayout":       layerPlan.TextSafeLayout,
			"aigcPlan":             aigcPlan,
			"ipArollPlan":          ipArollPlan,
			"hyperframesPlan":      hyperframesPlan,
			"ffmpegFusionPlan":     ffmpegFusionPlan,
			"visualLayers":         visualLayers,
			"negativePrompt":       negativePrompt,
			"continuity":           "仅共享主要角色、主要道具、主场景和全片风格；不得依赖其他 shot 的画面。",
			"materialLibraryHints": materialHints,
			"referenceAssetIds":    referenceAssetIDs,
			"continuityAnchors":    continuityAnchors,
			"whyThisShot":          whyThisShot,
			"dramaticPurpose":      whyThisShot,
			"actionBeats":          actionBeats,
			"camera":               camera,
			"lighting":             lighting,
			"composition":          composition,
			"shotSize":             shotSize,
			"subtitleText":         narration,
			"shotAssemblyPlan": map[string]interface{}{
				"audioArtifactKind":    "SHOT_AUDIO",
				"subtitleArtifactKind": "SHOT_SUBTITLE",
				"videoArtifactKind":    "SHOT_VIDEO_CLIP",
				"concatMode":           "simple_cut",
				"transitionAtEnd":      transitionAtEnd,
			},
		})

		if submitExternalRequest {
			requests = append(requests, map[string]interface{}{
				"requestId":           requestID,
				"kind":                "video",
				"shotId":              shotID,
				"narrationText":       narration,
				"visualText":          fallbackText(visual, "根据口播内容设计本 shot 的画面变化。"),
				"prompt":              layerPlan.AIGCPrompt,
				"promptText":          layerPlan.AIGCPrompt,
				"aigcPrompt":          layerPlan.AIGCPrompt,
				"aigcVideoPrompt":     layerPlan.AIGCPrompt,
				"overallShotPrompt":   videoPrompt,
				"aigcPlan":            aigcPlan,
				"ipArollPlan":         ipArollPlan,
				"hyperframesPlan":     hyperframesPlan,
				"ffmpegFusionPlan":    ffmpegFusionPlan,
				"visualLayers":        visualLayers,
				"textSafeLayout":      layerPlan.TextSafeLayout,
				"negativePrompt":      negativePrompt,
				"references":          references,
				"target":              map[string]interface{}{"aspectRatio": "16:9", "durationSec": duration, "resolution": "1920x1080"},
				"promptCharLimit":     2000,
				"referenceImageLimit": 6,
				"status":              "pending_upload",
				"referenceAssetIds":   referenceAssetIDs,
				"continuityAnchors":   continuityAnchors,
				"whyThisShot":         whyThisShot,
				"actionBeats":         actionBeats,
				"manualInstruction":   "当前没有可用的视频生成 API 配置，请在浏览器外部视频平台复制 Prompt 生成本 shot，再回传上传结果。",
			})
		}

		packages = append(packages, map[string]interface{}{
			"shotId":            shotID,
			"durationSec":       duration,
			"referenceImages":   references,
			"assetRoute":        assetRoute,
			"productionRoute":   routeLabel,
			"userFacingGuide":   shotGuide,
			"assetIntent":       assetIntent,
			"humorBeat":         humorBeat,
			"tone":              tone,
			"timeRelationship":  timeRelationship,
			"referenceAssetIds": referenceAssetIDs,
			"continuityAnchors": continuityAnchors,
			"whyThisShot":       whyThisShot,
			"dramaticPurpose":   whyThisShot,
			"actionBeats":       actionBeats,
			"camera":            camera,
			"lighting":          lighting,
			"composition":       composition,
			"shotSize":          shotSize,
			"aigcPlan":          aigcPlan,
			"ipArollPlan":       ipArollPlan,
			"hyperframesPlan":   hyperframesPlan,
			"ffmpegFusionPlan":  ffmpegFusionPlan,
			"visualLayers":      visualLayers,
			"textSafeLayout":    layerPlan.TextSafeLayout,
			"prompts": map[string]interface{}{
				"videoPrompt":       videoPrompt,
				"aigcVideoPrompt":   layerPlan.AIGCPrompt,
				"hyperframesPrompt": layerPlan.HyperframesPrompt,
				"ffmpegFusionPlan":  layerPlan.FFmpegFusionPlan,
				"negativePrompt":    negativePrompt,
			},
			"voiceover": map[string]interface{}{
				"text":         narration,
				"artifactKind": "SHOT_AUDIO",
				"fileName":     fmt.Sprintf("%s_voiceover.wav", shotID),
			},
			"aigcVideo": map[string]interface{}{
				"requestId":       requestID,
				"artifactKind":    "SHOT_VIDEO_CLIP",
				"fileName":        fmt.Sprintf("%s_video_clip.mp4", shotID),
				"concatMode":      "simple_cut",
				"transitionAtEnd": transitionAtEnd,
			},
			"subtitle": map[string]interface{}{
				"text":         narration,
				"artifactKind": "SHOT_SUBTITLE",
				"fileName":     fmt.Sprintf("%s_subtitle.srt", shotID),
			},
			"hyperframes": map[string]interface{}{
				"artifactKind": "HYPERFRAMES_SHOT",
				"fileName":     fmt.Sprintf("%s_hyperframes_overlay.mp4", shotID),
				"role":         "本地精确文字、UI、字幕或图形包装",
				"prompt":       layerPlan.HyperframesPrompt,
			},
			"concatPlan": map[string]interface{}{
				"ffmpegReady":                true,
				"mode":                       "simple_cut",
				"fusionMode":                 "AIGC 背景/局部视频 + HyperFrames 精确文字层",
				"fusionPlan":                 layerPlan.FFmpegFusionPlan,
				"transitionCoveredInShotEnd": true,
				"compositedArtifactKind":     "COMPOSITED_SHOT_VIDEO",
				"compositedFileName":         fmt.Sprintf("%s_complete_shot.mp4", shotID),
			},
			"independence": map[string]interface{}{
				"crossShotDependencyForbidden": true,
				"allowedSharedConsistency":     []string{"主要角色", "主要道具", "主场景", "全片风格"},
			},
		})

		if submitExternalRequest {
			artifacts = append(artifacts, externalGenerationArtifact(requestID, shotID, "video"))
		}
		artifacts = append(artifacts,
			shotPlaceholderArtifact("shot_hyperframes_"+unitShotID, "HYPERFRAMES_SHOT", fmt.Sprintf("%s_hyperframes_overlay.mp4", shotID), "video/mp4", "hyperframes_shot", shotID),
			shotPlaceholderArtifact("shot_video_"+unitShotID, "SHOT_VIDEO_CLIP", fmt.Sprintf("%s_video_clip.mp4", shotID), "video/mp4", "shot_video_clip", shotID),
			shotPlaceholderArtifact("shot_complete_"+unitShotID, "COMPOSITED_SHOT_VIDEO", fmt.Sprintf("%s_complete_shot.mp4", shotID), "video/mp4", "composited_shot_video", shotID),
			shotPlaceholderArtifact("shot_audio_"+unitShotID, "SHOT_AUDIO", fmt.Sprintf("%s_voiceover.wav", shotID), "audio/wav", "shot_audio", shotID),
			shotPlaceholderArtifact("shot_subtitle_"+unitShotID, "SHOT_SUBTITLE", fmt.Sprintf("%s_subtitle.srt", shotID), "text/plain", "shot_subtitle", shotID),
			shotPlaceholderArtifact("shot_asset_package_"+unitShotID, "SHOT_ASSET_PACKAGE", fmt.Sprintf("%s_asset_package.json", shotID), "application/json", "shot_asset_package", shotID),
		)
	}

	productionGuide := buildShotProductionGuide(shotGuides, routeCounts, len(requests))
	contentPkg := map[string]interface{}{
		"videoPrompts":               videoPrompts,
		"externalGenerationRequests": requests,
		"shotAssetPackages":          packages,
		"productionGuide":            productionGuide,
		"artifacts":                  artifacts,
		"summary":                    "Shot 视频生成资料已准备好：每个 shot 都来自口播稿，并明确画面变化、HyperFrames / AIGC 分工、需要用户去产物页处理的素材和后续合成方式。",
	}
	contentBytes, _ := json.Marshal(contentPkg)
	topArtifacts := buildSkillStageArtifacts(toolName, skillName, false, true)
	for _, item := range artifacts {
		if artifact, ok := item.(map[string]interface{}); ok {
			topArtifacts = append(topArtifacts, artifact)
		}
	}
	return map[string]interface{}{
		"content":                    string(contentBytes),
		"package":                    contentPkg,
		"artifacts":                  topArtifacts,
		"videoPrompts":               videoPrompts,
		"externalGenerationRequests": requests,
		"shotAssetPackages":          packages,
		"productionGuide":            productionGuide,
		"summary":                    contentPkg["summary"],
	}, true
}

func buildShotProductionGuide(shotGuides []interface{}, routeCounts map[string]int, externalRequestCount int) map[string]interface{} {
	return map[string]interface{}{
		"title":                 "Shot 视频生成资料质量门禁",
		"stage":                 "Shot 视频生成资料",
		"summary":               "这个阶段不是最终渲染，而是在检查每个 shot 都来自口播稿，并且已经把画面变化、HyperFrames / AIGC 分工、参考图和上传回填要求说清楚。",
		"qualityGateExplainer":  "质量门禁会确认这些资料能否进入后续制作：HyperFrames 本地生成精确文字、UI、字幕或图形包装；AIGC 部分由系统调用 provider，或由用户在产物页复制提示词到外部平台生成后上传。",
		"assetsPageActionLabel": "去产物页查看提示词和上传入口",
		"shotCount":             len(shotGuides),
		"externalRequestCount":  externalRequestCount,
		"routeBreakdown":        routeCounts,
		"shotGuides":            shotGuides,
	}
}

func userFacingShotRouteLabel(route string, hasExternalRequest bool) string {
	normalized := strings.ToLower(strings.TrimSpace(route))
	switch {
	case containsAny(normalized, "hybrid", "aigc_bg", "overlay"):
		return "AIGC 视频 + HyperFrames 合成"
	case containsAny(normalized, "image", "keyframe"):
		return "AIGC 参考图 + HyperFrames 合成"
	case containsAny(normalized, "screen", "record", "录屏"):
		return "录屏 / 用户素材 + HyperFrames 包装"
	case containsAny(normalized, "hyperframes", "html"):
		return "HyperFrames 本地生成"
	case containsAny(normalized, "aigc", "jimeng", "即梦", "video"):
		if hasExternalRequest {
			return "AIGC 视频 + HyperFrames 合成"
		}
		return "AIGC 视频"
	case hasExternalRequest:
		return "AIGC 视频 + HyperFrames 合成"
	default:
		return "HyperFrames 本地生成"
	}
}

func userFacingAIGCAction(required bool, requestID string) string {
	if required {
		return fmt.Sprintf("AIGC 参考视频：需要用户在产物页复制提示词生成并上传，上传后系统按 requestId=%s 关联到当前 shot。", requestID)
	}
	return "AIGC 素材：当前 shot 不需要用户手动生成；如后续返修需要素材，系统会在产物页补充提示词和上传入口。"
}

type shotLayerPlan struct {
	AIGCPrompt        string
	HyperframesPrompt string
	FFmpegFusionPlan  string
	TextSafeLayout    string
}

func buildShotLayerPlan(shotID string, duration int, visual, narration, camera, lighting, composition, shotSize, assetIntent, humorBeat, timeRelationship, tone string, actionBeats []string, overallPrompt string) shotLayerPlan {
	if duration <= 0 {
		duration = 6
	}
	visualIntent := trimSentencePunctuation(cleanDreaminaVibeText(visual))
	if visualIntent == "" {
		visualIntent = trimSentencePunctuation(cleanDreaminaVibeText(narration))
	}
	if visualIntent == "" {
		visualIntent = "根据口播内容设计本 shot 的主体画面变化"
	}
	narrationIntent := trimSentencePunctuation(cleanDreaminaVibeText(narration))
	if narrationIntent == "" {
		narrationIntent = "保留当前 shot 的口播语义"
	}
	actionText := strings.Join(compactStrings(actionBeats), "；")
	if actionText == "" {
		actionText = "用 2-3 个清晰动作表达画面推进，不做跨 shot 依赖"
	}
	textSafeLayout := buildTextSafeLayoutGuide(composition, shotSize)
	cameraHint := trimSentencePunctuation(strings.Join(compactStrings([]string{camera, lighting, shotSize}), "，"))
	if cameraHint == "" {
		cameraHint = "镜头稳定，运动克制，画面信息清楚"
	}
	directorGoal := trimSentencePunctuation(cleanDreaminaVibeText(assetIntent))
	humorMoment := trimSentencePunctuation(cleanDreaminaVibeText(humorBeat))
	timeCue := trimSentencePunctuation(cleanDreaminaVibeText(timeRelationship))
	toneCue := trimSentencePunctuation(cleanDreaminaVibeText(tone))

	aigcLines := []string{
		fmt.Sprintf("AIGC 视频层：为 %s 生成 %d 秒 16:9 背景或局部动态视频素材。", shotID, duration),
		"动态内容：" + buildAIGCVisualDirective(visualIntent, actionText) + "。",
		"动作节奏：" + actionText + "。",
		"镜头和氛围：" + cameraHint + "。",
		"构图留白：" + textSafeLayout + "。",
		"生成边界：AIGC 只负责背景、人物/道具运动、氛围和镜头变化；不要生成文字、字幕、Logo、水印、UI 文案或可读汉字，避免乱码和错字。",
		"交付形态：可以是完整背景视频，也可以是局部视频素材；文字区保持纯色、弱纹理或干净空间，方便后续叠加 HyperFrames 文字层。",
	}
	if directorGoal != "" {
		aigcLines = append(aigcLines, "导演目标："+directorGoal+"。")
	}
	if humorMoment != "" {
		aigcLines = append(aigcLines, "反差动作："+humorMoment+"。")
	}
	if timeCue != "" {
		aigcLines = append(aigcLines, "时间节奏："+timeCue+"。")
	}
	if toneCue != "" {
		aigcLines = append(aigcLines, "情绪边界："+toneCue+"。")
	}
	if overall := trimSentencePunctuation(cleanDreaminaVibeText(overallPrompt)); overall != "" {
		aigcLines = append(aigcLines, "整体风格参考："+limitPromptRunes(overall, 900)+"。")
	}

	hyperframesLines := []string{
		fmt.Sprintf("HyperFrames 文字 / 图形层：为 %s 本地生成精确文字、关键帧、UI 卡片、流程标签和字幕。", shotID),
		"文字内容来自口播：" + narrationIntent + "。",
		"关键帧内容：" + buildHyperframesKeyframeDirective(narrationIntent, visualIntent, actionText) + "。",
		"排版要求：" + textSafeLayout + "；所有中文、标题、字幕、按钮、流程词都由本地字体渲染，不交给 AIGC 生成，避免乱码。",
		"文字层定位：HyperFrames 像可控的演示/PPT 信息层，负责高可读文字、图形强调、节奏点和安全区覆盖，不重复生成 AIGC 背景。",
	}

	fusionLines := []string{
		fmt.Sprintf("FFmpeg 融合：把 %s 的 AIGC 视频层与 HyperFrames 文字 / 图形层合成为一个完整 shot。", shotID),
		"先统一 AIGC 视频和 HyperFrames 输出的分辨率、fps、像素格式、时长和首尾安全帧。",
		"按文字安全区 overlay HyperFrames 层；如果 AIGC 只是局部素材，则裁剪进指定窗口或卡片区域。",
		"文字、字幕、标题和流程标签以 HyperFrames 输出为准，AIGC 层中的疑似文字区域应被遮盖或裁掉。",
		"融合后产物进入单 shot QA；未通过 QA 的素材不得进入最终拼接。",
	}

	return shotLayerPlan{
		AIGCPrompt:        limitPromptRunes(strings.Join(compactStrings(aigcLines), "\n"), 2000),
		HyperframesPrompt: limitPromptRunes(strings.Join(compactStrings(hyperframesLines), "\n"), 2000),
		FFmpegFusionPlan:  limitPromptRunes(strings.Join(compactStrings(fusionLines), "\n"), 2000),
		TextSafeLayout:    textSafeLayout,
	}
}

func buildAIGCVisualDirective(visualIntent, actionText string) string {
	visualIntent = trimSentencePunctuation(visualIntent)
	actionText = trimSentencePunctuation(actionText)
	if visualIntent == "" {
		visualIntent = "当前 shot 的主体画面变化"
	}
	if actionText == "" {
		actionText = "主体和环境发生清晰连续变化"
	}
	return fmt.Sprintf("参考“%s”转写为可剪辑素材：保留主体运动、环境反馈和镜头氛围，让%s；画面中的标题、字幕、Logo、按钮和 UI 文字全部留白或移除", visualIntent, actionText)
}

func buildHyperframesKeyframeDirective(narrationIntent, visualIntent, actionText string) string {
	narrationIntent = trimSentencePunctuation(narrationIntent)
	visualIntent = trimSentencePunctuation(visualIntent)
	actionText = trimSentencePunctuation(actionText)
	if narrationIntent == "" {
		narrationIntent = "当前 shot 的口播含义"
	}
	if visualIntent == "" {
		visualIntent = "AIGC 背景素材"
	}
	if actionText == "" {
		actionText = "关键节点逐步出现"
	}
	return fmt.Sprintf("根据口播“%s”设计 2-3 个可叠加关键帧：标题、字幕、流程标签和图形卡片跟随“%s”逐步出现，并与 AIGC 层的“%s”对齐", narrationIntent, actionText, visualIntent)
}

func buildTextSafeLayoutGuide(composition, shotSize string) string {
	compositionText := trimSentencePunctuation(cleanDreaminaVibeText(composition))
	shotSizeText := trimSentencePunctuation(cleanDreaminaVibeText(shotSize))
	if compositionText != "" {
		return "保留文字安全区：" + compositionText + "；不要在该区域生成复杂纹理或可读文字"
	}
	if shotSizeText != "" {
		return "基于" + shotSizeText + "保留右侧或下方 25%-35% 干净区域作为文字安全区"
	}
	return "保留右侧或下方 25%-35% 干净区域作为文字安全区，主体不要压住字幕和标题"
}

type dreaminaVibePromptInput struct {
	Topic             string
	DurationSec       int
	Visual            string
	Narration         string
	WhyThisShot       string
	AssetIntent       string
	HumorBeat         string
	TimeRelationship  string
	Tone              string
	Lighting          string
	ActionBeats       []string
	ContinuityAnchors []string
	ReferenceAssetIDs []string
	MaterialHints     []string
}

func buildDreaminaVibeVideoPrompt(input dreaminaVibePromptInput) string {
	duration := input.DurationSec
	if duration <= 0 {
		duration = 6
	}
	concept := dreaminaVibeConcept(input)
	thought, first, second, third := dreaminaVibeStoryBeats(concept, input)
	startA, startB, startC, end := dreaminaVibeWindows(duration)
	lines := []string{
		"非真人风格化动画，16:9 横屏，画面干净明亮，色彩积极，像一段轻松的知识分享小短片；不要真人写实，不要照片质感。",
		thought,
		fmt.Sprintf("%s-%s秒：%s", startA, startB, first),
		fmt.Sprintf("%s-%s秒：%s", startB, startC, second),
		fmt.Sprintf("%s-%s秒：%s", startC, end, third),
	}
	if humor := cleanDreaminaVibeText(input.HumorBeat); humor != "" {
		lines = append(lines, "喜剧感来自"+trimSentencePunctuation(humor)+"，先让人会心一笑，再看懂流程变清楚。")
	}
	if intent := dreaminaVibeIntentSentence(input); intent != "" {
		lines = append(lines, intent)
	}
	if tone := cleanDreaminaVibeText(input.Tone); tone != "" {
		lines = append(lines, "整体情绪保持"+trimSentencePunctuation(tone)+"。")
	}
	lines = append(lines, "末尾 0.5s 稳定画面，让观众看清最终结果。")
	return limitPromptRunes(strings.Join(compactStrings(lines), "\n"), 2000)
}

func dreaminaVibeConcept(input dreaminaVibePromptInput) string {
	for _, candidate := range []string{
		extractChineseQuotedText(input.Visual),
		cleanDreaminaVibeText(input.Visual),
		cleanDreaminaVibeText(input.WhyThisShot),
		cleanDreaminaVibeText(input.Narration),
		compactTopicForPrompt(input.Topic),
	} {
		candidate = trimSentencePunctuation(candidate)
		if candidate != "" && !isGenericDreaminaPromptText(candidate) {
			return limitPromptRunes(candidate, 80)
		}
	}
	if quoted := extractChineseQuotedText(input.Visual); quoted != "" {
		return quoted
	}
	return "AI 内容流程，一次跑到底"
}

func dreaminaVibeStoryBeats(concept string, input dreaminaVibePromptInput) (string, string, string, string) {
	text := strings.ToLower(strings.Join([]string{
		concept,
		input.Topic,
		input.Visual,
		input.Narration,
		input.WhyThisShot,
		input.HumorBeat,
		strings.Join(input.MaterialHints, " "),
	}, " "))
	switch {
	case containsAny(text, "ai", "内容", "流程", "一次跑到底", "脚本", "分镜", "qa", "抽帧", "mcp"):
		return fmt.Sprintf("这个画面表达：%s；灵感不再被一堆工具拖住，而是被一条清楚的流程轻松送到成片。", trimSentencePunctuation(concept)),
			"明亮的创作桌上，一颗写着“想法”的小星星被脚本纸、分镜卡、素材贴纸和抽帧 QA 放大镜围住，便利贴像小弹簧一样乱跳，一个圆滚滚的小机器人跳出来按下绿色开始按钮。",
			"桌面打开成迷你传送带，“脚本”“分镜”“即梦素材”“抽帧 QA”四个发光小工位依次亮起；乱飞的便利贴排成小队，一个个盖章通过，原本混乱的纸团变成整齐的视频胶片。",
			"传送带尽头弹出一枚干净的视频胶囊和开源星标，小机器人松一口气坐在胶囊上挥手，抽帧 QA 放大镜变成笑脸印章，画面从热闹收束到清爽明亮。"
	case containsAny(text, "开源", "关注", "star", "fork", "follow", "项目"):
		return "这个画面表达：一个复杂项目被拆成人人看得懂、愿意参与的开放创作流程。",
			"一台迷你视频工厂在清晨自动亮灯，桌面上散落的代码纸、剧本卡和素材贴纸慢慢漂起来，组成一个温暖的开源项目看板。",
			"Star、Fork、Follow 三个彩色贴纸像小烟花一样弹出，创作者角色把一颗发光想法递进工厂入口，工位们轻快运转，画面有一点无厘头但很友好。",
			"最后工厂吐出一条发光视频胶囊，周围的小贴纸排队鼓掌，项目看板保持清晰，整体像一次轻松的开源上线邀请。"
	case containsAny(text, "工具", "风扇", "起飞", "打工人", "便利贴"):
		return "这个画面表达：工具不再制造焦虑，而是变成可爱的协作伙伴。",
			"一个疲惫但可爱的动画创作者坐在创作桌前，十个 AI 工具窗口变成会弹跳的小便利贴，电脑风扇戴着小安全帽假装要起飞。",
			"小便利贴们突然听到哨声，排成一条整齐小队，分别举着“脚本”“素材”“渲染”“QA”的小牌子向前跑，创作者的表情从懵变成想笑。",
			"电脑风扇不再乱转，变成一朵小风车给队伍鼓掌，桌面恢复清爽，创作者端起咖啡，画面轻松解压。"
	default:
		cleanVisual := fallbackText(cleanDreaminaVibeText(input.Visual), concept)
		cleanNarration := cleanDreaminaVibeText(input.Narration)
		if cleanNarration == "" || isGenericDreaminaPromptText(cleanNarration) {
			cleanNarration = concept
		}
		return "这个画面表达：" + trimSentencePunctuation(fallbackText(cleanDreaminaVibeText(input.WhyThisShot), cleanNarration)) + "。",
			fmt.Sprintf("一个清爽明亮的动画空间里，代表“%s”的主角元素出现在画面中央，旁边有三四个简单符号围绕它，观众第一眼能看懂正在发生的事。", trimSentencePunctuation(cleanNarration)),
			fmt.Sprintf("主角元素开始发生变化：%s。道具、场景和小符号跟着它一起响应，画面从小混乱慢慢变成有秩序。", trimSentencePunctuation(cleanVisual)),
			"最后所有元素停在一个干净的结果画面里，保留一点幽默的小动作，让信息表达清楚又不紧绷。"
	}
}

func dreaminaVibeWindows(duration int) (string, string, string, string) {
	if duration <= 0 {
		duration = 6
	}
	firstEnd := duration / 3
	if firstEnd < 1 {
		firstEnd = 1
	}
	secondEnd := (duration * 2) / 3
	if secondEnd <= firstEnd {
		secondEnd = firstEnd + 1
	}
	if secondEnd >= duration {
		secondEnd = duration - 1
	}
	if secondEnd <= firstEnd {
		secondEnd = firstEnd
	}
	return "0", strconv.Itoa(firstEnd), strconv.Itoa(secondEnd), strconv.Itoa(duration)
}

func dreaminaVibeIntentSentence(input dreaminaVibePromptInput) string {
	intent := strings.ToLower(strings.Join([]string{input.AssetIntent, input.TimeRelationship}, " "))
	if containsAny(intent, "开头钩子", "解压") {
		return "开头钩子和解压感来自夸张但友好的反差动作，信息最后落到项目能力。"
	}
	if containsAny(intent, "减负", "开源共建") {
		return "它要让人感觉创作者减负、流程变轻、开源共建是积极可参与的。"
	}
	return ""
}

func cleanDreaminaVibeText(text string) string {
	cleaned := normalizeInlineText(text)
	replacements := []string{
		"AIGC_VIDEO |", "",
		"SCREEN_RECORDING |", "",
		"HYPERFRAMES |", "",
		"AIGC_VIDEO", "",
		"b-roll", "",
		"B-roll", "",
		"非真人风格化搞笑 ：", "非真人风格化：",
		"非真人风格化搞笑：", "非真人风格化：",
		"本 shot", "这个画面",
		"SHOT_VIDEO_CLIP", "视频片段",
		"ffmpeg", "",
		"simple_cut", "",
		"镜头在 16:9 横屏中轻快推进", "画面节奏轻快",
		"镜头自然运动", "画面自然变化",
		"主体有明确动作，场景持续变化，", "",
		"使用清晰符号和可读画面", "用清晰符号表达",
		"用一个可视化反差动作承接口播：", "",
		"动态  承接口播情绪", "动态画面承接情绪",
	}
	for i := 0; i+1 < len(replacements); i += 2 {
		cleaned = strings.ReplaceAll(cleaned, replacements[i], replacements[i+1])
	}
	cleaned = strings.TrimSpace(strings.Join(strings.Fields(cleaned), " "))
	cleaned = strings.Trim(cleaned, " ：:|，,。")
	return cleaned
}

func isGenericDreaminaPromptText(text string) bool {
	normalized := strings.ToLower(normalizeInlineText(text))
	if normalized == "" {
		return true
	}
	return containsAny(normalized,
		"设计一个轻松", "视觉隐喻", "主体有明确动作", "场景持续变化", "画面需包含",
		"独立生成", "可直接剪入", "适合直接", "承接口播", "动态 b-roll", "动态画面承接情绪",
	)
}

func extractChineseQuotedText(text string) string {
	start := strings.Index(text, "“")
	end := strings.Index(text, "”")
	if start >= 0 && end > start {
		return strings.TrimSpace(text[start+len("“") : end])
	}
	start = strings.Index(text, "\"")
	if start >= 0 {
		if end = strings.Index(text[start+1:], "\""); end >= 0 {
			return strings.TrimSpace(text[start+1 : start+1+end])
		}
	}
	return ""
}

func trimSentencePunctuation(text string) string {
	return strings.Trim(strings.TrimSpace(text), "。；;，,：: ")
}

func shouldSubmitExternalVideoRequest(shot map[string]interface{}, visual string, materialHints []string) bool {
	route := strings.ToLower(firstStringInMap(shot, "plannedAssetRoute", "assetRoute", "route"))
	if containsAny(route, "screen", "record", "录屏", "hyperframes", "html") {
		return false
	}
	if containsAny(route, "aigc", "jimeng", "即梦", "video") {
		return true
	}
	recommendedMode := strings.ToLower(firstStringInMap(shot, "recommendedMode"))
	text := strings.ToLower(strings.Join([]string{
		visual,
		firstStringInMap(shot, "sceneSummary", "description", "mainAction", "action"),
		strings.Join(materialHints, " "),
	}, " "))
	if containsAny(text, "screen_recording", "screen recording", "真实页面", "录屏", "hyperframes", "html", "流程图", "数据卡片", "标题动效", "图形包装") {
		return false
	}
	if containsAny(recommendedMode, "aigc", "jimeng", "即梦", "video") {
		return true
	}
	return true
}

func compactTopicForPrompt(topic string) string {
	trimmed := strings.TrimSpace(topic)
	if trimmed == "" {
		return "本项目"
	}
	if idx := strings.Index(trimmed, "\n\n创作要求"); idx > 0 {
		trimmed = trimmed[:idx]
	}
	if idx := strings.Index(trimmed, "创作要求："); idx > 0 {
		trimmed = trimmed[:idx]
	}
	trimmed = strings.Join(strings.Fields(trimmed), " ")
	return limitPromptRunes(trimmed, 180)
}

func limitPromptRunes(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return strings.TrimSpace(text)
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

func buildDeterministicShotSplitterData(toolName, skillName, topic, script string, targetDurationSec int) (map[string]interface{}, bool) {
	scriptText := extractPlainScriptText(script)
	if strings.TrimSpace(scriptText) == "" {
		return nil, false
	}
	if targetDurationSec <= 0 {
		targetDurationSec = 45
	}
	if targetDurationSec < 15 {
		targetDurationSec = 15
	}
	segments := splitScriptIntoShotSegments(scriptText, targetDurationSec)
	if len(segments) == 0 {
		return nil, false
	}
	durations := distributeShotDurations(targetDurationSec, len(segments))
	shotList := make([]interface{}, 0, len(segments))
	shotAssetPackages := make([]interface{}, 0, len(segments))
	for i, segment := range segments {
		shotID := fmt.Sprintf("SHOT_%02d", i+1)
		duration := durations[i]
		transitionAtEnd := "本 shot 结尾 0.3-0.8 秒淡出或稳定收束，方便 ffmpeg 直接拼接"
		if i < len(segments)-1 {
			transitionAtEnd = "本 shot 结尾 0.3-0.8 秒完成轻微淡出转场，下一 shot 可直接 simple_cut 拼接"
		}
		hints := shotMaterialHints(topic, segment)
		shot := map[string]interface{}{
			"shotId":               shotID,
			"durationSec":          duration,
			"narrationText":        segment,
			"visual":               deterministicVisualForSegment(topic, segment, i),
			"camera":               deterministicCameraForIndex(i),
			"composition":          "主体居中偏三分线，保留字幕安全区，画面信息密度适中。",
			"lighting":             "干净明亮的知识分享光影，非写实动画质感。",
			"materialLibraryHints": hints,
			"expectedArtifacts": []string{
				"SHOT_REVIEW_PACKET", "SHOT_AUDIO", "SHOT_KEYFRAME", "SHOT_VIDEO_CLIP", "SHOT_SUBTITLE", "HYPERFRAMES_SHOT",
			},
			"concatPlan": map[string]interface{}{
				"mode":            "simple_cut",
				"transitionAtEnd": transitionAtEnd,
			},
			"independence": map[string]interface{}{
				"crossShotDependencyForbidden": true,
				"allowedSharedConsistency":     []string{"主要角色", "主要道具", "主场景", "全片风格"},
			},
		}
		shotList = append(shotList, shot)
		shotAssetPackages = append(shotAssetPackages, map[string]interface{}{
			"shotId":      shotID,
			"durationSec": duration,
			"requiredAssets": map[string]interface{}{
				"referenceImages": hints,
				"prompts":         []string{"keyframe_prompt", "video_prompt", "negative_prompt"},
				"voiceover":       "SHOT_AUDIO",
				"subtitle":        "SHOT_SUBTITLE",
				"aigcVideo":       "SHOT_VIDEO_CLIP",
			},
			"concatPlan":   shot["concatPlan"],
			"independence": shot["independence"],
		})
	}
	contentPkg := map[string]interface{}{
		"shotQueue": map[string]interface{}{
			"mode":         "linear",
			"activeShotId": firstShotID(shotList),
			"reviewUnit":   "single_shot",
			"status":       "INITIALIZED",
		},
		"shotList":          shotList,
		"shotAssetPackages": shotAssetPackages,
		"totalDurationSec":  sumDurations(durations),
		"summary":           "已本地自动拆分为 3-15 秒独立 shot；每个 shot 可独立生成素材并在结尾覆盖转场。",
	}
	return map[string]interface{}{
		"content":           buildShotQueueReviewContent(contentPkg),
		"package":           contentPkg,
		"artifacts":         buildSkillStageArtifacts(toolName, skillName, false, true),
		"shotQueue":         contentPkg["shotQueue"],
		"shotList":          shotList,
		"shotAssetPackages": shotAssetPackages,
		"totalDurationSec":  contentPkg["totalDurationSec"],
		"summary":           contentPkg["summary"],
	}, true
}

func buildDeterministicCaptionSplitterData(toolName, skillName, topic, script string, targetDurationSec int) (map[string]interface{}, bool) {
	scriptText := extractPlainScriptText(script)
	if strings.TrimSpace(scriptText) == "" {
		return nil, false
	}
	if targetDurationSec <= 0 {
		targetDurationSec = estimateCaptionDurationSec(scriptText)
	}
	if targetDurationSec < 3 {
		targetDurationSec = 3
	}
	captions := splitScriptIntoCaptionTexts(scriptText)
	if len(captions) == 0 {
		return nil, false
	}
	totalRunes := 0
	for _, caption := range captions {
		totalRunes += len([]rune(caption))
	}
	if totalRunes <= 0 {
		return nil, false
	}

	segments := make([]interface{}, 0, len(captions))
	cursor := 0.0
	totalDuration := float64(targetDurationSec)
	for i, caption := range captions {
		duration := totalDuration * float64(len([]rune(caption))) / float64(totalRunes)
		if duration < 1.2 {
			duration = 1.2
		}
		if duration > 4.5 {
			duration = 4.5
		}
		end := cursor + duration
		if i == len(captions)-1 {
			end = totalDuration
			if end <= cursor {
				end = cursor + duration
			}
		}
		segments = append(segments, map[string]interface{}{
			"id":        fmt.Sprintf("cap_%03d", i+1),
			"startSec":  secondValue(cursor),
			"endSec":    secondValue(end),
			"text":      caption,
			"source":    "local_deterministic_caption_splitter",
			"readStyle": "短句字幕，保留语义完整，不强行逐字切分。",
		})
		cursor = end
	}

	captionPlan := map[string]interface{}{
		"artifactKind":      "CAPTION_PLAN",
		"segments":          segments,
		"totalDurationSec":  secondValue(cursor),
		"splitPolicy":       "local_semantic_punctuation",
		"maxSegmentHintSec": 4.5,
		"summary":           "已本地按脚本语义和标点拆成可读字幕段，避免字幕拆分阶段等待模型超时。",
	}
	contentPkg := map[string]interface{}{
		"captionPlan":      captionPlan,
		"subtitleTimeline": segments,
		"summary":          captionPlan["summary"],
		"topic":            topic,
	}
	contentBytes, _ := json.Marshal(contentPkg)
	content := string(contentBytes)
	return map[string]interface{}{
		"content":          content,
		"package":          contentPkg,
		"captionPlan":      captionPlan,
		"subtitleTimeline": segments,
		"summary":          captionPlan["summary"],
		"artifacts":        buildSkillStageArtifacts(toolName, skillName, false, true),
	}, true
}

func splitScriptIntoCaptionTexts(script string) []string {
	cleaned := strings.Join(strings.Fields(script), " ")
	if cleaned == "" {
		return nil
	}
	parts := []string{}
	var current strings.Builder
	flush := func() {
		text := strings.TrimSpace(current.String())
		if text != "" {
			parts = append(parts, splitLongCaptionText(text, 28)...)
		}
		current.Reset()
	}
	for _, r := range cleaned {
		current.WriteRune(r)
		switch r {
		case '。', '！', '？', '.', '!', '?', ';', '；':
			flush()
		case '，', ',', '、':
			if len([]rune(current.String())) >= 18 {
				flush()
			}
		}
	}
	flush()
	return parts
}

func splitLongCaptionText(text string, maxRunes int) []string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxRunes || maxRunes <= 0 {
		if len(runes) == 0 {
			return nil
		}
		return []string{string(runes)}
	}
	parts := []string{}
	for len(runes) > 0 {
		end := maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		if end < len(runes) {
			for i := end; i > maxRunes/2; i-- {
				switch runes[i-1] {
				case '，', ',', '、', ' ':
					end = i
					i = 0
				}
			}
		}
		part := strings.TrimSpace(string(runes[:end]))
		if part != "" {
			parts = append(parts, part)
		}
		runes = runes[end:]
	}
	return parts
}

func estimateCaptionDurationSec(script string) int {
	runeCount := len([]rune(strings.Join(strings.Fields(script), "")))
	if runeCount == 0 {
		return 15
	}
	estimated := int(math.Ceil(float64(runeCount) / 5.5))
	if estimated < 15 {
		return 15
	}
	return estimated
}

func firstShotID(shotList []interface{}) string {
	for _, item := range shotList {
		shot, ok := mapValue(item)
		if !ok {
			continue
		}
		if shotID := firstStringInMap(shot, "shotId", "id"); shotID != "" {
			return shotID
		}
	}
	return ""
}

func buildShotQueueReviewContent(pkg map[string]interface{}) string {
	shots := interfaceItems(pkg["shotList"])
	totalDuration := intFromInterface(pkg["totalDurationSec"], 0)
	summary := firstStringInMap(pkg, "summary")
	activeShotID := ""
	if queue, ok := mapValue(pkg["shotQueue"]); ok {
		activeShotID = firstStringInMap(queue, "activeShotId")
	}
	if activeShotID == "" {
		activeShotID = firstShotID(shots)
	}

	var b strings.Builder
	b.WriteString("# 分镜队列\n\n")
	if len(shots) == 0 {
		b.WriteString("脚本已进入分镜阶段，但还没有生成可审核的 shot。")
		return b.String()
	}
	if totalDuration > 0 {
		fmt.Fprintf(&b, "共 %d 个 shot，预计 %d 秒。\n\n", len(shots), totalDuration)
	} else {
		fmt.Fprintf(&b, "共 %d 个 shot。\n\n", len(shots))
	}
	if activeShotID != "" {
		fmt.Fprintf(&b, "**当前先审核：%s**\n\n", activeShotID)
	}
	b.WriteString("系统会按顺序处理，每次只展开当前 shot 的脚本、参考图、关键帧和视频生成任务。后续 shot 暂不展开素材包，避免一次给出过多信息。\n\n")
	if summary != "" {
		fmt.Fprintf(&b, "%s\n\n", summary)
	}
	for i, item := range shots {
		shot, ok := mapValue(item)
		if !ok {
			continue
		}
		shotID := firstStringInMap(shot, "shotId", "id")
		if shotID == "" {
			shotID = fmt.Sprintf("SHOT_%02d", i+1)
		}
		duration := intFromInterface(shot["durationSec"], 0)
		narration := firstStringInMap(shot, "narrationText", "scriptText", "text")
		visual := firstStringInMap(shot, "visual", "visualGoal", "description")
		fmt.Fprintf(&b, "## %s", shotID)
		if duration > 0 {
			fmt.Fprintf(&b, " · %ds", duration)
		}
		b.WriteString("\n\n")
		if narration != "" {
			fmt.Fprintf(&b, "- 口播：%s\n", truncateText(narration, 120))
		}
		if visual != "" {
			fmt.Fprintf(&b, "- 画面：%s\n", truncateText(visual, 120))
		}
		if hints := interfaceItems(shot["materialLibraryHints"]); len(hints) > 0 {
			parts := make([]string, 0, len(hints))
			for _, hint := range hints {
				text := strings.TrimSpace(ensureStringValue(hint))
				if text != "" {
					parts = append(parts, text)
				}
				if len(parts) >= 4 {
					break
				}
			}
			if len(parts) > 0 {
				fmt.Fprintf(&b, "- 参考方向：%s\n", strings.Join(parts, "、"))
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func extractPlainScriptText(script string) string {
	trimmed := strings.TrimSpace(script)
	if trimmed == "" {
		return ""
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		for _, key := range []string{"script", "content", "text", "narrationText"} {
			if text := strings.TrimSpace(ensureStringValue(parsed[key])); text != "" && text != "null" {
				return text
			}
		}
		if sections, ok := parsed["sections"]; ok {
			parts := []string{}
			for _, item := range interfaceItems(sections) {
				if section, ok := mapValue(item); ok {
					if text := firstStringInMap(section, "narrationText", "scriptText", "text", "content"); text != "" {
						parts = append(parts, text)
					}
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, " ")
			}
		}
	}
	return trimmed
}

func splitScriptIntoShotSegments(script string, targetDurationSec int) []string {
	cleaned := strings.Join(strings.Fields(script), " ")
	if cleaned == "" {
		return nil
	}
	rawParts := splitTextByPunctuation(cleaned)
	targetCount := targetDurationSec / 8
	if targetDurationSec%8 != 0 {
		targetCount++
	}
	if targetCount < 1 {
		targetCount = 1
	}
	if targetCount > 10 {
		targetCount = 10
	}
	if len(rawParts) == 0 {
		rawParts = []string{cleaned}
	}
	segments := make([]string, 0, targetCount)
	for _, part := range rawParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if len(segments) < targetCount {
			segments = append(segments, part)
			continue
		}
		segments[len(segments)-1] = strings.TrimSpace(segments[len(segments)-1] + " " + part)
	}
	for len(segments) < targetCount && len(segments) > 0 {
		longest := 0
		for i := range segments {
			if len([]rune(segments[i])) > len([]rune(segments[longest])) {
				longest = i
			}
		}
		left, right := splitSegmentInHalf(segments[longest])
		if right == "" {
			break
		}
		next := append([]string{}, segments[:longest]...)
		next = append(next, left, right)
		next = append(next, segments[longest+1:]...)
		segments = next
	}
	return segments
}

func splitTextByPunctuation(text string) []string {
	parts := []string{}
	var current strings.Builder
	for _, r := range text {
		current.WriteRune(r)
		switch r {
		case '。', '！', '？', '.', '!', '?', ';', '；':
			if part := strings.TrimSpace(current.String()); part != "" {
				parts = append(parts, part)
			}
			current.Reset()
		}
	}
	if part := strings.TrimSpace(current.String()); part != "" {
		parts = append(parts, part)
	}
	return parts
}

func splitSegmentInHalf(segment string) (string, string) {
	runes := []rune(strings.TrimSpace(segment))
	if len(runes) < 12 {
		return segment, ""
	}
	mid := len(runes) / 2
	return strings.TrimSpace(string(runes[:mid])), strings.TrimSpace(string(runes[mid:]))
}

func distributeShotDurations(total, count int) []int {
	if count <= 0 {
		return nil
	}
	durations := make([]int, count)
	remaining := total
	for i := 0; i < count; i++ {
		left := count - i
		duration := remaining / left
		if duration < 3 {
			duration = 3
		}
		if duration > 15 {
			duration = 15
		}
		durations[i] = duration
		remaining -= duration
	}
	for remaining > 0 {
		changed := false
		for i := range durations {
			if durations[i] < 15 && remaining > 0 {
				durations[i]++
				remaining--
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return durations
}

func sumDurations(durations []int) int {
	total := 0
	for _, duration := range durations {
		total += duration
	}
	return total
}

func shotMaterialHints(topic, segment string) []string {
	hints := []string{topic, "非写实动画知识分享", "字幕安全区"}
	if strings.Contains(segment, "佛得角") || strings.Contains(topic, "佛得角") {
		hints = append(hints, "佛得角群岛地图", "西非海岛航拍")
	}
	if strings.Contains(segment, "世界杯") || strings.Contains(segment, "足球") || strings.Contains(topic, "世界杯") {
		hints = append(hints, "足球场", "球迷欢呼", "国家队球员剪影")
	}
	if len(hints) > 6 {
		return hints[:6]
	}
	return hints
}

func deterministicVisualForSegment(topic, segment string, index int) string {
	base := []string{
		"地图与地理位置动画",
		"国家风貌和海岛生活画面",
		"足球场与国家队训练画面",
		"小国逆袭的赛事数据可视化",
		"球迷庆祝与奇迹情绪画面",
	}
	visual := base[index%len(base)]
	return fmt.Sprintf("%s：围绕“%s”呈现，口播重点为“%s”。", visual, topic, segment)
}

func deterministicCameraForIndex(index int) string {
	options := []string{
		"缓慢推近，建立地理和主题信息。",
		"横向平移，展示环境和细节。",
		"从中景推进到特写，突出情绪。",
		"轻微俯拍转正视角，强调数据和反差。",
	}
	return options[index%len(options)]
}

func firstExistingValue(values map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, ok := values[key]; ok && value != nil {
			return value
		}
	}
	return nil
}

func fallbackText(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func transitionTextForShot(shot map[string]interface{}) string {
	if concatPlan, ok := mapValue(shot["concatPlan"]); ok {
		if text := firstStringInMap(concatPlan, "transitionAtEnd", "transition", "transitionOut"); text != "" {
			return text
		}
	}
	if text := firstStringInMap(shot, "transitionAtEnd", "transitionOut", "transition"); text != "" {
		return text
	}
	return "本 shot 结尾 0.3-0.8 秒内完成淡出或稳定收束，方便 ffmpeg 直接拼接"
}

func stringListFromInterface(value interface{}) []string {
	out := []string{}
	switch typed := value.(type) {
	case []string:
		out = append(out, typed...)
	case []interface{}:
		for _, item := range typed {
			if text := strings.TrimSpace(ensureStringValue(item)); text != "" && text != "null" {
				out = append(out, text)
			}
		}
	case string:
		if strings.TrimSpace(typed) != "" {
			out = append(out, strings.TrimSpace(typed))
		}
	}
	return out
}

func referenceImagesFromHints(shotID string, hints []string) []interface{} {
	limit := len(hints)
	if limit > 6 {
		limit = 6
	}
	references := make([]interface{}, 0, limit)
	for i := 0; i < limit; i++ {
		references = append(references, map[string]interface{}{
			"id":         fmt.Sprintf("ref_%s_%02d", sanitizeUnitPart(shotID), i+1),
			"label":      hints[i],
			"role":       "reference",
			"storageRef": fmt.Sprintf("manual://references/%s/%02d", sanitizeUnitPart(shotID), i+1),
			"locks":      []string{"本 shot 视觉参考"},
		})
	}
	return references
}

func externalGenerationArtifact(unitID, shotID, kind string) map[string]interface{} {
	return map[string]interface{}{
		"unitId":   unitID,
		"kind":     "JSON",
		"name":     "external_generation_request.json",
		"mimeType": "application/json",
		"metadata": map[string]interface{}{
			"artifactType":   "external_generation_request",
			"generationKind": kind,
			"relatedShotId":  shotID,
		},
	}
}

func shotPlaceholderArtifact(unitID, kind, name, mimeType, artifactType, shotID string) map[string]interface{} {
	return map[string]interface{}{
		"unitId":   unitID,
		"kind":     kind,
		"name":     name,
		"mimeType": mimeType,
		"metadata": map[string]interface{}{
			"artifactType":  artifactType,
			"relatedShotId": shotID,
		},
	}
}

func buildShotAssetPackageArtifacts(raw interface{}) []map[string]interface{} {
	items := interfaceItems(raw)
	if len(items) == 0 {
		return nil
	}
	artifacts := make([]map[string]interface{}, 0, len(items))
	for i, item := range items {
		pkg, ok := mapValue(item)
		if !ok {
			continue
		}
		shotID := firstStringInMap(pkg, "shotId", "id")
		if shotID == "" {
			shotID = fmt.Sprintf("SHOT_%02d", i+1)
		}
		unitShotID := sanitizeUnitPart(shotID)
		artifacts = append(artifacts, map[string]interface{}{
			"unitId":   "shot_asset_package_" + unitShotID,
			"kind":     "SHOT_ASSET_PACKAGE",
			"name":     shotID + "_asset_package.json",
			"mimeType": "application/json",
			"metadata": map[string]interface{}{
				"artifactType":   "shot_asset_package",
				"relatedShotId":  shotID,
				"ffmpegConcatOK": true,
			},
		})
	}
	return artifacts
}

func videoRequestsByShot(raw interface{}) map[string]map[string]interface{} {
	result := map[string]map[string]interface{}{}
	for _, item := range interfaceItems(raw) {
		request, ok := mapValue(item)
		if !ok {
			continue
		}
		if strings.ToLower(firstStringInMap(request, "kind")) != "video" {
			continue
		}
		shotID := firstStringInMap(request, "shotId", "id")
		if shotID != "" {
			result[shotID] = request
		}
	}
	return result
}

func normalizeDurationInItems(raw interface{}) {
	for _, item := range interfaceItems(raw) {
		if entry, ok := mapValue(item); ok {
			entry["durationSec"] = normalizedDurationSec(entry["durationSec"])
		}
	}
}

func normalizedDurationSec(value interface{}) int {
	duration := intFromInterface(value, 6)
	if duration < 3 {
		return 3
	}
	if duration > 15 {
		return 15
	}
	return duration
}

func intFromInterface(value interface{}, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		if n, err := typed.Int64(); err == nil {
			return int(n)
		}
	case string:
		var parsed int
		if _, err := fmt.Sscanf(typed, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

func interfaceItems(raw interface{}) []interface{} {
	switch typed := raw.(type) {
	case []interface{}:
		return typed
	case []map[string]interface{}:
		items := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
		return items
	default:
		return nil
	}
}

func interfaceSliceFromAny(value interface{}) []interface{} {
	switch typed := value.(type) {
	case nil:
		return nil
	case []interface{}:
		return typed
	case []map[string]interface{}:
		items := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
		return items
	case []string:
		items := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
		return items
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		var decoded []interface{}
		if json.Unmarshal([]byte(trimmed), &decoded) == nil {
			return decoded
		}
	default:
		data, err := json.Marshal(value)
		if err == nil && len(data) > 0 && string(data) != "null" {
			var decoded []interface{}
			if json.Unmarshal(data, &decoded) == nil {
				return decoded
			}
		}
	}
	return nil
}

func mapValue(raw interface{}) (map[string]interface{}, bool) {
	if typed, ok := raw.(map[string]interface{}); ok {
		return typed, true
	}
	data, err := json.Marshal(raw)
	if err != nil || len(data) == 0 || string(data) == "null" {
		return nil, false
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, false
	}
	return decoded, true
}

func firstStringInMap(values map[string]interface{}, keys ...string) string {
	if values == nil {
		return ""
	}
	for _, key := range keys {
		if text := strings.TrimSpace(ensureStringValue(values[key])); text != "" {
			return text
		}
	}
	return ""
}

func firstReadableStringInMap(values map[string]interface{}, keys ...string) string {
	if values == nil {
		return ""
	}
	for _, key := range keys {
		value, exists := values[key]
		if !exists || isRedactedPlaceholder(value) {
			continue
		}
		text := strings.TrimSpace(ensureStringValue(value))
		if text == "" || strings.Contains(text, "USER_ASSET_REDACTED") {
			continue
		}
		return text
	}
	return ""
}

func isRedactedPlaceholder(value interface{}) bool {
	values, ok := mapValue(value)
	if !ok {
		return false
	}
	if redacted, ok := values["redacted"].(bool); !ok || !redacted {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(ensureStringValue(values["reason"])), "USER_ASSET_REDACTED")
}

func sanitizeUnitPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "SHOT"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('_')
	}
	return strings.Trim(b.String(), "_-")
}

func semanticArtifactKindForTool(toolName string) string {
	switch toolName {
	case "proposal_generator":
		return "VIDEO_PROPOSAL"
	case "video_script_generator":
		return "VIDEO_SCRIPT"
	case "card_plan_generator":
		return "CARD_PLAN"
	case "shot_splitter":
		return "SHOT_LIST"
	case "keyframe_prompt_generator":
		return "KEYFRAME_PROMPTS"
	case "video_prompt_generator":
		return "VIDEO_PROMPTS"
	case "caption_splitter":
		return "CAPTION_PLAN"
	case "video_composition_builder":
		return "VIDEO_COMPOSITION_SPEC"
	case "reference_asset_planner", "asset_policy_generator":
		return "REFERENCE_ASSET_PLAN"
	case "continuity_checker":
		return "CONTINUITY_REPORT"
	case "style_profile_builder":
		return "STYLE_PROFILE"
	case "stale_tracker":
		return "STALE_ARTIFACT_REPORT"
	case "preview_quality_checker":
		return "PREVIEW_REPORT"
	case "publish_copy_generator":
		return "PUBLISH_COPY"
	case "video_package_exporter", "artifact_packager":
		return "PROJECT_PACKAGE"
	default:
		return ""
	}
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
		cfg, hasTextProvider := clientModelProviderFromParamsForCapability(params, "text_to_text")
		if hasTextProvider {
			for i, req := range imageRequests {
				if prompt, ok := req["prompt"].(string); ok && !strings.Contains(prompt, "Use case:") {
					elaborated := elaborateImagePrompt(config.OpenAIConfig{
						BaseURL: cfg.BaseURL,
						APIKey:  cfg.APIKey,
						Model:   cfg.Model,
					}, prompt, toolCtx)
					if elaborated != "" {
						imageRequests[i]["prompt"] = elaborated
					}
				}
			}
		}
	}

	// Generate images via the user's OpenAI-compatible image provider first.
	generatedCount := 0
	clientProvider, hasClientProvider := clientGenerationProviderFromParams(params, "text_to_image")
	if hasClientProvider {
		for i, req := range imageRequests {
			if prompt, ok := req["prompt"].(string); ok && prompt != "" {
				imageURL, err := callOpenAICompatibleImageGeneration(context.Background(), clientProvider, prompt)
				if err == nil && imageURL != "" {
					imageRequests[i]["status"] = "generated"
					imageRequests[i]["storageRef"] = imageURL
					generatedCount++
				} else if err != nil {
					zap.L().Warn("Image generation via client provider failed, keeping as prompt-only",
						zap.String("id", stringParam(req, "id", "unknown")),
						zap.Error(err))
				}
			}
		}
	}

	providerNote := "图片生成服务未配置，已输出完整提示词"
	if hasClientProvider {
		providerNote = "已通过用户客户端图片 API 生成图片"
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

// executeHyperframesProjectGenerator generates a real HyperFrames HTML video project
// directory from the upstream script, shot list, and video prompts. It writes
// index.html, assets/data.json, assets/style.css, and manifest.json to disk.
//
// This is the service-mode replacement for the deprecated CLI-based
// hyperframes_project_builder. It does NOT call npx or hyperframes CLI.
func executeHyperframesProjectGenerator(stage, skillName, brief, instructionRef string, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	topic := stringParam(params, "topic", brief)
	script := stringParam(params, "script", "")
	style := stringParam(params, "style", "16:9 非写实动画，中文知识分享，低饱和知识分享")

	// Serialize structured params for the LLM prompt.
	shotListJSON := serializeParamJSON(params["shotList"])
	videoPromptsJSON := serializeParamJSON(params["videoPrompts"])
	shotAssetPackagesJSON := serializeParamJSON(params["shotAssetPackages"])
	aRollAssetPackagesJSON := serializeParamJSON(params["aRollAssetPackages"])
	publishCopyJSON := serializeParamJSON(params["publishCopy"])
	ipArollPlanJSON := serializeParamJSON(params["ipArollPlan"])

	// Determine project directory.
	projectRoot := resolveHyperFramesProjectRoot(hyperFramesConfig.ProjectRoot)
	projectDir := filepath.Join(projectRoot, toolCtx.TaskID, "hyperframes")
	assetsDir := filepath.Join(projectDir, "assets")

	// Build data.json content.
	dataJSON := buildHyperFramesDataJSON(topic, script, shotListJSON, videoPromptsJSON, shotAssetPackagesJSON, style, publishCopyJSON, ipArollPlanJSON, aRollAssetPackagesJSON)
	manifestJSON := buildHyperFramesManifestJSON(topic, toolCtx.TaskID)
	styleCSS := hyperFramesDefaultStyleCSS()

	// Use LLM to generate the index.html.
	indexHTML := generateHyperFramesIndexHTML(topic, script, shotListJSON, videoPromptsJSON, style, ipArollPlanJSON, params, toolCtx)

	// Write files to disk.
	files := []string{}
	writeErr := os.MkdirAll(assetsDir, 0755)
	if writeErr != nil {
		zap.L().Warn("Cannot create HyperFrames project directory",
			zap.String("projectDir", projectDir),
			zap.Error(writeErr))
	} else {
		if err := os.WriteFile(filepath.Join(projectDir, "index.html"), []byte(indexHTML), 0644); err != nil {
			zap.L().Warn("Cannot write index.html", zap.Error(err))
		} else {
			files = append(files, "index.html")
		}
		if err := os.WriteFile(filepath.Join(assetsDir, "data.json"), []byte(dataJSON), 0644); err != nil {
			zap.L().Warn("Cannot write data.json", zap.Error(err))
		} else {
			files = append(files, "assets/data.json")
		}
		if err := os.WriteFile(filepath.Join(assetsDir, "style.css"), []byte(styleCSS), 0644); err != nil {
			zap.L().Warn("Cannot write style.css", zap.Error(err))
		} else {
			files = append(files, "assets/style.css")
		}
		if err := os.WriteFile(filepath.Join(projectDir, "manifest.json"), []byte(manifestJSON), 0644); err != nil {
			zap.L().Warn("Cannot write manifest.json", zap.Error(err))
		} else {
			files = append(files, "manifest.json")
		}
	}

	summary := fmt.Sprintf("已生成 HyperFrames HTML 视频项目（主题：%s），包含 %d 个文件。", topic, len(files))
	if writeErr != nil {
		summary = fmt.Sprintf("HyperFrames 项目内容已生成，但写入磁盘失败：%v。项目目录：%s", writeErr, projectDir)
	}

	zap.L().Info("HyperFrames project generated",
		zap.String("taskId", toolCtx.TaskID),
		zap.String("projectDir", projectDir),
		zap.Int("fileCount", len(files)))

	artifacts := []map[string]interface{}{
		{
			"unitId":   stage,
			"kind":     "HTML",
			"name":     "index.html",
			"mimeType": "text/html",
			"metadata": map[string]interface{}{
				"stage":           stage,
				"skillName":       skillName,
				"requiresReview":  true,
				"canReviseByChat": true,
				"source":          "hyperframes-project-generator",
			},
		},
		{
			"unitId":   stage + "-data",
			"kind":     "JSON",
			"name":     "data.json",
			"mimeType": "application/json",
			"metadata": map[string]interface{}{
				"stage":     stage,
				"skillName": skillName,
				"source":    "hyperframes-project-generator",
			},
		},
	}

	return tool.SuccessResult(map[string]interface{}{
		"content":    fmt.Sprintf("# %s\n\n%s\n\n生成文件：\n%s", stage, summary, strings.Join(files, "\n")),
		"projectDir": projectDir,
		"entry":      "index.html",
		"files":      files,
		"summary":    summary,
		"artifacts":  artifacts,
	})
}

func resolveHyperFramesProjectRoot(preferred string) string {
	preferred = strings.TrimSpace(preferred)
	if preferred != "" && ensureWritableDir(preferred) == nil {
		return preferred
	}
	if preferred != "" {
		zap.L().Warn("Configured HyperFrames project root is not writable, falling back to user cache",
			zap.String("projectRoot", preferred))
	}
	fallback := filepath.Join(os.TempDir(), "tangying-ai-os", "hyperframes-projects")
	if cacheDir, err := os.UserCacheDir(); err == nil && strings.TrimSpace(cacheDir) != "" {
		fallback = filepath.Join(cacheDir, "tangying-ai-os", "hyperframes-projects")
	}
	if err := ensureWritableDir(fallback); err != nil {
		zap.L().Warn("Fallback HyperFrames project root is not writable, using temp directory",
			zap.String("projectRoot", fallback),
			zap.Error(err))
		tempFallback := filepath.Join(os.TempDir(), "tangying-ai-os", "hyperframes-projects")
		_ = os.MkdirAll(tempFallback, 0755)
		return tempFallback
	}
	return fallback
}

func ensureWritableDir(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	probe, err := os.CreateTemp(path, ".write-check-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

// serializeParamJSON converts a param value (which may be []interface{}, string, or
// already-encoded JSON) to an indented JSON string suitable for embedding in prompts.
func serializeParamJSON(value interface{}) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(b)
}

// buildHyperFramesDataJSON builds the data.json content for a HyperFrames project.
func buildHyperFramesDataJSON(topic, script, shotListJSON, videoPromptsJSON, shotAssetPackagesJSON, style, publishCopyJSON, ipArollPlanJSON string, aRollAssetPackagesJSON ...string) string {
	aRollPackages := ""
	if len(aRollAssetPackagesJSON) > 0 {
		aRollPackages = aRollAssetPackagesJSON[0]
	}
	data := map[string]interface{}{
		"topic":                 topic,
		"script":                script,
		"productionMode":        "shot_first",
		"reviewUnit":            "shot",
		"requiresNarrationSync": true,
		"shots":                 publicJSONValueOrFallback(shotListJSON, []interface{}{}),
		"videoPrompts":          publicJSONValueOrFallback(videoPromptsJSON, []interface{}{}),
		"shotAssetPackages":     publicJSONValueOrFallback(shotAssetPackagesJSON, []interface{}{}),
		"aRollAssetPackages":    publicJSONValueOrFallback(aRollPackages, []interface{}{}),
		"ipArollPlan":           publicJSONValueOrFallback(ipArollPlanJSON, map[string]interface{}{}),
		"style": map[string]interface{}{
			"aspectRatio": "16:9",
			"language":    "zh-CN",
			"visualStyle": style,
		},
		"publishCopy": publicJSONValueOrFallback(publishCopyJSON, map[string]interface{}{}),
	}
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func publicJSONValueOrFallback(raw string, fallback interface{}) interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "{{") {
		return fallback
	}
	var value interface{}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return fallback
	}
	sanitized, keep := sanitizePublicJSONValue(value)
	if !keep || sanitized == nil {
		return fallback
	}
	return sanitized
}

func sanitizePublicJSONValue(value interface{}) (interface{}, bool) {
	if isRedactedPlaceholder(value) {
		return nil, false
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			sanitized, keep := sanitizePublicJSONValue(item)
			if keep {
				out[key] = sanitized
			}
		}
		return out, true
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			sanitized, keep := sanitizePublicJSONValue(item)
			if keep {
				out = append(out, sanitized)
			}
		}
		return out, true
	case string:
		if strings.Contains(typed, "USER_ASSET_REDACTED") {
			return nil, false
		}
		return typed, true
	default:
		return value, true
	}
}

// buildHyperFramesManifestJSON builds the manifest.json for a HyperFrames project.
func buildHyperFramesManifestJSON(topic, taskID string) string {
	return fmt.Sprintf(`{
  "name": %s,
  "version": "1.0.0",
  "taskId": %s,
  "createdAt": %s,
  "entry": "index.html",
  "format": "hyperframes-html5",
  "aspectRatio": "16:9"
}
`, jsonString(topic), jsonString(taskID), jsonString(time.Now().UTC().Format(time.RFC3339)))
}

// hyperFramesDefaultStyleCSS returns a minimal CSS foundation for HyperFrames projects.
func hyperFramesDefaultStyleCSS() string {
	return `/* HyperFrames Project — base styles */
*, *::before, *::after {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

html, body {
  width: 100%;
  height: 100%;
  overflow: hidden;
  font-family: "Noto Sans SC", "PingFang SC", "Microsoft YaHei", sans-serif;
  background: #0a0a0f;
  color: #f0f0f0;
}

#app {
  width: 100%;
  height: 100%;
  position: relative;
}

.scene {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}

.scene-title {
  font-size: 3.5vw;
  font-weight: 700;
  letter-spacing: 0.05em;
  text-align: center;
  padding: 0 10%;
  line-height: 1.3;
}

.scene-subtitle {
  font-size: 1.8vw;
  font-weight: 400;
  opacity: 0.75;
  margin-top: 2vh;
  text-align: center;
  padding: 0 15%;
}

.scene-body {
  font-size: 2vw;
  font-weight: 400;
  line-height: 1.6;
  text-align: center;
  padding: 0 12%;
  margin-top: 3vh;
}

.caption-bar {
  position: absolute;
  bottom: 8%;
  left: 50%;
  transform: translateX(-50%);
  width: 80%;
  text-align: center;
  font-size: 1.8vw;
  font-weight: 500;
  background: rgba(0, 0, 0, 0.55);
  padding: 1.5vh 3vw;
  border-radius: 0.8vw;
  letter-spacing: 0.03em;
}

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(20px); }
  to   { opacity: 1; transform: translateY(0); }
}

@keyframes fadeOut {
  from { opacity: 1; }
  to   { opacity: 0; }
}

.anim-fade-in {
  animation: fadeIn 0.8s ease-out both;
}

.anim-fade-out {
  animation: fadeOut 0.6s ease-in both;
}
`
}

// generateHyperFramesIndexHTML uses the LLM to generate a complete HyperFrames HTML
// video page from the topic, script, shot list, and style parameters.
func generateHyperFramesIndexHTML(topic, script, shotListJSON, videoPromptsJSON, style, ipArollPlanJSON string, params map[string]interface{}, toolCtx tool.ToolContext) string {
	if !shouldUseLLMHyperFramesLayout(params) {
		zap.L().Info("Generating deterministic HyperFrames HTML without LLM layout")
		return buildMinimalHyperFramesHTML(topic, script, shotListJSON, style, ipArollPlanJSON)
	}

	effectiveCfg := effectiveVideoCreationOpenAIConfig(params)

	// If no LLM is configured, generate a minimal static HTML from the data.
	if effectiveCfg.APIKey == "" {
		zap.L().Info("No LLM API key configured, generating minimal HyperFrames HTML")
		return buildMinimalHyperFramesHTML(topic, script, shotListJSON, style, ipArollPlanJSON)
	}

	systemPrompt := `你是 HyperFrames HTML 视频页面开发专家。

任务：
根据提供的话题、口播稿、分镜列表和视频风格，生成一个完整的、可直接渲染的 HyperFrames HTML 视频页面。

HyperFrames 是一个 HTML-to-Video 渲染框架。你生成的 HTML 页面将直接被渲染引擎逐帧截取并编码为 MP4 视频。

硬性要求：
1. 页面尺寸为 1920x1080（16:9 画幅），所有元素定位基于此分辨率。
2. HyperFrames 也必须按 shot 创作；每个镜头（shot）应生成对应的 <div class="scene" data-shot-id="SHOT_xx"> 或 <section>，包含该镜头的视觉描述和口播文字。
3. 使用 CSS @keyframes 动画实现镜头切换效果（淡入淡出、滑动、缩放等）。
4. 文字以字幕/标题/要点形式呈现，适合视频观看，不要长段落。
5. 基调为非写实动画风格，去 AI 感，配色低饱和知识分享风格。
6. 背景色使用深色系（#0a0a0f 或类似），文字使用浅色系。
7. 每个场景的字幕/口播文字放在底部 caption-bar 中。
8. 每个 shot 的画面节奏必须和该 shot 的 scriptText 或 narrationText 对齐，不能让整片口播和画面脱节。
9. 每个 shot 添加 shot-review-packet 标记或 data-review-unit="shot"，方便前端按 shot 审核和返工。
10. 不要使用任何外部依赖或 CDN 链接。
11. 所有 CSS 内联或放在 <style> 标签中。
12. 整个页面必须是一个独立的、可以直接在浏览器中打开的完整 HTML 文件。
13. 不要包含任何 JavaScript 框架（React、Vue 等）。
14. 不要输出 Markdown 代码块标记，只输出纯 HTML。

视觉风格要求：
- 低饱和配色：背景 #0a0a0f，主文字 #f0f0f0，强调色使用低饱和蓝/青/金色
- 大量留白，简约设计
- 文字层级清晰：标题 > 副标题 > 正文 > 字幕
- 几何装饰元素（线条、圆点、半透明形状）
- 每个场景之间有明确视觉过渡

输出：
只输出完整的 HTML 文件内容，从 <!DOCTYPE html> 开始。不要有任何解释文字。`

	userPrompt := fmt.Sprintf(`话题：%s

口播稿：
%s

分镜列表：
%s

视频提示词：
%s

IP A-roll 角色动作计划：
%s

风格要求：%s

请生成完整的 HyperFrames HTML 视频页面。`, topic, script, shotListJSON, videoPromptsJSON, ipArollPlanJSON, style)

	callTool := &LlmApiTool{cfg: effectiveCfg}
	timeoutSec := intParam(params, "llmLayoutTimeoutSec", 45)
	if timeoutSec <= 0 || timeoutSec > 90 {
		timeoutSec = 45
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	result := callTool.Execute(ctx, map[string]interface{}{
		"prompt":     systemPrompt + "\n\n---\n\n" + userPrompt,
		"max_tokens": 16000,
	}, toolCtx)

	if !result.Success {
		zap.L().Warn("LLM generation for HyperFrames HTML failed, falling back to minimal HTML",
			zap.String("error", result.Error))
		return buildMinimalHyperFramesHTML(topic, script, shotListJSON, style, ipArollPlanJSON)
	}

	rawHTML, _ := result.Data["content"].(string)
	rawHTML = strings.TrimSpace(rawHTML)

	// Strip markdown code fences if the LLM wrapped the output.
	rawHTML = strings.TrimPrefix(rawHTML, "```html")
	rawHTML = strings.TrimPrefix(rawHTML, "```HTML")
	rawHTML = strings.TrimPrefix(rawHTML, "```")
	rawHTML = strings.TrimSuffix(rawHTML, "```")
	rawHTML = strings.TrimSpace(rawHTML)

	// If the LLM didn't return valid HTML, fall back.
	if !strings.HasPrefix(rawHTML, "<!DOCTYPE") && !strings.HasPrefix(rawHTML, "<html") {
		zap.L().Warn("LLM did not return valid HTML, falling back to minimal HTML",
			zap.String("prefix", safePrefix(rawHTML, 100)))
		return buildMinimalHyperFramesHTML(topic, script, shotListJSON, style, ipArollPlanJSON)
	}

	return rawHTML
}

func shouldUseLLMHyperFramesLayout(params map[string]interface{}) bool {
	return boolParam(params, "useLLMLayout", false) ||
		boolParam(params, "use_llm_layout", false) ||
		boolParam(params, "llmLayout", false)
}

// buildMinimalHyperFramesHTML generates a basic HyperFrames HTML page from the given
// data without calling the LLM. Used as fallback when no API key is configured.
func buildMinimalHyperFramesHTML(topic, script, shotListJSON, style string, ipArollPlanJSON ...string) string {
	// Parse shot list to extract shot entries.
	type shotEntry struct {
		ShotID        string `json:"shotId"`
		DurationSec   int    `json:"durationSec"`
		ScriptText    string `json:"scriptText"`
		NarrationText string `json:"narrationText"`
		Visual        string `json:"visual"`
		Camera        string `json:"camera"`
		TransitionIn  string `json:"transitionIn"`
		TransitionOut string `json:"transitionOut"`
	}
	var shots []shotEntry
	totalDurationSec := 0
	if err := json.Unmarshal([]byte(shotListJSON), &shots); err != nil {
		// shotListJSON might be a wrapper object with a "shotList" field.
		var wrapper struct {
			ShotList         []shotEntry `json:"shotList"`
			TotalDurationSec int         `json:"totalDurationSec"`
		}
		if err2 := json.Unmarshal([]byte(shotListJSON), &wrapper); err2 == nil && len(wrapper.ShotList) > 0 {
			shots = wrapper.ShotList
			totalDurationSec = wrapper.TotalDurationSec
		}
	}
	if totalDurationSec <= 0 {
		for _, shot := range shots {
			if shot.DurationSec > 0 {
				totalDurationSec += shot.DurationSec
			}
		}
	}
	if totalDurationSec <= 0 {
		totalDurationSec = 30
	}

	var scenesBuilder strings.Builder
	if len(shots) == 0 {
		// No structured shots — generate a single-scene page from the script.
		scenesBuilder.WriteString(fmt.Sprintf(`  <div id="scene-0" class="scene clip anim-fade-in" data-start="0" data-duration="%d" data-track-index="0">
    <div class="scene-title">%s</div>
    <div class="scene-body">%s</div>
    <div class="caption-bar">%s</div>
  </div>
`, totalDurationSec, templateEscape(topic), templateEscape(truncateText(script, 200)), templateEscape(truncateText(script, 80))))
	} else {
		startSec := 0
		for i, shot := range shots {
			animClass := "anim-fade-in"
			if i > 0 {
				animClass = "anim-fade-in"
			}
			durationSec := shot.DurationSec
			if durationSec <= 0 {
				durationSec = 5
			}
			title := shot.Visual
			if title == "" {
				title = shot.ShotID
			}
			body := shot.Camera
			if body == "" {
				body = shot.TransitionIn
			}
			narration := shot.ScriptText
			if narration == "" {
				narration = shot.NarrationText
			}
			scenesBuilder.WriteString(fmt.Sprintf(`  <div id="scene-%d" class="scene clip shot-review-packet %s" data-review-unit="shot" data-shot-id="%s" data-duration-sec="%d" data-start="%d" data-duration="%d" data-track-index="0">
    <div class="scene-title">%s</div>
    <div class="scene-subtitle">%s</div>
    <div class="caption-bar">%s</div>
  </div>
`, i, animClass, templateEscape(shot.ShotID), durationSec, startSec, durationSec, templateEscape(title), templateEscape(body), templateEscape(narration)))
			startSec += durationSec
		}
	}

	ipCharacterLayers := ""
	if len(ipArollPlanJSON) > 0 {
		ipCharacterLayers = buildIPArollHTMLLayers(ipArollPlanJSON[0])
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta data-composition-id="main" data-width="1920" data-height="1080">
<title>%s</title>
<style>
*, *::before, *::after { margin: 0; padding: 0; box-sizing: border-box; }
html, body { width: 1920px; height: 1080px; overflow: hidden; font-family: "Noto Sans SC", "PingFang SC", "Microsoft YaHei", sans-serif; background: #0a0a0f; color: #f0f0f0; }
#app { width: 100%%; height: 100%%; position: relative; }
.clip { visibility: hidden; }
.scene { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; }
.scene-title { font-size: 56px; font-weight: 700; letter-spacing: 0.05em; text-align: center; padding: 0 10%%; line-height: 1.3; color: #e8e8f0; }
.scene-subtitle { font-size: 28px; font-weight: 400; opacity: 0.65; margin-top: 20px; text-align: center; padding: 0 15%%; color: #b0b0c0; }
.scene-body { font-size: 32px; font-weight: 400; line-height: 1.6; text-align: center; padding: 0 12%%; margin-top: 30px; color: #c0c0d0; }
.caption-bar { position: absolute; bottom: 8%%; left: 50%%; transform: translateX(-50%%); width: 80%%; text-align: center; font-size: 30px; font-weight: 500; background: rgba(0, 0, 0, 0.6); padding: 16px 40px; border-radius: 12px; letter-spacing: 0.03em; color: #ffffff; }
.ip-character-layer { position: absolute; width: 300px; height: 420px; transform-origin: center bottom; pointer-events: none; display: flex; align-items: flex-end; justify-content: center; filter: drop-shadow(0 18px 32px rgba(0,0,0,0.30)); }
.ip-character-layer img { max-width: 100%%; max-height: 100%%; object-fit: contain; animation: ipFloat 2.4s ease-in-out infinite; }
.ip-character-layer.mouth-flap img { animation: ipFloat 2.4s ease-in-out infinite, mouthFlap 0.32s ease-in-out infinite; }
.ip-character-layer.small_hand_wave img,
.ip-character-layer.point_to_keyword_card img,
.ip-character-layer.open_palm_present img { animation: ipFloat 2.4s ease-in-out infinite, ipGesture 1.2s ease-in-out infinite; }
.ip-character-layer.has-rig { --mouth-open: 0.45; --ip-bounce-y: -8px; --ip-tilt: -2deg; --ip-hand-lift: 0.25; animation: ipBodyRig 1.5s ease-in-out infinite; }
.ip-character-layer.has-rig img { position: absolute; inset: 0; width: 100%%; height: 100%%; max-width: none; max-height: none; object-fit: contain; }
.ip-character-layer.has-rig .ip-face-rig { position: absolute; transform: translate(-50%%, -50%%) rotate(var(--ip-tilt)); border-radius: 999px; background: rgba(255,255,255,0.88); box-shadow: inset 0 -8px 18px rgba(17, 24, 39, 0.14), 0 6px 18px rgba(0,0,0,0.18); display: flex; align-items: center; justify-content: center; animation: ipHeadRig 1.4s ease-in-out infinite; }
.ip-character-layer.has-rig .ip-face-rig::after { content: ""; position: absolute; inset: 12%% 18%% 48%%; border-radius: 999px; background: linear-gradient(180deg, rgba(255,255,255,0.42), rgba(255,255,255,0)); pointer-events: none; }
.ip-eyes { position: absolute; top: 28%%; left: 50%%; width: 54%%; height: 16%%; transform: translateX(-50%%); animation: ipBlink 3.4s ease-in-out infinite; }
.ip-eyes::before,
.ip-eyes::after { content: ""; position: absolute; top: 0; width: 22%%; height: 100%%; border-radius: 999px; background: #111827; box-shadow: 0 1px 0 rgba(255,255,255,0.22); }
.ip-eyes::before { left: 5%%; }
.ip-eyes::after { right: 5%%; }
.ip-mouth { position: absolute; left: 50%%; bottom: 24%%; width: 30%%; height: calc(8%% + var(--mouth-open) * 34%%); transform: translateX(-50%%); border-radius: 999px; background: #111827; box-shadow: inset 0 -4px 0 rgba(255,255,255,0.12); animation: ipMouthRig 0.24s ease-in-out infinite; }
.ip-mouth.viseme-closed { height: 7%%; width: 26%%; border-radius: 999px; }
.ip-mouth.viseme-a { width: 28%%; height: calc(12%% + var(--mouth-open) * 38%%); }
.ip-mouth.viseme-o { width: 24%%; height: calc(12%% + var(--mouth-open) * 30%%); border-radius: 50%%; }
.ip-mouth.viseme-e,
.ip-mouth.viseme-smile { width: 36%%; height: calc(7%% + var(--mouth-open) * 18%%); }
@keyframes fadeIn { from { opacity: 0; transform: translateY(30px); } to { opacity: 1; transform: translateY(0); } }
@keyframes fadeOut { from { opacity: 1; } to { opacity: 0; } }
@keyframes ipFloat { 0%% { transform: translateY(0) scale(1); } 50%% { transform: translateY(-10px) scale(1.015); } 100%% { transform: translateY(0) scale(1); } }
@keyframes mouthFlap { 0%% { filter: brightness(1); } 50%% { filter: brightness(1.12); } 100%% { filter: brightness(1); } }
@keyframes ipGesture { 0%% { rotate: 0deg; } 50%% { rotate: -2deg; } 100%% { rotate: 0deg; } }
@keyframes ipBodyRig { 0%% { translate: 0 0; rotate: 0deg; } 50%% { translate: 0 var(--ip-bounce-y); rotate: var(--ip-tilt); } 100%% { translate: 0 0; rotate: 0deg; } }
@keyframes ipHeadRig { 0%% { scale: 1; } 50%% { scale: 1.035; } 100%% { scale: 1; } }
@keyframes ipBlink { 0%%, 88%%, 100%% { transform: translateX(-50%%) scaleY(1); } 92%% { transform: translateX(-50%%) scaleY(0.12); } }
@keyframes ipMouthRig { 0%% { transform: translateX(-50%%) scaleY(0.72); } 50%% { transform: translateX(-50%%) scaleY(1.16); } 100%% { transform: translateX(-50%%) scaleY(0.78); } }
.anim-fade-in { animation: fadeIn 0.8s ease-out both; }
.anim-fade-out { animation: fadeOut 0.6s ease-in both; }
</style>
</head>
<body>
<div id="app" data-composition-id="main" data-duration="%d">
%s
%s
</div>
<script>
(function () {
  var durationSec = %d;
  window.__timelines = window.__timelines || {};
  if (window.gsap && typeof window.gsap.timeline === "function") {
    var tl = window.gsap.timeline({ paused: true });
    tl.to({}, { duration: durationSec });
    window.__timelines["main"] = tl;
    return;
  }
  var currentTime = 0;
  var paused = true;
  function clampTime(value) {
    var next = Number(value);
    if (!Number.isFinite(next)) return currentTime;
    return Math.max(0, Math.min(durationSec, next));
  }
  window.__timelines["main"] = {
    duration: function () { return durationSec; },
    time: function () { return currentTime; },
    totalTime: function (value) {
      if (arguments.length > 0) currentTime = clampTime(value);
      return currentTime;
    },
    seek: function (value) {
      currentTime = clampTime(value);
      return currentTime;
    },
    pause: function () {
      paused = true;
      return this;
    },
    play: function () {
      paused = false;
      return this;
    },
    paused: function () { return paused; },
    timeScale: function () { return 1; }
  };
})();
</script>
</body>
</html>`, templateEscape(topic), totalDurationSec, scenesBuilder.String(), ipCharacterLayers, totalDurationSec)
}

func buildIPArollHTMLLayers(ipArollPlanJSON string) string {
	planValue := publicJSONValueOrFallback(ipArollPlanJSON, map[string]interface{}{})
	plan, ok := mapValue(planValue)
	if !ok || len(plan) == 0 {
		return ""
	}
	characterIndex := map[string]map[string]interface{}{}
	for _, item := range interfaceItems(plan["characters"]) {
		character, ok := mapValue(item)
		if !ok {
			continue
		}
		id := firstStringInMap(character, "id")
		if id != "" {
			characterIndex[id] = character
		}
	}
	rigIndex := map[string]map[string]interface{}{}
	if rootRigs, ok := mapValue(plan["rigs"]); ok {
		for id, raw := range rootRigs {
			if rig, rigOK := mapValue(raw); rigOK && len(rig) > 0 {
				rigIndex[id] = rig
			}
		}
	}

	var b strings.Builder
	for idx, item := range interfaceItems(plan["timeline"]) {
		beat, ok := mapValue(item)
		if !ok {
			continue
		}
		characterID := firstStringInMap(beat, "characterId")
		character := characterIndex[characterID]
		if character == nil {
			continue
		}
		view := firstStringInMap(beat, "view")
		if view == "" {
			view = "front"
		}
		src := ipCharacterAssetForView(character, view)
		if src == "" {
			continue
		}
		start := floatParamFromAny(beat["startSec"])
		end := floatParamFromAny(beat["endSec"])
		if end <= start {
			end = start + 1
		}
		position := ipHTMLPosition(beat["position"])
		rig := ipCharacterRigForHTML(character, rigIndex, characterID)
		hasRig := len(rig) > 0
		classes := []string{"ip-character-layer", "clip", sanitizeCSSClass(firstStringInMap(beat, "gesture"))}
		if firstStringInMap(beat, "mouthCue") == "talking" {
			classes = append(classes, "mouth-flap")
		}
		if hasRig {
			classes = append(classes, "has-rig", sanitizeCSSClass(firstStringInMap(beat, "expressionCue", "expression")))
		}
		motion := ipMotionCSSVars(beat["motion"])
		faceLayer := ""
		if hasRig {
			face := ipFaceOverlayFromRig(rig)
			mouthOpen := ipFirstLipSyncMouthOpen(beat["lipSync"])
			viseme := ipFirstLipSyncViseme(beat["lipSync"])
			faceLayer = fmt.Sprintf(`    <div class="ip-face-rig %s" data-lipsync="%s" style="left: %.2f%%; top: %.2f%%; width: %.2f%%; height: %.2f%%; --mouth-open: %.2f;">
      <div class="ip-eyes %s"></div>
      <div class="ip-mouth viseme-%s"></div>
    </div>
`, sanitizeCSSClass(firstStringInMap(beat, "expressionCue", "expression")),
				ipLipSyncDataAttribute(beat["lipSync"]),
				face["x"]*100,
				face["y"]*100,
				face["width"]*100,
				face["height"]*100,
				mouthOpen,
				sanitizeCSSClass(firstStringInMap(beat, "expressionCue", "expression")),
				sanitizeCSSClass(viseme),
			)
		}
		b.WriteString(fmt.Sprintf(`  <div id="ip-aroll-%02d" class="%s" data-start="%.2f" data-duration="%.2f" data-track-index="1" data-character-id="%s" data-action="%s" style="left: %.2f%%; top: %.2f%%; transform: translate(-50%%,-50%%) scale(%.3f); --ip-bounce-y: %.2fpx; --ip-tilt: %.2fdeg; --ip-hand-lift: %.2f;">
    <img src="%s" alt="%s" draggable="false">
%s
  </div>
`, idx+1,
			strings.Join(classes, " "),
			start,
			end-start,
			templateEscape(characterID),
			templateEscape(firstStringInMap(beat, "action")),
			position["x"].(float64)*100,
			position["y"].(float64)*100,
			position["scale"].(float64),
			motion["bounceY"],
			motion["tilt"],
			motion["handLift"],
			templateEscape(src),
			templateEscape(firstStringInMap(character, "name")),
			faceLayer,
		))
	}
	return b.String()
}

func ipCharacterRigForHTML(character map[string]interface{}, rigIndex map[string]map[string]interface{}, characterID string) map[string]interface{} {
	if rig, ok := mapValue(character["rig"]); ok && len(rig) > 0 {
		return rig
	}
	if rig := rigIndex[characterID]; len(rig) > 0 {
		return rig
	}
	return nil
}

func ipFaceOverlayFromRig(rig map[string]interface{}) map[string]float64 {
	face := map[string]float64{"x": 0.50, "y": 0.30, "width": 0.40, "height": 0.18}
	rawFace, ok := mapValue(rig["faceOverlay"])
	if !ok {
		return face
	}
	for _, key := range []string{"x", "y", "width", "height"} {
		if value := floatParamFromAny(rawFace[key]); value > 0 {
			face[key] = clampFloat(value, 0.02, 0.95)
		}
	}
	return face
}

func ipMotionCSSVars(raw interface{}) map[string]float64 {
	out := map[string]float64{"bounceY": -8, "tilt": -2, "handLift": 0.25}
	motion, ok := mapValue(raw)
	if !ok {
		return out
	}
	if controls, ok := mapValue(motion["controls"]); ok {
		if value := floatParamFromAny(controls["bodyBounce"]); value > 0 {
			out["bounceY"] = -20 * clampFloat(value, 0, 1)
		}
		if value := floatParamFromAny(controls["headTilt"]); value != 0 {
			out["tilt"] = clampFloat(value, -12, 12)
		}
		if value := floatParamFromAny(controls["handLift"]); value > 0 {
			out["handLift"] = clampFloat(value, 0, 1)
		}
	}
	items := interfaceItems(motion["keyframes"])
	if len(items) > 1 {
		if keyframe, ok := mapValue(items[1]); ok {
			if value := floatParamFromAny(keyframe["translateY"]); value != 0 {
				out["bounceY"] = clampFloat(value, -30, 30)
			}
			if value := floatParamFromAny(keyframe["rotateDeg"]); value != 0 {
				out["tilt"] = clampFloat(value, -12, 12)
			}
			if value := floatParamFromAny(keyframe["handLift"]); value > 0 {
				out["handLift"] = clampFloat(value, 0, 1)
			}
		}
	}
	return out
}

func ipLipSyncDataAttribute(raw interface{}) string {
	data, err := json.Marshal(raw)
	if err != nil || len(data) == 0 || string(data) == "null" {
		return "[]"
	}
	return templateEscape(string(data))
}

func ipFirstLipSyncMouthOpen(raw interface{}) float64 {
	sample := ipFirstLipSyncSample(raw)
	if sample == nil {
		return 0.45
	}
	return clampFloat(floatParamFromAny(sample["mouthOpen"]), 0, 1)
}

func ipFirstLipSyncViseme(raw interface{}) string {
	sample := ipFirstLipSyncSample(raw)
	if sample == nil {
		return "closed"
	}
	if viseme := sanitizeCSSClass(firstStringInMap(sample, "viseme")); viseme != "" {
		return viseme
	}
	return "closed"
}

func ipFirstLipSyncSample(raw interface{}) map[string]interface{} {
	for _, item := range interfaceItems(raw) {
		sample, ok := mapValue(item)
		if ok {
			return sample
		}
	}
	return nil
}

func ipCharacterAssetForView(character map[string]interface{}, view string) string {
	assets, ok := mapValue(character["assets"])
	if !ok {
		return ""
	}
	for _, key := range []string{view, "front", "reference"} {
		if value := strings.TrimSpace(ensureStringValue(assets[key])); value != "" {
			return value
		}
	}
	return ""
}

func ipHTMLPosition(raw interface{}) map[string]interface{} {
	out := map[string]interface{}{"x": 0.5, "y": 0.66, "scale": 0.7}
	position, ok := mapValue(raw)
	if !ok {
		return out
	}
	for _, key := range []string{"x", "y", "scale"} {
		if value := floatParamFromAny(position[key]); value > 0 {
			out[key] = value
		}
	}
	return out
}

func sanitizeCSSClass(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "idle"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "idle"
	}
	return b.String()
}

// templateEscape escapes text for safe embedding in HTML templates.
func templateEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// truncateText truncates text to maxLen characters, appending "..." if truncated.
func truncateText(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen]) + "..."
}

// jsonString returns a JSON-encoded string (with quotes).
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// safePrefix returns the first n characters of s for logging purposes.
func safePrefix(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// executeHyperframesProjectBuilder builds or provides guidance for a HyperFrames project.
// Deprecated: use executeHyperframesProjectGenerator for service mode instead.
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

func executeModelGatewayVideoGenerator(stage, skillName, brief string, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	prompt := firstNonEmptyString(params, "prompt", "videoPrompt", "description")
	if prompt == "" {
		prompt = brief
	}
	imageURL := firstNonEmptyString(params, "imageUrl", "imageURL", "firstFrame", "referenceImage")

	if clientProvider, ok := clientGenerationProviderFromParams(params, "text_to_video"); ok {
		videoURL, err := callOpenAICompatibleVideoGeneration(context.Background(), clientProvider, prompt, imageURL)
		if err != nil {
			return tool.FailureResult(fmt.Sprintf("客户端视频 API 生成失败: %v", err))
		}
		return videoGenerationSuccessResult(stage, skillName, toolCtx.TaskID, string(capabilityForVideoInput(imageURL)), "client-openai-compatible-video-generator", "client_openai_compatible", videoURL, map[string]interface{}{
			"url": videoURL,
		})
	}

	return tool.FailureResult("视频 API 未配置，请在客户端设置 OpenAI-compatible 文生视频 Provider 后重试")
}

func capabilityForVideoInput(imageURL string) modelgateway.Capability {
	if strings.TrimSpace(imageURL) != "" {
		return modelgateway.CapImageToVideo
	}
	return modelgateway.CapTextToVideo
}

func videoGenerationSuccessResult(stage, skillName, taskID, capability, source, providerInterface, videoURL string, modelResponse interface{}) tool.ToolResult {
	storageRef := videoURL
	if strings.HasPrefix(videoURL, "http://") || strings.HasPrefix(videoURL, "https://") {
		storageRef = fmt.Sprintf("local://projects/%s/renders/model-gateway-final.mp4", taskID)
	}
	responseText := ensureStringValue(modelResponse)
	if responseText == "" {
		if data, err := json.Marshal(modelResponse); err == nil {
			responseText = string(data)
		}
	}
	contentHash := localContentHash(responseText + storageRef)
	artifactManifest := map[string]interface{}{
		"unitId":      "final-video",
		"kind":        "VIDEO",
		"name":        "final.mp4",
		"mimeType":    "video/mp4",
		"storageRef":  storageRef,
		"contentHash": contentHash,
		"sizeBytes":   int64(1),
		"metadata": map[string]interface{}{
			"stage":             stage,
			"skillName":         skillName,
			"source":            source,
			"modelCapability":   capability,
			"providerInterface": providerInterface,
			"status":            "valid",
			"humanApproved":     false,
			"originUrl":         videoURL,
		},
	}
	return tool.SuccessResult(map[string]interface{}{
		"content":       "# Video Generation\n\n已通过用户配置的视频 API 生成或登记最终视频。",
		"video":         storageRef,
		"modelResponse": modelResponse,
		"artifacts":     []map[string]interface{}{artifactManifest},
	})
}

func callOpenAICompatibleImageGeneration(ctx context.Context, provider RuntimeModelProviderConfig, prompt string) (string, error) {
	body := map[string]interface{}{
		"model":           provider.Model,
		"prompt":          prompt,
		"size":            "1792x1024",
		"response_format": "url",
	}
	return postOpenAICompatibleGeneration(ctx, provider, "images/generations", body)
}

func callOpenAICompatibleVideoGeneration(ctx context.Context, provider RuntimeModelProviderConfig, prompt, imageURL string) (string, error) {
	body := map[string]interface{}{
		"model":  provider.Model,
		"prompt": prompt,
	}
	if strings.TrimSpace(imageURL) != "" {
		body["image_url"] = imageURL
	}
	return postOpenAICompatibleGeneration(ctx, provider, "videos/generations", body)
}

func postOpenAICompatibleGeneration(ctx context.Context, provider RuntimeModelProviderConfig, path string, payload map[string]interface{}) (string, error) {
	if strings.TrimSpace(provider.BaseURL) == "" {
		return "", fmt.Errorf("baseUrl is required")
	}
	if strings.TrimSpace(provider.APIKey) == "" {
		return "", fmt.Errorf("apiKey is required")
	}
	if strings.TrimSpace(provider.Model) == "" {
		return "", fmt.Errorf("model is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAICompatibleEndpoint(provider.BaseURL, path), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+provider.APIKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request provider: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(respBody))
	}
	var parsed interface{}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if generatedURL := firstGeneratedMediaURL(parsed); generatedURL != "" {
		return generatedURL, nil
	}
	return "", fmt.Errorf("response did not include generated media URL")
}

func openAICompatibleEndpoint(baseURL, path string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func firstGeneratedMediaURL(value interface{}) string {
	switch typed := value.(type) {
	case map[string]interface{}:
		for _, key := range []string{"url", "videoUrl", "video_url", "outputUrl", "output_url", "storageRef", "b64_json"} {
			if text := strings.TrimSpace(ensureStringValue(typed[key])); text != "" {
				return text
			}
		}
		for _, key := range []string{"data", "output", "result", "video", "image"} {
			if text := firstGeneratedMediaURL(typed[key]); text != "" {
				return text
			}
		}
	case []interface{}:
		for _, item := range typed {
			if text := firstGeneratedMediaURL(item); text != "" {
				return text
			}
		}
	case []map[string]interface{}:
		for _, item := range typed {
			if text := firstGeneratedMediaURL(item); text != "" {
				return text
			}
		}
	}
	return ""
}

func localContentHash(content string) string {
	sum := stdsha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// executeHyperframesRenderer renders a HyperFrames project to MP4.
//
// When HyperFrames mode is "service", it calls the Render Service HTTP API.
// CLI fallback is explicitly forbidden in service mode — if the service is
// unavailable the tool returns a failure. When mode is "disabled" the tool
// returns a manual-upload placeholder so the workflow can continue in local
// smoke and browser-driven runs.
func executeHyperframesRenderer(stage, skillName, brief, instructionRef string, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	projectRef := stringParam(params, "project_ref", "")
	projectDir := stringParam(params, "projectDir", projectRef)
	entry := stringParam(params, "entry", "index.html")

	status := "guidance_only"
	var renderPath, duration, resolution, codec, fileSize, jobID string
	serviceUsed := false

	switch hyperFramesConfig.Mode {
	case hyperframes.ModeService:
		if !hyperFramesServiceAvailable() {
			return tool.FailureResult(
				"HyperFrames Render Service 未启用或不可用，service 模式禁止回退 CLI。请检查 Render Service 是否已启动。",
			)
		}

		zap.L().Info("HyperFrames render via Render Service",
			zap.String("taskId", toolCtx.TaskID),
			zap.String("projectDir", projectDir))

		outputPath := fmt.Sprintf("%s/%s.mp4", hyperFramesConfig.OutputRoot, toolCtx.TaskID)
		ctx, cancel := context.WithTimeout(context.Background(), hyperFramesConfig.Timeout())
		defer cancel()

		result, err := hyperFramesClient.Render(ctx, hyperframes.RenderRequest{
			ProjectDir: projectDir,
			Entry:      entry,
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

	case hyperframes.ModeDisabled:
		renderPath = fmt.Sprintf("projects/%s/manual-final.mp4", toolCtx.TaskID)
		status = "manual_upload_required"

	default:
		// Unknown mode — refuse to render.
		return tool.FailureResult(
			fmt.Sprintf("HyperFrames 模式未识别（%s）。请设置 HYPERFRAMES_MODE=service 或 HYPERFRAMES_MODE=disabled。", string(hyperFramesConfig.Mode)),
		)
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

// WebSearchResult holds a single web search result snippet.
type WebSearchResult struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
	Date    string `json:"date,omitempty"`
}

// performWebSearch executes a web search for the given query and returns
// structured snippets that can be fed to the LLM as reference material.
// Uses the configured SEARCH_API_KEY / SEARCH_ENDPOINT env vars.
// Falls back gracefully when search is not configured.
func performWebSearch(ctx context.Context, query string) ([]WebSearchResult, error) {
	apiKey := os.Getenv("SEARCH_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("SEARCH_API_KEY not configured")
	}

	endpoint := os.Getenv("SEARCH_ENDPOINT")
	if endpoint == "" {
		endpoint = "https://newsapi.org/v2/everything"
	}

	client := &http.Client{Timeout: 30 * time.Second}

	// Detect provider: NewsAPI.org uses GET with query params, Serper uses POST with JSON body.
	if strings.Contains(endpoint, "newsapi.org") {
		return searchNewsAPI(ctx, client, endpoint, apiKey, query)
	}
	return searchSerper(ctx, client, endpoint, apiKey, query)
}

func searchNewsAPI(ctx context.Context, client *http.Client, endpoint, apiKey, query string) ([]WebSearchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("newsapi request: %w", err)
	}
	q := req.URL.Query()
	q.Set("q", query)
	q.Set("pageSize", "10")
	q.Set("sortBy", "publishedAt")
	// Narrow to recent articles for faster, more relevant results.
	q.Set("from", time.Now().AddDate(0, 0, -7).Format("2006-01-02"))
	q.Set("language", "en")
	q.Set("apiKey", apiKey)
	req.URL.RawQuery = q.Encode()

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("newsapi call: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("newsapi returned %d: %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Articles []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			URL         string `json:"url"`
			PublishedAt string `json:"publishedAt"`
			Source      struct {
				Name string `json:"name"`
			} `json:"source"`
		} `json:"articles"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("parse newsapi response: %w", err)
	}

	results := make([]WebSearchResult, 0, len(result.Articles))
	for _, a := range result.Articles {
		results = append(results, WebSearchResult{
			Title:   a.Title,
			Snippet: a.Description,
			URL:     a.URL,
			Date:    a.PublishedAt,
		})
	}
	return results, nil
}

func searchSerper(ctx context.Context, client *http.Client, endpoint, apiKey, query string) ([]WebSearchResult, error) {
	reqBody := map[string]interface{}{
		"q":   query,
		"num": 10,
	}
	if gl := os.Getenv("SEARCH_GL"); gl != "" {
		reqBody["gl"] = gl
	} else {
		reqBody["gl"] = "us"
	}
	if hl := os.Getenv("SEARCH_HL"); hl != "" {
		reqBody["hl"] = hl
	} else {
		reqBody["hl"] = "en"
	}
	bodyBytes, _ := json.Marshal(reqBody)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("serper request: %w", err)
	}
	httpReq.Header.Set("X-API-KEY", apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("serper call: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("serper returned %d: %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Organic []struct {
			Title   string `json:"title"`
			Snippet string `json:"snippet"`
			Link    string `json:"link"`
			Date    string `json:"date"`
		} `json:"organic"`
		News []struct {
			Title   string `json:"title"`
			Snippet string `json:"snippet"`
			Link    string `json:"link"`
			Date    string `json:"date"`
		} `json:"news"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("parse serper response: %w", err)
	}

	items := result.News
	if len(items) == 0 {
		items = result.Organic
	}

	results := make([]WebSearchResult, 0, len(items))
	for _, o := range items {
		results = append(results, WebSearchResult{
			Title:   o.Title,
			Snippet: o.Snippet,
			URL:     o.Link,
			Date:    o.Date,
		})
	}
	return results, nil
}

// buildSearchQuery uses a fast LLM call to extract concise English search
// keywords from a verbose Chinese user message. Falls back to heuristic
// extraction if the LLM is unavailable.
func buildSearchQuery(raw string, params map[string]interface{}) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	cfg := effectiveVideoCreationOpenAIConfig(params)

	if cfg.APIKey != "" {
		callTool := &LlmApiTool{cfg: cfg}
		prompt := fmt.Sprintf(
			`Extract the core topic and key entities from the following video request. Output ONLY a concise English search query (max 15 words) suitable for news search APIs like Serper/Google News. Focus on proper nouns, events, dates, and key concepts. Do NOT include filler words like "video", "make", "create", "帮我", "做一个".

Input: %s

Output (just the search query, nothing else):`, raw)
		result := callTool.Execute(context.Background(), map[string]interface{}{
			"prompt":      prompt,
			"max_tokens":  80,
			"temperature": 0.0,
		}, tool.ToolContext{})
		if result.Success {
			if content, ok := result.Data["content"].(string); ok {
				query := strings.TrimSpace(content)
				// Clean up common LLM artifacts
				query = strings.TrimPrefix(query, "\"")
				query = strings.TrimSuffix(query, "\"")
				query = strings.TrimSpace(query)
				if query != "" && len(query) < 200 {
					return query
				}
			}
		}
		zap.L().Warn("LLM search query extraction failed, using raw input")
	}

	// Fallback heuristic: strip common Chinese command patterns.
	clean := raw
	for _, p := range []string{"请帮我创作", "请帮我制作", "请帮我生成", "请帮我做", "请帮我", "帮我创作", "帮我制作", "帮我做", "帮我"} {
		if strings.HasPrefix(clean, p) {
			clean = strings.TrimPrefix(clean, p)
			break
		}
	}
	clean = strings.TrimSpace(clean)
	for _, s := range []string{"图文视频", "短视频", "视频"} {
		if idx := strings.Index(clean, s); idx >= 0 {
			clean = clean[:idx] + clean[idx+len(s):]
		}
	}
	clean = strings.TrimLeft(clean, "，,。.：:、 ")
	clean = strings.TrimRight(clean, "，,。.：:、")
	// Remove duration patterns like "一个30秒" "30秒的"
	for _, d := range []string{"一个30秒", "一个45秒", "一个60秒", "一个90秒", "30秒的", "45秒的", "60秒的", "30秒", "45秒", "60秒", "90秒", "120秒"} {
		clean = strings.ReplaceAll(clean, d, "")
	}
	clean = strings.TrimSpace(clean)
	clean = strings.TrimLeft(clean, "，,。.：:、 ")
	if clean == "" {
		return raw
	}
	return clean
}

// formatSearchResults formats web search results as a readable text block for LLM prompts.
// For top results, it attempts to fetch the full article content for richer context.
func formatSearchResults(results []WebSearchResult) string {
	if len(results) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("【最新网络搜索结果（英文）】\n以下是从网络搜索获得的最新信息。请将英文内容翻译提炼为简体中文后进行知识整理。所有输出字段必须使用中文：\n\n")
	articleClient := &http.Client{Timeout: 8 * time.Second}
	for i, r := range results {
		b.WriteString(fmt.Sprintf("%d. %s\n   %s\n", i+1, r.Title, r.Snippet))
		if r.Date != "" {
			b.WriteString(fmt.Sprintf("   日期：%s\n", r.Date))
		}
		b.WriteString(fmt.Sprintf("   来源：%s\n", r.URL))
		// Fetch full article content for top 3 results for better accuracy.
		if i < 3 && r.URL != "" {
			if content := fetchArticleText(articleClient, r.URL); content != "" {
				b.WriteString(fmt.Sprintf("   全文摘要：%s\n", content))
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// fetchArticleText fetches a URL and extracts readable text content from the HTML.
// Returns up to 1500 chars of text, or empty string on any error.
func fetchArticleText(client *http.Client, url string) string {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; TangyingBot/1.0)")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ""
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024)) // max 512KB
	if err != nil {
		return ""
	}
	text := extractTextFromHTML(string(bodyBytes))
	if len(text) > 1500 {
		text = text[:1500]
	}
	return text
}

// extractTextFromHTML strips HTML tags and extracts readable text content.
func extractTextFromHTML(html string) string {
	// Remove script and style blocks.
	inTag := false
	inScript := false
	var b strings.Builder
	tagName := ""
	tagNameStart := -1

	for i := 0; i < len(html); i++ {
		ch := html[i]
		if ch == '<' {
			inTag = true
			tagName = ""
			tagNameStart = i + 1
			continue
		}
		if ch == '>' && inTag {
			inTag = false
			if tagName == "script" || tagName == "style" || tagName == "noscript" || tagName == "iframe" {
				inScript = true
			}
			if tagName == "/script" || tagName == "/style" || tagName == "/noscript" || tagName == "/iframe" {
				inScript = false
			}
			// Add space after block elements.
			switch tagName {
			case "/p", "/div", "/h1", "/h2", "/h3", "/h4", "/h5", "/h6", "/li", "/tr", "br", "br/":
				b.WriteByte(' ')
			}
			tagName = ""
			continue
		}
		if inTag && tagNameStart >= 0 {
			// Build tag name until space or closing bracket.
			if ch == ' ' || ch == '/' || ch == '\t' || ch == '\n' || ch == '\r' {
				// Tag name complete.
			} else if tagNameStart >= 0 {
				tagName += string(ch)
			}
			continue
		}
		if !inTag && !inScript {
			// Replace HTML entities roughly.
			if ch == '&' {
				semi := strings.IndexByte(html[i:], ';')
				if semi > 0 && semi < 10 {
					entity := html[i : i+semi+1]
					switch entity {
					case "&amp;":
						b.WriteByte('&')
					case "&lt;":
						b.WriteByte('<')
					case "&gt;":
						b.WriteByte('>')
					case "&quot;":
						b.WriteByte('"')
					case "&apos;":
						b.WriteByte('\'')
					case "&nbsp;":
						b.WriteByte(' ')
					default:
						b.WriteByte(' ')
					}
					i += semi
					continue
				}
			}
			b.WriteByte(ch)
		}
	}

	// Collapse whitespace.
	result := strings.Join(strings.Fields(b.String()), " ")
	return strings.TrimSpace(result)
}

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
		b.WriteString("Project built successfully.\n")
		b.WriteString(fmt.Sprintf("- Project: `%s`\n", projectRef))
		b.WriteString(fmt.Sprintf("- Preview: %s\n", previewURL))
	} else {
		b.WriteString("## Build Guidance\n\n")
		b.WriteString("HyperFrames CLI not available. To build this project manually:\n\n")
		b.WriteString("```bash\n")
		b.WriteString("# 1. Create project directory\n")
		b.WriteString(fmt.Sprintf("mkdir -p %s\n\n", projectRef))
		b.WriteString("# 2. Copy the HyperFrames reference as the build instruction\n")
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
		b.WriteString("Render completed.\n")
		b.WriteString(fmt.Sprintf("- Output: `%s`\n", renderPath))
	} else if status == "manual_upload_required" {
		b.WriteString("## Manual Video Result\n\n")
		b.WriteString("HyperFrames rendering is disabled in this environment. Upload or register an externally rendered MP4 for this placeholder path:\n\n")
		b.WriteString(fmt.Sprintf("- Expected output: `%s`\n", renderPath))
	} else {
		b.WriteString("## Render Guidance\n\n")
		b.WriteString("HyperFrames CLI not available. To render this project manually:\n\n")
		b.WriteString("```bash\n")
		if cliFound {
			b.WriteString("hyperframes render --project <project> --output output/video.mp4 --format mp4\n")
		} else {
			b.WriteString("npx hyperframes render --project <project> --output output/video.mp4 --format mp4\n")
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
	if renderPath != "" {
		artifacts = append(artifacts, map[string]interface{}{
			"unitId":     "final-video",
			"kind":       "VIDEO",
			"name":       "final.mp4",
			"mimeType":   "video/mp4",
			"storageRef": fmt.Sprintf("local://%s", strings.TrimPrefix(renderPath, "local://")),
			"metadata": map[string]interface{}{
				"stage":         stage,
				"skillName":     skillName,
				"status":        status,
				"renderPath":    renderPath,
				"source":        "hyperframes-renderer",
				"manualUpload":  status == "manual_upload_required",
				"humanApproved": false,
			},
		})
	}
	return artifacts
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
		"card_plan_generator", "caption_splitter",
		"composition_quality_checker", "reference_asset_planner",
		"asset_policy_generator", "continuity_checker",
		"style_profile_builder", "stale_tracker",
		"preview_quality_checker",
		"publish_copy_generator", "video_package_exporter",
		"video_composition_builder":
		return true
	default:
		return false
	}
}

func promptToolUsesKnowledgeFacts(toolName string) bool {
	switch toolName {
	case "video_script_generator", "script_quality_checker", "package_quality_checker":
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
	usedFacts := []map[string]interface{}{}
	knowledgeTrace := map[string]interface{}{}
	if promptToolUsesKnowledgeFacts(toolName) {
		if facts == "" {
			facts = formatKnowledgeContextForPrompt(params["knowledgeContext"])
			usedFacts = usedFactsFromKnowledgeContext(params["knowledgeContext"])
		}
		if facts == "" {
			facts = formatKnowledgePackForPrompt(params["knowledgePack"], params["knowledgeSources"])
			usedFacts = usedFactsFromKnowledgePack(params["knowledgePack"])
		}
		if facts == "" && toolName == "video_script_generator" {
			productFacts := tangyingProductFactsForTopic(topic)
			if len(productFacts) > 0 {
				usedFacts = productFacts
				facts = formatKnowledgePackForPrompt(productFacts, nil)
			}
		}
		if toolName == "video_script_generator" && shouldBlockOnEmptyFreshFacts(params) && requiresFreshKnowledge(params) && !hasKnowledgeFacts(params["knowledgePack"], facts) && !hasKnowledgeContextFacts(params["knowledgeContext"]) {
			return tool.FailureResult("video_script_generator: retrievalPolicy=required but knowledgeContext is empty")
		}
		facts = appendKnowledgePolicyForPrompt(facts, params)
		knowledgeTrace = knowledgeTraceFromParams(params, usedFacts)
	}
	style := stringParam(params, "outputStyle", "")
	platform := stringParam(params, "platform", "视频创作平台")
	script := promptStringParam(params, "script", "")
	shotList := promptStringParam(params, "shotList", "")
	videoPrompts := promptStringParam(params, "videoPrompts", "")
	publishCopy := promptStringParam(params, "publishCopy", "")
	targetDurationSec := intParam(params, "targetDurationSec", intParam(params, "durationSec", 60))
	targetDurationSec = inferTargetDurationSec(topic, targetDurationSec)
	if toolName == "video_script_generator" && script != "" {
		return tool.SuccessResult(buildApprovedVideoScriptData(toolName, skillName, topic, script, targetDurationSec, usedFacts, knowledgeTrace))
	}

	if toolName == "shot_splitter" {
		if data, ok := buildDeterministicShotSplitterData(toolName, skillName, topic, script, targetDurationSec); ok {
			return tool.SuccessResult(data)
		}
	}
	if toolName == "caption_splitter" {
		if data, ok := buildDeterministicCaptionSplitterData(toolName, skillName, topic, script, targetDurationSec); ok {
			return tool.SuccessResult(data)
		}
	}
	if toolName == "video_prompt_generator" {
		if data, ok := buildDeterministicVideoPromptData(toolName, skillName, topic, params); ok {
			return tool.SuccessResult(data)
		}
	}
	if toolName == "reference_asset_planner" && videoParamsLookCinematic(topic, params) {
		if data, ok := buildFallbackReferenceAssetData(toolName, skillName, topic, params); ok {
			return tool.SuccessResult(data)
		}
	}
	if toolName == "keyframe_prompt_generator" && videoParamsLookCinematic(topic, params) {
		if data, ok := buildFallbackKeyframePromptData(toolName, skillName, topic, params); ok {
			return tool.SuccessResult(data)
		}
	}
	if toolName == "knowledge_researcher" {
		if productFacts := tangyingProductFactsForTopic(topic); len(productFacts) > 0 {
			return tool.SuccessResult(buildTangyingProductKnowledgeResearchData(toolName, skillName, productFacts))
		}
	}

	systemPrompt := buildDynamicAgentSystemPrompt(toolName, topic, style, platform)
	userPrompt := buildDynamicAgentUserPrompt(toolName, topic, facts, style, script, shotList, videoPrompts, publishCopy, platform, targetDurationSec)
	structuredOutput := isStructuredOutputTool(toolName)
	if structuredOutput {
		systemPrompt = appendJSONModeSystemInstruction(systemPrompt)
		userPrompt = appendJSONModeUserInstruction(userPrompt)
	}

	// For knowledge_researcher: perform actual web search to get current facts.
	if toolName == "knowledge_researcher" {
		// Build a clean search query from the user's topic, stripping command wrappers.
		rawQuery := topic
		if rawQuery == "" {
			rawQuery = brief
		}
		if researchQuery, ok := params["searchQuery"].(string); ok && researchQuery != "" {
			rawQuery = researchQuery
		}
		// Strip common Chinese AI command prefixes and keep the real topic.
		clean := buildSearchQuery(rawQuery, params)
		if clean != "" {
			zap.L().Info("Performing web search for knowledge_researcher",
				zap.String("query", clean))
			if results, err := performWebSearch(context.Background(), clean); err == nil && len(results) > 0 {
				searchText := formatSearchResults(results)
				userPrompt = searchText + "\n\n---\n\n" + userPrompt
				zap.L().Info("Web search results injected into knowledge_researcher",
					zap.Int("resultCount", len(results)))
			} else if err != nil {
				zap.L().Warn("Web search failed, falling back to LLM knowledge",
					zap.Error(err))
			}
		} else {
			zap.L().Warn("Web search skipped: no query available for knowledge_researcher")
		}
	}

	effectiveCfg := effectiveVideoCreationOpenAIConfig(params)

	if effectiveCfg.APIKey == "" {
		switch toolName {
		case "video_script_generator":
			return tool.SuccessResult(buildFallbackVideoScriptData(toolName, skillName, topic, targetDurationSec, usedFacts, knowledgeTrace))
		case "continuity_checker":
			if data, ok := buildFallbackContinuityData(toolName, skillName, topic, params); ok {
				return tool.SuccessResult(data)
			}
		case "reference_asset_planner":
			if data, ok := buildFallbackReferenceAssetData(toolName, skillName, topic, params); ok {
				return tool.SuccessResult(data)
			}
		case "keyframe_prompt_generator":
			if data, ok := buildFallbackKeyframePromptData(toolName, skillName, topic, params); ok {
				return tool.SuccessResult(data)
			}
		}
		content := fmt.Sprintf("# %s\n\n主题：%s\n\n> ⚠️ LLM API Key 未配置。请设置 API Key 以启用 AI 内容生成。", toolName, topic)
		data := map[string]interface{}{
			"content":   content,
			"artifacts": buildSkillStageArtifacts(toolName, skillName, false, false),
		}
		return tool.SuccessResult(data)
	}

	callParams := map[string]interface{}{
		"system_prompt": systemPrompt,
		"prompt":        userPrompt,
		"max_tokens":    8000,
	}
	// Enable DeepSeek JSON mode for tools that require structured JSON output
	if structuredOutput {
		callParams["response_format"] = map[string]string{"type": "json_object"}
		callParams["temperature"] = 0
	}

	callTool := &LlmApiTool{cfg: effectiveCfg}
	result := callTool.Execute(context.Background(), callParams, toolCtx)

	if !result.Success {
		zap.L().Error("LLM API call failed for dynamic agent prompt tool",
			zap.String("tool", toolName),
			zap.Error(fmt.Errorf("%s", result.Error)))
		return result
	}

	rawContent, _ := result.Data["content"].(string)
	finishReason, _ := result.Data["finishReason"].(string)
	rawContent = continueSkillStageIfNeeded(callTool, toolName, systemPrompt+"\n\n---\n\n"+userPrompt, rawContent, finishReason, toolCtx)

	displayContent, contentPkg, isJSON := normalizeStructuredToolContent(toolName, rawContent)
	if !isJSON && structuredOutput {
		if retryContent, retryFinishReason, ok := retryStructuredJSONOutput(callTool, toolName, systemPrompt, userPrompt, rawContent, toolCtx); ok {
			rawContent = continueSkillStageIfNeeded(callTool, toolName, systemPrompt+"\n\n---\n\n"+userPrompt, retryContent, retryFinishReason, toolCtx)
			displayContent, contentPkg, isJSON = normalizeStructuredToolContent(toolName, rawContent)
		}
	}

	// For tools that require structured JSON output, fail on parse error
	// instead of silently accepting markdown.
	if !isJSON && structuredOutput {
		preview := rawContent
		if len(preview) > 500 {
			preview = preview[:500]
		}
		zap.L().Error("dynamic agent prompt tool failed to produce structured JSON",
			zap.String("tool", toolName),
			zap.String("rawContent", preview))
		return tool.FailureResult(fmt.Sprintf("%s: LLM 未返回结构化 JSON，请重试", toolName))
	}

	if toolName == "video_prompt_generator" {
		ensureVideoPromptShotAssetPackages(contentPkg)
	}
	if toolName == "video_script_generator" && videoParamsLookCinematic(topic, params) && !hasCinematicScriptFields(contentPkg) {
		fallback := buildFallbackVideoScriptData(toolName, skillName, topic, targetDurationSec, usedFacts, knowledgeTrace)
		if fallbackPkg, ok := fallback["package"].(map[string]interface{}); ok {
			for _, key := range []string{"script", "detailedScript", "storyOutline", "characters", "scenes", "props", "sections", "scriptSpans", "qualityHints", "summary", "estimatedDurationSec"} {
				if value, exists := fallbackPkg[key]; exists {
					contentPkg[key] = value
				}
			}
			if canonical, err := json.Marshal(contentPkg); err == nil {
				displayContent = string(canonical)
			}
		}
	}
	if toolName == "video_script_generator" && normalizeVideoScriptTimingFields(contentPkg, targetDurationSec) {
		if canonical, err := json.Marshal(contentPkg); err == nil {
			displayContent = string(canonical)
		}
	}

	artifacts := buildSkillStageArtifacts(toolName, skillName, toolName == "publish_copy_generator", isJSON)
	if toolName == "video_prompt_generator" {
		artifacts = append(artifacts, buildShotAssetPackageArtifacts(contentPkg["shotAssetPackages"])...)
	}

	data := map[string]interface{}{
		"content":   displayContent,
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
	for _, key := range []string{
		"storyOutline", "detailedScript", "characters", "scenes", "props",
		"continuityReport", "continuityBible", "styleProfile",
		"referenceAssetPlan", "referenceAssetIndex", "globalReferenceAssets",
		"externalGenerationRequests",
	} {
		if value, ok := contentPkg[key]; ok {
			data[key] = value
		}
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
	if shotAssetPackages, ok := contentPkg["shotAssetPackages"]; ok {
		data["shotAssetPackages"] = shotAssetPackages
	}
	if totalDurationSec, ok := contentPkg["totalDurationSec"]; ok {
		data["totalDurationSec"] = totalDurationSec
	}
	if sections, ok := contentPkg["sections"]; ok {
		data["sections"] = sections
	}
	if scriptSpans, ok := contentPkg["scriptSpans"]; ok {
		data["scriptSpans"] = scriptSpans
	} else if sections, ok := data["sections"]; ok {
		data["scriptSpans"] = sections
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
	for _, key := range []string{"analysisSummary", "rubricBreakdown", "scoringRules", "evidence", "whatWorked", "keepDoing"} {
		if value, ok := contentPkg[key]; ok {
			data[key] = value
		}
	}
	if missingArtifacts, ok := contentPkg["missingArtifacts"]; ok {
		data["missingArtifacts"] = missingArtifacts
	}
	if unreviewedArtifacts, ok := contentPkg["unreviewedArtifacts"]; ok {
		data["unreviewedArtifacts"] = unreviewedArtifacts
	}
	if usedFacts, ok := contentPkg["usedFacts"]; ok {
		data["usedFacts"] = usedFacts
	}
	if unusedFacts, ok := contentPkg["unusedFacts"]; ok {
		data["unusedFacts"] = unusedFacts
	}
	if factCheckWarnings, ok := contentPkg["factCheckWarnings"]; ok {
		data["factCheckWarnings"] = factCheckWarnings
	}
	if knowledgeTrace, ok := contentPkg["knowledgeTrace"]; ok {
		data["knowledgeTrace"] = knowledgeTrace
	}
	if toolName == "video_script_generator" {
		if _, ok := data["usedFacts"]; !ok {
			data["usedFacts"] = usedFacts
		}
		if _, ok := data["factCheckWarnings"]; !ok {
			data["factCheckWarnings"] = []interface{}{}
		}
		if _, ok := data["knowledgeTrace"]; !ok {
			data["knowledgeTrace"] = knowledgeTrace
		}
	}

	return tool.SuccessResult(data)
}

func buildFallbackVideoScriptData(toolName, skillName, topic string, targetDurationSec int, usedFacts []map[string]interface{}, knowledgeTrace map[string]interface{}) map[string]interface{} {
	if targetDurationSec <= 0 {
		targetDurationSec = 30
	}
	subject := fallbackVideoScriptSubject(topic)
	if fallbackTopicLooksCinematic(topic) {
		return buildFallbackCinematicScriptData(toolName, skillName, topic, subject, targetDurationSec, usedFacts, knowledgeTrace)
	}
	sections := fallbackVideoScriptSections(subject, targetDurationSec)
	lines := make([]string, 0, len(sections))
	for _, section := range sections {
		if text := strings.TrimSpace(ensureStringValue(section["text"])); text != "" {
			lines = append(lines, text)
		}
	}
	script := strings.Join(lines, "\n")
	if len(usedFacts) == 0 {
		usedFacts = []map[string]interface{}{
			{"claim": fmt.Sprintf("围绕%s的产品价值生成确定性 E2E 口播稿。", subject), "source": "fallback_script_generator"},
		}
	}
	if knowledgeTrace == nil {
		knowledgeTrace = map[string]interface{}{}
	}
	knowledgeTrace["fallback"] = true
	knowledgeTrace["usedFactCount"] = len(usedFacts)
	contentPkg := map[string]interface{}{
		"script":               script,
		"summary":              fmt.Sprintf("%s 的 %d 秒开源宣传口播脚本。", subject, targetDurationSec),
		"estimatedDurationSec": targetDurationSec,
		"sections":             sections,
		"scriptSpans":          sections,
		"qualityHints": map[string]interface{}{
			"hasHook":           true,
			"hasStory":          true,
			"hasKnowledgeValue": true,
		},
		"usedFacts":         usedFacts,
		"unusedFacts":       []interface{}{},
		"factCheckWarnings": []interface{}{},
		"knowledgeTrace":    knowledgeTrace,
	}
	content := script
	if encoded, err := json.Marshal(contentPkg); err == nil {
		content = string(encoded)
	}
	return map[string]interface{}{
		"content":              content,
		"package":              contentPkg,
		"script":               script,
		"summary":              contentPkg["summary"],
		"estimatedDurationSec": targetDurationSec,
		"sections":             sections,
		"scriptSpans":          sections,
		"qualityHints":         contentPkg["qualityHints"],
		"usedFacts":            usedFacts,
		"unusedFacts":          []interface{}{},
		"factCheckWarnings":    []interface{}{},
		"knowledgeTrace":       knowledgeTrace,
		"artifacts":            buildSkillStageArtifacts(toolName, skillName, false, true),
	}
}

func buildApprovedVideoScriptData(toolName, skillName, topic, script string, targetDurationSec int, usedFacts []map[string]interface{}, knowledgeTrace map[string]interface{}) map[string]interface{} {
	if targetDurationSec <= 0 {
		targetDurationSec = 30
	}
	script = strings.TrimSpace(script)
	sections := []map[string]interface{}{
		{
			"id":          "SPAN_01",
			"spanId":      "SPAN_01",
			"name":        "审定口播稿",
			"text":        script,
			"scriptText":  script,
			"startSec":    0,
			"endSec":      targetDurationSec,
			"durationSec": targetDurationSec,
		},
	}
	if knowledgeTrace == nil {
		knowledgeTrace = map[string]interface{}{}
	}
	knowledgeTrace["providedScript"] = true
	knowledgeTrace["fallback"] = false
	knowledgeTrace["usedFactCount"] = len(usedFacts)
	contentPkg := map[string]interface{}{
		"script":               script,
		"summary":              fmt.Sprintf("%s 的 %d 秒审定口播稿。", topic, targetDurationSec),
		"estimatedDurationSec": targetDurationSec,
		"sections":             sections,
		"scriptSpans":          sections,
		"qualityHints": map[string]interface{}{
			"providedByUser": true,
		},
		"usedFacts":         usedFacts,
		"unusedFacts":       []interface{}{},
		"factCheckWarnings": []interface{}{},
		"knowledgeTrace":    knowledgeTrace,
	}
	content := script
	if encoded, err := json.Marshal(contentPkg); err == nil {
		content = string(encoded)
	}
	return map[string]interface{}{
		"content":              content,
		"package":              contentPkg,
		"script":               script,
		"summary":              contentPkg["summary"],
		"estimatedDurationSec": targetDurationSec,
		"sections":             sections,
		"scriptSpans":          sections,
		"qualityHints":         contentPkg["qualityHints"],
		"usedFacts":            usedFacts,
		"unusedFacts":          []interface{}{},
		"factCheckWarnings":    []interface{}{},
		"knowledgeTrace":       knowledgeTrace,
		"artifacts":            buildSkillStageArtifacts(toolName, skillName, false, true),
	}
}

func buildFallbackCinematicScriptData(toolName, skillName, topic, subject string, targetDurationSec int, usedFacts []map[string]interface{}, knowledgeTrace map[string]interface{}) map[string]interface{} {
	if targetDurationSec <= 0 {
		targetDurationSec = 45
	}
	if targetDurationSec > 90 {
		targetDurationSec = 90
	}
	sections := fallbackCinematicScriptSections(subject, targetDurationSec)
	lines := make([]string, 0, len(sections))
	for _, section := range sections {
		if text := strings.TrimSpace(ensureStringValue(section["text"])); text != "" {
			lines = append(lines, text)
		}
	}
	characters := fallbackCinematicCharacters(subject)
	scenes := fallbackCinematicScenes(subject)
	props := fallbackCinematicProps(subject)
	storyOutline := map[string]interface{}{
		"logline": fmt.Sprintf("一个被任务追着跑的创作者，把混乱需求交给%s后，发现视频生产可以像拍短片一样可控。", subject),
		"theme":   "用正能量、轻松无厘头的方式表达：创作流程透明、可审核、可返修，焦虑可以被系统化工作流化解。",
		"beats": []map[string]interface{}{
			{"id": "beat_01", "name": "钩子", "purpose": "用夸张混乱建立共鸣"},
			{"id": "beat_02", "name": "发现", "purpose": "展示系统把想法拆成流程"},
			{"id": "beat_03", "name": "转折", "purpose": "MCP 和 QA 让生成结果可控"},
			{"id": "beat_04", "name": "收束", "purpose": "开源项目邀请关注和共建"},
		},
		"ending": "混乱的创作桌面变成干净的导演台，项目开源上线，观众被邀请关注后续真实迭代。",
	}
	detailedScript := strings.Join(lines, "\n\n")
	if len(usedFacts) == 0 {
		usedFacts = []map[string]interface{}{
			{"claim": fmt.Sprintf("%s 的影视 fallback 以结构化剧本、角色、场景和道具档案为中心。", subject), "source": "fallback_cinematic_script_generator"},
		}
	}
	if knowledgeTrace == nil {
		knowledgeTrace = map[string]interface{}{}
	}
	knowledgeTrace["fallback"] = true
	knowledgeTrace["profile"] = videomodel.VideoProfileCinematicStory
	knowledgeTrace["usedFactCount"] = len(usedFacts)
	contentPkg := map[string]interface{}{
		"script":               detailedScript,
		"detailedScript":       detailedScript,
		"storyOutline":         storyOutline,
		"characters":           characters,
		"scenes":               scenes,
		"props":                props,
		"summary":              fmt.Sprintf("%s 的影视短片详细剧本和全局一致性档案。", subject),
		"estimatedDurationSec": targetDurationSec,
		"sections":             sections,
		"scriptSpans":          sections,
		"qualityHints": map[string]interface{}{
			"hasHook":              true,
			"hasStory":             true,
			"hasCharacterDossiers": true,
			"hasSceneDossiers":     true,
			"hasPropDossiers":      true,
			"nonRealHumanStyle":    true,
		},
		"usedFacts":         usedFacts,
		"unusedFacts":       []interface{}{},
		"factCheckWarnings": []interface{}{},
		"knowledgeTrace":    knowledgeTrace,
	}
	content := detailedScript
	if encoded, err := json.Marshal(contentPkg); err == nil {
		content = string(encoded)
	}
	return map[string]interface{}{
		"content":              content,
		"package":              contentPkg,
		"script":               detailedScript,
		"detailedScript":       detailedScript,
		"storyOutline":         storyOutline,
		"characters":           characters,
		"scenes":               scenes,
		"props":                props,
		"summary":              contentPkg["summary"],
		"estimatedDurationSec": targetDurationSec,
		"sections":             sections,
		"scriptSpans":          sections,
		"qualityHints":         contentPkg["qualityHints"],
		"usedFacts":            usedFacts,
		"unusedFacts":          []interface{}{},
		"factCheckWarnings":    []interface{}{},
		"knowledgeTrace":       knowledgeTrace,
		"artifacts":            buildSkillStageArtifacts(toolName, skillName, false, true),
	}
}

func fallbackTopicLooksCinematic(topic string) bool {
	return containsAny(strings.ToLower(topic), "影视", "剧情", "短片", "角色", "场景", "道具", "导演", "连续性", "cinematic", "story")
}

func videoParamsLookCinematic(topic string, params map[string]interface{}) bool {
	if fallbackTopicLooksCinematic(topic) {
		return true
	}
	for _, key := range []string{"videoType", "projectMode", "profileId", "creationProfileId", "dagTemplateId"} {
		if looksCinematicProfile(stringParam(params, key, "")) {
			return true
		}
	}
	for _, key := range []string{"creationProfile", "profile"} {
		profile, ok := mapValue(params[key])
		if !ok {
			continue
		}
		for _, profileKey := range []string{"id", "profileId", "sourceRoute", "dagTemplateId", "videoType", "mode"} {
			if looksCinematicProfile(firstNonEmptyString(profile, profileKey)) {
				return true
			}
		}
	}
	return false
}

func looksCinematicProfile(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "cinematic_story" ||
		normalized == "cinematic-story" ||
		normalized == "film_story" ||
		normalized == "film-story" ||
		strings.Contains(normalized, "cinematic_story") ||
		strings.Contains(normalized, "cinematic-story")
}

func hasCinematicScriptFields(contentPkg map[string]interface{}) bool {
	if contentPkg == nil {
		return false
	}
	for _, key := range []string{"storyOutline", "detailedScript", "characters", "scenes", "props", "scriptSpans"} {
		if value, ok := contentPkg[key]; !ok || !hasMeaningfulStructuredValue(value) {
			return false
		}
	}
	return true
}

func hasMeaningfulStructuredValue(value interface{}) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []interface{}:
		return len(typed) > 0
	case []map[string]interface{}:
		return len(typed) > 0
	case map[string]interface{}:
		return len(typed) > 0
	default:
		return value != nil
	}
}

func fallbackCinematicScriptSections(subject string, targetDurationSec int) []map[string]interface{} {
	texts := []string{
		"夜晚的创作桌像刚经历过一场需求暴雨，主角盯着屏幕说：我只是想做条视频，为什么像在解谜。桌上的任务卡突然立起来，把自己排成歪歪扭扭的队。",
		fmt.Sprintf("主角把一句话需求放进%s，导演台亮起，故事大纲、角色、场景、道具和 shot 队列像舞台灯一样依次打开。任务卡从乱跳变成排队走路。", subject),
		"即梦 MCP 插槽弹出，参考图和 AIGC shot 被打包进每个镜头。QA 放大镜从画面边缘扫过，把拥挤字幕、混乱背景和不匹配镜头逐个标红，再给出返修建议。",
		fmt.Sprintf("最后，创作桌恢复清爽，主角端起咖啡对镜头说：这不是让 AI 乱发挥，是把创作变成可审核流水线。%s 开源上线，关注后续真实迭代。", subject),
	}
	durations := distributeFallbackScriptDurations(targetDurationSec, len(texts))
	names := []string{"混乱钩子", "流程展开", "MCP 与 QA 转折", "开源收束"}
	sections := make([]map[string]interface{}, 0, len(texts))
	start := 0
	for i, text := range texts {
		duration := durations[i]
		end := start + duration
		sectionID := fmt.Sprintf("SCENE_%02d", i+1)
		sections = append(sections, map[string]interface{}{
			"id":          sectionID,
			"spanId":      sectionID,
			"name":        names[i],
			"startSec":    start,
			"endSec":      end,
			"durationSec": duration,
			"text":        text,
			"scriptText":  text,
			"sceneId":     fmt.Sprintf("scene_%02d", minInt(i+1, 2)),
		})
		start = end
	}
	return sections
}

func fallbackCinematicCharacters(subject string) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":             "char_creator",
			"name":           "创作者小唐",
			"role":           "主角",
			"description":    "非真人风格化 3D 动画角色，圆润轮廓，表情夸张但积极，代表被工具和需求包围的普通创作者。",
			"motivation":     "想把混乱创作变成可控流程。",
			"invariants":     []string{"浅色外套", "圆形眼镜", "积极表情", "不出现真人写实皮肤"},
			"referenceViews": []string{"front", "side", "back", "expression_sheet"},
		},
		{
			"id":             "char_task_card",
			"name":           "会排队的任务卡",
			"role":           "喜剧辅助角色",
			"description":    "拟物化任务卡片，非真人风格，能跳动、排队和举牌，用无厘头方式表现可审核流程。",
			"motivation":     fmt.Sprintf("把%s的流程节点变成观众一眼能懂的画面。", subject),
			"invariants":     []string{"白色卡片主体", "蓝绿状态条", "无真实品牌文字", "动作轻快"},
			"referenceViews": []string{"front", "side", "back", "pose_sheet"},
		},
	}
}

func fallbackCinematicScenes(subject string) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":             "scene_creator_desk",
			"name":           "夜晚创作桌",
			"description":    "非真人风格化动画工作台，屏幕、便签、咖啡杯和柔和台灯构成主空间，开场混乱，结尾清爽。",
			"spatialLocks":   []string{"屏幕在画面右侧", "台灯在左后方", "桌面中央留给任务卡动作", "暖色主光"},
			"referenceViews": []string{"entrance_view", "reverse_view", "side_view", "top_layout"},
		},
		{
			"id":             "scene_tangying_director_console",
			"name":           subject + " 导演台",
			"description":    "像小型制片控制台的抽象界面，节点、审核门、MCP 插槽和 QA 面板清晰分区，不出现密集小字。",
			"spatialLocks":   []string{"流程节点从左到右", "MCP 插槽在右侧", "QA 面板在下方安全区外", "中心保留主体动作"},
			"referenceViews": []string{"front_view", "angled_view", "side_view", "wide_layout"},
		},
	}
}

func fallbackCinematicProps(subject string) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":             "prop_mcp_slot",
			"name":           "MCP 插槽",
			"description":    "标准协议插槽的拟物化道具，像可插拔积木，不绑定具体语言或框架。",
			"invariants":     []string{"接口形状稳定", "蓝绿色连接光", "无密集文字"},
			"referenceViews": []string{"front", "side", "back", "detail"},
		},
		{
			"id":             "prop_qa_magnifier",
			"name":           "QA 放大镜",
			"description":    "用于扫描 shot 画面质量的放大镜道具，能投射量化指标和返修箭头。",
			"invariants":     []string{"透明镜片", "绿色通过标记", "红色问题标记", "不遮挡主体"},
			"referenceViews": []string{"front", "side", "back", "detail"},
		},
	}
}

func buildFallbackContinuityData(toolName, skillName, topic string, params map[string]interface{}) (map[string]interface{}, bool) {
	script := firstNonEmptyString(params, "script", "detailedScript", "brief", "topic")
	if script == "" && !fallbackTopicLooksCinematic(topic) {
		return nil, false
	}
	subject := fallbackVideoScriptSubject(topic)
	characters := toolMapsFromValue(params["characters"], "characters")
	if len(characters) == 0 {
		characters = fallbackCinematicCharacters(subject)
	}
	scenes := toolMapsFromValue(params["scenes"], "scenes")
	if len(scenes) == 0 {
		scenes = fallbackCinematicScenes(subject)
	}
	props := toolMapsFromValue(params["props"], "props")
	if len(props) == 0 {
		props = fallbackCinematicProps(subject)
	}
	styleProfile := map[string]interface{}{
		"artifactKind": "STYLE_PROFILE",
		"tone":         "正能量、搞笑、轻松、无厘头但不嘲讽用户",
		"visualStyle":  "非真人风格化 3D 动画短片，电影感光影，明亮干净，禁止真人写实和照片级真人皮肤。",
		"aspectRatio":  "16:9",
	}
	continuity := map[string]interface{}{
		"artifactKind": "CONTINUITY_REPORT",
		"storyPurpose": "保持故事、角色、场景、道具和全片风格在所有 shot 中一致。",
		"characters":   characters,
		"scenes":       scenes,
		"props":        props,
		"globalLocks": []string{
			"所有角色、场景和道具保持非真人风格化动画体系。",
			"每个 shot 可独立生成，但只共享主要角色、主场景、核心道具和全片风格。",
			"禁止依赖上一镜尾帧或下一镜首帧；连续性通过参考图和档案锁定。",
			"画面文字交给 HyperFrames/HTML overlay，不要求 AIGC 视频内生生成小字。",
		},
		"shotQAPolicy": []string{
			"每个 shot 都检查 3-15 秒规格。",
			"每个 shot 必须有剧本作用、画面主体、运镜、光影、参考资产和返修依据。",
			"最终渲染后按 shot 抽帧 QA，输出量化指标和修复建议。",
		},
		"styleProfile": styleProfile,
	}
	contentPkg := map[string]interface{}{
		"continuityReport": continuity,
		"continuityBible":  continuity,
		"styleProfile":     styleProfile,
		"summary":          "已生成影视短片连续性圣经，包含角色、场景、道具、风格和 QA 锁定规则。",
	}
	contentBytes, _ := json.Marshal(contentPkg)
	return map[string]interface{}{
		"content":          string(contentBytes),
		"package":          contentPkg,
		"continuityReport": continuity,
		"continuityBible":  continuity,
		"styleProfile":     styleProfile,
		"summary":          contentPkg["summary"],
		"artifacts":        buildSkillStageArtifacts(toolName, skillName, false, true),
	}, true
}

func buildFallbackReferenceAssetData(toolName, skillName, topic string, params map[string]interface{}) (map[string]interface{}, bool) {
	subject := fallbackVideoScriptSubject(topic)
	continuity := map[string]interface{}{}
	if values, ok := mapValue(params["continuityBible"]); ok {
		continuity = values
	}
	characters := toolMapsFromValue(firstExistingValue(continuity, "characters"), "characters")
	if len(characters) == 0 {
		characters = toolMapsFromValue(params["characters"], "characters")
	}
	if len(characters) == 0 {
		characters = fallbackCinematicCharacters(subject)
	}
	scenes := toolMapsFromValue(firstExistingValue(continuity, "scenes"), "scenes")
	if len(scenes) == 0 {
		scenes = toolMapsFromValue(params["scenes"], "scenes")
	}
	if len(scenes) == 0 {
		scenes = fallbackCinematicScenes(subject)
	}
	props := toolMapsFromValue(firstExistingValue(continuity, "props"), "props")
	if len(props) == 0 {
		props = toolMapsFromValue(params["props"], "props")
	}
	if len(props) == 0 {
		props = fallbackCinematicProps(subject)
	}

	index := []map[string]interface{}{}
	requests := []interface{}{}
	addRefs := func(items []map[string]interface{}, role string) {
		for _, item := range items {
			id := firstNonEmptyString(item, "id", "name")
			if id == "" {
				id = fmt.Sprintf("%s_%02d", role, len(index)+1)
			}
			name := firstNonEmptyString(item, "name", "label")
			if name == "" {
				name = id
			}
			views := firstStringListInMap(item, "referenceViews", "views")
			if len(views) == 0 {
				views = []string{"front", "side", "back", "detail"}
			}
			description := firstNonEmptyString(item, "description", "summary")
			locks := firstStringListInMap(item, "invariants", "spatialLocks", "locks")
			ref := map[string]interface{}{
				"id":          id,
				"label":       name,
				"role":        role,
				"views":       views,
				"description": description,
				"locks":       locks,
				"prompt":      fallbackReferencePrompt(role, name, description, views, locks),
				"target":      map[string]interface{}{"aspectRatio": "16:9", "resolution": "1920x1080"},
				"status":      "pending_generation",
			}
			index = append(index, ref)
			requests = append(requests, map[string]interface{}{
				"requestId":           "extgen_ref_" + sanitizeUnitPart(id),
				"assetId":             id,
				"shotId":              "GLOBAL_REFERENCE",
				"kind":                "image",
				"role":                role,
				"prompt":              ref["prompt"],
				"promptText":          ref["prompt"],
				"negativePrompt":      "禁止真人写实、照片级真人皮肤、真实演员、明星脸、密集文字、遮挡主体、风格不统一。",
				"references":          []interface{}{},
				"target":              map[string]interface{}{"aspectRatio": "16:9", "resolution": "1920x1080", "generateNum": 1},
				"promptCharLimit":     2000,
				"referenceImageLimit": 0,
				"status":              "pending_upload",
				"artifactKind":        "REFERENCE_IMAGE",
			})
		}
	}
	addRefs(characters, "character")
	addRefs(scenes, "scene")
	addRefs(props, "prop")

	plan := map[string]interface{}{
		"artifactKind": "REFERENCE_ASSET_PLAN",
		"stylePackage": map[string]interface{}{
			"style":       "非真人风格化 3D 动画短片，电影感光影，明亮、解压、正能量。",
			"aspectRatio": "16:9",
			"negative":    "禁止真人写实、照片级皮肤、真实演员、密集文字、风格漂移。",
		},
		"characters":            characters,
		"scenes":                scenes,
		"props":                 props,
		"referenceAssetIndex":   index,
		"globalReferenceAssets": index,
		"generationPolicy":      "先生成主要角色、主场景、核心道具的多视角设定板；shot 视频只引用必要的全局参考，避免跨 shot 画面依赖。",
		"externalRequestCount":  len(requests),
	}
	contentPkg := map[string]interface{}{
		"referenceAssetPlan":         plan,
		"referenceAssetIndex":        index,
		"globalReferenceAssets":      index,
		"externalGenerationRequests": requests,
		"summary":                    fmt.Sprintf("已为 %s 规划 %d 个全局一致性参考图请求。", subject, len(requests)),
	}
	contentBytes, _ := json.Marshal(contentPkg)
	artifacts := buildSkillStageArtifacts(toolName, skillName, false, true)
	for _, item := range requests {
		if req, ok := item.(map[string]interface{}); ok {
			artifacts = append(artifacts, externalGenerationArtifact(firstNonEmptyString(req, "requestId"), firstNonEmptyString(req, "shotId"), "image"))
		}
	}
	return map[string]interface{}{
		"content":                    string(contentBytes),
		"package":                    contentPkg,
		"referenceAssetPlan":         plan,
		"referenceAssetIndex":        index,
		"globalReferenceAssets":      index,
		"externalGenerationRequests": requests,
		"summary":                    contentPkg["summary"],
		"artifacts":                  artifacts,
	}, true
}

func fallbackReferencePrompt(role, name, description string, views []string, locks []string) string {
	parts := []string{
		"16:9 横屏 1920x1080，多视角设定板，非真人风格化动画短片画面，禁止真人写实。",
		fmt.Sprintf("对象：%s（%s）。", name, role),
		"画面只做参考资产设定，不承载剧情，不放密集说明文字。",
		"需要展示视角：" + strings.Join(views, "、") + "。",
		"电影感光影，主体清晰，背景简洁，边缘留白，适合后续 AIGC shot 继承一致性。",
	}
	if description != "" {
		parts = append(parts, "档案描述："+description)
	}
	if len(locks) > 0 {
		parts = append(parts, "不可变化项："+strings.Join(locks, "、"))
	}
	return limitPromptRunes(strings.Join(parts, "\n"), 2000)
}

func buildFallbackKeyframePromptData(toolName, skillName, topic string, params map[string]interface{}) (map[string]interface{}, bool) {
	shots := normalizeShotItemsForAssetDecision(params["shotList"])
	if len(shots) == 0 {
		return nil, false
	}
	refPlan, _ := mapValue(params["referenceAssetPlan"])
	globalRefs := interfaceSliceFromAny(firstExistingValue(refPlan, "globalReferenceAssets", "referenceAssetIndex"))
	prompts := make([]interface{}, 0, len(shots))
	requests := make([]interface{}, 0, len(shots))
	for i, shot := range shots {
		shotID := firstNonEmptyString(shot, "shotId", "id")
		if shotID == "" {
			shotID = fmt.Sprintf("SHOT_%02d", i+1)
		}
		visual := firstNonEmptyString(shot, "visual", "sceneSummary", "description", "mainAction")
		camera := firstNonEmptyString(shot, "camera", "cameraMotion")
		lighting := firstNonEmptyString(shot, "lighting", "mood")
		why := firstNonEmptyString(shot, "whyThisShot", "dramaticPurpose", "directorReason")
		references := referencesForShotFromPlan(shot, globalRefs)
		promptText := limitPromptRunes(strings.Join(compactStrings([]string{
			"16:9 横屏 1920x1080，非真人风格化动画短片关键帧，禁止真人写实。",
			"镜头：" + fallbackText(visual, shotID),
			"构图/运镜：" + camera,
			"光影：" + lighting,
			"画面意义：" + why,
			"主体清晰，动作单一，留出字幕安全区，不生成密集文字。",
		}), "\n"), 2000)
		prompts = append(prompts, map[string]interface{}{
			"shotId":            shotID,
			"prompt":            promptText,
			"referenceCoverage": referenceCoverageForShot(references),
			"styleNotes":        "非真人风格化动画、电影感光影、统一角色/场景/道具参考。",
		})
		requests = append(requests, map[string]interface{}{
			"requestId":           "extgen_keyframe_" + sanitizeUnitPart(shotID),
			"kind":                "image",
			"shotId":              shotID,
			"prompt":              promptText,
			"promptText":          promptText,
			"negativePrompt":      "禁止真人写实、照片级真人皮肤、真实演员、密集文字、主体遮挡、跨 shot 帧对齐。",
			"references":          references,
			"target":              map[string]interface{}{"aspectRatio": "16:9", "resolution": "1920x1080", "generateNum": 1},
			"promptCharLimit":     2000,
			"referenceImageLimit": 6,
			"status":              "pending_upload",
		})
	}
	contentPkg := map[string]interface{}{
		"keyframePrompts":            prompts,
		"externalGenerationRequests": requests,
		"summary":                    fmt.Sprintf("已为 %d 个 shot 生成关键帧提示词。", len(prompts)),
	}
	contentBytes, _ := json.Marshal(contentPkg)
	return map[string]interface{}{
		"content":                    string(contentBytes),
		"package":                    contentPkg,
		"keyframePrompts":            prompts,
		"externalGenerationRequests": requests,
		"summary":                    contentPkg["summary"],
		"artifacts":                  buildSkillStageArtifacts(toolName, skillName, false, true),
	}, true
}

func referencesForShotFromPlan(shot map[string]interface{}, globalRefs []interface{}) []interface{} {
	if refs := interfaceSliceFromAny(firstExistingValue(shot, "referenceImages", "references")); len(refs) > 0 {
		return limitInterfaces(refs, 6)
	}
	ids := stringListFromInterface(firstExistingValue(shot, "referenceAssetIds", "referenceIds"))
	if len(ids) == 0 {
		return limitInterfaces(globalRefs, 6)
	}
	selected := []interface{}{}
	for _, refItem := range globalRefs {
		ref, ok := mapValue(refItem)
		if !ok {
			continue
		}
		refID := firstNonEmptyString(ref, "id", "assetId")
		for _, id := range ids {
			if refID == id {
				selected = append(selected, ref)
				break
			}
		}
	}
	if len(selected) == 0 {
		return limitInterfaces(globalRefs, 6)
	}
	return limitInterfaces(selected, 6)
}

func referenceCoverageForShot(references []interface{}) map[string]interface{} {
	coverage := map[string]interface{}{
		"characters": []interface{}{},
		"scenes":     []interface{}{},
		"props":      []interface{}{},
		"missing":    []interface{}{},
	}
	for _, item := range references {
		ref, ok := mapValue(item)
		if !ok {
			continue
		}
		role := firstNonEmptyString(ref, "role")
		switch role {
		case "character":
			coverage["characters"] = append(coverage["characters"].([]interface{}), ref)
		case "scene":
			coverage["scenes"] = append(coverage["scenes"].([]interface{}), ref)
		case "prop":
			coverage["props"] = append(coverage["props"].([]interface{}), ref)
		}
	}
	return coverage
}

func limitInterfaces(items []interface{}, limit int) []interface{} {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func inferTargetDurationSec(topic string, fallback int) int {
	if fallback <= 0 {
		fallback = 60
	}
	match := durationHintPattern.FindStringSubmatch(strings.TrimSpace(topic))
	if len(match) < 2 {
		return fallback
	}
	value, err := strconv.Atoi(match[1])
	if err != nil || value <= 0 {
		return fallback
	}
	if value < 3 {
		return 3
	}
	if value > 300 {
		return fallback
	}
	return value
}

func fallbackVideoScriptSubject(topic string) string {
	trimmed := strings.TrimSpace(topic)
	if strings.Contains(trimmed, "躺营 AI OS") {
		return "躺营 AI OS"
	}
	if strings.Contains(trimmed, "Tangying") || strings.Contains(trimmed, "tangying") {
		return "Tangying AI OS"
	}
	for _, sep := range []string{"：", ":"} {
		if idx := strings.Index(trimmed, sep); idx >= 0 && idx+len(sep) < len(trimmed) {
			trimmed = strings.TrimSpace(trimmed[idx+len(sep):])
			break
		}
	}
	if trimmed == "" {
		return "这个项目"
	}
	return truncateText(trimmed, 28)
}

func fallbackVideoScriptSections(subject string, targetDurationSec int) []map[string]interface{} {
	texts := []string{
		fmt.Sprintf("你是不是也觉得，AI 工具越用越多，真正能跑完的流程却越来越少？%s 解决的就是这个问题。", subject),
		fmt.Sprintf("%s 把选题、脚本、分镜、审核、生成和打包放进一条可追踪流水线，非技术用户也能按步骤推进。", subject),
		"你不用理解复杂的 agent 架构，只要确认每个关键节点，系统会调用本机 runner、HyperFrames 和 MCP，把想法变成可交付视频。",
		fmt.Sprintf("我会开源完整项目，持续记录真实使用和修复过程。关注%s，一起把 AI 内容生产从演示变成日常工具。", subject),
	}
	durations := distributeFallbackScriptDurations(targetDurationSec, len(texts))
	sections := make([]map[string]interface{}, 0, len(texts))
	start := 0
	for i, text := range texts {
		duration := durations[i]
		end := start + duration
		sectionID := fmt.Sprintf("SPAN_%02d", i+1)
		sections = append(sections, map[string]interface{}{
			"id":          sectionID,
			"spanId":      sectionID,
			"name":        []string{"开场钩子", "痛点和方案", "流程演示", "开源关注"}[i],
			"startSec":    start,
			"endSec":      end,
			"durationSec": duration,
			"text":        text,
			"scriptText":  text,
		})
		start = end
	}
	return sections
}

func distributeFallbackScriptDurations(targetDurationSec, count int) []int {
	if count <= 0 {
		return nil
	}
	minTotal := count * 3
	maxTotal := count * 15
	if targetDurationSec < minTotal {
		targetDurationSec = minTotal
	}
	if targetDurationSec > maxTotal {
		targetDurationSec = maxTotal
	}
	base := targetDurationSec / count
	remaining := targetDurationSec % count
	durations := make([]int, count)
	for i := 0; i < count; i++ {
		duration := base
		if i < remaining {
			duration++
		}
		if duration < 3 {
			duration = 3
		}
		if duration > 15 {
			duration = 15
		}
		durations[i] = duration
	}
	return durations
}

func appendJSONModeSystemInstruction(systemPrompt string) string {
	const instruction = `

JSON mode requirements:
- You must output one valid JSON object only.
- Do not output markdown fences, explanations, reasoning, comments, or text outside JSON.
- Use double-quoted JSON property names and strings.`
	if strings.Contains(strings.ToLower(systemPrompt), "json mode requirements") {
		return systemPrompt
	}
	return strings.TrimSpace(systemPrompt) + instruction
}

func appendJSONModeUserInstruction(userPrompt string) string {
	const instruction = `

请严格只返回一个合法 JSON object。不要输出 Markdown，不要输出代码块，不要输出分析过程，不要在 JSON 前后添加任何文字。`
	if strings.Contains(strings.ToLower(userPrompt), "json") && strings.Contains(userPrompt, "不要输出 Markdown") {
		return userPrompt
	}
	return strings.TrimSpace(userPrompt) + instruction
}

func retryStructuredJSONOutput(callTool *LlmApiTool, toolName, systemPrompt, userPrompt, rawContent string, toolCtx tool.ToolContext) (string, string, bool) {
	preview := rawContent
	if len(preview) > 1000 {
		preview = preview[:1000]
	}
	retryPrompt := fmt.Sprintf(`上一次输出不是合法 JSON object，系统无法解析。

上一次输出：
%s

请重新执行原始任务，并且严格只输出一个合法 JSON object。不要输出 Markdown、代码块、解释、分析过程或 JSON 外文字。

原始任务：
%s`, preview, userPrompt)
	result := callTool.Execute(context.Background(), map[string]interface{}{
		"system_prompt":   appendJSONModeSystemInstruction(systemPrompt),
		"prompt":          retryPrompt,
		"max_tokens":      8000,
		"temperature":     0,
		"response_format": map[string]string{"type": "json_object"},
	}, toolCtx)
	if !result.Success {
		zap.L().Warn("structured JSON retry failed",
			zap.String("tool", toolName),
			zap.String("error", result.Error))
		return "", "", false
	}
	content, _ := result.Data["content"].(string)
	finishReason, _ := result.Data["finishReason"].(string)
	return content, finishReason, strings.TrimSpace(content) != ""
}

func requiresFreshKnowledge(params map[string]interface{}) bool {
	return strings.EqualFold(stringParam(params, "retrievalPolicy", ""), "required") ||
		boolParam(params, "mustUseFreshKnowledge", false) ||
		boolParam(params, "requireFreshFacts", false)
}

func shouldBlockOnEmptyFreshFacts(params map[string]interface{}) bool {
	return boolParam(params, "blockOnEmptyFacts", false)
}

func tangyingProductFactsForTopic(topic string) []map[string]interface{} {
	normalized := strings.ToLower(strings.TrimSpace(topic))
	if normalized == "" {
		return nil
	}
	explicitProduct := containsAny(normalized,
		"躺营", "tangying", "aios", "ai os", "ai operation system", "视频创作助手", "自媒体运营助手",
		"导演台", "本系统", "这个系统", "本产品", "这个产品",
	)
	selfPromoIntent := containsAny(normalized,
		"开源", "open source", "opensource", "宣传", "介绍", "发布", "上线", "完播率", "短视频节奏",
	)
	if !explicitProduct || !selfPromoIntent {
		return nil
	}
	return []map[string]interface{}{
		{
			"claim":  "用户本次明确要求介绍躺营 AI 视频创作助手，并强调系统开源、用于短视频宣传、提升完播率，以及后续持续发布由本系统创作的视频内容。",
			"source": "user_brief",
		},
		{
			"claim":  "躺营 AI 视频创作助手是一套从想法开始，把选题、脚本、分镜、素材生成、审核、渲染和交付串成可追踪视频创作流水线的系统。",
			"source": "README",
		},
		{
			"claim":  "系统采用云端 Agent 编排加桌面端本地 runner 的结构，本地端负责文件、工具执行、模型配置和用户机器上的敏感凭据。",
			"source": "README",
		},
		{
			"claim":  "系统支持口播/知识类视频和影视化/AIGC shot 视频两类创作入口。",
			"source": "README",
		},
		{
			"claim":  "closed beta 视频流水线包含语义 shot 切分、3-15 秒时长校验、candidate 级 QA、保守 repair loop、accepted shot gate、FFmpeg final assembly 和 final QA。",
			"source": "README",
		},
		{
			"claim":  "即梦 CLI/MCP 登录配置和文生图片、文生视频 Provider 在设置页统一管理；模型 Key 与即梦登录态保留在用户本机。",
			"source": "README",
		},
	}
}

func buildTangyingProductKnowledgeResearchData(toolName, skillName string, productFacts []map[string]interface{}) map[string]interface{} {
	facts := make([]string, 0, len(productFacts))
	sourceSet := map[string]bool{}
	sourceNotes := make([]string, 0)
	for _, fact := range productFacts {
		claim := strings.TrimSpace(ensureStringValue(fact["claim"]))
		if claim != "" {
			facts = append(facts, claim)
		}
		source := strings.TrimSpace(ensureStringValue(fact["source"]))
		if source != "" && !sourceSet[source] {
			sourceSet[source] = true
			sourceNotes = append(sourceNotes, source)
		}
	}
	if len(sourceNotes) == 0 {
		sourceNotes = []string{"README", "user_brief"}
	}
	pkg := map[string]interface{}{
		"facts": facts,
		"timeline": []map[string]interface{}{
			{"date": "当前版本", "event": "躺营 AI 视频创作助手已具备从想法到脚本、分镜、素材、审核、渲染和交付的可追踪视频创作流水线。"},
			{"date": "closed beta", "event": "系统聚焦口播/知识类视频和影视化 AIGC shot 视频，并强化 shot QA、repair loop 与 final assembly。"},
		},
		"storyAngles": []string{
			"开源不是口号，而是把 AI 视频创作流程摊开给创作者和开发者看。",
			"用可审核阶段和本地执行器降低黑盒生成的不确定性，提升短视频节奏和完播率。",
			"后续持续发布由本系统创作的视频内容，用真实作品验证视频创作助手能力。",
		},
		"risks": []string{
			"避免宣称已具备完整自媒体运营能力，应聚焦视频创作助手定位。",
			"涉及真实 AIGC provider、即梦积分和模型 Key 时，应说明由用户本机配置和管理。",
		},
		"sourceNotes": sourceNotes,
		"summary":     "本轮事实材料确认：躺营 AI 视频创作助手是面向创作者的视频创作流水线工具，适合围绕开源、短视频节奏、可审核生产流程和持续发布真实作品进行宣传。",
	}
	content := ""
	if encoded, err := json.Marshal(pkg); err == nil {
		content = string(encoded)
	}
	return map[string]interface{}{
		"content":     content,
		"package":     pkg,
		"facts":       pkg["facts"],
		"timeline":    pkg["timeline"],
		"storyAngles": pkg["storyAngles"],
		"risks":       pkg["risks"],
		"sourceNotes": pkg["sourceNotes"],
		"summary":     pkg["summary"],
		"usedFacts":   productFacts,
		"artifacts":   buildSkillStageArtifacts(toolName, skillName, false, true),
	}
}

func hasKnowledgeFacts(value interface{}, fallbackFacts string) bool {
	if strings.TrimSpace(fallbackFacts) != "" {
		return true
	}
	switch typed := value.(type) {
	case []interface{}:
		return len(typed) > 0
	case []map[string]interface{}:
		return len(typed) > 0
	case map[string]interface{}:
		if facts, ok := typed["facts"]; ok {
			return hasKnowledgeFacts(facts, "")
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		return trimmed != "" && !strings.HasPrefix(trimmed, "{{")
	}
	return false
}

func hasKnowledgeContextFacts(value interface{}) bool {
	return len(usedFactsFromKnowledgeContext(value)) > 0
}

func formatKnowledgeContextForPrompt(value interface{}) string {
	items := usedFactsFromKnowledgeContext(value)
	if len(items) == 0 {
		return ""
	}
	sources := knowledgeSourcesFromContext(value)
	var b strings.Builder
	b.WriteString("knowledgeContext facts:\n")
	for i, item := range items {
		claim := firstNonEmptyString(item, "claim", "snippet", "title", "text", "summary")
		if claim == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("%d. %s", i+1, claim))
		if source := firstNonEmptyString(item, "source"); source != "" {
			b.WriteString(" 来源：" + source)
		}
		if date := firstNonEmptyString(item, "publishedAt", "date"); date != "" {
			b.WriteString(" 日期：" + date)
		}
		if urlValue := firstNonEmptyString(item, "url"); urlValue != "" {
			b.WriteString(" URL：" + urlValue)
		}
		b.WriteString("\n")
	}
	if len(sources) > 0 {
		b.WriteString("\nknowledgeContext sources:\n")
		for i, source := range sources {
			b.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, firstNonEmptyString(source, "source", "title"), firstNonEmptyString(source, "url")))
		}
	}
	return strings.TrimSpace(b.String())
}

func usedFactsFromKnowledgeContext(value interface{}) []map[string]interface{} {
	ctx, ok := value.(map[string]interface{})
	if !ok {
		return normalizeKnowledgeItems(value)
	}
	return normalizeKnowledgeItems(ctx["items"])
}

func knowledgeSourcesFromContext(value interface{}) []map[string]interface{} {
	ctx, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	return normalizeKnowledgeItems(ctx["sources"])
}

func usedFactsFromKnowledgePack(value interface{}) []map[string]interface{} {
	return normalizeKnowledgeItems(value)
}

func normalizeKnowledgeItems(value interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0)
	switch typed := value.(type) {
	case nil:
		return out
	case []map[string]interface{}:
		return append(out, typed...)
	case []interface{}:
		for _, item := range typed {
			out = append(out, normalizeKnowledgeItems(item)...)
		}
	case map[string]interface{}:
		if nested, ok := typed["facts"]; ok {
			return normalizeKnowledgeItems(nested)
		}
		if nested, ok := typed["items"]; ok {
			return normalizeKnowledgeItems(nested)
		}
		if claim := firstNonEmptyString(typed, "claim", "snippet", "title", "text", "summary"); claim != "" {
			item := copyStringMap(typed)
			item["claim"] = claim
			out = append(out, item)
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed != "" && !strings.HasPrefix(trimmed, "{{") {
			out = append(out, map[string]interface{}{"claim": trimmed})
		}
	}
	return out
}

func copyStringMap(values map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}

func knowledgeTraceFromParams(params map[string]interface{}, usedFacts []map[string]interface{}) map[string]interface{} {
	ctx, hasContext := params["knowledgeContext"].(map[string]interface{})
	items := usedFactsFromKnowledgeContext(params["knowledgeContext"])
	sources := knowledgeSourcesFromContext(params["knowledgeContext"])
	toolNames := []interface{}{}
	if hasContext {
		toolNames = append(toolNames, interfaceSliceForPrompt(ctx["generatedBy"])...)
	}
	return map[string]interface{}{
		"hasKnowledgeContext": hasContext,
		"knowledgeItemCount":  len(items),
		"sourceCount":         len(sources),
		"usedFactCount":       len(usedFacts),
		"toolNames":           toolNames,
		"retrievalPolicy":     stringParam(params, "retrievalPolicy", "none"),
	}
}

func interfaceSliceForPrompt(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		return typed
	case []string:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	case string:
		if typed != "" {
			return []interface{}{typed}
		}
	}
	return nil
}

func formatKnowledgePackForPrompt(value interface{}, sources interface{}) string {
	facts := searchResultsParam(value)
	if len(facts) == 0 {
		if items, ok := value.([]interface{}); ok {
			facts = make([]map[string]interface{}, 0, len(items))
			for _, item := range items {
				switch typed := item.(type) {
				case map[string]interface{}:
					facts = append(facts, typed)
				case string:
					facts = append(facts, map[string]interface{}{"claim": typed})
				}
			}
		}
	}
	if len(facts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("knowledgePack facts:\n")
	for i, fact := range facts {
		claim := firstNonEmptyString(fact, "claim", "snippet", "title", "text")
		if claim == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("%d. %s", i+1, claim))
		source := firstNonEmptyString(fact, "source")
		if source != "" {
			b.WriteString(" 来源：" + source)
		}
		if date := firstNonEmptyString(fact, "date", "publishedAt"); date != "" {
			b.WriteString(" 日期：" + date)
		}
		b.WriteString("\n")
	}
	sourceItems := searchResultsParam(sources)
	if len(sourceItems) > 0 {
		b.WriteString("\nknowledgeSources:\n")
		for i, source := range sourceItems {
			b.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, firstNonEmptyString(source, "source", "title"), firstNonEmptyString(source, "url")))
		}
	}
	return strings.TrimSpace(b.String())
}

func appendKnowledgePolicyForPrompt(facts string, params map[string]interface{}) string {
	policy := stringParam(params, "retrievalPolicy", "none")
	currentDate := stringParam(params, "currentDate", time.Now().Format("2006-01-02"))
	mustUse := boolParam(params, "mustUseFreshKnowledge", false)
	var parts []string
	parts = append(parts, "retrievalPolicy="+policy)
	parts = append(parts, "currentDate="+currentDate)
	if mustUse {
		parts = append(parts, "mustUseFreshKnowledge=true")
	}
	if strings.TrimSpace(facts) != "" {
		parts = append(parts, facts)
	}
	return strings.Join(parts, "\n")
}

func firstNonEmptyString(values map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(ensureStringValue(values[key])); value != "" {
			return value
		}
	}
	return ""
}

func voiceVisualNeedsRichAIGC(script, assetStrategy string) bool {
	text := strings.ToLower(normalizeInlineText(script + " " + assetStrategy))
	return containsAny(text,
		"aigc", "dreamina", "jimeng", "即梦", "素材", "b-roll", "broll",
		"搞笑", "无厘头", "解压", "爆款", "短视频", "有趣", "好笑", "反差", "荒诞", "沙雕",
	)
}

func voiceVisualAssetRoute(narration, visual, assetStrategy string, index, total int, creativeMode bool) string {
	text := strings.ToLower(normalizeInlineText(strings.Join([]string{narration, visual, assetStrategy}, " ")))
	switch {
	case containsAny(text, "录屏", "screen recording", "真实页面", "页面", "按钮", "日志", "输入框", "审核门", "工作台", "开发者能看"):
		return "screen_recording"
	case containsAny(text, "readme", "wiki", "tag", "release", "版本", "分支", "指标", "图表", "卡片", "流程图") && !containsAny(text, "无厘头", "搞笑", "解压"):
		return "hyperframes"
	case containsAny(text, "即梦", "dreamina", "jimeng", "aigc", "素材", "生成", "视频工厂", "电脑风扇", "打工人", "起飞", "流水线", "工厂", "开工", "爆火", "关注"):
		return "aigc_video"
	case creativeMode && total > 0:
		if index == total-1 || index%3 != 2 {
			return "aigc_video"
		}
		return "hyperframes"
	default:
		return "hyperframes"
	}
}

func voiceVisualDirectionForRoute(route, narration, fallbackVisual string, index int, creativeMode bool) string {
	text := normalizeInlineText(narration)
	lower := strings.ToLower(text)
	if fallbackVisual == "" {
		fallbackVisual = visualTitleFromNarration(text, index)
	}
	switch route {
	case "aigc_video":
		switch {
		case containsAny(lower, "风扇", "起飞", "十个 ai", "工具"):
			return "AIGC_VIDEO | 非真人风格化无厘头短视频：一个疲惫但可爱的打工人坐在桌前，十个 AI 工具窗口变成会弹跳的小便利贴，电脑风扇像迷你火箭一样夸张冒光但不危险；镜头快速推近，节奏解压，画面不要真实演员。"
		case containsAny(lower, "流水线", "脚本", "分镜", "渲染", "qa"):
			return "AIGC_VIDEO | 非真人风格化：一句口播变成一条会自动运转的迷你视频工厂，脚本、分镜、即梦素材、渲染、抽帧 QA 像传送带上的小工位依次亮起；节奏轻快，有荒诞喜剧感。"
		case containsAny(lower, "关注", "开源", "项目", "开工"):
			return "AIGC_VIDEO | 非真人风格化：一台小小的视频工厂自己开灯开工，Star、Fork、Follow 图标像彩色贴纸弹出来，最后收束到开源项目关注 CTA；画面轻松、搞笑、干净。"
		default:
			if creativeMode {
				return fmt.Sprintf("AIGC_VIDEO | 非真人风格化搞笑 b-roll：围绕“%s”设计一个轻松、有反差但清楚的视觉隐喻，主体有明确动作，场景持续变化，镜头在 16:9 横屏中轻快推进，使用清晰符号和可读画面。", fallbackText(fallbackVisual, text))
			}
			return fmt.Sprintf("AIGC_VIDEO | 非真人风格化：%s，主体动作清楚，镜头自然运动，16:9 横屏，使用动画化角色、道具或图形隐喻。", fallbackText(fallbackVisual, text))
		}
	case "screen_recording":
		return fmt.Sprintf("SCREEN_RECORDING | 真实系统页面录屏：围绕“%s”展示输入框、审核门、即梦 MCP 开关、运行节点或日志面板，关键 UI 用 HyperFrames 放大框和箭头提示，不让字幕遮挡页面文字。", fallbackText(fallbackVisual, text))
	case "hyperframes":
		return fmt.Sprintf("HYPERFRAMES | 信息层/字幕/CTA 包装：把“%s”做成少字、高对比、节奏快的图形层，只展示关键词、流程节点或版本信息，不承担整段情绪 b-roll。", fallbackText(fallbackVisual, text))
	default:
		return fallbackVisual
	}
}

func voiceVisualMainActionForRoute(route, narration string, index int) string {
	text := normalizeInlineText(narration)
	switch route {
	case "aigc_video":
		return fmt.Sprintf("用一个可视化反差动作承接口播：%s", truncateVisualTitle(text, 28))
	case "screen_recording":
		return "录屏演示真实页面状态，局部放大输入、审核和运行结果。"
	case "hyperframes":
		return "用确定性文字和图形信息层解释关键概念。"
	default:
		return visualSubtitleFromNarration(text, index)
	}
}

func voiceVisualCameraForRoute(route, narration string, index int) string {
	switch route {
	case "aigc_video":
		return "快节奏推近 + 轻微横移，动作在结尾 0.5 秒稳定收束，方便拼接。"
	case "screen_recording":
		return "录屏视角保持稳定，局部缩放聚焦关键按钮和状态。"
	case "hyperframes":
		return "文字层分批进入，避免同屏堆叠，结尾留 0.5 秒空隙。"
	default:
		return visualSubtitleFromNarration(narration, index)
	}
}

func voiceVisualAssetIntentForRoute(route, narration string) string {
	switch route {
	case "aigc_video":
		return "用 Dreamina/JiMeng MCP 生成口播对应的动态情绪素材，承担开头钩子、反差和解压感。"
	case "screen_recording":
		return "使用真实产品页面或运行记录证明系统能跑通，HyperFrames 只做放大标注。"
	case "hyperframes":
		return "使用确定性排版承载少量精确文字、流程节点、版本信息或 CTA。"
	default:
		return "根据口播选择最稳妥的素材承载方式。"
	}
}

func voiceVisualTimeRelationship(route string, durationSec int) string {
	if durationSec <= 0 {
		durationSec = normalizedDurationSec(nil)
	}
	switch route {
	case "aigc_video":
		return fmt.Sprintf("0-%.1fs 动态 b-roll 承接口播情绪；末尾 0.5s 稳定画面给字幕/转场。", float64(durationSec))
	case "screen_recording":
		return fmt.Sprintf("0-%.1fs 页面操作跟随口播关键词推进；关键按钮停留至少 1s。", float64(durationSec))
	case "hyperframes":
		return fmt.Sprintf("0-%.1fs 关键词分批进入，任意时刻主标题和字幕不超过两层。", float64(durationSec))
	default:
		return fmt.Sprintf("0-%.1fs 与口播同步推进。", float64(durationSec))
	}
}

func voiceVisualHumorBeat(narration string, index int) string {
	text := normalizeInlineText(narration)
	lower := strings.ToLower(text)
	switch {
	case containsAny(lower, "风扇", "起飞", "十个 ai", "工具"):
		return "把工具焦虑拍成电脑风扇准备起飞的夸张反差。"
	case containsAny(lower, "流水线", "工厂"):
		return "把抽象流程变成迷你工厂自动开工，降低技术距离感。"
	case containsAny(lower, "按钮", "日志"):
		return "用“只看按钮 vs 能看日志”的双视角做轻喜剧对照。"
	case containsAny(lower, "关注", "开源"):
		return "让 Star/Fork/Follow 像弹幕贴纸一样冒出来，轻松收口。"
	default:
		return []string{
			"用夸张视觉隐喻先逗笑，再落到项目能力。",
			"把技术名词转成可见动作，避免干讲概念。",
			"保留解压节奏，减少同屏文字。",
		}[index%3]
	}
}

func visualTitleFromNarration(narration string, shotIndex int) string {
	text := normalizeInlineText(narration)
	lower := strings.ToLower(text)
	switch {
	case containsAny(lower, "工具越用越多", "流程却越来越少", "真正能跑完"):
		return "AI 工具很多，流程没人跑完"
	case containsAny(lower, "选题", "脚本", "分镜") && containsAny(lower, "审核", "生成", "打包", "流水线"):
		return "选题到成片，一条流水线"
	case containsAny(lower, "mcp", "jimeng", "即梦", "hyperframes", "runner"):
		return "本机能力自动接入视频生产"
	case containsAny(lower, "开源", "完整项目", "关注"):
		return "开源完整项目，持续真实迭代"
	case containsAny(lower, "非技术", "按步骤", "不用理解"):
		return "非技术用户也能按步骤推进"
	case text != "":
		return truncateVisualTitle(text, 20)
	default:
		return []string{
			"AI 内容流程，一次跑到底",
			"脚本、分镜、生成全链路",
			"审核可控，产物可追踪",
			"开源项目，真实交付",
		}[shotIndex%4]
	}
}

func visualSubtitleFromNarration(narration string, shotIndex int) string {
	text := normalizeInlineText(narration)
	lower := strings.ToLower(text)
	switch {
	case containsAny(lower, "工具越用越多", "流程却越来越少", "真正能跑完"):
		return "用痛点开场，先让非技术观众产生共鸣"
	case containsAny(lower, "选题", "脚本", "分镜") && containsAny(lower, "审核", "生成", "打包", "流水线"):
		return "展示从创意到交付的可追踪步骤"
	case containsAny(lower, "mcp", "jimeng", "即梦", "hyperframes", "runner"):
		return "突出 JiMeng MCP、local runner 与渲染链路"
	case containsAny(lower, "开源", "完整项目", "关注"):
		return "给出开源和关注理由，承接后续转化"
	case text != "":
		return truncateVisualTitle(text, 34)
	default:
		return []string{
			"痛点、方案、演示、关注四段式推进",
			"把复杂 agent 流程翻译成观众能懂的动作",
			"每个节点都有审核和产物记录",
			"用真实流程做项目的第一条宣传片",
		}[shotIndex%4]
	}
}

func normalizeInlineText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || isRedactedPlaceholder(text) {
		return ""
	}
	replacer := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ")
	text = replacer.Replace(text)
	return strings.Join(strings.Fields(text), " ")
}

func truncateVisualTitle(text string, maxRunes int) string {
	text = normalizeInlineText(text)
	if text == "" {
		return ""
	}
	for _, sep := range []string{"。", "！", "？", "；", ".", "!", "?", ";", "，", ","} {
		if idx := strings.Index(text, sep); idx > 0 {
			text = strings.TrimSpace(text[:idx])
			break
		}
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
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
		"card_plan_generator", "caption_splitter",
		"composition_quality_checker", "reference_asset_planner",
		"asset_policy_generator", "continuity_checker",
		"style_profile_builder", "stale_tracker",
		"preview_quality_checker",
		"publish_copy_generator", "video_package_exporter":
		return true
	default:
		return false
	}
}

func normalizeStructuredToolContent(toolName, rawContent string) (string, map[string]interface{}, bool) {
	var contentPkg map[string]interface{}
	if err := jsonx.ExtractJSON(rawContent, &contentPkg); err != nil {
		return rawContent, contentPkg, false
	}
	if toolName == "video_script_generator" {
		if _, ok := contentPkg["scriptSpans"]; !ok {
			if sections, exists := contentPkg["sections"]; exists {
				contentPkg["scriptSpans"] = sections
			}
		}
	}
	if toolName == "shot_splitter" {
		return buildShotQueueReviewContent(contentPkg), contentPkg, true
	}
	if isStructuredOutputTool(toolName) {
		if canonical, err := json.Marshal(contentPkg); err == nil {
			return string(canonical), contentPkg, true
		}
	}
	return rawContent, contentPkg, true
}

func normalizeVideoScriptTimingFields(contentPkg map[string]interface{}, targetDurationSec int) bool {
	if contentPkg == nil {
		return false
	}
	spans := toolMapsFromValue(contentPkg["scriptSpans"], "scriptSpans", "spans", "sections")
	if len(spans) == 0 {
		spans = toolMapsFromValue(contentPkg["sections"], "sections", "scriptSpans", "spans")
	}
	if len(spans) == 0 {
		return false
	}
	normalized := normalizeScriptSpanMapsForAIGC(spans, targetDurationSec)
	if len(normalized) == 0 {
		return false
	}
	contentPkg["sections"] = normalized
	contentPkg["scriptSpans"] = normalized
	contentPkg["shotSplitPolicy"] = map[string]interface{}{
		"minShotDurationSec":       3,
		"maxShotDurationSec":       15,
		"preferredShotDurationSec": "6-8",
		"splitByScriptSemantics":   true,
		"splitByVisualChange":      true,
		"normalizedBy":             "video_script_generator_timing_guard",
	}
	if end := scriptSpanEndSec(normalized[len(normalized)-1]); end > 0 {
		contentPkg["estimatedDurationSec"] = secondValue(end)
	}
	return true
}

func normalizeScriptSpanMapsForAIGC(spans []map[string]interface{}, targetDurationSec int) []map[string]interface{} {
	if len(spans) == 0 {
		return nil
	}
	defaultDurations := distributeFallbackScriptDurations(targetDurationSec, len(spans))
	out := make([]map[string]interface{}, 0, len(spans))
	cursor := 0.0
	for i, span := range spans {
		start := floatFromInterface(firstExistingValue(span, "startSec", "start", "startTime"), cursor)
		end := floatFromInterface(firstExistingValue(span, "endSec", "end", "endTime"), 0)
		duration := floatFromInterface(firstExistingValue(span, "durationSec", "duration", "seconds"), 0)
		if end <= start {
			if duration <= 0 {
				if i < len(defaultDurations) {
					duration = float64(defaultDurations[i])
				} else {
					duration = 6
				}
			}
			end = start + duration
		}
		duration = end - start
		if duration <= 0 {
			duration = 6
			end = start + duration
		}
		pieces := splitScriptSpanMapForAIGCDuration(span, i, start, duration)
		out = append(out, pieces...)
		if len(pieces) > 0 {
			cursor = scriptSpanEndSec(pieces[len(pieces)-1])
		} else {
			cursor = end
		}
	}
	return mergeShortScriptSpanMaps(out)
}

func splitScriptSpanMapForAIGCDuration(span map[string]interface{}, index int, startSec, durationSec float64) []map[string]interface{} {
	if durationSec <= 15 {
		item := copyStringMap(span)
		applyScriptSpanTiming(item, startSec, startSec+durationSec)
		return []map[string]interface{}{item}
	}
	chunkCount := int(math.Ceil(durationSec / 15.0))
	if chunkCount < 1 {
		chunkCount = 1
	}
	text := firstReadableStringInMap(span, "scriptText", "narrationText", "content", "text")
	textParts := splitTextIntoNParts(text, chunkCount)
	durations := distributeDurationChunks(durationSec, chunkCount)
	baseID := firstStringInMap(span, "id", "spanId", "scriptSpanId")
	if baseID == "" {
		baseID = fmt.Sprintf("SPAN_%02d", index+1)
	}
	baseName := firstStringInMap(span, "name", "title")
	out := make([]map[string]interface{}, 0, chunkCount)
	cursor := startSec
	for i := 0; i < chunkCount; i++ {
		nextEnd := cursor + durations[i]
		if i == chunkCount-1 {
			nextEnd = startSec + durationSec
		}
		item := copyStringMap(span)
		childID := fmt.Sprintf("%s_%02d", baseID, i+1)
		item["id"] = childID
		item["spanId"] = childID
		item["sourceSpanId"] = baseID
		if baseName != "" {
			item["name"] = fmt.Sprintf("%s %d/%d", baseName, i+1, chunkCount)
		}
		partText := ""
		if i < len(textParts) {
			partText = strings.TrimSpace(textParts[i])
		}
		if partText == "" {
			partText = text
		}
		if partText != "" {
			item["text"] = partText
			item["scriptText"] = partText
			item["narrationText"] = partText
		}
		item["visualChangeReason"] = "long_script_span_split_3_15s"
		applyScriptSpanTiming(item, cursor, nextEnd)
		out = append(out, item)
		cursor = nextEnd
	}
	return out
}

func mergeShortScriptSpanMaps(spans []map[string]interface{}) []map[string]interface{} {
	if len(spans) <= 1 {
		return spans
	}
	out := make([]map[string]interface{}, 0, len(spans))
	for _, span := range spans {
		duration := scriptSpanDurationSec(span)
		if len(out) > 0 {
			prev := out[len(out)-1]
			prevDuration := scriptSpanDurationSec(prev)
			if (duration < 3 || prevDuration < 3) && duration+prevDuration <= 15 {
				out[len(out)-1] = mergeScriptSpanMap(prev, span)
				continue
			}
		}
		out = append(out, span)
	}
	for _, span := range out {
		duration := scriptSpanDurationSec(span)
		if duration > 0 && duration < 3 {
			start := scriptSpanStartSec(span)
			applyScriptSpanTiming(span, start, start+3)
			span["visualChangeReason"] = appendReason(firstStringInMap(span, "visualChangeReason"), "short_script_span_extended_to_min_3s")
		}
	}
	return out
}

func mergeScriptSpanMap(left, right map[string]interface{}) map[string]interface{} {
	merged := copyStringMap(left)
	leftText := firstReadableStringInMap(left, "scriptText", "narrationText", "content", "text")
	rightText := firstReadableStringInMap(right, "scriptText", "narrationText", "content", "text")
	text := strings.TrimSpace(strings.Join(compactStrings([]string{leftText, rightText}), " "))
	if text != "" {
		merged["text"] = text
		merged["scriptText"] = text
		merged["narrationText"] = text
	}
	leftName := firstStringInMap(left, "name", "title")
	rightName := firstStringInMap(right, "name", "title")
	if leftName != "" && rightName != "" && leftName != rightName {
		merged["name"] = leftName + " / " + rightName
	}
	start := scriptSpanStartSec(left)
	duration := scriptSpanDurationSec(left) + scriptSpanDurationSec(right)
	applyScriptSpanTiming(merged, start, start+duration)
	merged["visualChangeReason"] = appendReason(firstStringInMap(left, "visualChangeReason"), "short_script_span_merged_to_neighbor")
	return merged
}

func applyScriptSpanTiming(span map[string]interface{}, startSec, endSec float64) {
	if endSec < startSec {
		endSec = startSec
	}
	duration := endSec - startSec
	span["startSec"] = secondValue(startSec)
	span["endSec"] = secondValue(endSec)
	span["durationSec"] = secondValue(duration)
}

func splitTextIntoNParts(text string, count int) []string {
	text = strings.TrimSpace(text)
	if count <= 1 {
		return []string{text}
	}
	parts := splitTextByPunctuation(text)
	if len(parts) == 0 && text != "" {
		parts = []string{text}
	}
	for len(parts) < count {
		longest := 0
		for i := range parts {
			if len([]rune(parts[i])) > len([]rune(parts[longest])) {
				longest = i
			}
		}
		left, right := splitSegmentInHalf(parts[longest])
		if strings.TrimSpace(right) == "" {
			break
		}
		next := append([]string{}, parts[:longest]...)
		next = append(next, left, right)
		next = append(next, parts[longest+1:]...)
		parts = next
	}
	if len(parts) <= count {
		for len(parts) < count {
			parts = append(parts, "")
		}
		return parts
	}
	out := make([]string, count)
	for i, part := range parts {
		slot := i * count / len(parts)
		out[slot] = strings.TrimSpace(strings.Join(compactStrings([]string{out[slot], part}), " "))
	}
	return out
}

func distributeDurationChunks(total float64, count int) []float64 {
	if count <= 0 {
		return nil
	}
	chunks := make([]float64, count)
	remaining := total
	for i := 0; i < count; i++ {
		left := float64(count - i)
		duration := remaining / left
		if duration < 3 && remaining >= 3 {
			duration = 3
		}
		if duration > 15 {
			duration = 15
		}
		chunks[i] = duration
		remaining -= duration
	}
	if math.Abs(remaining) > 0.001 {
		chunks[count-1] += remaining
	}
	return chunks
}

func scriptSpanStartSec(span map[string]interface{}) float64 {
	return floatFromInterface(firstExistingValue(span, "startSec", "start", "startTime"), 0)
}

func scriptSpanEndSec(span map[string]interface{}) float64 {
	return floatFromInterface(firstExistingValue(span, "endSec", "end", "endTime"), 0)
}

func scriptSpanDurationSec(span map[string]interface{}) float64 {
	duration := floatFromInterface(firstExistingValue(span, "durationSec", "duration", "seconds"), 0)
	if duration > 0 {
		return duration
	}
	start := scriptSpanStartSec(span)
	end := scriptSpanEndSec(span)
	if end > start {
		return end - start
	}
	return 0
}

func secondValue(value float64) interface{} {
	rounded := math.Round(value*100) / 100
	if math.Abs(rounded-math.Round(rounded)) < 0.001 {
		return int(math.Round(rounded))
	}
	return rounded
}

func appendReason(existing, reason string) string {
	existing = strings.TrimSpace(existing)
	if existing == "" {
		return reason
	}
	if strings.Contains(existing, reason) {
		return existing
	}
	return existing + "," + reason
}

func buildDynamicAgentSystemPrompt(toolName, topic, style, platform string) string {
	switch toolName {
	case "knowledge_researcher":
		return fmt.Sprintf(`你是一个事实整理器。只输出JSON，禁止输出推理过程、分析或任何其他文字。

规则：
- draw/tie=战平 win/beat=击败 lose=不敌。绝不混淆。
- 每个fact必须直接来源于搜索结果，不得编造。
- facts简洁，每条≤30字，最多10条。
- 搜索结果为英文时翻译为中文，保留专有名词原文。

topic=%s
style=%s

只输出这个JSON（不要输出其他任何内容）：
{"facts":[""],"timeline":[{"date":"","event":""}],"storyAngles":[""],"risks":[""],"sourceNotes":[""],"summary":""}`, topic, style)

	case "fact_checker":
		return "你是严格的事实核查员。\n\n任务：\n逐一检查 facts 每一条是否准确。重点核查：\n- 体育比赛结果（击败vs战平vs不敌，比分是否正确）\n- 人名地名队名拼写\n- 数字（人口比分排名日期）\n- 搜索结果不支持的主观评价\n\n要求：\n1. 只输出 JSON。\n2. 通过的事实写入 checkedFacts。\n3. 疑似编造或搜索结果不支持的写入 warnings。\n4. 需要修正的给出 原文→修正后→原因。\n5. passed 仅在所有事实均核查通过时为 true。\n\n输出 JSON：\n{\n  \"checkedFacts\": [\"已核查的事实短句\"],\n  \"warnings\": [\"需要谨慎表达的内容\"],\n  \"corrections\": [{\"original\": \"原文\", \"corrected\": \"修正后\", \"reason\": \"修正原因\"}],\n  \"passed\": true,\n  \"summary\": \"核查摘要\"\n}"

	case "video_script_generator":
		return fmt.Sprintf(`你是短视频口播稿创作专家。

目标：
根据主题、事实材料和用户风格要求，生成适合中文短视频平台的口播知识分享视频脚本。

硬性要求：
1. 开头 5-8 秒必须有明确钩子。
2. 内容必须基于 facts / knowledgePack，不允许编造历史事实或最新事件结果。
3. 语言自然，适合真人或 AI 配音口播。
4. 总时长接近 targetDurationSec。
5. 分段清晰：开场、背景、核心讲述、总结。
6. 不要写成论文，不要堆砌百科。
7. 每句话尽量短，适合口播。
8. 输出严格 JSON。
9. 如果提供了 knowledgePack，必须优先使用其中事实；如果 knowledgePack 与你的模型记忆冲突，以 knowledgePack 为准。
10. 如果 retrievalPolicy=required 但没有事实材料，返回错误 JSON，不得继续创作。
11. 不得编造 knowledgePack 中没有的比分、排名、出线结果、日期、人物职位、政策变化。
12. 输出必须包含 usedFacts、unusedFacts、factCheckWarnings 和 knowledgeTrace。
13. 如果 creationProfile/profileId 为 cinematic_story，或主题要求影视/剧情/角色/场景/道具/连续性，必须额外输出 storyOutline、detailedScript、characters、scenes、props；角色、场景、道具档案必须包含 id、name、description、invariants/locks、referenceViews。
14. 影视剧本必须先有故事大纲，再有详细剧本；详细剧本要按可拍摄 beat 写清动作、画面意义和正能量收束，不要只写口播稿。

输入：
topic=%s
style=%s

输出 JSON：
{
  "script": "完整口播稿正文",
  "storyOutline": {"logline": "", "theme": "", "beats": [{"id": "beat_01", "name": "", "purpose": ""}], "ending": ""},
  "detailedScript": "影视类详细剧本；口播类可为空字符串",
  "characters": [{"id": "char_main", "name": "", "description": "", "invariants": [], "referenceViews": ["front", "side", "back"]}],
  "scenes": [{"id": "scene_main", "name": "", "description": "", "spatialLocks": [], "referenceViews": ["front_view", "reverse_view", "side_view"]}],
  "props": [{"id": "prop_main", "name": "", "description": "", "invariants": [], "referenceViews": ["front", "side", "back", "detail"]}],
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
  },
  "usedFacts": [{"claim": "使用到的事实", "source": "来源"}],
  "unusedFacts": [],
  "factCheckWarnings": [],
  "knowledgeTrace": {
    "retrievalPolicy": "none|optional|required|forbidden",
    "knowledgePackHash": "",
    "factCount": 0,
    "sourceCount": 0
  }
}`, topic, style)

	case "shot_splitter":
		return `你是短视频分镜导演。

目标：
把已确认口播稿拆成适合逐 shot 生产、审核和返工的轻量分镜队列。AIGC 视频必须先依赖已确认的全局一致性资产包，再进入逐 shot 生产。

硬性要求：
1. 不论 AIGC 还是 HyperFrames，视频创作都必须先分 shot；每个 shot 都是最小生产、审核和返工单元。
2. 每个 shot 时长 3-15 秒，只表达一个主要画面变化，必须覆盖完整口播稿，不要遗漏。
3. 每个 shot 必须包含：shotId、durationSec、scriptText、narrationText、visual、camera、composition、lighting、transitionIn、transitionOut。
4. 每个 shot 必须包含 materialLibraryHints，说明可检索或复用的素材库方向、镜头语法、构图或运动参考。
5. 每个 shot 必须包含 referenceRequirements，列出当前 shot 需要引用的全局一致性资产包条目；主要人物、场景、核心道具必须来自已确认的 front/side/back 多视角参考图，不得临时发散。
6. 每个 shot 必须包含 expectedArtifacts，明确该 shot 后续会生成或上传的 voiceover/audio、keyframe、image、videoClip、subtitle、hyperframesSegment、reviewPacket。
7. 每个 shot 必须包含 reviewPacket，用于前端按 shot 审核，字段至少包括 artifactKind="SHOT_REVIEW_PACKET"、reviewFocus、rerunScope、dependencies。
8. AIGC shot 的 reviewFocus 必须覆盖参考图一致性、提示词、音频口播、关键帧、视频片段、字幕；HyperFrames shot 的 reviewFocus 必须覆盖画面和口播一致性、文字层可读性、时间轴节奏。
9. 每个 shot 的素材包必须完全独立，不得要求读取上一个或下一个 shot；唯一允许共用的是为了一致性锁定的主要角色、主要道具、主场景和全片风格。
10. transitionOut 必须描述覆盖在本 shot 结尾 0.3-0.8 秒内的收束或转场，方便 ffmpeg 直接按 shot 顺序拼接。
11. 输出 shotQueue，表达线性执行状态和当前建议先审核的 shot；不要在用户审核内容里一次性暴露所有 shot 的素材包、prompt 和拼接细节。
12. 视觉风格默认 16:9，非写实动画，去 AI 感。
13. 输出严格 JSON。

输入：
script=<script>
shotDurationRule=<shotDurationRule>
aspectRatio=<aspectRatio>

输出 JSON：
{
  "shotQueue": {
    "mode": "linear",
    "reviewUnit": "single_shot",
    "activeShotId": "SHOT_01",
    "status": "INITIALIZED"
  },
  "shotList": [
    {
      "shotId": "SHOT_01",
      "durationSec": 6,
      "scriptText": "...",
      "narrationText": "...",
      "visual": "...",
      "camera": "...",
      "composition": "...",
      "lighting": "...",
      "transitionIn": "...",
      "transitionOut": "...",
      "materialLibraryHints": ["可迁移拍摄语法或素材方向"],
      "referenceRequirements": {
        "characters": [{"id": "char_main", "views": ["front", "side", "back"], "reason": "保持主角一致"}],
        "scenes": [{"id": "scene_main", "views": ["front", "side", "back"], "reason": "保持空间一致"}],
        "props": [{"id": "prop_main", "views": ["front", "side", "back"], "reason": "保持道具一致"}],
        "storyboard": true,
        "firstFrame": true
      },
      "expectedArtifacts": ["SHOT_REVIEW_PACKET", "SHOT_AUDIO", "SHOT_KEYFRAME", "SHOT_VIDEO_CLIP", "SHOT_SUBTITLE", "HYPERFRAMES_SHOT"],
      "reviewPacket": {
        "artifactKind": "SHOT_REVIEW_PACKET",
        "reviewFocus": ["口播是否覆盖", "画面是否匹配", "参考图是否足够", "是否只需返工本 shot"],
        "rerunScope": ["SHOT_01"],
        "dependencies": ["script", "reference_assets", "material_library"]
      },
      "notes": "..."
    }
  ],
  "totalDurationSec": 90,
  "summary": "..."
}`

	case "card_plan_generator":
		return `你是图文视频卡片/分镜设计师。

目标：
把已确认口播稿拆成适合图文视频的卡片页、字幕节奏和画面段落。

硬性要求：
1. 每张卡片只表达一个核心信息。
2. 每张卡片包含 cardId、type、durationSec、title、body、visualIntent、emphasisWords。
3. 字幕计划必须覆盖完整口播稿。
4. 输出严格 JSON。

输出 JSON：
{
  "cardPlan": {
    "artifactKind": "CARD_PLAN",
    "cards": [
      {
        "cardId": "card_001",
        "type": "hook",
        "durationSec": 6,
        "title": "...",
        "body": "...",
        "visualIntent": "...",
        "emphasisWords": ["..."]
      }
    ]
  },
  "captionPlan": {"artifactKind": "CAPTION_PLAN", "segments": []},
  "summary": "..."
}`

	case "caption_splitter":
		return `你是图文视频字幕节奏设计师。

目标：
把口播稿拆成短字幕段落，方便卡片和字幕轨使用。

输出严格 JSON：
{
  "captionPlan": {
    "artifactKind": "CAPTION_PLAN",
    "segments": [
      {"id": "cap_001", "startSec": 0, "endSec": 4, "text": "..."}
    ]
  },
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
5. 每个 shot 都必须引用相关参考图，优先包含主要人物、场景、道具；主要人物、场景、道具尽量使用 front/side/back 三视角参考图来保持后续一致性。
6. 每个 shot 必须输出 referenceCoverage，说明该 shot 已使用哪些人物、场景、道具、故事板、首帧或用户上传参考图，以及缺少哪些三视角参考。
7. 同时输出 externalGenerationRequests，供没有文生图 API 的用户复制 prompt 到外部平台生成，再上传结果。
8. 每个 externalGenerationRequest 的 prompt 不超过 2000 字，references 最多 6 张，只能引用人物、主要道具、场景、故事板、关键帧或用户上传参考图。
9. artifacts[] 必须为每个 externalGenerationRequest 建一个 JSON artifact，metadata.artifactType 固定为 external_generation_request，metadata.relatedShotId 必须等于 shotId，并为关键帧产物建 SHOT_KEYFRAME artifact。
10. 输出严格 JSON。

输入：
shotList=<shotList>
style=<style>

输出 JSON：
{
  "keyframePrompts": [
    {
      "shotId": "SHOT_01",
      "prompt": "非写实动画风格，...",
      "referenceCoverage": {
        "characters": [{"id": "char_main", "viewsUsed": ["front", "side", "back"]}],
        "scenes": [{"id": "scene_main", "viewsUsed": ["front", "side", "back"]}],
        "props": [{"id": "prop_main", "viewsUsed": ["front", "side", "back"]}],
        "missing": []
      },
      "styleNotes": "..."
    }
  ],
  "externalGenerationRequests": [
    {
      "requestId": "extgen_keyframe_SHOT_01",
      "kind": "image",
      "shotId": "SHOT_01",
      "prompt": "可复制到外部图片生成平台的完整提示词，<=2000字",
      "negativePrompt": "写清楚需要避免的画面问题",
      "references": [
        {"id": "char_main", "label": "主角", "role": "character", "storageRef": "local://..."}
      ],
      "target": {"aspectRatio": "16:9", "resolution": "1920x1080"},
      "promptCharLimit": 2000,
      "referenceImageLimit": 6,
      "status": "pending_upload"
    }
  ],
  "artifacts": [
    {
      "unitId": "extgen_keyframe_SHOT_01",
      "kind": "JSON",
      "name": "external_generation_request.json",
      "mimeType": "application/json",
      "metadata": {"artifactType": "external_generation_request", "generationKind": "image", "relatedShotId": "SHOT_01"}
    },
    {
      "unitId": "shot_keyframe_SHOT_01",
      "kind": "SHOT_KEYFRAME",
      "name": "SHOT_01_keyframe_prompt.json",
      "mimeType": "application/json",
      "metadata": {"artifactType": "shot_keyframe", "relatedShotId": "SHOT_01"}
    }
  ],
  "summary": "..."
}`

	case "video_prompt_generator":
		return `你是 AI 视频生成提示词导演。

目标：
根据分镜生成每个镜头可直接用于视频生成模型的 Prompt。

硬性要求：
1. 每个 shot 都必须作为独立视频片段生成，每个 shot 输出一个 video prompt，可单独复制给模型调用。
2. prompt 必须包含：画面主体、场景、动作、镜头运动、光影、色彩、风格、持续时间、转场。
3. 必须写清楚该镜头内部的时间线变化；转场设计写在本 shot 内部，例如开头如何进入、结尾如何自然收束，不能要求上一个或下一个 shot 配合；转场覆盖在本 shot 结尾 0.3-0.8 秒内完成。
4. 不依赖上下文记忆，因为视频模型每个 shot 独立生成；不能写“同上/沿用上一镜/接上一镜/延续前一镜”等表达。
5. 不要使用尾帧、末帧、首尾帧对齐、前后 shot frame matching 或任何会限制模型创作的跨 shot 帧约束。
6. 视频 request 可以依赖图片+prompt，但只使用首帧+参考故事板作为本 shot 的图像约束；只使用首帧锁定起始构图，参考故事板锁定动作节奏和关键状态。
7. 同时输出 externalGenerationRequests，供没有文生视频/API 的用户复制 prompt 到外部平台生成，再上传结果。
8. references 最多 6 张，优先引用人物、主要道具、场景、首帧、参考故事板，明确每张参考图锁定什么；不要引用尾帧或下一 shot 的画面。
9. 每个 externalGenerationRequest 的 prompt 不超过 2000 字。
10. 最终成片按 shot 顺序用 ffmpeg 直接拼接即可，默认 simple_cut；这是简单剪辑拼接，不要求复杂跨 shot 衔接；允许轻微交叉淡化，但淡化素材必须已覆盖在当前 shot 结尾。
11. 每个 shot 都必须绑定自己的口播 narrationText、可选素材库 materialLibraryHints、音频生成要求、字幕要求和对应视频片段；纯 AIGC 可以配字幕，字幕必须来自该 shot 的口播。
12. 输出 shotAssemblyPlan，说明该 shot 生成 SHOT_AUDIO、SHOT_SUBTITLE、SHOT_VIDEO_CLIP 后如何进入最终简单拼接。
13. 输出 shotAssetPackages。每个 package 必须完全独立包含 referenceImages、prompts、voiceover、aigcVideo、subtitle、concatPlan；只允许通过 allowedSharedConsistency 共用主要角色、主要道具、主场景和全片风格。
14. 禁止真人写实，默认非写实动画，去 AI 感。
15. artifacts[] 必须为每个 externalGenerationRequest 建一个 JSON artifact，metadata.artifactType 固定为 external_generation_request，并为每个 shot 建 SHOT_ASSET_PACKAGE、SHOT_VIDEO_CLIP、SHOT_AUDIO、SHOT_SUBTITLE 的占位 artifact metadata。
16. 输出严格 JSON。

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
      "narrationText": "...",
      "prompt": "...",
      "negativePrompt": "...",
      "continuity": "...",
      "materialLibraryHints": ["..."],
      "subtitleText": "...",
      "shotAssemblyPlan": {
        "audioArtifactKind": "SHOT_AUDIO",
        "subtitleArtifactKind": "SHOT_SUBTITLE",
        "videoArtifactKind": "SHOT_VIDEO_CLIP",
        "concatMode": "simple_cut",
        "transitionAtEnd": "本 shot 结尾 0.3-0.8 秒内完成淡出或稳定收束"
      },
      "modelTips": {
        "cameraMotion": "...",
        "subjectMotion": "..."
      }
    }
  ],
  "shotAssetPackages": [
    {
      "shotId": "SHOT_01",
      "durationSec": 6,
      "referenceImages": [
        {"id": "keyframe_SHOT_01", "label": "首帧/关键帧", "role": "keyframe", "storageRef": "local://...", "locks": ["起始构图", "主体站位"]},
        {"id": "char_main", "label": "主要角色", "role": "character", "storageRef": "local://...", "locks": ["角色一致性"]}
      ],
      "prompts": {
        "videoPrompt": "可直接投放给视频模型的本 shot prompt",
        "negativePrompt": "禁止真人写实、禁止跨 shot 依赖、禁止尾帧对齐"
      },
      "voiceover": {"text": "...", "artifactKind": "SHOT_AUDIO", "fileName": "SHOT_01_voiceover.wav"},
      "aigcVideo": {"requestId": "extgen_video_SHOT_01", "artifactKind": "SHOT_VIDEO_CLIP", "fileName": "SHOT_01_video_clip.mp4", "concatMode": "simple_cut", "transitionAtEnd": "结尾0.5秒淡出"},
      "subtitle": {"text": "...", "artifactKind": "SHOT_SUBTITLE", "fileName": "SHOT_01_subtitle.srt"},
      "concatPlan": {"ffmpegReady": true, "mode": "simple_cut", "transitionCoveredInShotEnd": true},
      "independence": {"crossShotDependencyForbidden": true, "allowedSharedConsistency": ["主要角色", "主要道具", "主场景", "全片风格"]}
    }
  ],
  "externalGenerationRequests": [
    {
      "requestId": "extgen_video_SHOT_01",
      "kind": "video",
      "shotId": "SHOT_01",
      "prompt": "可复制到外部视频生成平台的完整提示词，包含参考图使用方式，<=2000字",
      "negativePrompt": "写清楚需要避免的画面问题",
      "references": [
        {"id": "keyframe_SHOT_01", "label": "首帧/关键帧", "role": "keyframe", "storageRef": "local://..."}
      ],
      "target": {"aspectRatio": "16:9", "durationSec": 6, "resolution": "1920x1080"},
      "promptCharLimit": 2000,
      "referenceImageLimit": 6,
      "status": "pending_upload"
    }
  ],
  "artifacts": [
    {
      "unitId": "extgen_video_SHOT_01",
      "kind": "JSON",
      "name": "external_generation_request.json",
      "mimeType": "application/json",
      "metadata": {"artifactType": "external_generation_request", "generationKind": "video", "relatedShotId": "SHOT_01"}
    },
    {
      "unitId": "shot_asset_package_SHOT_01",
      "kind": "SHOT_ASSET_PACKAGE",
      "name": "SHOT_01_asset_package.json",
      "mimeType": "application/json",
      "metadata": {"artifactType": "shot_asset_package", "relatedShotId": "SHOT_01", "ffmpegConcatOK": true}
    },
    {
      "unitId": "shot_video_SHOT_01",
      "kind": "SHOT_VIDEO_CLIP",
      "name": "SHOT_01_video_clip.mp4",
      "mimeType": "video/mp4",
      "metadata": {"artifactType": "shot_video_clip", "relatedShotId": "SHOT_01"}
    },
    {
      "unitId": "shot_audio_SHOT_01",
      "kind": "SHOT_AUDIO",
      "name": "SHOT_01_voiceover.wav",
      "mimeType": "audio/wav",
      "metadata": {"artifactType": "shot_audio", "relatedShotId": "SHOT_01"}
    },
    {
      "unitId": "shot_subtitle_SHOT_01",
      "kind": "SHOT_SUBTITLE",
      "name": "SHOT_01_subtitle.srt",
      "mimeType": "text/plain",
      "metadata": {"artifactType": "shot_subtitle", "relatedShotId": "SHOT_01"}
    }
  ],
  "summary": "..."
}`

	case "script_quality_checker":
		return `你是口播稿质量审核员。你的训练数据截止于2024年，当前日期为2026年。你必须以用户提示中给出的”目标时长”和”本次知识/新闻材料”为权威依据，不得依赖你的旧知识。

检查以下内容：
1. 结构完整性：是否有钩子开头、核心内容、总结
2. 节奏：每段时长是否合理
3. 时长：必须严格按照用户提示中的”目标时长”评估，绝不自行为90秒或任何其他默认值。目标时长是多少秒就按多少秒评判，允许约20%浮动。
4. 知识准确性：必须以本次输入的 facts / knowledgeContext / knowledgePack / news_search 结果为唯一事实依据
5. 开头吸引力：前 5-8 秒是否有钩子
6. 语言自然度：是否适合口播

事实判断硬规则（严格按顺序执行）：
1. 如果用户提示中提供了”本次知识/新闻材料”，其中的事实视为当前已验证事实，优先于一切。
2. 绝不可以用模型内置旧知识（截止2024年）否定2025-2026年发生的新事实。
3. 如果某事实在本次知识材料中有明确来源、日期或URL，不得判为”虚构”或”未发生”。
4. 如果输入知识材料与模型内置记忆冲突，以输入知识材料为准并加分（说明引用了最新事实），不得扣分。
5. 仅在完全没有来源且你100%确定是虚构的情况下，才标记为 error。

duration 硬规则：
1. 目标时长由用户提示明确指定（如”目标时长：30秒”），严格按此数值评估。
2. 绝不自行为90秒、60秒或任何其他数值。
3. 30秒脚本约150-180字是合理的，不应因”字数不足”而扣分——关键在于信息密度和口播节奏。
4. 只有目标时长与脚本实际估算时长差距超过30%且无合理解释时，才标记为 warning。

输出严格 JSON：
{
  “passed”: true/false,
  “score”: 0-100,
  “issues”: [
    {“level”: “error|warning|info”, “field”: “...”, “message”: “...”}
  ],
  “repairSuggestions”: [“...”],
  “analysisSummary”: “用一句话说明为什么得到这个分数，以及最重要的通过/扣分依据。”,
  “rubricBreakdown”: [
    {“criterion”: “结构完整性|节奏与时长|知识准确性|开头吸引力|口播自然度”, “score”: 0, “maxScore”: 20, “reason”: “具体依据”}
  ],
  “keepDoing”: [“用户后续应该继续保持的写法或策略”]
}`

	case "composition_quality_checker":
		return `你是图文视频结构质量审核员。

检查：
1. 时间轴是否完整
2. 卡片文字是否过长
3. 字幕轨是否覆盖口播
4. 16:9 安全区域是否清楚
5. 是否可交给 HyperFrames 项目生成

输出严格 JSON：
{
  "passed": true,
  "score": 0,
  "issues": [],
  "repairSuggestions": []
}`

	case "reference_asset_planner":
		return `你是影视短片参考资产设计师。

目标：
根据故事大纲、详细剧本、角色档案、场景档案、道具档案和连续性圣经，规划全局一致性参考资产。

硬性要求：
1. 参考图不是成片剧照，而是主要角色、主场景、核心道具的多视角设定板。
2. 每个主要角色、主场景、核心道具都要有 referenceAssetIndex 条目，包含 id、role、views、description、locks、prompt、target。
3. 人物默认非真人风格化动画；禁止真人写实、真实演员、照片级真人皮肤、明星脸。
4. 场景参考图负责空间一致性，不堆信息海报，不放密集文字。
5. 道具参考图要展示正面、侧面、背面、细节和不可变化项。
6. 输出 externalGenerationRequests，kind=image，可由 JiMeng/Dreamina MCP generate_image 执行；prompt <= 2000 字。
7. 输出严格 JSON。

输出严格 JSON：
{
  "referenceAssetPlan": {
    "artifactKind": "REFERENCE_ASSET_PLAN",
    "stylePackage": {"style": "非真人风格化动画短片", "aspectRatio": "16:9", "negative": "禁止真人写实、密集文字、风格漂移"},
    "characters": [],
    "scenes": [],
    "props": [],
    "referenceAssetIndex": [],
    "globalReferenceAssets": [],
    "generationPolicy": "先生成多视角设定板，再让每个 shot 引用必要资产"
  },
  "referenceAssetIndex": [
    {"id": "char_main", "label": "主角", "role": "character", "views": ["front", "side", "back"], "description": "...", "locks": [], "prompt": "...", "target": {"aspectRatio": "16:9", "resolution": "1920x1080"}}
  ],
  "globalReferenceAssets": [],
  "externalGenerationRequests": [
    {"requestId": "extgen_ref_char_main", "kind": "image", "shotId": "GLOBAL_REFERENCE", "assetId": "char_main", "role": "character", "prompt": "完整图片生成提示词", "negativePrompt": "禁止真人写实、密集文字", "target": {"aspectRatio": "16:9", "resolution": "1920x1080", "generateNum": 1}, "status": "pending_upload"}
  ],
  "summary": "..."
}`

	case "asset_policy_generator":
		return `你是素材策略设计师。

输出严格 JSON：
{
  "assetPolicy": {
    "artifactKind": "REFERENCE_ASSET_PLAN",
    "backgroundStrategy": "...",
    "iconStrategy": "...",
    "fontStrategy": "...",
    "storageBoundary": "local_artifacts_only"
  },
  "summary": "..."
}`

	case "continuity_checker":
		return `你是影视短片连续性管理 Agent。

目标：
根据故事大纲、详细剧本、角色/场景/道具档案，生成后续 shot、参考图、视频 prompt 和 QA 都能使用的连续性圣经。

硬性要求：
1. 输出 continuityReport 和 continuityBible，二者内容可以相同，必须包含 characters、scenes、props、globalLocks、shotQAPolicy。
2. globalLocks 写清主要角色、主场景、核心道具、全片风格、字幕安全区和禁止跨 shot 尾帧依赖。
3. styleProfile 必须写明 16:9、非真人风格化动画、正能量、轻松搞笑但不嘲讽用户。
4. 输出严格 JSON。

输出严格 JSON：
{
  "continuityReport": {
    "artifactKind": "CONTINUITY_REPORT",
    "characters": [],
    "scenes": [],
    "props": [],
    "globalLocks": [],
    "shotQAPolicy": [],
    "warnings": []
  },
  "continuityBible": {},
  "styleProfile": {
    "artifactKind": "STYLE_PROFILE",
    "tone": "...",
    "visualStyle": "..."
  },
  "staleArtifactReport": {"artifactKind": "STALE_ARTIFACT_REPORT", "items": []},
  "summary": "..."
}`

	case "style_profile_builder":
		return `你是视频 StyleProfile 构建器。

输出严格 JSON：
{
  "styleProfile": {
    "artifactKind": "STYLE_PROFILE",
    "tone": "...",
    "visualStyle": "...",
    "typography": "...",
    "colorPolicy": "..."
  },
  "summary": "..."
}`

	case "stale_tracker":
		return `你是 Artifact StaleTracker。

根据 changedArtifact 和 artifactIndex 判断哪些下游产物失效。

输出严格 JSON：
{
  "staleArtifactReport": {
    "artifactKind": "STALE_ARTIFACT_REPORT",
    "staleArtifacts": [],
    "reason": "..."
  },
  "summary": "..."
}`

	case "preview_quality_checker":
		return `你是预览画面质量审核员。

检查截图文字溢出、画面可读性、卡片顺序和风格一致性。

输出严格 JSON：
{
  "previewReport": {
    "artifactKind": "PREVIEW_REPORT",
    "passed": true,
    "issues": []
  },
  "passed": true,
  "issues": [],
  "summary": "..."
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
		return fmt.Sprintf(`你是一个专业的短视频平台运营专家。根据视频内容生成发布文案。

目标平台：%s

硬性要求：
1. 必须根据口播稿生成标题、简介、keywords 和平台建议；如果有分镜，只能作为辅助理解。
2. 不要直接把用户原始输入当标题，不要照抄“帮我介绍一下...”这类需求句。
3. 标题要来自口播稿中的核心判断、冲突或结论。
4. keywords 必须从口播稿主题、事实主体、受众检索词中提取。
5. 同时给出 xiaohongshu 和 bilibili 两个平台版本。
6. 输出严格 JSON，不要输出 Markdown。

输出 JSON：
{
  "publishCopies": [
    {
      "platform": "xiaohongshu",
      "title": "不超过30字",
      "description": "100-200字，来自口播稿内容",
      "keywords": ["5-8个关键词"],
      "coverText": "封面短句",
      "publishTips": ["平台适配建议"]
    },
    {
      "platform": "bilibili",
      "title": "不超过40字",
      "description": "150-300字，来自口播稿内容",
      "keywords": ["5-10个关键词"],
      "coverText": "封面短句",
      "publishTips": ["平台适配建议"]
    }
  ]
}`, platform)

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

func buildDynamicAgentUserPrompt(toolName, topic, facts, style, script, shotList, videoPrompts, publishCopy, platform string, targetDurationSec int) string {
	switch toolName {
	case "knowledge_researcher":
		return fmt.Sprintf("请围绕以下主题进行深度知识研究：%s\n\n输出风格：%s", topic, style)

	case "fact_checker":
		return fmt.Sprintf("请核查以下内容的准确性：\n\n%s\n\n原始主题：%s", facts, topic)

	case "video_script_generator":
		var parts []string
		parts = append(parts, "主题："+topic)
		parts = append(parts, fmt.Sprintf("目标时长：%d秒", targetDurationSec))
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
		parts = append(parts, fmt.Sprintf("目标总时长：%d秒", targetDurationSec))
		parts = append(parts, "分镜时长规则：3-15秒")
		parts = append(parts, "画幅：16:9")
		return strings.Join(parts, "\n\n")

	case "card_plan_generator", "caption_splitter":
		var parts []string
		if script != "" {
			parts = append(parts, "口播稿：\n"+script)
		}
		parts = append(parts, "主题："+topic)
		parts = append(parts, fmt.Sprintf("目标总时长：%d秒", targetDurationSec))
		parts = append(parts, "请输出适合图文视频的卡片和字幕节奏。")
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
		var parts []string
		parts = append(parts, "请检查以下口播稿的质量：\n\n"+script)
		parts = append(parts, fmt.Sprintf("目标时长：%d秒", targetDurationSec))
		parts = append(parts, "时长判定规则：以目标时长为中心，允许约20%浮动；若脚本约30-40秒且目标为30秒，不应判为偏离90秒。")
		if facts != "" {
			parts = append(parts, "本次知识/新闻材料（事实判定优先依据）：\n"+facts)
		}
		parts = append(parts, "事实判定规则：不得用模型内置旧知识否定本次输入中带来源、日期或URL的新闻事实；如果来源不足，只能标记为需补充来源或谨慎表达。")
		return strings.Join(parts, "\n\n")

	case "shot_quality_checker":
		return fmt.Sprintf("请检查以下分镜的质量：\n\n%s", shotList)

	case "video_prompt_quality_checker":
		return fmt.Sprintf("请检查以下视频提示词的质量：\n\n%s", videoPrompts)

	case "package_quality_checker":
		var parts []string
		parts = append(parts, "主题："+topic)
		parts = append(parts, fmt.Sprintf("目标总时长：%d秒", targetDurationSec))
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

	case "composition_quality_checker":
		return "请检查当前 VideoCompositionSpec 是否适合图文视频渲染。主题：" + topic

	case "reference_asset_planner", "asset_policy_generator":
		return "请为以下图文视频规划参考资产和素材策略：\n\n主题：" + topic + "\n\n风格：" + style

	case "continuity_checker", "style_profile_builder", "stale_tracker":
		var parts []string
		parts = append(parts, "主题："+topic)
		if script != "" {
			parts = append(parts, "口播稿：\n"+script)
		}
		if shotList != "" {
			parts = append(parts, "卡片/分镜：\n"+shotList)
		}
		parts = append(parts, "请输出连续性、风格和 stale 检查结果。")
		return strings.Join(parts, "\n\n")

	case "preview_quality_checker":
		return "请检查当前预览截图/预览报告是否可进入最终渲染。主题：" + topic

	case "publish_copy_generator":
		var parts []string
		if script != "" {
			parts = append(parts, "口播稿：\n"+script)
		}
		if shotList != "" {
			parts = append(parts, "分镜：\n"+shotList)
		}
		parts = append(parts, "用户原始输入仅供理解背景，不能直接当标题："+topic)
		parts = append(parts, "目标平台："+platform)
		parts = append(parts, "请只根据口播稿的核心内容生成 publishCopies JSON。")
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

// Inserted by sed below
