package agentruntime

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunTerminalEventJSONPreservesFutureTopAndPreparedV2Fields(t *testing.T) {
	payload := []byte(`{
  "eventId":"evt_terminal_future",
  "callbackIdempotencyKey":"evt_terminal_future",
  "runId":"agr_terminal_future",
  "status":"SUCCESS",
  "occurredAt":"2026-07-28T01:02:03Z",
  "preparedObservability":{
    "payload":"c2VhbGVkLWJlZm9yZQ==",
    "binding":{
      "schemaVersion":"1.0",
      "eventId":"evt_terminal_future",
      "ownerUserId":"usr_future",
      "source":{"service":"cloud-backend","component":"agent-runtime","environment":"development","futureSource":{"mode":"signed"}},
      "correlation":{"traceId":"trc_future","spanId":"spn_future"},
      "runtime":{"appVersion":"v1","futureRuntime":[1,{"exact":9007199254740993}]},
      "privacy":{"classification":"INTERNAL","redactedFields":[],"futurePrivacy":null},
      "severity":"INFO",
      "eventType":"agent.run.completed",
      "executionStatus":"COMPLETED",
      "occurredAt":"2026-07-28T01:02:03Z",
      "futureBinding":{"array":[null,true,{"kind":"future"}]}
    },
    "futureV2":{"object":{"largeInteger":9007199254740993123456789},"array":[1,2,3]},
    "futureNull":null
  },
  "futureTop":{"object":{"largeInteger":9007199254740993123456789},"array":[null,{"enabled":true}]},
  "futureArray":[1,{"nested":"kept"}],
  "futureNull":null
}`)

	var event RunTerminalEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	remarshaled, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	assertTerminalFutureJSON(t, remarshaled)

	// A claimed migration changes authenticated known fields. Future v2 fields
	// must survive that surgical replacement and cannot override the new value.
	event.PreparedObservability.Payload = []byte("sealed-after")
	remarshaled, err = json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	assertTerminalFutureJSON(t, remarshaled)
	var top map[string]json.RawMessage
	if err := json.Unmarshal(remarshaled, &top); err != nil {
		t.Fatal(err)
	}
	var prepared struct {
		Payload []byte `json:"payload"`
	}
	if err := json.Unmarshal(top["preparedObservability"], &prepared); err != nil {
		t.Fatal(err)
	}
	if string(prepared.Payload) != "sealed-after" {
		t.Fatalf("known migrated payload was not authoritative: %q", prepared.Payload)
	}
}

func TestRunTerminalEventJSONRejectsDuplicateKeysAtEveryDepth(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload string
	}{
		{name: "top", payload: `{"eventId":"evt_one","eventId":"evt_two"}`},
		{name: "prepared", payload: `{"preparedObservability":{"payload":"YQ==","payload":"Yg=="}}`},
		{name: "unknown object", payload: `{"future":{"same":1,"same":2}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var event RunTerminalEvent
			if err := json.Unmarshal([]byte(test.payload), &event); err == nil || !strings.Contains(err.Error(), "duplicate") {
				t.Fatalf("duplicate JSON error = %v", err)
			}
		})
	}
}

func TestRunTerminalEventJSONRejectsKnownAliasAfterFutureField(t *testing.T) {
	var event RunTerminalEvent
	err := json.Unmarshal([]byte(`{"future":{"kept":true},"EventId":"evt_alias_injection"}`), &event)
	if err == nil || !strings.Contains(err.Error(), "noncanonical known key") {
		t.Fatalf("known alias injection error = %v", err)
	}
}

func TestRunTerminalEventJSONEnforcesSizeAndDepthBounds(t *testing.T) {
	t.Run("bounded large future field", func(t *testing.T) {
		payload := []byte(`{"future":"` + strings.Repeat("x", 128*1024) + `"}`)
		var event RunTerminalEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatalf("bounded future field rejected: %v", err)
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(encoded, []byte(strings.Repeat("x", 128*1024))) {
			t.Fatal("bounded future field was not preserved")
		}
	})

	t.Run("oversized", func(t *testing.T) {
		payload := []byte(`{"future":"` + strings.Repeat("x", 256*1024) + `"}`)
		var event RunTerminalEvent
		if err := json.Unmarshal(payload, &event); err == nil || !strings.Contains(err.Error(), "size limit") {
			t.Fatalf("oversized JSON error = %v", err)
		}
	})

	t.Run("excessive depth", func(t *testing.T) {
		payload := []byte(`{"future":` + strings.Repeat("[", 40) + `null` + strings.Repeat("]", 40) + `}`)
		var event RunTerminalEvent
		if err := json.Unmarshal(payload, &event); err == nil || !strings.Contains(err.Error(), "depth limit") {
			t.Fatalf("deep JSON error = %v", err)
		}
	})
}

func assertTerminalFutureJSON(t *testing.T, payload []byte) {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal(payload, &top); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"futureTop", "futureArray", "futureNull"} {
		if _, ok := top[field]; !ok {
			t.Fatalf("future top-level field %q was lost: %s", field, payload)
		}
	}
	if !bytes.Contains(top["futureTop"], []byte("9007199254740993123456789")) {
		t.Fatalf("large future number changed: %s", top["futureTop"])
	}
	var prepared map[string]json.RawMessage
	if err := json.Unmarshal(top["preparedObservability"], &prepared); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"futureV2", "futureNull"} {
		if _, ok := prepared[field]; !ok {
			t.Fatalf("future prepared field %q was lost: %s", field, prepared)
		}
	}
	var binding map[string]json.RawMessage
	if err := json.Unmarshal(prepared["binding"], &binding); err != nil {
		t.Fatal(err)
	}
	if _, ok := binding["futureBinding"]; !ok {
		t.Fatalf("future binding field was lost: %s", prepared["binding"])
	}
	var source map[string]json.RawMessage
	if err := json.Unmarshal(binding["source"], &source); err != nil {
		t.Fatal(err)
	}
	if _, ok := source["futureSource"]; !ok {
		t.Fatalf("future source field was lost: %s", binding["source"])
	}
	var runtime map[string]json.RawMessage
	if err := json.Unmarshal(binding["runtime"], &runtime); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(runtime["futureRuntime"], []byte("9007199254740993")) {
		t.Fatalf("future runtime field changed: %s", binding["runtime"])
	}
	var privacy map[string]json.RawMessage
	if err := json.Unmarshal(binding["privacy"], &privacy); err != nil {
		t.Fatal(err)
	}
	if value, ok := privacy["futurePrivacy"]; !ok || string(value) != "null" {
		t.Fatalf("future privacy null was lost: %s", binding["privacy"])
	}
}
