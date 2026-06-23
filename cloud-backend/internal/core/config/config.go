package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type Config struct {
	Server      ServerConfig       `mapstructure:",squash"`
	Postgres    PostgresConfig     `mapstructure:",squash"`
	Redis       RedisConfig        `mapstructure:",squash"`
	Kafka       KafkaConfig        `mapstructure:",squash"`
	OpenAI      OpenAIConfig       `mapstructure:",squash"`
	Auth        AuthConfig         `mapstructure:",squash"`
	Worker      WorkerConfig       `mapstructure:",squash"`
	MinIO       MinIOConfig        `mapstructure:",squash"`
	BashTool    BashToolConfig     `mapstructure:",squash"`
	Services    ServicesConfig     `mapstructure:",squash"`
	Sandbox     SandboxConfig      `mapstructure:",squash"`
	Video       VideoConfig        `mapstructure:",squash"`
	Agent       AgentConfig        `mapstructure:",squash"`
	HyperFrames HyperFramesConfig  `mapstructure:",squash"`
}

// AgentConfig controls the dynamic agent runtime.
type AgentConfig struct {
	PlannerMode     string `mapstructure:"AGENT_PLANNER_MODE"` // "llm" | "hybrid" | "heuristic"
	PlannerMaxTools int    `mapstructure:"AGENT_PLANNER_MAX_TOOLS"`
}

// VideoConfig controls the video creation feature flags.
type VideoConfig struct {
	VideoCreationEnabled            bool   `mapstructure:"VIDEO_CREATION_ENABLED"`
	LocalRunnerEnabled              bool   `mapstructure:"LOCAL_RUNNER_ENABLED"`
	ModelProviderMode               string `mapstructure:"MODEL_PROVIDER_MODE"` // "fake" | "real"
	ImageProvider                   string `mapstructure:"IMAGE_PROVIDER"`      // "openai" | "stability" (image generation)
	SkillRoot                       string `mapstructure:"SKILL_ROOT"`          // path to legacy skills/ directory
	SkillCapabilityRoot             string `mapstructure:"SKILL_CAPABILITY_ROOT"`
	LegacySkillWorkflowAutoRegister bool   `mapstructure:"LEGACY_SKILL_WORKFLOW_AUTOREGISTER"`
	HyperFramesCLIPath              string `mapstructure:"HYPERFRAMES_CLI_PATH"` // [DEPRECATED] use HyperFramesConfig.Mode=service instead
}

// HyperFramesConfig controls the HyperFrames Render Service connection.
// When Mode is "service", the system calls the Render Service HTTP API
// instead of shelling out to the CLI.
type HyperFramesConfig struct {
	Mode           string `mapstructure:"HYPERFRAMES_MODE"`            // "disabled" | "service"
	ServiceURL     string `mapstructure:"HYPERFRAMES_SERVICE_URL"`     // e.g. http://127.0.0.1:8787
	TimeoutSec     int    `mapstructure:"HYPERFRAMES_TIMEOUT_SEC"`     // render timeout in seconds
	DefaultFPS     int    `mapstructure:"HYPERFRAMES_DEFAULT_FPS"`     // default frames per second
	DefaultQuality string `mapstructure:"HYPERFRAMES_DEFAULT_QUALITY"` // "draft" | "standard" | "high"
	DefaultFormat  string `mapstructure:"HYPERFRAMES_DEFAULT_FORMAT"`  // "mp4" | "webm"
	MaxWorkers     int    `mapstructure:"HYPERFRAMES_MAX_WORKERS"`     // max parallel render workers
	UseGPU         bool   `mapstructure:"HYPERFRAMES_USE_GPU"`         // enable GPU acceleration
	ProjectRoot    string `mapstructure:"HYPERFRAMES_PROJECT_ROOT"`    // allowed project dir root
	OutputRoot     string `mapstructure:"HYPERFRAMES_OUTPUT_ROOT"`     // allowed output dir root
}

type ServerConfig struct {
	Port int `mapstructure:"SERVER_PORT"`
}

type PostgresConfig struct {
	Host     string `mapstructure:"POSTGRES_HOST"`
	Port     int    `mapstructure:"POSTGRES_PORT"`
	User     string `mapstructure:"POSTGRES_USER"`
	Password string `mapstructure:"POSTGRES_PASSWORD"`
	DB       string `mapstructure:"POSTGRES_DB"`
	SSLMode  string `mapstructure:"POSTGRES_SSLMODE"`
}

