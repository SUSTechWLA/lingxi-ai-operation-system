package observability

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validEvent() Event {
	duration := int64(1240)
	size := int64(2048)
	return Event{
		SchemaVersion:    "1.0",
		EventID:          "evt_01j99zstagefailed",
		OccurredAt:       time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC),
		IngestedAt:       time.Date(2026, 7, 27, 12, 0, 0, 40_000_000, time.UTC),
		ProducerSequence: 1842,
		Severity:         SeverityError,
		EventType:        EventTypeWorkflowStageFailed,
		MessageKey:       "workflow.stage.failed",
		Source: Source{
			Service:     "cloud-backend",
			Component:   "workflow-runner",
			Environment: "test",
		},
		Correlation: Correlation{
			TraceID:       "trc_01j99zworkflow",
			SpanID:        "spn_01j99zstage",
			WorkflowRunID: "wfr_01j99zrun",
			StageID:       "render",
		},
		Execution: Execution{
			Status:     ExecutionStatusFailed,
			Attempt:    1,
			DurationMs: &duration,
		},
		Runtime: Runtime{
			AppVersion:             "0.2.1",
			WorkflowVersion:        "v1",
			ToolRegistrySnapshotID: "trs_01j99zsnapshot",
			Provider:               "local-renderer",
		},
		Evidence: Evidence{
			InputRefs:  []string{"artifact://art_01j99zinput"},
			OutputRefs: []string{},
			InputHash:  strings.Repeat("0", 64),
			SizeBytes:  &size,
		},
		Error: NormalizeError(
			"RENDER.FFMPEG.CODEC_UNSUPPORTED",
			nil,
			"workflow-runner",
			"",
		),
		Privacy: Privacy{
			Classification: PrivacyInternal,
			RedactedFields: []string{},
		},
	}
}

func TestEventValidateAcceptsContractCompatibleEvent(t *testing.T) {
	if err := validEvent().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestEventValidateRejectsInvalidContractFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Event)
		want   string
	}{
		{"schema version", func(e *Event) { e.SchemaVersion = "2.0" }, "schemaVersion"},
		{"event id", func(e *Event) { e.EventID = "event_1" }, "eventId"},
		{"occurred at", func(e *Event) { e.OccurredAt = time.Time{} }, "occurredAt"},
		{"ingested at", func(e *Event) { e.IngestedAt = time.Time{} }, "ingestedAt"},
		{"producer sequence", func(e *Event) { e.ProducerSequence = -1 }, "producerSequence"},
		{"severity", func(e *Event) { e.Severity = "CRITICAL" }, "severity"},
		{"closed event type", func(e *Event) { e.EventType = "workflow.stage.exploded" }, "eventType"},
		{"message key", func(e *Event) { e.MessageKey = "Workflow failed" }, "messageKey"},
		{"source service", func(e *Event) { e.Source.Service = "" }, "source.service"},
		{"source component", func(e *Event) { e.Source.Component = "" }, "source.component"},
		{"source environment", func(e *Event) { e.Source.Environment = "" }, "source.environment"},
		{"trace id", func(e *Event) { e.Correlation.TraceID = "trace_1" }, "correlation.traceId"},
		{"span id", func(e *Event) { e.Correlation.SpanID = "span_1" }, "correlation.spanId"},
		{"optional correlation id", func(e *Event) { e.Correlation.TaskID = "task_1" }, "correlation.taskId"},
		{"empty stage id", func(e *Event) { e.Correlation.StageID = " " }, "correlation.stageId"},
		{"execution status", func(e *Event) { e.Execution.Status = "BROKEN" }, "execution.status"},
		{"execution attempt", func(e *Event) { e.Execution.Attempt = 0 }, "execution.attempt"},
		{"negative duration", func(e *Event) { value := int64(-1); e.Execution.DurationMs = &value }, "execution.durationMs"},
		{"input ref", func(e *Event) { e.Evidence.InputRefs = []string{"https://example.test/input"} }, "evidence.inputRefs"},
		{"input hash", func(e *Event) { e.Evidence.InputHash = "not-a-hash" }, "evidence.inputHash"},
		{"negative size", func(e *Event) { value := int64(-1); e.Evidence.SizeBytes = &value }, "evidence.sizeBytes"},
		{"error code", func(e *Event) { e.Error.Code = "UNKNOWN.ERROR.CODE" }, "error.code"},
		{"error metadata mismatch", func(e *Event) { e.Error.Retryable = true }, "error.retryable"},
		{"error fingerprint", func(e *Event) { e.Error.Fingerprint = "short" }, "error.fingerprint"},
		{"error cause", func(e *Event) { e.Error.CausedByEventID = "event_1" }, "error.causedByEventId"},
		{"developer detail", func(e *Event) { e.Error.DeveloperDetail = "dial tcp 127.0.0.1:9001" }, "error.developerDetail"},
		{"error evidence ref", func(e *Event) { e.Error.EvidenceRefs = []string{"/tmp/error.log"} }, "error.evidenceRefs"},
		{"protected stack ref", func(e *Event) { e.Error.ProtectedStackRef = "/tmp/stack.log" }, "error.protectedStackRef"},
		{"classification", func(e *Event) { e.Privacy.Classification = "PRIVATE" }, "privacy.classification"},
		{"nil redacted fields", func(e *Event) { e.Privacy.RedactedFields = nil }, "privacy.redactedFields"},
		{"empty redacted field", func(e *Event) { e.Privacy.RedactedFields = []string{""} }, "privacy.redactedFields"},
		{"duplicate redacted field", func(e *Event) { e.Privacy.RedactedFields = []string{"evidence.prompt", "evidence.prompt"} }, "privacy.redactedFields"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := validEvent()
			tt.mutate(&event)
			err := event.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want field %q", err, tt.want)
			}
		})
	}
}

func TestEventValidateRequiresErrorForErrorSeverity(t *testing.T) {
	event := validEvent()
	event.Error = nil

	err := event.Validate()
	if err == nil || !strings.Contains(err.Error(), "error") {
		t.Fatalf("Validate() error = %v, want missing error", err)
	}
}

func TestEventJSONMatchesClosedWireShape(t *testing.T) {
	event := validEvent()
	event.Evidence.Attributes = map[string]any{"authorization": "Bearer raw-secret"}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}

	required := []string{
		"schemaVersion", "eventId", "occurredAt", "ingestedAt", "producerSequence",
		"severity", "eventType", "messageKey", "source", "correlation", "execution",
		"runtime", "evidence", "error", "privacy",
	}
	for _, field := range required {
		if _, ok := wire[field]; !ok {
			t.Fatalf("serialized event missing required field %q: %s", field, data)
		}
	}
	evidence := wire["evidence"].(map[string]any)
	if _, ok := evidence["attributes"]; ok {
		t.Fatalf("ingress-only evidence attributes leaked to wire: %s", data)
	}
	if strings.Contains(string(data), "raw-secret") {
		t.Fatalf("secret leaked to wire: %s", data)
	}
}
