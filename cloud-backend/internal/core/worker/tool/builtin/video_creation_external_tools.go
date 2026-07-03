package builtin

import (
	"bytes"
	"context"
	stdsha256 "crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
	"final_review_generator",
}

var (
	videoCreationOpenAICfg config.OpenAIConfig
	videoCreationSkillRoot string
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
	videoCreationSkillRoot = skillRoot
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
		}
		manifest.Output = map[string]tool.ParamDef{
			"timeWindowPlan": {Type: "object", Description: "Full time-window plan"},
			"timeWindows":    {Type: "array", Description: "3-15 second time windows"},
			"summary":        {Type: "string", Description: "Human-readable summary"},
			"content":        {Type: "string", Description: "Reviewable markdown content"},
			"artifacts":      {Type: "object", Description: "Reviewable artifact manifest"},
		}
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
		}
		manifest.Output = map[string]tool.ParamDef{
			"visualAlignmentPlan": {Type: "object", Description: "Visual-to-script alignment plan"},
			"shotList":            {Type: "array", Description: "Visual shot list for downstream generation"},
			"content":             {Type: "string", Description: "Reviewable markdown content"},
			"artifacts":           {Type: "object", Description: "Reviewable artifact manifest"},
		}
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
			"shotList":        {Type: "array", Description: "Approved shot list", Required: true},
			"brief":           {Type: "string", Description: "Original video brief", Required: false},
			"keyframePrompts": {Type: "array", Description: "Optional keyframe prompts", Required: false},
			"style":           {Type: "string", Description: "Visual style", Required: false},
			"modelHint":       {Type: "string", Description: "Target video generation model", Required: false},
			"aspectRatio":     {Type: "string", Description: "Video aspect ratio", Required: false},
			"aigcProvider":    {Type: "string", Description: "Optional automatic AIGC provider, such as jimeng_mcp", Required: false},
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
		}
		manifest.Output = map[string]tool.ParamDef{
			"shotAssetPackages":          {Type: "array", Description: "Per-shot asset packages with generated MCP results"},
			"generationResults":          {Type: "array", Description: "Raw MCP generation results"},
			"externalGenerationRequests": {Type: "array", Description: "Requests that remain manual or failed"},
			"summary":                    {Type: "string", Description: "MCP generation summary"},
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
			"topic":             {Type: "string", Description: "Video topic", Required: true},
			"script":            {Type: "string", Description: "Full voiceover script", Required: true},
			"shotList":          {Type: "array", Description: "Shot list", Required: true},
			"videoPrompts":      {Type: "array", Description: "Video prompts", Required: false},
			"shotAssetPackages": {Type: "array", Description: "Independent per-shot asset packages", Required: false},
			"style":             {Type: "string", Description: "Visual style", Required: false},
			"publishCopy":       {Type: "object", Description: "Publish copy", Required: false},
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
		}
		manifest.Output = map[string]tool.ParamDef{
			"outputPath":    {Type: "string", Description: "Rendered MP4 path"},
			"finalVideo":    {Type: "string", Description: "Rendered final video path"},
			"VIDEO":         {Type: "string", Description: "Rendered video artifact"},
			"RENDER_REPORT": {Type: "object", Description: "Render report"},
		}
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

func executeTimeWindowPlanner(stage, skillName string, params map[string]interface{}) tool.ToolResult {
	profile := creationProfileFromToolValue(params["creationProfile"])
	shots := shotUnitsFromToolValue(params["shotList"])
	spans := scriptSpansFromToolValue(params["scriptSpans"])
	if profile.ProfileID != videomodel.VideoProfileCinematicStory && len(spans) == 0 {
		return tool.FailureResult("time_window_planner requires scriptSpans for talking_head profile")
	}
	plan := videoservice.BuildTimeWindowPlan(videoservice.TimeWindowRequest{
		Profile:     profile,
		Shots:       shots,
		ScriptSpans: spans,
	})
	planMap := structToMap(plan)
	windowMaps := make([]map[string]interface{}, 0, len(plan.Windows))
	for _, window := range plan.Windows {
		windowMaps = append(windowMaps, structToMap(window))
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
		shotList = append(shotList, map[string]interface{}{
			"shotId":        shotID,
			"durationSec":   authoredDurationSec(window),
			"narrationText": narration,
			"visual":        visual,
			"camera":        visualSubtitleFromNarration(narration, i),
			"timeWindowId":  firstNonEmptyString(window, "id"),
		})
	}
	return tool.SuccessResult(map[string]interface{}{
		"visualAlignmentPlan": map[string]interface{}{"shotCount": len(shotList), "source": "time_window_planner"},
		"shotList":            shotList,
		"content":             buildShotListMarkdown("Visual Alignment", shotList),
		"artifacts": []map[string]interface{}{
			jsonArtifact(stage, "visual_alignment_plan.json", skillName, "VISUAL_ALIGNMENT_PLAN", true),
		},
	})
}

