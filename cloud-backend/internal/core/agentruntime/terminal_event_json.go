package agentruntime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/tangying-ai/aios-core/internal/core/observability"
)

const (
	maxTerminalEventJSONBytes  = 256 * 1024
	maxTerminalEventJSONDepth  = 32
	maxTerminalEventJSONValues = 16 * 1024
)

type terminalJSONSchema map[string]terminalJSONSchema

var terminalPreparedBindingSchema = terminalJSONSchema{
	"schemaVersion": nil,
	"eventId":       nil,
	"ownerUserId":   nil,
	"source": {
		"service": nil, "component": nil, "environment": nil,
	},
	"correlation": {
		"traceId": nil, "spanId": nil, "parentSpanId": nil, "sessionId": nil,
		"projectId": nil, "taskId": nil, "workflowRunId": nil, "agentRunId": nil,
		"stageId": nil, "shotId": nil, "artifactId": nil, "toolCallId": nil, "providerJobId": nil,
	},
	"runtime": {
		"appVersion": nil, "gitCommit": nil, "workflowVersion": nil,
		"toolRegistrySnapshotId": nil, "promptTemplateVersion": nil, "provider": nil, "model": nil,
	},
	"privacy": {
		"classification": nil, "redactedFields": nil,
	},
	"severity":        nil,
	"eventType":       nil,
	"executionStatus": nil,
	"errorCode":       nil,
	"occurredAt":      nil,
}

var terminalEventJSONSchema = terminalJSONSchema{
	"eventId":                nil,
	"callbackIdempotencyKey": nil,
	"runId":                  nil,
	"taskId":                 nil,
	"userId":                 nil,
	"traceId":                nil,
	"toolRegistrySnapshotId": nil,
	"status":                 nil,
	"context":                nil,
	"errorCode":              nil,
	"error":                  nil,
	"occurredAt":             nil,
	"preparedObservability": {
		"payload": nil,
		"binding": terminalPreparedBindingSchema,
	},
}

type terminalRawMember struct {
	name  string
	value json.RawMessage
}

type terminalRawObject struct {
	members []terminalRawMember
}

type terminalEventJSONState struct {
	original terminalRawObject
	baseline terminalRawObject
}

// UnmarshalJSON accepts the Round-1 string transport and the current sealed
// object without treating a future terminal schema as a closed struct. Unknown
// raw members are retained for a claimed-row surgical update. The bounded
// validator runs first so duplicate names can never be hidden by encoding/json.
func (event *RunTerminalEvent) UnmarshalJSON(payload []byte) error {
	if err := validateTerminalEventJSON(payload); err != nil {
		return err
	}
	original, err := decodeTerminalRawObject(payload)
	if err != nil {
		return err
	}

	type terminalAlias RunTerminalEvent
	var decoded terminalAlias
	var wire struct {
		Prepared json.RawMessage `json:"preparedObservability"`
		*terminalAlias
	}
	wire.terminalAlias = &decoded
	if err := json.Unmarshal(payload, &wire); err != nil {
		return err
	}
	*event = RunTerminalEvent(decoded)
	event.legacyPreparedObservability = nil
	event.terminalJSONState = nil
	raw := bytes.TrimSpace(wire.Prepared)
	if len(raw) != 0 && !bytes.Equal(raw, []byte("null")) {
		switch raw[0] {
		case '"':
			var legacy []byte
			if err := json.Unmarshal(raw, &legacy); err != nil {
				return fmt.Errorf("decode legacy prepared observability transport: %w", err)
			}
			if len(legacy) == 0 {
				return errors.New("legacy prepared observability payload is empty")
			}
			event.legacyPreparedObservability = append([]byte(nil), legacy...)
		case '{':
			var sealed observability.SealedPreparedEvent
			if err := json.Unmarshal(raw, &sealed); err != nil {
				return fmt.Errorf("decode sealed prepared observability: %w", err)
			}
			event.PreparedObservability = &sealed
		default:
			return errors.New("prepared observability JSON type is unsupported")
		}
	}

	hasUnknown, err := terminalRawObjectHasUnknown(original, terminalEventJSONSchema)
	if err != nil {
		return err
	}
	if hasUnknown {
		baselineJSON, err := marshalTerminalEventKnown(*event)
		if err != nil {
			return err
		}
		baseline, err := decodeTerminalRawObject(baselineJSON)
		if err != nil {
			return err
		}
		event.terminalJSONState = &terminalEventJSONState{original: original, baseline: baseline}
	}
	return nil
}

func (event RunTerminalEvent) MarshalJSON() ([]byte, error) {
	known, err := marshalTerminalEventKnown(event)
	if err != nil {
		return nil, err
	}
	if event.terminalJSONState == nil {
		if len(known) > maxTerminalEventJSONBytes {
			return nil, errors.New("terminal event JSON exceeds size limit")
		}
		return known, nil
	}
	current, err := decodeTerminalRawObject(known)
	if err != nil {
		return nil, err
	}
	merged, err := mergeTerminalRootObject(
		event.terminalJSONState.original,
		event.terminalJSONState.baseline,
		current,
		terminalEventJSONSchema,
	)
	if err != nil {
		return nil, err
	}
	result, err := encodeTerminalRawObject(merged)
	if err != nil {
		return nil, err
	}
	if len(result) > maxTerminalEventJSONBytes {
		return nil, errors.New("terminal event JSON exceeds size limit")
	}
	return result, nil
}

