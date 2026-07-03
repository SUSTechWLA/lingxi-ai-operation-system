package builtin

import (
	"os"
	"testing"
)

func TestSetRuntimeModelProviderConfigSyncsOpenAIEnvironment(t *testing.T) {
	oldKey := os.Getenv("OPENAI_API_KEY")
	oldBaseURL := os.Getenv("OPENAI_BASE_URL")
	oldModel := os.Getenv("OPENAI_MODEL")
	t.Cleanup(func() {
		setOrUnsetEnv("OPENAI_API_KEY", oldKey)
		setOrUnsetEnv("OPENAI_BASE_URL", oldBaseURL)
		setOrUnsetEnv("OPENAI_MODEL", oldModel)
		ClearRuntimeModelProviderConfig()
	})

	SetRuntimeModelProviderConfig(RuntimeModelProviderConfig{
		BaseURL: "https://api.deepseek.com",
		APIKey:  "sk-runtime",
		Model:   "deepseek-v4-pro",
	})

	if got := os.Getenv("OPENAI_API_KEY"); got != "sk-runtime" {
		t.Fatalf("OPENAI_API_KEY was not synced, got %q", got)
	}
	if got := os.Getenv("OPENAI_BASE_URL"); got != "https://api.deepseek.com" {
		t.Fatalf("OPENAI_BASE_URL was not synced, got %q", got)
	}
	if got := os.Getenv("OPENAI_MODEL"); got != "deepseek-v4-pro" {
		t.Fatalf("OPENAI_MODEL was not synced, got %q", got)
	}
}

func setOrUnsetEnv(key, value string) {
	if value == "" {
		_ = os.Unsetenv(key)
		return
	}
	_ = os.Setenv(key, value)
}