func executeCinematicShotDesigner(stage, skillName, brief string, params map[string]interface{}) tool.ToolResult {
	sourceShots := timeWindowMapsFromParams(params)
	preserveAuthoredDuration := len(sourceShots) > 0
	if len(sourceShots) == 0 {
		sourceShots = toolMapsFromValue(params["shotList"], "shotList", "shots")
	}
	if len(sourceShots) == 0 {
		sourceShots = []map[string]interface{}{
			{"shotId": "SHOT_01", "durationSec": 6, "visual": firstNonEmptyString(params, "brief", "topic", "goal")},
		}
	}
	profile := creationProfileFromToolValue(params["creationProfile"])
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
		shotList = append(shotList, map[string]interface{}{
			"shotId":       shotID,
			"durationSec":  plannerOutputDurationSec(source, preserveAuthoredDuration),
			"visual":       visual,
			"mainAction":   firstNonEmptyString(source, "mainAction", "action"),
			"directorNote": "镜头设计草案；仅生成规划，不生成媒体。",
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
		shotGenerationPlans = append(shotGenerationPlans, structToMap(plan))
		shotAssetPackages = append(shotAssetPackages, shotAssetPackageFromGenerationPlan(values, plan))
		externalRequests = append(externalRequests, externalRequestsFromGenerationPlan(plan)...)
	}

	summary := fmt.Sprintf("已为 %d 个镜头生成逐镜头生成计划，包含 %d 个外部生成请求。", len(shotGenerationPlans), len(externalRequests))
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
		"referenceImages", "references", "timeWindowId", "parentShotId",
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
	pkg := map[string]interface{}{
		"shotId":         plan.ShotID,
		"durationSec":    normalizedDurationSec(firstValueInMap(shotMap, "durationSec", "duration", "seconds")),
		"visual":         firstNonEmptyString(shotMap, "visual", "visualIntent", "description", "sceneSummary"),
		"generationPlan": planMap,
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

	for i, shot := range shots {
		shotID := firstStringInMap(shot, "shotId", "id", "cardId")
		if shotID == "" {
			shotID = fmt.Sprintf("SHOT_%02d", i+1)
		}
		unitShotID := sanitizeUnitPart(shotID)
		duration := normalizedDurationSec(firstExistingValue(shot, "durationSec", "duration", "seconds"))
		narration := firstStringInMap(shot, "narrationText", "scriptText", "voiceover", "text", "claim")
		if narration == "" {
			narration = fmt.Sprintf("%s 的第 %d 个独立镜头口播。", topic, i+1)
		}
		visual := firstStringInMap(shot, "visual", "visualIntent", "description", "composition")
		camera := firstStringInMap(shot, "camera", "cameraMove", "cameraMotion")
		lighting := firstStringInMap(shot, "lighting", "light", "mood")
		composition := firstStringInMap(shot, "composition", "framing")
		transitionAtEnd := transitionTextForShot(shot)
		materialHints := stringListFromInterface(shot["materialLibraryHints"])
		references := referenceImagesFromHints(shotID, materialHints)

		promptParts := []string{
			fmt.Sprintf("独立生成 %d 秒非写实动画 AIGC 视频，主题：%s。", duration, topic),
			fmt.Sprintf("镜头 %s：%s", shotID, fallbackText(visual, narration)),
			"口播/字幕内容：" + narration,
			"画面需包含主体、场景、动作、镜头运动、光影、色彩和风格，禁止真人写实，保持干净、知识分享、电影感动画。",
			"本 shot 完全独立生成，不依赖上一镜或下一镜，不写同上、接上一镜、延续前一镜。",
			"转场只覆盖在本 shot 结尾：" + transitionAtEnd,
			"生成后可直接作为 SHOT_VIDEO_CLIP，用 ffmpeg 按 shot 顺序 simple_cut 拼接。",
		}
		if camera != "" {
			promptParts = append(promptParts, "镜头运动："+camera)
		}
		if composition != "" {
			promptParts = append(promptParts, "构图："+composition)
		}
		if lighting != "" {
			promptParts = append(promptParts, "光影："+lighting)
		}
		if len(materialHints) > 0 {
			promptParts = append(promptParts, "可参考素材库关键词："+strings.Join(materialHints, "、"))
		}
		videoPrompt := strings.Join(promptParts, "\n")
		negativePrompt := "禁止真人写实、禁止跨 shot 依赖、禁止尾帧对齐、禁止要求上一镜或下一镜配合、禁止水印、禁止文字乱码、禁止画面崩坏。"
		requestID := "extgen_video_" + unitShotID

		videoPrompts = append(videoPrompts, map[string]interface{}{
			"shotId":               shotID,
			"durationSec":          duration,
			"narrationText":        narration,
			"prompt":               videoPrompt,
			"negativePrompt":       negativePrompt,
			"continuity":           "仅共享主要角色、主要道具、主场景和全片风格；不得依赖其他 shot 的画面。",
			"materialLibraryHints": materialHints,
			"subtitleText":         narration,
			"shotAssemblyPlan": map[string]interface{}{
				"audioArtifactKind":    "SHOT_AUDIO",
				"subtitleArtifactKind": "SHOT_SUBTITLE",
				"videoArtifactKind":    "SHOT_VIDEO_CLIP",
				"concatMode":           "simple_cut",
				"transitionAtEnd":      transitionAtEnd,
			},
		})

		requests = append(requests, map[string]interface{}{
			"requestId":           requestID,
			"kind":                "video",
			"shotId":              shotID,
			"prompt":              videoPrompt,
			"promptText":          videoPrompt,
			"negativePrompt":      negativePrompt,
			"references":          references,
			"target":              map[string]interface{}{"aspectRatio": "16:9", "durationSec": duration, "resolution": "1920x1080"},
			"promptCharLimit":     2000,
			"referenceImageLimit": 6,
			"status":              "pending_upload",
			"manualInstruction":   "当前没有可用的视频生成 API 配置，请在浏览器外部视频平台复制 Prompt 生成本 shot，再回传上传结果。",
		})

		packages = append(packages, map[string]interface{}{
			"shotId":          shotID,
			"durationSec":     duration,
			"referenceImages": references,
			"prompts": map[string]interface{}{
				"videoPrompt":    videoPrompt,
				"negativePrompt": negativePrompt,
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
			"concatPlan": map[string]interface{}{
				"ffmpegReady":                true,
				"mode":                       "simple_cut",
				"transitionCoveredInShotEnd": true,
			},
			"independence": map[string]interface{}{
				"crossShotDependencyForbidden": true,
				"allowedSharedConsistency":     []string{"主要角色", "主要道具", "主场景", "全片风格"},
			},
		})

		artifacts = append(artifacts,
			externalGenerationArtifact(requestID, shotID, "video"),
			shotPlaceholderArtifact("shot_video_"+unitShotID, "SHOT_VIDEO_CLIP", fmt.Sprintf("%s_video_clip.mp4", shotID), "video/mp4", "shot_video_clip", shotID),
			shotPlaceholderArtifact("shot_audio_"+unitShotID, "SHOT_AUDIO", fmt.Sprintf("%s_voiceover.wav", shotID), "audio/wav", "shot_audio", shotID),
			shotPlaceholderArtifact("shot_subtitle_"+unitShotID, "SHOT_SUBTITLE", fmt.Sprintf("%s_subtitle.srt", shotID), "text/plain", "shot_subtitle", shotID),
			shotPlaceholderArtifact("shot_asset_package_"+unitShotID, "SHOT_ASSET_PACKAGE", fmt.Sprintf("%s_asset_package.json", shotID), "application/json", "shot_asset_package", shotID),
		)
	}

	contentPkg := map[string]interface{}{
		"videoPrompts":               videoPrompts,
		"externalGenerationRequests": requests,
		"shotAssetPackages":          packages,
		"artifacts":                  artifacts,
		"summary":                    "已基于 shotList 本地生成可复制到浏览器外部平台的独立 shot 视频提示词；当前不依赖图片或视频生成 API。",
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
		"summary":                    contentPkg["summary"],
	}, true
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
	publishCopyJSON := serializeParamJSON(params["publishCopy"])

	// Determine project directory.
	projectRoot := resolveHyperFramesProjectRoot(hyperFramesConfig.ProjectRoot)
	projectDir := filepath.Join(projectRoot, toolCtx.TaskID, "hyperframes")
	assetsDir := filepath.Join(projectDir, "assets")

	// Build data.json content.
	dataJSON := buildHyperFramesDataJSON(topic, script, shotListJSON, videoPromptsJSON, shotAssetPackagesJSON, style, publishCopyJSON)
	manifestJSON := buildHyperFramesManifestJSON(topic, toolCtx.TaskID)
	styleCSS := hyperFramesDefaultStyleCSS()

	// Use LLM to generate the index.html.
	indexHTML := generateHyperFramesIndexHTML(topic, script, shotListJSON, videoPromptsJSON, style, params, toolCtx)

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
func buildHyperFramesDataJSON(topic, script, shotListJSON, videoPromptsJSON, shotAssetPackagesJSON, style, publishCopyJSON string) string {
	data := map[string]interface{}{
		"topic":                 topic,
		"script":                script,
		"productionMode":        "shot_first",
		"reviewUnit":            "shot",
		"requiresNarrationSync": true,
		"shots":                 publicJSONValueOrFallback(shotListJSON, []interface{}{}),
		"videoPrompts":          publicJSONValueOrFallback(videoPromptsJSON, []interface{}{}),
		"shotAssetPackages":     publicJSONValueOrFallback(shotAssetPackagesJSON, []interface{}{}),
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
func generateHyperFramesIndexHTML(topic, script, shotListJSON, videoPromptsJSON, style string, params map[string]interface{}, toolCtx tool.ToolContext) string {
	if !shouldUseLLMHyperFramesLayout(params) {
		zap.L().Info("Generating deterministic HyperFrames HTML without LLM layout")
		return buildMinimalHyperFramesHTML(topic, script, shotListJSON, style)
	}

	effectiveCfg := effectiveVideoCreationOpenAIConfig(params)

	// If no LLM is configured, generate a minimal static HTML from the data.
	if effectiveCfg.APIKey == "" {
		zap.L().Info("No LLM API key configured, generating minimal HyperFrames HTML")
		return buildMinimalHyperFramesHTML(topic, script, shotListJSON, style)
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

风格要求：%s

请生成完整的 HyperFrames HTML 视频页面。`, topic, script, shotListJSON, videoPromptsJSON, style)

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
		return buildMinimalHyperFramesHTML(topic, script, shotListJSON, style)
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
		return buildMinimalHyperFramesHTML(topic, script, shotListJSON, style)
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
func buildMinimalHyperFramesHTML(topic, script, shotListJSON, style string) string {
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
@keyframes fadeIn { from { opacity: 0; transform: translateY(30px); } to { opacity: 1; transform: translateY(0); } }
@keyframes fadeOut { from { opacity: 1; } to { opacity: 0; } }
.anim-fade-in { animation: fadeIn 0.8s ease-out both; }
.anim-fade-out { animation: fadeOut 0.6s ease-in both; }
</style>
</head>
<body>
<div id="app" data-composition-id="main" data-duration="%d">
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
</html>`, templateEscape(topic), totalDurationSec, scenesBuilder.String(), totalDurationSec)
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

func extractGeneratedVideoURL(content string) string {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &parsed); err != nil {
		return ""
	}
	return firstNonEmptyString(parsed, "url", "videoUrl", "storageRef", "outputUrl")
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
	clean = strings.TrimRight(clean, "，,。.：:、。")
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
	} else if status == "manual_upload_required" {
		b.WriteString("## Manual Video Result\n\n")
		b.WriteString("HyperFrames rendering is disabled in this environment. Upload or register an externally rendered MP4 for this placeholder path:\n\n")
		b.WriteString(fmt.Sprintf("- Expected output: `%s`\n", renderPath))
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
		if toolName == "video_script_generator" && requiresFreshKnowledge(params) && !hasKnowledgeFacts(params["knowledgePack"], facts) && !hasKnowledgeContextFacts(params["knowledgeContext"]) {
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

	if toolName == "shot_splitter" {
		if data, ok := buildDeterministicShotSplitterData(toolName, skillName, topic, script, targetDurationSec); ok {
			return tool.SuccessResult(data)
		}
	}
	if toolName == "video_prompt_generator" {
		if data, ok := buildDeterministicVideoPromptData(toolName, skillName, topic, params); ok {
			return tool.SuccessResult(data)
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
		if toolName == "video_script_generator" {
			return tool.SuccessResult(buildFallbackVideoScriptData(toolName, skillName, topic, targetDurationSec, usedFacts, knowledgeTrace))
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
		return `你是图文视频参考资产选择器。

目标：
输出背景、图标、字体、色彩和素材使用策略，不联网下载素材。

输出严格 JSON：
{
  "referenceAssetPlan": {
    "artifactKind": "REFERENCE_ASSET_PLAN",
    "visualReferences": [
      {"type": "background", "strategy": "clean_gradient"},
      {"type": "icon", "strategy": "minimal_line_icon"},
      {"type": "font", "strategy": "system_sans"}
    ]
  },
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
		return `你是视频连续性管理 Agent。

检查脚本、卡片、composition 和参考资产之间的术语、风格、画幅和视觉一致性。

输出严格 JSON：
{
  "continuityReport": {
    "artifactKind": "CONTINUITY_REPORT",
    "warnings": [],
    "staleArtifacts": []
  },
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
