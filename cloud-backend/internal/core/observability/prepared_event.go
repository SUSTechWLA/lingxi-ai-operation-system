package observability

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

const preparedEventEnvelopeVersion = "observability.prepared.v1"

// PreparedEvent is an opaque replay capability. Its private method prevents
// callers outside this package from treating an arbitrary Event as prepared.
type PreparedEvent interface {
	preparedEventCapability()
}

type preparedEvent struct {
	ownerUserID string
	event       Event
	serialized  []byte
}

func (*preparedEvent) preparedEventCapability() {}

// PreparedEventBinding ties a persisted capability back to the trusted
// outbox columns used to locate it. Empty optional fields are wildcards; the
// event ID, owner, and component are mandatory.
type PreparedEventBinding struct {
	EventID         string
	OwnerUserID     string
	Component       Component
	Source          Source
	Correlation     Correlation
	Runtime         Runtime
	Privacy         Privacy
	EventType       EventType
	ExecutionStatus ExecutionStatus
	ErrorCode       string
	OccurredAt      time.Time
}

type preparedEventEnvelope struct {
	Version     string `json:"version"`
	OwnerUserID string `json:"ownerUserId"`
	Event       Event  `json:"event"`
	SHA256      string `json:"sha256"`
}

type preparedEventDigestBody struct {
	Version     string `json:"version"`
	OwnerUserID string `json:"ownerUserId"`
	Event       Event  `json:"event"`
}

func newPreparedEvent(ownerUserID string, event Event) (PreparedEvent, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return nil, errors.New("prepared observability event requires a trusted owner")
	}
	if err := validatePreparedEventValue(event); err != nil {
		return nil, err
	}
	body := preparedEventDigestBody{
		Version: preparedEventEnvelopeVersion, OwnerUserID: ownerUserID, Event: event,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal prepared observability digest body: %w", err)
	}
	digest := sha256.Sum256(bodyJSON)
	envelope := preparedEventEnvelope{
		Version: body.Version, OwnerUserID: body.OwnerUserID, Event: body.Event,
		SHA256: hex.EncodeToString(digest[:]),
	}
	serialized, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal prepared observability envelope: %w", err)
	}
	return &preparedEvent{ownerUserID: ownerUserID, event: event, serialized: serialized}, nil
}

// MarshalPreparedEvent returns the stable bytes persisted by durable outboxes.
func MarshalPreparedEvent(prepared PreparedEvent) ([]byte, error) {
	capability, ok := prepared.(*preparedEvent)
	if !ok || capability == nil || len(capability.serialized) == 0 {
		return nil, errors.New("invalid prepared observability capability")
	}
	return append([]byte(nil), capability.serialized...), nil
}

// DecodePreparedEvent is the only persistence decoder. It validates the
// canonical envelope, corruption digest, event registry/redaction contract,
// and trusted outbox bindings before returning a replay capability.
func DecodePreparedEvent(payload []byte, binding PreparedEventBinding) (PreparedEvent, error) {
	if len(payload) == 0 {
		return nil, errors.New("prepared observability payload is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var envelope preparedEventEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode prepared observability envelope: %w", err)
	}
	if err := requirePreparedJSONEOF(decoder); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("canonicalize prepared observability envelope: %w", err)
	}
	if !bytes.Equal(payload, canonical) {
		return nil, errors.New("prepared observability envelope is not canonical")
	}
	if envelope.Version != preparedEventEnvelopeVersion {
		return nil, errors.New("prepared observability envelope version is unsupported")
	}
	if strings.TrimSpace(envelope.OwnerUserID) == "" || envelope.OwnerUserID != strings.TrimSpace(envelope.OwnerUserID) {
		return nil, errors.New("prepared observability owner is invalid")
	}
	bodyJSON, err := json.Marshal(preparedEventDigestBody{
		Version: envelope.Version, OwnerUserID: envelope.OwnerUserID, Event: envelope.Event,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal prepared observability digest body: %w", err)
	}
	digest := sha256.Sum256(bodyJSON)
	wantDigest, err := hex.DecodeString(envelope.SHA256)
	if err != nil || len(wantDigest) != sha256.Size || subtle.ConstantTimeCompare(digest[:], wantDigest) != 1 {
		return nil, errors.New("prepared observability envelope digest mismatch")
	}
	if err := validatePreparedEventValue(envelope.Event); err != nil {
		return nil, err
	}
	if err := validatePreparedEventBinding(envelope, binding); err != nil {
		return nil, err
	}
	return &preparedEvent{
		ownerUserID: envelope.OwnerUserID,
		event:       envelope.Event,
		serialized:  append([]byte(nil), canonical...),
	}, nil
}

func requirePreparedJSONEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("prepared observability envelope has trailing JSON")
	}
	return fmt.Errorf("decode prepared observability envelope trailing data: %w", err)
}

