package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
)

func TestProviderUsesRuntimeConfigForChatCompletion(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotModel string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("request body should be JSON: %v", err)
		}
		gotModel = body.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"revisedContent\":\"ok\",\"summary\":\"done\"}"}}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`))
	}))
	defer server.Close()

	provider := NewProviderWithConfigResolver(func() Config {
		return Config{
			APIKey:  "sk-runtime",
			BaseURL: server.URL + "/v1",
			Model:   "gpt-runtime",
		}
	})

	result, err := provider.Execute(context.Background(), &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Messages:   []modelgateway.Message{{Role: "user", Content: "revise"}},
	})
	if err != nil {
		t.Fatalf("runtime config should allow chat completion: %v", err)
	}
	if result.Content == "" {
		t.Fatal("expected response content")
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("unexpected chat completion path: %s", gotPath)
	}
	if gotAuth != "Bearer sk-runtime" {
		t.Fatalf("unexpected auth header: %s", gotAuth)
	}
	if gotModel != "gpt-runtime" {
		t.Fatalf("unexpected model: %s", gotModel)
	}
}

func TestOpenAIEndpointUsesConfiguredAPIBase(t *testing.T) {
	if got := openAIEndpoint("https://api.deepseek.com", "/chat/completions"); got != "https://api.deepseek.com/chat/completions" {
		t.Fatalf("unexpected endpoint for API base without v1: %s", got)
	}
	if got := openAIEndpoint("https://api.openai.com/v1", "/chat/completions"); got != "https://api.openai.com/v1/chat/completions" {
		t.Fatalf("unexpected endpoint for API base with v1: %s", got)
	}
}
