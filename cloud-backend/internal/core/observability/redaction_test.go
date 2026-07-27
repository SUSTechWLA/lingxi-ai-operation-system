package observability

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
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
	if !slices.Contains(got.Privacy.RedactedFields, "evidence.attributes") {
		t.Fatalf("redactedFields = %#v, missing bounded attribute category", got.Privacy.RedactedFields)
	}
}

func TestRedactNeverCopiesUntrustedAttributeKeyText(t *testing.T) {
	event := validEvent()
	event.Evidence.Attributes = map[string]any{
		"authorization.Bearer secret-value": "ignored",
	}

	got := Redact(event)
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-value") || strings.Contains(string(data), "Bearer") {
		t.Fatalf("untrusted attribute key leaked into redacted event: %s", data)
	}
	if !reflect.DeepEqual(got.Privacy.RedactedFields, []string{"evidence.attributes"}) {
		t.Fatalf("redactedFields = %#v, want bounded category", got.Privacy.RedactedFields)
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

func TestRedactRemovesMismatchedDiagnosticKey(t *testing.T) {
	event := validEvent()
	event.Error.DeveloperDetail = "diagnostic.secret.abc123"

	got := Redact(event)

	if got.Error.DeveloperDetail != "" {
		t.Fatalf("DeveloperDetail = %q, want empty", got.Error.DeveloperDetail)
	}
	if !slices.Contains(got.Privacy.RedactedFields, "error.developerDetail") {
		t.Fatalf("redactedFields = %#v, missing developer detail", got.Privacy.RedactedFields)
	}
	if !ContainsSecret(event.Error) {
		t.Fatal("ContainsSecret() accepted mismatched diagnostic key")
	}
}

func TestRedactRemovesSensitiveCausalEventReference(t *testing.T) {
	const unsafeCause = "evt_authorization_Bearer_secret-value"
	event := validEvent()
	event.Error.CausedByEventID = unsafeCause

	got := Redact(event)
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if got.Error.CausedByEventID != "" {
		t.Fatalf("CausedByEventID = %q, want empty", got.Error.CausedByEventID)
	}
	if strings.Contains(string(data), unsafeCause) {
		t.Fatalf("unsafe causal reference leaked after redaction: %s", data)
	}
	if !slices.Contains(got.Privacy.RedactedFields, "error.causedByEventId") {
		t.Fatalf("redactedFields = %#v, missing causal reference", got.Privacy.RedactedFields)
	}
	if !ContainsSecret(event.Error) {
		t.Fatal("ContainsSecret() accepted unsafe typed causal reference")
	}
}

func TestRedactPreservesSafeCanonicalCausalEventReference(t *testing.T) {
	const safeCause = "evt_01j99zstagefailed"
	event := validEvent()
	event.Error.CausedByEventID = safeCause

	got := Redact(event)

	if got.Error.CausedByEventID != safeCause {
		t.Fatalf("CausedByEventID = %q, want %q", got.Error.CausedByEventID, safeCause)
	}
	if ContainsSecret(got.Error) {
		t.Fatal("ContainsSecret() rejected safe causal reference")
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

func TestContainsSecretRejectsDecodedSecretClassification(t *testing.T) {
	value := map[string]any{
		"wrapper": map[string]any{
			"Classification": "secret",
		},
	}

	if !ContainsSecret(value) {
		t.Fatal("ContainsSecret() = false for decoded SECRET classification")
	}
}

func TestContainsSecretRequiresDiagnosticKeyToMatchCode(t *testing.T) {
	matching := &EventError{
		Code:            "MCP.CONNECTION.UNAVAILABLE",
		DeveloperDetail: "diagnostic.mcp.connection.unavailable",
	}
	mismatched := &EventError{
		Code:            "MCP.CONNECTION.UNAVAILABLE",
		DeveloperDetail: "diagnostic.secret.abc123",
	}

	if ContainsSecret(matching) {
		t.Fatal("ContainsSecret() rejected matching diagnostic key")
	}
	if !ContainsSecret(mismatched) {
		t.Fatal("ContainsSecret() accepted mismatched diagnostic key")
	}
}

func TestContainsSecretRejectsUnsafeDecodedCausalEventReference(t *testing.T) {
	value := map[string]any{
		"error": map[string]any{
			"causedByEventId": "evt_authorization_Bearer_secret-value",
		},
	}

	if !ContainsSecret(value) {
		t.Fatal("ContainsSecret() accepted unsafe decoded causal reference")
	}
}

func TestContainsSecretChecksEveryDecodedCausalEventAlias(t *testing.T) {
	value := map[string]any{
		"error": map[string]any{
			"causedByEventId":    "evt_01j99zstagefailed",
			"CAUSED_BY_EVENT_ID": "evt_authorization_Bearer_secret-value",
		},
	}

	for attempt := 0; attempt < 2048; attempt++ {
		if !ContainsSecret(value) {
			t.Fatalf("ContainsSecret() accepted mixed safe/unsafe aliases on attempt %d", attempt)
		}
	}
}

func TestContainsSecretAcceptsOnlySafeDecodedCausalEventAlias(t *testing.T) {
	value := map[string]any{
		"error": map[string]any{
			"CAUSED_BY_EVENT_ID": "evt_01j99zstagefailed",
		},
	}

	for attempt := 0; attempt < 128; attempt++ {
		if ContainsSecret(value) {
			t.Fatalf("ContainsSecret() rejected safe alias on attempt %d", attempt)
		}
	}
}

func TestContainsSecretRejectsNonStringDecodedCausalEventReference(t *testing.T) {
	value := map[string]any{
		"error": map[string]any{
			"causedByEventId": 42,
		},
	}

	if !ContainsSecret(value) {
		t.Fatal("ContainsSecret() accepted non-string causal reference")
	}
}
