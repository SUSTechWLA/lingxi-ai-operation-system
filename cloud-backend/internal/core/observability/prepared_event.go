package observability

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/trustedcontext"
)

const (
	legacyPreparedEventVersion      = "observability.prepared.v1"
	persistentPreparedEventVersion  = "observability.prepared.v2"
	maxSealedPreparedEventBytes     = 64 * 1024
	minPersistentSealingKeyBytes    = 32
	maxPersistentSealingKeyBytes    = 64
	maxPersistentSealingDomainBytes = 128
	maxPreviousSourceEnvironments   = 8
	maxSourceEnvironmentBytes       = 64
)

var persistentSealingDomainPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)
var sourceEnvironmentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// PersistentSealingConfig identifies one durable sealing domain. Key must be
// stable across process restarts and is copied into private emitter state.
type PersistentSealingConfig struct {
	Domain                     string
	Key                        string
	PreviousSourceEnvironments []string
}

// PreparedEvent is an opaque replay capability. Only a persistent component
// emitter can create or restore an implementation.
type PreparedEvent interface {
	preparedEventCapability()
}

type preparedEvent struct {
	sealed      SealedPreparedEvent
	ownerUserID string
	event       Event
	domainProof [sha256.Size]byte
}

func (*preparedEvent) preparedEventCapability() {}

// PreparedEventBinding duplicates the trusted outbox fields that must exactly
// match the authenticated event before any callback or sink side effect.
type PreparedEventBinding struct {
	SchemaVersion   string          `json:"schemaVersion"`
	EventID         string          `json:"eventId"`
	OwnerUserID     string          `json:"ownerUserId"`
	Source          Source          `json:"source"`
	Correlation     Correlation     `json:"correlation"`
	Runtime         Runtime         `json:"runtime"`
	Privacy         Privacy         `json:"privacy"`
	Severity        Severity        `json:"severity"`
	EventType       EventType       `json:"eventType"`
	ExecutionStatus ExecutionStatus `json:"executionStatus"`
	ErrorCode       string          `json:"errorCode,omitempty"`
	OccurredAt      time.Time       `json:"occurredAt"`
}

// SealedPreparedEvent is safe to persist but not trusted until the originating
// sealing domain restores it. Payload is authenticated; Binding is checked
// exactly against the authenticated event.
type SealedPreparedEvent struct {
	Payload []byte               `json:"payload"`
	Binding PreparedEventBinding `json:"binding"`
}

func (s SealedPreparedEvent) Clone() SealedPreparedEvent {
	clone := s
	clone.Payload = append([]byte(nil), s.Payload...)
	clone.Binding.Privacy.RedactedFields = append([]string(nil), s.Binding.Privacy.RedactedFields...)
	return clone
}

type persistentSealer struct {
	domain                     string
	key                        []byte
	previousSourceEnvironments map[string]struct{}
}

type persistentPreparedEnvelope struct {
	Version         string    `json:"version"`
	Domain          string    `json:"domain"`
	OwnerUserID     string    `json:"ownerUserId"`
	ProducerSource  Source    `json:"producerSource"`
	ProducerRuntime Runtime   `json:"producerRuntime"`
	Component       Component `json:"component"`
	Event           Event     `json:"event"`
	HMACSHA256      string    `json:"hmacSha256"`
}

type persistentPreparedBody struct {
	Version         string    `json:"version"`
	Domain          string    `json:"domain"`
	OwnerUserID     string    `json:"ownerUserId"`
	ProducerSource  Source    `json:"producerSource"`
	ProducerRuntime Runtime   `json:"producerRuntime"`
	Component       Component `json:"component"`
	Event           Event     `json:"event"`
}

type legacyPreparedEnvelope struct {
	Version     string `json:"version"`
	OwnerUserID string `json:"ownerUserId"`
	Event       Event  `json:"event"`
	SHA256      string `json:"sha256"`
}

type legacyPreparedBody struct {
	Version     string `json:"version"`
	OwnerUserID string `json:"ownerUserId"`
	Event       Event  `json:"event"`
}

type persistentComponentEmitter struct {
	emitter   *Emitter
	component Component
}