func marshalTerminalEventKnown(event RunTerminalEvent) ([]byte, error) {
	type terminalAlias RunTerminalEvent
	var prepared json.RawMessage
	var err error
	switch {
	case len(event.legacyPreparedObservability) > 0 && event.PreparedObservability != nil:
		return nil, errors.New("terminal event has conflicting prepared observability formats")
	case len(event.legacyPreparedObservability) > 0:
		prepared, err = json.Marshal(event.legacyPreparedObservability)
	case event.PreparedObservability != nil:
		prepared, err = json.Marshal(event.PreparedObservability)
	}
	if err != nil {
		return nil, err
	}
	wire := struct {
		Prepared json.RawMessage `json:"preparedObservability,omitempty"`
		terminalAlias
	}{Prepared: prepared, terminalAlias: terminalAlias(event)}
	return json.Marshal(wire)
}

type terminalJSONValidator struct {
	values int
}

func validateTerminalEventJSON(payload []byte) error {
	if len(payload) == 0 {
		return errors.New("terminal event JSON is empty")
	}
	if len(payload) > maxTerminalEventJSONBytes {
		return errors.New("terminal event JSON exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	first, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode terminal event JSON: %w", err)
	}
	if delimiter, ok := first.(json.Delim); !ok || delimiter != '{' {
		return errors.New("terminal event JSON must be an object")
	}
	validator := terminalJSONValidator{}
	if err := validator.validateObject(decoder, 1); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("terminal event JSON has trailing data")
		}
		return fmt.Errorf("decode terminal event JSON trailing data: %w", err)
	}
	return nil
}

func (validator *terminalJSONValidator) validateObject(decoder *json.Decoder, depth int) error {
	if depth > maxTerminalEventJSONDepth {
		return errors.New("terminal event JSON exceeds depth limit")
	}
	seen := make(map[string]struct{})
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode terminal event JSON object key: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return errors.New("terminal event JSON object key is invalid")
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("terminal event JSON contains duplicate key %q", key)
		}
		seen[key] = struct{}{}
		if err := validator.validateValue(decoder, depth); err != nil {
			return err
		}
	}
	end, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode terminal event JSON object end: %w", err)
	}
	if delimiter, ok := end.(json.Delim); !ok || delimiter != '}' {
		return errors.New("terminal event JSON object is not terminated")
	}
	return nil
}

func (validator *terminalJSONValidator) validateArray(decoder *json.Decoder, depth int) error {
	if depth > maxTerminalEventJSONDepth {
		return errors.New("terminal event JSON exceeds depth limit")
	}
	for decoder.More() {
		if err := validator.validateValue(decoder, depth); err != nil {
			return err
		}
	}
	end, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode terminal event JSON array end: %w", err)
	}
	if delimiter, ok := end.(json.Delim); !ok || delimiter != ']' {
		return errors.New("terminal event JSON array is not terminated")
	}
	return nil
}

func (validator *terminalJSONValidator) validateValue(decoder *json.Decoder, parentDepth int) error {
	validator.values++
	if validator.values > maxTerminalEventJSONValues {
		return errors.New("terminal event JSON exceeds value limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode terminal event JSON value: %w", err)
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		return validator.validateObject(decoder, parentDepth+1)
	case '[':
		return validator.validateArray(decoder, parentDepth+1)
	default:
		return errors.New("terminal event JSON contains an unexpected delimiter")
	}
}

func decodeTerminalRawObject(payload []byte) (terminalRawObject, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	first, err := decoder.Token()
	if err != nil {
		return terminalRawObject{}, err
	}
	if delimiter, ok := first.(json.Delim); !ok || delimiter != '{' {
		return terminalRawObject{}, errors.New("terminal raw JSON must be an object")
	}
	result := terminalRawObject{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return terminalRawObject{}, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return terminalRawObject{}, errors.New("terminal raw JSON key is invalid")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return terminalRawObject{}, err
		}
		result.members = append(result.members, terminalRawMember{
			name: key, value: append(json.RawMessage(nil), value...),
		})
	}
	if _, err := decoder.Token(); err != nil {
		return terminalRawObject{}, err
	}
	return result, nil
}

func encodeTerminalRawObject(object terminalRawObject) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	for index, member := range object.members {
		if index > 0 {
			buffer.WriteByte(',')
		}
		name, err := json.Marshal(member.name)
		if err != nil {
			return nil, err
		}
		buffer.Write(name)
		buffer.WriteByte(':')
		buffer.Write(member.value)
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}

