package hyperframes

import "time"

// Mode controls how the system connects to HyperFrames.
type Mode string

const (
	ModeDisabled Mode = "disabled"
	ModeService  Mode = "service"
)

// Config holds the configuration for the HyperFrames Render Service client.
type Config struct {
	Mode           Mode   `mapstructure:"HYPERFRAMES_MODE"`
	ServiceURL     string `mapstructure:"HYPERFRAMES_SERVICE_URL"`
	TimeoutSec     int    `mapstructure:"HYPERFRAMES_TIMEOUT_SEC"`
	DefaultFPS     int    `mapstructure:"HYPERFRAMES_DEFAULT_FPS"`
	DefaultQuality string `mapstructure:"HYPERFRAMES_DEFAULT_QUALITY"`
	DefaultFormat  string `mapstructure:"HYPERFRAMES_DEFAULT_FORMAT"`
	MaxWorkers     int    `mapstructure:"HYPERFRAMES_MAX_WORKERS"`
	UseGPU         bool   `mapstructure:"HYPERFRAMES_USE_GPU"`
	ProjectRoot    string `mapstructure:"HYPERFRAMES_PROJECT_ROOT"`
	OutputRoot     string `mapstructure:"HYPERFRAMES_OUTPUT_ROOT"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Mode:           ModeDisabled,
		ServiceURL:     "http://127.0.0.1:8787",
		TimeoutSec:     1800,
		DefaultFPS:     30,
		DefaultQuality: "standard",
		DefaultFormat:  "mp4",
		MaxWorkers:     4,
		UseGPU:         false,
		ProjectRoot:    "/data/aios/projects",
		OutputRoot:     "/data/aios/projects",
	}
}

// Timeout returns the configured timeout as time.Duration.
func (c Config) Timeout() time.Duration {
	if c.TimeoutSec <= 0 {
		return 30 * time.Minute
	}
	return time.Duration(c.TimeoutSec) * time.Second
}

// IsEnabled returns true when the service mode is active.
func (c Config) IsEnabled() bool {
	return c.Mode == ModeService
}