func newPersistentSealer(config PersistentSealingConfig) (*persistentSealer, error) {
	domain := strings.TrimSpace(config.Domain)
	if domain == "" || domain != config.Domain || len(domain) > maxPersistentSealingDomainBytes ||
		!persistentSealingDomainPattern.MatchString(domain) {
		return nil, errors.New("persistent observability sealing domain is invalid")
	}
	key, err := ParsePersistentSealingKey(config.Key)
	if err != nil {
		return nil, err
	}
	if len(config.PreviousSourceEnvironments) > maxPreviousSourceEnvironments {
		return nil, fmt.Errorf("persistent observability previous source environment allowlist exceeds %d entries", maxPreviousSourceEnvironments)
	}
	previous := make(map[string]struct{}, len(config.PreviousSourceEnvironments))
	for _, environment := range config.PreviousSourceEnvironments {
		if err := validateSourceEnvironment(environment); err != nil {
			return nil, fmt.Errorf("persistent observability previous source environment is invalid: %w", err)
		}
		if _, exists := previous[environment]; exists {
			return nil, errors.New("persistent observability previous source environment allowlist contains duplicates")
		}
		previous[environment] = struct{}{}
	}
	return &persistentSealer{domain: domain, key: key, previousSourceEnvironments: previous}, nil
}

func validateSourceEnvironment(environment string) error {
	if environment == "" || strings.TrimSpace(environment) != environment ||
		len(environment) > maxSourceEnvironmentBytes || !sourceEnvironmentPattern.MatchString(environment) {
		return errors.New("source environment must be a canonical 1-64 byte identifier")
	}
	return nil
}

func (s *persistentSealer) allowsPreviousSourceEnvironment(environment string) bool {
	_, ok := s.previousSourceEnvironments[environment]
	return ok
}

// ParsePersistentSealingKey is the single production parser for durable
// observability keys. The textual transport is deliberately explicit so a
// human-looking password cannot be mistaken for random key material.
func ParsePersistentSealingKey(encoded string) ([]byte, error) {
	const prefix = "base64:"
	if encoded == "" || strings.TrimSpace(encoded) != encoded || !strings.HasPrefix(encoded, prefix) {
		return nil, errors.New("persistent observability sealing key must use canonical base64: format")
	}
	transport := strings.TrimPrefix(encoded, prefix)
	if transport == "" || strings.IndexFunc(transport, func(r rune) bool { return r == ' ' || r == '\t' || r == '\r' || r == '\n' }) >= 0 {
		return nil, errors.New("persistent observability sealing key has invalid base64 transport")
	}
	key, err := base64.StdEncoding.Strict().DecodeString(transport)
	if err != nil || base64.StdEncoding.EncodeToString(key) != transport {
		return nil, errors.New("persistent observability sealing key has invalid base64 transport")
	}
	if len(key) < minPersistentSealingKeyBytes || len(key) > maxPersistentSealingKeyBytes {
		return nil, fmt.Errorf("persistent observability sealing key must decode to %d-%d bytes", minPersistentSealingKeyBytes, maxPersistentSealingKeyBytes)
	}
	seen := make(map[byte]struct{}, len(key))
	for _, value := range key {
		seen[value] = struct{}{}
	}
	if len(seen) < 16 || isRepeatedBytePattern(key) {
		return nil, errors.New("persistent observability sealing key must contain non-repeating random bytes")
	}
	lowerKeyText := strings.ToLower(string(key))
	for _, marker := range []string{"replace-with", "placeholder", "changeme", "development-only"} {
		if strings.Contains(lowerKeyText, marker) {
			return nil, errors.New("persistent observability sealing key must not contain placeholder text")
		}
	}
	return append([]byte(nil), key...), nil
}

func isRepeatedBytePattern(key []byte) bool {
	for blockSize := 1; blockSize <= len(key)/2; blockSize++ {
		if len(key)%blockSize != 0 {
			continue
		}
		repeated := true
		for index := blockSize; index < len(key); index++ {
			if key[index] != key[index%blockSize] {
				repeated = false
				break
			}
		}
		if repeated {
			return true
		}
	}
	return false
}

func (e *Emitter) ForPersistentComponent(component Component) (PersistentPreparedEventEmitter, error) {
	if e == nil || e.persistentSealer == nil {
		return nil, errors.New("persistent prepared observability is not configured")
	}
	if component == "" {
		return nil, errors.New("persistent prepared observability requires a component")
	}
	emitter := &persistentComponentEmitter{emitter: e, component: component}
	if err := emitter.ValidatePersistentConfiguration(); err != nil {
		return nil, err
	}
	return emitter, nil
}