func (c PostgresConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.DB, c.SSLMode)
}

type RedisConfig struct {
	Host     string `mapstructure:"REDIS_HOST"`
	Port     int    `mapstructure:"REDIS_PORT"`
	Password string `mapstructure:"REDIS_PASSWORD"`
	DB       int    `mapstructure:"REDIS_DB"`
}

func (c RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type KafkaConfig struct {
	BootstrapServers string `mapstructure:"KAFKA_BOOTSTRAP_SERVERS"`
}

type OpenAIConfig struct {
	APIKey      string  `mapstructure:"OPENAI_API_KEY"`
	BaseURL     string  `mapstructure:"OPENAI_BASE_URL"`
	Model       string  `mapstructure:"OPENAI_MODEL"`
	MaxTokens   int     `mapstructure:"OPENAI_MAX_TOKENS"`
	Temperature float64 `mapstructure:"OPENAI_TEMPERATURE"`
	Timeout     int     `mapstructure:"OPENAI_TIMEOUT"`
}

type AuthConfig struct {
	TokenSecret            string `mapstructure:"AUTH_TOKEN_SECRET"`
	AccessTokenTTLSeconds  int    `mapstructure:"AUTH_ACCESS_TOKEN_TTL_SECONDS"`
	RefreshTokenTTLSeconds int    `mapstructure:"AUTH_REFRESH_TOKEN_TTL_SECONDS"`
}

type WorkerConfig struct {
	ToolTimeoutSeconds   int           `mapstructure:"WORKER_TOOL_TIMEOUT"`
	ThreadPoolCore       int           `mapstructure:"WORKER_THREAD_POOL_CORE"`
	ThreadPoolMax        int           `mapstructure:"WORKER_THREAD_POOL_MAX"`
	HeartbeatIntervalSec int           `mapstructure:"WORKER_HEARTBEAT_INTERVAL"`
	HeartbeatTimeoutSec  int           `mapstructure:"WORKER_HEARTBEAT_TIMEOUT"`
	Sandbox              SandboxConfig `mapstructure:",squash"`
}

type MinIOConfig struct {
	Endpoint  string `mapstructure:"MINIO_ENDPOINT"`
	AccessKey string `mapstructure:"MINIO_ACCESS_KEY"`
	SecretKey string `mapstructure:"MINIO_SECRET_KEY"`
	Bucket    string `mapstructure:"MINIO_BUCKET"`
	UseSSL    bool   `mapstructure:"MINIO_USE_SSL"`
}

type BashToolConfig struct {
	AllowedCommands string `mapstructure:"BASH_TOOL_ALLOWED_COMMANDS"`
	TimeoutSeconds  int    `mapstructure:"BASH_TOOL_TIMEOUT"`
}

type ServicesConfig struct {
	OrchestratorURL   string `mapstructure:"ORCHESTRATOR_URL"`
	ContextServiceURL string `mapstructure:"CONTEXT_SERVICE_URL"`
}

type SandboxConfig struct {
	Enabled  bool   `mapstructure:"SANDBOX_ENABLED"`
	Address  string `mapstructure:"SANDBOX_ADDRESS"`
	Fallback bool   `mapstructure:"SANDBOX_FALLBACK"`
}

func Load() *Config {
	viper.SetConfigFile(".env")
	viper.AutomaticEnv()

	_ = viper.ReadInConfig()

	setDefaults()

	cfg := &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		zap.L().Fatal("Failed to unmarshal config", zap.Error(err))
	}
	if cwd, err := os.Getwd(); err == nil {
		cfg.Video.SkillRoot = resolveSkillRoot(cfg.Video.SkillRoot, cwd)
		cfg.Video.SkillCapabilityRoot = resolveSkillRoot(cfg.Video.SkillCapabilityRoot, cwd)
	}

	return cfg
}

