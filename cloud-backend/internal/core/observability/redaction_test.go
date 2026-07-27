package observability

import (
	"reflect"
	"slices"
	"testing"
)

func TestRedactNeverKeepsSecretValues(t *testing.T) {
	event := validEvent()
	event.Evidence.Attributes = map[string]any{
		"authorization": "Bearer secret-value",
		"cookie":        "session=secret-value",
		"token":         "secret-value",
		"password":      "secret-value",
		"apiKey":        "secret-value",
		"secret":        "secret-value",
		"prompt":        "raw prompt",
		"userInput":     "raw user input",
		"artifactId":    "art_1",
	}

	got := Redact(event)

	if ContainsSecret(got) {
		t.Fatal("redacted event still contains a secret")
	}
	if got.Evidence.Attributes != nil {
		t.Fatalf("non-contract evidence attributes were retained: %#v", got.Evidence.Attributes)
	}
	for _, field := range []string{
		"evidence.attributes.authorization",
		"evidence.attributes.cookie",
		"evidence.attributes.token",
		"evidence.attributes.password",
		"evidence.attributes.apiKey",
		"evidence.attributes.secret",
		"evidence.attributes.prompt",
		"evidence.attributes.userInput",
		"evidence.attributes.artifactId",
	} {
		if !slices.Contains(got.Privacy.RedactedFields, field) {
			t.Errorf("redactedFields = %#v, missing %q", got.Privacy.RedactedFields, field)
		}
	}
}

func TestRedactPreservesOnlyAllowlistedEvidenceAndDoesNotMutateInput(t *testing.T) {
	event := validEvent()
	event.Evidence.Attributes = map[string]any{"custom": map[string]any{"token": "raw-token"}}
	before := validEvent()
	before.Evidence.Attributes = map[string]any{"custom": map[string]any{"token": "raw-token"}}

	got := Redact(event)

	if !reflect.DeepEqual(event, before) {
		t.Fatalf("Redact mutated input:\n got  %#v\n want %#v", event, before)
	}
	if !reflect.DeepEqual(got.Evidence.InputRefs, event.Evidence.InputRefs) ||
		!reflect.DeepEqual(got.Evidence.OutputRefs, event.Evidence.OutputRefs) ||
		got.Evidence.InputHash != event.Evidence.InputHash ||
		got.Evidence.SizeBytes == nil ||
		*got.Evidence.SizeBytes != *event.Evidence.SizeBytes {
		t.Fatalf("allowlisted evidence changed: got %#v want %#v", got.Evidence, event.Evidence)
	}
	if got.Evidence.Attributes != nil {
		t.Fatalf("unexpected attributes: %#v", got.Evidence.Attributes)
	}
}

func TestRedactRemovesArbitraryDeveloperErrorText(t *testing.T) {
	event := validEvent()
	event.Error.DeveloperDetail = "dial tcp 127.0.0.1:9001 with password secret-value"

	got := Redact(event)

	if got.Error.DeveloperDetail != "" {
		t.Fatalf("DeveloperDetail = %q, want empty", got.Error.DeveloperDetail)
	}
	if !slices.Contains(got.Privacy.RedactedFields, "error.developerDetail") {
		t.Fatalf("redactedFields = %#v, missing developer detail", got.Privacy.RedactedFields)
	}
	if ContainsSecret(got) {
		t.Fatal("redacted event still contains developer error text")
	}
}

func TestContainsSecretFindsNestedSensitiveFieldsAndUnsafeDeveloperDetail(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{"authorization", map[string]any{"metadata": map[string]any{"authorization": "Bearer raw"}}},
		{"cookie", map[string]any{"cookie": "session=raw"}},
		{"token", map[string]any{"token": "raw"}},
		{"password", map[string]any{"password": "raw"}},
		{"api key", map[string]any{"api_key": "raw"}},
		{"secret", map[string]any{"clientSecret": "raw"}},
		{"prompt", map[string]any{"prompt": "raw"}},
		{"user input", map[string]any{"user-input": "raw"}},
		{"developer detail", EventError{DeveloperDetail: "raw provider response"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !ContainsSecret(tt.value) {
				t.Fatalf("ContainsSecret(%#v) = false", tt.value)
			}
		})
	}
}

func TestContainsSecretAcceptsSafeContractMetadata(t *testing.T) {
	event := validEvent()
	event.Runtime.PromptTemplateVersion = "prompt-template-v2"
	event.Privacy.RedactedFields = []string{
		"evidence.attributes.authorization",
		"evidence.attributes.prompt",
	}

	if ContainsSecret(event) {
		t.Fatalf("ContainsSecret(%#v) = true", event)
	}
}

func TestContainsSecretRejectsSecretClassification(t *testing.T) {
	event := validEvent()
	event.Privacy.Classification = PrivacySecret

	if !ContainsSecret(event) {
		t.Fatal("ContainsSecret() = false for SECRET-classified event")
	}
}
