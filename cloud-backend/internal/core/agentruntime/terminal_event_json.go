package agentruntime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tangying-ai/aios-core/internal/core/observability"
)

// legacyPreparedObservability is intentionally private. It only preserves a
// Round-1 frozen value until a claimed outbox row can validate and replace it.
func (event *RunTerminalEvent) UnmarshalJSON(payload []byte) error {
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
	raw := bytes.TrimSpace(wire.Prepared)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
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
	return nil
}

func (event RunTerminalEvent) MarshalJSON() ([]byte, error) {
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