func (e *persistentComponentEmitter) Emit(ctx context.Context, event Event) error {
	if err := e.ValidatePersistentConfiguration(); err != nil {
		return err
	}
	return e.emitter.emit(ctx, event, e.component, nil)
}

func (e *persistentComponentEmitter) EmitAndWait(ctx context.Context, event Event) error {
	if err := e.ValidatePersistentConfiguration(); err != nil {
		return err
	}
	return e.emitter.emitAndWait(ctx, event, e.component)
}

func (e *persistentComponentEmitter) ValidatePersistentConfiguration() error {
	if e == nil || e.emitter == nil || e.emitter.persistentSealer == nil {
		return errors.New("persistent prepared observability is not configured")
	}
	if e.component == "" {
		return errors.New("persistent prepared observability requires a component")
	}
	if e.emitter.source.Service == "" || e.emitter.source.Component == "" || e.emitter.source.Environment == "" {
		return errors.New("persistent prepared observability requires complete parent source identity")
	}
	return nil
}

func (e *persistentComponentEmitter) FreezeAndSeal(ctx context.Context, event Event) (SealedPreparedEvent, error) {
	if err := e.ValidatePersistentConfiguration(); err != nil {
		return SealedPreparedEvent{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	prepared, err := e.emitter.prepare(ctx, event, e.component)
	if err != nil {
		return SealedPreparedEvent{}, err
	}
	ownerUserID, _ := trustedcontext.UserID(ctx)
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return SealedPreparedEvent{}, errors.New("prepared observability event requires a trusted owner")
	}
	if err := validatePreparedEventValue(prepared); err != nil {
		return SealedPreparedEvent{}, err
	}
	body := persistentPreparedBody{
		Version: persistentPreparedEventVersion, Domain: e.emitter.persistentSealer.domain,
		OwnerUserID: ownerUserID, ProducerSource: e.emitter.source, ProducerRuntime: e.emitter.runtime,
		Component: e.component, Event: prepared,
	}
	payload, err := e.emitter.persistentSealer.seal(body)
	if err != nil {
		return SealedPreparedEvent{}, err
	}
	sealed := SealedPreparedEvent{Payload: payload, Binding: preparedEventBinding(ownerUserID, prepared)}
	return sealed.Clone(), nil
}

// MigrateClaimedLegacyPreparedEvent is a narrow compatibility bridge for a
// trusted, already-claimed durable row. It never returns the legacy Event or a
// replay capability; successful validation only yields a current HMAC seal.
func (e *persistentComponentEmitter) MigrateClaimedLegacyPreparedEvent(
	ctx context.Context,
	payload []byte,
	expected Event,
) (SealedPreparedEvent, error) {
	if err := e.ValidatePersistentConfiguration(); err != nil {
		return SealedPreparedEvent{}, err
	}
	envelope, err := decodeLegacyPreparedEnvelope(payload)
	if err != nil {
		return SealedPreparedEvent{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ownerUserID, _ := trustedcontext.UserID(ctx)
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" || envelope.OwnerUserID != ownerUserID {
		return SealedPreparedEvent{}, errors.New("legacy prepared observability owner binding mismatch")
	}
	currentSource := e.emitter.source
	currentSource.Component = string(e.component)
	historicalSource := envelope.Event.Source
	if historicalSource.Service != currentSource.Service || historicalSource.Component != currentSource.Component {
		return SealedPreparedEvent{}, errors.New("legacy prepared observability source binding mismatch")
	}
	if historicalSource.Environment != currentSource.Environment &&
		!e.emitter.persistentSealer.allowsPreviousSourceEnvironment(historicalSource.Environment) {
		return SealedPreparedEvent{}, errors.New("legacy prepared observability source environment is not allowed")
	}
	if expected.Runtime.ToolRegistrySnapshotID != envelope.Event.Runtime.ToolRegistrySnapshotID {
		return SealedPreparedEvent{}, errors.New("legacy prepared observability tool registry binding mismatch")
	}
	// Ingest time, sequence, and producer runtime are authenticated historical
	// evidence. Every terminal-owned field is rebuilt from the claimed row and
	// compared as one complete Event value.
	expected.SchemaVersion = "1.0"
	expected.IngestedAt = envelope.Event.IngestedAt
	expected.ProducerSequence = envelope.Event.ProducerSequence
	expected.Source = historicalSource
	expected.Runtime = envelope.Event.Runtime
	expected.Correlation = sanitizeCorrelation(expected.Correlation)
	if expected.Execution.Attempt == 0 {
		expected.Execution.Attempt = 1
	}
	if expected.Privacy.Classification == "" {
		expected.Privacy.Classification = PrivacyInternal
	}
	if expected.Privacy.RedactedFields == nil {
		expected.Privacy.RedactedFields = []string{}
	}
	expected = Redact(expected)
	if err := validatePreparedEventValue(expected); err != nil {
		return SealedPreparedEvent{}, fmt.Errorf("validate expected legacy observability event: %w", err)
	}
	if !reflect.DeepEqual(envelope.Event, expected) {
		return SealedPreparedEvent{}, errors.New("legacy prepared observability terminal binding mismatch")
	}
	migratedEvent := envelope.Event
	migratedEvent.Source = currentSource
	body := persistentPreparedBody{
		Version: persistentPreparedEventVersion, Domain: e.emitter.persistentSealer.domain,
		OwnerUserID: ownerUserID, ProducerSource: e.emitter.source, ProducerRuntime: envelope.Event.Runtime,
		Component: e.component, Event: migratedEvent,
	}
	sealedPayload, err := e.emitter.persistentSealer.seal(body)
	if err != nil {
		return SealedPreparedEvent{}, err
	}
	return SealedPreparedEvent{
		Payload: sealedPayload,
		Binding: preparedEventBinding(ownerUserID, migratedEvent),
	}.Clone(), nil
}

// MigrateClaimedPreparedEventSource authenticates a current v2 envelope before
// deciding whether it needs a one-time source-environment reseal. The HMAC,
// sealing domain, owner, complete signed source identities, component, trusted
// binding, and claimed-row binding all pass before any new envelope is issued.
// Only the environment may change; service and component identity stay exact.
func (e *persistentComponentEmitter) MigrateClaimedPreparedEventSource(
	ctx context.Context,
	sealed SealedPreparedEvent,
	expected Event,
) (SealedPreparedEvent, bool, error) {
	if err := e.ValidatePersistentConfiguration(); err != nil {
		return SealedPreparedEvent{}, false, err
	}
	envelope, err := e.emitter.persistentSealer.open(sealed.Payload)
	if err != nil {
		return SealedPreparedEvent{}, false, err
	}
	if envelope.Component != e.component {
		return SealedPreparedEvent{}, false, errors.New("prepared observability component domain mismatch")
	}
	currentProducer := e.emitter.source
	if envelope.ProducerSource.Service != currentProducer.Service ||
		envelope.ProducerSource.Component != currentProducer.Component {
		return SealedPreparedEvent{}, false, errors.New("prepared observability parent source identity mismatch")
	}
	historicalEventSource := envelope.ProducerSource
	historicalEventSource.Component = string(e.component)
	if envelope.Event.Source != historicalEventSource {
		return SealedPreparedEvent{}, false, errors.New("prepared observability event source binding mismatch")
	}
	if err := validatePreparedEventValue(envelope.Event); err != nil {
		return SealedPreparedEvent{}, false, err
	}
	wantBinding := preparedEventBinding(envelope.OwnerUserID, envelope.Event)
	if !reflect.DeepEqual(sealed.Binding, wantBinding) {
		return SealedPreparedEvent{}, false, errors.New("prepared observability trusted binding mismatch")
	}
	if err := e.validateExpectedPreparedEventBinding(ctx, sealed.Binding, expected, historicalEventSource); err != nil {
		return SealedPreparedEvent{}, false, err
	}
	if envelope.ProducerSource.Environment == currentProducer.Environment {
		return sealed.Clone(), false, nil
	}
	if !e.emitter.persistentSealer.allowsPreviousSourceEnvironment(envelope.ProducerSource.Environment) {
		return SealedPreparedEvent{}, false, errors.New("prepared observability source environment is not allowed")
	}

	migratedEvent := envelope.Event
	migratedEvent.Source = currentProducer
	migratedEvent.Source.Component = string(e.component)
	body := persistentPreparedBody{
		Version: persistentPreparedEventVersion, Domain: e.emitter.persistentSealer.domain,
		OwnerUserID: envelope.OwnerUserID, ProducerSource: currentProducer, ProducerRuntime: envelope.ProducerRuntime,
		Component: e.component, Event: migratedEvent,
	}
	payload, err := e.emitter.persistentSealer.seal(body)
	if err != nil {
		return SealedPreparedEvent{}, false, err
	}
	return SealedPreparedEvent{
		Payload: payload,
		Binding: preparedEventBinding(envelope.OwnerUserID, migratedEvent),
	}.Clone(), true, nil
}

func decodeLegacyPreparedEnvelope(payload []byte) (legacyPreparedEnvelope, error) {
	if len(payload) == 0 {
		return legacyPreparedEnvelope{}, errors.New("legacy prepared observability payload is empty")
	}
	if len(payload) > maxSealedPreparedEventBytes {
		return legacyPreparedEnvelope{}, errors.New("legacy prepared observability payload exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var envelope legacyPreparedEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return legacyPreparedEnvelope{}, fmt.Errorf("decode legacy prepared observability envelope: %w", err)
	}
	if err := requirePreparedJSONEOF(decoder); err != nil {
		return legacyPreparedEnvelope{}, err
	}
	canonical, err := json.Marshal(envelope)
	if err != nil {
		return legacyPreparedEnvelope{}, fmt.Errorf("canonicalize legacy prepared observability envelope: %w", err)
	}
	if !bytes.Equal(payload, canonical) {
		return legacyPreparedEnvelope{}, errors.New("legacy prepared observability envelope is not canonical")
	}
	if envelope.Version != legacyPreparedEventVersion {
		return legacyPreparedEnvelope{}, errors.New("legacy prepared observability envelope version is unsupported")
	}
	if strings.TrimSpace(envelope.OwnerUserID) == "" || strings.TrimSpace(envelope.OwnerUserID) != envelope.OwnerUserID {
		return legacyPreparedEnvelope{}, errors.New("legacy prepared observability owner is invalid")
	}
	bodyJSON, err := json.Marshal(legacyPreparedBody{
		Version: envelope.Version, OwnerUserID: envelope.OwnerUserID, Event: envelope.Event,
	})
	if err != nil {
		return legacyPreparedEnvelope{}, fmt.Errorf("marshal legacy prepared observability digest body: %w", err)
	}
	want, err := hex.DecodeString(envelope.SHA256)
	if err != nil || len(want) != sha256.Size {
		return legacyPreparedEnvelope{}, errors.New("legacy prepared observability digest is invalid")
	}
	got := sha256.Sum256(bodyJSON)
	if subtle.ConstantTimeCompare(got[:], want) != 1 {
		return legacyPreparedEnvelope{}, errors.New("legacy prepared observability digest mismatch")
	}
	if err := validatePreparedEventValue(envelope.Event); err != nil {
		return legacyPreparedEnvelope{}, err
	}
	return envelope, nil
}

func (e *persistentComponentEmitter) RestorePreparedEvent(sealed SealedPreparedEvent) (PreparedEvent, error) {
	if err := e.ValidatePersistentConfiguration(); err != nil {
		return nil, err
	}
	envelope, err := e.emitter.persistentSealer.open(sealed.Payload)
	if err != nil {
		return nil, err
	}
	if envelope.ProducerSource != e.emitter.source {
		return nil, errors.New("prepared observability parent source domain mismatch")
	}
	if envelope.Component != e.component {
		return nil, errors.New("prepared observability component domain mismatch")
	}
	wantSource := e.emitter.source
	wantSource.Component = string(e.component)
	if envelope.Event.Source != wantSource {
		return nil, errors.New("prepared observability event source binding mismatch")
	}
	if err := validatePreparedEventValue(envelope.Event); err != nil {
		return nil, err
	}
	wantBinding := preparedEventBinding(envelope.OwnerUserID, envelope.Event)
	if !reflect.DeepEqual(sealed.Binding, wantBinding) {
		return nil, errors.New("prepared observability trusted binding mismatch")
	}
	capability := &preparedEvent{
		sealed: sealed.Clone(), ownerUserID: envelope.OwnerUserID, event: envelope.Event,
		domainProof: e.domainProof(),
	}
	return capability, nil
}

// RestorePreparedEventFor additionally proves that the authenticated event is
// the one the caller expects for this trusted context. Durable outboxes use it
// before any callback so a valid envelope copied from another row cannot be
// replayed under a different owner, identity, correlation, runtime, or status.
func (e *persistentComponentEmitter) RestorePreparedEventFor(
	ctx context.Context,
	sealed SealedPreparedEvent,
	expected Event,
) (PreparedEvent, error) {
	capability, err := e.RestorePreparedEvent(sealed)
	if err != nil {
		return nil, err
	}
	wantSource := e.emitter.source
	wantSource.Component = string(e.component)
	if err := e.validateExpectedPreparedEventBinding(ctx, sealed.Binding, expected, wantSource); err != nil {
		return nil, err
	}
	return capability, nil
}

func (e *persistentComponentEmitter) validateExpectedPreparedEventBinding(
	ctx context.Context,
	binding PreparedEventBinding,
	expected Event,
	expectedSource Source,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ownerUserID, _ := trustedcontext.UserID(ctx)
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return errors.New("prepared observability expected event requires a trusted owner")
	}
	// Producer runtime is authenticated historical evidence, not part of the
	// restart key domain. Preserve it while keeping the caller-owned tool
	// snapshot an exact outbox expectation, including its empty value.
	expectedToolSnapshotID := expected.Runtime.ToolRegistrySnapshotID
	expected.Runtime = binding.Runtime
	expected.Runtime.ToolRegistrySnapshotID = expectedToolSnapshotID
	preparedExpected, err := e.emitter.prepare(ctx, expected, e.component)
	if err != nil {
		return fmt.Errorf("prepare expected observability event: %w", err)
	}
	preparedExpected.Source = expectedSource
	preparedExpected.Runtime = expected.Runtime
	if err := validatePreparedEventValue(preparedExpected); err != nil {
		return fmt.Errorf("validate expected observability event: %w", err)
	}
	wantBinding := preparedEventBinding(ownerUserID, preparedExpected)
	if !reflect.DeepEqual(binding, wantBinding) {
		return fmt.Errorf("prepared observability caller binding mismatch: %s", strings.Join(preparedEventBindingMismatchFields(binding, wantBinding), ","))
	}
	return nil
}

func preparedEventBindingMismatchFields(got, want PreparedEventBinding) []string {
	fields := make([]string, 0, 12)
	if got.SchemaVersion != want.SchemaVersion {
		fields = append(fields, "schema")
	}
	if got.EventID != want.EventID {
		fields = append(fields, "event-id")
	}
	if got.OwnerUserID != want.OwnerUserID {
		fields = append(fields, "owner")
	}
	if got.Source != want.Source {
		fields = append(fields, "source")
	}
	if got.Correlation != want.Correlation {
		fields = append(fields, "correlation")
	}
	if got.Runtime != want.Runtime {
		fields = append(fields, "runtime")
	}
	if !reflect.DeepEqual(got.Privacy, want.Privacy) {
		fields = append(fields, "privacy")
	}
	if got.Severity != want.Severity {
		fields = append(fields, "severity")
	}
	if got.EventType != want.EventType {
		fields = append(fields, "event-type")
	}
	if got.ExecutionStatus != want.ExecutionStatus {
		fields = append(fields, "execution-status")
	}
	if got.ErrorCode != want.ErrorCode {
		fields = append(fields, "error-code")
	}
	if !got.OccurredAt.Equal(want.OccurredAt) {
		fields = append(fields, "occurred-at")
	}
	return fields
}

func (e *persistentComponentEmitter) ReplayPreparedAndWait(ctx context.Context, prepared PreparedEvent) error {
	if err := e.ValidatePersistentConfiguration(); err != nil {
		return err
	}
	capability, ok := prepared.(*preparedEvent)
	if !ok || capability == nil {
		return errors.New("invalid prepared observability capability")
	}
	proof := e.domainProof()
	if subtle.ConstantTimeCompare(proof[:], capability.domainProof[:]) != 1 {
		return errors.New("prepared observability capability domain mismatch")
	}
	restored, err := e.RestorePreparedEvent(capability.sealed)
	if err != nil {
		return err
	}
	validated := restored.(*preparedEvent)
	if ctx == nil {
		ctx = context.Background()
	}
	result := make(chan error, 1)
	if err := e.emitter.enqueuePreparedForOwner(ctx, validated.event, validated.ownerUserID, result); err != nil {
		return err
	}
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *persistentSealer) seal(body persistentPreparedBody) ([]byte, error) {
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal prepared observability body: %w", err)
	}
	signature := s.sign(bodyJSON)
	envelope := persistentPreparedEnvelope{
		Version: body.Version, Domain: body.Domain, OwnerUserID: body.OwnerUserID,
		ProducerSource: body.ProducerSource, ProducerRuntime: body.ProducerRuntime,
		Component: body.Component, Event: body.Event, HMACSHA256: hex.EncodeToString(signature[:]),
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal prepared observability envelope: %w", err)
	}
	if len(payload) > maxSealedPreparedEventBytes {
		return nil, errors.New("prepared observability payload exceeds size limit")
	}
	return payload, nil
}

func (s *persistentSealer) open(payload []byte) (persistentPreparedEnvelope, error) {
	if len(payload) == 0 {
		return persistentPreparedEnvelope{}, errors.New("prepared observability payload is empty")
	}
	if len(payload) > maxSealedPreparedEventBytes {
		return persistentPreparedEnvelope{}, errors.New("prepared observability payload exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var envelope persistentPreparedEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return persistentPreparedEnvelope{}, fmt.Errorf("decode prepared observability envelope: %w", err)
	}
	if err := requirePreparedJSONEOF(decoder); err != nil {
		return persistentPreparedEnvelope{}, err
	}
	canonical, err := json.Marshal(envelope)
	if err != nil {
		return persistentPreparedEnvelope{}, fmt.Errorf("canonicalize prepared observability envelope: %w", err)
	}
	if !bytes.Equal(payload, canonical) {
		return persistentPreparedEnvelope{}, errors.New("prepared observability envelope is not canonical")
	}
	if envelope.Version != persistentPreparedEventVersion {
		return persistentPreparedEnvelope{}, errors.New("prepared observability envelope version is unsupported")
	}
	if envelope.Domain != s.domain {
		return persistentPreparedEnvelope{}, errors.New("prepared observability sealing domain mismatch")
	}
	body := persistentPreparedBody{
		Version: envelope.Version, Domain: envelope.Domain, OwnerUserID: envelope.OwnerUserID,
		ProducerSource: envelope.ProducerSource, ProducerRuntime: envelope.ProducerRuntime,
		Component: envelope.Component, Event: envelope.Event,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return persistentPreparedEnvelope{}, fmt.Errorf("marshal prepared observability body: %w", err)
	}
	want, err := hex.DecodeString(envelope.HMACSHA256)
	if err != nil || len(want) != sha256.Size {
		return persistentPreparedEnvelope{}, errors.New("prepared observability signature is invalid")
	}
	got := s.sign(bodyJSON)
	if subtle.ConstantTimeCompare(got[:], want) != 1 {
		return persistentPreparedEnvelope{}, errors.New("prepared observability signature mismatch")
	}
	return envelope, nil
}

func (s *persistentSealer) sign(payload []byte) [sha256.Size]byte {
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write(payload)
	var signature [sha256.Size]byte
	copy(signature[:], mac.Sum(nil))
	return signature
}

func (e *persistentComponentEmitter) domainProof() [sha256.Size]byte {
	identity, _ := json.Marshal(struct {
		Purpose   string    `json:"purpose"`
		Domain    string    `json:"domain"`
		Source    Source    `json:"source"`
		Runtime   Runtime   `json:"runtime"`
		Component Component `json:"component"`
	}{
		Purpose: "prepared-capability-domain-v1", Domain: e.emitter.persistentSealer.domain,
		Source: e.emitter.source, Runtime: e.emitter.runtime, Component: e.component,
	})
	return e.emitter.persistentSealer.sign(identity)
}

func preparedEventBinding(ownerUserID string, event Event) PreparedEventBinding {
	errorCode := ""
	if event.Error != nil {
		errorCode = event.Error.Code
	}
	privacy := event.Privacy
	privacy.RedactedFields = append([]string(nil), event.Privacy.RedactedFields...)
	return PreparedEventBinding{
		SchemaVersion: event.SchemaVersion, EventID: event.EventID, OwnerUserID: ownerUserID,
		Source: event.Source, Correlation: event.Correlation, Runtime: event.Runtime, Privacy: privacy,
		Severity: event.Severity, EventType: event.EventType, ExecutionStatus: event.Execution.Status,
		ErrorCode: errorCode, OccurredAt: event.OccurredAt.UTC(),
	}
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
