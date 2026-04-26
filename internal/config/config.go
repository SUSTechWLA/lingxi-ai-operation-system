package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type Config struct {
	Server   ServerConfig   `mapstructure:",squash"`
	Postgres PostgresConfig `mapstructure:",squash"`
	Redis    RedisConfig    `mapstructure:",squash"`
	Kafka    KafkaConfig    `mapstructure:",squash"`
	OpenAI   OpenAIConfig   `mapstructure:",squash"`
	Worker   WorkerConfig   `mapstructure:",squash"`
	BashTool BashToolConfig `mapstructure:",squash"`
	Services ServicesConfig `mapstructure:",squash"`
	Sandbox  SandboxConfig  `mapstructure:",squash"`
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

type WorkerConfig struct {
	ToolTimeoutSeconds int           `mapstructure:"WORKER_TOOL_TIMEOUT"`
	ThreadPoolCore     int           `mapstructure:"WORKER_THREAD_POOL_CORE"`
	ThreadPoolMax      int           `mapstructure:"WORKER_THREAD_POOL_MAX"`
	Sandbox            SandboxConfig `mapstructure:",squash"`
}

type BashToolConfig struct {
	AllowedCommands string `mapstructure:"BASH_TOOL_ALLOWED_COMMANDS"`
	TimeoutSeconds  int    `mapstructure:"BASH_TOOL_TIMEOUT"`
}

type ServicesConfig struct {
	OrchestratorURL  string `mapstructure:"ORCHESTRATOR_URL"`
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

	return cfg
}

func setDefaults() {
	viper.SetDefault("SERVER_PORT", 8080)
	viper.SetDefault("POSTGRES_HOST", "localhost")
	viper.SetDefault("POSTGRES_PORT", 5432)
	viper.SetDefault("POSTGRES_USER", "postgres")
	viper.SetDefault("POSTGRES_PASSWORD", "changeme")
	viper.SetDefault("POSTGRES_DB", "lingxi_db")
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
	viper.SetDefault("WORKER_TOOL_TIMEOUT", 120)
	viper.SetDefault("WORKER_THREAD_POOL_CORE", 10)
	viper.SetDefault("WORKER_THREAD_POOL_MAX", 50)
	viper.SetDefault("BASH_TOOL_ALLOWED_COMMANDS", "*")
	viper.SetDefault("BASH_TOOL_TIMEOUT", 60)
	viper.SetDefault("ORCHESTRATOR_URL", "http://localhost:8080")
	viper.SetDefault("CONTEXT_SERVICE_URL", "http://localhost:8082")

	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		viper.SetDefault("OPENAI_API_KEY", apiKey)
	}
}