func validatePreparedEventValue(event Event) error {
	if ContainsSecret(event) {
		return errors.New("prepared observability event contains secret material")
	}
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate prepared observability event: %w", err)
	}
	redactedJSON, err := json.Marshal(Redact(event))
	if err != nil {
		return err
	}
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if !bytes.Equal(eventJSON, redactedJSON) {
		return errors.New("prepared observability event is not fully redacted")
	}
	return nil
}

func validatePreparedEventBinding(envelope preparedEventEnvelope, binding PreparedEventBinding) error {
	if binding.EventID == "" || binding.OwnerUserID == "" || binding.Component == "" {
		return errors.New("prepared observability binding requires event ID, owner, and component")
	}
	event := envelope.Event
	if event.EventID != binding.EventID {
		return errors.New("prepared observability event ID binding mismatch")
	}
	if envelope.OwnerUserID != binding.OwnerUserID {
		return errors.New("prepared observability owner binding mismatch")
	}
	if event.Source.Component != string(binding.Component) {
		return errors.New("prepared observability component binding mismatch")
	}
	if err := compareOptionalSource(event.Source, binding.Source); err != nil {
		return err
	}
	if err := compareOptionalCorrelation(event.Correlation, sanitizeCorrelation(binding.Correlation)); err != nil {
		return err
	}
	if err := compareOptionalRuntime(event.Runtime, binding.Runtime); err != nil {
		return err
	}
	if binding.Privacy.Classification != "" && event.Privacy.Classification != binding.Privacy.Classification {
		return errors.New("prepared observability privacy binding mismatch")
	}
	if binding.Privacy.RedactedFields != nil && !slices.Equal(event.Privacy.RedactedFields, binding.Privacy.RedactedFields) {
		return errors.New("prepared observability redacted fields binding mismatch")
	}
	if binding.EventType != "" && event.EventType != binding.EventType {
		return errors.New("prepared observability event type binding mismatch")
	}
	if binding.ExecutionStatus != "" && event.Execution.Status != binding.ExecutionStatus {
		return errors.New("prepared observability execution binding mismatch")
	}
	actualErrorCode := ""
	if event.Error != nil {
		actualErrorCode = event.Error.Code
	}
	if binding.ErrorCode != actualErrorCode {
		return errors.New("prepared observability error binding mismatch")
	}
	if !binding.OccurredAt.IsZero() && !event.OccurredAt.Equal(binding.OccurredAt) {
		return errors.New("prepared observability occurred-at binding mismatch")
	}
	return nil
}

func compareOptionalSource(actual, expected Source) error {
	for _, field := range []struct{ name, actual, expected string }{
		{"service", actual.Service, expected.Service},
		{"component", actual.Component, expected.Component},
		{"environment", actual.Environment, expected.Environment},
	} {
		if field.expected != "" && field.actual != field.expected {
			return fmt.Errorf("prepared observability source %s binding mismatch", field.name)
		}
	}
	return nil
}

func compareOptionalCorrelation(actual, expected Correlation) error {
	for _, field := range []struct{ name, actual, expected string }{
		{"traceId", actual.TraceID, expected.TraceID}, {"spanId", actual.SpanID, expected.SpanID},
		{"parentSpanId", actual.ParentSpanID, expected.ParentSpanID}, {"sessionId", actual.SessionID, expected.SessionID},
		{"projectId", actual.ProjectID, expected.ProjectID}, {"taskId", actual.TaskID, expected.TaskID},
		{"workflowRunId", actual.WorkflowRunID, expected.WorkflowRunID}, {"agentRunId", actual.AgentRunID, expected.AgentRunID},
		{"stageId", actual.StageID, expected.StageID}, {"shotId", actual.ShotID, expected.ShotID},
		{"artifactId", actual.ArtifactID, expected.ArtifactID}, {"toolCallId", actual.ToolCallID, expected.ToolCallID},
		{"providerJobId", actual.ProviderJobID, expected.ProviderJobID},
	} {
		if field.expected != "" && field.actual != field.expected {
			return fmt.Errorf("prepared observability correlation %s binding mismatch", field.name)
		}
	}
	return nil
}

func compareOptionalRuntime(actual, expected Runtime) error {
	for _, field := range []struct{ name, actual, expected string }{
		{"appVersion", actual.AppVersion, expected.AppVersion}, {"gitCommit", actual.GitCommit, expected.GitCommit},
		{"workflowVersion", actual.WorkflowVersion, expected.WorkflowVersion},
		{"toolRegistrySnapshotId", actual.ToolRegistrySnapshotID, expected.ToolRegistrySnapshotID},
		{"promptTemplateVersion", actual.PromptTemplateVersion, expected.PromptTemplateVersion},
		{"provider", actual.Provider, expected.Provider}, {"model", actual.Model, expected.Model},
	} {
		if field.expected != "" && field.actual != field.expected {
			return fmt.Errorf("prepared observability runtime %s binding mismatch", field.name)
		}
	}
	return nil
}