func setDefaults() {
	viper.SetDefault("SERVER_PORT", 8080)
	viper.SetDefault("POSTGRES_HOST", "localhost")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "changeme")
	viper.SetDefault("POSTGRES_DB", "tangying_db")
	viper.SetDefault("POSTGRES_SSLMODE", "disable")
	viper.SetDefault("REDIS_HOST", "localhost")
	viper.SetDefault("REDIS_PORT", 6379)
	viper.SetDefault("REDIS_DB", 0)
	viper.SetDefault("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092")
	viper.SetDefault("OPENAI_BASE_URL", "https://api.openai.com/v1")
	viper.SetDefault("OPENAI_MODEL", "gpt-4")
	viper.SetDefault("OPENAI_MAX_TOKENS", 2000)
	viper.SetDefault("OPENAI_TEMPERATURE", 0.7)
	viper.SetDefault("OPENAI_TIMEOUT", 60)
	viper.SetDefault("AUTH_TOKEN_SECRET", "development-only-change-me")
	viper.SetDefault("AUTH_ACCESS_TOKEN_TTL_SECONDS", 3600)
	viper.SetDefault("AUTH_REFRESH_TOKEN_TTL_SECONDS", 2592000)
	viper.SetDefault("WORKER_TOOL_TIMEOUT", 120)
	viper.SetDefault("WORKER_THREAD_POOL_CORE", 10)
	viper.SetDefault("WORKER_THREAD_POOL_MAX", 50)
	viper.SetDefault("WORKER_HEARTBEAT_INTERVAL", 30)
	viper.SetDefault("WORKER_HEARTBEAT_TIMEOUT", 300)
	viper.SetDefault("BASH_TOOL_ALLOWED_COMMANDS", "*")
	viper.SetDefault("BASH_TOOL_TIMEOUT", 60)
	viper.SetDefault("ORCHESTRATOR_URL", "http://localhost:8080")
	viper.SetDefault("CONTEXT_SERVICE_URL", "http://localhost:8082")
	viper.SetDefault("MINIO_ENDPOINT", "localhost:9000")
	viper.SetDefault("MINIO_ACCESS_KEY", "minioadmin")
	viper.SetDefault("MINIO_SECRET_KEY", "changeme")
	viper.SetDefault("MINIO_BUCKET", "media-assets")
	viper.SetDefault("MINIO_USE_SSL", false)

	// Video creation is part of the default cloud backend surface. Local desktop
	// execution remains off unless explicitly enabled.
	viper.SetDefault("VIDEO_CREATION_ENABLED", true)
	viper.SetDefault("LOCAL_RUNNER_ENABLED", false)
	viper.SetDefault("MODEL_PROVIDER_MODE", "fake")
	viper.SetDefault("IMAGE_PROVIDER", "openai")
	viper.SetDefault("SKILL_ROOT", "skills")
	viper.SetDefault("SKILL_CAPABILITY_ROOT", "skill-capabilities")
	viper.SetDefault("LEGACY_SKILL_WORKFLOW_AUTOREGISTER", false)
	viper.SetDefault("AGENT_PLANNER_MODE", "hybrid")
	viper.SetDefault("AGENT_PLANNER_MAX_TOOLS", 6)
	viper.SetDefault("HYPERFRAMES_MODE", "disabled")
	viper.SetDefault("HYPERFRAMES_SERVICE_URL", "http://127.0.0.1:8787")
	viper.SetDefault("HYPERFRAMES_TIMEOUT_SEC", 1800)
	viper.SetDefault("HYPERFRAMES_DEFAULT_FPS", 30)
	viper.SetDefault("HYPERFRAMES_DEFAULT_QUALITY", "standard")
	viper.SetDefault("HYPERFRAMES_DEFAULT_FORMAT", "mp4")
	viper.SetDefault("HYPERFRAMES_MAX_WORKERS", 4)
	viper.SetDefault("HYPERFRAMES_USE_GPU", false)
	viper.SetDefault("HYPERFRAMES_PROJECT_ROOT", "/data/aios/projects")
	viper.SetDefault("HYPERFRAMES_OUTPUT_ROOT", "/data/aios/projects")

	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		viper.SetDefault("OPENAI_API_KEY", apiKey)
	}
}

func resolveSkillRoot(root, cwd string) string {
	if root == "" {
		return root
	}
	if filepath.IsAbs(root) {
		return filepath.Clean(root)
	}

	candidates := []string{
		filepath.Join(cwd, root),
		filepath.Join(cwd, "cloud-backend", root),
		filepath.Join(cwd, "..", root),
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, root),
			filepath.Join(exeDir, "..", root),
			filepath.Join(exeDir, "..", "cloud-backend", root),
			filepath.Join(exeDir, "..", "..", "cloud-backend", root),
		)
	}

	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return root
}
