package builtin

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/common/crypto"
	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
)

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
	syncOpenAIEnvLocked(runtimeModelProviderConfig)
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
		syncOpenAIEnvLocked(runtimeModelProviderConfig)
	}
}

func syncOpenAIEnvLocked(cfg RuntimeModelProviderConfig) {
	if cfg.APIKey != "" {
		_ = os.Setenv("OPENAI_API_KEY", cfg.APIKey)
	}
	if cfg.BaseURL != "" {
		_ = os.Setenv("OPENAI_BASE_URL", cfg.BaseURL)
	}
	if cfg.Model != "" {
		_ = os.Setenv("OPENAI_MODEL", cfg.Model)
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

var localAgentConfigFetcher = TryFetchLocalAgentConfig