func terminalRawObjectHasUnknown(object terminalRawObject, schema terminalJSONSchema) (bool, error) {
	hasUnknown := false
	for _, member := range object.members {
		child, known, err := terminalSchemaMember(schema, member.name)
		if err != nil {
			return false, err
		}
		if !known {
			hasUnknown = true
			continue
		}
		if child == nil || len(bytes.TrimSpace(member.value)) == 0 || bytes.TrimSpace(member.value)[0] != '{' {
			continue
		}
		nested, err := decodeTerminalRawObject(member.value)
		if err != nil {
			return false, err
		}
		nestedHasUnknown, err := terminalRawObjectHasUnknown(nested, child)
		if err != nil {
			return false, err
		}
		hasUnknown = hasUnknown || nestedHasUnknown
	}
	return hasUnknown, nil
}

func mergeTerminalRootObject(
	original terminalRawObject,
	baseline terminalRawObject,
	current terminalRawObject,
	schema terminalJSONSchema,
) (terminalRawObject, error) {
	baselineValues := terminalRawValues(baseline)
	currentValues := terminalRawValues(current)
	seenKnown := make(map[string]struct{}, len(schema))
	merged := terminalRawObject{members: make([]terminalRawMember, 0, len(original.members)+len(current.members))}
	for _, member := range original.members {
		child, known, err := terminalSchemaMember(schema, member.name)
		if err != nil {
			return terminalRawObject{}, err
		}
		if !known {
			merged.members = append(merged.members, cloneTerminalRawMember(member))
			continue
		}
		seenKnown[member.name] = struct{}{}
		baselineValue, baselineOK := baselineValues[member.name]
		currentValue, currentOK := currentValues[member.name]
		if baselineOK == currentOK && (!baselineOK || bytes.Equal(baselineValue, currentValue)) {
			merged.members = append(merged.members, cloneTerminalRawMember(member))
			continue
		}
		if !currentOK {
			continue
		}
		value, err := mergeTerminalKnownValue(member.value, currentValue, child)
		if err != nil {
			return terminalRawObject{}, err
		}
		merged.members = append(merged.members, terminalRawMember{name: member.name, value: value})
	}
	for _, member := range current.members {
		_, known, err := terminalSchemaMember(schema, member.name)
		if err != nil {
			return terminalRawObject{}, err
		}
		if !known {
			continue
		}
		if _, exists := seenKnown[member.name]; exists {
			continue
		}
		merged.members = append(merged.members, cloneTerminalRawMember(member))
	}
	return merged, nil
}

func mergeTerminalKnownObject(original, current terminalRawObject, schema terminalJSONSchema) (terminalRawObject, error) {
	currentValues := terminalRawValues(current)
	seenKnown := make(map[string]struct{}, len(schema))
	merged := terminalRawObject{members: make([]terminalRawMember, 0, len(original.members)+len(current.members))}
	for _, member := range original.members {
		child, known, err := terminalSchemaMember(schema, member.name)
		if err != nil {
			return terminalRawObject{}, err
		}
		if !known {
			merged.members = append(merged.members, cloneTerminalRawMember(member))
			continue
		}
		seenKnown[member.name] = struct{}{}
		currentValue, exists := currentValues[member.name]
		if !exists {
			continue
		}
		value, err := mergeTerminalKnownValue(member.value, currentValue, child)
		if err != nil {
			return terminalRawObject{}, err
		}
		merged.members = append(merged.members, terminalRawMember{name: member.name, value: value})
	}
	for _, member := range current.members {
		_, known, err := terminalSchemaMember(schema, member.name)
		if err != nil {
			return terminalRawObject{}, err
		}
		if !known {
			continue
		}
		if _, exists := seenKnown[member.name]; exists {
			continue
		}
		merged.members = append(merged.members, cloneTerminalRawMember(member))
	}
	return merged, nil
}

func mergeTerminalKnownValue(original, current json.RawMessage, schema terminalJSONSchema) (json.RawMessage, error) {
	if schema == nil || len(bytes.TrimSpace(original)) == 0 || len(bytes.TrimSpace(current)) == 0 ||
		bytes.TrimSpace(original)[0] != '{' || bytes.TrimSpace(current)[0] != '{' {
		return append(json.RawMessage(nil), current...), nil
	}
	originalObject, err := decodeTerminalRawObject(original)
	if err != nil {
		return nil, err
	}
	currentObject, err := decodeTerminalRawObject(current)
	if err != nil {
		return nil, err
	}
	merged, err := mergeTerminalKnownObject(originalObject, currentObject, schema)
	if err != nil {
		return nil, err
	}
	encoded, err := encodeTerminalRawObject(merged)
	return json.RawMessage(encoded), err
}

func terminalSchemaMember(schema terminalJSONSchema, name string) (terminalJSONSchema, bool, error) {
	if child, ok := schema[name]; ok {
		return child, true, nil
	}
	for known := range schema {
		if strings.EqualFold(known, name) {
			return nil, false, fmt.Errorf("terminal event JSON contains noncanonical known key %q", name)
		}
	}
	return nil, false, nil
}

func terminalRawValues(object terminalRawObject) map[string]json.RawMessage {
	values := make(map[string]json.RawMessage, len(object.members))
	for _, member := range object.members {
		values[member.name] = member.value
	}
	return values
}

func cloneTerminalRawMember(member terminalRawMember) terminalRawMember {
	return terminalRawMember{name: member.name, value: append(json.RawMessage(nil), member.value...)}
}
